package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestArchivePayloadPathRejectsEscape(t *testing.T) {
	root := t.TempDir()
	if _, err := archivePayloadPath(root, "archive_1", "../../etc/passwd"); err == nil {
		t.Fatal("expected escaping archive payload path to be rejected")
	}
}

func TestArchiveRestoreRejectsSymlinkedPayloadFile(t *testing.T) {
	root, actions, _, projectStore, ticketStore, _ := newImportExportHarness(t)
	ctx := context.Background()
	now := actions.now()
	old := now.AddDate(0, 0, -10)

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatal(err)
	}
	if err := ticketStore.CreateTicket(ctx, contracts.TicketSnapshot{
		ID: "APP-1", Project: "APP", Title: "Arch", Summary: "Arch", Type: contracts.TicketTypeTask,
		Status: contracts.StatusDone, Priority: contracts.PriorityHigh, CreatedAt: old, UpdatedAt: old,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatal(err)
	}
	run := normalizeRunSnapshot(contracts.RunSnapshot{
		RunID: "run_symlink", TicketID: "APP-1", Project: "APP", Status: contracts.RunStatusCompleted,
		Kind: contracts.RunKindWork, CreatedAt: old, CompletedAt: old, SchemaVersion: contracts.CurrentSchemaVersion,
	})
	if err := (RunStore{Root: root}).SaveRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	live := storage.RuntimeLaunchFile(root, run.RunID, "codex")
	if err := os.MkdirAll(filepath.Dir(live), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(live, []byte("launch"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(live, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(storage.RuntimeDir(root, run.RunID), old, old); err != nil {
		t.Fatal(err)
	}

	applied, err := actions.ApplyArchive(ctx, contracts.RetentionTargetRuntime, "APP", true, contracts.Actor("human:owner"), "archive")
	if err != nil {
		t.Fatal(err)
	}
	payloadFile := filepath.Join(applied.Record.PayloadDir, storage.TrackerDirName, "runtime", run.RunID, "launch.codex.txt")
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(payloadFile); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, payloadFile); err != nil {
		t.Fatal(err)
	}
	if _, err := actions.RestoreArchive(ctx, applied.Record.ArchiveID, contracts.Actor("human:owner"), "restore"); err == nil {
		t.Fatal("expected symlink payload restore to fail")
	}
}
