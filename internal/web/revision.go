package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

// ticketRevisionMaterial is the stable subset of a ticket snapshot used as an
// optimistic-concurrency token. Identity plus the fields a human or agent
// actually edits — not request-only viewmodel chrome.
type ticketRevisionMaterial struct {
	ID                 string   `json:"id"`
	TicketUID          string   `json:"ticket_uid,omitempty"`
	Project            string   `json:"project"`
	Title              string   `json:"title"`
	Type               string   `json:"type"`
	Status             string   `json:"status"`
	Priority           string   `json:"priority"`
	Parent             string   `json:"parent,omitempty"`
	Labels             []string `json:"labels,omitempty"`
	Assignee           string   `json:"assignee,omitempty"`
	Reviewer           string   `json:"reviewer,omitempty"`
	BlockedBy          []string `json:"blocked_by,omitempty"`
	Blocks             []string `json:"blocks,omitempty"`
	UpdatedAt          string   `json:"updated_at"`
	Archived           bool     `json:"archived,omitempty"`
	ReviewState        string   `json:"review_state,omitempty"`
	Summary            string   `json:"summary,omitempty"`
	Description        string   `json:"description,omitempty"`
	AcceptanceCriteria []string `json:"acceptance_criteria,omitempty"`
	Notes              string   `json:"notes,omitempty"`
	LatestRunID        string   `json:"latest_run_id,omitempty"`
	ScheduleAt         string   `json:"schedule_at,omitempty"`
	ScheduleRunner     string   `json:"schedule_runner,omitempty"`
}

func TicketRevision(ticket contracts.TicketSnapshot) string {
	material := ticketRevisionMaterial{
		ID:                 ticket.ID,
		TicketUID:          ticket.TicketUID,
		Project:            ticket.Project,
		Title:              ticket.Title,
		Type:               string(ticket.Type),
		Status:             string(ticket.Status),
		Priority:           string(ticket.Priority),
		Parent:             ticket.Parent,
		Labels:             sortedCopy(ticket.Labels),
		Assignee:           string(ticket.Assignee),
		Reviewer:           string(ticket.Reviewer),
		BlockedBy:          sortedCopy(ticket.BlockedBy),
		Blocks:             sortedCopy(ticket.Blocks),
		UpdatedAt:          ticket.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Archived:           ticket.Archived,
		ReviewState:        string(ticket.ReviewState),
		Summary:            ticket.Summary,
		Description:        ticket.Description,
		AcceptanceCriteria: append([]string{}, ticket.AcceptanceCriteria...),
		Notes:              ticket.Notes,
		LatestRunID:        ticket.LatestRunID,
	}
	if ticket.Schedule != nil {
		material.ScheduleAt = ticket.Schedule.At.UTC().Format(time.RFC3339Nano)
		material.ScheduleRunner = string(ticket.Schedule.CreatedBy)
	}
	raw, err := json.Marshal(material)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func sortedCopy(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := append([]string{}, values...)
	sort.Strings(out)
	return out
}

func validRevision(raw string) bool {
	if len(raw) != 64 {
		return false
	}
	for _, r := range raw {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func errRevisionConflict(message string) error {
	return apperr.New(apperr.CodeConflict, message)
}

func revisionFromRequest(r *http.Request, ticketID string) string {
	if ticketID != "" {
		if keyed := strings.TrimSpace(r.Form.Get("expected_revision." + ticketID)); keyed != "" {
			return keyed
		}
	}
	return strings.TrimSpace(r.Form.Get("expected_revision"))
}

func bulkRevisionsFromRequest(r *http.Request, ids []string) map[string]string {
	out := map[string]string{}
	for _, id := range ids {
		if rev := revisionFromRequest(r, id); rev != "" {
			out[id] = rev
		}
	}
	return out
}

func (s *Server) matchTicketRevision(ctx context.Context, ticketID, expected string) error {
	if expected == "" {
		return nil
	}
	if !validRevision(expected) {
		return errRevisionConflict("malformed expected_revision")
	}
	current, err := s.actions.Tickets.GetTicket(ctx, ticketID)
	if err != nil {
		return err
	}
	got := TicketRevision(current)
	if got != expected {
		return errRevisionConflict("ticket was updated; reload and retry")
	}
	return nil
}

// guardTicketRevision runs fn under the workspace write lock when a
// precondition is present, so the digest check and the ActionService write
// cannot race. Callers without expected_revision keep the previous API.
func (s *Server) guardTicketRevision(ctx context.Context, r *http.Request, ticketID string, fn func(context.Context)) error {
	expected := revisionFromRequest(r, ticketID)
	if expected == "" {
		fn(ctx)
		return nil
	}
	if s.actions == nil || s.actions.LockManager == nil {
		return errRevisionConflict("ticket was updated; reload and retry")
	}
	return service.WithWriteLock(ctx, s.actions.LockManager, "web revision check", func(locked context.Context) error {
		if err := s.matchTicketRevision(locked, ticketID, expected); err != nil {
			return err
		}
		fn(locked)
		return nil
	})
}

func (s *Server) guardBulkRevisions(ctx context.Context, r *http.Request, ids []string, fn func(context.Context)) error {
	revs := bulkRevisionsFromRequest(r, ids)
	if len(revs) == 0 {
		fn(ctx)
		return nil
	}
	if s.actions == nil || s.actions.LockManager == nil {
		return errRevisionConflict("ticket was updated; reload and retry")
	}
	return service.WithWriteLock(ctx, s.actions.LockManager, "web bulk revision check", func(locked context.Context) error {
		for _, id := range ids {
			expected := revs[id]
			if expected == "" {
				return errRevisionConflict("malformed expected_revision")
			}
			if err := s.matchTicketRevision(locked, id, expected); err != nil {
				return err
			}
		}
		fn(locked)
		return nil
	})
}
