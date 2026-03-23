package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type ActionService struct {
	Root       string
	Projects   contracts.ProjectStore
	Tickets    contracts.TicketStore
	Events     contracts.EventLog
	Projection contracts.ProjectionStore
	Clock      func() time.Time
}

func NewActionService(root string, projects contracts.ProjectStore, tickets contracts.TicketStore, events contracts.EventLog, projection contracts.ProjectionStore, clock func() time.Time) *ActionService {
	return &ActionService{Root: root, Projects: projects, Tickets: tickets, Events: events, Projection: projection, Clock: clock}
}

func (s *ActionService) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

func (s *ActionService) NextEventID(ctx context.Context, project string) (int64, error) {
	events, err := s.Events.StreamEvents(ctx, project, 0)
	if err != nil {
		return 0, err
	}
	var maxID int64
	for _, event := range events {
		if event.EventID > maxID {
			maxID = event.EventID
		}
	}
	return maxID + 1, nil
}

func (s *ActionService) CreateProject(ctx context.Context, project contracts.Project) error {
	return s.Projects.CreateProject(ctx, contracts.NormalizeProject(project))
}

func (s *ActionService) UpdateProject(ctx context.Context, project contracts.Project) error {
	return s.Projects.UpdateProject(ctx, contracts.NormalizeProject(project))
}

func (s *ActionService) CreateTicket(ctx context.Context, ticket contracts.TicketSnapshot) error {
	return s.Tickets.CreateTicket(ctx, contracts.NormalizeTicketSnapshot(ticket))
}

func (s *ActionService) UpdateTicket(ctx context.Context, ticket contracts.TicketSnapshot) error {
	return s.Tickets.UpdateTicket(ctx, contracts.NormalizeTicketSnapshot(ticket))
}

func (s *ActionService) SoftDeleteTicket(ctx context.Context, id string, actor contracts.Actor, reason string) error {
	return s.Tickets.SoftDeleteTicket(ctx, id, actor, reason)
}

func (s *ActionService) AppendAndProject(ctx context.Context, event contracts.Event) error {
	if err := s.Events.AppendEvent(ctx, event); err != nil {
		return err
	}
	if s.Projection != nil {
		if err := s.Projection.ApplyEvent(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (s *ActionService) ClaimTicket(ctx context.Context, ticketID string, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	if !actor.IsValid() {
		return contracts.TicketSnapshot{}, fmt.Errorf("invalid actor: %s", actor)
	}
	ticket, err := s.Tickets.GetTicket(ctx, ticketID)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}
	if ticket.Lease.Actor != "" && !ticket.Lease.Active(s.now()) {
		if _, err := s.expireLease(ctx, &ticket, "lease expired before claim"); err != nil {
			return contracts.TicketSnapshot{}, err
		}
	}
	if ticket.Lease.Active(s.now()) && ticket.Lease.Actor == actor {
		return ticket, nil
	}
	if ticket.Lease.Active(s.now()) && ticket.Lease.Actor != actor {
		return contracts.TicketSnapshot{}, fmt.Errorf("ticket %s is already claimed by %s", ticket.ID, ticket.Lease.Actor)
	}
	policy, err := resolveEffectivePolicy(ctx, s.Root, s.Projects, s.Tickets, ticket)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}
	if len(policy.AllowedWorkers) > 0 && !actorInList(actor, policy.AllowedWorkers) && actor != contracts.Actor("human:owner") {
		return contracts.TicketSnapshot{}, fmt.Errorf("actor %s is not allowed by effective policy", actor)
	}

	kind := contracts.LeaseKindWork
	if ticket.Status == contracts.StatusInReview {
		if actor != ticket.Reviewer && actor != contracts.Actor("human:owner") {
			return contracts.TicketSnapshot{}, fmt.Errorf("review claims must belong to the reviewer or owner")
		}
		kind = contracts.LeaseKindReview
	}
	now := s.now()
	ticket.Lease = contracts.LeaseState{
		Actor:           actor,
		Kind:            kind,
		AcquiredAt:      now,
		ExpiresAt:       now.Add(policy.LeaseTTL),
		LastHeartbeatAt: now,
	}
	ticket.UpdatedAt = now
	if err := s.UpdateTicket(ctx, ticket); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	eventID, err := s.NextEventID(ctx, ticket.Project)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}
	event := contracts.Event{
		EventID:       eventID,
		Timestamp:     now,
		Actor:         actor,
		Reason:        reason,
		Type:          contracts.EventTicketClaimed,
		Project:       ticket.Project,
		TicketID:      ticket.ID,
		Payload:       ticket,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := s.AppendAndProject(ctx, event); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	return ticket, nil
}

func (s *ActionService) ReleaseTicket(ctx context.Context, ticketID string, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	if !actor.IsValid() {
		return contracts.TicketSnapshot{}, fmt.Errorf("invalid actor: %s", actor)
	}
	ticket, err := s.Tickets.GetTicket(ctx, ticketID)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}
	if ticket.Lease.Actor == "" {
		return contracts.TicketSnapshot{}, fmt.Errorf("ticket %s is not claimed", ticket.ID)
	}
	if ticket.Lease.Actor != actor && actor != contracts.Actor("human:owner") {
		return contracts.TicketSnapshot{}, fmt.Errorf("ticket %s is claimed by %s", ticket.ID, ticket.Lease.Actor)
	}
	now := s.now()
	ticket.Lease = contracts.LeaseState{}
	ticket.UpdatedAt = now
	if err := s.UpdateTicket(ctx, ticket); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	eventID, err := s.NextEventID(ctx, ticket.Project)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}
	event := contracts.Event{
		EventID:       eventID,
		Timestamp:     now,
		Actor:         actor,
		Reason:        reason,
		Type:          contracts.EventTicketReleased,
		Project:       ticket.Project,
		TicketID:      ticket.ID,
		Payload:       ticket,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := s.AppendAndProject(ctx, event); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	return ticket, nil
}

func (s *ActionService) HeartbeatTicket(ctx context.Context, ticketID string, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	if !actor.IsValid() {
		return contracts.TicketSnapshot{}, fmt.Errorf("invalid actor: %s", actor)
	}
	ticket, err := s.Tickets.GetTicket(ctx, ticketID)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}
	if ticket.Lease.Actor == "" || ticket.Lease.Actor != actor || !ticket.Lease.Active(s.now()) {
		return contracts.TicketSnapshot{}, fmt.Errorf("actor %s does not hold an active lease on %s", actor, ticket.ID)
	}
	policy, err := resolveEffectivePolicy(ctx, s.Root, s.Projects, s.Tickets, ticket)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}
	now := s.now()
	ticket.Lease.LastHeartbeatAt = now
	ticket.Lease.ExpiresAt = now.Add(policy.LeaseTTL)
	ticket.UpdatedAt = now
	if err := s.UpdateTicket(ctx, ticket); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	eventID, err := s.NextEventID(ctx, ticket.Project)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}
	event := contracts.Event{
		EventID:       eventID,
		Timestamp:     now,
		Actor:         actor,
		Reason:        reason,
		Type:          contracts.EventTicketHeartbeat,
		Project:       ticket.Project,
		TicketID:      ticket.ID,
		Payload:       ticket,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := s.AppendAndProject(ctx, event); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	return ticket, nil
}

func (s *ActionService) SweepExpiredClaims(ctx context.Context, actor contracts.Actor, reason string) ([]contracts.TicketSnapshot, error) {
	if !actor.IsValid() {
		return nil, fmt.Errorf("invalid actor: %s", actor)
	}
	tickets, err := s.Tickets.ListTickets(ctx, contracts.TicketListOptions{IncludeArchived: false})
	if err != nil {
		return nil, err
	}
	expired := make([]contracts.TicketSnapshot, 0)
	for _, ticket := range tickets {
		if ticket.Lease.Actor == "" || ticket.Lease.Active(s.now()) || ticket.Lease.ExpiresAt.IsZero() {
			continue
		}
		updated, err := s.expireLease(ctx, &ticket, reason)
		if err != nil {
			return nil, err
		}
		expired = append(expired, updated)
	}
	return expired, nil
}

func (s *ActionService) expireLease(ctx context.Context, ticket *contracts.TicketSnapshot, reason string) (contracts.TicketSnapshot, error) {
	now := s.now()
	expiredActor := ticket.Lease.Actor
	ticket.Lease = contracts.LeaseState{}
	ticket.UpdatedAt = now
	if err := s.UpdateTicket(ctx, *ticket); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	eventID, err := s.NextEventID(ctx, ticket.Project)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}
	why := strings.TrimSpace(reason)
	if why == "" {
		why = "lease expired"
	}
	event := contracts.Event{
		EventID:       eventID,
		Timestamp:     now,
		Actor:         expiredActor,
		Reason:        why,
		Type:          contracts.EventTicketLeaseExpired,
		Project:       ticket.Project,
		TicketID:      ticket.ID,
		Payload:       *ticket,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := s.AppendAndProject(ctx, event); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	return *ticket, nil
}
