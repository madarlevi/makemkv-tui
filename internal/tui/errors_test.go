package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/madarlevi/makemkv-tui/internal/batchstate"
	"strings"
	"testing"
	"time"
)

func TestFullFilesystemErrorIsExpandableAndScrollable(t *testing.T) {
	m, _, _ := batchModel(t)
	message := "rename /mnt/nas/" + strings.Repeat("long folder 季節/", 12) + "title_t00.mkv -> /mnt/nas/done/title0.mkv: input/output error END_OF_ERROR"
	m.snapshot.State.Errors = []batchstate.Failure{{At: time.Now(), Message: message}}
	m.width = 40
	m.height = 10
	m, _ = bkey(m, tea.KeyMsg{Type: tea.KeyCtrlE})
	if !m.showErrors {
		t.Fatal("error display did not expand")
	}
	all := ""
	for i := 0; i < 70; i++ {
		view := m.View()
		assertFits(t, view, 40, 10)
		all += strings.ReplaceAll(ansi.Strip(view), "\n", "")
		m, _ = bkey(m, tea.KeyMsg{Type: tea.KeyDown})
	}
	if !strings.Contains(all, "END_OF_ERROR") || !strings.Contains(all, "input/output error") {
		t.Fatal("error tail was truncated")
	}
	m, _ = bkey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.showErrors || m.stopping {
		t.Fatal("closing errors stopped the batch")
	}
	model := explorerFixture(t)
	model.err = message
	model = key(model, tea.KeyMsg{Type: tea.KeyCtrlE})
	if !model.showErrors {
		t.Fatal("single-source error view unavailable")
	}
	model = key(model, tea.KeyMsg{Type: tea.KeyCtrlC})
	if model.ctx.Err() == nil {
		t.Fatal("error view swallowed immediate stop")
	}
}

func TestErrorViewMonochromeAndSmallTerminal(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	for _, size := range [][2]int{{1, 1}, {20, 5}, {70, 20}} {
		view := errorView([]string{"publish: " + strings.Repeat("path 季節/", 20) + ": input/output error"}, 8, size[0], size[1])
		assertFits(t, view, size[0], size[1])
		if strings.Contains(view, "\x1b") {
			t.Fatal("NO_COLOR ignored")
		}
	}
}
