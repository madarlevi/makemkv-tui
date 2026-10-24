package tui

import (
	"context"
	"errors"
	"fmt"
	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/madarlevi/makemkv-tui/internal/backend"
	"github.com/madarlevi/makemkv-tui/internal/batch"
	"github.com/madarlevi/makemkv-tui/internal/batchstate"
	"github.com/madarlevi/makemkv-tui/internal/media"
	"path/filepath"
	"strings"
	"time"
)

type pickerNext struct{}
type batchTick struct{}
type batchScan struct {
	disc *media.Disc
	err  error
}
type batchFinished struct{ err error }

// BatchStopMsg also lets the CLI signal handler request a second-stop force while
// the UI is busy saving state. AlreadyRequested avoids counting that stop twice.
type BatchStopMsg struct {
	AlreadyRequested bool
	Err              error
}
type BatchModel struct {
	logView                     bool
	logOffset                   int
	showErrors                  bool
	errorOffset                 int
	scanCancel                  context.CancelFunc
	skipScan                    bool
	bar                         progress.Model
	control                     *batch.Control
	engine                      backend.Engine
	options                     Options
	picker                      Model
	phase, err                  string
	index, focus, width, height int
	snapshot                    batch.Snapshot
	stopping, autoRun           bool
}

func NewBatch(c *batch.Control, x backend.Engine, o Options, autoResume bool) BatchModel {
	snap := c.Snapshot()
	phase := "batch-summary"
	if !snap.State.AllReviewed() {
		phase = "batch-planning"
	}
	if snap.State.Status == batchstate.Completed {
		phase = "batch-done"
		autoResume = false
	}
	o.Planning = true
	o.Profile = snap.State.Profile
	return BatchModel{bar: progress.New(progress.WithDefaultGradient()), control: c, engine: x, options: o, phase: phase, index: snap.State.PlanningISO, focus: snap.State.PlanningISO, width: 100, height: 32, snapshot: snap, autoRun: autoResume}
}
func tickBatch() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg { return batchTick{} })
}
func (m BatchModel) Init() tea.Cmd {
	if m.phase == "batch-planning" {
		return tea.Batch(func() tea.Msg { return openISO{index: m.index} }, tickBatch())
	}
	if m.autoRun {
		return tea.Batch(func() tea.Msg { return startBatch{} }, tickBatch())
	}
	return tickBatch()
}

type openISO struct{ index int }
type startBatch struct{}

func (m *BatchModel) refresh() { m.snapshot = m.control.Snapshot() }
func (m *BatchModel) open(i int) tea.Cmd {
	m.refresh()
	if i < 0 || i >= len(m.snapshot.State.ISOs) {
		return nil
	}
	if !m.snapshot.State.StartedAt.IsZero() {
		m.err = "Execution has started; selections are locked to protect completed work"
		m.phase = "batch-summary"
		return nil
	}
	if e := m.control.Navigate(i); e != nil {
		m.err = e.Error()
		return nil
	}
	m.index = i
	m.err = ""
	j := m.snapshot.State.ISOs[i]
	if j.Metadata == nil && len(j.Titles) == 0 {
		if m.picker.cancel != nil {
			m.picker.cancel()
		}
		m.phase = "batch-scan"
		ctx, cancel := context.WithCancel(m.control.Context())
		m.scanCancel = cancel
		m.skipScan = false
		return func() tea.Msg { defer cancel(); d, e := m.control.ScanContext(ctx, i); return batchScan{d, e} }
	}
	return m.installPicker(i)
}
func (m *BatchModel) installPicker(i int) tea.Cmd {
	m.refresh()
	j := m.snapshot.State.ISOs[i]
	if m.picker.cancel != nil {
		m.picker.cancel()
	}
	o := m.options
	o.Preferences = j.Preferences
	m.picker = NewWithOptions(m.control.Context(), m.engine, "", o)
	m.picker.phase = "titles"
	m.picker.source = "iso:" + filepath.Join(m.snapshot.State.InputRoot, filepath.FromSlash(j.RelativePath))
	m.picker.disc = media.NewDisc()
	m.picker.disc.Attributes = j.Metadata
	for _, t := range j.Titles {
		m.picker.titles = append(m.picker.titles, t.Metadata)
		m.picker.disc.Titles[t.ID] = t.Metadata
		m.picker.selected[t.ID] = t.Selected
	}
	m.picker.openFirstTitle()
	m.picker.width = m.width
	m.picker.height = max(1, m.height-2)
	m.phase = "batch-planning"
	return nil
}
func (m *BatchModel) savePicker(review bool) error {
	if m.picker.phase == "" {
		return nil
	}
	ids := []int{}
	for _, t := range m.picker.titles {
		if m.picker.selected[t.ID] {
			ids = append(ids, t.ID)
		}
	}
	return m.control.Selection(m.index, ids, m.picker.opts.Preferences, review)
}
func (m *BatchModel) next(skip bool) tea.Cmd {
	if skip {
		m.refresh()
		p := m.snapshot.State.ISOs[m.index].Preferences
		if e := m.control.Selection(m.index, nil, p, true); e != nil {
			m.err = e.Error()
			return nil
		}
	} else if e := m.savePicker(true); e != nil {
		m.err = e.Error()
		return nil
	}
	m.refresh()
	if m.index+1 < len(m.snapshot.State.ISOs) {
		return m.open(m.index + 1)
	}
	m.phase = "batch-summary"
	m.focus = 0
	return nil
}
func (m *BatchModel) run() tea.Cmd {
	m.refresh()
	if !m.snapshot.State.AllReviewed() {
		m.err = "Every ISO must be reviewed or explicitly skipped"
		return nil
	}
	m.phase = "batch-run"
	m.err = ""
	return func() tea.Msg { return batchFinished{m.control.Run()} }
}
func (m BatchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.bar.Width = max(10, min(65, v.Width-20))
		m.width = v.Width
		m.height = v.Height
		if m.phase == "batch-planning" {
			n, _ := m.picker.Update(tea.WindowSizeMsg{Width: v.Width, Height: max(1, v.Height-2)})
			m.picker = n.(Model)
		}
	case batchTick:
		m.refresh()
		return m, tickBatch()
	case openISO:
		cmd := m.open(v.index)
		return m, cmd
	case startBatch:
		cmd := m.run()
		return m, cmd
	case batchScan:
		if m.stopping {
			return m, tea.Quit
		}
		if m.skipScan {
			m.skipScan = false
			cmd := m.next(true)
			return m, cmd
		}
		if v.err != nil {
			m.err = v.err.Error()
			if e := m.control.ScanFailure(m.index, v.err); e != nil {
				m.err = e.Error()
			}
			m.phase = "batch-scan-error"
			return m, nil
		}
		if e := m.control.Metadata(m.index, v.disc); e != nil {
			m.err = e.Error()
			m.phase = "batch-scan-error"
			return m, nil
		}
		cmd := m.installPicker(m.index)
		return m, cmd
	case pickerNext:
		cmd := m.next(false)
		return m, cmd
	case batchFinished:
		m.refresh()
		if v.err != nil && !errors.Is(v.err, context.Canceled) {
			m.err = v.err.Error()
		}
		if m.stopping || (m.snapshot.State.Stop.Mode == "after_iso" && m.snapshot.State.Status == batchstate.Paused) {
			return m, tea.Quit
		}
		m.phase = "batch-done"
		return m, nil
	case BatchStopMsg:
		m.stopping = true
		if !v.AlreadyRequested {
			if e := m.control.StopNow(); e != nil {
				m.err = e.Error()
			}
		} else if v.Err != nil {
			m.err = v.Err.Error()
		}
		if m.phase != "batch-run" && m.phase != "batch-scan" {
			return m, tea.Quit
		}
		m.refresh()
		return m, nil
	case tea.KeyMsg:
		k := v.String()
		if k == "ctrl+e" && (m.phase != "batch-planning" || !m.picker.editing()) {
			m.showErrors = !m.showErrors
			m.errorOffset = 0
			m.logView = false
			return m, nil
		}
		if m.showErrors && k != "ctrl+c" && !(m.phase == "batch-run" && (k == "s" || k == "S")) {
			if k == "esc" {
				m.showErrors = false
			} else {
				m.errorOffset = min(scrollErrors(m.errorOffset, k, m.height), errorLimit(m.errorMessages(), m.width, m.height-1))
			}
			return m, nil
		}
		if k == "ctrl+l" {
			m.logView = !m.logView
			m.logOffset = 0
			return m, nil
		}
		if m.logView && k != "ctrl+c" && !(m.phase == "batch-run" && (k == "s" || k == "S")) {
			switch k {
			case "up", "pgup":
				m.logOffset = min(max(0, len(m.snapshot.Logs)-1), m.logOffset+max(1, m.height/2))
			case "down", "pgdown":
				m.logOffset = max(0, m.logOffset-max(1, m.height/2))
			case "esc":
				m.logView = false
			}
			return m, nil
		}
		if k == "ctrl+c" {
			cmd := m.stop()
			return m, cmd
		}
		if m.phase == "batch-run" {
			if k == "s" || k == "S" {
				if e := m.control.StopAfterISO(); e != nil {
					m.err = e.Error()
				}
				m.refresh()
			}
			return m, nil
		}
		if (k == "q" || k == "esc") && (m.phase != "batch-planning" || !m.picker.editing()) {
			cmd := m.stop()
			return m, cmd
		}
		switch m.phase {
		case "batch-scan":
			if k == "x" || k == "X" {
				m.skipScan = true
				if m.scanCancel != nil {
					m.scanCancel()
				}
			}
		case "batch-planning":
			if m.picker.phase == "titles" {
				switch k {
				case "x", "X":
					cmd := m.next(true)
					return m, cmd
				case "pgup":
					if e := m.savePicker(false); e != nil {
						m.err = e.Error()
						return m, nil
					}
					cmd := m.open(max(0, m.index-1))
					return m, cmd
				case "pgdown":
					cmd := m.next(false)
					return m, cmd
				case "f", "F":
					if e := m.savePicker(false); e != nil {
						m.err = e.Error()
					} else {
						m.phase = "batch-summary"
						m.focus = m.index
					}
					return m, nil
				}
			}
			n, cmd := m.picker.Update(v)
			m.picker = n.(Model)
			if m.picker.phase == "titles" {
				if e := m.savePicker(false); e != nil {
					m.err = e.Error()
					return m, nil
				}
			}
			return m, cmd
		case "batch-scan-error":
			switch k {
			case "x", "X":
				cmd := m.next(true)
				return m, cmd
			case "r", "R":
				cmd := m.open(m.index)
				return m, cmd
			case "pgup":
				cmd := m.open(max(0, m.index-1))
				return m, cmd
			}
		case "batch-summary":
			switch k {
			case "up":
				m.focus = max(0, m.focus-1)
			case "down":
				m.focus = min(len(m.snapshot.State.ISOs)-1, m.focus+1)
			case "e", "E":
				cmd := m.open(m.focus)
				return m, cmd
			case "r", "R":
				if e := m.control.Retry(); e != nil {
					m.err = e.Error()
				}
				m.refresh()
			case "enter":
				m.refresh()
				if !m.snapshot.State.AllReviewed() {
					m.err = "Review or skip every ISO first; use E to edit"
				} else {
					m.phase = "batch-confirm"
				}
			}
		case "batch-confirm":
			if k == "y" || k == "Y" {
				if e := m.control.Confirm(); e != nil {
					m.err = e.Error()
					return m, nil
				}
				cmd := m.run()
				return m, cmd
			}
			if k == "n" || k == "N" || k == "backspace" {
				m.phase = "batch-summary"
			}
		case "batch-done":
			if k == "r" || k == "R" {
				if e := m.control.Retry(); e != nil {
					m.err = e.Error()
				} else {
					m.phase = "batch-summary"
					m.err = ""
				}
				m.refresh()
			}
		}
	}
	return m, nil
}
func (m *BatchModel) stop() tea.Cmd {
	m.stopping = true
	if e := m.control.StopNow(); e != nil {
		m.err = e.Error()
	}
	if m.phase == "batch-run" || m.phase == "batch-scan" {
		return nil
	}
	return tea.Quit
}
func (m BatchModel) View() string {
	s := m.snapshot.State
	if m.showErrors {
		status := fmt.Sprintf("Batch %s · Stop: %s", s.Status, value(s.Stop.Mode, "none"))
		return fitView(status+"\n"+errorView(m.errorMessages(), m.errorOffset, m.width, max(1, m.height-1)), m.width, m.height)
	}
	if m.phase == "batch-planning" && m.picker.phase == "titles" && !m.logView {
		lines := []string{paint(fmt.Sprintf("ISO %d/%d · PLANNING ONLY · makemkv-tui", m.index+1, len(s.ISOs)), neutralColor), safe(s.ISOs[m.index].RelativePath)}
		if m.err != "" {
			lines = append(lines, errorText(m.err))
		}
		picker := m.picker
		picker.width = m.width
		picker.height = max(1, m.height-len(lines))
		return fitView(strings.Join(lines, "\n")+"\n"+picker.View(), m.width, m.height)
	}
	var b strings.Builder
	b.WriteString(paint("makemkv-tui — Batch Processing", neutralColor) + "\n")
	fmt.Fprintf(&b, "Discovered: %d ISOs · Status: %s · Saved: %s\n", len(s.ISOs), s.Status, s.UpdatedAt.Local().Format("15:04:05"))
	if len(s.Errors) > 0 || m.err != "" {
		b.WriteString(errorText("Ctrl+E expands full errors; staging is retained") + "\n")
	}
	if m.logView {
		end := max(0, len(m.snapshot.Logs)-m.logOffset)
		start := max(0, end-max(3, m.height-9))
		fmt.Fprintf(&b, "Latest 500 log lines · PgUp/PgDown · Ctrl+L/Esc closes\nStop: %s · Full log: %s\n", s.Stop.Mode, safe(m.options.LogPath))
		for _, line := range m.snapshot.Logs[start:end] {
			b.WriteString(safe(line) + "\n")
		}
		return fitView(b.String(), m.width, m.height)
	}
	switch m.phase {
	case "batch-planning", "batch-scan", "batch-scan-error":
		fmt.Fprintf(&b, "ISO %d/%d · %s\n", m.index+1, len(s.ISOs), safe(s.ISOs[m.index].RelativePath))
		b.WriteString("PLANNING ONLY — no extraction · Enter/PgDn: review & next · PgUp: back · X: skip · F: summary\n")
		if m.phase == "batch-planning" {
			b.WriteString(m.picker.View())
		} else if m.phase == "batch-scan" {
			b.WriteString("Reading title metadata… Ctrl+C cancels safely\n")
		} else {
			b.WriteString("Scan failed. R retries; X skips; PgUp goes back.\n")
		}
	case "batch-summary", "batch-confirm":
		bytes, unknown := batch.Estimate(&s)
		fmt.Fprintf(&b, "Selected titles: %d · Estimated size: %.2f GiB · Unknown estimates: %d\n", s.Progress.Selected, float64(bytes)/(1<<30), unknown)
		lines := []string{}
		focusLine := 0
		selectedISOs, skipped := 0, 0
		for i, j := range s.ISOs {
			tag := " "
			if i == m.focus {
				tag = ">"
				focusLine = len(lines)
			}
			ids := batch.Selected(j)
			status := "selected"
			if !j.Reviewed {
				status = "UNREVIEWED"
			} else if len(ids) == 0 {
				status = "skipped"
				skipped++
			} else {
				selectedISOs++
			}
			lines = append(lines, fmt.Sprintf("%s ISO %d · %s · %s · titles %v", tag, i+1, safe(j.RelativePath), status, ids))
			for _, t := range j.Titles {
				if t.Selected {
					lines = append(lines, fmt.Sprintf("    ID %d · %s · %.2f GiB → %s", t.ID, t.Status, float64(t.EstimatedBytes)/(1<<30), safe(filepath.Join(s.OutputRoot, filepath.FromSlash(t.Output)))))
				}
			}
		}
		fmt.Fprintf(&b, "Selected ISOs: %d · Skipped files: %d\n", selectedISOs, skipped)
		b.WriteString(strings.Join(window(lines, focusLine, max(4, m.height-13)), "\n") + "\n")
		if m.phase == "batch-summary" {
			b.WriteString("↑/↓: ISO · E: edit selections · R: retry failed titles · Enter: final confirmation · q: save & exit\n")
		} else {
			b.WriteString("Start extraction of this saved plan? Y explicitly starts · N returns to summary\n")
		}
	case "batch-run", "batch-done":
		if s.CurrentISO >= 0 && s.CurrentISO < len(s.ISOs) {
			j := s.ISOs[s.CurrentISO]
			pos, total := 0, 0
			for _, t := range j.Titles {
				if t.Selected {
					total++
					if t.ID == s.CurrentTitle {
						pos = total
					}
				}
			}
			fmt.Fprintf(&b, "ISO: %d/%d · %s\nTitle: %d/%d · ID %d\n", s.CurrentISO+1, len(s.ISOs), safe(j.RelativePath), pos, total, s.CurrentTitle)
		}
		fmt.Fprintf(&b, "Current process: %.1f%% · Current title: %.1f%% · Overall: %.1f%%\n", 100*m.snapshot.ProcessProgress, 100*s.Progress.CurrentFraction, 100*s.Progress.Overall)
		b.WriteString("Title   " + m.bar.ViewAs(s.Progress.CurrentFraction) + "\nOverall " + m.bar.ViewAs(s.Progress.Overall) + "\n")
		fmt.Fprintf(&b, "Completed: %d · Pending/running: %d · Interrupted: %d · Failed: %d\n", s.Progress.Completed, s.Progress.Pending, s.Progress.Interrupted, s.Progress.Failed)
		fmt.Fprintf(&b, "Operation: %s · Elapsed execution: %s\n", safe(s.Operation), time.Duration(s.ElapsedSeconds*float64(time.Second)).Round(time.Second))
		if m.phase == "batch-run" {
			b.WriteString("[Ctrl+C] stop immediately (again: force) · [S] stop after current ISO\n")
		} else {
			b.WriteString("R: retry failed titles · q: exit\n")
		}
	}
	if s.Stop.Mode != "" {
		fmt.Fprintf(&b, "\nSTOP REQUEST: %s · ISO %d · %s\n", s.Stop.Mode, s.Stop.ISO+1, s.Stop.RequestedAt.Local().Format("15:04:05"))
	}
	if m.stopping {
		b.WriteString("Stopping immediately; waiting for subprocess cleanup and state persistence…\n")
	}
	if m.err != "" {
		b.WriteString("\n" + errorText(m.err) + "\n")
	}
	for _, line := range recentLogs(m.snapshot.Logs, 4) {
		b.WriteString(line + "\n")
	}
	return fitView(b.String(), m.width, m.height)
}
