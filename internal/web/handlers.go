package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		s.writeError(w, r, apperr.New(apperr.CodeNotFound, "page not found"), http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		s.writeError(w, r, apperr.New(apperr.CodeInvalidInput, "method not allowed"), http.StatusMethodNotAllowed)
		return
	}
	page, err := s.buildWelcomePage(r.Context(), r)
	if err != nil {
		page.Error = err.Error()
	}
	s.renderPage(w, r, page, http.StatusOK)
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, r, apperr.New(apperr.CodeInvalidInput, "method not allowed"), http.StatusMethodNotAllowed)
		return
	}
	page, err := s.buildSettingsPage()
	if err != nil {
		page.Error = err.Error()
	}
	s.renderPage(w, r, page, http.StatusOK)
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
			Page:         "board",
			Workspace:    s.cfg.Workspace,
			Host:         s.cfg.Host,
			Actor:        s.cfg.Actor,
			ReadOnly:     s.cfg.ReadOnly,
			Project:      s.cfg.Project,
			CSRFToken:    s.cfg.CSRFToken,
			LocationName: locationName(s.cfg.Location, s.cfg.Clock()),
			Error:        err.Error(),
		}
	}
	s.renderPage(w, r, page, http.StatusOK)
}

func (s *Server) renderPage(w http.ResponseWriter, r *http.Request, page any, status int) {
	// Render to a buffer first. A template failure mid-stream would otherwise
	// write half a page with a success status and error text appended.
	var buf bytes.Buffer
	templates, err := s.templatesForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := templates.ExecuteTemplate(&buf, "layout", page); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) templatesForRequest(r *http.Request) (*template.Template, error) {
	lang := s.requestLanguage(r)
	t := translator(lang)
	templates, err := s.templates.Clone()
	if err != nil {
		return nil, err
	}
	return templates.Funcs(template.FuncMap{
		"lang":              func() string { return lang },
		"langURL":           func(next string) string { return languageURL(r.URL, next) },
		"recentDescription": func(change RecentChange) string { return recentDescription(t, change) },
		"t":                 t,
	}), nil
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

func (s *Server) handleSchedule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, r, apperr.New(apperr.CodeInvalidInput, "method not allowed"), http.StatusMethodNotAllowed)
		return
	}
	page, err := s.buildSchedulePage(r.Context(), r)
	if err != nil {
		page.Error = err.Error()
	}
	s.renderPage(w, r, page, http.StatusOK)
}

func (s *Server) handleScheduleAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, r, apperr.New(apperr.CodeInvalidInput, "method not allowed"), http.StatusMethodNotAllowed)
		return
	}
	page, err := s.buildSchedulePage(r.Context(), r)
	if err != nil {
		s.writeError(w, r, err, statusForError(err))
		return
	}
	s.writeJSON(w, "atlas_web_schedule", page)
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
		s.writeActionError(w, r, apperr.New(apperr.CodePermissionDenied, "web board is read-only"), "")
		return
	}
	actor := s.actorFromForm(r)
	reason := reasonFromForm(r, "web ticket create")
	status, err := parseStatusDefault(r.Form.Get("status"), contracts.StatusBacklog)
	if err != nil {
		s.writeActionError(w, r, err, "")
		return
	}
	// same invariant the CLI enforces: tickets are not born finished
	if contracts.IsTerminalStatus(status) {
		s.writeActionError(w, r, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("status %s is not allowed on ticket create", status)), "")
		return
	}
	ticketType, err := parseTypeDefault(r.Form.Get("type"), contracts.TicketTypeTask)
	if err != nil {
		s.writeActionError(w, r, err, "")
		return
	}
	priority, err := parsePriorityDefault(r.Form.Get("priority"), contracts.PriorityMedium)
	if err != nil {
		s.writeActionError(w, r, err, "")
		return
	}
	now := s.cfg.Clock().UTC()
	ticket := contracts.TicketSnapshot{
		Project:            firstNonEmpty(r.Form.Get("project"), s.cfg.Project),
		Title:              strings.TrimSpace(r.Form.Get("title")),
		Type:               ticketType,
		Status:             status,
		Priority:           priority,
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
		s.writeActionError(w, r, err, "")
		return
	}
	s.actionSuccess(w, r, created.ID, "created "+created.ID)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, r, apperr.New(apperr.CodeInvalidInput, "method not allowed"), http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.ReadOnly {
		s.writeProjectActionError(w, r, apperr.New(apperr.CodePermissionDenied, "web board is read-only"))
		return
	}
	project := contracts.Project{
		Key:           strings.TrimSpace(r.Form.Get("key")),
		Name:          strings.TrimSpace(r.Form.Get("name")),
		CreatedAt:     s.cfg.Clock().UTC(),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	project = contracts.NormalizeProject(project)
	if err := project.Validate(); err != nil {
		s.writeProjectActionError(w, r, apperr.New(apperr.CodeInvalidInput, err.Error()))
		return
	}
	if err := s.actions.CreateProject(s.mutationContext(r, s.actorFromForm(r)), project); err != nil {
		s.writeProjectActionError(w, r, err)
		return
	}
	if wantsJSON(r) {
		s.writeJSON(w, "atlas_web_project_action", map[string]any{"ok": true, "project": project.Key})
		return
	}
	http.Redirect(w, r, "/?flash="+url.QueryEscape("created project "+project.Key), http.StatusSeeOther)
}

func (s *Server) handleTicketAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, r, apperr.New(apperr.CodeInvalidInput, "method not allowed"), http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.ReadOnly {
		s.writeActionError(w, r, apperr.New(apperr.CodePermissionDenied, "web board is read-only"), "")
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
		ticket, err = s.actions.MutateTrackedTicket(ctx, id, actor, reason, "web edit ticket", editMutatorFromForm(r))
	case "move":
		var handled bool
		ticket, handled, err = s.applyMoveAction(w, r, ctx, id, actor, reason)
		if handled {
			return
		}
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
	case "schedule":
		var at time.Time
		at, err = s.parseScheduleLocal(r.Form.Get("at"))
		if err == nil {
			ticket, err = s.actions.SetTicketSchedule(ctx, id, at, contracts.Actor(strings.TrimSpace(r.Form.Get("runner"))), actor, reason)
		}
	case "schedule/clear":
		ticket, err = s.actions.ClearTicketSchedule(ctx, id, actor, reason)
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
		s.writeActionError(w, r, err, id)
		return
	}
	s.actionSuccess(w, r, ticket.ID, fmt.Sprintf("updated %s", ticket.ID))
}

func (s *Server) handleScheduleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, r, apperr.New(apperr.CodeInvalidInput, "method not allowed"), http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.ReadOnly {
		s.writeActionError(w, r, apperr.New(apperr.CodePermissionDenied, "web schedule is read-only"), "")
		return
	}
	action := strings.Trim(strings.TrimPrefix(r.URL.Path, "/actions/schedule/"), "/")
	actor := s.actorFromForm(r)
	ctx := s.mutationContext(r, actor)
	switch action {
	case "set":
		at, err := s.parseScheduleLocal(r.Form.Get("at"))
		if err != nil {
			s.writeActionError(w, r, err, "")
			return
		}
		ticketID := strings.TrimSpace(r.Form.Get("ticket_id"))
		if ticketID == "" {
			s.writeActionError(w, r, apperr.New(apperr.CodeInvalidInput, "ticket id is required"), "")
			return
		}
		ticket, err := s.actions.SetTicketSchedule(ctx, ticketID, at, contracts.Actor(strings.TrimSpace(r.Form.Get("runner"))), actor, reasonFromForm(r, "web schedule ticket"))
		if err != nil {
			s.writeActionError(w, r, err, ticketID)
			return
		}
		s.actionSuccess(w, r, ticket.ID, "scheduled "+ticket.ID)
	case "tick":
		result, err := s.actions.TickSchedules(ctx, time.Time{}, actor, reasonFromForm(r, "web schedule tick"))
		if err != nil {
			s.writeActionError(w, r, err, "")
			return
		}
		if wantsJSON(r) {
			s.writeJSON(w, "atlas_web_schedule_tick", result)
			return
		}
		s.actionSuccess(w, r, "", fmt.Sprintf("processed %d due schedule(s)", len(result.Entries)))
	default:
		s.writeActionError(w, r, apperr.New(apperr.CodeInvalidInput, "unknown web schedule action: "+action), "")
	}
}

func (s *Server) parseScheduleLocal(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, apperr.New(apperr.CodeInvalidInput, "schedule time is required")
	}
	for _, layout := range []string{"2006-01-02T15:04", "2006-01-02T15:04:05"} {
		at, err := time.ParseInLocation(layout, raw, s.cfg.Location)
		if err == nil {
			if at.In(s.cfg.Location).Format(layout) != raw {
				return time.Time{}, apperr.New(apperr.CodeInvalidInput, "schedule time does not exist in "+locationName(s.cfg.Location, s.cfg.Clock()))
			}
			return at.UTC(), nil
		}
	}
	return time.Time{}, apperr.New(apperr.CodeInvalidInput, "schedule time must be a local date and time in "+locationName(s.cfg.Location, s.cfg.Clock()))
}

// editMutatorFromForm applies only the submitted fields; the store validates
// on create but not on update, so edits must reject what create would refuse.
func editMutatorFromForm(r *http.Request) func(*contracts.TicketSnapshot) error {
	return func(ticket *contracts.TicketSnapshot) error {
		if r.Form.Has("title") {
			title := strings.TrimSpace(r.Form.Get("title"))
			if title == "" {
				return apperr.New(apperr.CodeInvalidInput, "title is required")
			}
			ticket.Title = title
		}
		if r.Form.Has("description") {
			ticket.Description = strings.TrimSpace(r.Form.Get("description"))
		}
		if r.Form.Has("priority") {
			priority, err := parsePriorityDefault(r.Form.Get("priority"), ticket.Priority)
			if err != nil {
				return err
			}
			ticket.Priority = priority
		}
		if r.Form.Has("assignee") {
			assignee := strings.TrimSpace(r.Form.Get("assignee"))
			if assignee != "" && !contracts.Actor(assignee).IsValid() {
				return apperr.New(apperr.CodeInvalidInput, "invalid assignee actor: "+assignee)
			}
			ticket.Assignee = contracts.Actor(assignee)
		}
		if r.Form.Has("reviewer") {
			reviewer := strings.TrimSpace(r.Form.Get("reviewer"))
			if reviewer != "" && !contracts.Actor(reviewer).IsValid() {
				return apperr.New(apperr.CodeInvalidInput, "invalid reviewer actor: "+reviewer)
			}
			ticket.Reviewer = contracts.Actor(reviewer)
		}
		if r.Form.Has("labels") {
			ticket.Labels = splitCSV(r.Form.Get("labels"))
		}
		if r.Form.Has("acceptance") {
			ticket.AcceptanceCriteria = splitLines(r.Form.Get("acceptance"))
		}
		return nil
	}
}

// applyMoveAction validates the drop target and short-circuits same-status
// drops: cards can render in a projected column (e.g. Blocked) while the
// stored status is something else, and dropping a card on its own status is
// a no-op, not a forbidden self-transition. handled=true means the response
// was already written.
func (s *Server) applyMoveAction(w http.ResponseWriter, r *http.Request, ctx context.Context, id string, actor contracts.Actor, reason string) (contracts.TicketSnapshot, bool, error) {
	to, err := parseStatusStrict(r.Form.Get("status"))
	if err != nil {
		return contracts.TicketSnapshot{}, false, err
	}
	current, err := s.actions.Tickets.GetTicket(ctx, id)
	if err != nil {
		return contracts.TicketSnapshot{}, false, err
	}
	if current.Status == to {
		s.actionSuccess(w, r, id, fmt.Sprintf("%s is already %s", id, statusLabel(to)))
		return current, true, nil
	}
	ticket, err := s.moveTicket(ctx, id, to, actor, reason, contracts.Actor(strings.TrimSpace(r.Form.Get("reviewer"))))
	return ticket, false, err
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
	if q.Get("return") == "schedule" {
		q.Del("return")
		q.Set("flash", flash)
		http.Redirect(w, r, "/schedule?"+q.Encode(), http.StatusSeeOther)
		return
	}
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

func parseStatusStrict(raw string) (contracts.Status, error) {
	status := contracts.Status(strings.TrimSpace(raw))
	if !status.IsValid() {
		return "", apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid status: %s (valid: %s)", strings.TrimSpace(raw), strings.Join(contracts.ValidStatusValues(), ", ")))
	}
	return status, nil
}

func parseStatusDefault(raw string, fallback contracts.Status) (contracts.Status, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	return parseStatusStrict(raw)
}

func parseTypeDefault(raw string, fallback contracts.TicketType) (contracts.TicketType, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback, nil
	}
	value := contracts.TicketType(trimmed)
	if !value.IsValid() {
		return "", apperr.New(apperr.CodeInvalidInput, "invalid ticket type: "+trimmed)
	}
	return value, nil
}

func parsePriorityDefault(raw string, fallback contracts.Priority) (contracts.Priority, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback, nil
	}
	value := contracts.Priority(trimmed)
	if !value.IsValid() {
		return "", apperr.New(apperr.CodeInvalidInput, "invalid priority: "+trimmed)
	}
	return value, nil
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
