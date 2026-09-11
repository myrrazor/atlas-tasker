package setup

import (
	"fmt"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
)

// Run-level status strings from the transaction model §7.
const (
	RunStatusConnected  = "connected"
	RunStatusPending    = "pending"
	RunStatusUnverified = "unverified"
	RunStatusPartial    = "partial"
	RunStatusFailed     = "failed"
)

// ProviderReport is one provider row in a setup or status report.
type ProviderReport struct {
	Target           integrations.Target `json:"target"`
	OperationState   OperationState      `json:"operation_state,omitempty"`
	IntegrationState adapter.State       `json:"integration_state,omitempty"`
	NoOp             bool                `json:"no_op,omitempty"`
	Selected         bool                `json:"selected"`
	MachineWide      bool                `json:"machine_wide"`
	RepairReason     string              `json:"repair_reason,omitempty"`
	Interrupted      bool                `json:"interrupted,omitempty"`
	Recovery         Recovery            `json:"recovery,omitempty"`
	SkillVersion     string              `json:"skill_version,omitempty"`
	ManagedBlock     string              `json:"managed_block_version,omitempty"`
	MCPRegistration  string              `json:"mcp_registration,omitempty"`
	MCPProfile       string              `json:"mcp_profile,omitempty"`
	ProviderVersion  string              `json:"provider_version,omitempty"`
	WorkspaceBinding string              `json:"workspace_binding,omitempty"`
	Actor            string              `json:"actor,omitempty"`
	ApprovalState    string              `json:"approval_restart_state,omitempty"`
}

// RunReport is the JSON payload for setup, status, repair, and removal.
type RunReport struct {
	Kind                  string           `json:"kind"`
	Status                string           `json:"status"`
	WorkspaceID           string           `json:"workspace_id"`
	WorkspaceRoot         string           `json:"workspace_root"`
	Providers             []ProviderReport `json:"providers"`
	ManagedMode           *ManagedModePlan `json:"managed_mode,omitempty"`
	Backup                *BackupPlan      `json:"backup,omitempty"`
	RemainingDependencies []string         `json:"remaining_dependencies,omitempty"`
	Fingerprint           string           `json:"fingerprint,omitempty"`
	RepairReason          string           `json:"repair_reason,omitempty"`
	LastVerified          string           `json:"last_verified_checkpoint,omitempty"`
	BackupWorker          string           `json:"backup_worker_state,omitempty"`
}

func deriveRunStatus(reports []ProviderReport) string {
	consented := 0
	kept := 0
	pendingHuman := 0
	unverified := 0
	failed := 0
	for _, report := range reports {
		if !report.Selected && report.OperationState == "" {
			continue
		}
		if report.NoOp && report.OperationState == "" {
			// A selected no-op still counts as a consented success with its
			// promised integration state.
			if report.Selected {
				consented++
				kept++
				switch report.IntegrationState {
				case adapter.StatePendingWorkspaceTrust, adapter.StatePendingMCPApproval:
					pendingHuman++
				case adapter.StateConfiguredUnverified, adapter.StatePortableReady, adapter.StateUnsupportedClientVersion:
					unverified++
				}
			}
			continue
		}
		consented++
		if report.OperationState.KeepsWrites() {
			kept++
			switch report.IntegrationState {
			case adapter.StatePendingWorkspaceTrust, adapter.StatePendingMCPApproval:
				pendingHuman++
			case adapter.StateConfiguredUnverified, adapter.StatePortableReady, adapter.StateUnsupportedClientVersion:
				unverified++
			}
		} else {
			failed++
		}
	}
	if consented == 0 {
		return RunStatusFailed
	}
	if kept == consented && pendingHuman == 0 && unverified == 0 {
		return RunStatusConnected
	}
	if kept == consented && pendingHuman > 0 {
		return RunStatusPending
	}
	if kept == consented {
		return RunStatusUnverified
	}
	if kept > 0 {
		return RunStatusPartial
	}
	return RunStatusFailed
}

func errorForRunStatus(status string) error {
	switch status {
	case RunStatusConnected, RunStatusPending, RunStatusUnverified:
		return nil
	case RunStatusPartial:
		return apperr.New(apperr.CodeConflict, "setup completed with partial success")
	case RunStatusFailed:
		return apperr.New(apperr.CodeInternal, "setup failed")
	default:
		return apperr.New(apperr.CodeInternal, fmt.Sprintf("unknown setup status %q", status))
	}
}
