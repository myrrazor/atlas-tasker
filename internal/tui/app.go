package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/render"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

type screen int

const (
	screenBoard screen = iota
	screenQueues
	screenDetail
	screenSearch
	screenReview
	screenOwner
)

var screenNames = []string{"Board", "Queues", "Detail", "Search", "Review", "Owner"}

type keyMap struct {
	Left    key.Binding
	Right   key.Binding
	Refresh key.Binding
	Quit    key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Left, k.Right, k.Refresh, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Left, k.Right, k.Refresh, k.Quit}}
}

type model struct {
	root       string
	queries    *service.QueryService
	projection *sqlitestore.Store
	actor      contracts.Actor
	actorErr   string
	keys       keyMap
	help       help.Model
	screen     screen
	width      int
	height     int
	board      service.BoardViewModel
	queue      service.QueueView
	review     service.QueueView
	owner      service.QueueView
	detail     service.TicketDetailView
	status     string
}

type loadedMsg struct {
	board    service.BoardViewModel
	queue    service.QueueView
	review   service.QueueView
	owner    service.QueueView
	detail   service.TicketDetailView
	actor    contracts.Actor
	actorErr string
	err      error
}

func Run(root string, explicitActor contracts.Actor) error {
	m, err := newModel(root, explicitActor)
	if err != nil {
		return err
	}
	defer m.close()
	program := tea.NewProgram(m, tea.WithAltScreen())
	_, err = program.Run()
	return err
}

func newModel(root string, explicitActor contracts.Actor) (model, error) {
	ticketStore := mdstore.TicketStore{RootDir: root}
	eventLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventLog)
	if err != nil {
		return model{}, err
	}
	projectStore := mdstore.ProjectStore{RootDir: root}
	queries := service.NewQueryService(root, projectStore, ticketStore, eventLog, projection, time.Now)
	km := keyMap{
		Left:    key.NewBinding(key.WithKeys("left", "shift+tab"), key.WithHelp("←/shift+tab", "prev tab")),
		Right:   key.NewBinding(key.WithKeys("right", "tab"), key.WithHelp("→/tab", "next tab")),
		Refresh: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
	return model{
		root:       root,
		queries:    queries,
		projection: projection,
		actor:      explicitActor,
		keys:       km,
		help:       help.New(),
		status:     "loading…",
	}, nil
}

func (m model) Init() tea.Cmd {
	return m.refresh()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case loadedMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.board = msg.board
		m.queue = msg.queue
		m.review = msg.review
		m.owner = msg.owner
		m.detail = msg.detail
		m.actor = msg.actor
		m.actorErr = msg.actorErr
		if m.actorErr != "" {
			m.status = m.actorErr
		} else {
			m.status = "synced"
		}
		return m, nil
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Left):
			m.screen = (m.screen + screen(len(screenNames)) - 1) % screen(len(screenNames))
			return m, nil
		case key.Matches(msg, m.keys.Right):
			m.screen = (m.screen + 1) % screen(len(screenNames))
			return m, nil
		case key.Matches(msg, m.keys.Refresh):
			m.status = "refreshing…"
			return m, m.refresh()
		}
	}
	return m, nil
}

func (m model) View() string {
	tabStyle := lipgloss.NewStyle().Padding(0, 1)
	activeStyle := tabStyle.Bold(true)
	if renderEnabled() {
		activeStyle = activeStyle.Foreground(lipgloss.Color("10"))
	}
	tabs := make([]string, 0, len(screenNames))
	for idx, label := range screenNames {
		if int(m.screen) == idx {
			tabs = append(tabs, activeStyle.Render(label))
			continue
		}
		tabs = append(tabs, tabStyle.Render(label))
	}
	header := strings.Join(tabs, " ")
	body := m.bodyView()
	footer := fmt.Sprintf("actor: %s | %s", optionalActor(m.actor, "unset"), m.status)
	if width := m.width; width > 0 && width < lipgloss.Width(body) {
		body = lipgloss.NewStyle().Width(width).Render(body)
	}
	return strings.TrimSpace(header + "\n\n" + body + "\n\n" + footer + "\n" + m.help.View(m.keys))
}

func (m model) bodyView() string {
	switch m.screen {
	case screenBoard:
		return render.BoardPretty(m.board.Board)
	case screenQueues:
		if m.actor == "" {
			return render.EmptyState("Queues", "Set --actor, TRACKER_ACTOR, or actor.default to populate queue tabs.")
		}
		return queuePretty(m.queue)
	case screenDetail:
		if m.detail.Ticket.ID == "" {
			return render.EmptyState("Detail", "No ticket selected yet. The foundation TUI will pick this up in PR-207.")
		}
		return render.TicketPretty(m.detail.Ticket, m.detail.Comments)
	case screenSearch:
		return render.EmptyState("Search", "Search/list screen lands in PR-207.")
	case screenReview:
		if m.actor == "" {
			return render.EmptyState("Review", "Set an actor to see review work.")
		}
		return queuePretty(m.review)
	case screenOwner:
		return queuePretty(m.owner)
	default:
		return ""
	}
}

func (m model) refresh() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		actor := m.actor
		actorErr := ""
		if actor == "" {
			resolved, err := m.queries.ResolveActor(ctx, "")
			if err != nil {
				actorErr = "queue context is unset; board-only mode is available"
			} else {
				actor = resolved
			}
		}
		board, err := m.queries.Board(ctx, contracts.BoardQueryOptions{})
		if err != nil {
			return loadedMsg{err: err}
		}
		queue := service.QueueView{}
		review := service.QueueView{}
		owner, err := m.queries.Queue(ctx, contracts.Actor("human:owner"))
		if err != nil {
			return loadedMsg{err: err}
		}
		if actor != "" {
			queue, err = m.queries.Queue(ctx, actor)
			if err != nil {
				return loadedMsg{err: err}
			}
			review = service.QueueView{Actor: actor, GeneratedAt: queue.GeneratedAt, Categories: map[service.QueueCategory][]service.QueueEntry{
				service.QueueNeedsReview: queue.Categories[service.QueueNeedsReview],
			}}
		}
		detail := service.TicketDetailView{}
		for _, status := range []contracts.Status{contracts.StatusReady, contracts.StatusInProgress, contracts.StatusInReview, contracts.StatusBlocked, contracts.StatusBacklog, contracts.StatusDone} {
			tickets := board.Board.Columns[status]
			if len(tickets) == 0 {
				continue
			}
			detail, err = m.queries.TicketDetail(ctx, tickets[0].ID)
			if err != nil {
				return loadedMsg{err: err}
			}
			break
		}
		return loadedMsg{board: board, queue: queue, review: review, owner: owner, detail: detail, actor: actor, actorErr: actorErr}
	}
}

func (m model) close() {
	if m.projection != nil {
		_ = m.projection.Close()
	}
}

func queuePretty(queue service.QueueView) string {
	if len(queue.Categories) == 0 {
		return render.EmptyState("Queues", "No queue entries yet.")
	}
	lines := []string{}
	for _, category := range []service.QueueCategory{
		service.QueueReadyForMe,
		service.QueueClaimedByMe,
		service.QueueNeedsReview,
		service.QueueAwaitingOwner,
		service.QueueBlockedForMe,
		service.QueueStaleClaims,
		service.QueuePolicyViolations,
	} {
		entries := queue.Categories[category]
		if len(entries) == 0 {
			continue
		}
		lines = append(lines, string(category)+":")
		for _, entry := range entries {
			lines = append(lines, fmt.Sprintf("- %s %s (%s)", entry.Ticket.ID, entry.Ticket.Title, entry.Reason))
		}
		lines = append(lines, "")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func optionalActor(actor contracts.Actor, fallback string) string {
	if strings.TrimSpace(string(actor)) == "" {
		return fallback
	}
	return string(actor)
}

func renderEnabled() bool {
	return strings.TrimSpace(os.Getenv("NO_COLOR")) == ""
}
