package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/madarlevi/makemkv-tui/internal/media"
	"os"
	"strings"
)

var (
	videoColor   = lipgloss.CompleteColor{TrueColor: "#FF6B6B", ANSI256: "203", ANSI: "9"}
	audioColor   = lipgloss.CompleteColor{TrueColor: "#4ADE80", ANSI256: "83", ANSI: "10"}
	subColor     = lipgloss.CompleteColor{TrueColor: "#60A5FA", ANSI256: "75", ANSI: "12"}
	neutralColor = lipgloss.CompleteColor{TrueColor: "#F8FAFC", ANSI256: "255", ANSI: "15"}
	mutedColor   = lipgloss.CompleteColor{TrueColor: "#94A3B8", ANSI256: "246", ANSI: "8"}
)

func noColor() bool { return os.Getenv("NO_COLOR") != "" }
func outputKind(kind string) media.StreamKind {
	switch kind {
	case "video":
		return media.Video
	case "audio":
		return media.Audio
	case "subtitles":
		return media.Subtitle
	}
	return media.Other
}
func kindColor(kind media.StreamKind) lipgloss.TerminalColor {
	switch kind {
	case media.Video:
		return videoColor
	case media.Audio:
		return audioColor
	case media.Subtitle:
		return subColor
	}
	return neutralColor
}
func paint(text string, color lipgloss.TerminalColor) string {
	if noColor() {
		return text
	}
	return lipgloss.NewStyle().Foreground(color).Render(text)
}
func legend() string {
	return paint("VIDEO", videoColor) + " · " + paint("AUDIO", audioColor) + " · " + paint("SUB", subColor) + "   [x] selected  > focus"
}
func errorText(message string) string { return paint("ERROR: "+safe(message), videoColor) }

// Bound by terminal cells, not bytes/runes. Truncation preserves ANSI sequences
// and grapheme clusters; NO_COLOR strips even styles from third-party widgets.
func fitView(view string, width, height int) string {
	if noColor() {
		view = ansi.Strip(view)
	}
	width, height = max(1, width), max(1, height)
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "…")
	}
	return strings.Join(lines, "\n")
}
