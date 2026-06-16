package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		s.writeError(w, r, apperr.New(apperr.CodeNotFound, "page not found"), http.StatusNotFound)
		return
	}
	http.Redirect(w, r, "/board", http.StatusSeeOther)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/static/favicon.svg", http.StatusSeeOther)
}

func (s *Server) handleBoard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, r, apperr.New(apperr.CodeInvalidInput, "method not allowed"), http.StatusMethodNotAllowed)
		return
	}
	page, err := s.buildBoardPage(r.Context(), r)
	if err != nil {
		page = BoardPage{
			Workspace: s.cfg.Workspace,
			Host:      s.cfg.Host,
			Actor:     s.cfg.Actor,
			ReadOnly:  s.cfg.ReadOnly,
			Project:   s.cfg.Project,
			CSRFToken: s.cfg.CSRFToken,
			Error:     err.Error(),
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "layout", page); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleBoardAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, r, apperr.New(apperr.CodeInvalidInput, "method not allowed"), http.StatusMethodNotAllowed)
		return
	}
	page, err := s.buildBoardPage(r.Context(), r)
	if err != nil {
		s.writeError(w, r, err, http.StatusBadRequest)
		return
	}
	s.writeJSON(w, "atlas_web_board", page)
}

func (s *Server) handleTicketAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, r, apperr.New(apperr.CodeInvalidInput, "method not allowed"), http.StatusMethodNotAllowed)
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/tickets/"), "/")
	if id == "" {
		s.writeError(w, r, apperr.New(apperr.CodeInvalidInput, "ticket id is required"), http.StatusBadRequest)
		return
	}
	detail, err := s.ticketDetail(r.Context(), id)
	if err != nil {
		s.writeError(w, r, err, statusForError(err))
		return
	}
	s.writeJSON(w, "atlas_web_ticket", detail)
}

func (s *Server) handleTicketPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, r, apperr.New(apperr.CodeInvalidInput, "method not allowed"), http.StatusMethodNotAllowed)
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/tickets/"), "/")
	if rest == "" {
		http.Redirect(w, r, "/board", http.StatusSeeOther)
		return
	}
	id, _, _ := strings.Cut(rest, "/")
	q := r.URL.Query()
	q.Set("ticket", id)
	target := url.URL{Path: "/board", RawQuery: q.Encode()}
	http.Redirect(w, r, target.String(), http.StatusSeeOther)
}

func (s *Server) handleNewTicket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, r, apperr.New(apperr.CodeInvalidInput, "method not allowed"), http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	q.Set("new", "1")
	target := url.URL{Path: "/board", RawQuery: q.Encode()}
	http.Redirect(w, r, target.String(), http.StatusSeeOther)
}

func (s *Server) handleCreateTicket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, r, apperr.New(apperr.CodeInvalidInput, "method not allowed"), http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.ReadOnly {
		s.writeError(w, r, apperr.New(apperr.CodePermissionDenied, "web board is read-only"), http.StatusForbidden)
		return
	}
	actor := s.actorFromForm(r)
	reason := reasonFromForm(r, "web ticket create")
	now := s.cfg.Clock().UTC()
	ticket := contracts.TicketSnapshot{
		Project:            firstNonEmpty(r.Form.Get("project"), s.cfg.Project),
		Title:              strings.TrimSpace(r.Form.Get("title")),
		Type:               parseType(r.Form.Get("type")),
		Status:             parseStatus(firstNonEmpty(r.Form.Get("status"), string(contracts.StatusBacklog))),
		Priority:           parsePriority(r.Form.Get("priority")),
		Assignee:           contracts.Actor(strings.TrimSpace(r.Form.Get("assignee"))),
		Reviewer:           contracts.Actor(strings.TrimSpace(r.Form.Get("reviewer"))),
		Labels:             splitCSV(r.Form.Get("labels")),
		Description:        strings.TrimSpace(r.Form.Get("description")),
		AcceptanceCriteria: splitLines(r.Form.Get("acceptance")),
		CreatedAt:          now,
		UpdatedAt:          now,
		SchemaVersion:      contracts.CurrentSchemaVersion,
	}
	created, err := s.actions.CreateTrackedTicket(s.mutationContext(r, actor), ticket, actor, reason)
	if err != nil {
		s.writeError(w, r, err, statusForError(err))
		return
	}
	s.actionSuccess(w, r, created.ID, "created "+created.ID)
}

func (s *Server) handleTicketAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, r, apperr.New(apperr.CodeInvalidInput, "method not allowed"), http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.ReadOnly {
		s.writeError(w, r, apperr.New(apperr.CodePermissionDenied, "web board is read-only"), http.StatusForbidden)
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/actions/tickets/"), "/")
	id, action, ok := strings.Cut(rest, "/")
	if !ok || id == "" || action == "" {
		s.writeError(w, r, apperr.New(apperr.CodeInvalidInput, "ticket action path is invalid"), http.StatusBadRequest)
		return
	}
	actor := s.actorFromForm(r)
	reason := reasonFromForm(r, "web ticket "+strings.ReplaceAll(action, "/", " "))
	ctx := s.mutationContext(r, actor)
	var ticket contracts.TicketSnapshot
	var err error
	switch action {
	case "edit":
		ticket, err = s.actions.MutateTrackedTicket(ctx, id, actor, reason, "web edit ticket", func(ticket *contracts.TicketSnapshot) error {
			if r.Form.Has("title") {
				ticket.Title = strings.TrimSpace(r.Form.Get("title"))
			}
			if r.Form.Has("description") {
				ticket.Description = strings.TrimSpace(r.Form.Get("description"))
			}
			if r.Form.Has("priority") {
				ticket.Priority = parsePriority(r.Form.Get("priority"))
			}
			if r.Form.Has("assignee") {
				ticket.Assignee = contracts.Actor(strings.TrimSpace(r.Form.Get("assignee")))
			}
			if r.Form.Has("reviewer") {
				ticket.Reviewer = contracts.Actor(strings.TrimSpace(r.Form.Get("reviewer")))
			}
			if r.Form.Has("labels") {
				ticket.Labels = splitCSV(r.Form.Get("labels"))
			}
			if r.Form.Has("acceptance") {
				ticket.AcceptanceCriteria = splitLines(r.Form.Get("acceptance"))
			}
			return nil
		})
	case "move":
		to := parseStatus(r.Form.Get("status"))
		ticket, err = s.moveTicket(ctx, id, to, actor, reason, contracts.Actor(strings.TrimSpace(r.Form.Get("reviewer"))))
	case "assign":
		ticket, err = s.actions.AssignTicket(ctx, id, contracts.Actor(strings.TrimSpace(r.Form.Get("assignee"))), actor, reason)
	case "comment":
		err = s.actions.CommentTicket(ctx, id, strings.TrimSpace(r.Form.Get("body")), actor, reason)
		if err == nil {
			ticket, err = s.actions.Tickets.GetTicket(ctx, id)
		}
	case "request-review":
		ticket, err = s.actions.RequestReviewWithReviewer(ctx, id, contracts.Actor(strings.TrimSpace(r.Form.Get("reviewer"))), actor, reason)
	case "approve":
		ticket, err = s.actions.ApproveTicket(ctx, id, actor, reason)
	case "complete":
		ticket, err = s.actions.CompleteTicket(ctx, id, actor, reason)
	case "label/add":
		label := strings.TrimSpace(r.Form.Get("label"))
		ticket, err = s.actions.MutateTrackedTicket(ctx, id, actor, reason, "web add label", func(ticket *contracts.TicketSnapshot) error {
			if label != "" && !containsString(ticket.Labels, label) {
				ticket.Labels = append(ticket.Labels, label)
			}
			return nil
		})
	case "label/remove":
		label := strings.TrimSpace(r.Form.Get("label"))
		ticket, err = s.actions.MutateTrackedTicket(ctx, id, actor, reason, "web remove label", func(ticket *contracts.TicketSnapshot) error {
			next := ticket.Labels[:0]
			for _, value := range ticket.Labels {
				if !strings.EqualFold(value, label) {
					next = append(next, value)
				}
			}
			ticket.Labels = next
			return nil
		})
	default:
		err = apperr.New(apperr.CodeInvalidInput, "unknown web ticket action: "+action)
	}
	if err != nil {
		s.writeError(w, r, err, statusForError(err))
		return
	}
	s.actionSuccess(w, r, ticket.ID, fmt.Sprintf("updated %s", ticket.ID))
}

func (s *Server) moveTicket(ctx context.Context, id string, to contracts.Status, actor contracts.Actor, reason string, reviewer contracts.Actor) (contracts.TicketSnapshot, error) {
	switch to {
	case contracts.StatusInReview:
		return s.actions.RequestReviewWithReviewer(ctx, id, reviewer, actor, reason)
	case contracts.StatusDone:
		return s.actions.CompleteTicket(ctx, id, actor, reason)
	default:
		return s.actions.MoveTicket(ctx, id, to, actor, reason)
	}
}

func (s *Server) actionSuccess(w http.ResponseWriter, r *http.Request, ticketID string, flash string) {
	if wantsJSON(r) {
		s.writeJSON(w, "atlas_web_action", map[string]any{"ok": true, "ticket_id": ticketID, "flash": flash})
		return
	}
	q := r.URL.Query()
	q.Set("ticket", ticketID)
	q.Set("flash", flash)
	http.Redirect(w, r, "/board?"+q.Encode(), http.StatusSeeOther)
}

func (s *Server) actorFromForm(r *http.Request) contracts.Actor {
	if actor := strings.TrimSpace(r.Form.Get("actor")); actor != "" {
		return contracts.Actor(actor)
	}
	return s.cfg.Actor
}

func reasonFromForm(r *http.Request, fallback string) string {
	if reason := strings.TrimSpace(r.Form.Get("reason")); reason != "" {
		return reason
	}
	return fallback
}

func parseStatus(raw string) contracts.Status {
	status := contracts.Status(strings.TrimSpace(raw))
	if !status.IsValid() {
		return contracts.StatusBacklog
	}
	return status
}

func parseType(raw string) contracts.TicketType {
	value := contracts.TicketType(strings.TrimSpace(raw))
	if !value.IsValid() {
		return contracts.TicketTypeTask
	}
	return value
}

func parsePriority(raw string) contracts.Priority {
	value := contracts.Priority(strings.TrimSpace(raw))
	if !value.IsValid() {
		return contracts.PriorityMedium
	}
	return value
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func splitLines(raw string) []string {
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func statusForError(err error) int {
	switch apperr.CodeOf(err) {
	case apperr.CodeNotFound:
		return http.StatusNotFound
	case apperr.CodeInvalidInput:
		return http.StatusBadRequest
	case apperr.CodePermissionDenied:
		return http.StatusForbidden
	case apperr.CodeConflict:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func encodeJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}
