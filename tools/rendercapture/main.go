// Command rendercapture renders captured ANSI View output as lightweight SVG.
package main

import (
	"flag"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var sgr = regexp.MustCompile("\x1b\\[([0-9;]*)m")
var palette = []string{"#111827", "#ef4444", "#22c55e", "#eab308", "#3b82f6", "#a855f7", "#06b6d4", "#d1d5db", "#6b7280", "#f87171", "#4ade80", "#facc15", "#60a5fa", "#c084fc", "#22d3ee", "#ffffff"}

type style struct {
	fg, bg string
	bold   bool
}

func (s *style) apply(raw string) {
	if raw == "" {
		raw = "0"
	}
	parts := strings.Split(raw, ";")
	for i := 0; i < len(parts); i++ {
		n, _ := strconv.Atoi(parts[i])
		switch {
		case n == 0:
			*s = style{fg: "#e5e7eb"}
		case n == 1:
			s.bold = true
		case n == 22:
			s.bold = false
		case n == 39:
			s.fg = "#e5e7eb"
		case n == 49:
			s.bg = ""
		case (n == 38 || n == 48) && i+4 < len(parts) && parts[i+1] == "2":
			c := []int{}
			for _, v := range parts[i+2 : i+5] {
				x, _ := strconv.Atoi(v)
				c = append(c, x)
			}
			color := fmt.Sprintf("#%02x%02x%02x", c[0], c[1], c[2])
			if n == 38 {
				s.fg = color
			} else {
				s.bg = color
			}
			i += 4
		case (n == 38 || n == 48) && i+2 < len(parts) && parts[i+1] == "5":
			v, _ := strconv.Atoi(parts[i+2])
			color := indexed(v)
			if n == 38 {
				s.fg = color
			} else {
				s.bg = color
			}
			i += 2
		case n >= 30 && n <= 37:
			s.fg = palette[n-30]
		case n >= 90 && n <= 97:
			s.fg = palette[n-90+8]
		case n >= 40 && n <= 47:
			s.bg = palette[n-40]
		case n >= 100 && n <= 107:
			s.bg = palette[n-100+8]
		}
	}
}
func indexed(n int) string {
	if n < 16 && n >= 0 {
		return palette[n]
	}
	if n >= 232 {
		v := 8 + (n-232)*10
		return fmt.Sprintf("#%02x%02x%02x", v, v, v)
	}
	n -= 16
	level := []int{0, 95, 135, 175, 215, 255}
	return fmt.Sprintf("#%02x%02x%02x", level[n/36%6], level[n/6%6], level[n%6])
}
func render(name, content string) string {
	lines := strings.Split(content, "\n")
	width := 110
	for _, line := range lines {
		if w := ansi.StringWidth(line); w > width {
			width = w
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"%d\" height=\"%d\" viewBox=\"0 0 %d %d\"><title>%s — actual renderer, synthetic data</title><rect width=\"100%%\" height=\"100%%\" rx=\"10\" fill=\"#0b1020\"/><g font-family=\"Consolas,DejaVu Sans Mono,monospace\" font-size=\"13\" xml:space=\"preserve\"><text x=\"16\" y=\"24\" fill=\"#94a3b8\">makemkv-tui · ACTUAL VIEW RENDER · SYNTHETIC DATA</text>\n", width*8+32, len(lines)*19+56, width*8+32, len(lines)*19+56, html.EscapeString(name))
	st := style{fg: "#e5e7eb"}
	for y, line := range lines {
		x := 0
		pos := 0
		emit := func(text string) {
			if text == "" {
				return
			}
			w := ansi.StringWidth(text) * 8
			if st.bg != "" {
				fmt.Fprintf(&b, "<rect x=\"%d\" y=\"%d\" width=\"%d\" height=\"19\" fill=\"%s\"/>", 16+x, 36+y*19, w, st.bg)
			}
			weight := "normal"
			if st.bold {
				weight = "bold"
			}
			fmt.Fprintf(&b, "<text x=\"%d\" y=\"%d\" fill=\"%s\" font-weight=\"%s\">%s</text>\n", 16+x, 51+y*19, st.fg, weight, html.EscapeString(text))
			x += w
		}
		for _, m := range sgr.FindAllStringSubmatchIndex(line, -1) {
			emit(line[pos:m[0]])
			st.apply(line[m[2]:m[3]])
			pos = m[1]
		}
		emit(line[pos:])
	}
	b.WriteString("</g></svg>\n")
	return b.String()
}
func main() {
	dir := flag.String("dir", "docs/screenshots", "ANSI capture directory")
	flag.Parse()
	paths, e := filepath.Glob(filepath.Join(*dir, "*.ansi"))
	if e != nil {
		panic(e)
	}
	if len(paths) == 0 {
		panic("no .ansi captures")
	}
	for _, p := range paths {
		b, e := os.ReadFile(p)
		if e != nil {
			panic(e)
		}
		dest := strings.TrimSuffix(p, ".ansi") + ".svg"
		if e = os.WriteFile(dest, []byte(render(filepath.Base(p), string(b))), 0644); e != nil {
			panic(e)
		}
		fmt.Println(dest)
	}
}
