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
)

var screenNames = []string{"Board", "Queues", "Detail", "Search", "Review", "Owner"}

type dialogKind int

const (
	dialogNone dialogKind = iota
	dialogPalette
	dialogPrompt
	dialogForm
)

type dialogAction string

const (
	dialogCreate  dialogAction = "create"
	dialogEdit    dialogAction = "edit"
	dialogMove    dialogAction = "move"
	dialogAssign  dialogAction = "assign"
	dialogLink    dialogAction = "link"
	dialogUnlink  dialogAction = "unlink"
	dialogComment dialogAction = "comment"
	dialogReject  dialogAction = "reject"
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
	Cancel        key.Binding
	Quit          key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Left, k.Right, k.Up, k.Down, k.Select, k.Palette, k.New, k.Claim, k.RequestReview, k.Refresh, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Left, k.Right, k.Up, k.Down, k.Select, k.Refresh, k.Quit, k.Cancel},
		{k.Palette, k.New, k.Edit, k.Move, k.Assign, k.Link, k.Unlink},
		{k.Claim, k.Comment, k.RequestReview, k.Approve, k.Reject, k.Complete},
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
	root       string
	actions    *service.ActionService
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
	search     textinput.Model
	searchHits []contracts.TicketSnapshot
	selectedID string
	cursor     int
	status     string
	dialog     dialogState
}

type loadedMsg struct {
	board      service.BoardViewModel
	queue      service.QueueView
	review     service.QueueView
	owner      service.QueueView
	detail     service.TicketDetailView
	searchHits []contracts.TicketSnapshot
	selectedID string
	actor      contracts.Actor
	actorErr   string
	status     string
	err        error
}

type detailMsg struct {
	detail service.TicketDetailView
	err    error
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
	cfg, err := config.Load(root)
	if err != nil {
		return model{}, err
	}
	notifier, err := service.BuildNotifier(root, cfg, os.Stderr)
	if err != nil {
		return model{}, err
	}
	actions := service.NewActionService(root, projectStore, ticketStore, eventLog, projection, time.Now, notifier)
	queries := service.NewQueryService(root, projectStore, ticketStore, eventLog, projection, time.Now)
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
		case key.Matches(msg, m.keys.Reject):
			if id := m.selectedTicketID(); id != "" {
				m.dialog = newPromptDialog(dialogReject, id, "Reject Review", "Why is this going back?", "")
			}
			return m, nil
		case key.Matches(msg, m.keys.Complete):
			return m, m.completeSelected()
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
	footer := fmt.Sprintf("actor: %s | %s", optionalActor(m.actor, "unset"), m.status)
	if width := m.width; width > 0 && width < lipgloss.Width(body) {
		body = lipgloss.NewStyle().Width(width).Render(body)
	}
	return strings.TrimSpace(header + "\n\n" + body + "\n\n" + footer + "\n" + m.help.View(m.keys))
}

func (m model) bodyView() string {
	switch m.screen {
	case screenBoard:
		return ticketsListView("Board", m.itemsForScreen(), m.cursor)
	case screenQueues:
		if m.actor == "" {
			return render.EmptyState("Queues", "Set --actor, TRACKER_ACTOR, or actor.default to populate queue tabs.")
		}
		return ticketsListView("Queues", m.itemsForScreen(), m.cursor)
	case screenDetail:
		if m.detail.Ticket.ID == "" {
			return render.EmptyState("Detail", "No ticket selected yet.")
		}
		return render.TicketPretty(m.detail.Ticket, m.detail.Comments)
	case screenSearch:
		body := m.search.View() + "\n\n"
		if len(m.searchHits) == 0 {
			return body + render.EmptyState("Search", "Type a query and press enter.")
		}
		return body + ticketsListView("Search Results", m.searchHits, m.cursor)
	case screenReview:
		if m.actor == "" {
			return render.EmptyState("Review", "Set an actor to see review work.")
		}
		return ticketsListView("Review Inbox", m.itemsForScreen(), m.cursor)
	case screenOwner:
		return ticketsListView("Owner Attention", m.itemsForScreen(), m.cursor)
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
		if selectedID == "" {
			selectedID = firstBoardTicketID(board)
		}
		if selectedID != "" {
			detail, err = m.queries.TicketDetail(ctx, selectedID)
			if err != nil {
				return loadedMsg{err: err}
			}
		}
		return loadedMsg{board: board, queue: queue, review: review, owner: owner, detail: detail, searchHits: searchHits, selectedID: selectedID, actor: actor, actorErr: actorErr, status: status}
	}
}

func (m model) searchCmd() tea.Cmd {
	query := strings.TrimSpace(m.search.Value())
	return m.reload(m.selectedID, query, "search synced")
}

func (m model) loadDetail(ticketID string) tea.Cmd {
	return func() tea.Msg {
		detail, err := m.queries.TicketDetail(context.Background(), ticketID)
		return detailMsg{detail: detail, err: err}
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

func ticketsListView(title string, tickets []contracts.TicketSnapshot, cursor int) string {
	if len(tickets) == 0 {
		return render.EmptyState(title, "No items in this view yet.")
	}
	lines := []string{title + ":"}
	for idx, ticket := range tickets {
		prefix := "  "
		if idx == cursor {
			prefix = "> "
		}
		lines = append(lines, fmt.Sprintf("%s%s [%s/%s] %s", prefix, ticket.ID, ticket.Status, ticket.Priority, ticket.Title))
	}
	return strings.Join(lines, "\n")
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

func firstBoardTicketID(board service.BoardViewModel) string {
	for _, status := range []contracts.Status{contracts.StatusReady, contracts.StatusInProgress, contracts.StatusInReview, contracts.StatusBlocked, contracts.StatusBacklog, contracts.StatusDone} {
		tickets := board.Board.Columns[status]
		if len(tickets) > 0 {
			return tickets[0].ID
		}
	}
	return ""
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
