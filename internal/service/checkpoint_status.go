package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

// AutoBackupStatus is the path-free automatic-backup health view.
type AutoBackupStatus struct {
	Kind                     string                      `json:"kind"`
	GeneratedAt              time.Time                   `json:"generated_at"`
	WorkspaceID              string                      `json:"workspace_id,omitempty"`
	ReplicaID                string                      `json:"replica_id,omitempty"`
	State                    contracts.BackupOutboxState `json:"worker_state,omitempty"`
	LastCanonicalChangeAt    time.Time                   `json:"last_canonical_change_at,omitempty"`
	LastLocalCheckpointAt    time.Time                   `json:"last_local_checkpoint_at,omitempty"`
	LastLocalCheckpointID    string                      `json:"last_local_checkpoint_id,omitempty"`
	LastLocalCommit          string                      `json:"last_local_commit,omitempty"`
	UnbackedEventCount       int                         `json:"unbacked_event_count"`
	OldestUnbackedAgeSeconds int                         `json:"oldest_unbacked_age_seconds,omitempty"`
	LastErrorClass           string                      `json:"last_error_class,omitempty"`
	HealthWarning            string                      `json:"health_warning,omitempty"`
	DiskBytes                int64                       `json:"disk_bytes,omitempty"`
	VerifiedRemote           bool                        `json:"verified_remote"`
	AutomaticEnabled         bool                        `json:"automatic_enabled"`
	DefaultTargetID          string                      `json:"default_target_id,omitempty"`
	TargetCount              int                         `json:"target_count,omitempty"`
	LastRemoteCheckpointID   string                      `json:"last_remote_checkpoint_id,omitempty"`
	LastRemoteVerifiedAt     time.Time                   `json:"last_remote_verified_at,omitempty"`
	SchedulerState           string                      `json:"scheduler_state,omitempty"`
	RestoreDrillAgeSeconds   int                         `json:"restore_drill_age_seconds,omitempty"`
	OriginOverlap            bool                        `json:"origin_overlap,omitempty"`
	Notes                    []string                    `json:"notes,omitempty"`
}

func (s *QueryService) AutoBackupStatus(ctx context.Context) (AutoBackupStatus, error) {
	view := AutoBackupStatus{Kind: "backup_auto_status", GeneratedAt: s.now()}
	workspaceID, err := LoadWorkspaceIdentity(s.Root)
	if err != nil {
		return view, err
	}
	view.WorkspaceID = workspaceID
	if workspaceID == "" {
		view.Notes = append(view.Notes, "workspace identity is missing")
		return view, nil
	}
	stateDir, err := resolveUserStateDir(s.StateDir, s.Home)
	if err != nil {
		view.Notes = append(view.Notes, "automatic backup state directory is not configured")
		return view, nil
	}
	paths := backupStatePaths(stateDir, workspaceID)
	ledger, err := loadLedger(paths.Ledger)
	if err != nil {
		return view, err
	}
	box, err := loadOutbox(paths.Outbox)
	if err != nil {
		return view, err
	}
	view.ReplicaID = ledger.ReplicaID
	view.State = box.State
	view.LastCanonicalChangeAt = ledger.LastCanonicalChangeAt
	view.LastLocalCheckpointAt = ledger.LastCheckpointAt
	view.LastLocalCheckpointID = ledger.LastCheckpointID
	view.LastLocalCommit = ledger.LastLocalCommit
	view.UnbackedEventCount = box.PendingEventCount
	if !box.PendingSince.IsZero() {
		view.OldestUnbackedAgeSeconds = int(s.now().Sub(box.PendingSince).Seconds())
	}
	view.LastErrorClass = ledger.LastErrorClass
	view.HealthWarning = ledger.HealthWarning
	view.DiskBytes = ledger.DiskBytes
	view.LastRemoteCheckpointID = ledger.LastRemoteCheckpointID
	view.LastRemoteVerifiedAt = ledger.LastVerifiedAt
	view.SchedulerState = ledger.SchedulerState
	if !ledger.LastDrillAt.IsZero() {
		view.RestoreDrillAgeSeconds = int(s.now().Sub(ledger.LastDrillAt).Seconds())
	}
	if cfg, err := loadAutoConfig(paths.Auto); err == nil {
		view.AutomaticEnabled = cfg.Enabled
		view.DefaultTargetID = cfg.DefaultTargetID
	}
	view.VerifiedRemote = ledger.LastVerifiedCommit != "" && !ledger.LastVerifiedAt.IsZero() &&
		(view.DefaultTargetID == "" || ledger.LastVerifiedTargetID == "" || ledger.LastVerifiedTargetID == view.DefaultTargetID)
	if view.State == contracts.BackupOutboxBlocked || view.LastErrorClass == contracts.BackupErrorBlockedRemoteDiverged {
		view.VerifiedRemote = false
	}
	if store, err := loadTargetStore(paths.Targets); err == nil {
		view.TargetCount = len(store.Targets)
		for _, target := range store.Targets {
			if target.OriginOverlap {
				view.OriginOverlap = true
				view.Notes = append(view.Notes, "target_matches_workspace_remote")
			}
		}
	}
	if view.State == contracts.BackupOutboxVerified && !view.VerifiedRemote {
		view.State = contracts.BackupOutboxCheckpointCreated
	}
	if strings.Contains(strings.ToLower(view.LastErrorClass+view.HealthWarning), "http") ||
		strings.Contains(view.LastErrorClass, "://") || strings.Contains(view.HealthWarning, "://") {
		view.LastErrorClass = "redacted"
		view.HealthWarning = "redacted"
	}
	return view, nil
}

func (s *ActionService) AutoBackupStatus(ctx context.Context) (AutoBackupStatus, error) {
	queries := NewQueryService(s.Root, s.Projects, s.Tickets, s.Events, s.Projection, s.Clock)
	queries.StateDir = s.StateDir
	queries.Home = s.Home
	return queries.AutoBackupStatus(ctx)
}

func (s *ActionService) ensurePreDestructiveCheckpoint(ctx context.Context, operation string) error {
	engine, err := s.checkpointEngine()
	if err != nil || engine == nil {
		if s.RequirePreDestructiveCheckpoint {
			return apperr.New(apperr.CodeConflict, fmt.Sprintf("pre-destructive checkpoint required before %s: %v", operation, err))
		}
		return nil
	}
	active := engine.HasLedger() && (engine.HasPending() || ledgerHasCheckpoint(engine))
	if !active && !s.RequirePreDestructiveCheckpoint {
		if _, err := engine.Tick(ctx, true); err != nil {
			engine.noteWarning("pre_destructive_checkpoint_failed")
		}
		return nil
	}
	result, err := engine.Tick(ctx, true)
	if err != nil {
		if result.Commit != "" || result.CheckpointID != "" || remotePublishErrorClass(result.ErrorClass) {
			engine.noteWarning("pre_destructive_remote_publish_failed")
			return nil
		}
		return apperr.New(apperr.CodeConflict, fmt.Sprintf("pre-destructive checkpoint failed before %s: %v", operation, err))
	}
	return nil
}

func ledgerHasCheckpoint(engine *CheckpointEngine) bool {
	ledger, err := loadLedger(engine.paths.Ledger)
	if err != nil {
		return false
	}
	return ledger.LastLocalCommit != ""
}

// MaterializeLatestCheckpoint writes the latest local checkpoint tree into dest
// (excluding nothing the restore planner can consume) and returns the manifest.
func (s *ActionService) MaterializeLatestCheckpoint(ctx context.Context, dest string) (contracts.CheckpointManifest, error) {
	engine, err := s.checkpointEngine()
	if err != nil {
		return contracts.CheckpointManifest{}, err
	}
	commit, err := engine.currentRefCommit(ctx)
	if err != nil {
		return contracts.CheckpointManifest{}, err
	}
	if err := engine.materializeCommit(ctx, commit, dest); err != nil {
		return contracts.CheckpointManifest{}, err
	}
	return engine.readCommitManifest(ctx, commit)
}

// AttachUserState wires the machine-local state directory onto action and query
// services. Callers that omit home fall back to $HOME / XDG_STATE_HOME.
func AttachUserState(actions *ActionService, queries *QueryService, home, stateDir string) {
	if actions != nil {
		actions.Home = home
		actions.StateDir = stateDir
	}
	if queries != nil {
		queries.Home = home
		queries.StateDir = stateDir
	}
}
