package tui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/madarlevi/makemkv-tui/internal/batchstate"
	"strings"
)

// SavedPlans provides a keyboard-only entry point to the existing batch flow.
type SavedPlans struct {
	plans    []batchstate.Plan
	cursor   int
	Selected string
}

func NewSavedPlans(plans []batchstate.Plan) SavedPlans { return SavedPlans{plans: plans} }
func (m SavedPlans) Init() tea.Cmd                     { return nil }
func (m SavedPlans) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor+1 < len(m.plans) {
				m.cursor++
			}
		case "enter":
			if len(m.plans) > 0 && m.plans[m.cursor].Err == nil {
				m.Selected = m.plans[m.cursor].Path
				return m, tea.Quit
			}
		}
	}
	return m, nil
}
func (m SavedPlans) View() string {
	var b strings.Builder
	b.WriteString("makemkv-tui — Saved batch plans\n\n↑/↓ Select · Enter Resume · Esc Cancel\n\n")
	if len(m.plans) == 0 {
		b.WriteString("No saved plans found.\n")
	}
	start := m.cursor - 5
	if start < 0 {
		start = 0
	}
	end := start + 12
	if end > len(m.plans) {
		end = len(m.plans)
	}
	for i := start; i < end; i++ {
		p := m.plans[i]
		marker := "  "
		if i == m.cursor {
			marker = "> "
		}
		if p.Err != nil {
			fmt.Fprintf(&b, "%s%s\n    ERROR: %s\n", marker, p.Path, p.Err)
			continue
		}
		fmt.Fprintf(&b, "%s%s · %s · %d/%d titles\n    %s\n", marker, p.State.InputRoot, p.State.Status, p.State.Progress.Completed, p.State.Progress.Selected, p.Path)
	}
	return b.String()
}
