package tui

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/madarlevi/makemkv-tui/internal/backend"
	"github.com/madarlevi/makemkv-tui/internal/batch"
	"github.com/madarlevi/makemkv-tui/internal/batchstate"
	"github.com/madarlevi/makemkv-tui/internal/media"
	"github.com/madarlevi/makemkv-tui/internal/preferences"
	"os"
	"path/filepath"
	"testing"
)

type batchFake struct{ extracts int }

func (f *batchFake) Capabilities() backend.Capabilities       { return backend.Capabilities{} }
func (f *batchFake) NormalizeSource(s string) (string, error) { return s, nil }
func (f *batchFake) Drives(context.Context, func(backend.Event)) ([]backend.Drive, error) {
	return nil, nil
}
func (f *batchFake) Scan(context.Context, string, func(backend.Event)) (*media.Disc, error) {
	d := media.NewDisc()
	d.Titles[7] = &media.Title{ID: 7, Attributes: map[int]string{2: "Feature"}}
	return d, nil
}
func (f *batchFake) Extract(_ context.Context, r backend.Request, _ func(backend.Event)) ([]backend.Artifact, error) {
	f.extracts++
	p := filepath.Join(r.Output, r.Filenames[r.IDs[0]])
	if e := os.WriteFile(p, []byte{0x1a, 0x45, 0xdf, 0xa3, 0, 1}, 0600); e != nil {
		return nil, e
	}
	return []backend.Artifact{{TitleID: r.IDs[0], Path: p}}, nil
}
func batchModel(t *testing.T) (BatchModel, *batchFake, *batchstate.Store) {
	t.Helper()
	root := t.TempDir()
	in := filepath.Join(root, "in")
	os.MkdirAll(in, 0755)
	for _, name := range []string{"a.iso", "b.iso"} {
		os.WriteFile(filepath.Join(in, name), []byte("ISO"), 0600)
	}
	s, e := batch.New(in, filepath.Join(root, "out"), preferences.Defaults(), "")
	if e != nil {
		t.Fatal(e)
	}
	f := &batchFake{}
	for i := range s.ISOs {
		d, _ := f.Scan(context.Background(), "", func(backend.Event) {})
		if e = batch.SetMetadata(s, i, d); e != nil {
			t.Fatal(e)
		}
	}
	store, e := batchstate.Open(filepath.Join(root, "state.json"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { store.Close() })
	if e = store.Save(s); e != nil {
		t.Fatal(e)
	}
	c := batch.NewControl(context.Background(), s, store, f)
	t.Cleanup(c.Shutdown)
	m := NewBatch(c, f, Options{Preferences: s.Preferences, ConfigPath: filepath.Join(root, "prefs.json")}, false)
	n, _ := m.Update(openISO{0})
	return n.(BatchModel), f, store
}
func bkey(m BatchModel, k tea.KeyMsg) (BatchModel, tea.Cmd) {
	n, c := m.Update(k)
	return n.(BatchModel), c
}
func TestBatchUsesExistingPickerAndExplicitConfirmation(t *testing.T) {
	m, f, store := batchModel(t)
	if m.phase != "batch-planning" || m.picker.phase != "titles" {
		t.Fatal(m.phase)
	}
	m, _ = bkey(m, tea.KeyMsg{Type: tea.KeySpace})
	saved, e := store.Load()
	if e != nil || !saved.ISOs[0].Titles[0].Selected || saved.ISOs[0].Reviewed {
		t.Fatal("selection not saved immediately", e)
	}
	m, cmd := bkey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("next missing")
	}
	n, _ := m.Update(cmd())
	m = n.(BatchModel)
	if m.index != 1 || m.phase != "batch-planning" {
		t.Fatal(m.index, m.phase)
	}
	// Back/forward must preserve the original sparse ID selection.
	m, _ = bkey(m, tea.KeyMsg{Type: tea.KeyPgUp})
	if m.index != 0 || !m.picker.selected[7] {
		t.Fatal("selection lost")
	}
	m, cmd = bkey(m, tea.KeyMsg{Type: tea.KeyEnter})
	n, _ = m.Update(cmd())
	m = n.(BatchModel)
	m, _ = bkey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if m.phase != "batch-summary" || f.extracts != 0 {
		t.Fatal("planning extracted", m.phase, f.extracts)
	}
	m, _ = bkey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.phase != "batch-confirm" || f.extracts != 0 {
		t.Fatal("summary did not protect start")
	}
	m, cmd = bkey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if m.phase != "batch-run" || cmd == nil {
		t.Fatal(m.phase)
	}
	n, _ = m.Update(cmd())
	m = n.(BatchModel)
	if f.extracts != 1 || m.snapshot.State.Status != batchstate.Completed {
		t.Fatal(fmt.Sprint(m.snapshot.State.Status), m.err, f.extracts)
	}
}
func TestBatchPlanningInterruptionKeepsDecisions(t *testing.T) {
	m, _, store := batchModel(t)
	m, _ = bkey(m, tea.KeyMsg{Type: tea.KeySpace})
	_, cmd := bkey(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("idle planning must quit")
	}
	saved, e := store.Load()
	if e != nil || !saved.ISOs[0].Titles[0].Selected || saved.Status != batchstate.Planning {
		t.Fatal(saved, e)
	}
}
