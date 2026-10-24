package tui

import (
	"github.com/madarlevi/makemkv-tui/internal/backend"
	"strings"
)

// Normal views show messages, not robot protocol dumps. Raw records (including
// unknown/malformed ones) remain in the optional diagnostic log and on disk.
func normalLog(line string) string {
	line = safe(backend.DisplayLog(line))
	if strings.HasPrefix(line, "ERROR:") {
		return paint(line, videoColor)
	}
	return line
}
func recentLogs(logs []string, n int) []string {
	var out []string
	for i := len(logs) - 1; i >= 0 && len(out) < n; i-- {
		if line := normalLog(logs[i]); line != "" {
			out = append(out, line)
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}
