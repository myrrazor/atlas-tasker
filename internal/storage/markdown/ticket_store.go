package markdown

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

// TicketStore persists ticket markdown snapshots.
type TicketStore struct {
	RootDir string
	Clock   func() time.Time
}

func (s TicketStore) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

func (s TicketStore) CreateTicket(_ context.Context, ticket contracts.TicketSnapshot) error {
	ticket = contracts.NormalizeTicketSnapshot(ticket)
	if err := ticket.ValidateForCreate(); err != nil {
		return err
	}
	path := storage.TicketFile(s.RootDir, ticket.Project, ticket.ID)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("ticket %s already exists", ticket.ID)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create ticket dir: %w", err)
	}
	content, err := EncodeTicketMarkdown(ticket)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write ticket: %w", err)
	}
	return nil
}

func (s TicketStore) GetTicket(_ context.Context, id string) (contracts.TicketSnapshot, error) {
	matches, err := filepath.Glob(filepath.Join(storage.ProjectsDir(s.RootDir), "*", "tickets", id+".md"))
	if err != nil {
		return contracts.TicketSnapshot{}, fmt.Errorf("glob ticket: %w", err)
	}
	if len(matches) == 0 {
		return contracts.TicketSnapshot{}, fmt.Errorf("ticket %s not found", id)
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		return contracts.TicketSnapshot{}, fmt.Errorf("read ticket %s: %w", id, err)
	}
	ticket, err := DecodeTicketMarkdown(string(raw))
	if err != nil {
		return contracts.TicketSnapshot{}, fmt.Errorf("decode ticket %s: %w", id, err)
	}
	return contracts.NormalizeTicketSnapshot(ticket), nil
}

func (s TicketStore) UpdateTicket(_ context.Context, ticket contracts.TicketSnapshot) error {
	ticket = contracts.NormalizeTicketSnapshot(ticket)
	if strings.TrimSpace(ticket.ID) == "" {
		return fmt.Errorf("ticket id is required")
	}
	if strings.TrimSpace(ticket.Project) == "" {
		return fmt.Errorf("ticket project is required")
	}
	path := storage.TicketFile(s.RootDir, ticket.Project, ticket.ID)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("ticket %s not found: %w", ticket.ID, err)
	}
	content, err := EncodeTicketMarkdown(ticket)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write ticket %s: %w", ticket.ID, err)
	}
	return nil
}

func (s TicketStore) ListTickets(_ context.Context, opts contracts.TicketListOptions) ([]contracts.TicketSnapshot, error) {
	matches, err := filepath.Glob(s.ticketGlob(opts.Project))
	if err != nil {
		return nil, fmt.Errorf("glob tickets: %w", err)
	}

	tickets := make([]contracts.TicketSnapshot, 0, len(matches))
	for _, match := range matches {
		raw, err := os.ReadFile(match)
		if err != nil {
			return nil, err
		}
		ticket, err := DecodeTicketMarkdown(string(raw))
		if err != nil {
			return nil, err
		}
		ticket = contracts.NormalizeTicketSnapshot(ticket)
		if !opts.IncludeArchived && ticket.Archived {
			continue
		}
		if opts.Assignee != "" && ticket.Assignee != opts.Assignee {
			continue
		}
		if opts.Type != "" && ticket.Type != opts.Type {
			continue
		}
		if opts.Label != "" && !containsString(ticket.Labels, opts.Label) {
			continue
		}
		if len(opts.Statuses) > 0 && !containsStatus(opts.Statuses, ticket.Status) {
			continue
		}
		tickets = append(tickets, ticket)
	}

	sort.Slice(tickets, func(i, j int) bool {
		if tickets[i].UpdatedAt.Equal(tickets[j].UpdatedAt) {
			return tickets[i].ID < tickets[j].ID
		}
		return tickets[i].UpdatedAt.Before(tickets[j].UpdatedAt)
	})

	if opts.Limit > 0 && len(tickets) > opts.Limit {
		return tickets[:opts.Limit], nil
	}
	return tickets, nil
}

func (s TicketStore) SoftDeleteTicket(ctx context.Context, id string, actor contracts.Actor, reason string) error {
	if !actor.IsValid() {
		return fmt.Errorf("invalid actor: %s", actor)
	}
	ticket, err := s.GetTicket(ctx, id)
	if err != nil {
		return err
	}
	ticket.Status = contracts.StatusCanceled
	ticket.Archived = true
	timestamp := s.now()
	ticket.UpdatedAt = timestamp
	auditLine := fmt.Sprintf("Archived by %s at %s", actor, timestamp.Format(time.RFC3339))
	if strings.TrimSpace(reason) != "" {
		auditLine = auditLine + " — " + strings.TrimSpace(reason)
	}
	if strings.TrimSpace(ticket.Notes) == "" {
		ticket.Notes = auditLine
	} else {
		ticket.Notes = strings.TrimSpace(ticket.Notes) + "\n\n" + auditLine
	}
	return s.UpdateTicket(ctx, ticket)
}

func (s TicketStore) ticketGlob(project string) string {
	if project != "" {
		return filepath.Join(storage.TicketsDir(s.RootDir, project), "*.md")
	}
	return filepath.Join(storage.ProjectsDir(s.RootDir), "*", "tickets", "*.md")
}

func containsString(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func containsStatus(values []contracts.Status, candidate contracts.Status) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
