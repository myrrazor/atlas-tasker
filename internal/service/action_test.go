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

func TestActionServiceReviewAndPolicyFlow(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 23, 5, 0, 0, 0, time.UTC)
	clock := now

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return clock }}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer projection.Close()
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOwnerGate}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	project := contracts.Project{Key: "APP", Name: "App", CreatedAt: now}
	if err := projectStore.CreateProject(ctx, project); err != nil {
		t.Fatalf("create project: %v", err)
	}

	actions := NewActionService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return clock })
	updatedProject, err := actions.SetProjectPolicy(ctx, "APP", contracts.ProjectDefaults{
		CompletionMode:   contracts.CompletionModeDualGate,
		LeaseTTLMinutes:  45,
		AllowedWorkers:   []contracts.Actor{"agent:builder-1"},
		RequiredReviewer: contracts.Actor("agent:reviewer-1"),
	}, contracts.Actor("human:owner"), "project defaults")
	if err != nil {
		t.Fatalf("set project policy: %v", err)
	}
	if updatedProject.Defaults.CompletionMode != contracts.CompletionModeDualGate {
		t.Fatalf("unexpected project completion mode: %s", updatedProject.Defaults.CompletionMode)
	}

	ticket := contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Review flow",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusInProgress,
		Priority:      contracts.PriorityHigh,
		Reviewer:      contracts.Actor("agent:reviewer-1"),
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if err := actions.AppendAndProject(ctx, contracts.Event{EventID: 2, Timestamp: now, Actor: contracts.Actor("human:owner"), Type: contracts.EventTicketCreated, Project: "APP", TicketID: ticket.ID, Payload: ticket, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("append create event: %v", err)
	}

	updatedTicket, err := actions.SetTicketPolicy(ctx, ticket.ID, contracts.TicketPolicy{
		CompletionMode: contracts.CompletionModeDualGate,
		AllowedWorkers: []contracts.Actor{"agent:builder-1"},
	}, contracts.Actor("human:owner"), "ticket override")
	if err != nil {
		t.Fatalf("set ticket policy: %v", err)
	}
	if updatedTicket.Policy.CompletionMode != contracts.CompletionModeDualGate {
		t.Fatalf("unexpected ticket policy: %#v", updatedTicket.Policy)
	}

	clock = clock.Add(time.Minute)
	requested, err := actions.RequestReview(ctx, ticket.ID, contracts.Actor("agent:builder-1"), "ready for review")
	if err != nil {
		t.Fatalf("request review: %v", err)
	}
	if requested.Status != contracts.StatusInReview || requested.ReviewState != contracts.ReviewStatePending {
		t.Fatalf("unexpected request review state: %#v", requested)
	}

	clock = clock.Add(time.Minute)
	rejected, err := actions.RejectTicket(ctx, ticket.ID, contracts.Actor("agent:reviewer-1"), "needs changes")
	if err != nil {
		t.Fatalf("reject ticket: %v", err)
	}
	if rejected.Status != contracts.StatusInProgress || rejected.ReviewState != contracts.ReviewStateChangesRequested {
		t.Fatalf("unexpected rejected ticket: %#v", rejected)
	}

	clock = clock.Add(time.Minute)
	if _, err := actions.RequestReview(ctx, ticket.ID, contracts.Actor("agent:builder-1"), "retry"); err != nil {
		t.Fatalf("request review second time: %v", err)
	}
	clock = clock.Add(time.Minute)
	approved, err := actions.ApproveTicket(ctx, ticket.ID, contracts.Actor("agent:reviewer-1"), "looks good")
	if err != nil {
		t.Fatalf("approve ticket: %v", err)
	}
	if approved.ReviewState != contracts.ReviewStateApproved || approved.Status != contracts.StatusInReview {
		t.Fatalf("unexpected approved ticket: %#v", approved)
	}
	clock = clock.Add(time.Minute)
	returned, err := actions.MoveTicket(ctx, ticket.ID, contracts.StatusInProgress, contracts.Actor("agent:builder-1"), "follow-up work")
	if err != nil {
		t.Fatalf("move ticket back to in_progress: %v", err)
	}
	if returned.ReviewState != contracts.ReviewStateChangesRequested {
		t.Fatalf("expected changes_requested after approved review regresses, got %#v", returned)
	}
	clock = clock.Add(time.Minute)
	if _, err := actions.RequestReview(ctx, ticket.ID, contracts.Actor("agent:builder-1"), "retry"); err != nil {
		t.Fatalf("request review third time: %v", err)
	}
	clock = clock.Add(time.Minute)
	if _, err := actions.ApproveTicket(ctx, ticket.ID, contracts.Actor("agent:reviewer-1"), "now good"); err != nil {
		t.Fatalf("approve ticket second time: %v", err)
	}
	if _, err := actions.CompleteTicket(ctx, ticket.ID, contracts.Actor("agent:reviewer-1"), "trying to finish"); err == nil {
		t.Fatal("expected dual_gate to reject reviewer completion")
	}
	completed, err := actions.CompleteTicket(ctx, ticket.ID, contracts.Actor("human:owner"), "ship it")
	if err != nil {
		t.Fatalf("complete ticket: %v", err)
	}
	if completed.Status != contracts.StatusDone {
		t.Fatalf("unexpected completed ticket: %#v", completed)
	}
}
