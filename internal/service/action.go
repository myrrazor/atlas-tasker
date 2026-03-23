package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/domain"
)

type ActionService struct {
	Root       string
	Projects   contracts.ProjectStore
	Tickets    contracts.TicketStore
	Events     contracts.EventLog
	Projection contracts.ProjectionStore
	Clock      func() time.Time
	Notifier   Notifier
}

func NewActionService(root string, projects contracts.ProjectStore, tickets contracts.TicketStore, events contracts.EventLog, projection contracts.ProjectionStore, clock func() time.Time, notifier Notifier) *ActionService {
	return &ActionService{Root: root, Projects: projects, Tickets: tickets, Events: events, Projection: projection, Clock: clock, Notifier: notifier}
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
	if s.Notifier != nil {
		if err := s.Notifier.Notify(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (s *ActionService) AllocateTicketID(ctx context.Context, project string) (string, error) {
	tickets, err := s.Tickets.ListTickets(ctx, contracts.TicketListOptions{Project: strings.TrimSpace(project), IncludeArchived: true})
	if err != nil {
		return "", err
	}
	max := 0
	prefix := strings.TrimSpace(project) + "-"
	for _, ticket := range tickets {
		if !strings.HasPrefix(ticket.ID, prefix) {
			continue
		}
		raw := strings.TrimPrefix(ticket.ID, prefix)
		n, err := strconv.Atoi(raw)
		if err == nil && n > max {
			max = n
		}
	}
	return fmt.Sprintf("%s-%d", strings.TrimSpace(project), max+1), nil
}

func (s *ActionService) CreateTrackedTicket(ctx context.Context, ticket contracts.TicketSnapshot, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	if !actor.IsValid() {
		return contracts.TicketSnapshot{}, fmt.Errorf("invalid actor: %s", actor)
	}
	if _, err := s.Projects.GetProject(ctx, ticket.Project); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	normalized := contracts.NormalizeTicketSnapshot(ticket)
	if strings.TrimSpace(normalized.ID) == "" {
		id, err := s.AllocateTicketID(ctx, normalized.Project)
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
		normalized.ID = id
	}
	if normalized.SchemaVersion == 0 {
		normalized.SchemaVersion = contracts.CurrentSchemaVersion
	}
	if err := s.CreateTicket(ctx, normalized); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	eventID, err := s.NextEventID(ctx, normalized.Project)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}
	event := contracts.Event{
		EventID:       eventID,
		Timestamp:     normalized.UpdatedAt,
		Actor:         actor,
		Reason:        reason,
		Type:          contracts.EventTicketCreated,
		Project:       normalized.Project,
		TicketID:      normalized.ID,
		Payload:       normalized,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := s.AppendAndProject(ctx, event); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	return normalized, nil
}

func (s *ActionService) SaveTrackedTicket(ctx context.Context, ticket contracts.TicketSnapshot, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	if !actor.IsValid() {
		return contracts.TicketSnapshot{}, fmt.Errorf("invalid actor: %s", actor)
	}
	normalized := contracts.NormalizeTicketSnapshot(ticket)
	if err := s.UpdateTicket(ctx, normalized); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	eventID, err := s.NextEventID(ctx, normalized.Project)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}
	event := contracts.Event{
		EventID:       eventID,
		Timestamp:     normalized.UpdatedAt,
		Actor:         actor,
		Reason:        reason,
		Type:          contracts.EventTicketUpdated,
		Project:       normalized.Project,
		TicketID:      normalized.ID,
		Payload:       normalized,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := s.AppendAndProject(ctx, event); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	return normalized, nil
}

func (s *ActionService) AssignTicket(ctx context.Context, ticketID string, assignee contracts.Actor, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	if !actor.IsValid() {
		return contracts.TicketSnapshot{}, fmt.Errorf("invalid actor: %s", actor)
	}
	if assignee != "" && !assignee.IsValid() {
		return contracts.TicketSnapshot{}, fmt.Errorf("invalid assignee actor: %s", assignee)
	}
	ticket, err := s.Tickets.GetTicket(ctx, ticketID)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}
	ticket.Assignee = assignee
	ticket.UpdatedAt = s.now()
	return s.SaveTrackedTicket(ctx, ticket, actor, reason)
}

func (s *ActionService) CommentTicket(ctx context.Context, ticketID string, body string, actor contracts.Actor, reason string) error {
	if !actor.IsValid() {
		return fmt.Errorf("invalid actor: %s", actor)
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return fmt.Errorf("comment body is required in v1 non-interactive mode")
	}
	ticket, err := s.Tickets.GetTicket(ctx, ticketID)
	if err != nil {
		return err
	}
	now := s.now()
	eventID, err := s.NextEventID(ctx, ticket.Project)
	if err != nil {
		return err
	}
	event := contracts.Event{
		EventID:       eventID,
		Timestamp:     now,
		Actor:         actor,
		Reason:        reason,
		Type:          contracts.EventTicketCommented,
		Project:       ticket.Project,
		TicketID:      ticket.ID,
		Payload:       map[string]any{"body": body},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	return s.AppendAndProject(ctx, event)
}

func (s *ActionService) LinkTickets(ctx context.Context, id string, otherID string, kind domain.LinkKind, actor contracts.Actor, reason string) (contracts.Event, error) {
	if !actor.IsValid() {
		return contracts.Event{}, fmt.Errorf("invalid actor: %s", actor)
	}
	mapped, err := s.loadTicketsMap(ctx)
	if err != nil {
		return contracts.Event{}, err
	}
	if err := domain.ApplyLink(mapped, id, otherID, kind); err != nil {
		return contracts.Event{}, err
	}
	now := s.now()
	trimmedOther := strings.TrimSpace(otherID)
	for _, ticketID := range []string{strings.TrimSpace(id), trimmedOther} {
		ticket := mapped[ticketID]
		ticket.UpdatedAt = now
		if err := s.UpdateTicket(ctx, ticket); err != nil {
			return contracts.Event{}, err
		}
	}
	eventID, err := s.NextEventID(ctx, mapped[strings.TrimSpace(id)].Project)
	if err != nil {
		return contracts.Event{}, err
	}
	event := contracts.Event{
		EventID:   eventID,
		Timestamp: now,
		Actor:     actor,
		Reason:    reason,
		Type:      contracts.EventTicketLinked,
		Project:   mapped[strings.TrimSpace(id)].Project,
		TicketID:  strings.TrimSpace(id),
		Payload: map[string]any{
			"id":           strings.TrimSpace(id),
			"other_id":     trimmedOther,
			"kind":         kind,
			"ticket":       mapped[strings.TrimSpace(id)],
			"other_ticket": mapped[trimmedOther],
		},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := s.AppendAndProject(ctx, event); err != nil {
		return contracts.Event{}, err
	}
	return event, nil
}

func (s *ActionService) UnlinkTickets(ctx context.Context, id string, otherID string, actor contracts.Actor, reason string) (contracts.Event, error) {
	if !actor.IsValid() {
		return contracts.Event{}, fmt.Errorf("invalid actor: %s", actor)
	}
	mapped, err := s.loadTicketsMap(ctx)
	if err != nil {
		return contracts.Event{}, err
	}
	if err := domain.RemoveLink(mapped, id, otherID); err != nil {
		return contracts.Event{}, err
	}
	now := s.now()
	trimmedID := strings.TrimSpace(id)
	trimmedOther := strings.TrimSpace(otherID)
	for _, ticketID := range []string{trimmedID, trimmedOther} {
		ticket := mapped[ticketID]
		ticket.UpdatedAt = now
		if err := s.UpdateTicket(ctx, ticket); err != nil {
			return contracts.Event{}, err
		}
	}
	eventID, err := s.NextEventID(ctx, mapped[trimmedID].Project)
	if err != nil {
		return contracts.Event{}, err
	}
	event := contracts.Event{
		EventID:   eventID,
		Timestamp: now,
		Actor:     actor,
		Reason:    reason,
		Type:      contracts.EventTicketUnlinked,
		Project:   mapped[trimmedID].Project,
		TicketID:  trimmedID,
		Payload: map[string]any{
			"id":           trimmedID,
			"other_id":     trimmedOther,
			"ticket":       mapped[trimmedID],
			"other_ticket": mapped[trimmedOther],
		},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := s.AppendAndProject(ctx, event); err != nil {
		return contracts.Event{}, err
	}
	return event, nil
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

func (s *ActionService) MoveTicket(ctx context.Context, ticketID string, to contracts.Status, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	if !actor.IsValid() {
		return contracts.TicketSnapshot{}, fmt.Errorf("invalid actor: %s", actor)
	}
	ticket, err := s.Tickets.GetTicket(ctx, ticketID)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}
	if to == contracts.StatusDone {
		return s.CompleteTicket(ctx, ticketID, actor, reason)
	}
	policy, err := resolveEffectivePolicy(ctx, s.Root, s.Projects, s.Tickets, ticket)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}
	if err := domain.ValidateTransition(ticket.Status, to); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	now := s.now()
	from := ticket.Status
	priorReviewState := ticket.ReviewState
	ticket.Status = to
	if to == contracts.StatusInProgress && from == contracts.StatusInReview && priorReviewState == contracts.ReviewStateApproved && policy.CompletionMode != contracts.CompletionModeReviewGate {
		ticket.ReviewState = contracts.ReviewStateChangesRequested
	} else if to != contracts.StatusInReview {
		ticket.ReviewState = contracts.ReviewStateNone
	}
	if to == contracts.StatusInReview && priorReviewState == contracts.ReviewStateNone {
		ticket.ReviewState = contracts.ReviewStatePending
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
		Type:          contracts.EventTicketMoved,
		Project:       ticket.Project,
		TicketID:      ticket.ID,
		Payload:       map[string]any{"from": from, "to": to, "ticket": ticket},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := s.AppendAndProject(ctx, event); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	return ticket, nil
}

func (s *ActionService) RequestReview(ctx context.Context, ticketID string, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	if !actor.IsValid() {
		return contracts.TicketSnapshot{}, fmt.Errorf("invalid actor: %s", actor)
	}
	ticket, err := s.Tickets.GetTicket(ctx, ticketID)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}
	if err := domain.ValidateTransition(ticket.Status, contracts.StatusInReview); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	now := s.now()
	ticket.Status = contracts.StatusInReview
	ticket.ReviewState = contracts.ReviewStatePending
	if ticket.Lease.Kind == contracts.LeaseKindWork {
		ticket.Lease = contracts.LeaseState{}
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
		Type:          contracts.EventTicketReviewRequested,
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

func (s *ActionService) ApproveTicket(ctx context.Context, ticketID string, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	if !actor.IsValid() {
		return contracts.TicketSnapshot{}, fmt.Errorf("invalid actor: %s", actor)
	}
	ticket, err := s.Tickets.GetTicket(ctx, ticketID)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}
	if ticket.Status != contracts.StatusInReview {
		return contracts.TicketSnapshot{}, fmt.Errorf("ticket %s is not in review", ticket.ID)
	}
	if actor != contracts.Actor("human:owner") && actor != ticket.Reviewer {
		return contracts.TicketSnapshot{}, fmt.Errorf("only the assigned reviewer or human:owner can approve")
	}
	policy, err := resolveEffectivePolicy(ctx, s.Root, s.Projects, s.Tickets, ticket)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}
	now := s.now()
	ticket.ReviewState = contracts.ReviewStateApproved
	ticket.UpdatedAt = now
	if policy.CompletionMode == contracts.CompletionModeReviewGate {
		ticket.Status = contracts.StatusDone
		ticket.Lease = contracts.LeaseState{}
	}
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
		Type:          contracts.EventTicketApproved,
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

func (s *ActionService) RejectTicket(ctx context.Context, ticketID string, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	if !actor.IsValid() {
		return contracts.TicketSnapshot{}, fmt.Errorf("invalid actor: %s", actor)
	}
	if strings.TrimSpace(reason) == "" {
		return contracts.TicketSnapshot{}, fmt.Errorf("reject requires a reason")
	}
	ticket, err := s.Tickets.GetTicket(ctx, ticketID)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}
	if ticket.Status != contracts.StatusInReview {
		return contracts.TicketSnapshot{}, fmt.Errorf("ticket %s is not in review", ticket.ID)
	}
	if actor != contracts.Actor("human:owner") && actor != ticket.Reviewer {
		return contracts.TicketSnapshot{}, fmt.Errorf("only the assigned reviewer or human:owner can reject")
	}
	now := s.now()
	ticket.Status = contracts.StatusInProgress
	ticket.ReviewState = contracts.ReviewStateChangesRequested
	if ticket.Lease.Kind == contracts.LeaseKindReview {
		ticket.Lease = contracts.LeaseState{}
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
		Type:          contracts.EventTicketRejected,
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

func (s *ActionService) CompleteTicket(ctx context.Context, ticketID string, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	if !actor.IsValid() {
		return contracts.TicketSnapshot{}, fmt.Errorf("invalid actor: %s", actor)
	}
	ticket, err := s.Tickets.GetTicket(ctx, ticketID)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}
	if ticket.Status != contracts.StatusInReview {
		return contracts.TicketSnapshot{}, fmt.Errorf("ticket %s must be in_review to complete", ticket.ID)
	}
	policy, err := resolveEffectivePolicy(ctx, s.Root, s.Projects, s.Tickets, ticket)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}
	if ticket.ReviewState != contracts.ReviewStateApproved {
		return contracts.TicketSnapshot{}, fmt.Errorf("ticket %s must be approved before completion", ticket.ID)
	}
	if err := domain.CheckCompletionPermission(policy.CompletionMode, actor, ticket.Reviewer); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	now := s.now()
	from := ticket.Status
	ticket.Status = contracts.StatusDone
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
		Type:          contracts.EventTicketMoved,
		Project:       ticket.Project,
		TicketID:      ticket.ID,
		Payload:       map[string]any{"from": from, "to": contracts.StatusDone, "ticket": ticket},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := s.AppendAndProject(ctx, event); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	return ticket, nil
}

func (s *ActionService) GetTicketPolicy(ctx context.Context, ticketID string) (contracts.TicketPolicy, EffectivePolicyView, error) {
	ticket, err := s.Tickets.GetTicket(ctx, ticketID)
	if err != nil {
		return contracts.TicketPolicy{}, EffectivePolicyView{}, err
	}
	effective, err := resolveEffectivePolicy(ctx, s.Root, s.Projects, s.Tickets, ticket)
	if err != nil {
		return contracts.TicketPolicy{}, EffectivePolicyView{}, err
	}
	return ticket.Policy, effective, nil
}

func (s *ActionService) SetTicketPolicy(ctx context.Context, ticketID string, policy contracts.TicketPolicy, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	if !actor.IsValid() {
		return contracts.TicketSnapshot{}, fmt.Errorf("invalid actor: %s", actor)
	}
	if err := policy.Validate(); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	ticket, err := s.Tickets.GetTicket(ctx, ticketID)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}
	ticket.Policy = policy
	ticket.UpdatedAt = s.now()
	if err := s.UpdateTicket(ctx, ticket); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	eventID, err := s.NextEventID(ctx, ticket.Project)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}
	event := contracts.Event{
		EventID:       eventID,
		Timestamp:     ticket.UpdatedAt,
		Actor:         actor,
		Reason:        reason,
		Type:          contracts.EventTicketPolicyUpdated,
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

func (s *ActionService) GetProjectPolicy(ctx context.Context, key string) (contracts.ProjectDefaults, error) {
	project, err := s.Projects.GetProject(ctx, key)
	if err != nil {
		return contracts.ProjectDefaults{}, err
	}
	return project.Defaults, nil
}

func (s *ActionService) SetProjectPolicy(ctx context.Context, key string, defaults contracts.ProjectDefaults, actor contracts.Actor, reason string) (contracts.Project, error) {
	if !actor.IsValid() {
		return contracts.Project{}, fmt.Errorf("invalid actor: %s", actor)
	}
	if err := defaults.Validate(); err != nil {
		return contracts.Project{}, err
	}
	project, err := s.Projects.GetProject(ctx, key)
	if err != nil {
		return contracts.Project{}, err
	}
	project.Defaults = defaults
	if err := s.UpdateProject(ctx, project); err != nil {
		return contracts.Project{}, err
	}
	eventID, err := s.NextEventID(ctx, key)
	if err != nil {
		return contracts.Project{}, err
	}
	event := contracts.Event{
		EventID:       eventID,
		Timestamp:     s.now(),
		Actor:         actor,
		Reason:        reason,
		Type:          contracts.EventProjectPolicyUpdated,
		Project:       key,
		Payload:       project,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := s.AppendAndProject(ctx, event); err != nil {
		return contracts.Project{}, err
	}
	return project, nil
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

func (s *ActionService) loadTicketsMap(ctx context.Context) (map[string]contracts.TicketSnapshot, error) {
	tickets, err := s.Tickets.ListTickets(ctx, contracts.TicketListOptions{IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	mapped := make(map[string]contracts.TicketSnapshot, len(tickets))
	for _, ticket := range tickets {
		mapped[ticket.ID] = ticket
	}
	return mapped, nil
}
