package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestSyncStatusDoesNotStampWorkspaceIdentityOnRead(t *testing.T) {
	ctx := context.Background()
	root, _, queries, _, _, _ := newImportExportHarness(t)

	status, err := queries.SyncStatus(ctx, "")
	if err != nil {
		t.Fatalf("sync status: %v", err)
	}
	if status.WorkspaceID != "" || status.MigrationComplete {
		t.Fatalf("expected unstamped workspace status, got %#v", status)
	}
	if _, err := os.Stat(storage.WorkspaceMetadataFile(root)); !os.IsNotExist(err) {
		t.Fatalf("sync status should not stamp workspace metadata, err=%v", err)
	}
}

func TestSyncPathRemoteRoundTrip(t *testing.T) {
	ctx := context.Background()
	_, sourceActions, sourceQueries, sourceProjects, _, sourceEvents := newImportExportHarness(t)
	seedSyncWorkspace(t, ctx, sourceActions, sourceProjects)

	remoteDir := filepath.Join(t.TempDir(), "path-remote")
	actor := contracts.Actor("human:owner")
	remote, err := sourceActions.AddSyncRemote(ctx, contracts.SyncRemote{
		RemoteID:      "origin",
		Kind:          contracts.SyncRemoteKindPath,
		Location:      remoteDir,
		DefaultAction: contracts.SyncDefaultActionPush,
		Enabled:       true,
	}, actor, "seed path remote")
	if err != nil {
		t.Fatalf("add path remote: %v", err)
	}

	pushView, err := sourceActions.SyncPush(ctx, remote.RemoteID, actor, "push workspace")
	if err != nil {
		t.Fatalf("sync push path remote: %v", err)
	}
	if pushView.Job.Mode != contracts.SyncJobModePush || pushView.Publication.BundleID == "" || pushView.Publication.WorkspaceID == "" {
		t.Fatalf("unexpected push view: %#v", pushView)
	}
	for _, suffix := range []string{pushView.Publication.ArtifactName, pushView.Publication.ManifestName, pushView.Publication.ChecksumName, "publication.json"} {
		if _, err := os.Stat(filepath.Join(remoteDir, pushView.Publication.WorkspaceID, suffix)); err != nil {
			t.Fatalf("expected remote publication file %s: %v", suffix, err)
		}
	}

	sourceStatus, err := sourceQueries.SyncStatus(ctx, remote.RemoteID)
	if err != nil {
		t.Fatalf("source sync status: %v", err)
	}
	if !sourceStatus.MigrationComplete || len(sourceStatus.Remotes) != 1 || len(sourceStatus.Remotes[0].Publications) != 1 {
		t.Fatalf("unexpected source sync status: %#v", sourceStatus)
	}

	_, targetActions, targetQueries, _, _, _ := newImportExportHarness(t)
	if _, err := targetActions.AddSyncRemote(ctx, contracts.SyncRemote{
		RemoteID:      "origin",
		Kind:          contracts.SyncRemoteKindPath,
		Location:      remoteDir,
		DefaultAction: contracts.SyncDefaultActionPull,
		Enabled:       true,
	}, actor, "seed target remote"); err != nil {
		t.Fatalf("add target path remote: %v", err)
	}

	fetchView, err := targetActions.SyncFetch(ctx, "origin", actor, "fetch remote")
	if err != nil {
		t.Fatalf("sync fetch path remote: %v", err)
	}
	if fetchView.Job.Mode != contracts.SyncJobModeFetch || fetchView.Job.Counts["publications"] != 1 {
		t.Fatalf("unexpected fetch view: %#v", fetchView)
	}
	targetStatus, err := targetQueries.SyncStatus(ctx, "origin")
	if err != nil {
		t.Fatalf("target sync status: %v", err)
	}
	if len(targetStatus.Remotes) != 1 || len(targetStatus.Remotes[0].Publications) != 1 {
		t.Fatalf("expected fetched publication in target status, got %#v", targetStatus)
	}

	pullView, err := targetActions.SyncPull(ctx, "origin", pushView.Publication.WorkspaceID, actor, "pull workspace")
	if err != nil {
		t.Fatalf("sync pull path remote: %v", err)
	}
	if pullView.Job.Mode != contracts.SyncJobModePull || pullView.Publication.BundleID != pushView.Publication.BundleID {
		t.Fatalf("unexpected pull view: %#v", pullView)
	}
	if _, err := targetQueries.TicketDetail(ctx, "APP-1"); err != nil {
		t.Fatalf("expected synced ticket detail: %v", err)
	}
	if _, err := targetActions.SyncPull(ctx, "origin", pushView.Publication.WorkspaceID, actor, "pull again"); err != nil {
		t.Fatalf("expected repeat pull to reconcile cleanly, got %v", err)
	}

	events, err := sourceEvents.StreamEvents(ctx, workspaceEventProject, 0)
	if err != nil {
		t.Fatalf("stream sync events: %v", err)
	}
	if !containsEventType(events, contracts.EventBundleCreated) {
		t.Fatalf("expected bundle and sync completion events, got %#v", events)
	}
}

func TestSyncPullOpensConflictAndAppliesSafeFiles(t *testing.T) {
	ctx := context.Background()
	_, sourceActions, _, sourceProjects, _, _ := newImportExportHarness(t)
	now := sourceActions.now()
	if err := sourceProjects.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create source project: %v", err)
	}
	if _, err := sourceActions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "Remote title",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed source ticket"); err != nil {
		t.Fatalf("create source ticket: %v", err)
	}
	if _, err := sourceActions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "Remote-only ticket",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityMedium,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed source ticket two"); err != nil {
		t.Fatalf("create source ticket two: %v", err)
	}

	remoteDir := filepath.Join(t.TempDir(), "path-remote")
	actor := contracts.Actor("human:owner")
	if _, err := sourceActions.AddSyncRemote(ctx, contracts.SyncRemote{
		RemoteID:      "origin",
		Kind:          contracts.SyncRemoteKindPath,
		Location:      remoteDir,
		DefaultAction: contracts.SyncDefaultActionPush,
		Enabled:       true,
	}, actor, "seed remote"); err != nil {
		t.Fatalf("add source remote: %v", err)
	}
	pushView, err := sourceActions.SyncPush(ctx, "origin", actor, "push source")
	if err != nil {
		t.Fatalf("push source workspace: %v", err)
	}

	_, targetActions, targetQueries, targetProjects, _, _ := newImportExportHarness(t)
	if err := targetProjects.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create target project: %v", err)
	}
	if _, err := targetActions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "Local title",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, actor, "seed target ticket"); err != nil {
		t.Fatalf("create target ticket: %v", err)
	}
	if _, err := targetActions.AddSyncRemote(ctx, contracts.SyncRemote{
		RemoteID:      "origin",
		Kind:          contracts.SyncRemoteKindPath,
		Location:      remoteDir,
		DefaultAction: contracts.SyncDefaultActionPull,
		Enabled:       true,
	}, actor, "seed target remote"); err != nil {
		t.Fatalf("add target remote: %v", err)
	}

	if _, err := targetActions.SyncPull(ctx, "origin", pushView.Publication.WorkspaceID, actor, "pull with conflict"); err == nil || apperr.CodeOf(err) != apperr.CodeConflict {
		t.Fatalf("expected conflict pull failure, got %v", err)
	}

	conflicts, err := targetQueries.ListConflicts(ctx)
	if err != nil {
		t.Fatalf("list conflicts: %v", err)
	}
	if len(conflicts) == 0 {
		t.Fatalf("expected at least one sync conflict")
	}
	if conflicts[0].Status != contracts.ConflictStatusOpen {
		t.Fatalf("expected open conflict, got %#v", conflicts[0])
	}

	ticket, err := targetQueries.TicketDetail(ctx, "APP-1")
	if err != nil {
		t.Fatalf("load conflicted ticket: %v", err)
	}
	if ticket.Ticket.Title != "Local title" {
		t.Fatalf("expected local ticket to stay in place until resolve, got %#v", ticket)
	}
	if _, err := targetQueries.TicketDetail(ctx, "APP-2"); err != nil {
		t.Fatalf("expected remote-only ticket to apply despite conflict: %v", err)
	}

	ticketConflictID := ""
	for _, conflict := range conflicts {
		if conflict.EntityKind == "ticket" {
			ticketConflictID = conflict.ConflictID
			break
		}
	}
	if ticketConflictID == "" {
		t.Fatalf("expected a ticket conflict in %#v", conflicts)
	}
	resolved, err := targetActions.ResolveConflict(ctx, ticketConflictID, contracts.ConflictResolutionUseRemote, actor, "take remote")
	if err != nil {
		t.Fatalf("resolve conflict: %v", err)
	}
	if resolved.Conflict.Status != contracts.ConflictStatusResolved || resolved.Conflict.Resolution != contracts.ConflictResolutionUseRemote {
		t.Fatalf("unexpected resolved conflict: %#v", resolved)
	}
	ticket, err = targetQueries.TicketDetail(ctx, "APP-1")
	if err != nil {
		t.Fatalf("reload resolved ticket: %v", err)
	}
	if ticket.Ticket.Title != "Remote title" {
		t.Fatalf("expected remote title after resolve, got %#v", ticket)
	}
	if _, err := targetActions.ResolveConflict(ctx, ticketConflictID, contracts.ConflictResolutionUseLocal, actor, "resolve twice"); err == nil || apperr.CodeOf(err) != apperr.CodeConflict {
		t.Fatalf("expected second resolve to fail with conflict, got %v", err)
	}
}

func TestReconcileEventFileOpensUIDCollisionAndKeepsUniqueEvents(t *testing.T) {
	_, actions, _, _, _, _ := newImportExportHarness(t)
	ctx := context.Background()

	localEvent := contracts.NormalizeEvent(contracts.Event{
		EventID:           1,
		EventUID:          "event-fixed",
		Timestamp:         actions.now(),
		OriginWorkspaceID: "ws-a",
		LogicalClock:      1,
		Actor:             contracts.Actor("human:owner"),
		Type:              contracts.EventTicketCommented,
		Project:           workspaceEventProject,
		Payload:           map[string]any{"body": "local"},
		SchemaVersion:     contracts.CurrentSchemaVersion,
	})
	remoteCollision := localEvent
	remoteCollision.Payload = map[string]any{"body": "remote"}
	remoteUnique := contracts.NormalizeEvent(contracts.Event{
		EventID:           2,
		EventUID:          "event-remote-2",
		Timestamp:         actions.now().Add(time.Second),
		OriginWorkspaceID: "ws-b",
		LogicalClock:      2,
		Actor:             contracts.Actor("human:owner"),
		Type:              contracts.EventTicketCommented,
		Project:           workspaceEventProject,
		Payload:           map[string]any{"body": "remote-unique"},
		SchemaVersion:     contracts.CurrentSchemaVersion,
	})

	localRaw, err := encodeSyncEventFile(map[string]contracts.Event{localEvent.EventUID: localEvent})
	if err != nil {
		t.Fatalf("encode local event file: %v", err)
	}
	remoteRaw, err := encodeSyncEventFile(map[string]contracts.Event{
		remoteCollision.EventUID: remoteCollision,
		remoteUnique.EventUID:    remoteUnique,
	})
	if err != nil {
		t.Fatalf("encode remote event file: %v", err)
	}

	type reconcileResult struct {
		raw         []byte
		conflictIDs []string
	}
	result, err := withWriteLock(ctx, actions.LockManager, "test reconcile events", func(ctx context.Context) (reconcileResult, error) {
		mergedRaw, conflictIDs, err := actions.reconcileEventFile(ctx, "job-1", ".tracker/events/2026-03.jsonl", localRaw, remoteRaw, contracts.Actor("human:owner"), "reconcile events")
		if err != nil {
			return reconcileResult{}, err
		}
		return reconcileResult{raw: mergedRaw, conflictIDs: conflictIDs}, nil
	})
	if err != nil {
		t.Fatalf("reconcile event file: %v", err)
	}
	mergedRaw, conflictIDs := result.raw, result.conflictIDs
	if len(conflictIDs) != 1 {
		t.Fatalf("expected one uid collision conflict, got %#v", conflictIDs)
	}
	merged, err := parseSyncEventFile(mergedRaw)
	if err != nil {
		t.Fatalf("parse merged event file: %v", err)
	}
	if len(merged) != 2 {
		t.Fatalf("expected merged event file to keep unique events, got %#v", merged)
	}
	if !syncEventsEqual(merged[localEvent.EventUID], localEvent) {
		t.Fatalf("expected local event to win unresolved collision, got %#v", merged[localEvent.EventUID])
	}
	if !syncEventsEqual(merged[remoteUnique.EventUID], remoteUnique) {
		t.Fatalf("expected unique remote event to merge, got %#v", merged[remoteUnique.EventUID])
	}
}

func TestSyncGitRemoteRoundTrip(t *testing.T) {
	ctx := context.Background()
	_, sourceActions, _, sourceProjects, _, _ := newImportExportHarness(t)
	seedSyncWorkspace(t, ctx, sourceActions, sourceProjects)

	gitRemote := filepath.Join(t.TempDir(), "sync-remote.git")
	gitRun(t, t.TempDir(), "init", "--bare", gitRemote)

	actor := contracts.Actor("human:owner")
	if _, err := sourceActions.AddSyncRemote(ctx, contracts.SyncRemote{
		RemoteID:      "origin",
		Kind:          contracts.SyncRemoteKindGit,
		Location:      gitRemote,
		DefaultAction: contracts.SyncDefaultActionPush,
		Enabled:       true,
	}, actor, "seed git remote"); err != nil {
		t.Fatalf("add git remote: %v", err)
	}
	pushView, err := sourceActions.SyncPush(ctx, "origin", actor, "push git remote")
	if err != nil {
		t.Fatalf("sync push git remote: %v", err)
	}
	if pushView.Publication.WorkspaceID == "" {
		t.Fatalf("expected publication workspace id, got %#v", pushView.Publication)
	}

	_, targetActions, targetQueries, _, _, _ := newImportExportHarness(t)
	if _, err := targetActions.AddSyncRemote(ctx, contracts.SyncRemote{
		RemoteID:      "origin",
		Kind:          contracts.SyncRemoteKindGit,
		Location:      gitRemote,
		DefaultAction: contracts.SyncDefaultActionPull,
		Enabled:       true,
	}, actor, "seed target git remote"); err != nil {
		t.Fatalf("add target git remote: %v", err)
	}
	fetchView, err := targetActions.SyncFetch(ctx, "origin", actor, "fetch git remote")
	if err != nil {
		t.Fatalf("sync fetch git remote: %v", err)
	}
	if fetchView.Job.Counts["publications"] != 1 {
		t.Fatalf("expected one fetched git publication, got %#v", fetchView)
	}
	status, err := targetQueries.SyncStatus(ctx, "origin")
	if err != nil {
		t.Fatalf("target git sync status: %v", err)
	}
	if len(status.Remotes) != 1 || len(status.Remotes[0].Publications) != 1 || status.Remotes[0].Publications[0].SourceRef == "" {
		t.Fatalf("expected fetched git publication with source ref, got %#v", status)
	}
	if _, err := targetActions.SyncPull(ctx, "origin", pushView.Publication.WorkspaceID, actor, "pull git remote"); err != nil {
		t.Fatalf("sync pull git remote: %v", err)
	}
	if _, err := targetQueries.TicketDetail(ctx, "APP-1"); err != nil {
		t.Fatalf("expected synced git ticket detail: %v", err)
	}
}

func TestAddSyncRemoteRejectsUnsafeLocations(t *testing.T) {
	ctx := context.Background()
	root, actions, _, _, _, _ := newImportExportHarness(t)
	actor := contracts.Actor("human:owner")

	if _, err := actions.AddSyncRemote(ctx, contracts.SyncRemote{
		RemoteID:      "bad-url",
		Kind:          contracts.SyncRemoteKindGit,
		Location:      "https://user:secret@example.com/acme/repo.git",
		DefaultAction: contracts.SyncDefaultActionFetch,
		Enabled:       true,
	}, actor, "reject embedded credentials"); err == nil || apperr.CodeOf(err) != apperr.CodeInvalidInput {
		t.Fatalf("expected invalid git remote URL, got %v", err)
	}

	if _, err := actions.AddSyncRemote(ctx, contracts.SyncRemote{
		RemoteID:      "bad-path",
		Kind:          contracts.SyncRemoteKindPath,
		Location:      filepath.Join(root, ".tracker", "sync"),
		DefaultAction: contracts.SyncDefaultActionFetch,
		Enabled:       true,
	}, actor, "reject workspace recursion"); err == nil || apperr.CodeOf(err) != apperr.CodeInvalidInput {
		t.Fatalf("expected invalid path remote, got %v", err)
	}
}

func seedSyncWorkspace(t *testing.T, ctx context.Context, actions *ActionService, projects interface {
	CreateProject(context.Context, contracts.Project) error
}) {
	t.Helper()
	now := actions.now()
	if err := projects.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create sync project: %v", err)
	}
	if _, err := actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "Ship sync",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed sync ticket"); err != nil {
		t.Fatalf("create sync ticket: %v", err)
	}
}

func containsEventType(events []contracts.Event, eventType contracts.EventType) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}
