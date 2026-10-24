package tui

import (
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/madarlevi/makemkv-tui/internal/batchstate"
	"testing"
)

func TestSavedPlanPicker(t *testing.T) {
	m := NewSavedPlans([]batchstate.Plan{{Path: "broken.json", Err: errors.New("corrupt")}, {Path: "legacy/source 季節/.batch-state.json", State: &batchstate.State{Status: batchstate.Paused}}})
	n, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || n.(SavedPlans).Selected != "" {
		t.Fatal("corrupt plan selected")
	}
	n, _ = n.Update(tea.KeyMsg{Type: tea.KeyDown})
	n, cmd = n.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || n.(SavedPlans).Selected != m.plans[1].Path {
		t.Fatal("plan not selected")
	}
	if m.View() == "" {
		t.Fatal("empty view")
	}
}
