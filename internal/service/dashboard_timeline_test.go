package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestDashboardSummarizesOperationalPressure(t *testing.T) {
	root, actions, queries, projectStore, ticketStore, _ := newImportExportHarness(t)
	ctx := context.Background()
	now := actions.now()
	old := now.AddDate(0, 0, -10)

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	tickets := []contracts.TicketSnapshot{
		{ID: "APP-1", Project: "APP", Title: "Needs review", Summary: "Needs review", Type: contracts.TicketTypeTask, Status: contracts.StatusInReview, Priority: contracts.PriorityHigh, CreatedAt: now, UpdatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion},
		{ID: "APP-2", Project: "APP", Title: "Owner wait", Summary: "Owner wait", Type: contracts.TicketTypeTask, Status: contracts.StatusDone, Priority: contracts.PriorityHigh, CreatedAt: now, UpdatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion},
		{ID: "APP-3", Project: "APP", Title: "Ready to merge", Summary: "Ready to merge", Type: contracts.TicketTypeTask, Status: contracts.StatusInReview, Priority: contracts.PriorityHigh, CreatedAt: now, UpdatedAt: now, ChangeReadyState: contracts.ChangeReadyMergeReady, SchemaVersion: contracts.CurrentSchemaVersion},
		{ID: "APP-4", Project: "APP", Title: "Checks blocked", Summary: "Checks blocked", Type: contracts.TicketTypeTask, Status: contracts.StatusInProgress, Priority: contracts.PriorityHigh, CreatedAt: now, UpdatedAt: now, ChangeReadyState: contracts.ChangeReadyChecksPending, SchemaVersion: contracts.CurrentSchemaVersion},
	}
	for _, ticket := range tickets {
		if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
			t.Fatalf("create ticket %s: %v", ticket.ID, err)
		}
	}
	if err := (GateStore{Root: root}).SaveGate(ctx, contracts.GateSnapshot{
		GateID:        "gate_owner_1",
		TicketID:      "APP-2",
		Kind:          contracts.GateKindOwner,
		State:         contracts.GateStateOpen,
		CreatedBy:     contracts.Actor("human:owner"),
		CreatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save gate: %v", err)
	}
	runs := []contracts.RunSnapshot{
		normalizeRunSnapshot(contracts.RunSnapshot{RunID: "run_active", TicketID: "APP-1", Project: "APP", AgentID: "builder-1", Status: contracts.RunStatusActive, Kind: contracts.RunKindWork, CreatedAt: now, StartedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}),
		normalizeRunSnapshot(contracts.RunSnapshot{RunID: "run_stale", TicketID: "APP-3", Project: "APP", AgentID: "builder-1", Status: contracts.RunStatusCompleted, Kind: contracts.RunKindWork, CreatedAt: old, CompletedAt: old, WorktreePath: filepath.Join(root, ".atlas-tasker-worktrees", "run_stale"), SchemaVersion: contracts.CurrentSchemaVersion}),
		normalizeRunSnapshot(contracts.RunSnapshot{RunID: "run_archive", TicketID: "APP-4", Project: "APP", AgentID: "builder-1", Status: contracts.RunStatusCompleted, Kind: contracts.RunKindWork, CreatedAt: old, CompletedAt: old, SchemaVersion: contracts.CurrentSchemaVersion}),
	}
	for _, run := range runs {
		if err := (RunStore{Root: root}).SaveRun(ctx, run); err != nil {
			t.Fatalf("save run %s: %v", run.RunID, err)
		}
	}
	if err := os.MkdirAll(storage.RuntimeDir(root, "run_archive"), 0o755); err != nil {
		t.Fatalf("mkdir runtime dir: %v", err)
	}
	if err := os.WriteFile(storage.RuntimeBriefFile(root, "run_archive"), []byte("brief"), 0o644); err != nil {
		t.Fatalf("write runtime brief: %v", err)
	}
	if err := os.Chtimes(storage.RuntimeDir(root, "run_archive"), old, old); err != nil {
		t.Fatalf("chtimes runtime dir: %v", err)
	}
	if err := os.Chtimes(storage.RuntimeBriefFile(root, "run_archive"), old, old); err != nil {
		t.Fatalf("chtimes runtime brief: %v", err)
	}

	view, err := queries.Dashboard(ctx)
	if err != nil {
		t.Fatalf("dashboard: %v", err)
	}
	if view.ActiveRuns != 1 {
		t.Fatalf("expected 1 active run, got %#v", view)
	}
	if view.AwaitingReview.Count != 2 {
		t.Fatalf("expected 2 awaiting review tickets, got %#v", view.AwaitingReview)
	}
	if view.AwaitingOwner.Count != 1 {
		t.Fatalf("expected 1 awaiting owner ticket, got %#v", view.AwaitingOwner)
	}
	if view.MergeReady.Count != 1 || view.BlockedByChecks.Count != 1 {
		t.Fatalf("unexpected merge/check buckets: %#v", view)
	}
	if len(view.StaleWorktrees) != 1 || view.StaleWorktrees[0] != "run_stale" {
		t.Fatalf("expected stale worktree run_stale, got %#v", view.StaleWorktrees)
	}
	if len(view.RetentionTargets) == 0 || view.RetentionTargets[0] != string(contracts.RetentionTargetRuntime) {
		t.Fatalf("expected runtime retention pressure, got %#v", view.RetentionTargets)
	}
}

func TestTimelineOrdersHistoryDeterministically(t *testing.T) {
	_, actions, queries, projectStore, _, _ := newImportExportHarness(t)
	ctx := context.Background()
	now := actions.now()

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	created, err := actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "Timeline seed",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed ticket")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if err := actions.CommentTicket(ctx, created.ID, "first comment", contracts.Actor("agent:builder-1"), "comment for timeline"); err != nil {
		t.Fatalf("comment ticket: %v", err)
	}
	if _, err := actions.MoveTicket(ctx, created.ID, contracts.StatusInProgress, contracts.Actor("agent:builder-1"), "started work"); err != nil {
		t.Fatalf("move ticket: %v", err)
	}

	view, err := queries.Timeline(ctx, created.ID)
	if err != nil {
		t.Fatalf("timeline: %v", err)
	}
	if view.TicketID != created.ID || len(view.Entries) < 3 {
		t.Fatalf("unexpected timeline payload: %#v", view)
	}
	for idx := 1; idx < len(view.Entries); idx++ {
		prev := view.Entries[idx-1]
		next := view.Entries[idx]
		if next.Timestamp.Before(prev.Timestamp) {
			t.Fatalf("timeline not sorted: %#v", view.Entries)
		}
		if next.Timestamp.Equal(prev.Timestamp) && next.EventID < prev.EventID {
			t.Fatalf("timeline tie-break is unstable: %#v", view.Entries)
		}
	}
	last := view.Entries[len(view.Entries)-1]
	if last.Type != contracts.EventTicketMoved {
		t.Fatalf("expected last timeline entry to be move, got %#v", last)
	}
}
