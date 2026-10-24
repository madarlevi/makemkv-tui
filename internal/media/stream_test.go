package media

import (
	"strings"
	"testing"
)

func TestTitleSizeFallback(t *testing.T) {
	title := &Title{Attributes: map[int]string{11: "1073741824"}}
	if title.DisplaySize() != "1.0 GiB" {
		t.Fatal(title.DisplaySize())
	}
	title.Attributes[10] = "1.1 GB"
	if title.DisplaySize() != "1.1 GB" {
		t.Fatal("engine formatting replaced")
	}
	delete(title.Attributes, 10)
	title.Attributes[11] = "unknown"
	if title.DisplaySize() != "N/A" {
		t.Fatal("size invented")
	}
}

func TestStreamFieldsDoNotInventMetadata(t *testing.T) {
	title := &Title{Streams: map[int]map[int]string{7: {}, 9: {1: "Video", 6: "HEVC Main 10", 18: "24"}, 11: {1: "Audio", 3: "hun", 4: "Magyar", 6: "AC3", 14: "6", 40: "5.1", 17: "48000", 13: "640 Kb/s", 18: "24", 39: "Default"}, 15: {1: "Subtitles", 3: "eng", 6: "PGS", 22: "4096", 2: "Signs"}}}
	if s := title.Stream(7); s.Kind != Other || len(s.Fields()) != 0 || s.Summary() != "N/A" {
		t.Fatal(s)
	}
	for _, field := range title.Stream(9).Fields() {
		if field.Name == "Bit depth" || field.Name == "HDR" || field.Name == "Sample size" {
			t.Fatal("inferred video metadata", field)
		}
	}
	audio := title.Stream(11)
	if audio.Language() != "Magyar (hun)" || audio.Codec() != "AC3" || audio.Flags() != "Default" || !strings.Contains(audio.Summary(), "5.1") {
		t.Fatal(audio.Summary())
	}
	fields := map[string]string{}
	for _, f := range audio.Fields() {
		fields[f.Name] = f.Value
	}
	for name, value := range map[string]string{"Sample rate": "48000", "Bitrate": "640 Kb/s", "Channels": "6", "Sample size": "24"} {
		if fields[name] != value {
			t.Fatal(fields)
		}
	}
	if title.Stream(15).Flags() != "Forced subtitles" {
		t.Fatal("reported forced flag missing")
	}
	if ids := title.SortedStreams(); len(ids) != 4 || ids[0].ID != 7 || ids[3].ID != 15 {
		t.Fatal(ids)
	}
}
