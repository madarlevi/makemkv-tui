// Command release builds deterministic frontend archives without proprietary engines.
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var versionRE = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?$`)
var targets = map[string]bool{"freebsd/amd64": true, "linux/amd64": true, "windows/amd64": true, "darwin/amd64": true, "darwin/arm64": true}
var documents = []string{"LICENSE", "README.md", "THIRD_PARTY.md", "THIRD_PARTY_LICENSES.txt"}

func main() {
	version := flag.String("version", "", "version tag, e.g. v0.1.0")
	target := flag.String("target", "", "OS/architecture")
	out := flag.String("out", "dist", "archive directory")
	checksums := flag.Bool("checksums", false, "write SHA256SUMS for archives in --out")
	flag.Parse()
	var err error
	if *checksums {
		err = sums(*out)
	} else {
		err = build(*version, *target, *out)
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

func build(version, target, out string) error {
	if !versionRE.MatchString(version) || !targets[target] {
		return fmt.Errorf("require a valid version tag and supported --target OS/architecture")
	}
	stamp, e := epoch()
	if e != nil {
		return e
	}
	if e = os.MkdirAll(out, 0755); e != nil {
		return e
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
	cmd := exec.Command("go", "build", "-mod=readonly", "-trimpath", "-buildvcs=false", "-ldflags", "-s -w -buildid= -X main.version="+version, "-o", binaryPath, "./cmd/makemkv-tui")
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
func sums(dir string) error {
	entries, e := os.ReadDir(dir)
	if e != nil {
		return e
	}
	var b strings.Builder
	for _, f := range entries {
		if f.IsDir() || (!strings.HasSuffix(f.Name(), ".tar.gz") && !strings.HasSuffix(f.Name(), ".zip")) {
			continue
		}
		if !strings.HasPrefix(f.Name(), "makemkv-tui-v") {
			return fmt.Errorf("unexpected archive %q", f.Name())
		}
		file, e := os.Open(filepath.Join(dir, f.Name()))
		if e != nil {
			return e
		}
		h := sha256.New()
		_, readErr := io.Copy(h, file)
		closeErr := file.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		fmt.Fprintf(&b, "%x  %s\n", h.Sum(nil), f.Name())
	}
	if b.Len() == 0 {
		return fmt.Errorf("no release archives in %s", dir)
	}
	return exclusiveWrite(filepath.Join(dir, "SHA256SUMS"), []byte(b.String()))
}
