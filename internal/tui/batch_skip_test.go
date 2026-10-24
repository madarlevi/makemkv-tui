package tui

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/madarlevi/makemkv-tui/internal/backend"
	"github.com/madarlevi/makemkv-tui/internal/batch"
	"github.com/madarlevi/makemkv-tui/internal/batchstate"
	"github.com/madarlevi/makemkv-tui/internal/media"
	"testing"
)

type slowScan struct{ *batchFake }

func (s slowScan) Scan(ctx context.Context, _ string, _ func(backend.Event)) (*media.Disc, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func TestSkipCancelsOnlyCurrentPlanningScan(t *testing.T) {
	old, engine, store := batchModel(t)
	state, e := store.Load()
	if e != nil {
		t.Fatal(e)
	}
	state.ISOs[0].Metadata = nil
	state.ISOs[0].Titles = nil
	state.ISOs[0].Reviewed = false
	state.Status = batchstate.Planning
	if e = store.Save(state); e != nil {
		t.Fatal(e)
	}
	c := batch.NewControl(context.Background(), state, store, slowScan{engine})
	defer c.Shutdown()
	m := NewBatch(c, slowScan{engine}, old.options, false)
	n, cmd := m.Update(openISO{0})
	m = n.(BatchModel)
	if m.phase != "batch-scan" || cmd == nil {
		t.Fatal(m.phase)
	}
	m, _ = bkey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	n, _ = m.Update(cmd())
	m = n.(BatchModel)
	if m.index != 1 || m.phase != "batch-planning" || c.Context().Err() != nil || engine.extracts != 0 {
		t.Fatal(m.phase, m.index, c.Context().Err())
	}
	q, e := store.Load()
	if e != nil || q.ISOs[0].Status != batchstate.Skipped || !q.ISOs[0].Reviewed {
		t.Fatal(q, e)
	}
	m, _ = bkey(m, tea.KeyMsg{Type: tea.KeyCtrlL})
	m, _ = bkey(m, tea.KeyMsg{Type: tea.KeyPgUp})
	_ = m.View()
}
