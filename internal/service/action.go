package service

import (
	"context"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type ActionService struct {
	Projects   contracts.ProjectStore
	Tickets    contracts.TicketStore
	Events     contracts.EventLog
	Projection contracts.ProjectionStore
	Clock      func() time.Time
}

func NewActionService(projects contracts.ProjectStore, tickets contracts.TicketStore, events contracts.EventLog, projection contracts.ProjectionStore, clock func() time.Time) *ActionService {
	return &ActionService{Projects: projects, Tickets: tickets, Events: events, Projection: projection, Clock: clock}
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
