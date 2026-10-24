package batchstate

import (
	"bytes"
	"errors"
	"github.com/madarlevi/makemkv-tui/internal/media"
	"github.com/madarlevi/makemkv-tui/internal/preferences"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestInitializeNeverOverwritesExistingPlan(t *testing.T) {
	store, e := Open(filepath.Join(t.TempDir(), "state.json"))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	s := fixture(t)
	if e = store.Initialize(s); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(store.Path)
	s.Operation = "replacement"
	if store.Initialize(s) == nil {
		t.Fatal("existing plan was replaced")
	}
	after, _ := os.ReadFile(store.Path)
	if !bytes.Equal(before, after) {
		t.Fatal("original changed")
	}
}

func fixture(t *testing.T) *State {
	t.Helper()
	root := t.TempDir()
	p := preferences.Defaults()
	return &State{SchemaVersion: SchemaVersion, ID: ID(), InputRoot: root, OutputRoot: root, Status: Ready, CurrentISO: -1, CurrentTitle: -1, Preferences: p, CreatedAt: time.Now().UTC(), ISOs: []ISO{{RelativePath: "movie.iso", Reviewed: true, Status: Pending, Preferences: p, Titles: []Title{{ID: 7, Selected: true, Status: Pending, Output: "movie/title7.mkv", Metadata: &media.Title{ID: 7, Streams: map[int]map[int]string{4: {3: "jpn"}}}}}}}}
}
func TestAtomicSaveLoadAndSelection(t *testing.T) {
	s := fixture(t)
	path := filepath.Join(t.TempDir(), "state.json")
	store, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	if e = store.Save(s); e != nil {
		t.Fatal(e)
	}
	q, e := store.Load()
	if e != nil || !q.ISOs[0].Titles[0].Selected || q.ISOs[0].Titles[0].Metadata.Streams[4][3] != "jpn" {
		t.Fatal(q, e)
	}
	before, _ := os.ReadFile(path)
	store.beforeReplace = func() error { return errors.New("disk fault") }
	s.Operation = "changed"
	if store.Save(s) == nil {
		t.Fatal("fault ignored")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("old plan damaged")
	}
	store.beforeReplace = nil
	if e = store.Save(s); e != nil {
		t.Fatal(e)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, f := range entries {
		if filepath.Ext(f.Name()) == ".tmp" {
			t.Fatal("temp leaked")
		}
	}
}
func TestSchemaCorruptionAndInvariants(t *testing.T) {
	store, e := Open(filepath.Join(t.TempDir(), "state.json"))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	for _, mutate := range []func(*State){func(s *State) { s.SchemaVersion = 999 }, func(s *State) { s.ISOs[0].RelativePath = "../escape.iso" }, func(s *State) { s.ISOs[0].Titles[0].Output = "../escape.mkv" }, func(s *State) { s.Status = "unknown" }, func(s *State) { s.ISOs[0].Titles[0].Status = Completed }, func(s *State) { s.ISOs[0].Titles[0].AttemptID = "../bad" }} {
		s := fixture(t)
		mutate(s)
		if store.Save(s) == nil {
			t.Fatal("invalid state accepted")
		}
	}
	for _, data := range []string{"{", "{\"SchemaVersion\":999}"} {
		os.WriteFile(store.Path, []byte(data), 0600)
		if _, e = store.Load(); e == nil {
			t.Fatal("corrupt state accepted")
		}
		after, _ := os.ReadFile(store.Path)
		if string(after) != data {
			t.Fatal("state reset")
		}
	}
}
func TestDuplicateInstance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	one, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	if two, e := Open(path); e == nil {
		two.Close()
		t.Fatal("second writer acquired lock")
	}
	one.Close()
	two, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	two.Close()
}
func TestLockCrashHelper(t *testing.T) {
	if os.Getenv("BATCH_LOCK_HELPER") != "1" {
		return
	}
	s, e := Open(os.Getenv("BATCH_LOCK_PATH"))
	if e != nil {
		os.Exit(19)
	}
	_ = s
	os.Exit(23)
}
func TestStaleLockAfterProcessExit(t *testing.T) {
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "state.json")
	cmd := exec.Command(exe, "-test.run=^TestLockCrashHelper$")
	cmd.Env = append(os.Environ(), "BATCH_LOCK_HELPER=1", "BATCH_LOCK_PATH="+path)
	if e = cmd.Run(); e == nil {
		t.Fatal("helper did not exit abruptly")
	}
	store, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	if !store.StaleLock {
		t.Fatal("stale lock not detected")
	}
}
