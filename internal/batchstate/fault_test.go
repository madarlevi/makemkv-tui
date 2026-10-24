package batchstate

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type faultyFile struct {
	stateFile
	fault string
}

func (f faultyFile) Write(b []byte) (int, error) {
	if f.fault == "write" {
		n, _ := f.stateFile.Write(b[:len(b)/2])
		return n, errors.New("injected write failure")
	}
	if f.fault == "short write" {
		return f.stateFile.Write(b[:len(b)/2])
	}
	return f.stateFile.Write(b)
}
func (f faultyFile) Sync() error {
	if f.fault == "sync" {
		return errors.New("injected sync failure")
	}
	return f.stateFile.Sync()
}
func (f faultyFile) Close() error {
	e := f.stateFile.Close()
	if f.fault == "close" {
		return errors.Join(e, errors.New("injected close failure"))
	}
	return e
}

func TestReplacementFailurePreservesPreviousJSON(t *testing.T) {
	for _, fault := range []string{"write", "short write", "sync", "close", "rename"} {
		t.Run(fault, func(t *testing.T) {
			store, e := Open(filepath.Join(t.TempDir(), "plan 季節 with spaces.json"))
			if e != nil {
				t.Fatal(e)
			}
			defer store.Close()
			s := fixture(t)
			if e = store.Initialize(s); e != nil {
				t.Fatal(e)
			}
			before, _ := os.ReadFile(store.Path)
			create := store.createTemp
			store.createTemp = func(name string) (stateFile, error) {
				f, e := create(name)
				if e != nil {
					return nil, e
				}
				return faultyFile{f, fault}, nil
			}
			if fault == "rename" {
				store.rename = func(string, string) error { return errors.New("injected rename failure") }
			}
			s.Operation = "new transition"
			e = store.Save(s)
			if e == nil {
				t.Fatal("failure ignored")
			}
			if fault == "short write" && !errors.Is(e, io.ErrShortWrite) {
				t.Fatal(e)
			}
			after, _ := os.ReadFile(store.Path)
			if !bytes.Equal(before, after) {
				t.Fatal("previous JSON changed")
			}
			if _, e = store.Load(); e != nil {
				t.Fatal(e)
			}
			entries, _ := os.ReadDir(filepath.Dir(store.Path))
			for _, entry := range entries {
				if strings.HasSuffix(entry.Name(), ".tmp") {
					t.Fatal("temporary file leaked")
				}
			}
		})
	}
}

func TestRenamePublicationAndDirectorySync(t *testing.T) {
	store, e := Open(filepath.Join(t.TempDir(), "state.json"))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	s := fixture(t)
	renames := 0
	store.rename = func(a, b string) error {
		if filepath.Dir(a) != filepath.Dir(b) {
			t.Fatal("cross-directory replacement")
		}
		renames++
		return os.Rename(a, b)
	}
	if e = store.Initialize(s); e != nil {
		t.Fatal(e)
	}
	s.Operation = "replaced"
	if e = store.Save(s); e != nil {
		t.Fatal(e)
	}
	q, e := store.Load()
	if e != nil || q.Operation != "replaced" || renames != 2 {
		t.Fatal(q, e, renames)
	}
	store.syncParent = func(string) error { return errors.New("injected directory sync failure") }
	s.Operation = "published but not confirmed durable"
	if e = store.Save(s); e == nil || !strings.Contains(e.Error(), "durability unconfirmed") {
		t.Fatal(e)
	}
	q, e = store.Load()
	if e != nil || q.Operation != s.Operation {
		t.Fatal(q, e)
	}
}
