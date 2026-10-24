package preferences

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "preferences.json")
	p := Defaults()
	p.AudioLanguages = "eng,jpn"
	if e := Save(path, p); e != nil {
		t.Fatal(e)
	}
	q, e := Load(path)
	if e != nil || q != p {
		t.Fatal(q, e)
	}
	p.SubtitleLanguages = "english"
	if Save(path, p) == nil {
		t.Fatal("invalid language accepted")
	}
}
func TestMalformedAndDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), "prefs")
	q, e := Load(p)
	if e != nil || q.Version != 1 {
		t.Fatal(q, e)
	}
	os.WriteFile(p, []byte("{"), 0600)
	if _, e = Load(p); e == nil {
		t.Fatal("bad JSON accepted")
	}
}
