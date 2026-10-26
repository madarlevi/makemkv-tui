// Command release builds deterministic frontend archives without proprietary engines.
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

var versionRE = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?$`)
var targets = map[string]bool{"freebsd/amd64": true, "linux/amd64": true, "windows/amd64": true, "darwin/amd64": true, "darwin/arm64": true}
var documents = []string{"LICENSE", "README.md", "THIRD_PARTY.md", "THIRD_PARTY_LICENSES.txt"}

func main() {
	version := flag.String("version", "", "semantic version tag")
	target := flag.String("target", "", "OS/architecture, or all")
	out := flag.String("out", "dist/release", "isolated release output directory")
	commit := flag.String("commit", "", "exact source commit SHA")
	toolchain := flag.String("go-version", "", "required Go version (default .go-version)")
	preview := flag.Bool("preview", false, "local simulation; artifacts cannot be published")
	checksums := flag.Bool("checksums", false, "validate all five archives and create SHA256SUMS.txt")
	verify := flag.Bool("verify", false, "validate all five archives and existing checksums")
	flag.Parse()
	if *toolchain == "" {
		b, e := os.ReadFile(".go-version")
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		*toolchain = strings.TrimSpace(string(b))
	}
	var err error
	switch {
	case *checksums && *verify:
		err = fmt.Errorf("choose checksums or verify")
	case *checksums:
		err = writeSums(*out, *version, *commit, *toolchain, *preview)
	case *verify:
		_, err = validateSet(*out, *version, *commit, *toolchain, *preview, true)
	case *target == "all":
		for _, t := range targetList() {
			if err = build(*version, t, *out, *commit, *toolchain, *preview); err != nil {
				break
			}
		}
	default:
		err = build(*version, *target, *out, *commit, *toolchain, *preview)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func epoch() (time.Time, error) {
	s := os.Getenv("SOURCE_DATE_EPOCH")
	if s == "" {
		s = "0"
	}
	n, e := strconv.ParseInt(s, 10, 64)
	if e != nil || n < 0 {
		return time.Time{}, fmt.Errorf("invalid SOURCE_DATE_EPOCH %q", s)
	}
	return time.Unix(n, 0).UTC(), nil
}

func build(version, target, out, commit, toolchain string, preview bool) error {
	if !versionRE.MatchString(version) || !targets[target] {
		return fmt.Errorf("require a valid version tag and supported --target OS/architecture")
	}
	if e := validateIdentity(version, commit, preview); e != nil {
		return e
	}
	if runtime.Version() != "go"+strings.TrimPrefix(toolchain, "go") {
		return fmt.Errorf("use pinned Go %s, have %s", toolchain, runtime.Version())
	}
	stamp, e := epoch()
	if e != nil {
		return e
	}
	if e = os.MkdirAll(out, 0755); e != nil {
		return e
	}
	entries, e := os.ReadDir(out)
	if e != nil {
		return e
	}
	for _, entry := range entries {
		known := false
		for _, t := range targetList() {
			if entry.Name() == archiveName(version, t) && entry.Type().IsRegular() {
				if _, _, e = inspectArchive(filepath.Join(out, entry.Name()), t, version, commit, toolchain, preview); e != nil {
					return e
				}
				known = true
				break
			}
		}
		if !known {
			return fmt.Errorf("choose a clean output directory; unexpected %s", entry.Name())
		}
	}
	scratch, e := os.MkdirTemp(out, ".release-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(scratch)
	parts := strings.Split(target, "/")
	binary := "makemkv-tui"
	extension := ".tar.gz"
	if parts[0] == "windows" {
		binary += ".exe"
		extension = ".zip"
	}
	binaryPath, e := filepath.Abs(filepath.Join(scratch, binary))
	if e != nil {
		return e
	}
	identity := "MKVTUI_BUILD_V1|" + version + "|" + commit + "|END_MKVTUI_BUILD_V1"
	cmd := exec.Command("go", "build", "-mod=readonly", "-trimpath", "-buildvcs=false", "-ldflags", "-s -w -buildid= -X main.releaseIdentity="+identity, "-o", binaryPath, "./cmd/makemkv-tui")
	// Remove inherited target settings before appending explicit build values.
	for _, v := range os.Environ() {
		k, _, _ := strings.Cut(v, "=")
		if !strings.EqualFold(k, "GOOS") && !strings.EqualFold(k, "GOARCH") && !strings.EqualFold(k, "CGO_ENABLED") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	cmd.Env = append(cmd.Env, "GOOS="+parts[0], "GOARCH="+parts[1], "CGO_ENABLED=0")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if e = cmd.Run(); e != nil {
		return fmt.Errorf("build %s: %w", target, e)
	}
	if parts[0] == runtime.GOOS && parts[1] == runtime.GOARCH {
		output, e := exec.Command(binaryPath, "--version").Output()
		if e != nil || strings.TrimSpace(string(output)) != "makemkv-tui "+version {
			return fmt.Errorf("native release version smoke failed: %q: %v", output, e)
		}
		output, e = exec.Command(binaryPath, "version", "--json").Output()
		var info map[string]string
		if e != nil || json.Unmarshal(output, &info) != nil || info["version"] != version || info["commit"] != commit || info["go_version"] != runtime.Version() {
			return fmt.Errorf("native JSON identity smoke failed: %q: %v", output, e)
		}
	}
	files := map[string][]byte{}
	files[binary], e = os.ReadFile(binaryPath)
	if e != nil {
		return e
	}
	for _, name := range documents {
		files[name], e = os.ReadFile(name)
		if e != nil {
			return e
		}
	}
	if e = validateBinary(files[binary], target, version, commit, toolchain); e != nil {
		return e
	}
	meta := metadata{1, version, commit, target, runtime.Version(), stamp.Unix(), hashBytes(files[binary]), preview}
	files[metadataName], e = json.MarshalIndent(meta, "", "  ")
	if e != nil {
		return e
	}
	files[metadataName] = append(files[metadataName], '\n')
	var content bytes.Buffer
	if parts[0] == "windows" {
		e = zipArchive(&content, files, binary, stamp)
	} else {
		e = tarArchive(&content, files, binary, stamp)
	}
	if e != nil {
		return e
	}
	name := "makemkv-tui-" + version + "-" + parts[0] + "-" + parts[1] + extension
	dest := filepath.Join(out, name)
	// Release packaging also refuses accidental overwrites of existing artifacts.
	if e = exclusiveWrite(dest, content.Bytes()); e != nil {
		return e
	}
	if _, _, e = inspectArchive(dest, target, version, commit, toolchain, preview); e != nil {
		return e
	}
	fmt.Println(dest)
	return nil
}

func sortedFiles(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
func tarArchive(w io.Writer, files map[string][]byte, binary string, stamp time.Time) error {
	gz := gzip.NewWriter(w)
	gz.Header.ModTime = stamp
	tw := tar.NewWriter(gz)
	for _, n := range sortedFiles(files) {
		mode := int64(0644)
		if n == binary {
			mode = 0755
		}
		if e := tw.WriteHeader(&tar.Header{Name: n, Mode: mode, Size: int64(len(files[n])), ModTime: stamp, Format: tar.FormatPAX}); e != nil {
			return e
		}
		if _, e := tw.Write(files[n]); e != nil {
			return e
		}
	}
	if e := tw.Close(); e != nil {
		return e
	}
	return gz.Close()
}
func zipArchive(w io.Writer, files map[string][]byte, binary string, stamp time.Time) error {
	if stamp.Year() < 1980 {
		stamp = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	zw := zip.NewWriter(w)
	for _, n := range sortedFiles(files) {
		h := &zip.FileHeader{Name: n, Method: zip.Deflate, Modified: stamp}
		mode := os.FileMode(0644)
		if n == binary {
			mode = 0755
		}
		h.SetMode(mode)
		f, e := zw.CreateHeader(h)
		if e != nil {
			return e
		}
		if _, e = f.Write(files[n]); e != nil {
			return e
		}
	}
	return zw.Close()
}
func exclusiveWrite(path string, b []byte) (err error) {
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if e != nil {
		return e
	}
	defer func() {
		if err != nil {
			_ = os.Remove(path)
		}
	}()
	if n, e := f.Write(b); e != nil {
		err = e
	} else if n != len(b) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	return err
}
func writeSums(dir, version, commit, toolchain string, preview bool) error {
	names, e := validateSet(dir, version, commit, toolchain, preview, false)
	if e != nil {
		return e
	}
	b, e := checksumData(dir, names)
	if e != nil {
		return e
	}
	if e = exclusiveWrite(filepath.Join(dir, checksumName), b); e != nil {
		return e
	}
	_, e = validateSet(dir, version, commit, toolchain, preview, true)
	return e
}
