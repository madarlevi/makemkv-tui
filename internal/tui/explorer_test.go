package tui

import (
	"bufio"
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/madarlevi/makemkv-tui/internal/backend"
	"github.com/madarlevi/makemkv-tui/internal/makemkv"
	"github.com/madarlevi/makemkv-tui/internal/media"
	"github.com/muesli/termenv"
	"os"
	"strings"
	"testing"
)

func explorerFixture(t *testing.T) Model {
	t.Helper()
	f, e := os.Open("../makemkv/testdata/explorer.txt")
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	d := makemkv.NewDisc()
	scanner := bufio.NewScanner(f)
	var logs []string
	for scanner.Scan() {
		logs = append(logs, scanner.Text())
		r, e := makemkv.Parse(scanner.Text())
		if e != nil {
			t.Fatal(e)
		}
		d.Apply(r)
	}
	if e = scanner.Err(); e != nil {
		t.Fatal(e)
	}
	m := New(context.Background(), backend.MakeMKV{Runner: unusedRunner{}}, "")
	t.Cleanup(m.cancel)
	m.phase = "titles"
	m.source = "iso:/media/Évadsorozat.iso"
	m.disc = d.Disc
	m.titles = d.Sorted()
	m.logs = logs
	m.width = 110
	m.height = 32
	m.openFirstTitle()
	return m
}

func assertFits(t *testing.T, view string, w, h int) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) > h {
		t.Fatalf("height %d > %d\n%s", len(lines), h, view)
	}
	for _, line := range lines {
		if lipgloss.Width(line) > w {
			t.Fatalf("width %d > %d: %q", lipgloss.Width(line), w, line)
		}
	}
}

func TestExplorerColorsAndMonochrome(t *testing.T) {
	m := explorerFixture(t)
	old := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	t.Setenv("NO_COLOR", "")
	lipgloss.SetColorProfile(termenv.TrueColor)
	view := m.View()
	if videoColor.TrueColor != "#FF6B6B" || audioColor.TrueColor != "#4ADE80" || subColor.TrueColor != "#60A5FA" {
		t.Fatal("palette changed")
	}
	// termenv/colorful use floating-point conversion: green's red channel may
	// truncate to 73 or round to 74 across compilers/architectures. The exact
	// product palette is asserted above; both renderer representations are valid.
	for _, rgb := range []string{"38;2;255;107;107", "38;2;96;165;250"} {
		if !strings.Contains(view, rgb) {
			t.Fatalf("missing %s in %q", rgb, view)
		}
	}
	if !strings.Contains(view, "38;2;73;222;128") && !strings.Contains(view, "38;2;74;222;128") {
		t.Fatal("missing green AUDIO foreground", view)
	}
	for _, label := range []string{"VIDEO", "AUDIO", "SUB", "Magyar", "DTS-HD MA", "PGS", "N/A"} {
		if !strings.Contains(view, label) {
			t.Fatalf("missing %s\n%s", label, view)
		}
	}
	lipgloss.SetColorProfile(termenv.ANSI)
	view = m.View()
	for _, color := range []string{"\x1b[91m", "\x1b[92m", "\x1b[94m"} {
		if !strings.Contains(view, color) {
			t.Fatalf("missing ANSI fallback %q in %q", color, view)
		}
	}
	t.Setenv("NO_COLOR", "1")
	lipgloss.SetColorProfile(termenv.TrueColor) // NO_COLOR wins over forced styling.
	m.err = "Example error"
	view = m.View()
	if strings.Contains(view, "\x1b") || !strings.Contains(view, "> ▼ [ ] Title 6") || !strings.Contains(view, "ERROR: Example error") {
		t.Fatal(view)
	}
	for _, phase := range []string{"scan", "run", "tracks", "failed"} {
		m.phase = phase
		if strings.Contains(m.View(), "\x1b") {
			t.Fatal("NO_COLOR failed in", phase)
		}
	}
}

func TestExplorerFoldingNavigationAndSelection(t *testing.T) {
	m := explorerFixture(t)
	if len(m.rows()) != 11 {
		t.Fatal(m.rows())
	}
	m = key(m, tea.KeyMsg{Type: tea.KeySpace})
	if !m.selected[6] {
		t.Fatal("actual title ID not selected")
	}
	m = key(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.rows()[m.cursor].stream != 0 {
		t.Fatal("video row missing")
	}
	m = key(m, tea.KeyMsg{Type: tea.KeyRight})
	if !m.expanded["s6/0"] || len(m.rows()) <= 11 {
		t.Fatal("stream not expanded")
	}
	m = key(m, tea.KeyMsg{Type: tea.KeyDown}) // First stream field.
	m = key(m, tea.KeyMsg{Type: tea.KeyLeft})
	if m.rows()[m.cursor].kind != "stream" || m.expanded["s6/0"] {
		t.Fatal("field collapse lost focus")
	}
	m = key(m, tea.KeyMsg{Type: tea.KeySpace})
	if m.err == "" || len(m.selected) != 1 {
		t.Fatal("fake stream selection")
	}
	m = key(m, tea.KeyMsg{Type: tea.KeyLeft}) // Closed stream -> parent title.
	if m.cursor != 0 || m.expanded["t6"] || !m.selected[6] {
		t.Fatal("parent collapse lost selection/focus")
	}
	m = key(m, tea.KeyMsg{Type: tea.KeyTab})
	if !m.expanded["t6"] {
		t.Fatal("Tab fold failed")
	}
	m = key(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	if !m.selected[42] || !m.selected[99] {
		t.Fatal("select all changed")
	}
	m = key(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	if m.selected[6] || m.selected[42] || m.selected[99] {
		t.Fatal("deselect all changed")
	}
	m = key(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.expanded["t6"] || m.phase != "titles" {
		t.Fatal("unselected title Enter did not fold")
	}
	m = key(m, tea.KeyMsg{Type: tea.KeySpace})
	m = key(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.phase != "output" || len(m.ids) != 1 || m.ids[0] != 6 {
		t.Fatal("existing Enter output shortcut changed")
	}
}

func TestExplorerResizeAndLongUnicode(t *testing.T) {
	m := explorerFixture(t)
	m.source = "iso:/media/" + strings.Repeat("日本語és", 80) + ".iso"
	m.titles[0].Attributes[2] = strings.Repeat("Magyar 日本語 é é", 80)
	m.titles[0].Streams[1][2] = strings.Repeat("Коментар 日本語 ", 100)
	for _, size := range [][2]int{{1, 1}, {8, 3}, {20, 6}, {40, 12}, {80, 24}, {120, 40}} {
		n, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = n.(Model)
		assertFits(t, m.View(), size[0], size[1])
		m.cursor = 2 // Audio track.
		view := m.View()
		assertFits(t, view, size[0], size[1])
		if size[0] >= 80 && (!strings.Contains(view, "AUDIO") || !strings.Contains(view, "Enter")) {
			t.Fatal("lost focus/instructions", view)
		}
	}
}

func TestSingleSourceEnterFromStreamPreservesOutputShortcut(t *testing.T) {
	m := explorerFixture(t)
	m = key(m, tea.KeyMsg{Type: tea.KeyDown})
	m = key(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.phase != "titles" || !m.expanded["s6/0"] {
		t.Fatal("Enter should fold without a title selection")
	}
	m.selected[6] = true
	m = key(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.phase != "output" || len(m.ids) != 1 || m.ids[0] != 6 {
		t.Fatal("existing stream-row Enter shortcut changed")
	}
}

func TestStructuredDetailsAndRawDiagnosticIsolation(t *testing.T) {
	m := explorerFixture(t)
	m.cursor = 2
	details := strings.Join(m.explorerDetails(m.rows()[m.cursor]), "\n")
	for _, field := range []string{"AUDIO", "Magyar (hun)", "Sample rate:", "48000", "Default", "Lossless conversion"} {
		if !strings.Contains(ansi.Strip(details), field) {
			t.Fatal("missing", field, details)
		}
	}
	if strings.Contains(m.View(), "SINFO:") || strings.Contains(m.View(), "Attribute [99]") {
		t.Fatal("raw protocol in explorer")
	}
	m.phase = "scan"
	if view := m.View(); strings.Contains(view, "SINFO:") || strings.Contains(view, "TINFO:") || strings.Contains(view, "FUTURE:") {
		t.Fatal("raw scan output leaked", view)
	}
	m = key(m, tea.KeyMsg{Type: tea.KeyCtrlL})
	if !strings.Contains(m.View(), "SINFO:") {
		t.Fatal("diagnostics lost raw records")
	}
	if got := ansi.Strip(normalLog(`ERROR · MSG:5001,0,1,"Failed to open %1","Failed to open %1","Movie, Disc"`)); got != "ERROR: Failed to open Movie, Disc" {
		t.Fatal(got)
	}
	if got := normalLog(`SINFO:6,1,39,0,"Default"`); got != "" {
		t.Fatal(got)
	}
}

func TestLargeExplorerScrollsToFocusedTitle(t *testing.T) {
	m := explorerFixture(t)
	m.titles = nil
	for i := 0; i < 500; i++ {
		title := &media.Title{ID: i * 2, Attributes: map[int]string{2: fmt.Sprintf("Title %d 日本語", i)}, Streams: map[int]map[int]string{}}
		for j := 0; j < 20; j++ {
			title.Streams[j] = map[int]string{1: "Audio", 3: "eng", 6: "AC3"}
		}
		m.titles = append(m.titles, title)
		m.expanded[fmt.Sprintf("t%d", title.ID)] = true
	}
	rows := m.rows()
	if len(rows) != 11000 {
		t.Fatal(len(rows))
	}
	m.cursor = len(rows) - 22 // Last title's parent row.
	m.width = 70
	m.height = 20
	view := m.View()
	assertFits(t, view, 70, 20)
	if !strings.Contains(view, "Title 998") {
		t.Fatal("focused title not visible", view)
	}
	m = key(m, tea.KeyMsg{Type: tea.KeyLeft})
	if r := m.rows()[m.cursor]; m.titles[r.title].ID != 998 || r.kind != "title" {
		t.Fatal("large fold lost focus", r)
	}
}

func TestBatchTreeKeepsSavedSelectionAndNextShortcut(t *testing.T) {
	m, f, store := batchModel(t)
	fixture := explorerFixture(t)
	if e := m.control.Metadata(0, fixture.disc); e != nil {
		t.Fatal(e)
	}
	m.installPicker(0)
	m, _ = bkey(m, tea.KeyMsg{Type: tea.KeySpace})
	m, _ = bkey(m, tea.KeyMsg{Type: tea.KeyDown})
	// Enter from a stream still means next ISO in batch planning.
	m, cmd := bkey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("batch Enter shortcut lost")
	}
	n, _ := m.Update(cmd())
	m = n.(BatchModel)
	if m.index != 1 || f.extracts != 0 {
		t.Fatal("planning navigation extracted")
	}
	m, _ = bkey(m, tea.KeyMsg{Type: tea.KeyPgUp})
	if !m.picker.selected[6] {
		t.Fatal("selection lost on back")
	}
	s, e := store.Load()
	if e != nil || !s.ISOs[0].Titles[0].Selected || s.ISOs[0].Titles[0].Metadata.Stream(12).Kind != media.Video {
		t.Fatal("selection/type metadata not persisted", e)
	}
	for _, size := range [][2]int{{30, 8}, {60, 18}, {110, 32}} {
		n, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = n.(BatchModel)
		assertFits(t, m.View(), size[0], size[1])
	}
	t.Setenv("NO_COLOR", "1")
	if strings.Contains(m.View(), "\x1b") {
		t.Fatal("batch NO_COLOR")
	}
}
