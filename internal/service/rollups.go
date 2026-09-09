package service

import (
	"context"
	"fmt"
	"sort"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

const maxRecentEvents = 20

// ProjectRollup is the welcome-page status summary for one project.
type ProjectRollup struct {
	Project contracts.Project `json:"project"`
	Active  int               `json:"active"`
	Backlog int               `json:"backlog"`
	Done    int               `json:"done"`
	Blocked int               `json:"blocked"`
}

// ProjectRollups counts the visible board columns for every project.
func (s *QueryService) ProjectRollups(ctx context.Context) ([]ProjectRollup, error) {
	projects, err := s.Projects.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	rollups := make([]ProjectRollup, 0, len(projects))
	for _, project := range projects {
		board, err := s.Board(ctx, contracts.BoardQueryOptions{Project: project.Key})
		if err != nil {
			return nil, fmt.Errorf("query board for %s: %w", project.Key, err)
		}
		done, err := s.Tickets.ListTickets(ctx, contracts.TicketListOptions{
			Project:  project.Key,
			Statuses: []contracts.Status{contracts.StatusDone},
		})
		if err != nil {
			return nil, fmt.Errorf("query done tickets for %s: %w", project.Key, err)
		}
		rollups = append(rollups, ProjectRollup{
			Project: project,
			Active: len(board.Board.Columns[contracts.StatusReady]) +
				len(board.Board.Columns[contracts.StatusInProgress]) +
				len(board.Board.Columns[contracts.StatusInReview]),
			Backlog: len(board.Board.Columns[contracts.StatusBacklog]),
			// Only completed work counts as done; cancellation stays separate.
			Done:    len(done),
			Blocked: len(board.Board.Columns[contracts.StatusBlocked]),
		})
	}
	return rollups, nil
}

// RecentEvents returns ticket activity across projects, newest first.
func (s *QueryService) RecentEvents(ctx context.Context, limit int) ([]contracts.Event, error) {
	if limit <= 0 {
		return []contracts.Event{}, nil
	}
	if limit > maxRecentEvents {
		limit = maxRecentEvents
	}
	projects, err := s.Projects.ListProjects(ctx)
	if err != nil {
		return nil, err
	}

	// JSONL has no cross-project index yet. Cache each stream for this request
	// so the full scan happens at most once per project while we build the feed.
	eventsByProject := make(map[string][]contracts.Event, len(projects))
	events := make([]contracts.Event, 0, limit)
	for _, project := range projects {
		projectEvents, ok := eventsByProject[project.Key]
		if !ok {
			projectEvents, err = s.Events.StreamEvents(ctx, project.Key, 0)
			if err != nil {
				return nil, fmt.Errorf("stream events for %s: %w", project.Key, err)
			}
			eventsByProject[project.Key] = projectEvents
		}
		for _, event := range projectEvents {
			if isWelcomeEvent(event.Type) {
				events = append(events, event)
			}
		}
	}

	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Timestamp.Equal(events[j].Timestamp) {
			if events[i].EventID == events[j].EventID {
				if events[i].Project == events[j].Project {
					return events[i].TicketID > events[j].TicketID
				}
				return events[i].Project > events[j].Project
			}
			return events[i].EventID > events[j].EventID
		}
		return events[i].Timestamp.After(events[j].Timestamp)
	})
	if len(events) > limit {
		events = events[:limit]
	}
	return events, nil
}

func isWelcomeEvent(eventType contracts.EventType) bool {
	switch eventType {
	case contracts.EventTicketCreated,
		contracts.EventTicketMoved,
		contracts.EventTicketCommented,
		contracts.EventTicketUpdated:
		return true
	default:
		return false
	}
}
