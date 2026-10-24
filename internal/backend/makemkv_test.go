package backend

import (
	"context"
	"github.com/madarlevi/makemkv-tui/internal/preferences"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type engineFake struct {
	calls [][]string
	write bool
}

func (f *engineFake) Run(ctx context.Context, args []string, emit func(string)) error {
	f.calls = append(f.calls, args)
	if f.write {
		return os.WriteFile(filepath.Join(args[len(args)-1], "engine.mkv"), []byte("movie"), 0600)
	}
	emit("TCOUNT:1")
	emit("TINFO:7,2,0,\"Feature\"")
	return nil
}
func TestAdapterAndNames(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.iso")
	os.WriteFile(src, []byte("source"), 0600)
	f := &engineFake{}
	m := MakeMKV{f}
	d, e := m.Scan(context.Background(), "iso:"+src, func(Event) {})
	if e != nil || d.Sorted()[0].ID != 7 {
		t.Fatal(d, e)
	}
	f.write = true
	r := Request{Source: "iso:" + src, Output: dir, IDs: []int{7}, Filenames: map[int]string{7: "My Feature.mkv"}}
	a, e := m.Extract(context.Background(), r, func(Event) {})
	if e != nil || len(a) != 1 || filepath.Base(a[0].Path) != "My Feature.mkv" {
		t.Fatal(a, e)
	}
	if _, e = m.Extract(context.Background(), r, func(Event) {}); e == nil {
		t.Fatal("overwritten output")
	}
}
func TestPublishNoClobber(t *testing.T) {
	d := t.TempDir()
	a, b := filepath.Join(d, "a"), filepath.Join(d, "b")
	os.WriteFile(a, []byte("new"), 0600)
	os.WriteFile(b, []byte("old"), 0600)
	if Publish(a, b) == nil {
		t.Fatal("overwritten")
	}
	s, _ := os.ReadFile(b)
	if string(s) != "old" {
		t.Fatal(string(s))
	}
}
func TestRemuxIDs(t *testing.T) {
	inv := Inventory{Tracks: []Track{{ID: 2, Type: "video"}, {ID: 8, Type: "audio"}, {ID: 15, Type: "audio"}, {ID: 22, Type: "subtitles"}}}
	args, e := RemuxArgs("/input movie.mkv", "/output movie.mkv", inv, map[int]bool{8: true})
	if e != nil {
		t.Fatal(e)
	}
	s := strings.Join(args, "|")
	if !strings.Contains(s, "--audio-tracks|8|--no-subtitles|/input movie.mkv") {
		t.Fatal(args)
	}
	if _, e = RemuxArgs("a", "b", inv, map[int]bool{123: true}); e == nil {
		t.Fatal("unknown track ID accepted")
	}
}
func TestFilenameValidation(t *testing.T) {
	for _, s := range []string{"../bad.mkv", "a/b.mkv", "bad.txt", "a\n.mkv", "x:.mkv"} {
		if preferences.ValidateName(s) == nil {
			t.Fatal(s)
		}
	}
}

type remuxFake struct{ calls [][]string }

func (f *remuxFake) Run(ctx context.Context, args []string, emit func(string)) error {
	f.calls = append(f.calls, args)
	if args[0] == "-J" {
		emit("{\"tracks\":[{\"id\":2,\"type\":\"video\",\"codec\":\"AVC\"},{\"id\":11,\"type\":\"audio\",\"codec\":\"DTS\",\"properties\":{\"language\":\"eng\"}}],\"chapters\":[{\"num_entries\":12}]}")
		return nil
	}
	return os.WriteFile(args[1], []byte("remuxed"), 0600)
}
func TestIdentifyAndRemuxPreservesOriginal(t *testing.T) {
	f := &remuxFake{}
	m := MKVToolNix{Runner: f}
	dir := t.TempDir()
	input := filepath.Join(dir, "original.mkv")
	os.WriteFile(input, []byte("original"), 0600)
	inv, e := m.Identify(context.Background(), input)
	if e != nil || inv.Tracks[1].ID != 11 || inv.Chapters[0].Count != 12 {
		t.Fatal(inv, e)
	}
	out, e := m.Remux(context.Background(), input, "selected.mkv", inv, map[int]bool{11: true}, func(Event) {})
	if e != nil {
		t.Fatal(e)
	}
	original, _ := os.ReadFile(input)
	if string(original) != "original" || filepath.Base(out) != "selected.mkv" {
		t.Fatal(out, string(original))
	}
	if _, e = m.Remux(context.Background(), input, "selected.mkv", inv, map[int]bool{}, func(Event) {}); e == nil {
		t.Fatal("existing remux overwritten")
	}
}
func TestMessageSeverity(t *testing.T) {
	if reportedError("MSG:5004,0,0,\"Partial\",\"Partial\"") == nil {
		t.Fatal("partial completion ignored")
	}
	if reportedError("MSG:1,516,0,\"Error\",\"Error\"") == nil {
		t.Fatal("error flags ignored")
	}
	if !strings.HasPrefix(convert("MSG:1,1028,0,\"Warning\",\"Warning\"", 0, 0, 1).Line, "WARNING") {
		t.Fatal("warning not labeled")
	}
}

type linesFake []string

func (f linesFake) Run(ctx context.Context, args []string, emit func(string)) error {
	for _, s := range f {
		emit(s)
	}
	return nil
}
func TestDriveDiscoveryDoesNotAssumeBooleanState(t *testing.T) {
	m := MakeMKV{Runner: linesFake{`DRV:0,2,999,1,"BD-ROM, USB","Movie","/dev/sr0"`, `DRV:7,256,999,0,"","",""`}}
	ds, e := m.Drives(context.Background(), func(Event) {})
	if e != nil || len(ds) != 1 || ds[0].Name != "BD-ROM, USB" || ds[0].State != "2" || ds[0].Device != "/dev/sr0" {
		t.Fatal(ds, e)
	}
}
