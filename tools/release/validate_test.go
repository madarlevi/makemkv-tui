package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchiveReaderRejectsTraversalDuplicateAndModes(t *testing.T) {
	for _, kind := range []string{"traversal", "duplicate", "mode"} {
		t.Run(kind, func(t *testing.T) {
			var b bytes.Buffer
			z := zip.NewWriter(&b)
			name := "README.md"
			mode := os.FileMode(0644)
			if kind == "traversal" {
				name = "../README.md"
			}
			if kind == "mode" {
				mode = 0777
			}
			h := &zip.FileHeader{Name: name}
			h.SetMode(mode)
			w, e := z.CreateHeader(h)
			if e != nil {
				t.Fatal(e)
			}
			w.Write([]byte("doc"))
			if kind == "duplicate" {
				h2 := &zip.FileHeader{Name: name}
				h2.SetMode(mode)
				w, e = z.CreateHeader(h2)
				if e != nil {
					t.Fatal(e)
				}
				w.Write([]byte("other"))
			}
			if e = z.Close(); e != nil {
				t.Fatal(e)
			}
			p := filepath.Join(t.TempDir(), "bad.zip")
			if e = os.WriteFile(p, b.Bytes(), 0600); e != nil {
				t.Fatal(e)
			}
			if _, e = archiveFiles(p); e == nil {
				t.Fatal("unsafe archive accepted")
			}
		})
	}
}
func TestReleaseIdentityAndStaleDirectory(t *testing.T) {
	commit := strings.Repeat("a", 40)
	if validateIdentity("v1.2.3", commit, false) != nil {
		t.Fatal("valid tag refused")
	}
	if validateIdentity("v1.2.3", "uncommitted", false) == nil {
		t.Fatal("uncommitted production release")
	}
	if validateIdentity("v1.2.3", "uncommitted", true) != nil {
		t.Fatal("explicit preview refused")
	}
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "stale.zip"), []byte("old"), 0600)
	if _, e := validateSet(d, "v1.2.3", commit, "1.26.10", false, false); e == nil {
		t.Fatal("partial stale set accepted")
	}
}
