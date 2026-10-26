package main

import (
	"github.com/madarlevi/makemkv-tui/internal/batchstate"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBatchArgumentOrderAndSpaces(t *testing.T) {
	b, e := parseBatch([]string{"/media/季節 & movies", "--output", "/out with spaces", "--state", "/plans/a.json"}, io.Discard)
	if e != nil || b.input != "/media/季節 & movies" || b.output != "/out with spaces" {
		t.Fatal(b, e)
	}
	b, e = parseBatch([]string{"--resume", "/plans/a.json", "--retry-failed"}, io.Discard)
	if e != nil || !b.retry {
		t.Fatal(b, e)
	}
	for _, args := range [][]string{{"--resume", "a", "--output", "b"}, {"input"}, {"--output"}, {"--unknown"}, {"input", "other", "--output", "out"}} {
		if _, e = parseBatch(args, io.Discard); e == nil {
			t.Fatal("accepted", args)
		}
	}
}

func TestBatchPlanCommands(t *testing.T) {
	for _, args := range [][]string{{"--plans"}, {"--plans", "--retry-failed"}, {"--list"}, {"--state-dir"}, {"input", "--output", "out", "--new"}} {
		if _, e := parseBatch(args, io.Discard); e != nil {
			t.Fatal(args, e)
		}
	}
	for _, args := range [][]string{{"--plans", "--list"}, {"--list", "input"}, {"--list", "--retry-failed"}, {"--resume", "old.json", "--new"}} {
		if _, e := parseBatch(args, io.Discard); e == nil {
			t.Fatal("accepted", args)
		}
	}
}

func TestBatchLocalStorageDuplicateAndLegacyResume(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "ISO 季節 with spaces")
	output := filepath.Join(root, "NAS output")
	if e := os.MkdirAll(input, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(input, "disc.iso"), []byte("test ISO"), 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "local state"))
	t.Setenv("PATH", t.TempDir()) // Engine unavailable: initialization must still persist the plan.
	args := []string{input, "--output", output, "--config", filepath.Join(root, "preferences.json")}
	if e := runBatch(args); e == nil {
		t.Fatal("expected missing engine")
	}
	dir, _ := batchstate.Directory()
	plans, e := batchstate.Plans(dir)
	if e != nil || len(plans) != 1 || plans[0].Err != nil {
		t.Fatal(plans, e)
	}
	path := plans[0].Path
	if plans[0].State.InputRoot == "" || filepath.Dir(path) == output {
		t.Fatal("not local storage")
	}
	if _, e := os.Stat(filepath.Join(input, ".batch-state.json")); !os.IsNotExist(e) {
		t.Fatal(e)
	}
	if e = runBatch(args); e == nil || !strings.Contains(e.Error(), "unfinished matching plan") {
		t.Fatal(e)
	}
	if e = runBatch(append(args, "--new")); e == nil {
		t.Fatal("expected missing engine")
	}
	plans, e = batchstate.Plans(dir)
	if e != nil || len(plans) != 2 {
		t.Fatal(plans, e)
	}
	// Legacy paths resume in place even with an unusable XDG default.
	legacy := filepath.Join(input, ".batch-state.json")
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(legacy, data, 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("XDG_STATE_HOME", "relative-invalid")
	if e = runBatch([]string{"--resume", legacy}); e == nil || strings.Contains(e.Error(), "state directory must be absolute") {
		t.Fatal(e)
	}
	store, e := batchstate.Open(legacy)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	s, e := store.Load()
	if e != nil || s.ID != plans[0].State.ID && s.ID != plans[1].State.ID {
		t.Fatal(s, e)
	}
	stored, e := os.Stat(store.Path)
	if e != nil {
		t.Fatal(e)
	}
	original, e := os.Stat(legacy)
	if e != nil {
		t.Fatal(e)
	}
	// Canonical parent spelling can differ on macOS and Windows. The original
	// explicit legacy path must still name the same updated state file.
	if !os.SameFile(stored, original) || s.ISOs[0].RelativePath != "disc.iso" {
		t.Fatal("legacy file or selection changed", store.Path, legacy)
	}
}
