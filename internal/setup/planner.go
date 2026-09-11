package setup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
)

const setupPlanFormat = "atlas_setup_plan_v1"

// PlanOptions selects what the read-only planner inspects and proposes.
type PlanOptions struct {
	Agents       []integrations.Target
	AgentsAll    bool
	Mode         contracts.ManagedMode
	Backup       bool
	BackupTarget string
	Team         string
}

// SetupPlan is the deterministic, secret-free plan for one setup run.
type SetupPlan struct {
	Format                string           `json:"format"`
	WorkspaceID           string           `json:"workspace_id"`
	WorkspaceRoot         string           `json:"workspace_root"`
	TrackerBinary         TrackerIdentity  `json:"tracker_binary"`
	Inspection            Inspection       `json:"inspection"`
	Providers             []ProviderPlan   `json:"providers"`
	ManagedMode           *ManagedModePlan `json:"managed_mode,omitempty"`
	Backup                *BackupPlan      `json:"backup,omitempty"`
	Team                  *TeamPlan        `json:"team,omitempty"`
	RemainingDependencies []string         `json:"remaining_dependencies,omitempty"`
	Fingerprint           string           `json:"fingerprint"`
	GeneratedAt           time.Time        `json:"generated_at"`
}

// ProviderPlan is the public (payload-free) view of one provider transaction.
type ProviderPlan struct {
	Target              integrations.Target     `json:"target"`
	Selected            bool                    `json:"selected"`
	MachineWide         bool                    `json:"machine_wide"`
	NoOp                bool                    `json:"no_op"`
	ResultingState      adapter.State           `json:"resulting_state,omitempty"`
	Plan                adapter.IntegrationPlan `json:"plan"`
	AdapterMissing      bool                    `json:"adapter_missing"`
	RemainingDependency string                  `json:"remaining_dependency,omitempty"`
}

// ManagedModePlan describes the managed-mode file write. The file lives under
// .tracker and is applied as an engine-owned transaction, not an adapter plan.
type ManagedModePlan struct {
	NoOp     bool                        `json:"no_op"`
	Path     string                      `json:"path"`
	Policy   contracts.ManagedModePolicy `json:"policy"`
	Declared string                      `json:"declared_mode"`
}

// BackupPlan is the backup consent group. Apply enables an already-configured target.
type BackupPlan struct {
	Requested             bool     `json:"requested"`
	TargetID              string   `json:"target_id,omitempty"`
	Writes                bool     `json:"writes"`
	Deferred              bool     `json:"deferred"`
	RemainingDependencies []string `json:"remaining_dependencies"`
	Notes                 []string `json:"notes"`
}

// TeamPlan records a requested team policy without silently changing roles.
// AT114-209 owns applying team policy and agent identity.
type TeamPlan struct {
	Requested           string `json:"requested"`
	Suggested           string `json:"suggested,omitempty"`
	Provider            string `json:"provider,omitempty"`
	Existing            string `json:"existing_completion_mode,omitempty"`
	Writes              bool   `json:"writes"`
	RemainingDependency string `json:"remaining_dependency,omitempty"`
}

// PreparedSetup is a plan plus the private payloads needed to apply it.
// Payloads never appear in the public JSON plan.
type PreparedSetup struct {
	Plan      SetupPlan
	Providers []preparedProvider
	ModeBody  []byte
}

func (e *Engine) Plan(opts PlanOptions) (*PreparedSetup, error) {
	if e.WorkspaceID == "" {
		return nil, apperr.New(apperr.CodeInvalidInput, "workspace identity is required")
	}
	inspection, err := inspectWorkspace(e.WorkspaceRoot, e.Home, e.StateDir, e.lookPath(), e.getenv())
	if err != nil {
		return nil, err
	}
	now := e.now()
	if err := e.ensureRegistry(); err != nil {
		return nil, err
	}
	selected, err := e.selectTargets(opts, inspection)
	if err != nil {
		return nil, err
	}
	if err := validateTeamName(opts.Team); err != nil {
		return nil, err
	}
	selectedList := sortedTargets(selected)
	providers := make([]preparedProvider, 0, len(integrations.DetectableTargets()))
	public := make([]ProviderPlan, 0, len(integrations.DetectableTargets()))
	deps := []string{}
	depSeen := map[string]struct{}{}
	addDep := func(dep string) {
		if dep == "" {
			return
		}
		if _, ok := depSeen[dep]; ok {
			return
		}
		depSeen[dep] = struct{}{}
		deps = append(deps, dep)
	}
	for _, target := range integrations.DetectableTargets() {
		_, chosen := selected[target]
		detection := detectionFor(inspection, target)
		var existing *adapter.IntegrationState
		if inspection.Manifest != nil {
			if row, ok := inspection.Manifest.integration(target); ok && row.Record != nil {
				existing = row.Record
			}
		}
		prepared, err := e.planTarget(target, detection, existing, actorHintFor(target, opts.Team, selectedList), now)
		if err != nil {
			return nil, err
		}
		prepared.Selected = chosen
		if !chosen {
			prepared.Plan.Steps = nil
			prepared.Plan.NoOp = true
			prepared.Payloads = nil
		}
		providers = append(providers, prepared)
		public = append(public, ProviderPlan{
			Target:              target,
			Selected:            chosen,
			MachineWide:         prepared.MachineWide,
			NoOp:                prepared.Plan.NoOp || !chosen,
			ResultingState:      prepared.ResultingState,
			Plan:                redactPlan(prepared.Plan),
			AdapterMissing:      prepared.AdapterMissing,
			RemainingDependency: prepared.RemainingDependency,
		})
		if chosen {
			addDep(prepared.RemainingDependency)
		}
	}
	var modePlan *ManagedModePlan
	var modeBody []byte
	if opts.Mode != "" {
		mode := opts.Mode
		if mode == contracts.ManagedModeDelivery {
			return nil, apperr.New(apperr.CodeInvalidInput, "delivery mode is not a setup default; enable it as a separate power-user action")
		}
		policy, err := contracts.ManagedModePolicyForMode(mode)
		if err != nil {
			return nil, apperr.New(apperr.CodeInvalidInput, err.Error())
		}
		encoded, err := policy.Encode()
		if err != nil {
			return nil, err
		}
		path := managedModePath(e.WorkspaceRoot)
		noOp := false
		if current, err := os.ReadFile(path); err == nil && string(current) == string(encoded) {
			noOp = true
		}
		modePlan = &ManagedModePlan{NoOp: noOp, Path: path, Policy: policy, Declared: string(mode)}
		if !noOp {
			modeBody = encoded
		}
	}
	var backupPlan *BackupPlan
	if opts.Backup || strings.TrimSpace(opts.BackupTarget) != "" {
		notes := []string{"backup is a separate consent group; --yes is not backup consent"}
		deps := []string{}
		if inspection.Scheduler.UserServicePresent {
			notes = append(notes, "user-level scheduler is already present")
		} else {
			deps = append(deps, "optional: tracker backup schedule install")
			notes = append(notes, "scheduler install remains a separate explicit consent")
		}
		backupPlan = &BackupPlan{
			Requested:             true,
			TargetID:              strings.TrimSpace(opts.BackupTarget),
			Writes:                true,
			Deferred:              false,
			RemainingDependencies: deps,
			Notes:                 notes,
		}
		for _, dep := range deps {
			addDep(dep)
		}
	}
	var teamPlan *TeamPlan
	if strings.TrimSpace(opts.Team) != "" {
		requested := strings.TrimSpace(opts.Team)
		teamPlan = &TeamPlan{
			Requested: requested,
			Suggested: suggestTeamPreset(len(selectedList)),
			Provider:  teamProvider(selectedList),
			Existing:  inspection.CompletionMode,
			Writes:    true,
		}
	}
	plan := SetupPlan{
		Format:                setupPlanFormat,
		WorkspaceID:           e.WorkspaceID,
		WorkspaceRoot:         e.WorkspaceRoot,
		TrackerBinary:         trackerIdentity(e.TrackerPath, e.TrackerVersion),
		Inspection:            inspection,
		Providers:             public,
		ManagedMode:           modePlan,
		Backup:                backupPlan,
		Team:                  teamPlan,
		RemainingDependencies: deps,
		GeneratedAt:           now,
	}
	fp, err := fingerprintSetupPlan(plan)
	if err != nil {
		return nil, err
	}
	plan.Fingerprint = fp
	return &PreparedSetup{Plan: plan, Providers: providers, ModeBody: modeBody}, nil
}

func (o PlanOptions) planOnlyInspection() bool {
	return len(o.Agents) == 0 && !o.AgentsAll && o.Mode == "" && !o.Backup && o.BackupTarget == ""
}

func (e *Engine) selectTargets(opts PlanOptions, inspection Inspection) (map[integrations.Target]struct{}, error) {
	chosen := map[integrations.Target]struct{}{}
	if opts.AgentsAll {
		for _, target := range integrations.DetectableTargets() {
			chosen[target] = struct{}{}
		}
		return chosen, nil
	}
	if len(opts.Agents) > 0 {
		for _, target := range opts.Agents {
			if !knownTarget(target) {
				return nil, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("unknown agent target %q", target))
			}
			chosen[target] = struct{}{}
		}
		return chosen, nil
	}
	for _, detection := range inspection.Detections {
		if detection.Found && detection.Target != integrations.TargetGeneric && detection.Target != integrations.TargetOpenClaw {
			chosen[detection.Target] = struct{}{}
		}
	}
	return chosen, nil
}

func knownTarget(target integrations.Target) bool {
	for _, candidate := range integrations.DetectableTargets() {
		if candidate == target {
			return true
		}
	}
	return false
}

func detectionFor(inspection Inspection, target integrations.Target) integrations.Detection {
	for _, detection := range inspection.Detections {
		if detection.Target == target {
			return detection
		}
	}
	return integrations.Detection{Target: target}
}

func redactPlan(plan adapter.IntegrationPlan) adapter.IntegrationPlan {
	clone := plan
	if clone.Warnings == nil {
		clone.Warnings = []string{}
	}
	return clone
}

func fingerprintSetupPlan(plan SetupPlan) (string, error) {
	clone := plan
	clone.Fingerprint = ""
	clone.GeneratedAt = time.Time{}
	clone.Inspection.Manifest = nil
	for i := range clone.Providers {
		clone.Providers[i].Plan.GeneratedAt = time.Time{}
		clone.Providers[i].Plan.PlanID = ""
	}
	raw, err := json.Marshal(clone)
	if err != nil {
		return "", fmt.Errorf("fingerprint setup plan: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func parseAgentList(raw string) ([]integrations.Target, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if raw == "all" {
		return append([]integrations.Target{}, integrations.DetectableTargets()...), nil
	}
	return integrations.ParseTargetList(raw)
}

func executablePath() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return filepath.Clean(path), nil
}

func sortedTargets(set map[integrations.Target]struct{}) []integrations.Target {
	out := make([]integrations.Target, 0, len(set))
	for target := range set {
		out = append(out, target)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
