package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

func TestRepairWorkspaceReplaysPendingMutationAndClearsJournal(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: clock}
	eventLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer projection.Close()
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket := contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Replay me",
		Summary:       "Replay me",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket snapshot: %v", err)
	}
	event := contracts.Event{
		EventID:       1,
		Timestamp:     now,
		Actor:         contracts.Actor("human:owner"),
		Reason:        "seed",
		Type:          contracts.EventTicketCreated,
		Project:       "APP",
		TicketID:      ticket.ID,
		Payload:       ticket,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	journal := MutationJournal{Root: root, Clock: clock}
	entry, err := journal.Begin("create tracked ticket", "ticket_snapshot", event)
	if err != nil {
		t.Fatalf("begin journal: %v", err)
	}
	if _, err := journal.Mark(entry, MutationStageCanonicalWritten, ""); err != nil {
		t.Fatalf("mark canonical written: %v", err)
	}

	report, err := RepairWorkspace(ctx, root, clock, eventLog, projection)
	if err != nil {
		t.Fatalf("repair workspace: %v", err)
	}
	if report.Pending != 1 {
		t.Fatalf("expected one pending journal, got %d", report.Pending)
	}
	if len(report.Actions) != 2 {
		t.Fatalf("expected replay + rebuild actions, got %#v", report.Actions)
	}
	history, err := projection.QueryHistory(ctx, ticket.ID)
	if err != nil {
		t.Fatalf("query history: %v", err)
	}
	if len(history) != 1 || history[0].Type != contracts.EventTicketCreated {
		t.Fatalf("unexpected history after repair: %#v", history)
	}
	entries, err := journal.List()
	if err != nil {
		t.Fatalf("list journals: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected journal cleanup, got %#v", entries)
	}
}

func TestRepairWorkspaceRebuildsProjectionWithoutPendingJournal(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 24, 11, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: clock}
	eventLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer projection.Close()
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket := contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Projection only",
		Summary:       "Projection only",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket snapshot: %v", err)
	}
	if err := eventLog.AppendEvent(ctx, contracts.Event{
		EventID:       1,
		Timestamp:     now,
		Actor:         contracts.Actor("human:owner"),
		Type:          contracts.EventTicketCreated,
		Project:       "APP",
		TicketID:      ticket.ID,
		Payload:       ticket,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("append event: %v", err)
	}

	report, err := RepairWorkspace(ctx, root, clock, eventLog, projection)
	if err != nil {
		t.Fatalf("repair workspace: %v", err)
	}
	if report.Pending != 0 {
		t.Fatalf("expected no pending journals, got %d", report.Pending)
	}
	if len(report.Actions) != 1 || report.Actions[0] != "rebuilt projection" {
		t.Fatalf("unexpected repair actions: %#v", report.Actions)
	}
	detail, err := projection.QueryTicket(ctx, ticket.ID)
	if err != nil {
		t.Fatalf("query rebuilt ticket: %v", err)
	}
	if detail.ID != ticket.ID {
		t.Fatalf("unexpected rebuilt ticket: %#v", detail)
	}
}

func TestRepairWorkspaceDoesNotDuplicateReplayOnlyEvents(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 24, 11, 30, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: clock}
	eventLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer projection.Close()
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket := contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Replay comment and review",
		Summary:       "Replay comment and review",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusInReview,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket snapshot: %v", err)
	}
	createEvent := contracts.Event{
		EventID:       1,
		Timestamp:     now,
		Actor:         contracts.Actor("human:owner"),
		Type:          contracts.EventTicketCreated,
		Project:       "APP",
		TicketID:      ticket.ID,
		Payload:       ticket,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := eventLog.AppendEvent(ctx, createEvent); err != nil {
		t.Fatalf("append create event: %v", err)
	}
	if err := projection.ApplyEvent(ctx, createEvent); err != nil {
		t.Fatalf("apply create event: %v", err)
	}

	journal := MutationJournal{Root: root, Clock: clock}
	commentEvent := contracts.Event{
		EventID:       2,
		Timestamp:     now.Add(time.Minute),
		Actor:         contracts.Actor("agent:builder-1"),
		Type:          contracts.EventTicketCommented,
		Project:       "APP",
		TicketID:      ticket.ID,
		Payload:       map[string]any{"body": "needs context"},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	commentEntry, err := journal.Begin("comment ticket", "event_only", commentEvent)
	if err != nil {
		t.Fatalf("begin comment journal: %v", err)
	}
	if _, err := journal.Mark(commentEntry, MutationStagePrepared, ""); err != nil {
		t.Fatalf("mark comment journal: %v", err)
	}

	reviewEvent := contracts.Event{
		EventID:       3,
		Timestamp:     now.Add(2 * time.Minute),
		Actor:         contracts.Actor("agent:builder-1"),
		Type:          contracts.EventTicketReviewRequested,
		Project:       "APP",
		TicketID:      ticket.ID,
		Payload:       ticket,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	reviewEntry, err := journal.Begin("request review", "event_only", reviewEvent)
	if err != nil {
		t.Fatalf("begin review journal: %v", err)
	}
	if _, err := journal.Mark(reviewEntry, MutationStagePrepared, ""); err != nil {
		t.Fatalf("mark review journal: %v", err)
	}

	first, err := RepairWorkspace(ctx, root, clock, eventLog, projection)
	if err != nil {
		t.Fatalf("repair workspace first pass: %v", err)
	}
	second, err := RepairWorkspace(ctx, root, clock, eventLog, projection)
	if err != nil {
		t.Fatalf("repair workspace second pass: %v", err)
	}
	if first.Pending != 2 || second.Pending != 0 {
		t.Fatalf("unexpected repair pending counts: first=%#v second=%#v", first, second)
	}

	history, err := projection.QueryHistory(ctx, ticket.ID)
	if err != nil {
		t.Fatalf("query history: %v", err)
	}
	commentCount := 0
	reviewCount := 0
	for _, event := range history {
		switch event.Type {
		case contracts.EventTicketCommented:
			commentCount++
		case contracts.EventTicketReviewRequested:
			reviewCount++
		}
	}
	if commentCount != 1 || reviewCount != 1 {
		t.Fatalf("expected single replayed comment and review request, got %#v", history)
	}
}
