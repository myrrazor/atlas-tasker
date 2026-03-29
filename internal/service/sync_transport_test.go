package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
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
	if status.Migration.State != MigrationStateUnstamped || len(status.Migration.ReasonCodes) == 0 || status.Migration.ReasonCodes[0] != "migration_incomplete" {
		t.Fatalf("expected explicit unstamped migration state, got %#v", status.Migration)
	}
	if _, err := os.Stat(storage.WorkspaceMetadataFile(root)); !os.IsNotExist(err) {
		t.Fatalf("sync status should not stamp workspace metadata, err=%v", err)
	}
}

func TestMigrationStatusDetectsDivergentTicketUID(t *testing.T) {
	ctx := context.Background()
	root, actions, queries, projectStore, _, _ := newImportExportHarness(t)
	seedSyncWorkspace(t, ctx, actions, projectStore)

	if _, err := actions.CreateSyncBundle(ctx, contracts.Actor("human:owner"), "stamp migration"); err != nil {
		t.Fatalf("create sync bundle: %v", err)
	}
	if err := replaceFrontmatterValue(storage.TicketFile(root, "APP", "APP-1"), "ticket_uid", "00000000-0000-0000-0000-000000000999"); err != nil {
		t.Fatalf("corrupt ticket uid: %v", err)
	}

	status, err := queries.MigrationStatus(ctx)
	if err != nil {
		t.Fatalf("migration status: %v", err)
	}
	if status.State != MigrationStateDivergent || status.Ready {
		t.Fatalf("expected divergent migration status, got %#v", status)
	}
	if !containsMigrationReason(status.ReasonCodes, "migration_divergent") {
		t.Fatalf("expected migration_divergent reason, got %#v", status.ReasonCodes)
	}
	if len(status.Entities) == 0 || status.Entities[0].Kind == "" {
		t.Fatalf("expected entity-level migration details, got %#v", status.Entities)
	}
	if _, err := actions.CreateSyncBundle(ctx, contracts.Actor("human:owner"), "retry with divergence"); err == nil || apperr.CodeOf(err) != apperr.CodeRepairNeeded {
		t.Fatalf("expected create sync bundle to fail with repair needed, got %v", err)
	}
}

func TestSyncGitRemoteConvergesAfterIndependentLegacyUpgradeAndUnrelatedWrites(t *testing.T) {
	ctx := context.Background()
	baseRoot, actions, _, projectStore, _, _ := newImportExportHarness(t)
	seedSyncWorkspace(t, ctx, actions, projectStore)
	legacyizeWorkspace(t, baseRoot)

	rootA := filepath.Join(t.TempDir(), "workspace-a")
	rootB := filepath.Join(t.TempDir(), "workspace-b")
	if err := copyDir(baseRoot, rootA); err != nil {
		t.Fatalf("copy workspace a: %v", err)
	}
	if err := copyDir(baseRoot, rootB); err != nil {
		t.Fatalf("copy workspace b: %v", err)
	}
	_, actionsA, queriesA, projectsA, _, _ := reopenImportExportHarness(t, rootA)
	_, actionsB, queriesB, projectsB, _, _ := reopenImportExportHarness(t, rootB)
	_ = projectsA
	_ = projectsB

	gitRemote := filepath.Join(t.TempDir(), "sync-remote.git")
	gitRun(t, t.TempDir(), "init", "--bare", gitRemote)
	actor := contracts.Actor("human:owner")

	if _, err := actionsA.AddSyncRemote(ctx, contracts.SyncRemote{RemoteID: "origin", Kind: contracts.SyncRemoteKindGit, Location: gitRemote, DefaultAction: contracts.SyncDefaultActionPush, Enabled: true}, actor, "seed git remote a"); err != nil {
		t.Fatalf("add git remote a: %v", err)
	}
	if _, err := actionsB.AddSyncRemote(ctx, contracts.SyncRemote{RemoteID: "origin", Kind: contracts.SyncRemoteKindGit, Location: gitRemote, DefaultAction: contracts.SyncDefaultActionPull, Enabled: true}, actor, "seed git remote b"); err != nil {
		t.Fatalf("add git remote b: %v", err)
	}

	if _, err := actionsA.AddCollaborator(ctx, contracts.CollaboratorProfile{CollaboratorID: "alice", DisplayName: "Alice", Status: contracts.CollaboratorStatusActive, TrustState: contracts.CollaboratorTrustStateTrusted}, actor, "local write a"); err != nil {
		t.Fatalf("add collaborator a: %v", err)
	}
	if _, err := actionsB.AddCollaborator(ctx, contracts.CollaboratorProfile{CollaboratorID: "bob", DisplayName: "Bob", Status: contracts.CollaboratorStatusActive, TrustState: contracts.CollaboratorTrustStateTrusted}, actor, "local write b"); err != nil {
		t.Fatalf("add collaborator b: %v", err)
	}

	pushA, err := actionsA.SyncPush(ctx, "origin", actor, "push upgraded replica a")
	if err != nil {
		t.Fatalf("push a: %v", err)
	}
	if _, err := actionsB.SyncPull(ctx, "origin", pushA.Publication.WorkspaceID, actor, "pull upgraded replica a"); err != nil {
		t.Fatalf("pull b from a: %v", err)
	}
	pushB, err := actionsB.SyncPush(ctx, "origin", actor, "push upgraded replica b")
	if err != nil {
		t.Fatalf("push b: %v", err)
	}
	if _, err := actionsA.SyncPull(ctx, "origin", pushB.Publication.WorkspaceID, actor, "pull upgraded replica b"); err != nil {
		t.Fatalf("pull a from b: %v", err)
	}

	ticketsA, err := projectsA.ListProjects(ctx)
	if err != nil || len(ticketsA) == 0 {
		t.Fatalf("expected projects after convergence, err=%v projects=%#v", err, ticketsA)
	}
	ticketA, err := queriesA.TicketDetail(ctx, "APP-1")
	if err != nil {
		t.Fatalf("ticket detail a: %v", err)
	}
	ticketB, err := queriesB.TicketDetail(ctx, "APP-1")
	if err != nil {
		t.Fatalf("ticket detail b: %v", err)
	}
	if ticketA.Ticket.TicketUID == "" || ticketA.Ticket.TicketUID != ticketB.Ticket.TicketUID {
		t.Fatalf("expected converged deterministic ticket uid, got %q vs %q", ticketA.Ticket.TicketUID, ticketB.Ticket.TicketUID)
	}
	if collaborators, err := queriesA.ListCollaborators(ctx); err != nil || len(collaborators) != 2 {
		t.Fatalf("expected converged collaborators on a, err=%v items=%#v", err, collaborators)
	}
	if collaborators, err := queriesB.ListCollaborators(ctx); err != nil || len(collaborators) != 2 {
		t.Fatalf("expected converged collaborators on b, err=%v items=%#v", err, collaborators)
	}
	statusA, err := queriesA.MigrationStatus(ctx)
	if err != nil {
		t.Fatalf("migration status a: %v", err)
	}
	statusB, err := queriesB.MigrationStatus(ctx)
	if err != nil {
		t.Fatalf("migration status b: %v", err)
	}
	if !statusA.Ready || !statusB.Ready || statusA.State != MigrationStateStamped || statusB.State != MigrationStateStamped {
		t.Fatalf("expected stamped migration status after convergence\na=%#v\nb=%#v", statusA, statusB)
	}
}

func TestSyncBundleConvergesAfterIndependentLegacyUpgradeAndUnrelatedWrites(t *testing.T) {
	ctx := context.Background()
	baseRoot, actions, _, projectStore, _, _ := newImportExportHarness(t)
	seedSyncWorkspace(t, ctx, actions, projectStore)
	legacyizeWorkspace(t, baseRoot)

	rootA := filepath.Join(t.TempDir(), "bundle-a")
	rootB := filepath.Join(t.TempDir(), "bundle-b")
	if err := copyDir(baseRoot, rootA); err != nil {
		t.Fatalf("copy bundle workspace a: %v", err)
	}
	if err := copyDir(baseRoot, rootB); err != nil {
		t.Fatalf("copy bundle workspace b: %v", err)
	}
	_, actionsA, queriesA, _, _, _ := reopenImportExportHarness(t, rootA)
	_, actionsB, queriesB, _, _, _ := reopenImportExportHarness(t, rootB)
	actor := contracts.Actor("human:owner")

	if _, err := actionsA.AddCollaborator(ctx, contracts.CollaboratorProfile{CollaboratorID: "alice", DisplayName: "Alice", Status: contracts.CollaboratorStatusActive, TrustState: contracts.CollaboratorTrustStateTrusted}, actor, "local write a"); err != nil {
		t.Fatalf("add collaborator a: %v", err)
	}
	if _, err := actionsB.AddCollaborator(ctx, contracts.CollaboratorProfile{CollaboratorID: "bob", DisplayName: "Bob", Status: contracts.CollaboratorStatusActive, TrustState: contracts.CollaboratorTrustStateTrusted}, actor, "local write b"); err != nil {
		t.Fatalf("add collaborator b: %v", err)
	}

	bundleA, err := actionsA.CreateSyncBundle(ctx, actor, "bundle a")
	if err != nil {
		t.Fatalf("create bundle a: %v", err)
	}
	if _, err := actionsB.ImportSyncBundle(ctx, bundleA.Job.BundleRef, actor, "import a"); err != nil {
		t.Fatalf("import bundle a into b: %v", err)
	}
	bundleB, err := actionsB.CreateSyncBundle(ctx, actor, "bundle b")
	if err != nil {
		t.Fatalf("create bundle b: %v", err)
	}
	if _, err := actionsA.ImportSyncBundle(ctx, bundleB.Job.BundleRef, actor, "import b"); err != nil {
		t.Fatalf("import bundle b into a: %v", err)
	}

	ticketA, err := queriesA.TicketDetail(ctx, "APP-1")
	if err != nil {
		t.Fatalf("ticket detail a: %v", err)
	}
	ticketB, err := queriesB.TicketDetail(ctx, "APP-1")
	if err != nil {
		t.Fatalf("ticket detail b: %v", err)
	}
	if ticketA.Ticket.TicketUID == "" || ticketA.Ticket.TicketUID != ticketB.Ticket.TicketUID {
		t.Fatalf("expected converged deterministic ticket uid after bundle import, got %q vs %q", ticketA.Ticket.TicketUID, ticketB.Ticket.TicketUID)
	}
	if collaborators, err := queriesA.ListCollaborators(ctx); err != nil || len(collaborators) != 2 {
		t.Fatalf("expected converged collaborators on a after bundle import, err=%v items=%#v", err, collaborators)
	}
	if collaborators, err := queriesB.ListCollaborators(ctx); err != nil || len(collaborators) != 2 {
		t.Fatalf("expected converged collaborators on b after bundle import, err=%v items=%#v", err, collaborators)
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

func containsMigrationReason(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func reopenImportExportHarness(t *testing.T, root string) (string, *ActionService, *QueryService, mdstore.ProjectStore, mdstore.TicketStore, *eventstore.Log) {
	t.Helper()
	now := time.Date(2026, 3, 27, 9, 0, 0, 0, time.UTC)
	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite at %s: %v", root, err)
	}
	t.Cleanup(func() { _ = projection.Close() })
	actions := NewActionService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now }, FileLockManager{Root: root}, nil, nil)
	queries := NewQueryService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now })
	return root, actions, queries, projectStore, ticketStore, eventsLog
}

func legacyizeWorkspace(t *testing.T, root string) {
	t.Helper()
	if err := os.Remove(storage.WorkspaceMetadataFile(root)); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove workspace metadata: %v", err)
	}
	if err := os.Remove(syncMigrationPath(root)); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove sync migration file: %v", err)
	}
	if err := removeFrontmatterField(storage.TicketFile(root, "APP", "APP-1"), "ticket_uid"); err != nil {
		t.Fatalf("remove ticket uid: %v", err)
	}
	entries, err := os.ReadDir(storage.EventsDir(root))
	if err != nil {
		t.Fatalf("read events dir: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(storage.EventsDir(root), entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read events file %s: %v", path, err)
		}
		lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
		rewritten := make([]string, 0, len(lines))
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(line), &payload); err != nil {
				t.Fatalf("parse event json: %v", err)
			}
			delete(payload, "event_uid")
			delete(payload, "logical_clock")
			delete(payload, "origin_workspace_id")
			normalized, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal legacy event json: %v", err)
			}
			rewritten = append(rewritten, string(normalized))
		}
		if err := os.WriteFile(path, []byte(strings.Join(rewritten, "\n")+"\n"), 0o644); err != nil {
			t.Fatalf("rewrite events file %s: %v", path, err)
		}
	}
}

func removeFrontmatterField(path string, field string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	prefix := strings.TrimSpace(field) + ":"
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			continue
		}
		filtered = append(filtered, line)
	}
	return os.WriteFile(path, []byte(strings.Join(filtered, "\n")), 0o644)
}

func replaceFrontmatterValue(path string, field string, value string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	prefix := strings.TrimSpace(field) + ":"
	replaced := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			lines[i] = field + ": " + value
			replaced = true
			break
		}
	}
	if !replaced {
		return fmt.Errorf("frontmatter field %s not found in %s", field, path)
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}
