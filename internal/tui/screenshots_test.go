package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/lipgloss"
	"github.com/madarlevi/makemkv-tui/internal/batch"
	"github.com/madarlevi/makemkv-tui/internal/batchstate"
	"github.com/muesli/termenv"
)

// TestCaptureScreenshots captures the real View renderer, never an engine run.
// Opt-in: all paths and metadata below are synthetic, safe public examples.
func TestCaptureScreenshots(t *testing.T) {
	dir := os.Getenv("MAKEMKV_TUI_CAPTURE_DIR")
	if dir == "" {
		t.Skip("opt-in synthetic renderer captures")
	}
	if !filepath.IsAbs(dir) {
		t.Fatal("capture directory must be absolute")
	}
	if e := os.MkdirAll(dir, 0755); e != nil {
		t.Fatal(e)
	}
	old := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	t.Setenv("NO_COLOR", "")
	lipgloss.SetColorProfile(termenv.TrueColor)
	m := explorerFixture(t)
	m.source = "iso:/media/isos/series/disc01.iso"
	m.titles[0].Attributes[2] = "Example episode — Magyar / 日本語"
	m.selected[m.titles[0].ID] = true
	m.expanded = map[string]bool{}
	capture := func(name, view string) {
		t.Helper()
		assertFits(t, view, 110, 32)
		if e := os.WriteFile(filepath.Join(dir, name+".ansi"), []byte(view), 0644); e != nil {
			t.Fatal(e)
		}
	}
	capture("title-selection", m.View())
	m.openFirstTitle()
	m.cursor = 2
	capture("track-hierarchy", m.View())
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.Local)
	s := batchstate.State{Status: batchstate.Planning, InputRoot: "/media/isos", OutputRoot: "/media/output", UpdatedAt: now, CurrentISO: 0, CurrentTitle: m.titles[0].ID}
	for i, name := range []string{"series/disc01.iso", "series/disc02.iso", "feature.iso"} {
		j := batchstate.ISO{RelativePath: name, Reviewed: true, Status: batchstate.Pending}
		for _, id := range []int{6, 17} {
			j.Titles = append(j.Titles, batchstate.Title{ID: id, Selected: true, Status: batchstate.Pending, EstimatedBytes: 16600000000, Output: batchstate.OutputPath(name, id)})
		}
		if i == 0 {
			j.Titles[0].Metadata = m.titles[0]
		}
		s.ISOs = append(s.ISOs, j)
	}
	s.Progress.Selected = 6
	s.Progress.Pending = 6
	m.opts.Planning = true
	bm := BatchModel{phase: "batch-planning", picker: m, snapshot: batch.Snapshot{State: s}, width: 110, height: 32, bar: progress.New(progress.WithDefaultGradient())}
	capture("batch-planning", bm.View())
	bm.phase = "batch-summary"
	bm.snapshot.State.Status = batchstate.Ready
	capture("batch-summary", bm.View())
	bm.phase = "batch-run"
	bm.snapshot.State.Status = batchstate.Running
	bm.snapshot.State.CurrentTitle = 17
	bm.snapshot.State.Progress.Completed = 1
	bm.snapshot.State.Progress.Pending = 5
	bm.snapshot.State.Progress.CurrentFraction = .61
	bm.snapshot.State.Progress.Overall = 1.61 / 6
	bm.snapshot.State.ElapsedSeconds = 900
	bm.snapshot.State.Operation = "Extracting title (synthetic progress)"
	bm.snapshot.ProcessProgress = .61
	bm.snapshot.Logs = []string{"Synthetic engine event: copying title 17", fmt.Sprintf("Verified completions: %d", 1)}
	capture("batch-progress", bm.View())
}
