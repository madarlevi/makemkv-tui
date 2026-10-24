package batchstate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFreeBSDStateDirectory(t *testing.T) {
	// FreeBSD root's HOME is /root. Test the resolver independently of host OS.
	home := filepath.FromSlash("/root")
	if !filepath.IsAbs(home) {
		home = filepath.Join(filepath.VolumeName(t.TempDir())+string(os.PathSeparator), "root")
	}
	got, e := stateDirectory("", home)
	want := filepath.Join(home, ".local", "state", "makemkv-tui", "batches")
	if e != nil || got != want {
		t.Fatal(got, e, want)
	}
	xdg := filepath.Join(t.TempDir(), "state 季節 with spaces")
	got, e = stateDirectory(xdg, home)
	if e != nil || got != filepath.Join(xdg, "makemkv-tui", "batches") {
		t.Fatal(got, e)
	}
	if _, e = stateDirectory("relative", home); e == nil {
		t.Fatal("relative XDG accepted")
	}
	if _, e = stateDirectory("", ""); e == nil {
		t.Fatal("missing home accepted")
	}
}

func TestPlanPathOverrideAndStableID(t *testing.T) {
	s := fixture(t)
	s.InputRoot = filepath.Join(t.TempDir(), "SG-1 季節")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	a, e := PlanPath("", s)
	if e != nil {
		t.Fatal(e)
	}
	b, _ := PlanPath("", s)
	if a != b || !strings.Contains(filepath.Base(a), s.ID) || !strings.HasSuffix(a, ".json") {
		t.Fatal(a, b)
	}
	t.Setenv("XDG_STATE_HOME", "invalid relative")
	legacy := filepath.Join(t.TempDir(), "ISO source 季節", ".batch-state.json")
	a, e = PlanPath(legacy, s)
	if e != nil || a != legacy {
		t.Fatal(a, e)
	}
	store, e := Open(a)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	if e = store.Initialize(s); e != nil {
		t.Fatal(e)
	}
	loaded, e := store.Load()
	if e != nil || loaded.ID != s.ID || !loaded.ISOs[0].Titles[0].Selected {
		t.Fatal(loaded, e)
	}
	loaded.Operation = "legacy resumed"
	if e = store.Save(loaded); e != nil {
		t.Fatal(e)
	}
	if store.Path != a {
		t.Fatal("legacy plan migrated")
	}
}

func TestCatalogMatchingUnfinishedPlans(t *testing.T) {
	dir := t.TempDir()
	s := fixture(t)
	store, e := Open(filepath.Join(dir, "movie.json"))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	if e = store.Initialize(s); e != nil {
		t.Fatal(e)
	}
	plans, e := Plans(dir)
	if e != nil || len(plans) != 1 {
		t.Fatal(plans, e)
	}
	match, e := Matching(plans, s)
	if e != nil || match != store.Path {
		t.Fatal(match, e)
	}
	plans[0].State.Status = Completed
	if match, e = Matching(plans, s); e != nil || match != "" {
		t.Fatal(match, e)
	}
	if e = os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{"), 0600); e != nil {
		t.Fatal(e)
	}
	plans, e = Plans(dir)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Matching(plans, s); e == nil {
		t.Fatal("corrupt plan silently ignored")
	}
}
