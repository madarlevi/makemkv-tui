// Release validation deliberately uses Go's own binary and archive readers.
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"debug/buildinfo"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const checksumName = "SHA256SUMS.txt"
const metadataName = "BUILDINFO.json"

var commitRE = regexp.MustCompile(`^[0-9a-f]{40}$`)

type metadata struct {
	SchemaVersion int    `json:"schema_version"`
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	Target        string `json:"target"`
	GoVersion     string `json:"go_version"`
	Epoch         int64  `json:"source_date_epoch"`
	BinarySHA256  string `json:"binary_sha256"`
	Preview       bool   `json:"preview"`
}

func hashBytes(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }
func archiveName(version, target string) string {
	suffix := ".tar.gz"
	if strings.HasPrefix(target, "windows/") {
		suffix = ".zip"
	}
	return "makemkv-tui-" + version + "-" + strings.ReplaceAll(target, "/", "-") + suffix
}
func targetList() []string {
	var names []string
	for t := range targets {
		names = append(names, t)
	}
	sort.Strings(names)
	return names
}
func validateIdentity(version, commit string, preview bool) error {
	if !versionRE.MatchString(version) {
		return fmt.Errorf("invalid semantic version tag %q", version)
	}
	if !commitRE.MatchString(commit) && !(preview && commit == "uncommitted") {
		return fmt.Errorf("require exact lowercase 40-character commit SHA (or --preview --commit uncommitted)")
	}
	return nil
}
func validateBinary(b []byte, target, version, commit, toolchain string) error {
	info, e := buildinfo.Read(bytes.NewReader(b))
	if e != nil {
		return fmt.Errorf("read Go binary metadata: %w", e)
	}
	if info.Path != "github.com/madarlevi/makemkv-tui/cmd/makemkv-tui" {
		return fmt.Errorf("unexpected executable module %s", info.Path)
	}
	settings := map[string]string{}
	for _, s := range info.Settings {
		settings[s.Key] = s.Value
	}
	parts := strings.Split(target, "/")
	if settings["GOOS"] != parts[0] || settings["GOARCH"] != parts[1] || settings["CGO_ENABLED"] != "0" || settings["-trimpath"] != "true" {
		return fmt.Errorf("incorrect binary target/CGO/trimpath metadata: %v", settings)
	}
	if info.GoVersion != "go"+strings.TrimPrefix(toolchain, "go") {
		return fmt.Errorf("unexpected compiler %s, want go%s", info.GoVersion, strings.TrimPrefix(toolchain, "go"))
	}
	identity := "MKVTUI_BUILD_V1|" + version + "|" + commit + "|END_MKVTUI_BUILD_V1"
	if bytes.Count(b, []byte(identity)) != 1 {
		return fmt.Errorf("binary lacks unique embedded version/commit identity")
	}
	switch parts[0] {
	case "freebsd", "linux":
		f, e := elf.NewFile(bytes.NewReader(b))
		if e != nil {
			return e
		}
		defer f.Close()
		if f.Class != elf.ELFCLASS64 || f.Machine != elf.EM_X86_64 {
			return fmt.Errorf("expected ELF64 amd64")
		}
		for _, p := range f.Progs {
			if p.Type == elf.PT_INTERP {
				return fmt.Errorf("unexpected dynamic interpreter in static frontend")
			}
		}
	case "windows":
		f, e := pe.NewFile(bytes.NewReader(b))
		if e != nil {
			return e
		}
		defer f.Close()
		if f.Machine != pe.IMAGE_FILE_MACHINE_AMD64 {
			return fmt.Errorf("expected Windows amd64 PE")
		}
	case "darwin":
		f, e := macho.NewFile(bytes.NewReader(b))
		if e != nil {
			return e
		}
		defer f.Close()
		want := macho.CpuAmd64
		if parts[1] == "arm64" {
			want = macho.CpuArm64
		}
		if f.Cpu != want || f.Type != macho.TypeExec {
			return fmt.Errorf("incorrect Mach-O architecture/type")
		}
	default:
		return fmt.Errorf("unsupported target")
	}
	return nil
}
func archiveFiles(path string) (map[string][]byte, error) {
	files := map[string][]byte{}
	add := func(name string, mode os.FileMode, r io.Reader) error {
		if filepath.Base(name) != name || strings.ContainsAny(name, "/\\") || files[name] != nil || !mode.IsRegular() {
			return fmt.Errorf("unsafe/duplicate archive member %q", name)
		}
		want := os.FileMode(0644)
		if name == "makemkv-tui" || name == "makemkv-tui.exe" {
			want = 0755
		}
		if mode.Perm() != want {
			return fmt.Errorf("incorrect mode %o for %s", mode.Perm(), name)
		}
		b, e := io.ReadAll(io.LimitReader(r, 128*1024*1024+1))
		if e != nil {
			return e
		}
		if len(b) > 128*1024*1024 {
			return fmt.Errorf("oversized archive member")
		}
		files[name] = b
		return nil
	}
	if strings.HasSuffix(path, ".zip") {
		z, e := zip.OpenReader(path)
		if e != nil {
			return nil, e
		}
		defer z.Close()
		for _, f := range z.File {
			r, e := f.Open()
			if e != nil {
				return nil, e
			}
			e = add(f.Name, f.Mode(), r)
			ce := r.Close()
			if e != nil {
				return nil, e
			}
			if ce != nil {
				return nil, ce
			}
		}
	} else {
		f, e := os.Open(path)
		if e != nil {
			return nil, e
		}
		defer f.Close()
		gz, e := gzip.NewReader(f)
		if e != nil {
			return nil, e
		}
		defer gz.Close()
		tr := tar.NewReader(gz)
		for {
			h, e := tr.Next()
			if e == io.EOF {
				break
			}
			if e != nil {
				return nil, e
			}
			if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
				return nil, fmt.Errorf("non-regular tar member")
			}
			if e = add(h.Name, h.FileInfo().Mode(), tr); e != nil {
				return nil, e
			}
		}
		// Consume the gzip trailer to verify its CRC even if tar stopped at EOF blocks.
		if _, e = io.Copy(io.Discard, gz); e != nil {
			return nil, e
		}
	}
	return files, nil
}
func inspectArchive(path, target, version, commit, toolchain string, preview bool) (metadata, map[string][]byte, error) {
	var m metadata
	files, e := archiveFiles(path)
	if e != nil {
		return m, nil, e
	}
	binary := "makemkv-tui"
	if strings.HasPrefix(target, "windows/") {
		binary += ".exe"
	}
	expected := append(append([]string{}, documents...), binary, metadataName)
	if len(files) != len(expected) {
		return m, nil, fmt.Errorf("unexpected archive members: got %d, need %d", len(files), len(expected))
	}
	for _, n := range expected {
		if len(files[n]) == 0 {
			return m, nil, fmt.Errorf("missing/empty archive member %s", n)
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(files[metadataName]))
	decoder.DisallowUnknownFields()
	if e = decoder.Decode(&m); e != nil {
		return m, nil, e
	}
	if decoder.Decode(new(any)) != io.EOF {
		return m, nil, fmt.Errorf("trailing metadata")
	}
	if m.SchemaVersion != 1 || m.Version != version || m.Commit != commit || m.Target != target || m.Preview != preview || m.GoVersion != "go"+strings.TrimPrefix(toolchain, "go") || m.Epoch < 0 {
		return m, nil, fmt.Errorf("artifact identity/preview/toolchain mismatch")
	}
	if hashBytes(files[binary]) != m.BinarySHA256 {
		return m, nil, fmt.Errorf("binary digest mismatch")
	}
	if !bytes.Contains(files["LICENSE"], []byte("MIT License")) || !bytes.Contains(files["LICENSE"], []byte("madarlevi")) {
		return m, nil, fmt.Errorf("missing frontend MIT license")
	}
	if e = validateBinary(files[binary], target, version, commit, toolchain); e != nil {
		return m, nil, e
	}
	return m, files, nil
}
func validateSet(dir, version, commit, toolchain string, preview, withSums bool) ([]string, error) {
	if e := validateIdentity(version, commit, preview); e != nil {
		return nil, e
	}
	expected := map[string]string{}
	for _, t := range targetList() {
		expected[archiveName(version, t)] = t
	}
	entries, e := os.ReadDir(dir)
	if e != nil {
		return nil, e
	}
	count := len(expected)
	if withSums {
		count++
	}
	if len(entries) != count {
		return nil, fmt.Errorf("release directory must contain exactly %d files; stale/missing artifacts refused", count)
	}
	var names []string
	var epoch int64 = -1
	docHashes := map[string]string{}
	for _, entry := range entries {
		if entry.Name() == checksumName && withSums {
			continue
		}
		target, ok := expected[entry.Name()]
		if !ok || !entry.Type().IsRegular() {
			return nil, fmt.Errorf("unexpected release entry %q", entry.Name())
		}
		m, files, e := inspectArchive(filepath.Join(dir, entry.Name()), target, version, commit, toolchain, preview)
		if e != nil {
			return nil, fmt.Errorf("validate %s: %w", entry.Name(), e)
		}
		if epoch != -1 && epoch != m.Epoch {
			return nil, fmt.Errorf("mixed source timestamps")
		}
		epoch = m.Epoch
		for _, d := range documents {
			h := hashBytes(files[d])
			if prev := docHashes[d]; prev != "" && prev != h {
				return nil, fmt.Errorf("mixed source documentation %s", d)
			}
			docHashes[d] = h
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	if withSums {
		b, e := os.ReadFile(filepath.Join(dir, checksumName))
		if e != nil {
			return nil, e
		}
		want, e := checksumData(dir, names)
		if e != nil {
			return nil, e
		}
		if !bytes.Equal(b, want) {
			return nil, fmt.Errorf("checksum file differs from actual final archives")
		}
	}
	return names, nil
}
func checksumData(dir string, names []string) ([]byte, error) {
	var b strings.Builder
	for _, n := range names {
		data, e := os.ReadFile(filepath.Join(dir, n))
		if e != nil {
			return nil, e
		}
		fmt.Fprintf(&b, "%s  %s\n", hashBytes(data), n)
	}
	return []byte(b.String()), nil
}
