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
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/myrrazor/atlas-tasker/internal/config"
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
	screenInbox
	screenViews
	screenOps
)

var screenNames = []string{"Board", "Queues", "Detail", "Search", "Review", "Owner", "Inbox", "Views", "Ops"}

type dialogKind int

const (
	dialogNone dialogKind = iota
	dialogPalette
	dialogPrompt
	dialogForm
)

type dialogAction string

const (
	dialogCreate             dialogAction = "create"
	dialogEdit               dialogAction = "edit"
	dialogMove               dialogAction = "move"
	dialogAssign             dialogAction = "assign"
	dialogLink               dialogAction = "link"
	dialogUnlink             dialogAction = "unlink"
	dialogComment            dialogAction = "comment"
	dialogReject             dialogAction = "reject"
	dialogBulk               dialogAction = "bulk"
	dialogCollaboratorFilter dialogAction = "collaborator_filter"
)

type keyMap struct {
	Left          key.Binding
	Right         key.Binding
	Up            key.Binding
	Down          key.Binding
	Select        key.Binding
	Refresh       key.Binding
	Palette       key.Binding
	New           key.Binding
	Edit          key.Binding
	Move          key.Binding
	Assign        key.Binding
	Link          key.Binding
	Unlink        key.Binding
	Claim         key.Binding
	Comment       key.Binding
	RequestReview key.Binding
	Approve       key.Binding
	Reject        key.Binding
	Complete      key.Binding
	Filter        key.Binding
	BulkPreview   key.Binding
	BulkApply     key.Binding
	Cancel        key.Binding
	Quit          key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Left, k.Right, k.Up, k.Down, k.Select, k.Palette, k.Filter, k.BulkPreview, k.Refresh, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Left, k.Right, k.Up, k.Down, k.Select, k.Refresh, k.Quit, k.Cancel},
		{k.Palette, k.New, k.Edit, k.Move, k.Assign, k.Link, k.Unlink, k.Filter},
		{k.Claim, k.Comment, k.RequestReview, k.Approve, k.Reject, k.Complete, k.BulkPreview, k.BulkApply},
	}
}

type formField struct {
	Key         string
	Label       string
	Required    bool
	Placeholder string
	Input       textinput.Model
}

type dialogState struct {
	Kind     dialogKind
	Action   dialogAction
	Title    string
	Hint     string
	TicketID string
	Input    textinput.Model
	Fields   []formField
	Focus    int
}

func (d dialogState) active() bool {
	return d.Kind != dialogNone
}

type model struct {
	root               string
	actions            *service.ActionService
	queries            *service.QueryService
	projection         *sqlitestore.Store
	actor              contracts.Actor
	actorErr           string
	keys               keyMap
	help               help.Model
	screen             screen
	width              int
	height             int
	board              service.BoardViewModel
	queue              service.QueueView
	review             service.QueueView
	owner              service.QueueView
	runs               []contracts.RunSnapshot
	runDetail          service.RunDetailView
	runLaunch          service.RunLaunchManifestView
	agents             []service.AgentDetailView
	approvals          []service.ApprovalItemView
	operatorInbox      []service.InboxItemView
	dispatchQueue      service.DispatchQueueView
	worktrees          []service.WorktreeStatusView
	dashboard          service.DashboardSummaryView
	timeline           service.TimelineView
	inbox              []service.NotificationDelivery
	deadLetters        []service.NotificationDelivery
	savedViews         []contracts.SavedView
	automations        []contracts.AutomationRule
	automationExplain  []service.AutomationResult
	detail             service.TicketDetailView
	search             textinput.Model
	searchHits         []contracts.TicketSnapshot
	collaboratorFilter string
	selectedID         string
	selectedView       string
	cursor             int
	status             string
	dialog             dialogState
	lastBulk           *service.BulkOperationResult
	pendingBulk        *service.BulkOperation
}

type loadedMsg struct {
	board             service.BoardViewModel
	queue             service.QueueView
	review            service.QueueView
	owner             service.QueueView
	runs              []contracts.RunSnapshot
	runDetail         service.RunDetailView
	runLaunch         service.RunLaunchManifestView
	agents            []service.AgentDetailView
	approvals         []service.ApprovalItemView
	operatorInbox     []service.InboxItemView
	dispatchQueue     service.DispatchQueueView
	worktrees         []service.WorktreeStatusView
	dashboard         service.DashboardSummaryView
	timeline          service.TimelineView
	inbox             []service.NotificationDelivery
	deadLetters       []service.NotificationDelivery
	savedViews        []contracts.SavedView
	automations       []contracts.AutomationRule
	automationExplain []service.AutomationResult
	detail            service.TicketDetailView
	searchHits        []contracts.TicketSnapshot
	selectedID        string
	selectedView      string
	actor             contracts.Actor
	actorErr          string
	status            string
	switchScreen      *screen
	err               error
}

type detailMsg struct {
	detail service.TicketDetailView
	err    error
}

type bulkMsg struct {
	result  service.BulkOperationResult
	op      service.BulkOperation
	applied bool
	err     error
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
	clock := func() time.Time { return time.Now().UTC() }
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: clock}
	eventLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventLog)
	if err != nil {
		return model{}, err
	}
	projectStore := mdstore.ProjectStore{RootDir: root}
	cfg, err := config.Load(root)
	if err != nil {
		return model{}, err
	}
	locks := service.FileLockManager{Root: root}
	queries := service.NewQueryService(root, projectStore, ticketStore, eventLog, projection, clock)
	notifier, err := service.BuildNotifier(root, cfg, os.Stderr, service.SubscriptionResolver{
		Store:   service.SubscriptionStore{Root: root},
		Queries: queries,
	})
	if err != nil {
		return model{}, err
	}
	automation := &service.AutomationEngine{Store: service.AutomationStore{Root: root}, Notifier: notifier}
	actions := service.NewActionService(root, projectStore, ticketStore, eventLog, projection, clock, locks, notifier, automation)
	km := keyMap{
		Left:          key.NewBinding(key.WithKeys("left", "shift+tab"), key.WithHelp("←/shift+tab", "prev tab")),
		Right:         key.NewBinding(key.WithKeys("right", "tab"), key.WithHelp("→/tab", "next tab")),
		Up:            key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "move")),
		Down:          key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "move")),
		Select:        key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open/submit")),
		Refresh:       key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Palette:       key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "command palette")),
		New:           key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new ticket")),
		Edit:          key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit ticket")),
		Move:          key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "move ticket")),
		Assign:        key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "assign ticket")),
		Link:          key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "link ticket")),
		Unlink:        key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "unlink ticket")),
		Claim:         key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "claim/release")),
		Comment:       key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "comment")),
		RequestReview: key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "request review")),
		Approve:       key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "approve")),
		Reject:        key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "reject")),
		Complete:      key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "complete")),
		Filter:        key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "collab filter")),
		BulkPreview:   key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "bulk preview")),
		BulkApply:     key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "apply bulk")),
		Cancel:        key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel dialog")),
		Quit:          key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
	searchInput := textinput.New()
	searchInput.Prompt = "search> "
	searchInput.Placeholder = "status=ready, text~auth, label=bug"
	searchInput.CharLimit = 120
	searchInput.Width = 48
	return model{
		root:       root,
		actions:    actions,
		queries:    queries,
		projection: projection,
		actor:      explicitActor,
		keys:       km,
		help:       help.New(),
		search:     searchInput,
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
		if msg.board.Board.Columns != nil {
			m.board = msg.board
		}
		if msg.queue.Categories != nil {
			m.queue = msg.queue
		}
		if msg.review.Categories != nil {
			m.review = msg.review
		}
		if msg.owner.Categories != nil {
			m.owner = msg.owner
		}
		if msg.runs != nil {
			m.runs = msg.runs
		}
		if msg.runDetail.GeneratedAt != (time.Time{}) {
			m.runDetail = msg.runDetail
		}
		if msg.runLaunch.GeneratedAt != (time.Time{}) {
			m.runLaunch = msg.runLaunch
		}
		if msg.agents != nil {
			m.agents = msg.agents
		}
		if msg.approvals != nil {
			m.approvals = msg.approvals
		}
		if msg.operatorInbox != nil {
			m.operatorInbox = msg.operatorInbox
		}
		if msg.dispatchQueue.GeneratedAt != (time.Time{}) || msg.dispatchQueue.Entries != nil {
			m.dispatchQueue = msg.dispatchQueue
		}
		if msg.worktrees != nil {
			m.worktrees = msg.worktrees
		}
		if msg.dashboard.GeneratedAt != (time.Time{}) {
			m.dashboard = msg.dashboard
		}
		if msg.timeline.GeneratedAt != (time.Time{}) {
			m.timeline = msg.timeline
		}
		if msg.inbox != nil {
			m.inbox = msg.inbox
		}
		if msg.deadLetters != nil {
			m.deadLetters = msg.deadLetters
		}
		if msg.savedViews != nil {
			m.savedViews = msg.savedViews
		}
		if msg.automations != nil {
			m.automations = msg.automations
		}
		if msg.automationExplain != nil {
			m.automationExplain = msg.automationExplain
		}
		if msg.detail.Ticket.ID != "" {
			m.detail = msg.detail
		}
		if msg.searchHits != nil {
			m.searchHits = msg.searchHits
		}
		if msg.selectedID != "" {
			m.selectedID = msg.selectedID
			m.syncCursor()
		}
		if msg.selectedView != "" {
			m.selectedView = msg.selectedView
		}
		if msg.switchScreen != nil {
			m.screen = *msg.switchScreen
		}
		if msg.actor != "" {
			m.actor = msg.actor
		}
		m.actorErr = msg.actorErr
		if msg.status != "" {
			m.status = msg.status
		} else if m.actorErr != "" {
			m.status = m.actorErr
		} else {
			m.status = "synced"
		}
		return m, nil
	case detailMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.detail = msg.detail
		m.selectedID = msg.detail.Ticket.ID
		m.screen = screenDetail
		m.status = "detail synced"
		return m, nil
	case bulkMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		result := msg.result
		m.lastBulk = &result
		op := msg.op
		m.pendingBulk = &op
		if msg.applied {
			m.pendingBulk = nil
			m.status = fmt.Sprintf("bulk %s applied", result.Preview.Kind)
			return m, m.reload(m.selectedID, strings.TrimSpace(m.search.Value()), m.status)
		}
		m.screen = screenOps
		m.status = fmt.Sprintf("bulk %s previewed", result.Preview.Kind)
		return m, nil
	case tea.KeyMsg:
		if m.dialog.active() {
			return m.updateDialog(msg)
		}
		if m.screen == screenSearch {
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			if msg.String() != "enter" && cmd != nil {
				return m, cmd
			}
		}
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Left):
			m.screen = (m.screen + screen(len(screenNames)) - 1) % screen(len(screenNames))
			m.syncCursor()
			return m, nil
		case key.Matches(msg, m.keys.Right):
			m.screen = (m.screen + 1) % screen(len(screenNames))
			m.syncCursor()
			return m, nil
		case key.Matches(msg, m.keys.Up):
			return m.moveCursor(-1), nil
		case key.Matches(msg, m.keys.Down):
			return m.moveCursor(1), nil
		case key.Matches(msg, m.keys.Select):
			if m.screen == screenSearch {
				return m, m.searchCmd()
			}
			if m.screen == screenViews {
				selected := m.selectedViewName()
				if selected == "" {
					return m, nil
				}
				return m, m.loadSavedView(selected)
			}
			selected := m.selectedTicketID()
			if selected == "" {
				return m, nil
			}
			return m, m.loadDetail(selected)
		case key.Matches(msg, m.keys.Refresh):
			m.status = "refreshing…"
			return m, m.refresh()
		case key.Matches(msg, m.keys.Palette):
			m.dialog = newPaletteDialog(m.selectedTicketID())
			return m, nil
		case key.Matches(msg, m.keys.New):
			m.dialog = m.newCreateDialog()
			return m, nil
		case key.Matches(msg, m.keys.Edit):
			if ticket, ok := m.selectedTicket(); ok {
				m.dialog = newEditDialog(ticket)
			}
			return m, nil
		case key.Matches(msg, m.keys.Move):
			if id := m.selectedTicketID(); id != "" {
				m.dialog = newPromptDialog(dialogMove, id, "Move Ticket", "Enter backlog|ready|in_progress|in_review|blocked", "")
			}
			return m, nil
		case key.Matches(msg, m.keys.Assign):
			if ticket, ok := m.selectedTicket(); ok {
				m.dialog = newPromptDialog(dialogAssign, ticket.ID, "Assign Ticket", "agent:builder-1 or blank to clear", string(ticket.Assignee))
			}
			return m, nil
		case key.Matches(msg, m.keys.Link):
			if id := m.selectedTicketID(); id != "" {
				m.dialog = newPromptDialog(dialogLink, id, "Link Ticket", "blocks APP-2 | blocked-by APP-2 | parent APP-1", "")
			}
			return m, nil
		case key.Matches(msg, m.keys.Unlink):
			if id := m.selectedTicketID(); id != "" {
				m.dialog = newPromptDialog(dialogUnlink, id, "Unlink Ticket", "APP-2", "")
			}
			return m, nil
		case key.Matches(msg, m.keys.Claim):
			return m, m.toggleClaimSelected()
		case key.Matches(msg, m.keys.Comment):
			if id := m.selectedTicketID(); id != "" {
				m.dialog = newPromptDialog(dialogComment, id, "Comment", "Leave a short comment", "")
			}
			return m, nil
		case key.Matches(msg, m.keys.RequestReview):
			return m, m.requestReviewSelected()
		case key.Matches(msg, m.keys.Approve):
			return m, m.approveSelected()
		case key.Matches(msg, m.keys.Filter):
			m.dialog = newPromptDialog(dialogCollaboratorFilter, "", "Collaborator Filter", "rev-1 or blank to clear", m.collaboratorFilter)
			return m, nil
		case key.Matches(msg, m.keys.Reject):
			if id := m.selectedTicketID(); id != "" {
				m.dialog = newPromptDialog(dialogReject, id, "Reject Review", "Why is this going back?", "")
			}
			return m, nil
		case key.Matches(msg, m.keys.Complete):
			return m, m.completeSelected()
		case key.Matches(msg, m.keys.BulkPreview):
			if len(m.currentBulkTicketIDs()) == 0 {
				m.status = "no tickets in the current view to batch"
				return m, nil
			}
			m.dialog = newPromptDialog(dialogBulk, "", "Bulk Action", "move in_progress | assign agent:builder-1 | request-review | complete | claim | release", "")
			return m, nil
		case key.Matches(msg, m.keys.BulkApply):
			return m, m.applyPendingBulk()
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
	if m.dialog.active() {
		body = body + "\n\n" + m.dialogView()
	}
	footer := fmt.Sprintf("actor: %s | collaborator: %s | %s", optionalActor(m.actor, "unset"), optionalString(strings.TrimSpace(m.collaboratorFilter), "all"), m.status)
	if m.width > 0 {
		footer = render.TruncateDisplay(footer, m.width)
	}
	if width := m.width; width > 0 && width < lipgloss.Width(body) {
		body = lipgloss.NewStyle().Width(width).Render(body)
	}
	return strings.TrimSpace(header + "\n\n" + body + "\n\n" + footer + "\n" + m.help.View(m.keys))
}

func (m model) bodyView() string {
	switch m.screen {
	case screenBoard:
		return ticketsListView("Board", m.itemsForScreen(), m.cursor, m.width)
	case screenQueues:
		if m.actor == "" {
			return render.EmptyState("Queues", "Set --actor, TRACKER_ACTOR, or actor.default to populate queues.")
		}
		return ticketsListView("Queues", m.itemsForScreen(), m.cursor, m.width)
	case screenDetail:
		if m.detail.Ticket.ID == "" {
			return render.EmptyState("Detail", "No ticket selected yet.")
		}
		return detailWithOrchestration(m.detail, m.runs, m.runDetail, m.runLaunch, m.timeline, m.collaboratorFilter)
	case screenSearch:
		body := m.search.View() + "\n\n"
		if len(m.searchHits) == 0 {
			return body + render.EmptyState("Search", "Type a query and press enter.")
		}
		return body + ticketsListView("Search Results", m.searchHits, m.cursor, m.width)
	case screenReview:
		if m.actor == "" {
			return render.EmptyState("Review", "Set an actor to see review work.")
		}
		return ticketsListView("Review Inbox", m.itemsForScreen(), m.cursor, m.width)
	case screenOwner:
		return ticketsListView("Owner Attention", m.itemsForScreen(), m.cursor, m.width)
	case screenInbox:
		return attentionView(m.approvals, m.operatorInbox, m.inbox, m.deadLetters)
	case screenViews:
		return savedViewsPanel(m.savedViews, m.selectedViewName(), m.cursor)
	case screenOps:
		return opsView(m.dashboard, m.agents, m.dispatchQueue, m.worktrees, m.automations, m.automationExplain, m.lastBulk, m.pendingBulk, m.collaboratorFilter)
	default:
		return ""
	}
}

func (m model) refresh() tea.Cmd {
	return m.reload(m.selectedID, strings.TrimSpace(m.search.Value()), "synced")
}

func (m model) reload(selectedID string, searchQuery string, status string) tea.Cmd {
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
		runs := []contracts.RunSnapshot{}
		runDetail := service.RunDetailView{GeneratedAt: m.now()}
		runLaunch := service.RunLaunchManifestView{GeneratedAt: m.now()}
		agents := []service.AgentDetailView{}
		approvals := []service.ApprovalItemView{}
		operatorInbox := []service.InboxItemView{}
		dispatchQueue := service.DispatchQueueView{}
		worktrees := []service.WorktreeStatusView{}
		dashboard := service.DashboardSummaryView{GeneratedAt: m.now()}
		timeline := service.TimelineView{GeneratedAt: m.now()}
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
		searchHits := []contracts.TicketSnapshot{}
		if searchQuery != "" {
			parsed, err := contracts.ParseSearchQuery(searchQuery)
			if err != nil {
				return loadedMsg{err: err}
			}
			searchHits, err = m.queries.Search(ctx, parsed)
			if err != nil {
				return loadedMsg{err: err}
			}
		}
		detail := service.TicketDetailView{}
		runs, err = m.queries.ListRuns(ctx, "", "", "")
		if err != nil {
			return loadedMsg{err: err}
		}
		agents, err = m.queries.ListAgents(ctx)
		if err != nil {
			return loadedMsg{err: err}
		}
		approvals, err = m.queries.Approvals(ctx, m.collaboratorFilter)
		if err != nil {
			return loadedMsg{err: err}
		}
		operatorInbox, err = m.queries.Inbox(ctx, m.collaboratorFilter)
		if err != nil {
			return loadedMsg{err: err}
		}
		dispatchQueue, err = m.queries.DispatchQueue(ctx)
		if err != nil {
			return loadedMsg{err: err}
		}
		worktrees, err = m.queries.WorktreeList(ctx)
		if err != nil {
			return loadedMsg{err: err}
		}
		dashboard, err = m.queries.Dashboard(ctx, m.collaboratorFilter)
		if err != nil {
			return loadedMsg{err: err}
		}
		if selectedID == "" {
			selectedID = firstBoardTicketID(board)
		}
		if selectedID != "" {
			detail, err = m.queries.TicketDetail(ctx, selectedID)
			if err != nil {
				return loadedMsg{err: err}
			}
			timeline, err = m.queries.Timeline(ctx, selectedID, m.collaboratorFilter)
			if err != nil {
				return loadedMsg{err: err}
			}
			if focusRunID := focusRunIDForTicket(detail.Ticket, runs); focusRunID != "" {
				runDetail, err = m.queries.RunDetail(ctx, focusRunID)
				if err != nil {
					return loadedMsg{err: err}
				}
				runLaunch, err = m.queries.RunOpen(ctx, focusRunID)
				if err != nil {
					return loadedMsg{err: err}
				}
			}
		}
		inbox, err := m.queries.NotificationLog(12)
		if err != nil {
			return loadedMsg{err: err}
		}
		deadLetters, err := m.queries.DeadLetters(6)
		if err != nil {
			return loadedMsg{err: err}
		}
		savedViews, err := m.queries.ListSavedViews()
		if err != nil {
			return loadedMsg{err: err}
		}
		automations, err := m.queries.AutomationRules()
		if err != nil {
			return loadedMsg{err: err}
		}
		automationExplain := []service.AutomationResult{}
		if selectedID != "" {
			automationExplain, err = m.queries.ExplainAutomationRules(ctx, selectedID)
			if err != nil {
				return loadedMsg{err: err}
			}
		}
		selectedView := m.selectedView
		if selectedView == "" && len(savedViews) > 0 {
			selectedView = savedViews[0].Name
		}
		return loadedMsg{
			board:             board,
			queue:             queue,
			review:            review,
			owner:             owner,
			runs:              runs,
			runDetail:         runDetail,
			runLaunch:         runLaunch,
			agents:            agents,
			approvals:         approvals,
			operatorInbox:     operatorInbox,
			dispatchQueue:     dispatchQueue,
			worktrees:         worktrees,
			dashboard:         dashboard,
			timeline:          timeline,
			inbox:             inbox,
			deadLetters:       deadLetters,
			savedViews:        savedViews,
			automations:       automations,
			automationExplain: automationExplain,
			detail:            detail,
			searchHits:        searchHits,
			selectedID:        selectedID,
			selectedView:      selectedView,
			actor:             actor,
			actorErr:          actorErr,
			status:            status,
		}
	}
}

func (m model) searchCmd() tea.Cmd {
	query := strings.TrimSpace(m.search.Value())
	return m.reload(m.selectedID, query, "search synced")
}

func (m model) loadDetail(ticketID string) tea.Cmd {
	return m.reload(ticketID, strings.TrimSpace(m.search.Value()), "detail synced")
}

func (m model) loadSavedView(name string) tea.Cmd {
	return func() tea.Msg {
		result, err := m.queries.RunSavedView(context.Background(), name, m.actor)
		if err != nil {
			return loadedMsg{err: err}
		}
		msg := loadedMsg{selectedView: name, status: fmt.Sprintf("loaded view %s", name)}
		switch result.View.Kind {
		case contracts.SavedViewKindBoard:
			target := screenBoard
			msg.switchScreen = &target
			if result.Board != nil {
				msg.board = *result.Board
				msg.selectedID = firstBoardTicketID(*result.Board)
				msg.detail = service.TicketDetailView{}
			}
			msg.status = fmt.Sprintf("loaded board view %s", name)
		case contracts.SavedViewKindSearch:
			target := screenSearch
			msg.switchScreen = &target
			msg.searchHits = result.Tickets
			if len(result.Tickets) > 0 {
				msg.selectedID = result.Tickets[0].ID
			}
			msg.status = fmt.Sprintf("loaded search view %s", name)
		case contracts.SavedViewKindQueue:
			target := screenQueues
			msg.switchScreen = &target
			if result.Queue != nil {
				msg.queue = *result.Queue
				items := queueItems(*result.Queue)
				if len(items) > 0 {
					msg.selectedID = items[0].ID
				}
			}
			msg.actor = result.Actor
			msg.status = fmt.Sprintf("loaded queue view %s", name)
		case contracts.SavedViewKindNext:
			target := screenSearch
			msg.switchScreen = &target
			if result.Next != nil {
				msg.searchHits = nextTickets(*result.Next)
				if len(msg.searchHits) > 0 {
					msg.selectedID = msg.searchHits[0].ID
				}
			}
			msg.actor = result.Actor
			msg.status = fmt.Sprintf("loaded next view %s", name)
		}
		return msg
	}
}

func (m model) close() {
	if m.projection != nil {
		_ = m.projection.Close()
	}
}

func (m model) itemsForScreen() []contracts.TicketSnapshot {
	switch m.screen {
	case screenBoard:
		items := make([]contracts.TicketSnapshot, 0)
		for _, status := range []contracts.Status{contracts.StatusReady, contracts.StatusInProgress, contracts.StatusInReview, contracts.StatusBlocked, contracts.StatusBacklog, contracts.StatusDone} {
			items = append(items, m.board.Board.Columns[status]...)
		}
		return items
	case screenQueues:
		return queueItems(m.queue)
	case screenReview:
		return queueItems(m.review)
	case screenOwner:
		return queueItems(m.owner)
	case screenSearch:
		return m.searchHits
	default:
		return nil
	}
}

func (m *model) syncCursor() {
	if m.screen == screenViews {
		if len(m.savedViews) == 0 {
			m.cursor = 0
			m.selectedView = ""
			return
		}
		for idx, view := range m.savedViews {
			if view.Name == m.selectedView {
				m.cursor = idx
				return
			}
		}
		if m.cursor >= len(m.savedViews) {
			m.cursor = len(m.savedViews) - 1
		}
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.selectedView = m.savedViews[m.cursor].Name
		return
	}
	items := m.itemsForScreen()
	if len(items) == 0 {
		m.cursor = 0
		return
	}
	for idx, ticket := range items {
		if ticket.ID == m.selectedID {
			m.cursor = idx
			return
		}
	}
	if m.cursor >= len(items) {
		m.cursor = len(items) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.selectedID = items[m.cursor].ID
}

func (m model) moveCursor(delta int) model {
	if m.screen == screenViews {
		if len(m.savedViews) == 0 {
			return m
		}
		if m.cursor >= len(m.savedViews) {
			m.cursor = len(m.savedViews) - 1
		}
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.cursor += delta
		if m.cursor < 0 {
			m.cursor = 0
		}
		if m.cursor >= len(m.savedViews) {
			m.cursor = len(m.savedViews) - 1
		}
		m.selectedView = m.savedViews[m.cursor].Name
		return m
	}
	items := m.itemsForScreen()
	if len(items) == 0 {
		return m
	}
	if m.cursor >= len(items) {
		m.cursor = len(items) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(items) {
		m.cursor = len(items) - 1
	}
	m.selectedID = items[m.cursor].ID
	return m
}

func (m model) selectedTicketID() string {
	switch m.screen {
	case screenBoard, screenQueues, screenDetail, screenSearch, screenReview, screenOwner:
	default:
		return ""
	}
	if strings.TrimSpace(m.selectedID) != "" {
		return m.selectedID
	}
	items := m.itemsForScreen()
	if len(items) == 0 {
		return ""
	}
	if m.cursor >= len(items) {
		m.cursor = len(items) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.selectedID = items[m.cursor].ID
	return m.selectedID
}

func (m model) selectedViewName() string {
	if strings.TrimSpace(m.selectedView) != "" {
		return m.selectedView
	}
	if len(m.savedViews) == 0 {
		return ""
	}
	if m.cursor >= len(m.savedViews) {
		m.cursor = len(m.savedViews) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.selectedView = m.savedViews[m.cursor].Name
	return m.selectedView
}

func (m model) selectedTicket() (contracts.TicketSnapshot, bool) {
	id := m.selectedTicketID()
	if id == "" {
		return contracts.TicketSnapshot{}, false
	}
	if m.detail.Ticket.ID == id {
		return m.detail.Ticket, true
	}
	for _, ticket := range m.itemsForScreen() {
		if ticket.ID == id {
			return ticket, true
		}
	}
	return contracts.TicketSnapshot{}, false
}

func (m model) dialogView() string {
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1).Width(maxInt(48, minInt(m.width-4, 92)))
	if !renderEnabled() {
		style = style.Border(lipgloss.NormalBorder())
	}
	lines := []string{m.dialog.Title}
	if strings.TrimSpace(m.dialog.Hint) != "" {
		lines = append(lines, m.dialog.Hint)
	}
	switch m.dialog.Kind {
	case dialogPalette, dialogPrompt:
		lines = append(lines, "", m.dialog.Input.View())
	case dialogForm:
		lines = append(lines, "")
		for idx, field := range m.dialog.Fields {
			prefix := "  "
			if idx == m.dialog.Focus {
				prefix = "> "
			}
			lines = append(lines, fmt.Sprintf("%s%s", prefix, field.Label))
			lines = append(lines, field.Input.View())
		}
		lines = append(lines, "", "enter submits current field, tab moves focus")
	}
	return style.Render(strings.Join(lines, "\n"))
}

func (m model) updateDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Cancel):
		m.dialog = dialogState{}
		m.status = "dialog canceled"
		return m, nil
	}
	switch m.dialog.Kind {
	case dialogPalette, dialogPrompt:
		if key.Matches(msg, m.keys.Select) {
			if m.dialog.Action == dialogCollaboratorFilter {
				m.collaboratorFilter = strings.TrimSpace(m.dialog.Input.Value())
			}
			cmd := m.submitDialog()
			m.dialog = dialogState{}
			return m, cmd
		}
		var cmd tea.Cmd
		m.dialog.Input, cmd = m.dialog.Input.Update(msg)
		return m, cmd
	case dialogForm:
		if len(m.dialog.Fields) == 0 {
			m.dialog = dialogState{}
			return m, nil
		}
		switch msg.String() {
		case "tab":
			m.dialog.Focus = (m.dialog.Focus + 1) % len(m.dialog.Fields)
			m.focusDialogField()
			return m, nil
		case "shift+tab":
			m.dialog.Focus = (m.dialog.Focus + len(m.dialog.Fields) - 1) % len(m.dialog.Fields)
			m.focusDialogField()
			return m, nil
		case "enter":
			if m.dialog.Focus == len(m.dialog.Fields)-1 {
				cmd := m.submitDialog()
				m.dialog = dialogState{}
				return m, cmd
			}
			m.dialog.Focus++
			m.focusDialogField()
			return m, nil
		}
		var cmd tea.Cmd
		m.dialog.Fields[m.dialog.Focus].Input, cmd = m.dialog.Fields[m.dialog.Focus].Input.Update(msg)
		return m, cmd
	default:
		return m, nil
	}
}

func (m *model) focusDialogField() {
	for idx := range m.dialog.Fields {
		if idx == m.dialog.Focus {
			m.dialog.Fields[idx].Input.Focus()
		} else {
			m.dialog.Fields[idx].Input.Blur()
		}
	}
}

func (m model) submitDialog() tea.Cmd {
	switch m.dialog.Kind {
	case dialogPalette:
		return m.runSlashMutation(strings.TrimSpace(m.dialog.Input.Value()))
	case dialogPrompt:
		return m.runPromptMutation(m.dialog)
	case dialogForm:
		return m.runFormMutation(m.dialog)
	default:
		return nil
	}
}

func newPaletteDialog(selectedID string) dialogState {
	input := textinput.New()
	input.Prompt = "command> "
	if selectedID != "" {
		input.SetValue("/ticket ")
	}
	input.Focus()
	input.Width = 72
	return dialogState{Kind: dialogPalette, Title: "Command Palette", Hint: "Run a slash command against the shared service layer.", Input: input, TicketID: selectedID}
}

func newPromptDialog(action dialogAction, ticketID string, title string, placeholder string, value string) dialogState {
	input := textinput.New()
	input.Prompt = "> "
	input.Placeholder = placeholder
	input.SetValue(value)
	input.Focus()
	input.Width = 72
	return dialogState{Kind: dialogPrompt, Action: action, Title: title, TicketID: ticketID, Input: input}
}

func (m model) newCreateDialog() dialogState {
	project := ""
	if ticket, ok := m.selectedTicket(); ok {
		project = ticket.Project
	}
	fields := []formField{
		newFormField("project", "Project", true, "APP", project),
		newFormField("title", "Title", true, "Fix auth race", ""),
		newFormField("type", "Type", true, "task", "task"),
		newFormField("description", "Description", false, "What should this ticket do?", ""),
	}
	fields[0].Input.Focus()
	return dialogState{Kind: dialogForm, Action: dialogCreate, Title: "Create Ticket", Hint: "Minimal create form. Optional fields stay on the CLI for now.", Fields: fields}
}

func newEditDialog(ticket contracts.TicketSnapshot) dialogState {
	fields := []formField{
		newFormField("title", "Title", true, "Ticket title", ticket.Title),
		newFormField("description", "Description", false, "Description", ticket.Description),
	}
	fields[0].Input.Focus()
	return dialogState{Kind: dialogForm, Action: dialogEdit, Title: fmt.Sprintf("Edit %s", ticket.ID), Hint: "Edit the selected ticket.", TicketID: ticket.ID, Fields: fields}
}

func newFormField(keyName string, label string, required bool, placeholder string, value string) formField {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = placeholder
	input.SetValue(value)
	input.Width = 72
	return formField{Key: keyName, Label: label, Required: required, Placeholder: placeholder, Input: input}
}

func queueItems(queue service.QueueView) []contracts.TicketSnapshot {
	items := make([]contracts.TicketSnapshot, 0)
	for _, category := range []service.QueueCategory{
		service.QueueReadyForMe,
		service.QueueClaimedByMe,
		service.QueueNeedsReview,
		service.QueueAwaitingOwner,
		service.QueueBlockedForMe,
		service.QueueStaleClaims,
		service.QueuePolicyViolations,
	} {
		for _, entry := range queue.Categories[category] {
			items = append(items, entry.Ticket)
		}
	}
	return items
}

func nextTickets(next service.NextView) []contracts.TicketSnapshot {
	items := make([]contracts.TicketSnapshot, 0, len(next.Entries))
	for _, entry := range next.Entries {
		items = append(items, entry.Entry.Ticket)
	}
	return items
}

func detailWithOrchestration(detail service.TicketDetailView, runs []contracts.RunSnapshot, runDetail service.RunDetailView, launch service.RunLaunchManifestView, timeline service.TimelineView, collaboratorFilter string) string {
	body := render.TicketPretty(detail.Ticket, detail.Comments)
	gitLines := []string{"Git Context:"}
	if !detail.Git.Repo.Present {
		gitLines = append(gitLines, "- repo: not detected")
	} else {
		gitLines = append(gitLines,
			fmt.Sprintf("- branch: %s", optionalString(detail.Git.Repo.Branch, "detached")),
			fmt.Sprintf("- dirty: %t", detail.Git.Repo.Dirty),
			fmt.Sprintf("- suggested: %s", optionalString(detail.Git.SuggestedBranch, "n/a")),
			fmt.Sprintf("- current matches ticket: %t", detail.Git.CurrentBranchMatches),
		)
		if len(detail.Git.Refs) == 0 {
			gitLines = append(gitLines, "- refs: none")
		} else {
			gitLines = append(gitLines, "- refs:")
			for _, ref := range detail.Git.Refs {
				gitLines = append(gitLines, fmt.Sprintf("  - %s %s", shortHash(ref.Hash), ref.Subject))
			}
		}
	}
	runLines := []string{"Runs:"}
	ticketRuns := runsForTicket(detail.Ticket.ID, runs)
	if len(ticketRuns) == 0 {
		runLines = append(runLines, "- none")
	} else {
		for _, run := range ticketRuns {
			runLines = append(runLines, fmt.Sprintf("- %s [%s/%s] agent=%s", run.RunID, run.Status, run.Kind, optionalString(run.AgentID, "unassigned")))
		}
	}
	evidenceLines := []string{"Evidence:"}
	if len(runDetail.Evidence) == 0 {
		evidenceLines = append(evidenceLines, "- none")
	} else {
		for _, item := range runDetail.Evidence {
			evidenceLines = append(evidenceLines, fmt.Sprintf("- %s [%s] %s", item.EvidenceID, item.Type, optionalString(item.Title, "(untitled)")))
		}
	}
	handoffLines := []string{"Handoffs:"}
	if len(runDetail.Handoffs) == 0 {
		handoffLines = append(handoffLines, "- none")
	} else {
		for _, item := range runDetail.Handoffs {
			handoffLines = append(handoffLines, fmt.Sprintf("- %s next=%s gate=%s", item.HandoffID, optionalString(item.SuggestedNextActor, "n/a"), optionalString(string(item.SuggestedNextGate), "n/a")))
		}
	}
	runtimeLines := []string{"Runtime:"}
	if launch.RunID == "" {
		runtimeLines = append(runtimeLines, "- none")
	} else {
		runtimeLines = append(runtimeLines,
			"- dir: "+launch.RuntimeDir,
			"- brief: "+launch.BriefPath,
			"- context: "+launch.ContextPath,
			"- codex: "+launch.CodexLaunchPath,
			"- claude: "+launch.ClaudeLaunchPath,
		)
	}
	timelineLines := []string{"Timeline:"}
	if len(timeline.Entries) == 0 {
		timelineLines = append(timelineLines, "- none")
	} else {
		if strings.TrimSpace(collaboratorFilter) != "" {
			timelineLines = append(timelineLines, "- collaborator_filter: "+collaboratorFilter)
		}
		timelineLines = append(timelineLines,
			fmt.Sprintf("- change_ready: %s", timeline.ChangeReady),
			fmt.Sprintf("- open_gates: %s", optionalString(strings.Join(timeline.OpenGateIDs, ","), "none")),
		)
		start := len(timeline.Entries) - 5
		if start < 0 {
			start = 0
		}
		for _, entry := range timeline.Entries[start:] {
			timelineLines = append(timelineLines, fmt.Sprintf("- %s %s %s", entry.Timestamp.Format(time.RFC3339), entry.Type, entry.Summary))
		}
	}
	return body + "\n\n" + strings.Join(gitLines, "\n") + "\n\n" + strings.Join(runLines, "\n") + "\n\n" + strings.Join(evidenceLines, "\n") + "\n\n" + strings.Join(handoffLines, "\n") + "\n\n" + strings.Join(runtimeLines, "\n") + "\n\n" + strings.Join(timelineLines, "\n")
}

func attentionView(approvals []service.ApprovalItemView, items []service.InboxItemView, records []service.NotificationDelivery, deadLetters []service.NotificationDelivery) string {
	lines := []string{"Approvals:"}
	if len(approvals) == 0 {
		lines = append(lines, "- none")
	} else {
		for _, item := range approvals {
			lines = append(lines, fmt.Sprintf("- %s %s [%s] %s", item.Gate.GateID, render.GateBadge(item.Gate.State), item.Gate.Kind, item.Summary))
		}
	}
	lines = append(lines, "", "Human Inbox:")
	if len(items) == 0 {
		lines = append(lines, "- none")
	} else {
		for _, item := range items {
			lines = append(lines, fmt.Sprintf("- %s [%s] %s", item.ID, item.State, item.Summary))
		}
	}
	lines = append(lines, "", "Recent Deliveries:")
	if len(records) == 0 {
		lines = append(lines, "- none")
	} else {
		for _, record := range records {
			lines = append(lines, fmt.Sprintf("- %s %s %s via %s", record.Timestamp.Format(time.RFC3339), record.Event.Type, optionalString(record.Event.TicketID, record.Event.Project), record.Sink))
		}
	}
	lines = append(lines, "", "Dead Letters:")
	if len(deadLetters) == 0 {
		lines = append(lines, "- none")
	} else {
		for _, record := range deadLetters {
			lines = append(lines, fmt.Sprintf("- %s %s via %s (%s)", record.Timestamp.Format(time.RFC3339), record.Event.Type, record.Sink, record.Error))
		}
	}
	return strings.Join(lines, "\n")
}

func savedViewsPanel(views []contracts.SavedView, selected string, cursor int) string {
	if len(views) == 0 {
		return render.EmptyState("Views", "No saved views yet.")
	}
	lines := []string{"Saved Views:"}
	for idx, view := range views {
		prefix := "  "
		if idx == cursor {
			prefix = "> "
		}
		title := view.Title
		if strings.TrimSpace(title) == "" {
			title = view.Name
		}
		lines = append(lines, fmt.Sprintf("%s%s [%s] %s", prefix, view.Name, view.Kind, title))
	}
	if strings.TrimSpace(selected) != "" {
		lines = append(lines, "", fmt.Sprintf("enter runs %s into the matching tab", selected))
	}
	return strings.Join(lines, "\n")
}

func opsView(dashboard service.DashboardSummaryView, agents []service.AgentDetailView, dispatch service.DispatchQueueView, worktrees []service.WorktreeStatusView, rules []contracts.AutomationRule, explain []service.AutomationResult, lastBulk *service.BulkOperationResult, pendingBulk *service.BulkOperation, collaboratorFilter string) string {
	lines := []string{
		"Dashboard:",
		fmt.Sprintf("- collaborator_filter: %s", optionalString(strings.TrimSpace(collaboratorFilter), "all")),
		fmt.Sprintf("- active_runs: %d", dashboard.ActiveRuns),
		fmt.Sprintf("- awaiting_review: %d", dashboard.AwaitingReview.Count),
		fmt.Sprintf("- awaiting_owner: %d", dashboard.AwaitingOwner.Count),
		fmt.Sprintf("- merge_ready: %d", dashboard.MergeReady.Count),
		fmt.Sprintf("- blocked_by_checks: %d", dashboard.BlockedByChecks.Count),
		fmt.Sprintf("- stale_worktrees: %s", optionalString(strings.Join(dashboard.StaleWorktrees, ","), "none")),
		fmt.Sprintf("- retention_pressure: %s", optionalString(strings.Join(dashboard.RetentionTargets, ","), "none")),
		"",
		"Collaboration:",
		fmt.Sprintf("- mentions: %d", len(dashboard.MentionQueue)),
		fmt.Sprintf("- conflicts: %d", len(dashboard.ConflictQueue)),
		fmt.Sprintf("- failed_sync_jobs: %s", optionalString(strings.Join(dashboard.FailedSyncJobs, ","), "none")),
		"",
		"Collaborator Workload:",
	}
	if len(dashboard.CollaboratorWorkload) == 0 {
		lines = append(lines, "- none")
	} else {
		for _, item := range dashboard.CollaboratorWorkload {
			lines = append(lines, fmt.Sprintf("- %s approvals=%d inbox=%d mentions=%d handoffs=%d", item.CollaboratorID, item.Approvals, item.InboxItems, item.Mentions, item.Handoffs))
		}
	}
	lines = append(lines, "", "Remote Health:")
	if len(dashboard.RemoteHealth) == 0 {
		lines = append(lines, "- none")
	} else {
		for _, item := range dashboard.RemoteHealth {
			lines = append(lines, fmt.Sprintf("- %s %s publications=%d failed=%d", item.RemoteID, render.SyncBadge(item.State), item.PublicationCount, item.FailedJobs))
		}
	}
	lines = append(lines, "", "Conflict Queue:")
	if len(dashboard.ConflictQueue) == 0 {
		lines = append(lines, "- none")
	} else {
		for _, item := range dashboard.ConflictQueue {
			lines = append(lines, fmt.Sprintf("- %s [%s] %s", item.ConflictID, item.EntityKind, item.ConflictType))
		}
	}
	lines = append(lines, "", "Mention Queue:")
	if len(dashboard.MentionQueue) == 0 {
		lines = append(lines, "- none")
	} else {
		for _, item := range dashboard.MentionQueue {
			lines = append(lines, fmt.Sprintf("- %s @%s %s", item.MentionUID, item.CollaboratorID, item.Summary))
		}
	}
	lines = append(lines, "", "Provider Mapping Warnings:")
	if len(dashboard.ProviderMappingWarnings) == 0 {
		lines = append(lines, "- none")
	} else {
		for _, warning := range dashboard.ProviderMappingWarnings {
			lines = append(lines, "- "+warning)
		}
	}
	lines = append(lines, "",
		"Agents:",
	)
	if len(agents) == 0 {
		lines = append(lines, "- none")
	} else {
		for _, agent := range agents {
			state := "disabled"
			if agent.Profile.Enabled {
				state = "enabled"
			}
			lines = append(lines, fmt.Sprintf("- %s [%s] active=%d", agent.Profile.AgentID, state, agent.ActiveRuns))
		}
	}
	lines = append(lines, "", "Dispatch Queue:")
	if len(dispatch.Entries) == 0 {
		lines = append(lines, "- none")
	} else {
		for _, entry := range dispatch.Entries {
			auto := optionalString(entry.Suggestion.AutoRouteAgentID, "manual")
			lines = append(lines, fmt.Sprintf("- %s auto=%s", entry.Ticket.ID, auto))
		}
	}
	lines = append(lines, "", "Worktrees:")
	if len(worktrees) == 0 {
		lines = append(lines, "- none")
	} else {
		for _, item := range worktrees {
			lines = append(lines, fmt.Sprintf("- %s present=%t dirty=%t", item.RunID, item.Present, item.Dirty))
		}
	}
	lines = append(lines, "", "Automation Rules:")
	if len(rules) == 0 {
		lines = append(lines, "- none")
	} else {
		for _, rule := range rules {
			state := "disabled"
			if rule.Enabled {
				state = "enabled"
			}
			lines = append(lines, fmt.Sprintf("- %s [%s]", rule.Name, state))
		}
	}
	lines = append(lines, "", "Automation Explain:")
	if len(explain) == 0 {
		lines = append(lines, "- select a ticket to inspect rule matches")
	} else {
		for _, result := range explain {
			state := "skip"
			if result.Matched {
				state = "match"
			}
			lines = append(lines, fmt.Sprintf("- %s [%s] %s", result.Rule.Name, state, strings.Join(result.Actions, ", ")))
		}
	}
	lines = append(lines, "", "Bulk Preview:")
	switch {
	case lastBulk != nil:
		lines = append(lines,
			fmt.Sprintf("- last batch: %s", lastBulk.BatchID),
			fmt.Sprintf("- kind: %s", lastBulk.Preview.Kind),
			fmt.Sprintf("- total: %d ok=%d failed=%d skipped=%d", lastBulk.Summary.Total, lastBulk.Summary.Succeeded, lastBulk.Summary.Failed, lastBulk.Summary.Skipped),
		)
	case pendingBulk != nil:
		lines = append(lines, fmt.Sprintf("- pending %s on %d tickets", pendingBulk.Kind, len(pendingBulk.TicketIDs)))
	default:
		lines = append(lines, "- press b to preview a bulk action for the current ticket list")
	}
	lines = append(lines, "- press y to apply the last preview")
	return strings.Join(lines, "\n")
}

func ticketsListView(title string, tickets []contracts.TicketSnapshot, cursor int, widths ...int) string {
	if len(tickets) == 0 {
		return render.EmptyState(title, "No items in this view yet.")
	}
	width := 88
	if len(widths) > 0 && widths[0] > 0 {
		width = widths[0]
	}
	lines := []string{title + ":"}
	for idx, ticket := range tickets {
		prefix := "  "
		if idx == cursor {
			prefix = "> "
		}
		lines = append(lines, prefix+render.TicketSummary(ticket, width-lipgloss.Width(prefix)))
	}
	return strings.Join(lines, "\n")
}

func optionalString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func shortHash(hash string) string {
	hash = strings.TrimSpace(hash)
	if len(hash) <= 8 {
		return hash
	}
	return hash[:8]
}

func optionalActor(actor contracts.Actor, fallback string) string {
	if strings.TrimSpace(string(actor)) == "" {
		return fallback
	}
	return string(actor)
}

func renderEnabled() bool {
	return render.ColorEnabled()
}

func firstBoardTicketID(board service.BoardViewModel) string {
	for _, status := range []contracts.Status{contracts.StatusReady, contracts.StatusInProgress, contracts.StatusInReview, contracts.StatusBlocked, contracts.StatusBacklog, contracts.StatusDone} {
		tickets := board.Board.Columns[status]
		if len(tickets) > 0 {
			return tickets[0].ID
		}
	}
	return ""
}

func focusRunIDForTicket(ticket contracts.TicketSnapshot, runs []contracts.RunSnapshot) string {
	if strings.TrimSpace(ticket.LatestRunID) != "" {
		return ticket.LatestRunID
	}
	items := runsForTicket(ticket.ID, runs)
	if len(items) == 0 {
		return ""
	}
	return items[0].RunID
}

func runsForTicket(ticketID string, runs []contracts.RunSnapshot) []contracts.RunSnapshot {
	items := make([]contracts.RunSnapshot, 0)
	for _, run := range runs {
		if run.TicketID == ticketID {
			items = append(items, run)
		}
	}
	return items
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
