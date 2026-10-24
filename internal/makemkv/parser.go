package makemkv

import (
	"encoding/csv"
	"fmt"
	"github.com/madarlevi/makemkv-tui/internal/media"
	"io"
	"strconv"
	"strings"
)

type Record struct {
	Kind   string
	Fields []string
}

// Parse converts MakeMKV backslash escapes into RFC4180 quoting before CSV decoding.
func Parse(line string) (Record, error) {
	kind, body, ok := strings.Cut(strings.TrimSpace(line), ":")
	if !ok {
		return Record{}, fmt.Errorf("not a robot record: %q", line)
	}
	var b strings.Builder
	for i := 0; i < len(body); i++ {
		if body[i] == '\\' && i+1 < len(body) {
			i++
			switch body[i] {
			case '"':
				b.WriteString("\"\"")
			case '\\':
				b.WriteByte('\\')
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			default:
				b.WriteByte('\\')
				b.WriteByte(body[i])
			}
		} else {
			b.WriteByte(body[i])
		}
	}
	reader := csv.NewReader(strings.NewReader(b.String()))
	reader.FieldsPerRecord = -1
	fields, err := reader.Read()
	if err != nil {
		return Record{}, err
	}
	if _, err = reader.Read(); err != io.EOF {
		return Record{}, fmt.Errorf("multiple records")
	}
	r := Record{kind, fields}
	indexes := map[string][]int{"DRV": {0, 1, 2, 3}, "TINFO": {0, 1, 2}, "SINFO": {0, 1, 2, 3}, "CINFO": {0, 1}, "TCOUNT": {0}, "TCOUT": {0}, "MSG": {0, 1, 2}, "PRGC": {0, 1}, "PRGT": {0, 1}, "PRGV": {0, 1, 2}}
	lengths := map[string]int{"DRV": 6, "TINFO": 4, "SINFO": 5, "CINFO": 3, "TCOUNT": 1, "TCOUT": 1, "MSG": 5, "PRGC": 3, "PRGT": 3, "PRGV": 3}
	if n, known := lengths[kind]; known {
		if len(fields) < n {
			return Record{}, fmt.Errorf("%s: expected at least %d fields", kind, n)
		}
		for _, i := range indexes[kind] {
			v, e := strconv.Atoi(fields[i])
			if e != nil || v < 0 {
				return Record{}, fmt.Errorf("%s: invalid integer %q", kind, fields[i])
			}
		}
		if kind == "MSG" {
			n, _ := strconv.Atoi(fields[2])
			if len(fields) != 5+n {
				return Record{}, fmt.Errorf("MSG parameter count mismatch")
			}
		}
	}
	return r, nil
}

type Title = media.Title
type Disc struct{ *media.Disc }

func NewDisc() *Disc      { return &Disc{media.NewDisc()} }
func number(s string) int { n, _ := strconv.Atoi(s); return n }
func (d *Disc) Apply(r Record) {
	f := r.Fields
	switch r.Kind {
	case "TCOUNT", "TCOUT":
		d.Count = number(f[0])
	case "CINFO":
		d.Attributes[number(f[0])] = f[2]
	case "TINFO", "SINFO":
		id := number(f[0])
		t := d.Titles[id]
		if t == nil {
			t = &Title{ID: id, Attributes: map[int]string{}, Streams: map[int]map[int]string{}}
			d.Titles[id] = t
		}
		if r.Kind == "TINFO" {
			t.Attributes[number(f[1])] = f[3]
			if t.AttributeCodes == nil {
				t.AttributeCodes = map[int]int{}
			}
			t.AttributeCodes[number(f[1])] = number(f[2])
		} else {
			sid := number(f[1])
			if t.Streams[sid] == nil {
				t.Streams[sid] = map[int]string{}
			}
			t.Streams[sid][number(f[2])] = f[4]
			if t.StreamCodes == nil {
				t.StreamCodes = map[int]map[int]int{}
			}
			if t.StreamCodes[sid] == nil {
				t.StreamCodes[sid] = map[int]int{}
			}
			t.StreamCodes[sid][number(f[2])] = number(f[3])
		}
	}
}
