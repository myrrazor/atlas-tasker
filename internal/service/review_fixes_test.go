package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestValidBackupWorkspaceIDRejectsTraversal(t *testing.T) {
	if validBackupWorkspaceID("../../escape") || validBackupWorkspaceID("foo/bar") || validBackupWorkspaceID("") {
		t.Fatal("traversal and empty ids must be rejected")
	}
	if !validBackupWorkspaceID("ws-setup-test") {
		t.Fatal("ordinary test ids must stay valid")
	}
	id, err := ensureWorkspaceIdentity(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !validBackupWorkspaceID(id) {
		t.Fatalf("generated workspace id %q must be portable", id)
	}
	root := t.TempDir()
	if err := os.MkdirAll(storage.TrackerDir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(storage.WorkspaceMetadataFile(root), []byte(`{"workspace_id":"../../escape"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWorkspaceIdentity(root); err == nil {
		t.Fatal("LoadWorkspaceIdentity must reject a path-like workspace_id")
	}
}

func TestSystemdTimerFiresOnStartup(t *testing.T) {
	unit := systemdTimer("/usr/local/bin/tracker", "/tmp/ws", "ws-1")
	if !strings.Contains(unit, "OnStartupSec=30s") || !strings.Contains(unit, "OnUnitActiveSec=30s") {
		t.Fatalf("systemd timer must start on boot and keep cadence:\n%s", unit)
	}
}

func TestEnableAutoBackupClearsOtherTargetVerified(t *testing.T) {
	ctx, actions := newCheckpointHarness(t)
	remoteA := initBareRemote(t)
	remoteB := initBareRemote(t)
	if _, err := actions.AddBackupTarget(ctx, BackupTargetAddOptions{
		TargetID: "alpha", URL: "file://" + remoteA, AcknowledgeBoundary: true, AttestPrivate: true, AllowLocalFile: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := actions.AddBackupTarget(ctx, BackupTargetAddOptions{
		TargetID: "beta", URL: "file://" + remoteB, AcknowledgeBoundary: true, AttestPrivate: true, AllowLocalFile: true,
	}); err != nil {
		t.Fatal(err)
	}
	paths, _, err := actions.backupPaths()
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := loadLedger(paths.Ledger)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 11, 18, 0, 0, 0, time.UTC)
	ledger.LastVerifiedCommit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	ledger.LastVerifiedAt = now
	ledger.LastVerifiedTargetID = "alpha"
	ledger.LastRemoteCheckpointID = "cp-alpha"
	if err := atomicWriteJSON(paths.Ledger, ledger); err != nil {
		t.Fatal(err)
	}
	if _, err := actions.EnableAutoBackup(ctx, "beta"); err != nil {
		t.Fatal(err)
	}
	status, err := actions.AutoBackupStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.VerifiedRemote {
		t.Fatal("switching the default target must not inherit the previous target's verified state")
	}
	if status.DefaultTargetID != "beta" {
		t.Fatalf("default target=%s", status.DefaultTargetID)
	}
}

func TestBlockedBackupIsNotVerifiedRemote(t *testing.T) {
	ctx, actions := newCheckpointHarness(t)
	remote := initBareRemote(t)
	if _, err := actions.AddBackupTarget(ctx, BackupTargetAddOptions{
		TargetID: "only", URL: "file://" + remote, AcknowledgeBoundary: true, AttestPrivate: true, AllowLocalFile: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := actions.EnableAutoBackup(ctx, "only"); err != nil {
		t.Fatal(err)
	}
	paths, _, err := actions.backupPaths()
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := loadLedger(paths.Ledger)
	if err != nil {
		t.Fatal(err)
	}
	ledger.LastVerifiedCommit = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	ledger.LastVerifiedAt = time.Date(2026, 9, 11, 18, 0, 0, 0, time.UTC)
	ledger.LastVerifiedTargetID = "only"
	ledger.LastErrorClass = contracts.BackupErrorBlockedRemoteDiverged
	if err := atomicWriteJSON(paths.Ledger, ledger); err != nil {
		t.Fatal(err)
	}
	if err := setOutboxState(paths.Outbox, contracts.BackupOutboxBlocked, ledger.LastVerifiedAt); err != nil {
		t.Fatal(err)
	}
	status, err := actions.AutoBackupStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.VerifiedRemote {
		t.Fatal("a blocked replica must not report verified_remote")
	}
	if status.State != contracts.BackupOutboxBlocked {
		t.Fatalf("worker_state=%s", status.State)
	}
}

func TestBackupStatePathsStayUnderStateDir(t *testing.T) {
	state := t.TempDir()
	paths := backupStatePaths(state, "ws-ok")
	if !strings.HasPrefix(paths.Root, state) {
		t.Fatalf("backup root escaped state dir: %s", paths.Root)
	}
	if filepath.Base(paths.Root) != "ws-ok" {
		t.Fatalf("unexpected backup root %s", paths.Root)
	}
}

func TestPinnedGitArgsDisableFiltersAndCRLF(t *testing.T) {
	args := gitRunner{Hooks: "/tmp/hooks"}.pinnedArgs("hash-object", "-w", "--no-filters", "--", "file")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "core.autocrlf=false") || !strings.Contains(joined, "core.eol=lf") {
		t.Fatalf("pinned git args must disable autocrlf and pin LF: %s", joined)
	}
}

func TestResetReplicaClearsVerified(t *testing.T) {
	ctx, actions := newCheckpointHarness(t)
	remote := initBareRemote(t)
	if _, err := actions.AddBackupTarget(ctx, BackupTargetAddOptions{
		TargetID: "only", URL: "file://" + remote, AcknowledgeBoundary: true, AttestPrivate: true, AllowLocalFile: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := actions.EnableAutoBackup(ctx, "only"); err != nil {
		t.Fatal(err)
	}
	if _, err := actions.BackupTick(ctx, true); err != nil {
		t.Fatal(err)
	}
	before, err := actions.AutoBackupStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !before.VerifiedRemote {
		t.Fatal("expected a verified remote before replica reset")
	}
	if _, err := actions.ResetReplica(ctx, true); err != nil {
		t.Fatal(err)
	}
	after, err := actions.AutoBackupStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.VerifiedRemote {
		t.Fatal("replica reset must not keep the previous replica's verified state")
	}
}

func TestCompactContinuesWhenOnlyRemotePublishFails(t *testing.T) {
	ctx, actions := newCheckpointHarness(t)
	if _, err := actions.BackupTick(ctx, true); err != nil {
		t.Fatal(err)
	}
	remote := initBareRemote(t)
	if _, err := actions.AddBackupTarget(ctx, BackupTargetAddOptions{
		TargetID: "gone", URL: "file://" + remote, AcknowledgeBoundary: true, AttestPrivate: true, AllowLocalFile: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := actions.EnableAutoBackup(ctx, "gone"); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(remote); err != nil {
		t.Fatal(err)
	}
	actions.RequirePreDestructiveCheckpoint = true
	if _, err := actions.CompactWorkspace(ctx, true, contracts.Actor("human:owner"), "compact after local checkpoint"); err != nil {
		t.Fatalf("compact must proceed from the local checkpoint when only remote publish fails: %v", err)
	}
}
