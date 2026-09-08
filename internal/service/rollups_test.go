package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

func TestQueryServiceProjectRollups(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 7, 23, 14, 0, 0, 0, time.UTC)
	projects := mdstore.ProjectStore{RootDir: root}
	tickets := mdstore.TicketStore{RootDir: root}
	events := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), tickets, events)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer projection.Close()

	for _, project := range []contracts.Project{
		{Key: "APP", Name: "App", CreatedAt: now},
		{Key: "OPS", Name: "Operations", CreatedAt: now},
	} {
		if err := projects.CreateProject(ctx, project); err != nil {
			t.Fatalf("create project %s: %v", project.Key, err)
		}
	}
	statuses := []contracts.Status{
		contracts.StatusBacklog,
		contracts.StatusReady,
		contracts.StatusInProgress,
		contracts.StatusInReview,
		contracts.StatusBlocked,
		contracts.StatusDone,
		contracts.StatusCanceled,
	}
	for i, status := range statuses {
		ticket := contracts.TicketSnapshot{
			ID:            "APP-" + string(rune('1'+i)),
			Project:       "APP",
			Title:         string(status),
			Type:          contracts.TicketTypeTask,
			Status:        status,
			Priority:      contracts.PriorityMedium,
			CreatedAt:     now,
			UpdatedAt:     now.Add(time.Duration(i) * time.Minute),
			SchemaVersion: contracts.CurrentSchemaVersion,
		}
		if err := tickets.CreateTicket(ctx, ticket); err != nil {
			t.Fatalf("create ticket %s: %v", ticket.ID, err)
		}
		event := contracts.Event{
			EventID:       int64(i + 1),
			Timestamp:     ticket.UpdatedAt,
			Actor:         contracts.Actor("human:owner"),
			Type:          contracts.EventTicketCreated,
			Project:       ticket.Project,
			TicketID:      ticket.ID,
			Payload:       ticket,
			SchemaVersion: contracts.CurrentSchemaVersion,
		}
		if err := events.AppendEvent(ctx, event); err != nil {
			t.Fatalf("append event: %v", err)
		}
		if err := projection.ApplyEvent(ctx, event); err != nil {
			t.Fatalf("apply event: %v", err)
		}
	}

	queries := NewQueryService(root, projects, tickets, events, projection, nil)
	rollups, err := queries.ProjectRollups(ctx)
	if err != nil {
		t.Fatalf("project rollups: %v", err)
	}
	if len(rollups) != 2 {
		t.Fatalf("expected two project rollups, got %#v", rollups)
	}
	app := rollups[0]
	if app.Project.Key != "APP" || app.Active != 3 || app.Backlog != 1 || app.Done != 1 || app.Blocked != 1 {
		t.Fatalf("unexpected APP rollup: %#v", app)
	}
	if rollups[1].Project.Key != "OPS" || rollups[1].Active != 0 {
		t.Fatalf("unexpected empty OPS rollup: %#v", rollups[1])
	}
}

func TestQueryServiceRecentEventsMergesFiltersAndCaps(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 7, 23, 15, 0, 0, 0, time.UTC)
	projects := mdstore.ProjectStore{RootDir: root}
	events := &eventstore.Log{RootDir: root}
	for _, project := range []contracts.Project{
		{Key: "APP", Name: "App", CreatedAt: now},
		{Key: "OPS", Name: "Operations", CreatedAt: now},
	} {
		if err := projects.CreateProject(ctx, project); err != nil {
			t.Fatalf("create project %s: %v", project.Key, err)
		}
	}
	for i := 1; i <= 24; i++ {
		project := "APP"
		if i%2 == 0 {
			project = "OPS"
		}
		eventType := contracts.EventTicketUpdated
		if i == 3 {
			eventType = contracts.EventTicketLinked
		}
		event := contracts.Event{
			EventID:       int64((i + 1) / 2),
			Timestamp:     now.Add(time.Duration(i) * time.Minute),
			Actor:         contracts.Actor("human:owner"),
			Type:          eventType,
			Project:       project,
			TicketID:      project + "-1",
			Payload:       map[string]any{"from": "ready", "to": "in_progress"},
			SchemaVersion: contracts.CurrentSchemaVersion,
		}
		if err := events.AppendEvent(ctx, event); err != nil {
			t.Fatalf("append event %d: %v", i, err)
		}
	}
	queries := &QueryService{Projects: projects, Events: events}
	recent, err := queries.RecentEvents(ctx, 99)
	if err != nil {
		t.Fatalf("recent events: %v", err)
	}
	if len(recent) != maxRecentEvents {
		t.Fatalf("expected cap of %d, got %d", maxRecentEvents, len(recent))
	}
	if recent[0].Timestamp != now.Add(24*time.Minute) {
		t.Fatalf("expected newest event first, got %#v", recent[0])
	}
	for _, event := range recent {
		if event.Type == contracts.EventTicketLinked {
			t.Fatalf("unsupported event leaked into recent feed: %#v", event)
		}
	}

	tiedAt := now.Add(30 * time.Minute)
	for _, event := range []contracts.Event{
		{EventID: 20, Timestamp: tiedAt, Actor: contracts.Actor("human:owner"), Type: contracts.EventTicketMoved, Project: "APP", TicketID: "APP-2", Payload: map[string]any{"from": "ready", "to": "done"}, SchemaVersion: contracts.CurrentSchemaVersion},
		{EventID: 20, Timestamp: tiedAt, Actor: contracts.Actor("human:owner"), Type: contracts.EventTicketCommented, Project: "OPS", TicketID: "OPS-2", Payload: map[string]any{"body": "done"}, SchemaVersion: contracts.CurrentSchemaVersion},
	} {
		if err := events.AppendEvent(ctx, event); err != nil {
			t.Fatalf("append tied event: %v", err)
		}
	}
	recent, err = queries.RecentEvents(ctx, 2)
	if err != nil {
		t.Fatalf("recent tied events: %v", err)
	}
	if len(recent) != 2 || recent[0].Project != "OPS" || recent[1].Project != "APP" {
		t.Fatalf("unexpected tie ordering: %#v", recent)
	}
}
