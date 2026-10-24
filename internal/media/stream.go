package media

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// DisplaySize uses the engine's formatted size first, then its exact byte count.
func (t *Title) DisplaySize() string {
	if value := t.Attributes[10]; value != "" {
		return value
	}
	if n, e := strconv.ParseUint(t.Attributes[11], 10, 64); e == nil {
		if n < 1<<20 {
			return fmt.Sprintf("%d bytes", n)
		}
		if n < 1<<30 {
			return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
		}
		return fmt.Sprintf("%.1f GiB", float64(n)/(1<<30))
	}
	return "N/A"
}

type StreamKind string

const (
	Video    StreamKind = "VIDEO"
	Audio    StreamKind = "AUDIO"
	Subtitle StreamKind = "SUB"
	Other    StreamKind = "OTHER"
)

type Field struct{ Name, Value string }

// Stream is a read-only presentation of the source attributes, not a source
// selection checkbox or a guess based on codec, stream index or attribute ID.
type Stream struct {
	ID         int
	Kind       StreamKind
	Attributes map[int]string
}

func (t *Title) Stream(id int) Stream {
	a := t.Streams[id]
	kind := Other
	// APP_TTREE_* message codes on ap_iaType (attribute 1), verified against
	// MakeMKV's published apdefs.h. Codes also work with localized type text.
	switch t.StreamCodes[id][1] {
	case 6201:
		kind = Video
	case 6202:
		kind = Audio
	case 6203:
		kind = Subtitle
	default:
		switch strings.ToLower(strings.TrimSpace(a[1])) {
		case "video":
			kind = Video
		case "audio":
			kind = Audio
		case "subtitle", "subtitles", "subpicture":
			kind = Subtitle
		}
	}
	return Stream{ID: id, Kind: kind, Attributes: a}
}

func (t *Title) SortedStreams() []Stream {
	ids := make([]int, 0, len(t.Streams))
	for id := range t.Streams {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	streams := make([]Stream, 0, len(ids))
	for _, id := range ids {
		streams = append(streams, t.Stream(id))
	}
	return streams
}

func first(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
func (s Stream) Codec() string { return first(s.Attributes[7], s.Attributes[6], s.Attributes[5]) }
func (s Stream) Language() string {
	name, code := s.Attributes[4], s.Attributes[3]
	if name != "" && code != "" && name != code {
		return name + " (" + code + ")"
	}
	return first(name, code)
}
func (s Stream) Flags() string {
	// Prefer the engine's human-readable MKV flag description; do not interpret
	// opaque flag strings as booleans or invent default/forced values.
	flags := s.Attributes[39]
	if n, e := strconv.ParseUint(s.Attributes[22], 10, 64); e == nil && n&4096 != 0 && !strings.Contains(strings.ToLower(flags), "forced") {
		if flags != "" {
			flags += ", "
		}
		flags += "Forced subtitles"
	}
	return flags
}
func (s Stream) Fields() []Field {
	fields := []Field{}
	add := func(name, value string) {
		if strings.TrimSpace(value) != "" {
			fields = append(fields, Field{name, value})
		}
	}
	a := s.Attributes
	add("Type", a[1])
	add("Name", a[2])
	add("Language", s.Language())
	add("Codec", s.Codec())
	if a[5] != s.Codec() {
		add("Codec ID", a[5])
	}
	switch s.Kind {
	case Video:
		add("Resolution", a[19])
		add("Frame rate", a[21])
		add("Aspect ratio", a[20])
		add("Bitrate", a[13])
	case Audio:
		add("Channel layout", a[40])
		add("Channels", a[14])
		add("Bitrate", a[13])
		add("Sample rate", a[17])
		add("Sample size", a[18])
	}
	add("Flags", s.Flags())
	add("Engine summary", a[30])
	add("Engine details", a[37])
	add("Output codec", a[41])
	add("Conversion", a[42])
	add("Output sample rate", a[43])
	add("Output sample size", a[44])
	add("Output channels", a[45])
	add("Output layout", a[46])
	add("Comment", a[49])
	return fields
}
func (s Stream) Summary() string {
	parts := []string{}
	add := func(value string) {
		if value != "" {
			parts = append(parts, value)
		}
	}
	if s.Kind != Video {
		add(s.Language())
	}
	add(s.Codec())
	switch s.Kind {
	case Video:
		add(s.Attributes[19])
		add(s.Attributes[21])
		add(s.Attributes[20])
	case Audio:
		add(first(s.Attributes[40], s.Attributes[14]))
		add(s.Attributes[13])
		add(s.Attributes[17])
	}
	add(s.Flags())
	add(s.Attributes[2])
	if len(parts) == 0 {
		return "N/A"
	}
	return strings.Join(parts, " · ")
}
