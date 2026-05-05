package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestCompactWorkspaceRemovesOnlyDerivedArtifacts(t *testing.T) {
	root, actions, _, projectStore, ticketStore, eventsLog := newImportExportHarness(t)
	ctx := context.Background()
	now := actions.now()
	old := now.AddDate(0, 0, -10)

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := ticketStore.CreateTicket(ctx, contracts.TicketSnapshot{ID: "APP-1", Project: "APP", Title: "Compact runtime", Summary: "Compact runtime", Type: contracts.TicketTypeTask, Status: contracts.StatusDone, Priority: contracts.PriorityHigh, CreatedAt: old, UpdatedAt: old, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	run := normalizeRunSnapshot(contracts.RunSnapshot{RunID: "run_compact", TicketID: "APP-1", Project: "APP", Status: contracts.RunStatusCompleted, Kind: contracts.RunKindWork, CreatedAt: old, CompletedAt: old, SchemaVersion: contracts.CurrentSchemaVersion})
	if err := (RunStore{Root: root}).SaveRun(ctx, run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	for path, body := range map[string]string{
		storage.RuntimeBriefFile(root, run.RunID):            "brief",
		storage.RuntimeContextFile(root, run.RunID):          "context",
		storage.RuntimeLaunchFile(root, run.RunID, "codex"):  "codex launch",
		storage.RuntimeLaunchFile(root, run.RunID, "claude"): "claude launch",
		filepath.Join(storage.ArchivePayloadDir(root, "archive_rt"), storage.TrackerDirName, "runtime", run.RunID, "brief.md"):          "archived brief",
		filepath.Join(storage.ArchivePayloadDir(root, "archive_rt"), storage.TrackerDirName, "runtime", run.RunID, "launch.codex.txt"):  "archived codex",
		filepath.Join(storage.ArchivePayloadDir(root, "archive_rt"), storage.TrackerDirName, "runtime", run.RunID, "launch.claude.txt"): "archived claude",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	if err := (ArchiveRecordStore{Root: root}).SaveArchiveRecord(ctx, contracts.ArchiveRecord{ArchiveID: "archive_rt", Target: contracts.RetentionTargetRuntime, Scope: "workspace", ProjectKey: "APP", SourcePaths: []string{filepath.Join(storage.TrackerDirName, "runtime", run.RunID)}, PayloadDir: storage.ArchivePayloadDir(root, "archive_rt"), ItemCount: 1, TotalBytes: 32, State: contracts.ArchiveRecordArchived, CreatedAt: old, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("save archive record: %v", err)
	}

	result, err := actions.CompactWorkspace(ctx, true, contracts.Actor("human:owner"), "compact derived artifacts")
	if err != nil {
		t.Fatalf("compact workspace: %v", err)
	}
	if result.BytesFreed == 0 || len(result.RemovedPaths) < 4 {
		t.Fatalf("unexpected compact result: %#v", result)
	}
	for _, path := range []string{
		storage.RuntimeLaunchFile(root, run.RunID, "codex"),
		storage.RuntimeLaunchFile(root, run.RunID, "claude"),
		filepath.Join(storage.ArchivePayloadDir(root, "archive_rt"), storage.TrackerDirName, "runtime", run.RunID, "launch.codex.txt"),
		filepath.Join(storage.ArchivePayloadDir(root, "archive_rt"), storage.TrackerDirName, "runtime", run.RunID, "launch.claude.txt"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected compacted path removed %s, got err=%v", path, err)
		}
	}
	for _, path := range []string{
		storage.RuntimeBriefFile(root, run.RunID),
		storage.RuntimeContextFile(root, run.RunID),
		filepath.Join(storage.ArchivePayloadDir(root, "archive_rt"), storage.TrackerDirName, "runtime", run.RunID, "brief.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected canonical artifact to remain %s: %v", path, err)
		}
	}
	events, err := eventsLog.StreamEvents(ctx, workspaceProjectKey, 0)
	if err != nil {
		t.Fatalf("stream events: %v", err)
	}
	found := false
	for _, event := range events {
		if event.Type == contracts.EventCompactCompleted {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected compact.completed event")
	}
}
