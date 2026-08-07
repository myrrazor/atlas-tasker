package web

import (
	"context"
	"net/http"
	"net/url"
	"os/user"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

type BoardPage struct {
	Page         string
	Workspace    string
	Host         string
	Actor        contracts.Actor
	ReadOnly     bool
	Project      string
	LocationName string
	// true when the project came from the request, not the server default —
	// saved views must not be narrowed by an implicit --project
	ProjectExplicit bool `json:"-"`
	View            string
	Query           string
	Assignee        string
	Reviewer        string
	Label           string
	Priority        string
	Type            string
	ActiveColumn    contracts.Status
	// rendered into the page for forms/fetch; never exposed via /api/board
	CSRFToken string `json:"-"`
	// submitted values of a rejected form, echoed back so typed content
	// survives server-side validation errors; FormTarget names the one form
	// ("create", "edit", "comment") allowed to consume them
	Form       url.Values `json:"-"`
	FormTarget string     `json:"-"`
	Columns    []BoardColumn
	Detail     *TicketDetail
	Flash      string
	Error      string
	ShowNew    bool
}

type WelcomePage struct {
	Page      string
	Workspace string
	Host      string
	Actor     contracts.Actor
	OwnerName string
	Projects  []ProjectRow
	Recent    []RecentChange
	CSRFToken string
	ReadOnly  bool
	Error     string
	Flash     string
	ShowNew   bool
	Form      url.Values
}

type ProjectRow struct {
	Project contracts.Project
	Active  int
	Backlog int
	Done    int
	Blocked int
}

type RecentChange struct {
	Project  string
	TicketID string
	Verb     string
	From     string
	To       string
	At       time.Time
}

// Describe turns a recent event into the short sentence used by the feed.
func (c RecentChange) Describe() string {
	if c.Verb == "moved" && c.From != "" && c.To != "" {
		return c.TicketID + " moved " + c.From + " → " + c.To
	}
	return strings.TrimSpace(c.TicketID + " " + c.Verb)
}

type SettingsPage struct {
	Page         string
	Workspace    string
	Host         string
	Actor        contracts.Actor
	OwnerName    string
	ActorDefault contracts.Actor
	Language     string
	AgentColors  []AgentColorSetting
	CSRFToken    string
	ReadOnly     bool
	Error        string
}

type AgentColorSetting struct {
	Agent string
	Color string
	Class string
}

var currentOSUser = user.Current

// FormFor hands the rejected form values to exactly the form that was
// submitted — echoing them anywhere else prefills unrelated forms (worst
// case: another ticket's edit form) with values meant for something else.
func (p BoardPage) FormFor(target string) url.Values {
	if p.Form == nil || p.FormTarget != target {
		return nil
	}
	return p.Form
}

type BoardColumn struct {
	Status       contracts.Status
	Label        string
	Count        int
	Tickets      []TicketCard
	ActiveMobile bool
}

type TicketCard struct {
	Ticket            contracts.TicketSnapshot
	EffectiveReviewer contracts.Actor
	Warnings          []string
	CommentCount      int
	BoardStatus       contracts.Status `json:"-"`
	StatusLabel       string           `json:"-"`
	AgentName         string           `json:"-"`
	AgentColorClass   string           `json:"-"`
}

type TicketDetail struct {
	View            service.TicketDetailView
	Warnings        []string
	Recent          []contracts.Event
	CheckCount      int
	ScheduleAtInput string
	ScheduleAtLabel string
}

var boardStatuses = []contracts.Status{
	contracts.StatusBacklog,
	contracts.StatusReady,
	contracts.StatusInProgress,
	contracts.StatusInReview,
	contracts.StatusBlocked,
	contracts.StatusDone,
}

func (s *Server) buildBoardPage(ctx context.Context, r *http.Request) (BoardPage, error) {
	query := r.URL.Query()
	activeColumn := contracts.Status(strings.TrimSpace(query.Get("column")))
	if !activeColumn.IsValid() || activeColumn == contracts.StatusCanceled {
		activeColumn = contracts.StatusReady
	}
	page := BoardPage{
		Page:            "board",
		Workspace:       s.cfg.Workspace,
		Host:            s.cfg.Host,
		Actor:           s.cfg.Actor,
		ReadOnly:        s.cfg.ReadOnly,
		Project:         firstNonEmpty(query.Get("project"), s.cfg.Project),
		ProjectExplicit: strings.TrimSpace(query.Get("project")) != "",
		View:            strings.TrimSpace(query.Get("view")),
		Query:           strings.TrimSpace(query.Get("q")),
		Assignee:        strings.TrimSpace(query.Get("assignee")),
		Reviewer:        strings.TrimSpace(query.Get("reviewer")),
		Label:           strings.TrimSpace(query.Get("label")),
		Priority:        strings.TrimSpace(query.Get("priority")),
		Type:            strings.TrimSpace(query.Get("type")),
		ActiveColumn:    activeColumn,
		CSRFToken:       s.cfg.CSRFToken,
		Flash:           strings.TrimSpace(query.Get("flash")),
		Error:           strings.TrimSpace(query.Get("error_flash")),
		ShowNew:         query.Get("new") == "1",
		LocationName:    locationName(s.cfg.Location, s.cfg.Clock()),
	}
	board, err := s.loadBoard(ctx, page)
	if err != nil {
		return page, err
	}
	cfg, err := config.Load(s.cfg.Root)
	if err != nil {
		return page, err
	}
	page.Columns = s.columnsFromBoard(ctx, board, page, cfg.Web.AgentColors)
	selected := strings.TrimSpace(query.Get("ticket"))
	if selected == "" {
		selected = firstTicketIDForColumn(page.Columns, page.ActiveColumn)
	}
	if selected != "" {
		detail, err := s.ticketDetail(ctx, selected)
		if err != nil {
			page.Error = err.Error()
		} else {
			page.Detail = &detail
		}
	}
	return page, nil
}

func (s *Server) buildWelcomePage(ctx context.Context, r *http.Request) (WelcomePage, error) {
	page := WelcomePage{
		Page:      "welcome",
		Workspace: s.cfg.Workspace,
		Host:      s.cfg.Host,
		Actor:     s.cfg.Actor,
		CSRFToken: s.cfg.CSRFToken,
		ReadOnly:  s.cfg.ReadOnly,
		Flash:     strings.TrimSpace(r.URL.Query().Get("flash")),
		ShowNew:   r.URL.Query().Get("new_project") == "1",
	}
	cfg, err := config.Load(s.cfg.Root)
	if err != nil {
		return page, err
	}
	page.OwnerName = resolveOwnerName(cfg)
	rollups, err := s.queries.ProjectRollups(ctx)
	if err != nil {
		return page, err
	}
	page.Projects = make([]ProjectRow, 0, len(rollups))
	for _, rollup := range rollups {
		page.Projects = append(page.Projects, ProjectRow{
			Project: rollup.Project,
			Active:  rollup.Active,
			Backlog: rollup.Backlog,
			Done:    rollup.Done,
			Blocked: rollup.Blocked,
		})
	}
	events, err := s.queries.RecentEvents(ctx, 20)
	if err != nil {
		return page, err
	}
	page.Recent = make([]RecentChange, 0, len(events))
	for _, event := range events {
		page.Recent = append(page.Recent, recentChangeFromEvent(event))
	}
	return page, nil
}

func (s *Server) buildSettingsPage() (SettingsPage, error) {
	page := SettingsPage{
		Page:      "settings",
		Workspace: s.cfg.Workspace,
		Host:      s.cfg.Host,
		Actor:     s.cfg.Actor,
		CSRFToken: s.cfg.CSRFToken,
		ReadOnly:  s.cfg.ReadOnly,
	}
	cfg, err := config.Load(s.cfg.Root)
	if err != nil {
		return page, err
	}
	page.OwnerName = cfg.Web.OwnerName
	page.ActorDefault = cfg.Actor.Default
	page.Language = cfg.Web.Lang
	agents := make([]string, 0, len(cfg.Web.AgentColors))
	for agent := range cfg.Web.AgentColors {
		agents = append(agents, agent)
	}
	sort.Strings(agents)
	page.AgentColors = make([]AgentColorSetting, 0, len(agents))
	for _, agent := range agents {
		color := cfg.Web.AgentColors[agent]
		page.AgentColors = append(page.AgentColors, AgentColorSetting{
			Agent: agent,
			Color: color,
			Class: agentColorClass(color),
		})
	}
	return page, nil
}

func resolveOwnerName(cfg contracts.TrackerConfig) string {
	if name := strings.TrimSpace(cfg.Web.OwnerName); name != "" {
		return name
	}
	if actor := strings.TrimSpace(string(cfg.Actor.Default)); actor != "" {
		return humanizeName(strings.TrimPrefix(actor, "human:"))
	}
	current, err := currentOSUser()
	if err != nil || current == nil {
		return ""
	}
	return humanizeName(current.Username)
}

func humanizeName(value string) string {
	parts := strings.FieldsFunc(strings.TrimSpace(value), func(r rune) bool {
		return r == '-' || r == '_' || r == '.' || r == ':' || unicode.IsSpace(r)
	})
	for i, part := range parts {
		runes := []rune(strings.ToLower(part))
		if len(runes) > 0 {
			runes[0] = unicode.ToUpper(runes[0])
			parts[i] = string(runes)
		}
	}
	return strings.Join(parts, " ")
}

func recentChangeFromEvent(event contracts.Event) RecentChange {
	change := RecentChange{
		Project:  event.Project,
		TicketID: event.TicketID,
		At:       event.Timestamp,
	}
	switch event.Type {
	case contracts.EventTicketCreated:
		change.Verb = "created"
	case contracts.EventTicketMoved:
		change.Verb = "moved"
		if payload, ok := event.Payload.(map[string]any); ok {
			change.From = eventPayloadText(payload["from"])
			change.To = eventPayloadText(payload["to"])
		}
	case contracts.EventTicketCommented:
		change.Verb = "commented"
	case contracts.EventTicketUpdated:
		change.Verb = "updated"
	}
	return change
}

func eventPayloadText(value any) string {
	switch value := value.(type) {
	case string:
		return strings.TrimSpace(value)
	case contracts.Status:
		return strings.TrimSpace(string(value))
	default:
		return ""
	}
}

func agentColorClass(color string) string {
	switch strings.ToLower(strings.TrimSpace(color)) {
	case "orange":
		return "chip--orange"
	case "blue":
		return "chip--blue"
	default:
		return "chip--plain"
	}
}

func (s *Server) loadBoard(ctx context.Context, page BoardPage) (contracts.BoardView, error) {
	var board contracts.BoardView
	if page.View != "" {
		result, err := s.queries.RunSavedView(ctx, page.View, "")
		if err != nil {
			return contracts.BoardView{}, err
		}
		if result.Board == nil {
			return contracts.BoardView{}, errUnsupportedSavedView(page.View)
		}
		board = result.Board.Board
	} else {
		// SQL scopes by project only; every other predicate lives in
		// filterBoard so both board paths share one filtering semantics
		vm, err := s.queries.Board(ctx, contracts.BoardQueryOptions{Project: page.Project})
		if err != nil {
			return contracts.BoardView{}, err
		}
		board = vm.Board
	}
	return filterBoard(board, page), nil
}

func (s *Server) columnsFromBoard(ctx context.Context, board contracts.BoardView, page BoardPage, agentColors map[string]string) []BoardColumn {
	ids := make([]string, 0, 64)
	for _, status := range boardStatuses {
		for _, ticket := range board.Columns[status] {
			ids = append(ids, ticket.ID)
		}
	}
	// badge data only — a failed count query should not take the board down
	commentCounts, err := s.queries.CommentCounts(ctx, ids)
	if err != nil {
		commentCounts = map[string]int{}
	}
	columns := make([]BoardColumn, 0, len(boardStatuses))
	for _, status := range boardStatuses {
		tickets := append([]contracts.TicketSnapshot{}, board.Columns[status]...)
		sort.SliceStable(tickets, func(i, j int) bool {
			if tickets[i].UpdatedAt.Equal(tickets[j].UpdatedAt) {
				return tickets[i].ID < tickets[j].ID
			}
			return tickets[i].UpdatedAt.After(tickets[j].UpdatedAt)
		})
		cards := make([]TicketCard, 0, len(tickets))
		for _, ticket := range tickets {
			agentName, colorClass := agentChipForAssignee(ticket.Assignee, agentColors)
			cards = append(cards, TicketCard{
				Ticket:            ticket,
				EffectiveReviewer: ticket.Reviewer,
				Warnings:          cardWarnings(ticket),
				CommentCount:      commentCounts[ticket.ID],
				BoardStatus:       status,
				StatusLabel:       statusLabel(status),
				AgentName:         agentName,
				AgentColorClass:   colorClass,
			})
		}
		columns = append(columns, BoardColumn{
			Status:       status,
			Label:        statusLabel(status),
			Count:        len(cards),
			Tickets:      cards,
			ActiveMobile: status == page.ActiveColumn,
		})
	}
	return columns
}

func agentChipForAssignee(assignee contracts.Actor, colors map[string]string) (string, string) {
	raw := strings.ToLower(strings.TrimSpace(string(assignee)))
	agent, ok := strings.CutPrefix(raw, "agent:")
	if !ok || strings.TrimSpace(agent) == "" {
		return "", ""
	}
	color, ok := colors[agent]
	if !ok {
		return "", ""
	}
	class := agentColorClass(color)
	if class == "chip--plain" {
		return "", ""
	}
	return agent, class
}

func (s *Server) ticketDetail(ctx context.Context, ticketID string) (TicketDetail, error) {
	view, err := s.queries.TicketDetail(ctx, ticketID)
	if err != nil {
		return TicketDetail{}, err
	}
	recent := view.History
	if len(recent) > 6 {
		recent = recent[len(recent)-6:]
	}
	detail := TicketDetail{
		View:       view,
		Warnings:   detailWarnings(view),
		Recent:     recent,
		CheckCount: len(view.Checks),
	}
	if view.Ticket.Schedule != nil {
		local := view.Ticket.Schedule.At.In(s.cfg.Location)
		detail.ScheduleAtInput = local.Format("2006-01-02T15:04")
		detail.ScheduleAtLabel = local.Format("Mon, Jan 2 at 3:04 PM")
	}
	return detail, nil
}

func filterBoard(board contracts.BoardView, page BoardPage) contracts.BoardView {
	out := contracts.BoardView{Columns: map[contracts.Status][]contracts.TicketSnapshot{}}
	query := strings.ToLower(page.Query)
	// project is special: the direct path already scopes it in SQL, and a
	// saved view defines its own scope — only an explicit user filter may
	// narrow a view further
	narrowProject := page.View != "" && page.ProjectExplicit && page.Project != ""
	for _, status := range boardStatuses {
		for _, ticket := range board.Columns[status] {
			if page.Assignee != "" && string(ticket.Assignee) != page.Assignee {
				continue
			}
			if narrowProject && !strings.EqualFold(ticket.Project, page.Project) {
				continue
			}
			if page.Type != "" && string(ticket.Type) != page.Type {
				continue
			}
			if page.Reviewer != "" && string(ticket.Reviewer) != page.Reviewer {
				continue
			}
			if page.Label != "" && !containsString(ticket.Labels, page.Label) {
				continue
			}
			if page.Priority != "" && string(ticket.Priority) != page.Priority {
				continue
			}
			if query != "" && !ticketMatchesText(ticket, query) {
				continue
			}
			out.Columns[status] = append(out.Columns[status], ticket)
		}
	}
	return out
}

func cardWarnings(ticket contracts.TicketSnapshot) []string {
	warnings := []string{}
	if len(ticket.BlockedBy) > 0 {
		warnings = append(warnings, "blocked")
	}
	if len(ticket.OpenGateIDs) > 0 {
		warnings = append(warnings, "gate")
	}
	return warnings
}

func detailWarnings(view service.TicketDetailView) []string {
	warnings := []string{}
	if len(view.Ticket.BlockedBy) > 0 {
		warnings = append(warnings, "Blocked by "+strings.Join(view.Ticket.BlockedBy, ", "))
	}
	if view.EffectiveReviewer != "" {
		warnings = append(warnings, "Required reviewer: "+string(view.EffectiveReviewer))
	}
	return warnings
}

func firstTicketID(columns []BoardColumn) string {
	for _, column := range columns {
		if len(column.Tickets) > 0 {
			return column.Tickets[0].Ticket.ID
		}
	}
	return ""
}

func firstTicketIDForColumn(columns []BoardColumn, status contracts.Status) string {
	for _, column := range columns {
		if column.Status == status && len(column.Tickets) > 0 {
			return column.Tickets[0].Ticket.ID
		}
	}
	return firstTicketID(columns)
}

func ticketMatchesText(ticket contracts.TicketSnapshot, query string) bool {
	haystack := strings.ToLower(strings.Join([]string{
		ticket.ID,
		ticket.Title,
		ticket.Summary,
		ticket.Description,
		strings.Join(ticket.Labels, " "),
		string(ticket.Assignee),
		string(ticket.Reviewer),
	}, " "))
	return strings.Contains(haystack, query)
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(wanted)) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func errUnsupportedSavedView(name string) error {
	return apperr.New(apperr.CodeInvalidInput, "saved view "+name+" is not a board view")
}
