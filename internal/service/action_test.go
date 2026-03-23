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

func TestActionServiceClaimHeartbeatReleaseSweep(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 23, 3, 0, 0, 0, time.UTC)
	clock := now

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return clock }}
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
	ticket := contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Lease me",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	actions := NewActionService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return clock })
	createdEvent := contracts.Event{EventID: 1, Timestamp: now, Actor: contracts.Actor("human:owner"), Type: contracts.EventTicketCreated, Project: "APP", TicketID: ticket.ID, Payload: ticket, SchemaVersion: contracts.CurrentSchemaVersion}
	if err := actions.AppendAndProject(ctx, createdEvent); err != nil {
		t.Fatalf("append create event: %v", err)
	}

	claimed, err := actions.ClaimTicket(ctx, ticket.ID, contracts.Actor("agent:builder-1"), "starting work")
	if err != nil {
		t.Fatalf("claim ticket: %v", err)
	}
	if claimed.Lease.Actor != contracts.Actor("agent:builder-1") || claimed.Lease.Kind != contracts.LeaseKindWork {
		t.Fatalf("unexpected claim lease: %#v", claimed.Lease)
	}

	clock = clock.Add(10 * time.Minute)
	heartbeat, err := actions.HeartbeatTicket(ctx, ticket.ID, contracts.Actor("agent:builder-1"), "still working")
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if !heartbeat.Lease.LastHeartbeatAt.Equal(clock) {
		t.Fatalf("expected heartbeat timestamp %s, got %s", clock, heartbeat.Lease.LastHeartbeatAt)
	}

	released, err := actions.ReleaseTicket(ctx, ticket.ID, contracts.Actor("agent:builder-1"), "done for now")
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if released.Lease.Actor != "" {
		t.Fatalf("expected cleared lease, got %#v", released.Lease)
	}

	stale := ticket
	stale.ID = "APP-2"
	stale.Title = "Stale"
	stale.Lease = contracts.LeaseState{Actor: contracts.Actor("agent:builder-2"), Kind: contracts.LeaseKindWork, AcquiredAt: now, ExpiresAt: now.Add(5 * time.Minute), LastHeartbeatAt: now}
	if err := ticketStore.CreateTicket(ctx, stale); err != nil {
		t.Fatalf("create stale ticket: %v", err)
	}
	if err := actions.AppendAndProject(ctx, contracts.Event{EventID: 5, Timestamp: now, Actor: contracts.Actor("human:owner"), Type: contracts.EventTicketCreated, Project: "APP", TicketID: stale.ID, Payload: stale, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("append stale create event: %v", err)
	}
	clock = now.Add(2 * time.Hour)
	expired, err := actions.SweepExpiredClaims(ctx, contracts.Actor("human:owner"), "sweep")
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if len(expired) != 1 || expired[0].ID != stale.ID {
		t.Fatalf("unexpected expired tickets: %#v", expired)
	}
	history, err := projection.QueryHistory(ctx, stale.ID)
	if err != nil {
		t.Fatalf("query history: %v", err)
	}
	if history[len(history)-1].Type != contracts.EventTicketLeaseExpired {
		t.Fatalf("expected lease_expired event, got %#v", history[len(history)-1])
	}
}
