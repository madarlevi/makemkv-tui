package tui

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/madarlevi/makemkv-tui/internal/media"
	"path/filepath"
	"strings"
)

func (m *Model) openFirstTitle() {
	if len(m.titles) > 0 && len(m.titles[0].Streams) > 0 {
		m.expanded[fmt.Sprintf("t%d", m.titles[0].ID)] = true
	}
}

func (m Model) rows() []row {
	var out []row
	for i, t := range m.titles {
		out = append(out, row{title: i, stream: -1, kind: "title", label: fmt.Sprintf("Title %d · %s · %s · %s", t.ID, value(t.Attributes[9], "N/A"), safe(t.DisplaySize()), safe(t.Attributes[2]))})
		if !m.expanded[fmt.Sprintf("t%d", t.ID)] {
			continue
		}
		for _, stream := range t.SortedStreams() {
			out = append(out, row{title: i, stream: stream.ID, kind: "stream", label: stream.Summary()})
			if m.expanded[fmt.Sprintf("s%d/%d", t.ID, stream.ID)] {
				for _, field := range stream.Fields() {
					out = append(out, row{title: i, stream: stream.ID, kind: "metadata", label: field.Name + ": " + field.Value})
				}
			}
		}
		out = append(out, row{title: i, stream: -1, kind: "chapters", label: "Chapters: " + value(t.Attributes[8], "N/A")})
	}
	return out
}

// Folding preserves row identity, so collapsing a child never jumps focus to
// an unrelated title whose row happens to occupy the previous cursor index.
func (m *Model) fold(expand, toggle bool) {
	rows := m.rows()
	if len(rows) == 0 {
		return
	}
	r := rows[min(m.cursor, len(rows)-1)]
	t := m.titles[r.title]
	key := fmt.Sprintf("t%d", t.ID)
	if r.stream >= 0 {
		key = fmt.Sprintf("s%d/%d", t.ID, r.stream)
	}
	if !expand && !toggle && (r.kind == "chapters" || (r.kind == "stream" && !m.expanded[key])) {
		key = fmt.Sprintf("t%d", t.ID)
		r.stream = -1
		r.kind = "title"
	}
	if toggle {
		expand = !m.expanded[key]
	}
	m.expanded[key] = expand
	if r.kind == "metadata" {
		r.kind = "stream"
	}
	for i, candidate := range m.rows() {
		if candidate.title == r.title && candidate.stream == r.stream && candidate.kind == r.kind {
			m.cursor = i
			break
		}
	}
	m.detailOffset = 0
}

func (m Model) explorerDetails(r row) []string {
	t := m.titles[r.title]
	lines := []string{}
	if r.stream >= 0 {
		s := t.Stream(r.stream)
		name := s.Codec()
		if s.Kind != media.Video && s.Language() != "" {
			name = s.Language()
		}
		if name == "" {
			name = "N/A"
		}
		lines = append(lines, paint(fmt.Sprintf("%s — %s · Stream %d", s.Kind, safe(name), s.ID), kindColor(s.Kind)))
		for _, f := range s.Fields() {
			lines = append(lines, paint(f.Name+": ", mutedColor)+safe(f.Value))
		}
		if len(lines) == 1 {
			lines = append(lines, "Metadata: N/A")
		}
	} else if r.kind == "chapters" {
		lines = append(lines, "CHAPTERS", "Count: "+value(t.Attributes[8], "N/A"), "Chapter timestamps are not reported by this scan.")
	} else {
		lines = append(lines, fmt.Sprintf("TITLE %d — %s", t.ID, value(t.Attributes[2], "N/A")))
		for _, id := range []int{9, 10, 11, 8, 15, 16, 24, 25, 26, 27, 49} {
			if t.Attributes[id] != "" {
				lines = append(lines, paint(media.AttributeName(id)+": ", mutedColor)+safe(t.Attributes[id]))
			}
		}
		counts := map[media.StreamKind]int{}
		for _, s := range t.SortedStreams() {
			counts[s.Kind]++
		}
		lines = append(lines, fmt.Sprintf("Streams: %d VIDEO · %d AUDIO · %d SUB · %d OTHER", counts[media.Video], counts[media.Audio], counts[media.Subtitle], counts[media.Other]))
		lines = append(lines, "Filename: "+value(m.names[t.ID], "engine default"))
	}
	return lines
}

func (m Model) explorerRow(r row, focused bool, width int) string {
	t := m.titles[r.title]
	prefix := "  "
	if focused {
		prefix = "> "
	}
	text := ""
	color := lipgloss.TerminalColor(mutedColor)
	switch r.kind {
	case "title":
		fold, check := "▶", "[ ]"
		if m.expanded[fmt.Sprintf("t%d", t.ID)] {
			fold = "▼"
		}
		if m.selected[t.ID] {
			check = "[x]"
		}
		text = prefix + fold + " " + check + " " + r.label
		color = neutralColor
	case "stream":
		s := t.Stream(r.stream)
		fold := "+"
		if m.expanded[fmt.Sprintf("s%d/%d", t.ID, r.stream)] {
			fold = "-"
		}
		text = prefix + "  ├─ " + fold + " " + string(s.Kind) + "  " + safe(r.label)
		color = kindColor(s.Kind)
	case "metadata":
		text = prefix + "  │    " + safe(r.label)
	case "chapters":
		text = prefix + "  └─ " + safe(r.label)
	}
	text = fitView(text, width, 1)
	if noColor() {
		return text
	}
	style := lipgloss.NewStyle().Foreground(color)
	if focused {
		style = style.Background(lipgloss.Color("#1E293B")).Bold(true).Width(width)
	}
	return style.Render(text)
}

func (m Model) explorerView() string {
	w, h := max(1, m.width), max(1, m.height)
	selected := 0
	for _, t := range m.titles {
		if m.selected[t.ID] {
			selected++
		}
	}
	name := filepath.Base(strings.TrimPrefix(m.source, "iso:"))
	if name == "." || name == "" {
		name = "Source"
	}
	header := []string{paint("makemkv-tui", neutralColor) + fmt.Sprintf(" · %d/%d selected · ", selected, len(m.titles)) + paint(safe(name), neutralColor), legend()}
	rows := m.rows()
	cursor := max(0, min(m.cursor, len(rows)-1))
	// On tiny terminals preserve the legend, focused row and a compact hint.
	if h < 12 {
		lines := []string{header[0]}
		if h >= 4 {
			lines = append(lines, header[1])
		}
		if len(rows) > 0 && h >= 2 {
			lines = append(lines, m.explorerRow(rows[cursor], true, w))
		}
		if h >= 5 {
			lines = append(lines, paint("Tracks read-only · profiles/post-process", mutedColor))
		}
		if m.err != "" && h >= 4 {
			lines = append(lines, errorText(m.err))
		}
		lines = append(lines, "←/→ fold · Space select · Enter next")
		return fitView(strings.Join(lines, "\n"), w, h)
	}
	// Footer stays visible; metadata and explorer share the remaining height.
	footer := []string{"↑/↓ move · ←/→/Tab fold · Space select · A/D all/none", "Enter: output · n: filename · Ctrl+P preferences · Ctrl+L logs"}
	if m.opts.Planning {
		footer[1] = "Enter/PgDn: next ISO · PgUp: back · X skip · F summary · Ctrl+L logs"
	}
	footer = append(footer, paint("Tracks read-only · supported profiles or post-extraction MKVToolNix", mutedColor))
	if m.err != "" {
		footer[0] = "Ctrl+E: expand full error · ←/→ fold · Space select"
		footer = append(footer, errorText(m.err))
	}
	bodyHeight := max(3, h-len(header)-len(footer))
	detailHeight := min(9, max(3, bodyHeight/3))
	treeHeight := max(1, bodyHeight-detailHeight-2)
	start := max(0, cursor-treeHeight+1)
	end := min(len(rows), start+treeHeight)
	header = append(header, paint(fmt.Sprintf("── Title explorer · rows %d–%d/%d ──", min(start+1, len(rows)), end, len(rows)), mutedColor))
	for i := start; i < end; i++ {
		header = append(header, m.explorerRow(rows[i], i == cursor, w))
	}
	for i := end - start; i < treeHeight; i++ {
		header = append(header, "")
	}
	header = append(header, paint("── Metadata · [ / ] scroll ──", mutedColor))
	details := []string{"No titles reported"}
	if len(rows) > 0 {
		details = m.explorerDetails(rows[cursor])
	}
	offset := min(m.detailOffset, max(0, len(details)-1))
	details = details[offset:min(len(details), offset+detailHeight)]
	header = append(header, details...)
	for i := len(details); i < detailHeight; i++ {
		header = append(header, "")
	}
	header = append(header, footer...)
	return fitView(strings.Join(header, "\n"), w, h)
}
