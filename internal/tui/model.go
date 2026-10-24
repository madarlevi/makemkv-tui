package tui

import (
	"context"
	"fmt"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/madarlevi/makemkv-tui/internal/backend"
	"github.com/madarlevi/makemkv-tui/internal/media"
	"github.com/madarlevi/makemkv-tui/internal/preferences"
	"io"
	"path/filepath"
	"strings"
	"unicode"
)

type cancelled struct{}
type event struct {
	backend.Event
	disc      *media.Disc
	drives    []backend.Drive
	artifacts []backend.Artifact
	inventory backend.Inventory
	path      string
	err       error
	done      bool
}
type Options struct {
	Planning                     bool // Reuse the title selector without exposing any extraction action.
	Preferences                  preferences.Preferences
	ConfigPath, Profile, LogPath string
	Log                          io.Writer
	Remuxer                      backend.Remuxer
}
type row struct {
	title       int
	stream      int
	kind, label string
}
type Model struct {
	prefOriginal                           preferences.Preferences
	returnInput                            string
	engine                                 backend.Engine
	opts                                   Options
	ctx                                    context.Context
	cancel                                 context.CancelFunc
	input                                  textinput.Model
	bar                                    progress.Model
	phase, source, out, err                string
	titles                                 []*media.Title
	disc                                   *media.Disc
	selected                               map[int]bool
	expanded                               map[string]bool
	names                                  map[int]string
	cursor                                 int
	ids                                    []int
	events                                 chan event
	logs                                   []string
	logOffset                              int
	showLogs                               bool
	showErrors                             bool
	errorOffset                            int
	current, total                         float64
	index, id                              int
	operation                              string
	quitting                               bool
	isos                                   []string
	isoCursor, height, width, detailOffset int
	entries                                []backend.Entry
	directory                              string
	browserCursor                          int
	drives                                 []backend.Drive
	artifacts                              []backend.Artifact
	artifactCursor                         int
	inventory                              backend.Inventory
	trackSelected                          map[int]bool
	trackCursor                            int
	remuxInput, remuxName                  string
	editID, prefField                      int
	returnPhase                            string
}

func New(ctx context.Context, x backend.Engine, initial string) Model {
	return NewWithOptions(ctx, x, initial, Options{Preferences: preferences.Defaults()})
}
func NewWithOptions(ctx context.Context, x backend.Engine, initial string, o Options) Model {
	ctx, cancel := context.WithCancel(ctx)
	in := textinput.New()
	in.CharLimit = 4096
	in.Width = 70
	in.SetValue(initial)
	in.Focus()
	dir, _ := filepath.Abs(".")
	entries, _ := backend.Browse(dir)
	var isos []string
	for _, e := range entries {
		if !e.Directory {
			isos = append(isos, e.Path)
		}
	}
	if o.Preferences.Version == 0 {
		o.Preferences = preferences.Defaults()
	}
	return Model{engine: x, opts: o, ctx: ctx, cancel: cancel, input: in, bar: progress.New(progress.WithDefaultGradient()), phase: "source", selected: map[int]bool{}, expanded: map[string]bool{}, names: map[int]string{}, isos: isos, directory: dir, height: 30, width: 90}
}
func (m Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, func() tea.Msg { <-m.ctx.Done(); return cancelled{} })
}
func wait(ch <-chan event) tea.Cmd { return func() tea.Msg { return <-ch } }
func (m Model) busy() bool {
	return m.phase == "scan" || m.phase == "run" || m.phase == "drive-scan" || m.phase == "identify" || m.phase == "remux"
}
func (m Model) editing() bool {
	switch m.phase {
	case "source", "output", "filename", "preferences", "remux-name":
		return true
	}
	return false
}
func (m *Model) start(op string) tea.Cmd {
	if m.opts.Planning && op != "scan" {
		return func() tea.Msg { return event{done: true, err: fmt.Errorf("extraction is forbidden during planning")} }
	}
	ch := make(chan event, 64)
	m.events = ch
	ctx := m.ctx
	x := m.engine
	r := backend.Request{Source: m.source, Output: m.out, IDs: append([]int(nil), m.ids...), Filenames: map[int]string{}, Profile: m.opts.Profile}
	for k, v := range m.names {
		r.Filenames[k] = v
	}
	remux := m.opts.Remuxer
	input, name, inv := m.remuxInput, m.remuxName, m.inventory
	sel := map[int]bool{}
	for k, v := range m.trackSelected {
		sel[k] = v
	}
	send := func(e backend.Event) {
		select {
		case ch <- event{Event: e}:
		case <-ctx.Done():
		}
	}
	go func() {
		e := event{done: true}
		switch op {
		case "scan":
			e.disc, e.err = x.Scan(ctx, r.Source, send)
		case "drives":
			e.drives, e.err = x.Drives(ctx, send)
		case "extract":
			e.artifacts, e.err = x.Extract(ctx, r, send)
		case "identify":
			if remux == nil {
				e.err = fmt.Errorf("install MKVToolNix (mkvmerge) and restart to review tracks")
			} else {
				e.inventory, e.err = remux.Identify(ctx, input)
			}
		case "remux":
			e.path, e.err = remux.Remux(ctx, input, name, inv, sel, send)
		}
		ch <- e
	}()
	return wait(ch)
}
func safe(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}
func (m *Model) log(s string) {
	if s == "" {
		return
	}
	if m.opts.Log != nil {
		if _, e := fmt.Fprintln(m.opts.Log, safe(s)); e != nil {
			m.err = "Could not write session log: " + e.Error()
			m.opts.Log = nil
		}
	}
	m.logs = append(m.logs, safe(s))
	if len(m.logs) > 5000 {
		m.logs = m.logs[len(m.logs)-5000:]
	}
}
func value(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return safe(s)
}
func (m *Model) edit(phase, value string) {
	m.phase = phase
	m.input.SetValue(value)
	m.input.CursorEnd()
	m.input.Focus()
	m.err = ""
}
func (m *Model) browse(dir string) {
	p, e := filepath.Abs(dir)
	if e != nil {
		m.err = e.Error()
		return
	}
	entries, e := backend.Browse(p)
	if e != nil {
		m.err = e.Error()
		return
	}
	m.entries = entries
	m.directory = p
	m.browserCursor = 0
	m.phase = "browser"
	m.err = ""
}
func (m Model) prefValue() string {
	switch m.prefField {
	case 0:
		return m.opts.Preferences.OutputDir
	case 1:
		return m.opts.Preferences.AudioLanguages
	case 2:
		return m.opts.Preferences.SubtitleLanguages
	default:
		return m.opts.Preferences.FilenameTemplate
	}
}
func (m *Model) commitPref() {
	s := m.input.Value()
	switch m.prefField {
	case 0:
		m.opts.Preferences.OutputDir = s
	case 1:
		m.opts.Preferences.AudioLanguages = s
	case 2:
		m.opts.Preferences.SubtitleLanguages = s
	case 3:
		m.opts.Preferences.FilenameTemplate = s
	}
}
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case cancelled:
		if m.busy() {
			m.quitting = true
			return m, nil
		}
		return m, tea.Quit
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
		m.input.Width = max(10, v.Width-8)
		m.bar.Width = max(10, min(70, v.Width-15))
	case event:
		if v.done {
			if m.quitting {
				return m, tea.Quit
			}
			if len(v.artifacts) > 0 {
				m.artifacts = v.artifacts
			}
			if v.err != nil {
				m.err = v.err.Error()
				m.log("ERROR: " + m.err)
				m.phase = "failed"
				return m, nil
			}
			switch m.phase {
			case "scan":
				m.disc = v.disc
				m.titles = v.disc.Sorted()
				m.cursor = 0
				m.openFirstTitle()
				m.phase = "titles"
			case "drive-scan":
				m.drives = v.drives
				m.browserCursor = 0
				m.phase = "drives"
			case "run":
				m.phase = "done"
				m.total = 1
				m.current = 1
			case "identify":
				m.inventory = v.inventory
				m.trackSelected = map[int]bool{}
				m.trackCursor = 0
				for _, t := range v.inventory.Tracks {
					keep := true
					switch t.Type {
					case "audio":
						keep = preferences.Matches(m.opts.Preferences.AudioLanguages, t.Properties.Language)
					case "subtitles":
						keep = preferences.Matches(m.opts.Preferences.SubtitleLanguages, t.Properties.Language)
					}
					m.trackSelected[t.ID] = keep
				}
				for _, w := range v.inventory.Warnings {
					m.log("Warning: " + w)
				}
				m.phase = "tracks"
			case "remux":
				m.log("Remux saved: " + v.path)
				if len(m.artifacts) > 0 {
					m.artifacts = append(m.artifacts, backend.Artifact{TitleID: m.artifacts[m.artifactCursor].TitleID, Path: v.path})
					m.artifactCursor = len(m.artifacts) - 1
				}
				m.phase = "done"
			}
			return m, nil
		}
		m.log(v.Line)
		m.index = v.Index
		m.id = v.TitleID
		if v.Operation != "" {
			m.operation = safe(v.Operation)
		}
		if v.Progress {
			m.current = v.Current
			m.total = v.Total
		}
		return m, wait(m.events)
	case tea.KeyMsg:
		k := v.String()
		if k == "ctrl+e" && !m.editing() {
			m.showErrors = !m.showErrors
			m.errorOffset = 0
			m.showLogs = false
			return m, nil
		}
		if m.showErrors && k != "ctrl+c" {
			if k == "esc" {
				m.showErrors = false
			} else {
				m.errorOffset = min(scrollErrors(m.errorOffset, k, m.height), errorLimit(m.errorMessages(), m.width, m.height))
			}
			return m, nil
		}
		if k == "ctrl+l" {
			m.showErrors = false
			m.showLogs = !m.showLogs
			m.logOffset = 0
			return m, nil
		}
		if m.showLogs {
			switch k {
			case "up", "pgup":
				m.logOffset = min(max(0, len(m.logs)-1), m.logOffset+max(1, m.height/2))
			case "down", "pgdown":
				m.logOffset = max(0, m.logOffset-max(1, m.height/2))
			case "esc":
				m.showLogs = false
			}
			if k != "ctrl+c" {
				return m, nil
			}
		}
		if k == "ctrl+c" || ((k == "q" || k == "esc") && !m.editing() && m.phase != "browser" && m.phase != "tracks" && m.phase != "remux-confirm") {
			if m.busy() {
				m.cancel()
				m.quitting = true
				m.operation = "Cancelling; waiting for engine cleanup"
				return m, nil
			}
			m.cancel()
			return m, tea.Quit
		}
		if !m.busy() && m.phase != "preferences" && (k == "ctrl+p") {
			m.returnInput = m.input.Value()
			m.prefOriginal = m.opts.Preferences
			m.returnPhase = m.phase
			m.prefField = 0
			m.edit("preferences", m.prefValue())
			return m, textinput.Blink
		}
		switch m.phase {
		case "source":
			if k == "ctrl+o" {
				m.browse(m.directory)
				return m, nil
			}
			if k == "ctrl+d" {
				m.phase = "drive-scan"
				return m, m.start("drives")
			}
			if k == "tab" && len(m.isos) > 0 {
				m.input.SetValue(m.isos[m.isoCursor%len(m.isos)])
				m.isoCursor++
				return m, nil
			}
			if k == "enter" {
				s, e := m.engine.NormalizeSource(m.input.Value())
				if e != nil {
					m.err = e.Error()
					return m, nil
				}
				m.source = s
				m.artifacts = nil
				m.selected = map[int]bool{}
				m.names = map[int]string{}
				m.expanded = map[string]bool{}
				m.err = ""
				m.phase = "scan"
				return m, m.start("scan")
			}
		case "browser":
			if k == "esc" {
				m.phase = "source"
				return m, nil
			}
			if k == "left" || k == "backspace" {
				m.browse(filepath.Dir(m.directory))
				return m, nil
			}
			if k == "up" {
				m.browserCursor = max(0, m.browserCursor-1)
			}
			if k == "down" {
				m.browserCursor = min(max(0, len(m.entries)-1), m.browserCursor+1)
			}
			if k == "." {
				m.edit("source", m.directory)
				return m, nil
			}
			if len(m.entries) > 0 && (k == "enter" || k == " ") {
				e := m.entries[m.browserCursor]
				if e.Directory && k == "enter" {
					m.browse(e.Path)
				} else {
					m.edit("source", e.Path)
				}
				return m, nil
			}
		case "drives":
			if k == "up" {
				m.browserCursor = max(0, m.browserCursor-1)
			}
			if k == "down" {
				m.browserCursor = min(max(0, len(m.drives)-1), m.browserCursor+1)
			}
			if k == "enter" && len(m.drives) > 0 {
				d := m.drives[m.browserCursor]
				m.edit("source", fmt.Sprintf("disc:%d", d.Index))
				return m, nil
			}
		case "titles":
			rows := m.rows()
			if len(rows) == 0 {
				return m, nil
			}
			m.cursor = min(m.cursor, len(rows)-1)
			r := rows[m.cursor]
			t := m.titles[r.title]
			switch k {
			case "up", "k":
				m.cursor = max(0, m.cursor-1)
				m.detailOffset = 0
			case "down", "j":
				m.cursor = min(len(rows)-1, m.cursor+1)
				m.detailOffset = 0
			case "[":
				m.detailOffset = max(0, m.detailOffset-1)
			case "]":
				m.detailOffset++
			case "right", "left":
				m.fold(k == "right", false)
			case "tab":
				m.fold(false, true)
			case " ":
				if r.kind == "title" {
					m.selected[t.ID] = !m.selected[t.ID]
					m.err = ""
				} else {
					m.err = "Source stream selection is unavailable in this backend. After extraction press r to review actual MKV tracks."
				}
			case "a", "A":
				for _, t := range m.titles {
					m.selected[t.ID] = true
				}
			case "d", "D":
				for _, t := range m.titles {
					m.selected[t.ID] = false
				}
			case "n":
				if m.opts.Planning {
					m.err = "Batch filenames are fixed as title<ID>.mkv"
					return m, nil
				}
				m.editID = t.ID
				m.edit("filename", m.names[t.ID])
				return m, textinput.Blink
			case "enter":
				if m.opts.Planning {
					return m, func() tea.Msg { return pickerNext{} }
				}
				// Enter remains the existing planning/output shortcut. With no
				// selection in single-source mode it can instead fold the tree.
				if r.stream >= 0 || (r.kind == "title" && len(t.Streams) > 0) {
					anySelected := false
					for _, title := range m.titles {
						anySelected = anySelected || m.selected[title.ID]
					}
					if !anySelected {
						m.fold(false, true)
						return m, nil
					}
				}
				m.ids = nil
				for _, t := range m.titles {
					if m.selected[t.ID] {
						m.ids = append(m.ids, t.ID)
						if m.names[t.ID] == "" && m.opts.Preferences.FilenameTemplate != "" {
							name := strings.ReplaceAll(m.opts.Preferences.FilenameTemplate, "{id}", fmt.Sprint(t.ID))
							clean := strings.Map(func(r rune) rune {
								if unicode.IsControl(r) || strings.ContainsRune("/\\:<>\"|?*", r) {
									return '_'
								}
								return r
							}, t.Attributes[2])
							m.names[t.ID] = strings.ReplaceAll(name, "{name}", clean)
						}
					}
				}
				if len(m.ids) == 0 {
					m.err = "Select at least one title with Space"
				} else {
					m.edit("output", m.opts.Preferences.OutputDir)
					return m, textinput.Blink
				}
			}
		case "filename":
			if k == "enter" {
				name := m.input.Value()
				if name != "" {
					if e := preferences.ValidateName(name); e != nil {
						m.err = e.Error()
						return m, nil
					}
				}
				m.names[m.editID] = name
				m.phase = "titles"
				return m, nil
			}
			if k == "esc" {
				m.phase = "titles"
				return m, nil
			}
		case "preferences":
			if k == "esc" {
				m.opts.Preferences = m.prefOriginal
				m.input.SetValue(m.returnInput)
				m.phase = m.returnPhase
				return m, nil
			}
			if k == "tab" {
				m.commitPref()
				m.prefField = (m.prefField + 1) % 4
				m.input.SetValue(m.prefValue())
				return m, nil
			}
			if k == "enter" {
				m.commitPref()
				if e := preferences.Save(m.opts.ConfigPath, m.opts.Preferences); e != nil {
					m.err = e.Error()
				} else {
					m.phase = m.returnPhase
					m.input.SetValue(m.returnInput)
					m.log("Preferences saved to " + m.opts.ConfigPath)
				}
				return m, nil
			}
		case "output":
			if k == "enter" {
				p, e := backend.Output(m.input.Value())
				if e != nil {
					m.err = e.Error()
				} else {
					m.out = p
					m.phase = "confirm"
					m.err = ""
				}
				return m, nil
			}
			if k == "esc" {
				m.phase = "titles"
				return m, nil
			}
		case "confirm":
			if k == "backspace" {
				m.edit("output", m.out)
				return m, nil
			}
			if k == "enter" {
				m.phase = "run"
				m.id = m.ids[0]
				return m, m.start("extract")
			}
		case "done", "failed":
			if k == "up" {
				m.artifactCursor = max(0, m.artifactCursor-1)
			}
			if k == "down" {
				m.artifactCursor = min(max(0, len(m.artifacts)-1), m.artifactCursor+1)
			}
			if k == "r" && len(m.artifacts) > 0 {
				m.remuxInput = m.artifacts[m.artifactCursor].Path
				m.phase = "identify"
				m.err = ""
				return m, m.start("identify")
			}
			if k == "b" {
				m.edit("source", m.source)
				return m, nil
			}
		case "tracks":
			if k == "esc" {
				m.phase = "done"
				return m, nil
			}
			if k == "up" {
				m.trackCursor = max(0, m.trackCursor-1)
			}
			if k == "down" {
				m.trackCursor = min(len(m.inventory.Tracks)-1, m.trackCursor+1)
			}
			if k == " " && len(m.inventory.Tracks) > 0 {
				t := m.inventory.Tracks[m.trackCursor]
				if t.Type == "audio" || t.Type == "subtitles" {
					m.trackSelected[t.ID] = !m.trackSelected[t.ID]
				} else {
					m.err = "Video and other tracks are preserved by this remux workflow"
				}
			}
			if k == "a" || k == "A" {
				for _, t := range m.inventory.Tracks {
					m.trackSelected[t.ID] = true
				}
			}
			if k == "enter" {
				base := strings.TrimSuffix(filepath.Base(m.remuxInput), filepath.Ext(m.remuxInput))
				m.edit("remux-name", base+".selected.mkv")
				return m, textinput.Blink
			}
		case "remux-name":
			if k == "esc" {
				m.phase = "tracks"
				return m, nil
			}
			if k == "enter" {
				if e := preferences.ValidateName(m.input.Value()); e != nil {
					m.err = e.Error()
					return m, nil
				}
				m.remuxName = m.input.Value()
				if _, e := backend.RemuxArgs(m.remuxInput, m.remuxName, m.inventory, m.trackSelected); e != nil {
					m.err = e.Error()
					return m, nil
				}
				m.phase = "remux-confirm"
				return m, nil
			}
		case "remux-confirm":
			if k == "esc" {
				m.phase = "tracks"
				return m, nil
			}
			if k == "enter" {
				m.phase = "remux"
				return m, m.start("remux")
			}
		}
	}
	if m.editing() {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}
func window(lines []string, cursor, n int) []string {
	if len(lines) == 0 {
		return nil
	}
	start := max(0, min(cursor, len(lines)-1)-n+1)
	return lines[start:min(len(lines), start+n)]
}
func (m Model) View() string {
	if m.showErrors {
		return errorView(m.errorMessages(), m.errorOffset, m.width, m.height)
	}
	if m.phase == "titles" && !m.showLogs {
		return m.explorerView()
	}
	var b strings.Builder
	b.WriteString(paint("makemkv-tui", neutralColor) + " / " + m.phase + "\n\n")
	if m.showLogs {
		n := max(2, m.height-8)
		end := max(0, len(m.logs)-m.logOffset)
		start := max(0, end-n)
		b.WriteString("Session log · PgUp/PgDown · Ctrl+L closes\n" + safe(m.opts.LogPath) + "\n" + strings.Join(m.logs[start:end], "\n"))
		return fitView(b.String(), m.width, m.height)
	}
	switch m.phase {
	case "source":
		b.WriteString("ISO / BDMV / DVD folder / disc:N / dev:/device\n" + m.input.View() + "\nTab: local ISOs · Ctrl+O: browse · Ctrl+D: drives · Ctrl+P: preferences · Enter: scan\n")
	case "browser":
		b.WriteString(safe(m.directory) + "\nEnter: open folder/choose ISO · Space: choose folder · .: current folder · Left: parent · Esc: back\n")
		lines := []string{}
		for i, e := range m.entries {
			tag := " "
			if i == m.browserCursor {
				tag = ">"
			}
			suffix := ""
			if e.Directory {
				suffix = "/"
			}
			lines = append(lines, tag+" "+safe(e.Name)+suffix)
		}
		b.WriteString(strings.Join(window(lines, m.browserCursor, max(2, m.height-10)), "\n"))
	case "drives":
		b.WriteString("↑/↓ · Enter: choose drive · q: quit\n")
		for i, d := range m.drives {
			tag := " "
			if i == m.browserCursor {
				tag = ">"
			}
			fmt.Fprintf(&b, "%s disc:%d · %s · %s · engine state=%s · %s\n", tag, d.Index, safe(d.Name), safe(d.Disc), safe(d.State), safe(d.Device))
		}
		if len(m.drives) == 0 {
			b.WriteString("No drives reported; check device permissions and FreeBSD drive mapping.")
		}
	case "filename":
		b.WriteString("Title filename (.mkv basename; blank uses engine default)\n" + m.input.View() + "\nEnter: apply · Esc: back\n")
	case "preferences":
		labels := []string{"Default output directory", "Audio language preferences (eng,jpn; blank=all)", "Subtitle language preferences (eng; blank=all)", "Filename template ({id}, {name}; blank=engine default)"}
		b.WriteString(labels[m.prefField] + "\n" + m.input.View() + "\nTab: next field · Enter: save all · Esc: back\nLanguage preferences apply to post-extraction review, not MakeMKV source selection.\n")
	case "output":
		b.WriteString("Output directory\n" + m.input.View() + "\nEnter: validate · Esc: back\n")
	case "confirm":
		fmt.Fprintf(&b, "Source: %s\nOutput: %s\n", safe(m.source), safe(m.out))
		for _, id := range m.ids {
			fmt.Fprintf(&b, "Title %d → %s\n", id, value(m.names[id], "engine filename"))
		}
		if m.opts.Profile != "" {
			b.WriteString("Profile: " + safe(m.opts.Profile) + " (version-dependent selection behavior)\n")
		}
		b.WriteString("Enter: extract · Backspace: edit · q: cancel\n")
	case "scan", "drive-scan", "identify":
		b.WriteString("Reading engine data… q / Ctrl+C cancels\n")
	case "run", "remux":
		fmt.Fprintf(&b, "Title %d · %s\nCurrent %s\nOverall %s\nq / Ctrl+C cancels and waits for cleanup\n", m.id, safe(m.operation), m.bar.ViewAs(m.current), m.bar.ViewAs(m.total))
	case "done", "failed":
		if m.phase == "done" {
			b.WriteString("Completed.\n")
		} else {
			b.WriteString("Operation stopped. Completed files are retained.\n")
		}
		for i, a := range m.artifacts {
			tag := " "
			if i == m.artifactCursor {
				tag = ">"
			}
			fmt.Fprintf(&b, "%s Title %d · %s\n", tag, a.TitleID, safe(a.Path))
		}
		b.WriteString("↑/↓: file · r: review actual MKV tracks · b: new source · q: quit\n")
	case "tracks":
		b.WriteString("Actual output tracks · ↑/↓ · Space: audio/subtitle · A: all · Enter: output name · Esc: back\n")
		b.WriteString(legend() + "\n")
		lines := []string{}
		for i, t := range m.inventory.Tracks {
			tag := " "
			if i == m.trackCursor {
				tag = ">"
			}
			check := "[ ]"
			if m.trackSelected[t.ID] {
				check = "[x]"
			}
			kind := outputKind(t.Type)
			lines = append(lines, paint(fmt.Sprintf("%s %s %s ID %d · %s · %s · %s · default=%t forced=%t", tag, check, kind, t.ID, safe(t.Codec), safe(t.Properties.Language), safe(t.Properties.Name), t.Properties.Default, t.Properties.Forced), kindColor(kind)))
		}
		b.WriteString(strings.Join(window(lines, m.trackCursor, max(2, m.height-14)), "\n") + "\n")
		for _, c := range m.inventory.Chapters {
			fmt.Fprintf(&b, "Chapters: %d (preserved)\n", c.Count)
		}
		b.WriteString("Only tracks present in the MKV can be kept. Video, chapters and attachments are preserved.\n")
	case "remux-name":
		b.WriteString("New remux filename (original is retained)\n" + m.input.View() + "\nEnter: review · Esc: back\n")
	case "remux-confirm":
		fmt.Fprintf(&b, "Remux source: %s\nNew filename: %s\n", safe(m.remuxInput), safe(m.remuxName))
		for _, t := range m.inventory.Tracks {
			if t.Type == "audio" || t.Type == "subtitles" {
				b.WriteString(paint(fmt.Sprintf("ID %d · %s · keep=%t\n", t.ID, outputKind(t.Type), m.trackSelected[t.ID]), kindColor(outputKind(t.Type))))
			}
		}
		b.WriteString("Original remains untouched. Enter: remux · Esc: edit tracks\n")
	}
	if m.err != "" {
		b.WriteString("\n" + errorText(m.err) + "\nCtrl+E: expand full error\n")
	}
	if !m.busy() || !m.showLogs {
		logs := recentLogs(m.logs, 3)
		if len(logs) > 0 {
			b.WriteString("\nRecent logs (Ctrl+L for diagnostics)\n" + strings.Join(logs, "\n"))
		}
	}
	return fitView(b.String(), m.width, m.height)
}
