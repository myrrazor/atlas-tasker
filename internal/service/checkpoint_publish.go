package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func (e *CheckpointEngine) publishIfEnabled(ctx context.Context, commit string, force bool) error {
	if e == nil || e.actions == nil {
		return nil
	}
	target, cfg, err := e.actions.defaultEnabledTarget()
	if err != nil {
		return err
	}
	if !cfg.Enabled || strings.TrimSpace(target.TargetID) == "" || !target.Enabled {
		return nil
	}
	if strings.TrimSpace(commit) == "" {
		ledger, loadErr := loadLedger(e.paths.Ledger)
		if loadErr != nil {
			return loadErr
		}
		commit = ledger.LastLocalCommit
	}
	if strings.TrimSpace(commit) == "" {
		return nil
	}
	ledger, err := loadLedger(e.paths.Ledger)
	if err != nil {
		return err
	}
	if ledger.LastVerifiedCommit == commit && ledger.LastVerifiedTargetID == target.TargetID && !ledger.LastVerifiedAt.IsZero() {
		return nil
	}
	if ledger.BlockedReason == contracts.BackupErrorBlockedRemoteDiverged || ledger.LastErrorClass == contracts.BackupErrorBlockedRemoteDiverged {
		if !force {
			return apperr.New(apperr.CodeConflict, contracts.BackupErrorBlockedRemoteDiverged)
		}
	}
	if !force && !ledger.NextRetryAt.IsZero() && e.now().Before(ledger.NextRetryAt) {
		return nil
	}
	timeout := 60 * time.Second
	if target.TransportTimeoutMS > 0 {
		timeout = time.Duration(target.TransportTimeoutMS) * time.Millisecond
	}
	pubCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := e.crash(CrashBeforePush); err != nil {
		return err
	}
	if err := setOutboxState(e.paths.Outbox, contracts.BackupOutboxPushPending, e.now()); err != nil {
		return err
	}
	if err := e.publishAndVerify(pubCtx, target, commit); err != nil {
		class := classifyRemoteBackupError(err)
		e.recordPublishFailure(target.TargetID, class, err.Error())
		if class == contracts.BackupErrorRemoteDiverged || class == contracts.BackupErrorBlockedRemoteDiverged {
			return apperr.New(apperr.CodeConflict, contracts.BackupErrorBlockedRemoteDiverged)
		}
		return err
	}
	return nil
}

func (e *CheckpointEngine) publishAndVerify(ctx context.Context, target contracts.BackupTarget, commit string) error {
	ref := backupRefName(e.workspaceID, e.replicaID)
	runner := e.gitRunner(e.paths.Tmp, "")
	if err := e.ensureBareRepo(ctx); err != nil {
		return err
	}
	remoteTip, err := e.lsRemote(ctx, runner, target.URL, ref)
	if err != nil {
		return err
	}
	if remoteTip != "" && remoteTip != commit {
		if err := e.confirmAncestry(ctx, runner, target.URL, remoteTip, commit, ref); err != nil {
			return err
		}
	}
	if remoteTip != commit {
		if _, err := runner.run(ctx, "push", "--", target.URL, commit+":"+ref); err != nil {
			return err
		}
	}
	if err := e.crash(CrashAfterPush); err != nil {
		return err
	}
	if err := setOutboxState(e.paths.Outbox, contracts.BackupOutboxPushedUnverified, e.now()); err != nil {
		return err
	}
	if err := e.crash(CrashBeforeRemoteVerify); err != nil {
		return err
	}
	observed, err := e.lsRemote(ctx, runner, target.URL, ref)
	if err != nil {
		return err
	}
	if observed != commit {
		if observed == "" {
			return apperr.New(apperr.CodeConflict, contracts.BackupErrorRemoteRefMissing)
		}
		return apperr.New(apperr.CodeConflict, "remote ref does not equal the expected commit")
	}
	manifest, err := e.verifyRemoteObjects(ctx, target.URL, ref, commit)
	if err != nil {
		return err
	}
	if err := e.crash(CrashAfterVerifyBeforePersist); err != nil {
		return err
	}
	return e.markVerified(commit, manifest, target.TargetID)
}

func (e *CheckpointEngine) verifyRemoteObjects(ctx context.Context, url, ref, commit string) (contracts.CheckpointManifest, error) {
	if err := os.MkdirAll(e.paths.Tmp, 0o700); err != nil {
		return contracts.CheckpointManifest{}, err
	}
	dest, err := os.MkdirTemp(e.paths.Tmp, "verify-*")
	if err != nil {
		return contracts.CheckpointManifest{}, err
	}
	defer func() { _ = os.RemoveAll(dest) }()
	init := exec.CommandContext(ctx, e.git, "init", "--bare", dest)
	init.Env = gitSafeEnv(e.git, dest, "", "")
	if out, err := init.CombinedOutput(); err != nil {
		return contracts.CheckpointManifest{}, fmt.Errorf("git init --bare verify repo: %w: %s", err, strings.TrimSpace(string(out)))
	}
	runner := gitRunner{Git: e.git, Repo: dest, Snapshot: dest}
	verifyRef := "refs/atlas/verify/" + commit
	if _, err := runner.run(ctx, "fetch", "--", url, ref+":"+verifyRef); err != nil {
		return contracts.CheckpointManifest{}, err
	}
	fetched, err := runner.run(ctx, "rev-parse", verifyRef)
	if err != nil {
		return contracts.CheckpointManifest{}, err
	}
	if fetched != commit {
		return contracts.CheckpointManifest{}, apperr.New(apperr.CodeConflict, "fetched commit does not match the pushed commit")
	}
	if _, err := runner.run(ctx, "rev-parse", commit+"^{tree}"); err != nil {
		return contracts.CheckpointManifest{}, err
	}
	raw, err := runner.run(ctx, "show", commit+":"+contracts.CheckpointManifestName)
	if err != nil {
		return contracts.CheckpointManifest{}, err
	}
	manifest, err := contracts.ParseCheckpointManifest([]byte(raw + "\n"))
	if err != nil {
		return contracts.CheckpointManifest{}, err
	}
	if err := manifest.Validate(); err != nil {
		return contracts.CheckpointManifest{}, apperr.New(apperr.CodeConflict, "corrupt_remote_checkpoint: "+err.Error())
	}
	hash, err := contracts.ManifestHash(manifest)
	if err != nil || hash != manifest.ManifestSHA256 {
		return contracts.CheckpointManifest{}, apperr.New(apperr.CodeConflict, contracts.BackupErrorCorruptRemoteCheckpoint)
	}
	if manifest.WorkspaceID != e.workspaceID {
		return contracts.CheckpointManifest{}, apperr.New(apperr.CodeConflict, "checkpoint workspace does not match")
	}
	if err := verifyCheckpointTree(ctx, runner, commit+"^{tree}", manifest); err != nil {
		return contracts.CheckpointManifest{}, err
	}
	return manifest, nil
}

func (e *CheckpointEngine) lsRemote(ctx context.Context, runner gitRunner, url, ref string) (string, error) {
	out, stderr, err := runner.runFull(ctx, "ls-remote", "--", url, ref)
	if err != nil {
		if strings.Contains(strings.ToLower(stderr+" "+err.Error()), "not found") {
			return "", apperr.New(apperr.CodeNotFound, contracts.BackupErrorRemoteMissing)
		}
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && (fields[1] == ref || strings.HasSuffix(fields[1], ref)) {
			return fields[0], nil
		}
		if len(fields) >= 1 && strings.TrimSpace(fields[0]) != "" && len(fields) == 1 {
			return fields[0], nil
		}
		if len(fields) >= 2 {
			return fields[0], nil
		}
	}
	return "", nil
}

func (e *CheckpointEngine) confirmAncestry(ctx context.Context, runner gitRunner, url, remoteTip, localCommit, ref string) error {
	fetchRef := "refs/atlas/remote-tip/" + remoteTip
	if _, err := runner.run(ctx, "fetch", "--", url, ref+":"+fetchRef); err != nil {
		if _, err2 := runner.run(ctx, "cat-file", "-e", remoteTip+"^{commit}"); err2 != nil {
			return apperr.New(apperr.CodeConflict, contracts.BackupErrorRemoteDiverged)
		}
	}
	_, _, err := runner.runFull(ctx, "merge-base", "--is-ancestor", remoteTip, localCommit)
	if err != nil {
		return apperr.New(apperr.CodeConflict, contracts.BackupErrorBlockedRemoteDiverged)
	}
	return nil
}

func (e *CheckpointEngine) markVerified(commit string, manifest contracts.CheckpointManifest, targetID string) error {
	ledger, err := loadLedger(e.paths.Ledger)
	if err != nil {
		return err
	}
	now := e.now()
	ledger.LastVerifiedCommit = commit
	ledger.LastVerifiedAt = now
	ledger.LastVerifiedTargetID = targetID
	ledger.LastVerifiedManifestSHA = manifest.ManifestSHA256
	ledger.LastVerifiedTreeSHA = manifest.CanonicalTreeSHA256
	ledger.LastRemoteCheckpointID = manifest.CheckpointID
	ledger.LastErrorClass = ""
	ledger.LastError = ""
	ledger.BlockedReason = ""
	ledger.NextRetryAt = time.Time{}
	ledger.RetryAttempt = 0
	ledger.RetryTargetID = targetID
	if err := atomicWriteJSON(e.paths.Ledger, ledger); err != nil {
		return err
	}
	box, err := loadOutbox(e.paths.Outbox)
	if err != nil {
		return err
	}
	box.State = contracts.BackupOutboxVerified
	box.LastCheckpointID = manifest.CheckpointID
	box.UpdatedAt = now
	if err := atomicWriteJSON(e.paths.Outbox, box); err != nil {
		return err
	}
	store, err := loadTargetStore(e.paths.Targets)
	if err == nil {
		for i, target := range store.Targets {
			if target.TargetID == targetID {
				store.Targets[i].LastVerified = now
				store.UpdatedAt = now
				_ = atomicWriteJSON(e.paths.Targets, store)
				break
			}
		}
	}
	return nil
}

func (e *CheckpointEngine) recordPublishFailure(targetID, class, message string) {
	ledger, err := loadLedger(e.paths.Ledger)
	if err != nil {
		return
	}
	if ledger.Format == "" {
		ledger = e.newLedger(e.now())
	}
	ledger.LastErrorClass = class
	ledger.LastError = sanitizeGitMessage(message)
	ledger.RetryTargetID = targetID
	if class == contracts.BackupErrorRemoteDiverged || class == contracts.BackupErrorBlockedRemoteDiverged {
		ledger.BlockedReason = contracts.BackupErrorBlockedRemoteDiverged
		ledger.NextRetryAt = time.Time{}
		_ = setOutboxState(e.paths.Outbox, contracts.BackupOutboxBlocked, e.now())
	} else {
		next, attempt := nextBackupRetry(e.now(), ledger.RetryAttempt, class)
		ledger.NextRetryAt = next
		ledger.RetryAttempt = attempt
		_ = setOutboxState(e.paths.Outbox, contracts.BackupOutboxRetryableFailure, e.now())
	}
	_ = atomicWriteJSON(e.paths.Ledger, ledger)
}

func (s *ActionService) PublishLatestCheckpoint(ctx context.Context, force bool) (AutoBackupResult, error) {
	engine, err := s.checkpointEngine()
	if err != nil {
		return AutoBackupResult{}, err
	}
	result, err := engine.Tick(ctx, force)
	return result, err
}
