package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func splashTestModel() model {
	return model{
		splash: splashState{active: true},
		keys: keyMap{
			Quit: key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		},
		width:  100,
		height: 32,
	}
}

func TestSplashBlocksViewUntilDataAndMinDelay(t *testing.T) {
	m := splashTestModel()
	if view := m.View(); !strings.Contains(view, splashTagline) {
		t.Fatalf("expected splash view, got: %q", view)
	}
	next, _ := m.Update(loadedMsg{})
	m = next.(model)
	if !m.splash.active {
		t.Fatal("splash should persist until the min delay elapses")
	}
	next, _ = m.Update(splashMinDelayMsg{})
	m = next.(model)
	if m.splash.active {
		t.Fatal("expected dismissal once data and delay are both in")
	}
}

func TestSplashAnyKeySkipsButQuitQuits(t *testing.T) {
	m := splashTestModel()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if next.(model).splash.active {
		t.Fatal("any key should skip the splash")
	}
	if cmd != nil {
		t.Fatal("the skipping key must be swallowed")
	}

	q := splashTestModel()
	_, qcmd := q.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if qcmd == nil {
		t.Fatal("quit must pass through the splash")
	}
	if _, ok := qcmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected quit command, got %T", qcmd())
	}
}

func TestSplashCompactAndTinyWidths(t *testing.T) {
	m := splashTestModel()
	m.width = 30
	view := m.View()
	if strings.Contains(view, "___") {
		t.Fatalf("no block art at tiny width, got: %q", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > 30 {
			t.Fatalf("splash line exceeds tiny width: %q", line)
		}
	}
	m.width = 60
	if view := m.View(); !strings.Contains(view, "___") {
		t.Fatalf("expected block art at width 60, got: %q", view)
	}
}

func TestSplashNoANSIWhenNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := splashTestModel()
	if view := m.View(); strings.Contains(view, "\x1b[") {
		t.Fatal("splash must be escape-free under NO_COLOR")
	}
}

func TestSplashDismissesOnLoadError(t *testing.T) {
	m := splashTestModel()
	m.splash.minDelayDone = true
	next, _ := m.Update(loadedMsg{err: errors.New("boom")})
	got := next.(model)
	if got.splash.active {
		t.Fatal("a load error must not strand the splash")
	}
	if !strings.Contains(got.status, "boom") {
		t.Fatalf("error should land in status, got %q", got.status)
	}
}
