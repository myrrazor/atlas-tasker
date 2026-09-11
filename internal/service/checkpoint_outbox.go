package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

const (
	checkpointLedgerFormat  = "atlas_backup_ledger_v1"
	checkpointOutboxFormat  = "atlas_backup_outbox_v1"
	localCheckpointTarget   = "local"
	gitMaintenanceThreshold = 1000
)

// BackupLedger is the machine-local automatic-backup ledger (ADR §4.2).
type BackupLedger struct {
	Format                  string    `json:"format"`
	WorkspaceID             string    `json:"workspace_id"`
	ReplicaID               string    `json:"replica_id"`
	LastCheckpointID        string    `json:"last_checkpoint_id,omitempty"`
	LastLocalCommit         string    `json:"last_local_commit,omitempty"`
	LastCanonicalTreeSHA256 string    `json:"last_canonical_tree_sha256,omitempty"`
	LastManifestSHA256      string    `json:"last_manifest_sha256,omitempty"`
	LastCheckpointAt        time.Time `json:"last_checkpoint_at,omitempty"`
	LastVerifiedCommit      string    `json:"last_verified_commit,omitempty"`
	LastVerifiedAt          time.Time `json:"last_verified_at,omitempty"`
	LastVerifiedTargetID    string    `json:"last_verified_target_id,omitempty"`
	LastVerifiedManifestSHA string    `json:"last_verified_manifest_sha256,omitempty"`
	LastVerifiedTreeSHA     string    `json:"last_verified_tree_sha256,omitempty"`
	LastRemoteCheckpointID  string    `json:"last_remote_checkpoint_id,omitempty"`
	LastCanonicalChangeAt   time.Time `json:"last_canonical_change_at,omitempty"`
	LastErrorClass          string    `json:"last_error_class,omitempty"`
	LastError               string    `json:"last_error,omitempty"`
	NextRetryAt             time.Time `json:"next_retry_at,omitempty"`
	RetryAttempt            int       `json:"retry_attempt,omitempty"`
	RetryTargetID           string    `json:"retry_target_id,omitempty"`
	BlockedReason           string    `json:"blocked_reason,omitempty"`
	LastDrillAt             time.Time `json:"last_drill_at,omitempty"`
	SchedulerState          string    `json:"scheduler_state,omitempty"`
	CommitCount             int       `json:"commit_count,omitempty"`
	CommitsSinceMaintenance int       `json:"commits_since_maintenance,omitempty"`
	DiskBytes               int64     `json:"disk_bytes,omitempty"`
	HealthWarning           string    `json:"health_warning,omitempty"`
	KnownCheckpointIDs      []string  `json:"known_checkpoint_ids,omitempty"`
	CreatedAt               time.Time `json:"created_at"`
}

// BackupOutbox is the reconstructable pending-mark file.
type BackupOutbox struct {
	Format             string                      `json:"format"`
	State              contracts.BackupOutboxState `json:"state"`
	PendingSince       time.Time                   `json:"pending_since,omitempty"`
	LastMutationAt     time.Time                   `json:"last_mutation_at,omitempty"`
	PendingEventCount  int                         `json:"pending_event_count"`
	Watermarks         map[string]int64            `json:"watermarks,omitempty"`
	VerifiedWatermarks map[string]int64            `json:"verified_watermarks,omitempty"`
	LastCheckpointID   string                      `json:"last_checkpoint_id,omitempty"`
	UpdatedAt          time.Time                   `json:"updated_at"`
}

type checkpointPaths struct {
	Root      string
	Repo      string
	Hooks     string
	Ledger    string
	Outbox    string
	Snapshots string
	Tmp       string
	Lock      string
	Targets   string
	Auto      string
	Replica   string
	Schedule  string
}

func backupStatePaths(stateDir, workspaceID string) checkpointPaths {
	root := filepath.Join(stateDir, "backups", workspaceID)
	return checkpointPaths{
		Root:      root,
		Repo:      filepath.Join(root, "repo.git"),
		Hooks:     filepath.Join(root, "hooks"),
		Ledger:    filepath.Join(root, "ledger.json"),
		Outbox:    filepath.Join(root, "outbox.json"),
		Snapshots: filepath.Join(root, "snapshots"),
		Tmp:       filepath.Join(root, "tmp"),
		Lock:      filepath.Join(root, "ledger.lock"),
		Targets:   filepath.Join(root, "targets.json"),
		Auto:      filepath.Join(root, "auto.json"),
		Replica:   filepath.Join(root, "replica.json"),
		Schedule:  filepath.Join(root, "schedule.json"),
	}
}

func (p checkpointPaths) ensure() error {
	for _, dir := range []string{p.Root, p.Hooks, p.Snapshots, p.Tmp} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func atomicWriteJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

func loadLedger(path string) (BackupLedger, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return BackupLedger{}, nil
		}
		return BackupLedger{}, err
	}
	var ledger BackupLedger
	if err := json.Unmarshal(raw, &ledger); err != nil {
		return BackupLedger{}, fmt.Errorf("parse backup ledger: %w", err)
	}
	return ledger, nil
}

func loadOutbox(path string) (BackupOutbox, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return BackupOutbox{}, nil
		}
		return BackupOutbox{}, err
	}
	var box BackupOutbox
	if err := json.Unmarshal(raw, &box); err != nil {
		return BackupOutbox{}, fmt.Errorf("parse backup outbox: %w", err)
	}
	return box, nil
}

func (s *ActionService) markBackupOutbox(ctx context.Context, event contracts.Event) {
	if !contracts.TriggersAutomaticCheckpoint(event.Type) {
		return
	}
	engine, err := s.checkpointEngine()
	if err != nil || engine == nil {
		return
	}
	if err := engine.MarkPending(ctx, event); err != nil {
		engine.noteWarning("outbox_write_failed")
	}
}

func (e *CheckpointEngine) MarkPending(ctx context.Context, event contracts.Event) error {
	if e == nil {
		return nil
	}
	now := e.now()
	if err := e.paths.ensure(); err != nil {
		return err
	}
	box, err := loadOutbox(e.paths.Outbox)
	if err != nil {
		return err
	}
	if box.Format == "" {
		box.Format = checkpointOutboxFormat
	}
	if box.State == "" || box.State == contracts.BackupOutboxCheckpointCreated || box.State == contracts.BackupOutboxVerified {
		box.State = contracts.BackupOutboxPending
	}
	if box.PendingSince.IsZero() {
		box.PendingSince = now
	}
	box.LastMutationAt = now
	box.PendingEventCount++
	if box.Watermarks == nil {
		box.Watermarks = map[string]int64{}
	}
	key := strings.TrimSpace(event.Project)
	if key == "" || key == workspaceProjectKey {
		key = "workspace"
	}
	if event.EventID > box.Watermarks[key] {
		box.Watermarks[key] = event.EventID
	}
	box.UpdatedAt = now
	if err := atomicWriteJSON(e.paths.Outbox, box); err != nil {
		return err
	}
	ledger, err := loadLedger(e.paths.Ledger)
	if err != nil {
		return err
	}
	if ledger.Format == "" {
		ledger = e.newLedger(now)
	}
	ledger.LastCanonicalChangeAt = now
	return atomicWriteJSON(e.paths.Ledger, ledger)
}

func (e *CheckpointEngine) newLedger(now time.Time) BackupLedger {
	return BackupLedger{
		Format:      checkpointLedgerFormat,
		WorkspaceID: e.workspaceID,
		ReplicaID:   e.replicaID,
		CreatedAt:   now,
	}
}

func (e *CheckpointEngine) noteWarning(class string) {
	if e == nil {
		return
	}
	ledger, err := loadLedger(e.paths.Ledger)
	if err != nil {
		return
	}
	if ledger.Format == "" {
		ledger = e.newLedger(e.now())
	}
	ledger.HealthWarning = class
	ledger.LastErrorClass = class
	_ = atomicWriteJSON(e.paths.Ledger, ledger)
}

func (e *CheckpointEngine) reconstruct(ctx context.Context) error {
	box, err := loadOutbox(e.paths.Outbox)
	if err != nil {
		return err
	}
	ledger, err := loadLedger(e.paths.Ledger)
	if err != nil {
		return err
	}
	files, err := collectExportFiles(e.actions.Root)
	if err != nil {
		return err
	}
	safe := backupRestoreSafeFiles(files)
	hashed, err := hashLiveFiles(e.actions.Root, safe)
	if err != nil {
		return err
	}
	tree := contracts.CanonicalTreeHash(hashed)
	marks, err := eventWatermarks(ctx, e.actions.Events)
	if err != nil {
		return err
	}
	if box.Format == "" {
		box.Format = checkpointOutboxFormat
	}
	if ledger.LastCanonicalTreeSHA256 != "" && ledger.LastCanonicalTreeSHA256 == tree {
		if box.State == contracts.BackupOutboxPending || box.State == "" {
			box.State = contracts.BackupOutboxCheckpointCreated
			box.PendingEventCount = 0
			box.PendingSince = time.Time{}
		}
	} else if ledger.LastCanonicalTreeSHA256 != tree {
		if box.State == "" || box.State == contracts.BackupOutboxCheckpointCreated || box.State == contracts.BackupOutboxVerified {
			box.State = contracts.BackupOutboxPending
			if box.PendingSince.IsZero() {
				box.PendingSince = e.now()
			}
		}
		box.Watermarks = marks
	}
	box.UpdatedAt = e.now()
	if err := atomicWriteJSON(e.paths.Outbox, box); err != nil {
		return err
	}
	if ledger.Format == "" {
		ledger = e.newLedger(e.now())
	}
	return atomicWriteJSON(e.paths.Ledger, ledger)
}

func hashLiveFiles(root string, files []string) ([]contracts.CheckpointFile, error) {
	out := make([]contracts.CheckpointFile, 0, len(files))
	for _, rel := range files {
		sum, err := fileSHA256(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		out = append(out, contracts.CheckpointFile{Path: rel, SHA256: sum, Size: info.Size()})
	}
	return out, nil
}
