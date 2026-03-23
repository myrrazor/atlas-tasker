package service

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type QueryService struct {
	Root       string
	Projects   contracts.ProjectStore
	Tickets    contracts.TicketStore
	Events     contracts.EventLog
	Projection contracts.ProjectionStore
	Clock      func() time.Time
}

func NewQueryService(root string, projects contracts.ProjectStore, tickets contracts.TicketStore, events contracts.EventLog, projection contracts.ProjectionStore, clock func() time.Time) *QueryService {
	return &QueryService{Root: root, Projects: projects, Tickets: tickets, Events: events, Projection: projection, Clock: clock}
}

func (s *QueryService) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

func (s *QueryService) Board(ctx context.Context, opts contracts.BoardQueryOptions) (BoardViewModel, error) {
	board, err := s.Projection.QueryBoard(ctx, opts)
	if err != nil {
		return BoardViewModel{}, err
	}
	return BoardViewModel{Board: board}, nil
}

func (s *QueryService) Search(ctx context.Context, query contracts.SearchQuery) ([]contracts.TicketSnapshot, error) {
	return s.Projection.QuerySearch(ctx, query)
}

func (s *QueryService) History(ctx context.Context, ticketID string) (HistoryView, error) {
	events, err := s.Projection.QueryHistory(ctx, ticketID)
	if err != nil {
		return HistoryView{}, err
	}
	return HistoryView{TicketID: ticketID, Events: events}, nil
}

func (s *QueryService) TicketDetail(ctx context.Context, ticketID string) (TicketDetailView, error) {
	ticket, err := s.Projection.QueryTicket(ctx, ticketID)
	if err != nil {
		ticket, err = s.Tickets.GetTicket(ctx, ticketID)
		if err != nil {
			return TicketDetailView{}, err
		}
	}
	history, err := s.History(ctx, ticketID)
	if err != nil {
		return TicketDetailView{}, err
	}
	comments := make([]string, 0)
	for _, event := range history.Events {
		if event.Type != contracts.EventTicketCommented {
			continue
		}
		if payloadMap, ok := event.Payload.(map[string]any); ok {
			if body, ok := payloadMap["body"].(string); ok && strings.TrimSpace(body) != "" {
				comments = append(comments, strings.TrimSpace(body))
			}
		}
	}
	policy, err := s.EffectivePolicy(ctx, ticket)
	if err != nil {
		return TicketDetailView{}, err
	}
	return TicketDetailView{Ticket: ticket, Comments: comments, History: history.Events, EffectivePolicy: policy}, nil
}

func (s *QueryService) EffectivePolicy(ctx context.Context, ticket contracts.TicketSnapshot) (EffectivePolicyView, error) {
	return resolveEffectivePolicy(ctx, s.Root, s.Projects, s.Tickets, ticket)
}

func (s *QueryService) Queue(ctx context.Context, actor contracts.Actor) (QueueView, error) {
	if actor == "" {
		resolved, err := s.ResolveActor(ctx, "")
		if err != nil {
			return QueueView{}, err
		}
		actor = resolved
	}
	tickets, err := s.Tickets.ListTickets(ctx, contracts.TicketListOptions{IncludeArchived: false})
	if err != nil {
		return QueueView{}, err
	}
	now := s.now()
	view := QueueView{Actor: actor, GeneratedAt: now, Categories: map[QueueCategory][]QueueEntry{}}
	for _, ticket := range tickets {
		policy, err := s.EffectivePolicy(ctx, ticket)
		if err != nil {
			return QueueView{}, err
		}
		if len(policy.AllowedWorkers) > 0 && !actorInList(actor, policy.AllowedWorkers) {
			view.Categories[QueuePolicyViolations] = append(view.Categories[QueuePolicyViolations], QueueEntry{Ticket: ticket, Reason: "actor not allowed by effective policy"})
		}
		switch {
		case ticket.Lease.Actor != "" && !ticket.Lease.ExpiresAt.IsZero() && !ticket.Lease.Active(now):
			view.Categories[QueueStaleClaims] = append(view.Categories[QueueStaleClaims], QueueEntry{Ticket: ticket, Reason: "lease expired"})
		case ticket.Lease.Active(now) && ticket.Lease.Actor == actor:
			view.Categories[QueueClaimedByMe] = append(view.Categories[QueueClaimedByMe], QueueEntry{Ticket: ticket, Reason: "active lease owned by actor"})
		case ticket.Status == contracts.StatusReady && (ticket.Assignee == "" || ticket.Assignee == actor):
			view.Categories[QueueReadyForMe] = append(view.Categories[QueueReadyForMe], QueueEntry{Ticket: ticket, Reason: "ready and assignable"})
		case contracts.BoardStatus(ticket) == contracts.StatusBlocked && (ticket.Assignee == "" || ticket.Assignee == actor):
			view.Categories[QueueBlockedForMe] = append(view.Categories[QueueBlockedForMe], QueueEntry{Ticket: ticket, Reason: "ticket is blocked"})
		}
		if ticket.Status == contracts.StatusInReview && (ticket.Reviewer == actor || actor == contracts.Actor("human:owner")) {
			view.Categories[QueueNeedsReview] = append(view.Categories[QueueNeedsReview], QueueEntry{Ticket: ticket, Reason: "waiting for review"})
		}
		if policy.CompletionMode == contracts.CompletionModeDualGate && ticket.ReviewState == contracts.ReviewStateApproved {
			view.Categories[QueueAwaitingOwner] = append(view.Categories[QueueAwaitingOwner], QueueEntry{Ticket: ticket, Reason: "approved and waiting for owner completion"})
		}
	}
	for category := range view.Categories {
		sortQueueEntries(view.Categories[category])
	}
	return view, nil
}

func (s *QueryService) Who(ctx context.Context) ([]contracts.TicketSnapshot, error) {
	tickets, err := s.Tickets.ListTickets(ctx, contracts.TicketListOptions{IncludeArchived: false})
	if err != nil {
		return nil, err
	}
	now := s.now()
	active := make([]contracts.TicketSnapshot, 0)
	for _, ticket := range tickets {
		if ticket.Lease.Actor == "" {
			continue
		}
		if ticket.Lease.Active(now) || (!ticket.Lease.ExpiresAt.IsZero() && ticket.Lease.ExpiresAt.Before(now)) {
			active = append(active, ticket)
		}
	}
	sort.Slice(active, func(i, j int) bool {
		if active[i].Lease.ExpiresAt.Equal(active[j].Lease.ExpiresAt) {
			return active[i].ID < active[j].ID
		}
		if active[i].Lease.ExpiresAt.IsZero() {
			return false
		}
		if active[j].Lease.ExpiresAt.IsZero() {
			return true
		}
		return active[i].Lease.ExpiresAt.Before(active[j].Lease.ExpiresAt)
	})
	return active, nil
}

func (s *QueryService) ResolveActor(ctx context.Context, explicit contracts.Actor) (contracts.Actor, error) {
	if explicit != "" {
		if !explicit.IsValid() {
			return "", fmt.Errorf("invalid actor: %s", explicit)
		}
		return explicit, nil
	}
	if envActor := strings.TrimSpace(os.Getenv("TRACKER_ACTOR")); envActor != "" {
		actor := contracts.Actor(envActor)
		if !actor.IsValid() {
			return "", fmt.Errorf("invalid TRACKER_ACTOR: %s", envActor)
		}
		return actor, nil
	}
	cfg, err := config.Load(s.Root)
	if err != nil {
		return "", err
	}
	if cfg.Actor.Default != "" {
		return cfg.Actor.Default, nil
	}
	return "", fmt.Errorf("actor is required: pass --actor, set TRACKER_ACTOR, or configure actor.default")
}

func applyPolicy(view *EffectivePolicyView, policy contracts.TicketPolicy) {
	if policy.CompletionMode != "" {
		view.CompletionMode = policy.CompletionMode
	}
	if len(policy.AllowedWorkers) > 0 {
		view.AllowedWorkers = append([]contracts.Actor{}, policy.AllowedWorkers...)
	}
	if policy.RequiredReviewer != "" {
		view.RequiredReviewer = policy.RequiredReviewer
	}
}

func actorInList(actor contracts.Actor, values []contracts.Actor) bool {
	for _, value := range values {
		if value == actor {
			return true
		}
	}
	return false
}

func sortQueueEntries(entries []QueueEntry) {
	priorityRank := map[contracts.Priority]int{
		contracts.PriorityCritical: 4,
		contracts.PriorityHigh:     3,
		contracts.PriorityMedium:   2,
		contracts.PriorityLow:      1,
	}
	sort.Slice(entries, func(i, j int) bool {
		left := priorityRank[entries[i].Ticket.Priority]
		right := priorityRank[entries[j].Ticket.Priority]
		if left != right {
			return left > right
		}
		if !entries[i].Ticket.UpdatedAt.Equal(entries[j].Ticket.UpdatedAt) {
			return entries[i].Ticket.UpdatedAt.Before(entries[j].Ticket.UpdatedAt)
		}
		return entries[i].Ticket.ID < entries[j].Ticket.ID
	})
}
