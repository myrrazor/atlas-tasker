package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/domain"
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
	actions := NewActionService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return clock }, FileLockManager{Root: root}, nil, nil)
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

	actions := NewActionService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return clock }, FileLockManager{Root: root}, nil, nil)
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

func TestActionServiceCreateEditCommentAndLinks(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 23, 9, 0, 0, 0, time.UTC)
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
	actions := NewActionService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return clock }, FileLockManager{Root: root}, nil, nil)

	created, err := actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "First pass",
		Summary:       "First pass",
		Description:   "ship the core path",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusBacklog,
		Priority:      contracts.PriorityMedium,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed from test")
	if err != nil {
		t.Fatalf("create tracked ticket: %v", err)
	}
	if created.ID != "APP-1" {
		t.Fatalf("expected APP-1, got %s", created.ID)
	}

	clock = clock.Add(time.Minute)
	assigned, err := actions.AssignTicket(ctx, created.ID, contracts.Actor("agent:builder-1"), contracts.Actor("human:owner"), "pick owner")
	if err != nil {
		t.Fatalf("assign ticket: %v", err)
	}
	if assigned.Assignee != contracts.Actor("agent:builder-1") {
		t.Fatalf("unexpected assignee: %#v", assigned)
	}

	clock = clock.Add(time.Minute)
	assigned.Description = "ship the full write path"
	if _, err := actions.SaveTrackedTicket(ctx, assigned, contracts.Actor("human:owner"), "edit from service"); err != nil {
		t.Fatalf("save tracked ticket: %v", err)
	}

	clock = clock.Add(time.Minute)
	if err := actions.CommentTicket(ctx, created.ID, "looks good", contracts.Actor("human:owner"), "feedback"); err != nil {
		t.Fatalf("comment ticket: %v", err)
	}

	clock = clock.Add(time.Minute)
	blocker, err := actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "Blocker",
		Summary:       "Blocker",
		Type:          contracts.TicketTypeBug,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     clock,
		UpdatedAt:     clock,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed blocker")
	if err != nil {
		t.Fatalf("create blocker: %v", err)
	}

	clock = clock.Add(time.Minute)
	if _, err := actions.LinkTickets(ctx, created.ID, blocker.ID, domain.LinkBlockedBy, contracts.Actor("human:owner"), "link blocker"); err != nil {
		t.Fatalf("link tickets: %v", err)
	}
	detail, err := projection.QueryTicket(ctx, created.ID)
	if err != nil {
		t.Fatalf("query linked ticket: %v", err)
	}
	if len(detail.BlockedBy) != 1 || detail.BlockedBy[0] != blocker.ID {
		t.Fatalf("expected blocked_by link, got %#v", detail.BlockedBy)
	}

	clock = clock.Add(time.Minute)
	if _, err := actions.UnlinkTickets(ctx, created.ID, blocker.ID, contracts.Actor("human:owner"), "clear blocker"); err != nil {
		t.Fatalf("unlink tickets: %v", err)
	}
	detail, err = projection.QueryTicket(ctx, created.ID)
	if err != nil {
		t.Fatalf("query unlinked ticket: %v", err)
	}
	if len(detail.BlockedBy) != 0 {
		t.Fatalf("expected link to be removed, got %#v", detail.BlockedBy)
	}
}

func TestActionServiceSerializesConcurrentClaims(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 23, 8, 0, 0, 0, time.UTC)
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
	ticket := contracts.TicketSnapshot{ID: "APP-1", Project: "APP", Title: "Race me", Type: contracts.TicketTypeTask, Status: contracts.StatusReady, Priority: contracts.PriorityHigh, CreatedAt: now, UpdatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}
	if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	actions := NewActionService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return clock }, FileLockManager{Root: root}, nil, nil)
	if err := actions.AppendAndProject(ctx, contracts.Event{EventID: 1, Timestamp: now, Actor: contracts.Actor("human:owner"), Type: contracts.EventTicketCreated, Project: "APP", TicketID: ticket.ID, Payload: ticket, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("append create event: %v", err)
	}

	errs := make(chan error, 2)
	for _, actor := range []contracts.Actor{"agent:builder-1", "agent:builder-2"} {
		go func(actor contracts.Actor) {
			_, err := actions.ClaimTicket(ctx, ticket.ID, actor, "race claim")
			errs <- err
		}(actor)
	}

	var okCount int
	var conflictCount int
	for range 2 {
		err := <-errs
		switch {
		case err == nil:
			okCount++
		case strings.Contains(err.Error(), "already claimed"):
			conflictCount++
		default:
			t.Fatalf("unexpected concurrent claim error: %v", err)
		}
	}
	if okCount != 1 || conflictCount != 1 {
		t.Fatalf("expected one success and one conflict, got ok=%d conflict=%d", okCount, conflictCount)
	}
}

func TestActionServiceMutateAndDeleteTrackedTicket(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 23, 9, 0, 0, 0, time.UTC)
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
	actions := NewActionService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return clock }, FileLockManager{Root: root}, nil, nil)
	created, err := actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "Edit me",
		Summary:       "Edit me",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityMedium,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed ticket")
	if err != nil {
		t.Fatalf("create tracked ticket: %v", err)
	}

	clock = clock.Add(2 * time.Minute)
	updated, err := actions.MutateTrackedTicket(ctx, created.ID, contracts.Actor("human:owner"), "rename ticket", "edit ticket", func(ticket *contracts.TicketSnapshot) error {
		ticket.Title = "Edited title"
		ticket.Summary = "Edited title"
		ticket.Labels = []string{"ops"}
		return nil
	})
	if err != nil {
		t.Fatalf("mutate tracked ticket: %v", err)
	}
	if updated.Title != "Edited title" || len(updated.Labels) != 1 || updated.Labels[0] != "ops" {
		t.Fatalf("unexpected updated ticket: %#v", updated)
	}

	clock = clock.Add(2 * time.Minute)
	deleted, err := actions.DeleteTrackedTicket(ctx, created.ID, contracts.Actor("human:owner"), "no longer needed")
	if err != nil {
		t.Fatalf("delete tracked ticket: %v", err)
	}
	if !deleted.Archived || deleted.Status != contracts.StatusCanceled {
		t.Fatalf("unexpected deleted ticket state: %#v", deleted)
	}
	if !strings.Contains(deleted.Notes, "Archived by human:owner") {
		t.Fatalf("expected archive audit line in notes, got %q", deleted.Notes)
	}

	history, err := projection.QueryHistory(ctx, created.ID)
	if err != nil {
		t.Fatalf("query history: %v", err)
	}
	if got := history[len(history)-1].Type; got != contracts.EventTicketClosed {
		t.Fatalf("expected final close event, got %s", got)
	}
}
