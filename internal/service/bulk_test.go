package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

func TestActionServiceRunBulkDryRunAndBatchMetadata(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC)

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
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
	actions := NewActionService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now }, FileLockManager{Root: root}, nil, nil)
	for i, ticket := range []contracts.TicketSnapshot{
		{ID: "APP-1", Project: "APP", Title: "Ready", Type: contracts.TicketTypeTask, Status: contracts.StatusReady, Priority: contracts.PriorityHigh, CreatedAt: now, UpdatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion},
		{ID: "APP-2", Project: "APP", Title: "Done", Type: contracts.TicketTypeTask, Status: contracts.StatusDone, Priority: contracts.PriorityMedium, CreatedAt: now, UpdatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion},
	} {
		if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
			t.Fatalf("create ticket %s: %v", ticket.ID, err)
		}
		if err := actions.AppendAndProject(ctx, contracts.Event{EventID: int64(i + 1), Timestamp: now, Actor: contracts.Actor("human:owner"), Type: contracts.EventTicketCreated, Project: "APP", TicketID: ticket.ID, Payload: ticket, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
			t.Fatalf("append create event %s: %v", ticket.ID, err)
		}
	}

	dryRun, err := actions.RunBulk(ctx, BulkOperation{
		Kind:      BulkOperationMove,
		Actor:     contracts.Actor("human:owner"),
		Status:    contracts.StatusInProgress,
		TicketIDs: []string{"APP-1", "APP-2"},
		DryRun:    true,
		Confirm:   false,
		BatchID:   "batch-preview",
	})
	if err != nil {
		t.Fatalf("dry-run bulk move: %v", err)
	}
	if dryRun.Summary.Skipped != 1 || dryRun.Summary.Failed != 1 {
		t.Fatalf("unexpected dry-run summary: %#v", dryRun.Summary)
	}
	ticket, err := ticketStore.GetTicket(ctx, "APP-1")
	if err != nil {
		t.Fatalf("get ticket after dry-run: %v", err)
	}
	if ticket.Status != contracts.StatusReady {
		t.Fatalf("expected dry-run to leave ticket untouched, got %s", ticket.Status)
	}

	result, err := actions.RunBulk(ctx, BulkOperation{
		Kind:      BulkOperationMove,
		Actor:     contracts.Actor("human:owner"),
		Status:    contracts.StatusInProgress,
		TicketIDs: []string{"APP-1", "APP-2"},
		Confirm:   true,
		BatchID:   "batch-live",
	})
	if err != nil {
		t.Fatalf("live bulk move: %v", err)
	}
	if result.Summary.Succeeded != 1 || result.Summary.Failed != 1 {
		t.Fatalf("unexpected live summary: %#v", result.Summary)
	}
	history, err := projection.QueryHistory(ctx, "APP-1")
	if err != nil {
		t.Fatalf("query history: %v", err)
	}
	last := history[len(history)-1]
	if last.Metadata.BatchID != "batch-live" {
		t.Fatalf("expected batch id to propagate, got %#v", last.Metadata)
	}
}

func TestActionServiceRunBulkRequiresConfirmation(t *testing.T) {
	root := t.TempDir()
	actions := NewActionService(root, nil, nil, nil, nil, func() time.Time { return time.Now().UTC() }, FileLockManager{Root: root}, nil, nil)
	_, err := actions.RunBulk(context.Background(), BulkOperation{
		Kind:      BulkOperationClaim,
		Actor:     contracts.Actor("human:owner"),
		TicketIDs: []string{"APP-1"},
	})
	if apperr.CodeOf(err) != apperr.CodeInvalidInput {
		t.Fatalf("expected invalid_input, got %v", err)
	}
}
