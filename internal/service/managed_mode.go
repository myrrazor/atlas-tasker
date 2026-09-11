package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

// ManagedModeView is the declared policy plus the mode this machine follows.
type ManagedModeView struct {
	Present                bool                                 `json:"present"`
	Policy                 contracts.ManagedModePolicy          `json:"policy"`
	DeclaredMode           contracts.ManagedMode                `json:"declared_mode"`
	EffectiveMode          contracts.ManagedMode                `json:"effective_mode"`
	DeliveryEnabledLocally bool                                 `json:"delivery_enabled_locally"`
	CompletionMode         contracts.CompletionMode             `json:"completion_mode"`
	RequiredReviewer       contracts.Actor                      `json:"required_reviewer,omitempty"`
	CaptureDecisions       map[string]contracts.CaptureDecision `json:"capture_decisions"`
}

// BackupHealthSummary is a path-free backup/sync snapshot for MCP status.
type BackupHealthSummary struct {
	Configured            bool      `json:"configured"`
	SnapshotCount         int       `json:"snapshot_count"`
	LatestBackupID        string    `json:"latest_backup_id,omitempty"`
	LatestCreatedAt       time.Time `json:"latest_created_at,omitempty"`
	ManagedModeIncluded   bool      `json:"managed_mode_included"`
	SyncRemoteCount       int       `json:"sync_remote_count"`
	SyncReasonCodes       []string  `json:"sync_reason_codes,omitempty"`
	WorkerState           string    `json:"worker_state,omitempty"`
	UnbackedEventCount    int       `json:"unbacked_event_count,omitempty"`
	LastLocalCheckpointID string    `json:"last_local_checkpoint_id,omitempty"`
	LastLocalCheckpointAt time.Time `json:"last_local_checkpoint_at,omitempty"`
	LastErrorClass         string    `json:"last_error_class,omitempty"`
	DiskBytes              int64     `json:"disk_bytes,omitempty"`
	AutomaticEnabled       bool      `json:"automatic_enabled,omitempty"`
	LastRemoteCheckpointID string    `json:"last_remote_checkpoint_id,omitempty"`
	LastRemoteVerifiedAt   time.Time `json:"last_remote_verified_at,omitempty"`
	SchedulerState         string    `json:"scheduler_state,omitempty"`
	RestoreDrillAgeSeconds   int       `json:"restore_drill_age_seconds,omitempty"`
	VerifiedRemote           bool      `json:"verified_remote,omitempty"`
	OldestUnbackedAgeSeconds int       `json:"oldest_unbacked_age_seconds,omitempty"`
	Notes                    []string  `json:"notes,omitempty"`
}

// LoadManagedModePolicy reads .tracker/managed-mode.json. A missing file is
// not an error: Present is false and Policy is the recommended default.
func LoadManagedModePolicy(root string) (contracts.ManagedModePolicy, bool, error) {
	raw, err := os.ReadFile(storage.ManagedModeFile(root))
	if err != nil {
		if os.IsNotExist(err) {
			return contracts.DefaultManagedModePolicy(), false, nil
		}
		return contracts.ManagedModePolicy{}, false, fmt.Errorf("read managed mode policy: %w", err)
	}
	policy, err := contracts.ParseManagedModePolicy(raw)
	if err != nil {
		return contracts.ManagedModePolicy{}, false, err
	}
	return policy, true, nil
}

// SaveManagedModePolicy writes the shared policy under the workspace write lock.
func (s *ActionService) SaveManagedModePolicy(ctx context.Context, policy contracts.ManagedModePolicy) error {
	return WithWriteLock(ctx, s.LockManager, "save managed mode", func(ctx context.Context) error {
		body, err := policy.Encode()
		if err != nil {
			return err
		}
		path := storage.ManagedModeFile(s.Root)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create tracker dir: %w", err)
		}
		tmp, err := os.CreateTemp(filepath.Dir(path), "managed-mode-*.tmp")
		if err != nil {
			return fmt.Errorf("create managed mode tempfile: %w", err)
		}
		tmpName := tmp.Name()
		if _, err := tmp.Write(body); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
			return fmt.Errorf("write managed mode policy: %w", err)
		}
		if err := tmp.Close(); err != nil {
			_ = os.Remove(tmpName)
			return fmt.Errorf("close managed mode policy: %w", err)
		}
		if err := os.Rename(tmpName, path); err != nil {
			_ = os.Remove(tmpName)
			return fmt.Errorf("replace managed mode policy: %w", err)
		}
		return nil
	})
}

// ManagedModeView loads the shared policy and the workspace completion mode.
// deliveryEnabledLocally is the machine-local record of a delivery-profile
// server; the shared file alone never upgrades a clone into delivery.
func (s *QueryService) ManagedModeView(deliveryEnabledLocally bool) (ManagedModeView, error) {
	policy, present, err := LoadManagedModePolicy(s.Root)
	if err != nil {
		return ManagedModeView{}, err
	}
	effective, err := policy.EffectiveMode(deliveryEnabledLocally)
	if err != nil {
		return ManagedModeView{}, err
	}
	cfg, err := config.Load(s.Root)
	if err != nil {
		return ManagedModeView{}, err
	}
	completion, err := policy.EffectiveCompletionMode(cfg.Workflow.CompletionMode)
	if err != nil {
		return ManagedModeView{}, err
	}
	intents := []contracts.WorkIntent{
		contracts.WorkIntentMaterialWork,
		contracts.WorkIntentStatusQuery,
		contracts.WorkIntentReadOnlyExplanation,
		contracts.WorkIntentCasualDiscussion,
		contracts.WorkIntentSimpleQuestion,
		contracts.WorkIntentTrackingExcluded,
	}
	decisions := map[string]contracts.CaptureDecision{}
	for _, intent := range intents {
		decision, decErr := policy.CaptureDecision(intent)
		if decErr != nil {
			return ManagedModeView{}, decErr
		}
		decisions[string(intent)] = decision
	}
	return ManagedModeView{
		Present:                present,
		Policy:                 policy,
		DeclaredMode:           policy.Mode,
		EffectiveMode:          effective,
		DeliveryEnabledLocally: deliveryEnabledLocally,
		CompletionMode:         completion,
		RequiredReviewer:       cfg.Workflow.RequiredReviewer,
		CaptureDecisions:       decisions,
	}, nil
}

// CaptureDecisionFor applies the persisted (or recommended) policy to intent.
func (s *QueryService) CaptureDecisionFor(intent contracts.WorkIntent) (contracts.CaptureDecision, error) {
	view, err := s.ManagedModeView(false)
	if err != nil {
		return "", err
	}
	return view.Policy.CaptureDecision(intent)
}

// RecordsProgressFor reports whether the persisted policy records this event.
func (s *QueryService) RecordsProgressFor(event contracts.ProgressEvent) (bool, error) {
	view, err := s.ManagedModeView(false)
	if err != nil {
		return false, err
	}
	return view.Policy.RecordsProgress(event)
}

// BackupHealth summarizes backup snapshots and sync remotes without paths.
func (s *QueryService) BackupHealth(ctx context.Context) (BackupHealthSummary, error) {
	summary := BackupHealthSummary{}
	if _, err := os.Stat(storage.ManagedModeFile(s.Root)); err == nil {
		summary.ManagedModeIncluded = true
	} else if err != nil && !os.IsNotExist(err) {
		return BackupHealthSummary{}, err
	}
	snapshots, err := s.Backups.ListBackupSnapshots(ctx)
	if err != nil && !os.IsNotExist(err) {
		return BackupHealthSummary{}, err
	}
	summary.SnapshotCount = len(snapshots)
	if len(snapshots) > 0 {
		summary.Configured = true
		latest := snapshots[0]
		for _, item := range snapshots[1:] {
			if item.CreatedAt.After(latest.CreatedAt) {
				latest = item
			}
		}
		summary.LatestBackupID = latest.BackupID
		summary.LatestCreatedAt = latest.CreatedAt.UTC()
	}
	syncView, syncErr := s.SyncStatus(ctx, "")
	if syncErr == nil {
		summary.SyncRemoteCount = len(syncView.Remotes)
		summary.SyncReasonCodes = append([]string(nil), syncView.ReasonCodes...)
		if summary.SyncRemoteCount > 0 {
			summary.Configured = true
		}
	}
	auto, autoErr := s.AutoBackupStatus(ctx)
	if autoErr == nil {
		summary.WorkerState = string(auto.State)
		summary.UnbackedEventCount = auto.UnbackedEventCount
		summary.LastLocalCheckpointID = auto.LastLocalCheckpointID
		summary.LastLocalCheckpointAt = auto.LastLocalCheckpointAt
		summary.LastErrorClass = auto.LastErrorClass
		summary.DiskBytes = auto.DiskBytes
		summary.AutomaticEnabled = auto.AutomaticEnabled
		summary.LastRemoteCheckpointID = auto.LastRemoteCheckpointID
		summary.LastRemoteVerifiedAt = auto.LastRemoteVerifiedAt
		summary.SchedulerState = auto.SchedulerState
		summary.RestoreDrillAgeSeconds = auto.RestoreDrillAgeSeconds
		summary.VerifiedRemote = auto.VerifiedRemote
		summary.OldestUnbackedAgeSeconds = auto.OldestUnbackedAgeSeconds
		if auto.LastLocalCheckpointID != "" || auto.UnbackedEventCount > 0 || auto.AutomaticEnabled {
			summary.Configured = true
		}
	}
	if !summary.Configured {
		summary.Notes = append(summary.Notes, "no backup snapshot or sync remote is configured on this workspace")
	}
	if !summary.ManagedModeIncluded {
		summary.Notes = append(summary.Notes, "managed-mode.json is not present; recommended defaults apply until setup writes the file")
	}
	sort.Strings(summary.Notes)
	return summary, nil
}

func compactTicketRef(ticket contracts.TicketSnapshot) TicketRef {
	return TicketRef{
		ID:       ticket.ID,
		Project:  ticket.Project,
		Title:    strings.TrimSpace(ticket.Title),
		Status:   string(ticket.Status),
		Priority: string(ticket.Priority),
		Type:     string(ticket.Type),
		Assignee: string(ticket.Assignee),
	}
}

// TicketRef is a bounded ticket identity for MCP context/status payloads.
type TicketRef struct {
	ID          string   `json:"id"`
	Project     string   `json:"project,omitempty"`
	Title       string   `json:"title,omitempty"`
	Status      string   `json:"status,omitempty"`
	Priority    string   `json:"priority,omitempty"`
	Type        string   `json:"type,omitempty"`
	Assignee    string   `json:"assignee,omitempty"`
	ReasonCodes []string `json:"reason_codes,omitempty"`
}

func isActiveWorkflowStatus(status contracts.Status) bool {
	switch status {
	case contracts.StatusReady, contracts.StatusInProgress, contracts.StatusInReview, contracts.StatusBlocked:
		return true
	default:
		return false
	}
}

func isActiveRunStatus(status contracts.RunStatus) bool {
	switch status {
	case contracts.RunStatusDispatched, contracts.RunStatusAttached, contracts.RunStatusActive,
		contracts.RunStatusHandoffReady, contracts.RunStatusAwaitingReview, contracts.RunStatusAwaitingOwner:
		return true
	default:
		return false
	}
}
