package backend

import (
	"fmt"
	"github.com/madarlevi/makemkv-tui/internal/makemkv"
	"strings"
)

// DisplayLog translates robot diagnostics to human messages without changing
// the raw Event.Line stored in session logs. The TUI need not parse the engine
// protocol. Plain messages from other engines pass through unchanged.
func DisplayLog(line string) string {
	if strings.HasPrefix(line, "ERROR:") || strings.HasPrefix(line, "WARNING:") || strings.HasPrefix(line, "INFO:") {
		return line
	}
	prefix := ""
	for _, tag := range []string{"INFO", "WARNING", "ERROR"} {
		if strings.HasPrefix(line, tag+" · ") {
			prefix = tag + ": "
			line = strings.TrimPrefix(line, tag+" · ")
			break
		}
	}
	if r, e := makemkv.Parse(line); e == nil && r.Kind == "MSG" {
		message := r.Fields[3]
		// Field 3 normally contains an already formatted message. Substitute
		// only when the engine supplies the format itself in that field too;
		// percent sequences in a formatted filename must remain literal.
		if message == r.Fields[4] {
			for i := len(r.Fields) - 1; i >= 5; i-- {
				message = strings.ReplaceAll(message, fmt.Sprintf("%%%d", i-4), r.Fields[i])
			}
		}
		return prefix + message
	}
	if kind, _, ok := strings.Cut(line, ":"); ok && len(kind) > 0 {
		robot := true
		for _, c := range kind {
			if !(c >= 'A' && c <= 'Z') && !(c >= '0' && c <= '9') {
				robot = false
				break
			}
		}
		if robot {
			if prefix == "ERROR: " {
				return "ERROR: Engine diagnostic; Ctrl+L for details"
			}
			return ""
		}
	}
	return prefix + line
}
