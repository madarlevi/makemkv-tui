package backend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEngineReceiptPreservesCompletedFileAndRejectsWrongTitle(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "staging 日本語 with spaces")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(dir, "title_t00.mkv")
	data := []byte("synthetic complete engine artifact")
	if e := os.WriteFile(path, data, 0600); e != nil {
		t.Fatal(e)
	}
	if e := WriteReceipt(dir, "iso:/media/example.iso", 7, "title_t00.mkv"); e != nil {
		t.Fatal(e)
	}
	r, e := ReadReceipt(dir, "iso:/media/example.iso", 7)
	if e != nil || r == nil || !r.EngineSucceeded || r.File != "title_t00.mkv" {
		t.Fatal(r, e)
	}
	if _, e = ReadReceipt(dir, "iso:/media/example.iso", 8); e == nil {
		t.Fatal("wrong title accepted")
	}
	if b, e := os.ReadFile(path); e != nil || string(b) != string(data) {
		t.Fatal("receipt error damaged extraction", e)
	}
}
func TestTruncatedEngineReceiptIsNotSuccess(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, ".engine-success-fragment.tmp")
	if e := os.WriteFile(name, []byte(`{"Version":1,`), 0600); e != nil {
		t.Fatal(e)
	}
	if r, e := ReadReceipt(dir, "iso:/example.iso", 0); r != nil || e == nil || !strings.Contains(e.Error(), "incomplete engine-success receipt") {
		t.Fatal(r, e)
	}
	if _, e := os.Stat(name); e != nil {
		t.Fatal("ambiguous receipt removed", e)
	}
	if e := os.WriteFile(filepath.Join(dir, "title_t00.mkv"), []byte("complete"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := WriteReceipt(dir, "iso:/example.iso", 0, "title_t00.mkv"); e != nil {
		t.Fatal(e)
	}
	if r, e := ReadReceipt(dir, "iso:/example.iso", 0); e != nil || r == nil {
		t.Fatal("valid receipt should recover despite old fragment", r, e)
	}
}
