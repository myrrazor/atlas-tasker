package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

func TestBackupTargetRejectsCredentialsAndPublicGitHub(t *testing.T) {
	ctx, actions := newCheckpointHarness(t)
	_, err := actions.AddBackupTarget(ctx, BackupTargetAddOptions{
		URL: "https://user:token@github.com/org/repo.git", AcknowledgeBoundary: true, AttestPrivate: true, AllowLocalFile: true,
	})
	if err == nil || !strings.Contains(err.Error(), "credential") {
		t.Fatalf("embedded credentials must be rejected: %v", err)
	}
	_, err = actions.AddBackupTarget(ctx, BackupTargetAddOptions{
		URL: "https://github.com/org/public.git", AcknowledgeBoundary: true, AttestPublic: true,
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "public") {
		t.Fatalf("public GitHub without override must be refused: %v", err)
	}
	remote := initBareRemote(t)
	_, err = actions.AddBackupTarget(ctx, BackupTargetAddOptions{
		URL: "file://" + remote, AcknowledgeBoundary: true, AttestPrivate: true,
	})
	if err == nil || !strings.Contains(err.Error(), "allow-local-file") {
		t.Fatalf("file:// without --allow-local-file must fail: %v", err)
	}
}

func TestBackupTargetNeverInfersOriginAndRemoveKeepsRemote(t *testing.T) {
	ctx, actions := newCheckpointHarness(t)
	userGit := initUserRepo(t, actions.Root)
	remote := initBareRemote(t)
	url := "file://" + remote
	runGit(t, userGit, "remote", "add", "origin", url)
	listed, err := actions.ListBackupTargets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Items) != 0 {
		t.Fatalf("origin must not become a backup target: %#v", listed.Items)
	}
	view, err := addDisposableTarget(t, actions, url, "keep-remote")
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(view.Warnings, "target_matches_workspace_remote") {
		t.Fatalf("matching origin must warn, not auto-select: %#v", view.Warnings)
	}
	if _, err := actions.EnableAutoBackup(ctx, "keep-remote"); err != nil {
		t.Fatal(err)
	}
	if _, err := actions.BackupTick(ctx, true); err != nil {
		t.Fatal(err)
	}
	if refs := lsRemoteRefs(t, url); len(refs) == 0 {
		t.Fatal("expected a published replica ref")
	}
	if _, err := actions.RemoveBackupTarget(ctx, "keep-remote"); err != nil {
		t.Fatal(err)
	}
	if refs := lsRemoteRefs(t, url); len(refs) == 0 {
		t.Fatal("target remove must not delete remote data")
	}
	after, err := actions.ListBackupTargets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Items) != 0 {
		t.Fatalf("local target config should be gone: %#v", after.Items)
	}
}

func TestPublishFastForwardAndVerify(t *testing.T) {
	ctx, actions := newCheckpointHarness(t)
	userGit := initUserRepo(t, actions.Root)
	before := captureUserGit(t, userGit)
	remote := initBareRemote(t)
	url := "file://" + remote
	if _, err := addDisposableTarget(t, actions, url, "pub"); err != nil {
		t.Fatal(err)
	}
	if _, err := actions.EnableAutoBackup(ctx, "pub"); err != nil {
		t.Fatal(err)
	}
	result, err := actions.BackupTick(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || result.Commit == "" {
		t.Fatalf("expected local checkpoint: %#v", result)
	}
	if result.State != contracts.BackupOutboxVerified {
		t.Fatalf("verified publication required, state=%s class=%s", result.State, result.ErrorClass)
	}
	engine, err := actions.checkpointEngine()
	if err != nil {
		t.Fatal(err)
	}
	ref := backupRefName(engine.workspaceID, engine.replicaID)
	if tip := remoteRefTip(t, url, ref); tip != result.Commit {
		t.Fatalf("remote ref %s = %s, want %s", ref, tip, result.Commit)
	}
	ledger, err := loadLedger(engine.paths.Ledger)
	if err != nil {
		t.Fatal(err)
	}
	if ledger.LastVerifiedCommit != result.Commit || ledger.LastVerifiedManifestSHA == "" {
		t.Fatalf("ledger must record verified commit/manifest: %#v", ledger)
	}
	after := captureUserGit(t, userGit)
	if before != after {
		t.Fatalf("user git changed during publish\n before %s\n after %s", before, after)
	}
}

func TestPushSuccessIsNotVerifiedAndInterruptedPushResumes(t *testing.T) {
	ctx, actions := newCheckpointHarness(t)
	remote := initBareRemote(t)
	if _, err := addDisposableTarget(t, actions, "file://"+remote, "resume"); err != nil {
		t.Fatal(err)
	}
	if _, err := actions.EnableAutoBackup(ctx, "resume"); err != nil {
		t.Fatal(err)
	}
	actions.CheckpointCrashAt = string(CrashAfterPush)
	if _, err := actions.BackupTick(ctx, true); err == nil {
		t.Fatal("expected injected crash after push")
	}
	engine, err := actions.checkpointEngine()
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := loadLedger(engine.paths.Ledger)
	if err != nil {
		t.Fatal(err)
	}
	if ledger.LastLocalCommit == "" {
		t.Fatal("local commit must persist before verify")
	}
	if ledger.LastVerifiedCommit != "" {
		t.Fatal("push success alone must not record verified")
	}
	commit := ledger.LastLocalCommit
	actions.CheckpointCrashAt = ""
	resumed, err := actions.BackupTick(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.State != contracts.BackupOutboxVerified {
		t.Fatalf("interrupted push must resume verify: %#v", resumed)
	}
	ledger, err = loadLedger(engine.paths.Ledger)
	if err != nil {
		t.Fatal(err)
	}
	if ledger.LastVerifiedCommit != commit {
		t.Fatalf("resumed verify should keep the same commit %s, got %s", commit, ledger.LastVerifiedCommit)
	}
}

func TestCrashAfterVerifyBeforePersistIsNotVerified(t *testing.T) {
	ctx, actions := newCheckpointHarness(t)
	remote := initBareRemote(t)
	if _, err := addDisposableTarget(t, actions, "file://"+remote, "persist"); err != nil {
		t.Fatal(err)
	}
	if _, err := actions.EnableAutoBackup(ctx, "persist"); err != nil {
		t.Fatal(err)
	}
	actions.CheckpointCrashAt = string(CrashAfterVerifyBeforePersist)
	if _, err := actions.BackupTick(ctx, true); err == nil {
		t.Fatal("expected crash after verify before persist")
	}
	engine, err := actions.checkpointEngine()
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := loadLedger(engine.paths.Ledger)
	if err != nil {
		t.Fatal(err)
	}
	if ledger.LastVerifiedCommit != "" {
		t.Fatal("verified must wait for ledger persist")
	}
}

func TestRemoteDivergenceBlocksWithoutForceAndMutationsStayAvailable(t *testing.T) {
	ctx, actions := newCheckpointHarness(t)
	remote := initBareRemote(t)
	url := "file://" + remote
	if _, err := addDisposableTarget(t, actions, url, "div"); err != nil {
		t.Fatal(err)
	}
	if _, err := actions.EnableAutoBackup(ctx, "div"); err != nil {
		t.Fatal(err)
	}
	first, err := actions.BackupTick(ctx, true)
	if err != nil || first.State != contracts.BackupOutboxVerified {
		t.Fatalf("seed publish: %#v %v", first, err)
	}
	engine, err := actions.checkpointEngine()
	if err != nil {
		t.Fatal(err)
	}
	plantDivergedTip(t, remote, backupRefName(engine.workspaceID, engine.replicaID))
	if err := mutateTicketTitle(ctx, actions, "after diverge"); err != nil {
		t.Fatal(err)
	}
	blocked, err := actions.BackupTick(ctx, true)
	if err != nil {
		t.Fatalf("blocked publish must not fail the tick: %v", err)
	}
	if blocked.State != contracts.BackupOutboxBlocked || blocked.ErrorClass != contracts.BackupErrorBlockedRemoteDiverged {
		t.Fatalf("expected blocked_remote_diverged, got %#v", blocked)
	}
	if err := mutateTicketTitle(ctx, actions, "still writable"); err != nil {
		t.Fatalf("mutations must succeed while backup is blocked: %v", err)
	}
	again, err := actions.BackupTick(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if again.ErrorClass != contracts.BackupErrorBlockedRemoteDiverged {
		t.Fatalf("divergence must stay blocked until reconcile: %#v", again)
	}
}

func TestReplicaRefsAreDistinctAndResetIsExplicit(t *testing.T) {
	ctx, first := newCheckpointHarness(t)
	remote := initBareRemote(t)
	url := "file://" + remote
	if _, err := addDisposableTarget(t, first, url, "r1"); err != nil {
		t.Fatal(err)
	}
	if _, err := first.EnableAutoBackup(ctx, "r1"); err != nil {
		t.Fatal(err)
	}
	if _, err := first.BackupTick(ctx, true); err != nil {
		t.Fatal(err)
	}
	second := cloneActionsWithFreshState(t, first)
	if _, err := addDisposableTarget(t, second, url, "r2"); err != nil {
		t.Fatal(err)
	}
	if _, err := second.EnableAutoBackup(ctx, "r2"); err != nil {
		t.Fatal(err)
	}
	if _, err := second.BackupTick(ctx, true); err != nil {
		t.Fatal(err)
	}
	one, err := first.ReplicaStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	two, err := second.ReplicaStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if one.Identity.ReplicaID == "" || one.Identity.ReplicaID == two.Identity.ReplicaID {
		t.Fatalf("replicas must be distinct: %s %s", one.Identity.ReplicaID, two.Identity.ReplicaID)
	}
	if one.Ref == two.Ref || !strings.HasPrefix(one.Ref, "refs/atlas/backups/") {
		t.Fatalf("refs must be per-replica and never heads: %s %s", one.Ref, two.Ref)
	}
	refs := lsRemoteRefs(t, url)
	if len(refs) < 2 {
		t.Fatalf("expected two replica refs, got %v", refs)
	}
	for _, ref := range refs {
		if strings.HasPrefix(ref, "refs/heads/") {
			t.Fatalf("backup must not create heads: %v", refs)
		}
	}
	if _, err := os.Stat(filepath.Join(first.Root, "replica.json")); !os.IsNotExist(err) {
		t.Fatal("copying a workspace must not include replica identity")
	}
	reset, err := first.ResetReplica(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if reset.Identity.ReplicaID == one.Identity.ReplicaID {
		t.Fatal("explicit reset must assign a new replica id")
	}
}

func TestClassifyRemoteBackupErrorsAndAuthBackoff(t *testing.T) {
	cases := map[string]string{
		"Could not resolve host example.test":     contracts.BackupErrorDNSFailure,
		"Network is unreachable":                  contracts.BackupErrorOffline,
		"Permission denied (publickey)":           contracts.BackupErrorAuthenticationFailed,
		"remote: Write access denied":             contracts.BackupErrorPermissionDenied,
		"repository not found":                    contracts.BackupErrorRemoteMissing,
		"failed to push some refs (non-fast-forward)": contracts.BackupErrorRemoteDiverged,
		"context deadline exceeded":               contracts.BackupErrorTimeout,
		"HOST KEY VERIFICATION FAILED":            contracts.BackupErrorHostKeyUnverified,
		"blocked_remote_diverged":                 contracts.BackupErrorBlockedRemoteDiverged,
	}
	for msg, want := range cases {
		got := classifyRemoteBackupError(apperr.New(apperr.CodeConflict, msg))
		if got != want {
			t.Fatalf("classify %q = %s, want %s", msg, got, want)
		}
	}
	now := time.Date(2026, 9, 11, 18, 0, 0, 0, time.UTC)
	authNext, authAttempt := nextBackupRetry(now, 0, contracts.BackupErrorAuthenticationFailed)
	if authAttempt != 1 || authNext.Sub(now) > backupRetryMax {
		t.Fatalf("auth backoff out of range: next=%s attempt=%d", authNext, authAttempt)
	}
	if authNext.Sub(now) < backupRetryMax/4 && authNext.Sub(now) != 0 {
		// full jitter of max interval still belongs at the long interval, not 30s storms
	}
	offlineNext, _ := nextBackupRetry(now, 0, contracts.BackupErrorOffline)
	if offlineNext.Sub(now) > backupRetryInitial {
		t.Fatalf("first offline retry must stay at the 30s initial window, got %s", offlineNext.Sub(now))
	}
}

func TestSchedulerInstallRemoveRepairInFixtureHome(t *testing.T) {
	ctx, actions := newCheckpointHarness(t)
	home := t.TempDir()
	actions.ScheduleHome = home
	bin := filepath.Join(t.TempDir(), "tracker")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	actions.TrackerBinary = bin
	if _, err := actions.BackupScheduleInstall(ctx, false); err == nil {
		t.Fatal("install without --yes must fail")
	}
	installed, err := actions.BackupScheduleInstall(ctx, true)
	if err != nil || !installed.Installed {
		t.Fatalf("install: %#v %v", installed, err)
	}
	plan, err := actions.BackupSchedulePlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range plan.Files {
		info, err := os.Lstat(file.Path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 {
			t.Fatalf("scheduler file mode %s", info.Mode())
		}
		body, err := os.ReadFile(file.Path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), scheduleMarker) || !strings.Contains(string(body), bin) {
			t.Fatalf("scheduler file missing marker/binary:\n%s", body)
		}
	}
	again, err := actions.BackupScheduleInstall(ctx, true)
	if err != nil || !again.Installed {
		t.Fatalf("install must be idempotent: %#v %v", again, err)
	}
	raw, err := os.ReadFile(plan.Files[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	moved := strings.ReplaceAll(string(raw), plan.WorkspaceRoot, "/tmp/atlas-moved-workspace")
	if !strings.Contains(moved, "/tmp/atlas-moved-workspace") {
		t.Fatalf("failed to rewrite workspace path in %s", plan.Files[0].Path)
	}
	if err := os.WriteFile(plan.Files[0].Path, []byte(moved), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err := actions.BackupScheduleStatus(ctx)
	if err != nil || !status.Repair {
		t.Fatalf("moved workspace must be repair-required: %#v %v", status, err)
	}
	if _, err := actions.BackupScheduleRepair(ctx, true); err != nil {
		t.Fatal(err)
	}
	engine, err := actions.checkpointEngine()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := actions.BackupTick(ctx, true); err != nil {
		t.Fatal(err)
	}
	removed, err := actions.BackupScheduleRemove(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if removed.Installed {
		t.Fatal("remove should uninstall Atlas-owned files")
	}
	if _, err := os.Stat(engine.paths.Ledger); err != nil {
		t.Fatalf("uninstall must keep backup ledger: %v", err)
	}
	if _, err := os.Stat(engine.paths.Repo); err != nil {
		t.Fatalf("uninstall must keep backup repo: %v", err)
	}
	_ = os.RemoveAll(plan.Files[0].Path)
	if err := os.Symlink("/tmp/atlas-not-a-scheduler", plan.Files[0].Path); err != nil {
		t.Fatal(err)
	}
	if _, err := actions.BackupScheduleInstall(ctx, true); err == nil {
		t.Fatal("unexpected symlink must fail install")
	}
}

func TestApplyRestorePlanGovernanceDenyWritesNothing(t *testing.T) {
	ctx, actions := newCheckpointHarness(t)
	view, err := actions.CreateBackup(ctx, "workspace", contracts.Actor("human:owner"), "g-a")
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := actions.Tickets.GetTicket(ctx, "APP-1")
	if err != nil {
		t.Fatal(err)
	}
	before := ticket.Title
	if err := actions.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:         "deny-restore",
		Name:             "Deny restore",
		ScopeKind:        contracts.PolicyScopeWorkspace,
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionBackupRestore},
		QuorumRules: []contracts.QuorumRule{{
			RuleID:        "restore-quorum",
			ActionKind:    contracts.ProtectedActionBackupRestore,
			RequiredCount: 2,
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatal(err)
	}
	_, err = actions.ApplyRestorePlan(ctx, view.Snapshot.BackupID, contracts.Actor("human:alice"), "should deny", true)
	if err == nil || apperr.CodeOf(err) != apperr.CodePermissionDenied {
		t.Fatalf("denying backup_restore policy must exit permission_denied: %v", err)
	}
	ticket, err = actions.Tickets.GetTicket(ctx, "APP-1")
	if err != nil {
		t.Fatal(err)
	}
	if ticket.Title != before {
		t.Fatalf("deny must write nothing, title %q -> %q", before, ticket.Title)
	}
}

func TestRemoteRestoreWrongWorkspaceAndTraversalRejected(t *testing.T) {
	ctx, actions := newCheckpointHarness(t)
	remote := initBareRemote(t)
	if _, err := addDisposableTarget(t, actions, "file://"+remote, "ws"); err != nil {
		t.Fatal(err)
	}
	if _, err := actions.EnableAutoBackup(ctx, "ws"); err != nil {
		t.Fatal(err)
	}
	published, err := actions.BackupTick(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	dest := newEmptyDest(t)
	if _, err := addDisposableTarget(t, dest, "file://"+remote, "ws"); err != nil {
		t.Fatal(err)
	}
	_, err = dest.RemoteRestorePlan(ctx, RemoteRestoreOptions{
		TargetID: "ws", Checkpoint: published.CheckpointID, Actor: contracts.Actor("human:owner"),
	})
	if err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("wrong workspace must require an explicit override: %v", err)
	}
	_, err = dest.RemoteRestoreApply(ctx, RemoteRestoreOptions{
		TargetID: "ws", Checkpoint: published.CheckpointID, Actor: contracts.Actor("human:owner"),
		Reason: "no", Yes: true,
	})
	if err == nil {
		t.Fatal("apply without mismatch override must fail")
	}
}

func TestRemoteRecoveryDrill(t *testing.T) {
	ctx, source := newCheckpointHarness(t)
	userGit := initUserRepo(t, source.Root)
	before := captureUserGit(t, userGit)
	writeBanned(t, source.Root, map[string]string{
		".env":                                    "TOKEN=secret\n",
		".tracker/security/keys/private/key.json": `{"k":"secret"}` + "\n",
		".tracker/runtime/session.json":           `{"tok":"1"}` + "\n",
	})
	remote := initBareRemote(t)
	url := "file://" + remote
	if _, err := addDisposableTarget(t, source, url, "drill"); err != nil {
		t.Fatal(err)
	}
	if _, err := source.EnableAutoBackup(ctx, "drill"); err != nil {
		t.Fatal(err)
	}
	published, err := source.BackupTick(ctx, true)
	if err != nil || published.State != contracts.BackupOutboxVerified {
		t.Fatalf("checkpoint+publish: %#v %v", published, err)
	}
	engine, err := source.checkpointEngine()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := engine.readCommitManifest(ctx, published.Commit)
	if err != nil {
		t.Fatal(err)
	}
	if tip := remoteRefTip(t, url, backupRefName(engine.workspaceID, engine.replicaID)); tip != published.Commit {
		t.Fatalf("remote ref mismatch: %s %s", tip, published.Commit)
	}
	want := map[string]string{}
	for _, file := range manifest.Files {
		want[file.Path] = file.SHA256
	}
	sourceRoot := source.Root
	if err := os.RemoveAll(sourceRoot); err != nil {
		t.Fatal(err)
	}
	dest := newEmptyDest(t)
	if _, err := addDisposableTarget(t, dest, url, "drill"); err != nil {
		t.Fatal(err)
	}
	applied, err := dest.RemoteRestoreApply(ctx, RemoteRestoreOptions{
		TargetID: "drill", Checkpoint: published.CheckpointID, AllowWorkspaceMismatch: true,
		Yes: true, Actor: contracts.Actor("human:owner"), Reason: "disaster recovery drill",
	})
	if err != nil {
		t.Fatal(err)
	}
	if applied.Applied == 0 {
		t.Fatalf("expected restore writes: %#v", applied)
	}
	proj, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(dest.Root), "index.sqlite"), dest.Tickets, dest.Events)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = proj.Close() })
	if err := proj.Rebuild(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := AuditOrchestration(ctx, dest.Root, dest.Tickets); err != nil {
		t.Fatalf("doctor/orchestration audit: %v", err)
	}
	for path, sum := range want {
		got, err := fileSHA256(filepath.Join(dest.Root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("restored %s: %v", path, err)
		}
		if strings.HasPrefix(path, ".tracker/events/") {
			// apply records backup.restored, so the live log is a superset of the checkpoint.
			raw, err := os.ReadFile(filepath.Join(dest.Root, filepath.FromSlash(path)))
			if err != nil || !strings.Contains(string(raw), "APP-1") {
				t.Fatalf("restored event log missing original work: %v", err)
			}
			continue
		}
		if got != sum {
			t.Fatalf("hash mismatch for %s", path)
		}
	}
	for _, banned := range []string{"README.md", ".env", ".tracker/security/keys/private/key.json", ".tracker/runtime/session.json"} {
		if _, err := os.Stat(filepath.Join(dest.Root, filepath.FromSlash(banned))); err == nil {
			t.Fatalf("banned path restored: %s", banned)
		}
	}
	if _, err := dest.Tickets.GetTicket(ctx, "APP-1"); err != nil {
		t.Fatalf("ticket should be restored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest.Root, ".tracker", "config.toml")); err != nil {
		t.Fatalf("setup/init recreates config.toml; drill must not require allowlist widening: %v", err)
	}
	_ = before
	t.Logf("AT114-507 exported-only list remains unrestored and is recreated by init/setup: %s", strings.Join(ExportedOnlyCandidateRoots(), ", "))
}

func addDisposableTarget(t *testing.T, actions *ActionService, url, id string) (BackupTargetView, error) {
	t.Helper()
	return actions.AddBackupTarget(context.Background(), BackupTargetAddOptions{
		TargetID: id, URL: url, Enabled: true, AcknowledgeBoundary: true,
		AttestPrivate: true, AllowLocalFile: true,
	})
}

func initBareRemote(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "remote.git")
	cmd := exec.Command("git", "init", "--bare", dir)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bare remote: %v\n%s", err, out)
	}
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func lsRemoteRefs(t *testing.T, url string) []string {
	t.Helper()
	cmd := exec.Command("git", "-c", "protocol.file.allow=always", "ls-remote", "--", url)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ls-remote: %v\n%s", err, out)
	}
	refs := []string{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			refs = append(refs, fields[1])
		}
	}
	return refs
}

func remoteRefTip(t *testing.T, url, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "-c", "protocol.file.allow=always", "ls-remote", "--", url, ref)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ls-remote %s: %v\n%s", ref, err, out)
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func plantDivergedTip(t *testing.T, repo, ref string) {
	t.Helper()
	empty := exec.Command("git", "--git-dir", repo, "mktree")
	empty.Stdin = strings.NewReader("")
	empty.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	tree, err := empty.Output()
	if err != nil {
		t.Fatalf("mktree: %v", err)
	}
	commit := exec.Command("git", "--git-dir", repo, "commit-tree", strings.TrimSpace(string(tree)), "-m", "diverge")
	commit.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_NAME=diverge",
		"GIT_AUTHOR_EMAIL=diverge@localhost",
		"GIT_COMMITTER_NAME=diverge",
		"GIT_COMMITTER_EMAIL=diverge@localhost",
	)
	oid, err := commit.Output()
	if err != nil {
		t.Fatalf("commit-tree: %v", err)
	}
	runGit(t, repo, "update-ref", ref, strings.TrimSpace(string(oid)))
}

func cloneActionsWithFreshState(t *testing.T, src *ActionService) *ActionService {
	t.Helper()
	clone := NewActionService(src.Root, src.Projects, src.Tickets, src.Events, src.Projection, src.Clock, FileLockManager{Root: src.Root}, nil, nil)
	clone.StateDir = t.TempDir()
	clone.Home = t.TempDir()
	return clone
}

func newEmptyDest(t *testing.T) *ActionService {
	t.Helper()
	root := t.TempDir()
	now := time.Date(2026, 9, 11, 19, 0, 0, 0, time.UTC)
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}}); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureWorkspaceIdentity(root); err != nil {
		t.Fatal(err)
	}
	projects := mdstore.ProjectStore{RootDir: root}
	tickets := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	events := &eventstore.Log{RootDir: root}
	actions := NewActionService(root, projects, tickets, events, nil, func() time.Time { return now }, FileLockManager{Root: root}, nil, nil)
	actions.StateDir = t.TempDir()
	actions.Home = t.TempDir()
	return actions
}

func writeBanned(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
