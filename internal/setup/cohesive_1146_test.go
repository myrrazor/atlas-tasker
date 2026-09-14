package setup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/integrations"
)

func TestCohesiveSetupRunsFirstCheckpointWhenRequested(t *testing.T) {
	engine := testEngine(t)
	engine.Hooks.BackupFirstCheckpoint = func(_ context.Context, targetID string) (FirstBackupResult, error) {
		if targetID != "local-drill" {
			t.Fatalf("target %s", targetID)
		}
		return FirstBackupResult{CheckpointID: "cp-1146", Verified: true}, nil
	}
	prepared, err := engine.Plan(PlanOptions{
		Agents: []integrations.Target{integrations.TargetGeneric},
		Mode:   contracts.ManagedModeManaged,
		Backup: true, BackupTarget: "local-drill",
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Plan.Backup == nil || !prepared.Plan.Backup.Requested {
		t.Fatal("backup group missing")
	}
	joined := strings.Join(prepared.Plan.Backup.Notes, "\n")
	if !strings.Contains(joined, "scheduler install remains a separate explicit consent") {
		t.Fatalf("scheduler must stay a separate consent: %s", joined)
	}
	report, err := engine.ApplyPrepared(context.Background(), prepared, ApplyOptions{Yes: true})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if report.LastVerified != "cp-1146" {
		t.Fatalf("last_verified=%q", report.LastVerified)
	}
	if report.BackupWorker != "enabled" {
		t.Fatalf("worker=%s", report.BackupWorker)
	}
	if _, err := os.Stat(filepath.Join(engine.WorkspaceRoot, ".tracker", "managed-mode.json")); err != nil {
		t.Fatal("managed mode should be written")
	}
	if _, err := os.Stat(filepath.Join(engine.WorkspaceRoot, ".tracker", "integrations", "generic-agent-skill", "SKILL.md")); err != nil {
		t.Fatal("generic skill should be written")
	}
}

func TestBackupFirstCheckpointFailureKeepsAgents(t *testing.T) {
	engine := testEngine(t)
	engine.Hooks.BackupFirstCheckpoint = func(context.Context, string) (FirstBackupResult, error) {
		return FirstBackupResult{}, os.ErrNotExist
	}
	prepared, err := engine.Plan(PlanOptions{
		Agents: []integrations.Target{integrations.TargetGeneric},
		Backup: true, BackupTarget: "missing",
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := engine.ApplyPrepared(context.Background(), prepared, ApplyOptions{Yes: true})
	if err == nil || report.Status != RunStatusPartial {
		t.Fatalf("status=%s err=%v", report.Status, err)
	}
	if _, err := os.Stat(filepath.Join(engine.WorkspaceRoot, "AGENTS.md")); err != nil {
		t.Fatal("agents must remain")
	}
	if report.LastVerified != "" {
		t.Fatalf("failed verify must not populate last_verified: %s", report.LastVerified)
	}
}

func TestUnverifiedFirstCheckpointDoesNotClaimLastVerified(t *testing.T) {
	engine := testEngine(t)
	engine.Hooks.BackupFirstCheckpoint = func(context.Context, string) (FirstBackupResult, error) {
		return FirstBackupResult{CheckpointID: "cp-local-only", Verified: false}, nil
	}
	prepared, err := engine.Plan(PlanOptions{
		Agents: []integrations.Target{integrations.TargetGeneric},
		Backup: true, BackupTarget: "local-drill",
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := engine.ApplyPrepared(context.Background(), prepared, ApplyOptions{Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.LastVerified != "" {
		t.Fatalf("unverified first checkpoint must not set last_verified: %s", report.LastVerified)
	}
	if report.Status == RunStatusConnected {
		t.Fatal("unverified backup must not make the run connected")
	}
}

func TestBackupStatusFieldsRequireVerifiedCommit(t *testing.T) {
	engine := testEngine(t)
	dir := filepath.Join(engine.StateDir, "backups", engine.WorkspaceID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "auto.json"), []byte(`{"format":"atlas_backup_auto_v1","enabled":true}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ledger.json"), []byte(`{"format":"atlas_backup_ledger_v1","last_remote_checkpoint_id":"cp-pushed-only"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	worker, last := engine.backupStatusFields()
	if worker != "enabled" || last != "" {
		t.Fatalf("push-only ledger must not populate last_verified: worker=%s last=%s", worker, last)
	}
}
