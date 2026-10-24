package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"testing"
	"time"
)

func TestArchivesReproducibleAndComplete(t *testing.T) {
	files := map[string][]byte{"makemkv-tui": []byte("frontend"), "LICENSE": []byte("license"), "README.md": []byte("readme"), "THIRD_PARTY.md": []byte("notice"), "THIRD_PARTY_LICENSES.txt": []byte("licenses")}
	stamp := time.Unix(1760000000, 0).UTC()
	for _, kind := range []string{"tar", "zip"} {
		var a, b bytes.Buffer
		pack := tarArchive
		if kind == "zip" {
			pack = zipArchive
		}
		if e := pack(&a, files, "makemkv-tui", stamp); e != nil {
			t.Fatal(e)
		}
		if e := pack(&b, files, "makemkv-tui", stamp); e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(a.Bytes(), b.Bytes()) {
			t.Fatal("non-reproducible archive")
		}
		seen := map[string]bool{}
		if kind == "tar" {
			gz, e := gzip.NewReader(bytes.NewReader(a.Bytes()))
			if e != nil {
				t.Fatal(e)
			}
			r := tar.NewReader(gz)
			for {
				h, e := r.Next()
				if e == io.EOF {
					break
				}
				if e != nil {
					t.Fatal(e)
				}
				data, e := io.ReadAll(r)
				if e != nil || !bytes.Equal(data, files[h.Name]) {
					t.Fatal("archive data mismatch")
				}
				if h.Name == "makemkv-tui" && h.Mode != 0755 {
					t.Fatal("executable mode")
				}
				seen[h.Name] = true
			}
			gz.Close()
		} else {
			r, e := zip.NewReader(bytes.NewReader(a.Bytes()), int64(a.Len()))
			if e != nil {
				t.Fatal(e)
			}
			for _, f := range r.File {
				rc, e := f.Open()
				if e != nil {
					t.Fatal(e)
				}
				data, e := io.ReadAll(rc)
				rc.Close()
				if e != nil || !bytes.Equal(data, files[f.Name]) {
					t.Fatal("zip data mismatch")
				}
				seen[f.Name] = true
			}
		}
		if len(seen) != len(files) {
			t.Fatal("missing license or executable")
		}
	}
}
func TestVersionValidation(t *testing.T) {
	for _, s := range []string{"v0.1.0;echo bad", "../v1.0.0", "v1", "invalid-owner"} {
		if versionRE.MatchString(s) {
			t.Fatalf("unsafe tag %s", s)
		}
	}
}
