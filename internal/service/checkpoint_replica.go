package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/myrrazor/atlas-tasker/internal/apperr"
)

const replicaIdentityFormat = "atlas_backup_replica_v1"

// ReplicaIdentity is the 128-bit machine-local replica id (AT114-503).
type ReplicaIdentity struct {
	Format      string    `json:"format"`
	WorkspaceID string    `json:"workspace_id"`
	ReplicaID   string    `json:"replica_id"`
	CreatedAt   time.Time `json:"created_at"`
	ResetAt     time.Time `json:"reset_at,omitempty"`
}

type ReplicaView struct {
	Kind        string          `json:"kind"`
	GeneratedAt time.Time       `json:"generated_at"`
	Identity    ReplicaIdentity `json:"identity"`
	Ref         string          `json:"ref"`
}

func loadReplicaIdentity(path string) (ReplicaIdentity, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ReplicaIdentity{}, nil
		}
		return ReplicaIdentity{}, err
	}
	var ident ReplicaIdentity
	if err := json.Unmarshal(raw, &ident); err != nil {
		return ReplicaIdentity{}, fmt.Errorf("parse replica identity: %w", err)
	}
	return ident, nil
}

func (s *ActionService) ensureReplicaIdentity(paths checkpointPaths, workspaceID string, reset bool) (ReplicaIdentity, error) {
	ident, err := loadReplicaIdentity(paths.Replica)
	if err != nil {
		return ReplicaIdentity{}, err
	}
	if !reset && strings.TrimSpace(ident.ReplicaID) != "" && ident.WorkspaceID == workspaceID {
		return ident, nil
	}
	ledger, err := loadLedger(paths.Ledger)
	if err != nil {
		return ReplicaIdentity{}, err
	}
	if !reset && strings.TrimSpace(ledger.ReplicaID) != "" && ledger.WorkspaceID == workspaceID {
		ident = ReplicaIdentity{
			Format:      replicaIdentityFormat,
			WorkspaceID: workspaceID,
			ReplicaID:   ledger.ReplicaID,
			CreatedAt:   ledger.CreatedAt,
		}
		if ident.CreatedAt.IsZero() {
			ident.CreatedAt = s.now()
		}
		if err := atomicWriteJSON(paths.Replica, ident); err != nil {
			return ReplicaIdentity{}, err
		}
		return ident, nil
	}
	now := s.now()
	ident = ReplicaIdentity{
		Format:      replicaIdentityFormat,
		WorkspaceID: workspaceID,
		ReplicaID:   newReplicaID(),
		CreatedAt:   now,
	}
	if reset {
		ident.ResetAt = now
	}
	if err := atomicWriteJSON(paths.Replica, ident); err != nil {
		return ReplicaIdentity{}, err
	}
	if ledger.Format == "" {
		ledger.Format = checkpointLedgerFormat
		ledger.WorkspaceID = workspaceID
		ledger.CreatedAt = now
	}
	ledger.ReplicaID = ident.ReplicaID
	ledger.BlockedReason = ""
	ledger.LastErrorClass = ""
	ledger.LastError = ""
	ledger.NextRetryAt = time.Time{}
	ledger.RetryAttempt = 0
	if reset {
		// A new replica publishes a new ref. Prior target verification does not apply.
		ledger.LastVerifiedCommit = ""
		ledger.LastVerifiedAt = time.Time{}
		ledger.LastVerifiedTargetID = ""
		ledger.LastVerifiedManifestSHA = ""
		ledger.LastVerifiedTreeSHA = ""
		ledger.LastRemoteCheckpointID = ""
	}
	if err := atomicWriteJSON(paths.Ledger, ledger); err != nil {
		return ReplicaIdentity{}, err
	}
	return ident, nil
}

func (s *ActionService) ReplicaStatus(ctx context.Context) (ReplicaView, error) {
	_ = ctx
	paths, workspaceID, err := s.backupPaths()
	if err != nil {
		return ReplicaView{}, err
	}
	ident, err := s.ensureReplicaIdentity(paths, workspaceID, false)
	if err != nil {
		return ReplicaView{}, err
	}
	return ReplicaView{
		Kind:        "backup_replica",
		GeneratedAt: s.now(),
		Identity:    ident,
		Ref:         backupRefName(workspaceID, ident.ReplicaID),
	}, nil
}

func (s *ActionService) ResetReplica(ctx context.Context, yes bool) (ReplicaView, error) {
	_ = ctx
	if !yes {
		return ReplicaView{}, apperr.New(apperr.CodeInvalidInput, "replica reset requires --yes")
	}
	paths, workspaceID, err := s.backupPaths()
	if err != nil {
		return ReplicaView{}, err
	}
	ident, err := s.ensureReplicaIdentity(paths, workspaceID, true)
	if err != nil {
		return ReplicaView{}, err
	}
	return ReplicaView{
		Kind:        "backup_replica",
		GeneratedAt: s.now(),
		Identity:    ident,
		Ref:         backupRefName(workspaceID, ident.ReplicaID),
	}, nil
}

func (s *ActionService) ReconcileReplica(ctx context.Context, yes bool) (ReplicaView, error) {
	return s.ResetReplica(ctx, yes)
}

func newReplicaID() string {
	return uuid.NewString()
}
