package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/buildinfo"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

const (
	defaultQuietPeriod            = 30 * time.Second
	defaultMaxUnbackedDelay       = 5 * time.Minute
	defaultMaxEventsPerCheckpoint = 100
	defaultWatchInterval          = 5 * time.Second
)

// CrashPoint is an injected failure used by AT114-405 recovery tests.
type CrashPoint string

const (
	CrashBeforeSnapshot           CrashPoint = "before_snapshot"
	CrashDuringSnapshot           CrashPoint = "during_snapshot"
	CrashAfterManifest            CrashPoint = "after_manifest"
	CrashAfterGitObjects          CrashPoint = "after_git_objects"
	CrashBeforeLocalRef           CrashPoint = "before_local_ref"
	CrashAfterLocalRef            CrashPoint = "after_local_ref"
	CrashBeforePush               CrashPoint = "before_push"
	CrashAfterPush                CrashPoint = "after_push"
	CrashBeforeRemoteVerify       CrashPoint = "before_remote_verify"
	CrashAfterVerifyBeforePersist CrashPoint = "after_verify_before_persist"
)

// CheckpointEngine runs local automatic checkpoints. Push and remote
// verification belong to Sprint 114.5; this engine stops at
// checkpoint_created after the isolated local ref is updated.
type CheckpointEngine struct {
	actions          *ActionService
	workspaceID      string
	replicaID        string
	stateDir         string
	paths            checkpointPaths
	git              string
	quietPeriod      time.Duration
	maxUnbackedDelay time.Duration
	maxEvents        int
	watchInterval    time.Duration
	maintenanceEvery int
	maintenanceRuns  int
	crashAt          CrashPoint
	nowFn            func() time.Time
}

func (s *ActionService) checkpointEngine() (*CheckpointEngine, error) {
	if s == nil {
		return nil, nil
	}
	stateDir, err := resolveUserStateDir(s.StateDir, s.Home)
	if err != nil {
		return nil, err
	}
	workspaceID, err := LoadWorkspaceIdentity(s.Root)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(workspaceID) == "" {
		return nil, fmt.Errorf("workspace identity is required for automatic backup")
	}
	if !validBackupWorkspaceID(workspaceID) {
		return nil, fmt.Errorf("workspace identity is not a portable backup id")
	}
	git, err := validateGitExecutable(s.GitPath)
	if err != nil {
		return nil, err
	}
	paths := backupStatePaths(stateDir, workspaceID)
	if err := paths.ensure(); err != nil {
		return nil, err
	}
	ident, err := s.ensureReplicaIdentity(paths, workspaceID, false)
	if err != nil {
		return nil, err
	}
	replicaID := strings.TrimSpace(ident.ReplicaID)
	if replicaID == "" {
		return nil, fmt.Errorf("replica identity is required for automatic backup")
	}
	engine := &CheckpointEngine{
		actions:          s,
		workspaceID:      workspaceID,
		replicaID:        replicaID,
		stateDir:         stateDir,
		paths:            paths,
		git:              git,
		quietPeriod:      defaultQuietPeriod,
		maxUnbackedDelay: defaultMaxUnbackedDelay,
		maxEvents:        defaultMaxEventsPerCheckpoint,
		watchInterval:    defaultWatchInterval,
		maintenanceEvery: gitMaintenanceThreshold,
		crashAt:          CrashPoint(s.CheckpointCrashAt),
		nowFn:            s.Clock,
	}
	if s.QuietPeriod > 0 {
		engine.quietPeriod = s.QuietPeriod
	}
	if s.MaxUnbackedDelay > 0 {
		engine.maxUnbackedDelay = s.MaxUnbackedDelay
	}
	if s.MaxEventsPerCheckpoint > 0 {
		engine.maxEvents = s.MaxEventsPerCheckpoint
	}
	if s.WatchInterval > 0 {
		engine.watchInterval = s.WatchInterval
	}
	if s.GitMaintenanceEvery > 0 {
		engine.maintenanceEvery = s.GitMaintenanceEvery
	}
	return engine, nil
}

func (e *CheckpointEngine) now() time.Time {
	if e != nil && e.nowFn != nil {
		return e.nowFn().UTC()
	}
	return time.Now().UTC()
}

func (e *CheckpointEngine) crash(point CrashPoint) error {
	if e != nil && e.crashAt != "" && e.crashAt == point {
		return apperr.New(apperr.CodeInternal, "injected checkpoint crash:"+string(point))
	}
	return nil
}

func (e *CheckpointEngine) HasLedger() bool {
	if e == nil {
		return false
	}
	_, err := os.Stat(e.paths.Ledger)
	return err == nil
}

func (e *CheckpointEngine) HasPending() bool {
	if e == nil {
		return false
	}
	box, err := loadOutbox(e.paths.Outbox)
	if err != nil {
		return false
	}
	return box.State == contracts.BackupOutboxPending || box.PendingEventCount > 0
}

// AutoBackupResult is the shared output of tick and run --now.
type AutoBackupResult struct {
	Kind           string                      `json:"kind"`
	GeneratedAt    time.Time                   `json:"generated_at"`
	State          contracts.BackupOutboxState `json:"state"`
	CheckpointID   string                      `json:"checkpoint_id,omitempty"`
	Commit         string                      `json:"commit,omitempty"`
	CanonicalTree  string                      `json:"canonical_tree_sha256,omitempty"`
	Created        bool                        `json:"created"`
	Skipped        bool                        `json:"skipped"`
	SkipReason     string                      `json:"skip_reason,omitempty"`
	UnbackedEvents int                         `json:"unbacked_event_count"`
	ReplicaID      string                      `json:"replica_id,omitempty"`
	ErrorClass     string                      `json:"error_class,omitempty"`
	DiskBytes      int64                       `json:"disk_bytes,omitempty"`
	CopyDuration   time.Duration               `json:"copy_duration,omitempty"`
}

func (s *ActionService) BackupTick(ctx context.Context, force bool) (AutoBackupResult, error) {
	engine, err := s.checkpointEngine()
	if err != nil {
		return AutoBackupResult{}, err
	}
	return engine.Tick(ctx, force)
}

func (s *ActionService) BackupWatch(ctx context.Context) error {
	engine, err := s.checkpointEngine()
	if err != nil {
		return err
	}
	ticker := time.NewTicker(engine.watchInterval)
	defer ticker.Stop()
	if _, err := engine.Tick(ctx, false); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := engine.Tick(ctx, false); err != nil {
				engine.noteWarning(classifyBackupError(err))
			}
		}
	}
}

func (e *CheckpointEngine) Tick(ctx context.Context, force bool) (AutoBackupResult, error) {
	result := AutoBackupResult{Kind: "backup_auto_result", GeneratedAt: e.now(), ReplicaID: e.replicaID}
	unlock, err := e.lockLedger()
	if err != nil {
		result.ErrorClass = classifyBackupError(err)
		return result, err
	}
	defer unlock()
	if err := e.reconstruct(ctx); err != nil {
		result.ErrorClass = classifyBackupError(err)
		result.State = contracts.BackupOutboxRetryableFailure
		return result, err
	}
	box, err := loadOutbox(e.paths.Outbox)
	if err != nil {
		return result, err
	}
	result.State = box.State
	result.UnbackedEvents = box.PendingEventCount
	if !force && !e.shouldCheckpoint(box) {
		result.Skipped = true
		result.SkipReason = skipReason(box, e)
		// An interrupted push must resume even when the local tree is unchanged.
		return e.finishTickWithPublish(ctx, result, "")
	}
	created, err := e.createLocalCheckpoint(ctx)
	if err != nil {
		e.recordFailure(classifyBackupError(err), err.Error())
		result.ErrorClass = classifyBackupError(err)
		result.State = contracts.BackupOutboxRetryableFailure
		return result, err
	}
	result.Created = created.Created
	result.Skipped = !created.Created
	result.SkipReason = created.SkipReason
	result.CheckpointID = created.CheckpointID
	result.Commit = created.Commit
	result.CanonicalTree = created.Tree
	result.State = contracts.BackupOutboxCheckpointCreated
	result.CopyDuration = created.CopyDuration
	result.DiskBytes = created.DiskBytes
	return e.finishTickWithPublish(ctx, result, created.Commit)
}

func (e *CheckpointEngine) finishTickWithPublish(ctx context.Context, result AutoBackupResult, commit string) (AutoBackupResult, error) {
	_ = retainLocalCheckpointState(e.paths, e.now())
	// Divergence stays blocked until replica reset / reconcile. --now never force-publishes.
	if pubErr := e.publishIfEnabled(ctx, commit, false); pubErr != nil {
		result.ErrorClass = classifyRemoteBackupError(pubErr)
		if result.ErrorClass == contracts.BackupErrorBlockedRemoteDiverged || result.ErrorClass == contracts.BackupErrorRemoteDiverged {
			result.State = contracts.BackupOutboxBlocked
			return result, nil
		}
		result.State = contracts.BackupOutboxRetryableFailure
		return result, pubErr
	}
	box, err := loadOutbox(e.paths.Outbox)
	if err == nil && box.State != "" {
		result.State = box.State
	}
	return result, nil
}

func (e *CheckpointEngine) lockLedger() (func(), error) {
	if e == nil {
		return func() {}, nil
	}
	if err := e.paths.ensure(); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(e.paths.Lock, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open backup ledger lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("acquire backup ledger lock: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}

func (e *CheckpointEngine) shouldCheckpoint(box BackupOutbox) bool {
	if box.State != contracts.BackupOutboxPending && box.PendingEventCount == 0 {
		return false
	}
	now := e.now()
	if box.PendingEventCount >= e.maxEvents {
		return true
	}
	if !box.PendingSince.IsZero() && now.Sub(box.PendingSince) >= e.maxUnbackedDelay {
		return true
	}
	if !box.LastMutationAt.IsZero() && now.Sub(box.LastMutationAt) >= e.quietPeriod {
		return true
	}
	return false
}

func skipReason(box BackupOutbox, e *CheckpointEngine) string {
	if box.State != contracts.BackupOutboxPending && box.PendingEventCount == 0 {
		return "canonical_state_unchanged"
	}
	return "quiet_period"
}

type createdCheckpoint struct {
	Created      bool
	SkipReason   string
	CheckpointID string
	Commit       string
	Tree         string
	CopyDuration time.Duration
	DiskBytes    int64
}

func (e *CheckpointEngine) createLocalCheckpoint(ctx context.Context) (createdCheckpoint, error) {
	if err := e.crash(CrashBeforeSnapshot); err != nil {
		return createdCheckpoint{}, err
	}
	if err := setOutboxState(e.paths.Outbox, contracts.BackupOutboxSnapshotting, e.now()); err != nil {
		return createdCheckpoint{}, err
	}
	dest := filepath.Join(e.paths.Snapshots, "snap-"+fmt.Sprintf("%d", e.now().UnixNano()))
	snap, err := e.actions.CaptureCanonicalSnapshot(ctx, dest)
	if err != nil {
		return createdCheckpoint{}, err
	}
	ledger, err := loadLedger(e.paths.Ledger)
	if err != nil {
		return createdCheckpoint{}, err
	}
	if ledger.LastCanonicalTreeSHA256 != "" && ledger.LastCanonicalTreeSHA256 == snap.TreeHash {
		_ = os.RemoveAll(dest)
		if err := e.markCheckpointCreated(ledger.LastCheckpointID, ledger.LastLocalCommit, snap.TreeHash, false); err != nil {
			return createdCheckpoint{}, err
		}
		return createdCheckpoint{SkipReason: "canonical_state_unchanged", CheckpointID: ledger.LastCheckpointID, Commit: ledger.LastLocalCommit, Tree: snap.TreeHash}, nil
	}
	id := contracts.LogicalCheckpointID(e.workspaceID, e.replicaID, snap.TreeHash, localCheckpointTarget, snap.Watermarks)
	for _, known := range ledger.KnownCheckpointIDs {
		if known == id && ledger.LastLocalCommit != "" {
			_ = os.RemoveAll(dest)
			if err := e.markCheckpointCreated(id, ledger.LastLocalCommit, snap.TreeHash, false); err != nil {
				return createdCheckpoint{}, err
			}
			return createdCheckpoint{SkipReason: "duplicate_logical_checkpoint", CheckpointID: id, Commit: ledger.LastLocalCommit, Tree: snap.TreeHash}, nil
		}
	}
	manifest, err := contracts.CheckpointManifest{
		Format:                   contracts.CheckpointManifestFormat,
		CheckpointID:             id,
		WorkspaceID:              e.workspaceID,
		ReplicaID:                e.replicaID,
		AtlasVersion:             buildinfo.Current().Version,
		CreatedAt:                e.now(),
		PreviousCheckpointCommit: ledger.LastLocalCommit,
		EventWatermarks:          snap.Watermarks,
		SchemaVersion:            contracts.CurrentSchemaVersion,
		Files:                    snap.CheckpointFiles(),
	}.Canonicalize()
	if err != nil {
		return createdCheckpoint{}, err
	}
	commit, err := e.commitSnapshot(ctx, snap, manifest)
	if err != nil {
		return createdCheckpoint{}, err
	}
	disk, _ := dirSize(e.paths.Repo)
	ledger.LastCheckpointID = id
	ledger.LastLocalCommit = commit
	ledger.LastCanonicalTreeSHA256 = snap.TreeHash
	ledger.LastManifestSHA256 = manifest.ManifestSHA256
	ledger.LastCheckpointAt = e.now()
	ledger.CommitCount++
	ledger.CommitsSinceMaintenance++
	ledger.DiskBytes = disk
	ledger.LastErrorClass = ""
	ledger.LastError = ""
	ledger.HealthWarning = ""
	ledger.KnownCheckpointIDs = appendUnique(ledger.KnownCheckpointIDs, id)
	if err := atomicWriteJSON(e.paths.Ledger, ledger); err != nil {
		return createdCheckpoint{}, err
	}
	if err := e.markCheckpointCreated(id, commit, snap.TreeHash, true); err != nil {
		return createdCheckpoint{}, err
	}
	_ = os.RemoveAll(dest)
	return createdCheckpoint{
		Created:      true,
		CheckpointID: id,
		Commit:       commit,
		Tree:         snap.TreeHash,
		CopyDuration: snap.CopyDuration,
		DiskBytes:    disk,
	}, nil
}

func (e *CheckpointEngine) markCheckpointCreated(id, commit, tree string, resetPending bool) error {
	box, err := loadOutbox(e.paths.Outbox)
	if err != nil {
		return err
	}
	box.Format = checkpointOutboxFormat
	box.State = contracts.BackupOutboxCheckpointCreated
	box.LastCheckpointID = id
	if resetPending {
		box.PendingEventCount = 0
		box.PendingSince = time.Time{}
		box.VerifiedWatermarks = box.Watermarks
	}
	box.UpdatedAt = e.now()
	return atomicWriteJSON(e.paths.Outbox, box)
}

func (e *CheckpointEngine) recordFailure(class, message string) {
	ledger, err := loadLedger(e.paths.Ledger)
	if err != nil {
		return
	}
	if ledger.Format == "" {
		ledger = e.newLedger(e.now())
	}
	ledger.LastErrorClass = class
	ledger.LastError = message
	_ = atomicWriteJSON(e.paths.Ledger, ledger)
	_ = setOutboxState(e.paths.Outbox, contracts.BackupOutboxRetryableFailure, e.now())
}

func setOutboxState(path string, state contracts.BackupOutboxState, now time.Time) error {
	box, err := loadOutbox(path)
	if err != nil {
		return err
	}
	if box.Format == "" {
		box.Format = checkpointOutboxFormat
	}
	box.State = state
	box.UpdatedAt = now
	return atomicWriteJSON(path, box)
}

func classifyBackupError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "busy") || apperr.CodeOf(err) == apperr.CodeBusy:
		return "busy"
	case strings.Contains(msg, "injected checkpoint crash"):
		return "injected_crash"
	case strings.Contains(msg, "git"):
		return "git_failed"
	default:
		return "checkpoint_failed"
	}
}

func appendUnique(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

func dirSize(root string) (int64, error) {
	var total int64
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return err
		}
		total += info.Size()
		return nil
	})
	return total, err
}
