package web

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

type BoardPage struct {
	Workspace string
	Host      string
	Actor     contracts.Actor
	ReadOnly  bool
	Project   string
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
}

type TicketDetail struct {
	View       service.TicketDetailView
	Warnings   []string
	Recent     []contracts.Event
	CheckCount int
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
	}
	board, err := s.loadBoard(ctx, page)
	if err != nil {
		return page, err
	}
	page.Columns = s.columnsFromBoard(ctx, board, page)
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

func (s *Server) columnsFromBoard(ctx context.Context, board contracts.BoardView, page BoardPage) []BoardColumn {
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
			cards = append(cards, TicketCard{
				Ticket:            ticket,
				EffectiveReviewer: ticket.Reviewer,
				Warnings:          cardWarnings(ticket),
				CommentCount:      commentCounts[ticket.ID],
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

func (s *Server) ticketDetail(ctx context.Context, ticketID string) (TicketDetail, error) {
	view, err := s.queries.TicketDetail(ctx, ticketID)
	if err != nil {
		return TicketDetail{}, err
	}
	recent := view.History
	if len(recent) > 6 {
		recent = recent[len(recent)-6:]
	}
	return TicketDetail{
		View:       view,
		Warnings:   detailWarnings(view),
		Recent:     recent,
		CheckCount: len(view.Checks),
	}, nil
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
