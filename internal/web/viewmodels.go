package web

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

type BoardPage struct {
	Workspace    string
	Host         string
	Actor        contracts.Actor
	ReadOnly     bool
	Project      string
	View         string
	Query        string
	Assignee     string
	Reviewer     string
	Label        string
	Priority     string
	Type         string
	ActiveColumn contracts.Status
	CSRFToken    string
	Columns      []BoardColumn
	Detail       *TicketDetail
	Flash        string
	Error        string
	ShowNew      bool
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
		Workspace:    s.cfg.Workspace,
		Host:         s.cfg.Host,
		Actor:        s.cfg.Actor,
		ReadOnly:     s.cfg.ReadOnly,
		Project:      firstNonEmpty(query.Get("project"), s.cfg.Project),
		View:         strings.TrimSpace(query.Get("view")),
		Query:        strings.TrimSpace(query.Get("q")),
		Assignee:     strings.TrimSpace(query.Get("assignee")),
		Reviewer:     strings.TrimSpace(query.Get("reviewer")),
		Label:        strings.TrimSpace(query.Get("label")),
		Priority:     strings.TrimSpace(query.Get("priority")),
		Type:         strings.TrimSpace(query.Get("type")),
		ActiveColumn: activeColumn,
		CSRFToken:    s.cfg.CSRFToken,
		Flash:        strings.TrimSpace(query.Get("flash")),
		ShowNew:      query.Get("new") == "1",
	}
	board, err := s.loadBoard(ctx, page)
	if err != nil {
		return page, err
	}
	page.Columns = s.columnsFromBoard(board, page)
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
		vm, err := s.queries.Board(ctx, contracts.BoardQueryOptions{
			Project:  page.Project,
			Assignee: contracts.Actor(page.Assignee),
			Type:     contracts.TicketType(page.Type),
		})
		if err != nil {
			return contracts.BoardView{}, err
		}
		board = vm.Board
	}
	return filterBoard(board, page), nil
}

func (s *Server) columnsFromBoard(board contracts.BoardView, page BoardPage) []BoardColumn {
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
	for _, status := range boardStatuses {
		for _, ticket := range board.Columns[status] {
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
	return &savedViewError{name: name}
}

type savedViewError struct {
	name string
}

func (e *savedViewError) Error() string {
	return "saved view " + e.name + " is not a board view"
}
