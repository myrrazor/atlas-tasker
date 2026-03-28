package service

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
)

func TestArchiveApplyListAndRestoreRuntimeRoundTrip(t *testing.T) {
	root, actions, queries, projectStore, ticketStore, eventsLog := newImportExportHarness(t)
	ctx := context.Background()
	now := actions.now()
	old := now.AddDate(0, 0, -10)

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := ticketStore.CreateTicket(ctx, contracts.TicketSnapshot{ID: "APP-1", Project: "APP", Title: "Archive runtime", Summary: "Archive runtime", Type: contracts.TicketTypeTask, Status: contracts.StatusDone, Priority: contracts.PriorityHigh, CreatedAt: old, UpdatedAt: old, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	run := normalizeRunSnapshot(contracts.RunSnapshot{RunID: "run_archive", TicketID: "APP-1", Project: "APP", Status: contracts.RunStatusCompleted, Kind: contracts.RunKindWork, CreatedAt: old, CompletedAt: old, SchemaVersion: contracts.CurrentSchemaVersion})
	if err := (RunStore{Root: root}).SaveRun(ctx, run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	for _, path := range []string{storage.RuntimeBriefFile(root, run.RunID), storage.RuntimeContextFile(root, run.RunID), storage.RuntimeLaunchFile(root, run.RunID, "codex"), storage.RuntimeLaunchFile(root, run.RunID, "claude")} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir runtime dir: %v", err)
		}
		if err := os.WriteFile(path, []byte("runtime"), 0o644); err != nil {
			t.Fatalf("write runtime artifact: %v", err)
		}
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatalf("chtimes %s: %v", path, err)
		}
	}
	if err := os.Chtimes(storage.RuntimeDir(root, run.RunID), old, old); err != nil {
		t.Fatalf("chtimes runtime dir: %v", err)
	}

	plan, err := queries.ArchivePlan(ctx, contracts.RetentionTargetRuntime, "APP")
	if err != nil {
		t.Fatalf("archive plan: %v", err)
	}
	if plan.Policy.PolicyID != "runtime-default" || len(plan.Items) != 1 {
		t.Fatalf("unexpected archive plan: %#v", plan)
	}

	applied, err := actions.ApplyArchive(ctx, contracts.RetentionTargetRuntime, "APP", true, contracts.Actor("human:owner"), "archive runtime")
	if err != nil {
		t.Fatalf("apply archive: %v", err)
	}
	if applied.Record.ArchiveID == "" || applied.Record.State != contracts.ArchiveRecordArchived || applied.Record.ItemCount != 1 {
		t.Fatalf("unexpected archive apply result: %#v", applied)
	}
	if _, err := os.Stat(storage.RuntimeDir(root, run.RunID)); !os.IsNotExist(err) {
		t.Fatalf("expected live runtime dir to move out, got err=%v", err)
	}
	payloadPath := filepath.Join(applied.Record.PayloadDir, storage.TrackerDirName, "runtime", run.RunID, "brief.md")
	if _, err := os.Stat(payloadPath); err != nil {
		t.Fatalf("expected archived payload %s: %v", payloadPath, err)
	}

	archives, err := queries.ListArchiveRecords(ctx, contracts.RetentionTargetRuntime, "APP")
	if err != nil {
		t.Fatalf("list archives: %v", err)
	}
	if len(archives) != 1 || archives[0].ArchiveID != applied.Record.ArchiveID {
		t.Fatalf("unexpected archive list: %#v", archives)
	}

	restored, err := actions.RestoreArchive(ctx, applied.Record.ArchiveID, contracts.Actor("human:owner"), "restore runtime")
	if err != nil {
		t.Fatalf("restore archive: %v", err)
	}
	if restored.Record.State != contracts.ArchiveRecordRestored || restored.Record.RestoredAt.IsZero() {
		t.Fatalf("unexpected restored record: %#v", restored.Record)
	}
	if _, err := os.Stat(storage.RuntimeBriefFile(root, run.RunID)); err != nil {
		t.Fatalf("expected restored runtime artifact: %v", err)
	}
	if _, err := os.Stat(payloadPath); err != nil {
		t.Fatalf("expected archive payload to remain after restore: %v", err)
	}

	events, err := eventsLog.StreamEvents(ctx, workspaceProjectKey, 0)
	if err != nil {
		t.Fatalf("stream events: %v", err)
	}
	types := make([]contracts.EventType, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	for _, expected := range []contracts.EventType{contracts.EventArchiveApplied, contracts.EventArchiveRestored} {
		if !slices.Contains(types, expected) {
			t.Fatalf("expected archive event %s in %#v", expected, types)
		}
	}
}

func TestArchivePlanUsesProjectRetentionPolicyBinding(t *testing.T) {
	_, actions, queries, projectStore, ticketStore, _ := newImportExportHarness(t)
	ctx := context.Background()
	now := actions.now()
	old := now.AddDate(0, 0, -2)

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	project, err := projectStore.GetProject(ctx, "APP")
	if err != nil {
		t.Fatalf("get project: %v", err)
	}
	project.Defaults.RetentionPolicies = []string{"runtime-tight"}
	if err := projectStore.UpdateProject(ctx, project); err != nil {
		t.Fatalf("update project: %v", err)
	}
	if err := actions.RetentionPolicies.SaveRetentionPolicy(ctx, contracts.RetentionPolicy{PolicyID: "runtime-tight", Target: contracts.RetentionTargetRuntime, MaxAgeDays: 1, ArchiveInsteadOfDelete: true, RequiresConfirmation: true, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("save retention policy: %v", err)
	}
	if err := ticketStore.CreateTicket(ctx, contracts.TicketSnapshot{ID: "APP-1", Project: "APP", Title: "Policy runtime", Summary: "Policy runtime", Type: contracts.TicketTypeTask, Status: contracts.StatusDone, Priority: contracts.PriorityHigh, CreatedAt: old, UpdatedAt: old, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	run := normalizeRunSnapshot(contracts.RunSnapshot{RunID: "run_policy", TicketID: "APP-1", Project: "APP", Status: contracts.RunStatusCompleted, Kind: contracts.RunKindWork, CreatedAt: old, CompletedAt: old, SchemaVersion: contracts.CurrentSchemaVersion})
	if err := (RunStore{Root: queries.Root}).SaveRun(ctx, run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	if err := os.MkdirAll(storage.RuntimeDir(queries.Root, run.RunID), 0o755); err != nil {
		t.Fatalf("mkdir runtime dir: %v", err)
	}
	if err := os.WriteFile(storage.RuntimeBriefFile(queries.Root, run.RunID), []byte("runtime"), 0o644); err != nil {
		t.Fatalf("write runtime brief: %v", err)
	}
	if err := os.Chtimes(storage.RuntimeDir(queries.Root, run.RunID), old, old); err != nil {
		t.Fatalf("chtimes runtime dir: %v", err)
	}
	if err := os.Chtimes(storage.RuntimeBriefFile(queries.Root, run.RunID), old, old); err != nil {
		t.Fatalf("chtimes runtime brief: %v", err)
	}

	plan, err := queries.ArchivePlan(ctx, contracts.RetentionTargetRuntime, "APP")
	if err != nil {
		t.Fatalf("archive plan: %v", err)
	}
	if plan.Policy.PolicyID != "runtime-tight" || len(plan.Items) != 1 {
		t.Fatalf("expected project-bound retention policy to drive the plan, got %#v", plan)
	}
}

func TestArchiveListFiltersByProject(t *testing.T) {
	root, actions, queries, projectStore, ticketStore, _ := newImportExportHarness(t)
	ctx := context.Background()
	now := actions.now()
	old := now.AddDate(0, 0, -10)

	for _, project := range []contracts.Project{
		{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion},
		{Key: "OPS", Name: "Ops", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion},
	} {
		if err := projectStore.CreateProject(ctx, project); err != nil {
			t.Fatalf("create project %s: %v", project.Key, err)
		}
	}
	for _, ticket := range []contracts.TicketSnapshot{
		{ID: "APP-1", Project: "APP", Title: "Archive app", Summary: "Archive app", Type: contracts.TicketTypeTask, Status: contracts.StatusDone, Priority: contracts.PriorityHigh, CreatedAt: old, UpdatedAt: old, SchemaVersion: contracts.CurrentSchemaVersion},
		{ID: "OPS-1", Project: "OPS", Title: "Archive ops", Summary: "Archive ops", Type: contracts.TicketTypeTask, Status: contracts.StatusDone, Priority: contracts.PriorityHigh, CreatedAt: old, UpdatedAt: old, SchemaVersion: contracts.CurrentSchemaVersion},
	} {
		if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
			t.Fatalf("create ticket %s: %v", ticket.ID, err)
		}
	}
	for _, run := range []contracts.RunSnapshot{
		normalizeRunSnapshot(contracts.RunSnapshot{RunID: "run_app", TicketID: "APP-1", Project: "APP", Status: contracts.RunStatusCompleted, Kind: contracts.RunKindWork, CreatedAt: old, CompletedAt: old, SchemaVersion: contracts.CurrentSchemaVersion}),
		normalizeRunSnapshot(contracts.RunSnapshot{RunID: "run_ops", TicketID: "OPS-1", Project: "OPS", Status: contracts.RunStatusCompleted, Kind: contracts.RunKindWork, CreatedAt: old, CompletedAt: old, SchemaVersion: contracts.CurrentSchemaVersion}),
	} {
		if err := (RunStore{Root: root}).SaveRun(ctx, run); err != nil {
			t.Fatalf("save run %s: %v", run.RunID, err)
		}
		if err := os.MkdirAll(storage.RuntimeDir(root, run.RunID), 0o755); err != nil {
			t.Fatalf("mkdir runtime dir %s: %v", run.RunID, err)
		}
		if err := os.WriteFile(storage.RuntimeBriefFile(root, run.RunID), []byte(run.Project), 0o644); err != nil {
			t.Fatalf("write runtime brief %s: %v", run.RunID, err)
		}
		if err := os.Chtimes(storage.RuntimeDir(root, run.RunID), old, old); err != nil {
			t.Fatalf("chtimes runtime dir %s: %v", run.RunID, err)
		}
		if err := os.Chtimes(storage.RuntimeBriefFile(root, run.RunID), old, old); err != nil {
			t.Fatalf("chtimes runtime brief %s: %v", run.RunID, err)
		}
	}

	if _, err := actions.ApplyArchive(ctx, contracts.RetentionTargetRuntime, "APP", true, contracts.Actor("human:owner"), "archive app"); err != nil {
		t.Fatalf("apply app archive: %v", err)
	}
	if _, err := actions.ApplyArchive(ctx, contracts.RetentionTargetRuntime, "OPS", true, contracts.Actor("human:owner"), "archive ops"); err != nil {
		t.Fatalf("apply ops archive: %v", err)
	}

	appArchives, err := queries.ListArchiveRecords(ctx, contracts.RetentionTargetRuntime, "APP")
	if err != nil {
		t.Fatalf("list app archives: %v", err)
	}
	if len(appArchives) != 1 || appArchives[0].ProjectKey != "APP" {
		t.Fatalf("expected one APP archive, got %#v", appArchives)
	}

	opsArchives, err := queries.ListArchiveRecords(ctx, contracts.RetentionTargetRuntime, "OPS")
	if err != nil {
		t.Fatalf("list ops archives: %v", err)
	}
	if len(opsArchives) != 1 || opsArchives[0].ProjectKey != "OPS" {
		t.Fatalf("expected one OPS archive, got %#v", opsArchives)
	}
}

func TestAuditOrchestrationSuppressesArchivedRuntimeAndEvidence(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket := contracts.TicketSnapshot{ID: "APP-1", Project: "APP", Title: "Archived runtime", Summary: "Archived runtime", Type: contracts.TicketTypeTask, Status: contracts.StatusDone, Priority: contracts.PriorityHigh, CreatedAt: now, UpdatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}
	if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	run := normalizeRunSnapshot(contracts.RunSnapshot{RunID: "run_archived", TicketID: ticket.ID, Project: ticket.Project, Status: contracts.RunStatusCompleted, Kind: contracts.RunKindWork, CreatedAt: now, CompletedAt: now, SchemaVersion: contracts.CurrentSchemaVersion})
	if err := (RunStore{Root: root}).SaveRun(ctx, run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	evidence := contracts.EvidenceItem{EvidenceID: "evidence_archived", RunID: run.RunID, TicketID: ticket.ID, Type: contracts.EvidenceTypeArtifactRef, Title: "archived artifact", Body: "archived", ArtifactPath: filepath.Join(storage.EvidenceDir(root, run.RunID), "artifact.log"), Actor: contracts.Actor("human:owner"), CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}
	if err := (EvidenceStore{Root: root}).SaveEvidence(ctx, evidence); err != nil {
		t.Fatalf("save evidence: %v", err)
	}
	runtimeArchive := contracts.ArchiveRecord{ArchiveID: "archive_runtime", Target: contracts.RetentionTargetRuntime, Scope: "workspace", ProjectKey: "APP", SourcePaths: []string{filepath.Join(storage.TrackerDirName, "runtime", run.RunID)}, PayloadDir: storage.ArchivePayloadDir(root, "archive_runtime"), ItemCount: 1, TotalBytes: 6, State: contracts.ArchiveRecordArchived, CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}
	evidenceArchive := contracts.ArchiveRecord{ArchiveID: "archive_evidence", Target: contracts.RetentionTargetEvidenceArtifacts, Scope: "workspace", ProjectKey: "APP", SourcePaths: []string{filepath.Join(storage.TrackerDirName, "evidence", run.RunID, "artifact.log")}, PayloadDir: storage.ArchivePayloadDir(root, "archive_evidence"), ItemCount: 1, TotalBytes: 7, State: contracts.ArchiveRecordArchived, CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}
	for _, item := range []struct {
		record contracts.ArchiveRecord
		path   string
		body   string
	}{
		{runtimeArchive, filepath.Join(runtimeArchive.PayloadDir, storage.TrackerDirName, "runtime", run.RunID, "brief.md"), "brief"},
		{evidenceArchive, filepath.Join(evidenceArchive.PayloadDir, storage.TrackerDirName, "evidence", run.RunID, "artifact.log"), "artifact"},
	} {
		if err := os.MkdirAll(filepath.Dir(item.path), 0o755); err != nil {
			t.Fatalf("mkdir payload dir: %v", err)
		}
		if err := os.WriteFile(item.path, []byte(item.body), 0o644); err != nil {
			t.Fatalf("write payload file: %v", err)
		}
		if err := (ArchiveRecordStore{Root: root}).SaveArchiveRecord(ctx, item.record); err != nil {
			t.Fatalf("save archive record: %v", err)
		}
	}

	report, err := AuditOrchestration(ctx, root, ticketStore)
	if err != nil {
		t.Fatalf("audit orchestration: %v", err)
	}
	if slices.Contains(report.IssueCodes, "runtime_dir_missing") {
		t.Fatalf("expected archived runtime to suppress runtime_dir_missing, got %#v", report.IssueCodes)
	}
	if slices.Contains(report.IssueCodes, "evidence_artifact_missing") {
		t.Fatalf("expected archived evidence artifact to suppress evidence_artifact_missing, got %#v", report.IssueCodes)
	}
}
