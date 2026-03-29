package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

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
	if _, err := targetActions.SyncPull(ctx, "origin", pushView.Publication.WorkspaceID, actor, "pull again"); err == nil || apperr.CodeOf(err) != apperr.CodeConflict {
		t.Fatalf("expected pull into non-empty workspace to fail with conflict, got %v", err)
	}

	events, err := sourceEvents.StreamEvents(ctx, workspaceEventProject, 0)
	if err != nil {
		t.Fatalf("stream sync events: %v", err)
	}
	if !containsEventType(events, contracts.EventBundleCreated) {
		t.Fatalf("expected bundle and sync completion events, got %#v", events)
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
