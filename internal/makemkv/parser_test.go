package makemkv

import (
	"bufio"
	"encoding/json"
	"github.com/madarlevi/makemkv-tui/internal/media"
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) []Record {
	t.Helper()
	f, e := os.Open("testdata/" + name)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	var out []Record
	for s.Scan() {
		r, e := Parse(s.Text())
		if e != nil {
			t.Fatalf("%s: %v", s.Text(), e)
		}
		out = append(out, r)
	}
	if e = s.Err(); e != nil {
		t.Fatal(e)
	}
	return out
}

func TestExplorerFixtureGroupingAndTypes(t *testing.T) {
	d := NewDisc()
	for _, r := range fixture(t, "explorer.txt") {
		d.Apply(r)
	}
	if d.Count != 3 || len(d.Titles) != 3 || len(d.Titles[6].Streams) != 7 || len(d.Titles[42].Streams) != 1 {
		t.Fatal(d.Disc)
	}
	title := d.Titles[6]
	for id, want := range map[int]media.StreamKind{0: media.Video, 1: media.Audio, 2: media.Audio, 3: media.Subtitle, 4: media.Subtitle, 12: media.Video, 90: media.Other} {
		if got := title.Stream(id).Kind; got != want {
			t.Errorf("stream %d: %s want %s", id, got, want)
		}
	}
	if title.Streams[1][38] != "d" || title.Streams[1][39] != "Default" || title.Streams[1][40] != "stereo" || title.Streams[1][42] != "( Lossless conversion )" {
		t.Fatal("SINFO fields shifted", title.Streams[1])
	}
	if title.StreamCodes[1][42] != 5088 || title.AttributeCodes[9] != 0 {
		t.Fatal("code/value lost")
	}
	if d.Titles[99].Stream(0).Kind != media.Other {
		t.Fatal("attribute/code mistaken for type")
	}
	if title.Streams[90][99] != "Unknown, future metadata" {
		t.Fatal("unknown metadata lost")
	}
	if title.Streams[3][2] != `Forced, "signs" and songs` || d.Titles[42].Stream(1).Language() != "日本語 (jpn)" {
		t.Fatal("CSV/Unicode lost")
	}
	if !strings.Contains(title.Attributes[2], `"quoted"`) || title.Attributes[9] != "1:37:06" {
		t.Fatal("TINFO fields shifted")
	}
	// Optional type codes survive JSON plans; old plans still use type text.
	b, e := json.Marshal(title)
	if e != nil {
		t.Fatal(e)
	}
	var loaded media.Title
	if e = json.Unmarshal(b, &loaded); e != nil || loaded.Stream(12).Kind != media.Video {
		t.Fatal(loaded, e)
	}
	delete(loaded.StreamCodes, 1)
	if loaded.Stream(1).Kind != media.Audio {
		t.Fatal("legacy type text no longer works")
	}
}
func TestInfoFixture(t *testing.T) {
	d := NewDisc()
	for _, r := range fixture(t, "info.txt") {
		d.Apply(r)
	}
	if d.Count != 2 || len(d.Titles) != 2 || d.Titles[0].Attributes[2] != "A \"quoted\" title" || d.Titles[4].Attributes[2] != "Bonus, feature" || len(d.Titles[0].Streams) != 3 {
		t.Fatalf("unexpected disc: %+v / %+v", d, d.Titles[0])
	}
	if d.Sorted()[1].ID != 4 {
		t.Fatal("IDs must not be renumbered")
	}
}
func TestProgressFixture(t *testing.T) {
	rs := fixture(t, "progress.txt")
	if rs[0].Fields[2] != "Saving, title" || rs[2].Fields[1] != "75" {
		t.Fatal(rs)
	}
}
func TestMalformed(t *testing.T) {
	for _, s := range []string{"TINFO:0,2", "SINFO:x,0,1,0,\"Video\"", "TCOUNT:-1", "PRGV:1,x,100", "MSG:1,0,2,\"a\",\"b\"", "CINFO:2,0,\"unclosed"} {
		if _, e := Parse(s); e == nil {
			t.Errorf("accepted %s", s)
		}
	}
}
func TestCSVAndEscapes(t *testing.T) {
	for _, s := range []string{`CINFO:2,0,"a, b \"quote\""`, `CINFO:2,0,"a, b ""quote"""`} {
		r, e := Parse(s)
		if e != nil || r.Fields[2] != `a, b "quote"` {
			t.Fatalf("%s => %+v %v", s, r, e)
		}
	}
	r, e := Parse(`CINFO:2,0,"a\nb\\c"`)
	if e != nil || !strings.Contains(r.Fields[2], "\n") {
		t.Fatal(r, e)
	}
}
