package tui

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"time"
)

// Expanded errors wrap rather than truncate, including long syscall paths and
// chained causes. Scrolling exposes the entire retained error message.
func wrappedErrors(messages []string, width int) []string {
	lines := []string{}
	for i, message := range messages {
		text := fmt.Sprintf("ERROR %d: %s", i+1, safe(message))
		lines = append(lines, strings.Split(ansi.Hardwrap(text, max(1, width), true), "\n")...)
		lines = append(lines, "")
	}
	if len(lines) == 0 {
		lines = []string{"No recorded errors."}
	}
	return lines
}
func errorLimit(messages []string, width, height int) int {
	return max(0, len(wrappedErrors(messages, width))-max(1, height-2))
}
func errorView(messages []string, offset, width, height int) string {
	w, h := max(1, width), max(1, height)
	lines := wrappedErrors(messages, w)
	page := max(1, h-2)
	offset = max(0, min(offset, max(0, len(lines)-page)))
	header := fmt.Sprintf("Full errors · line %d/%d", offset+1, len(lines))
	body := strings.Join(lines[offset:min(len(lines), offset+page)], "\n")
	return fitView(errorText(header)+"\n"+body+"\n↑/↓ PgUp/PgDn scroll · Ctrl+E/Esc close", w, h)
}

func (m Model) errorMessages() []string {
	messages := []string{}
	if m.err != "" {
		messages = append(messages, m.err)
	}
	for _, line := range m.logs {
		if text := ansi.Strip(normalLog(line)); strings.HasPrefix(text, "ERROR:") {
			messages = append(messages, text)
		}
	}
	return messages
}
func (m BatchModel) errorMessages() []string {
	messages := []string{}
	for _, f := range m.snapshot.State.Errors {
		messages = append(messages, f.At.Format(time.RFC3339)+" · "+f.Message)
	}
	if m.err != "" {
		messages = append(messages, m.err)
	}
	return messages
}

func scrollErrors(offset int, key string, height int) int {
	switch key {
	case "up":
		return max(0, offset-1)
	case "down":
		return offset + 1
	case "pgup":
		return max(0, offset-max(1, height-3))
	case "pgdown":
		return offset + max(1, height-3)
	case "home":
		return 0
	}
	return offset
}
