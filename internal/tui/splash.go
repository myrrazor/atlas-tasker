package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/myrrazor/atlas-tasker/internal/render"
	"github.com/myrrazor/atlas-tasker/internal/theme"
)

const (
	splashMinDelay    = 600 * time.Millisecond
	splashMinArtWidth = 54
)

// splashState gates the startup logo. The zero value is inactive, so every
// test that builds a model directly never sees it; only Run() turns it on.
type splashState struct {
	active       bool
	dataReady    bool
	minDelayDone bool
}

type splashMinDelayMsg struct{}

func splashMinDelayCmd() tea.Cmd {
	return tea.Tick(splashMinDelay, func(time.Time) tea.Msg { return splashMinDelayMsg{} })
}

func (s *splashState) maybeDismiss() {
	if s.active && s.dataReady && s.minDelayDone {
		s.active = false
	}
}

func (m model) splashView() string {
	useColor := renderEnabled()
	lines := make([]string, 0, 16)
	if m.width == 0 || m.width >= splashMinArtWidth {
		// art rows are never run through TruncateDisplay: its whitespace
		// normalization collapses the space runs the glyphs are made of
		lines = append(lines, renderSplashArt(splashArtAtlas, useColor)...)
		lines = append(lines, renderSplashArt(splashArtTasker, useColor)...)
	} else {
		// not enough columns for the logo; keep it dignified anyway
		title := "ATLAS TASKER"
		if useColor {
			title = lipgloss.NewStyle().Bold(true).Foreground(theme.Primary).Render(title)
		}
		lines = append(lines, title)
	}
	tagline := splashTagline
	hint := "loading workspace... any key to skip"
	if m.width > 0 {
		tagline = render.TruncateDisplay(tagline, m.width)
		hint = render.TruncateDisplay(hint, m.width)
	}
	if useColor {
		tagline = lipgloss.NewStyle().Foreground(theme.Accent).Render(tagline)
		hint = lipgloss.NewStyle().Foreground(theme.Muted).Render(hint)
	}
	lines = append(lines, "", tagline, hint)
	body := strings.Join(lines, "\n")
	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, body)
	}
	return body
}

// chrome shine, like the wordmark: ice on top fading to electric blue, with
// the box-drawing shadow glyphs dimmed to steel so they read as depth
// instead of outline noise.
func renderSplashArt(rows []string, useColor bool) []string {
	if !useColor {
		return rows
	}
	gradient := []lipgloss.TerminalColor{theme.Accent, theme.Accent, theme.Accent, theme.Primary, theme.Primary, theme.Primary}
	// barely-there navy: the shadow should suggest depth, not compete
	shadow := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#C9D6EA", Dark: "#1F3D6E"})
	out := make([]string, len(rows))
	for i, row := range rows {
		color := gradient[len(gradient)-1]
		if i < len(gradient) {
			color = gradient[i]
		}
		bright := lipgloss.NewStyle().Foreground(color)
		var b strings.Builder
		var run []rune
		runBright := false
		flush := func() {
			if len(run) == 0 {
				return
			}
			if runBright {
				b.WriteString(bright.Render(string(run)))
			} else {
				b.WriteString(shadow.Render(string(run)))
			}
			run = run[:0]
		}
		for _, r := range row {
			if r == ' ' {
				flush()
				b.WriteRune(' ')
				continue
			}
			isBlock := r == '█'
			if len(run) > 0 && isBlock != runBright {
				flush()
			}
			runBright = isBlock
			run = append(run, r)
		}
		flush()
		out[i] = b.String()
	}
	return out
}
