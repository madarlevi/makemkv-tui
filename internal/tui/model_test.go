package tui

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/madarlevi/makemkv-tui/internal/backend"
	"github.com/madarlevi/makemkv-tui/internal/makemkv"
	"testing"
)

type unusedRunner struct{}

func (unusedRunner) Run(context.Context, []string, func(string)) error { return nil }
func key(m Model, k tea.KeyMsg) Model                                  { n, _ := m.Update(k); return n.(Model) }
func TestSelectionAndConfirmation(t *testing.T) {
	m := New(context.Background(), backend.MakeMKV{Runner: unusedRunner{}}, "")
	defer m.cancel()
	m.phase = "titles"
	m.titles = []*makemkv.Title{{ID: 2}, {ID: 9}}
	m = key(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.phase != "titles" || m.err == "" {
		t.Fatal("empty selection accepted")
	}
	m = key(m, tea.KeyMsg{Type: tea.KeySpace})
	m = key(m, tea.KeyMsg{Type: tea.KeyDown})
	m = key(m, tea.KeyMsg{Type: tea.KeySpace})
	m = key(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.phase != "output" || len(m.ids) != 2 || m.ids[0] != 2 || m.ids[1] != 9 {
		t.Fatal(m.ids, m.phase)
	}
	m.input.SetValue(t.TempDir())
	m = key(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.phase != "confirm" {
		t.Fatal(m.phase, m.err)
	}
}
func TestProgressAndCancellationWait(t *testing.T) {
	m := New(context.Background(), backend.MakeMKV{Runner: unusedRunner{}}, "")
	defer m.cancel()
	m.phase = "run"
	m.ids = []int{2, 9}
	m.events = make(chan event, 1)
	n, _ := m.Update(event{Event: backend.Event{Index: 1, TitleID: 9, Progress: true, Current: 0.25, Total: 0.75}})
	m = n.(Model)
	if m.current != 0.25 || m.total != 0.75 {
		t.Fatal(m.current, m.total)
	}
	m = key(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if !m.quitting || m.ctx.Err() == nil {
		t.Fatal("cancellation not requested")
	}
	_, cmd := m.Update(event{done: true, err: context.Canceled})
	if cmd == nil {
		t.Fatal("must quit after process has stopped")
	}
}

func TestHierarchyAndMetadata(t *testing.T) {
	m := New(context.Background(), backend.MakeMKV{Runner: unusedRunner{}}, "")
	defer m.cancel()
	m.phase = "titles"
	m.titles = []*makemkv.Title{{ID: 3, Attributes: map[int]string{2: "Feature", 8: "12"}, Streams: map[int]map[int]string{4: {1: "Audio", 3: "eng", 6: "DTS"}}}}
	if len(m.rows()) != 1 {
		t.Fatal("initial tree must be collapsed")
	}
	m = key(m, tea.KeyMsg{Type: tea.KeyRight})
	if len(m.rows()) != 3 {
		t.Fatal(m.rows())
	}
	m = key(m, tea.KeyMsg{Type: tea.KeyDown})
	m = key(m, tea.KeyMsg{Type: tea.KeyRight})
	if len(m.rows()) <= 3 {
		t.Fatal("stream metadata did not expand")
	}
	m = key(m, tea.KeyMsg{Type: tea.KeySpace})
	if m.err == "" || m.selected[3] {
		t.Fatal("unsupported source track selection must not appear applied")
	}
}
func TestPreferenceCancelRestoresInputAndValues(t *testing.T) {
	m := New(context.Background(), backend.MakeMKV{Runner: unusedRunner{}}, "movie.iso")
	defer m.cancel()
	m = key(m, tea.KeyMsg{Type: tea.KeyCtrlP})
	m.input.SetValue("/changed")
	m = key(m, tea.KeyMsg{Type: tea.KeyTab})
	m = key(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.phase != "source" || m.input.Value() != "movie.iso" || m.opts.Preferences.OutputDir != "." {
		t.Fatal(m.phase, m.input.Value(), m.opts.Preferences)
	}
}
func TestEmptyLogScrollDoesNotPanic(t *testing.T) {
	m := New(context.Background(), backend.MakeMKV{Runner: unusedRunner{}}, "")
	defer m.cancel()
	m = key(m, tea.KeyMsg{Type: tea.KeyCtrlL})
	m = key(m, tea.KeyMsg{Type: tea.KeyPgUp})
	_ = m.View()
}
