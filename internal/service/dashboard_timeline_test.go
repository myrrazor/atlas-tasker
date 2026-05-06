package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

	view, err := queries.Dashboard(ctx, "")
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

	view, err := queries.Timeline(ctx, created.ID, "")
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
	if view.Entries[0].Provenance != "local" {
		t.Fatalf("expected local provenance for local ticket history, got %#v", view.Entries[0])
	}
}

func TestDashboardIncludesCollaborationQueues(t *testing.T) {
	root, actions, queries, projectStore, _, _ := newImportExportHarness(t)
	ctx := context.Background()
	now := actions.now()

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	created, err := actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "Collab seed",
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
	if _, err := actions.AddCollaborator(ctx, contracts.CollaboratorProfile{
		CollaboratorID: "rev-1",
		DisplayName:    "Reviewer One",
		AtlasActors:    []contracts.Actor{contracts.Actor("agent:reviewer-1")},
		ProviderHandles: map[string]string{
			"github": "rev-one",
		},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed collaborator"); err != nil {
		t.Fatalf("add collaborator: %v", err)
	}
	if _, err := actions.SetCollaboratorTrust(ctx, "rev-1", true, contracts.Actor("human:owner"), "trust collaborator"); err != nil {
		t.Fatalf("trust collaborator: %v", err)
	}
	if _, err := actions.BindMembership(ctx, contracts.MembershipBinding{
		CollaboratorID: "rev-1",
		ScopeKind:      contracts.MembershipScopeProject,
		ScopeID:        "APP",
		Role:           contracts.MembershipRoleReviewer,
	}, contracts.Actor("human:owner"), "bind reviewer"); err != nil {
		t.Fatalf("bind membership: %v", err)
	}
	if err := actions.CommentTicket(ctx, created.ID, "loop in @rev-1", contracts.Actor("agent:builder-1"), "mention reviewer"); err != nil {
		t.Fatalf("comment ticket: %v", err)
	}
	if err := (GateStore{Root: root}).SaveGate(ctx, contracts.GateSnapshot{
		GateID:        "gate_review_1",
		TicketID:      created.ID,
		Kind:          contracts.GateKindReview,
		State:         contracts.GateStateOpen,
		RequiredRole:  contracts.AgentRoleReviewer,
		CreatedBy:     contracts.Actor("human:owner"),
		CreatedAt:     now.Add(2 * time.Minute),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save gate: %v", err)
	}
	remote := normalizeSyncRemote(contracts.SyncRemote{
		RemoteID:      "origin",
		Kind:          contracts.SyncRemoteKindPath,
		Location:      filepath.Join(root, "remote"),
		Enabled:       true,
		DefaultAction: contracts.SyncDefaultActionPull,
		LastSuccessAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err := (SyncRemoteStore{Root: root}).SaveSyncRemote(ctx, remote); err != nil {
		t.Fatalf("save sync remote: %v", err)
	}
	bundleID := "bundle_dashboard"
	artifactPath := filepath.Join(root, bundleID+".tar.gz")
	files := []string{
		filepath.ToSlash(filepath.Join("projects", "APP", "tickets", created.ID+".md")),
	}
	manifest, err := buildBundleManifest(root, bundleID, syncBundleFormatV1, now, files)
	if err != nil {
		t.Fatalf("build bundle manifest: %v", err)
	}
	manifestRaw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := writeBundleArchive(root, artifactPath, manifestRaw, files); err != nil {
		t.Fatalf("write bundle archive: %v", err)
	}
	if err := (SyncJobStore{Root: root}).SaveSyncJob(ctx, contracts.SyncJob{
		JobID:         "sync_pull_1",
		RemoteID:      "origin",
		BundleRef:     artifactPath,
		Mode:          contracts.SyncJobModePull,
		State:         contracts.SyncJobStateFailed,
		StartedAt:     now.Add(3 * time.Minute),
		FinishedAt:    now.Add(4 * time.Minute),
		ReasonCodes:   []string{"sync_conflicts_detected"},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save sync job: %v", err)
	}
	if err := (ConflictStore{Root: root}).SaveConflict(ctx, contracts.ConflictRecord{
		ConflictID:    "conflict_ticket_1",
		EntityKind:    "ticket",
		EntityUID:     contracts.TicketUID("APP", created.ID),
		ConflictType:  contracts.ConflictTypeScalarDivergence,
		Status:        contracts.ConflictStatusOpen,
		OpenedByJob:   "sync_pull_1",
		OpenedAt:      now.Add(5 * time.Minute),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save conflict: %v", err)
	}

	view, err := queries.Dashboard(ctx, "rev-1")
	if err != nil {
		t.Fatalf("dashboard: %v", err)
	}
	if view.CollaboratorFilter != "rev-1" {
		t.Fatalf("expected collaborator filter, got %#v", view)
	}
	if len(view.CollaboratorWorkload) != 1 || view.CollaboratorWorkload[0].Approvals != 1 || view.CollaboratorWorkload[0].Mentions != 1 {
		t.Fatalf("unexpected collaborator workload: %#v", view.CollaboratorWorkload)
	}
	if len(view.MentionQueue) != 1 || view.MentionQueue[0].CollaboratorID != "rev-1" {
		t.Fatalf("unexpected mention queue: %#v", view.MentionQueue)
	}
	if len(view.ConflictQueue) != 1 || view.ConflictQueue[0].TicketID != created.ID {
		t.Fatalf("unexpected conflict queue: %#v", view.ConflictQueue)
	}
	if len(view.RemoteHealth) != 1 || view.RemoteHealth[0].RemoteID != "origin" || view.RemoteHealth[0].FailedJobs != 1 {
		t.Fatalf("unexpected remote health: %#v", view.RemoteHealth)
	}
	if len(view.FailedSyncJobs) != 1 || view.FailedSyncJobs[0] != "sync_pull_1" {
		t.Fatalf("unexpected failed sync jobs: %#v", view.FailedSyncJobs)
	}
}

func TestTimelineIncludesCollaborationEntries(t *testing.T) {
	root, actions, queries, projectStore, _, _ := newImportExportHarness(t)
	ctx := context.Background()
	now := actions.now()

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	created, err := actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "Timeline collab",
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
	if _, err := actions.AddCollaborator(ctx, contracts.CollaboratorProfile{
		CollaboratorID: "rev-1",
		AtlasActors:    []contracts.Actor{contracts.Actor("agent:reviewer-1")},
		SchemaVersion:  contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed collaborator"); err != nil {
		t.Fatalf("add collaborator: %v", err)
	}
	if _, err := actions.SetCollaboratorTrust(ctx, "rev-1", true, contracts.Actor("human:owner"), "trust collaborator"); err != nil {
		t.Fatalf("trust collaborator: %v", err)
	}
	if _, err := actions.BindMembership(ctx, contracts.MembershipBinding{
		CollaboratorID: "rev-1",
		ScopeKind:      contracts.MembershipScopeProject,
		ScopeID:        "APP",
		Role:           contracts.MembershipRoleReviewer,
	}, contracts.Actor("human:owner"), "bind reviewer"); err != nil {
		t.Fatalf("bind membership: %v", err)
	}
	if err := (GateStore{Root: root}).SaveGate(ctx, contracts.GateSnapshot{
		GateID:        "gate_review_1",
		TicketID:      created.ID,
		Kind:          contracts.GateKindReview,
		State:         contracts.GateStateOpen,
		RequiredRole:  contracts.AgentRoleReviewer,
		CreatedBy:     contracts.Actor("human:owner"),
		CreatedAt:     now.Add(2 * time.Minute),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save gate: %v", err)
	}
	bundleID := "bundle_timeline"
	artifactPath := filepath.Join(root, bundleID+".tar.gz")
	files := []string{
		filepath.ToSlash(filepath.Join("projects", "APP", "tickets", created.ID+".md")),
	}
	manifest, err := buildBundleManifest(root, bundleID, syncBundleFormatV1, now, files)
	if err != nil {
		t.Fatalf("build bundle manifest: %v", err)
	}
	manifestRaw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := writeBundleArchive(root, artifactPath, manifestRaw, files); err != nil {
		t.Fatalf("write bundle archive: %v", err)
	}
	if err := (SyncJobStore{Root: root}).SaveSyncJob(ctx, contracts.SyncJob{
		JobID:         "sync_push_1",
		RemoteID:      "origin",
		BundleRef:     artifactPath,
		Mode:          contracts.SyncJobModePush,
		State:         contracts.SyncJobStateCompleted,
		StartedAt:     now.Add(3 * time.Minute),
		FinishedAt:    now.Add(4 * time.Minute),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save sync job: %v", err)
	}
	if err := (ConflictStore{Root: root}).SaveConflict(ctx, contracts.ConflictRecord{
		ConflictID:    "conflict_ticket_1",
		EntityKind:    "ticket",
		EntityUID:     contracts.TicketUID("APP", created.ID),
		ConflictType:  contracts.ConflictTypeScalarDivergence,
		Status:        contracts.ConflictStatusResolved,
		OpenedByJob:   "sync_push_1",
		OpenedAt:      now.Add(5 * time.Minute),
		ResolvedAt:    now.Add(6 * time.Minute),
		ResolvedBy:    contracts.Actor("human:owner"),
		Resolution:    contracts.ConflictResolutionUseRemote,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save conflict: %v", err)
	}

	view, err := queries.Timeline(ctx, created.ID, "rev-1")
	if err != nil {
		t.Fatalf("timeline: %v", err)
	}
	if view.CollaboratorFilter != "rev-1" {
		t.Fatalf("expected collaborator filter, got %#v", view)
	}
	kinds := make([]string, 0, len(view.Entries))
	for _, entry := range view.Entries {
		kinds = append(kinds, entry.Kind)
	}
	for _, want := range []string{"approval", "sync_job", "conflict"} {
		if !containsString(kinds, want) {
			t.Fatalf("expected timeline kinds to include %s, got %#v", want, kinds)
		}
	}
	if last := view.Entries[len(view.Entries)-1]; !strings.Contains(last.Summary, "conflict resolved") {
		t.Fatalf("expected resolved conflict at tail, got %#v", last)
	}
}
