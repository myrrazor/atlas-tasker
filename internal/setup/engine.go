package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

// Engine is the Sprint 114.1 setup orchestrator.
type Engine struct {
	StateDir       string
	Home           string
	WorkspaceRoot  string
	WorkspaceID    string
	TrackerPath    string
	TrackerVersion string
	Registry       *adapter.Registry
	LookPath       func(string) (string, error)
	Getenv         func(string) string
	Now            func() time.Time
	Stdin          io.Reader
	Stdout         io.Writer
	Hooks          Hooks
	Runner         CommandRunner
	ProbeAbsent    func(cmd adapter.Command) (bool, error)
	currentUID     int
}

func (e *Engine) lookPath() func(string) (string, error) {
	if e.LookPath != nil {
		return e.LookPath
	}
	return nil
}

func (e *Engine) getenv() func(string) string {
	if e.Getenv != nil {
		return e.Getenv
	}
	return os.Getenv
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now().UTC()
}

// NewEngine fills defaults for a workspace that is already initialized.
func NewEngine(workspaceRoot string) (*Engine, error) {
	root, err := service.InitializedWorkspaceRoot(workspaceRoot)
	if err != nil {
		return nil, err
	}
	id, err := service.LoadWorkspaceIdentity(root)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(id) == "" {
		return nil, apperr.New(apperr.CodeInvalidInput, "workspace identity is missing")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	stateDir, err := DefaultStateDir(home, os.Getenv)
	if err != nil {
		return nil, err
	}
	tracker, err := executablePath()
	if err != nil {
		return nil, err
	}
	return &Engine{
		StateDir:      stateDir,
		Home:          filepath.Clean(home),
		WorkspaceRoot: root,
		WorkspaceID:   id,
		TrackerPath:   tracker,
		currentUID:    os.Getuid(),
	}, nil
}

// ApplyOptions controls consent and confirmation.
type ApplyOptions struct {
	Yes              bool
	Interactive      bool
	ConfirmRemoval   bool
	PlanOnly         bool
	AllowMachineWide bool
	AlreadyLocked    bool
}

// PlanAndMaybeApply builds a plan and, unless PlanOnly, applies consented
// transactions. Planning itself writes nothing.
func (e *Engine) PlanAndMaybeApply(ctx context.Context, planOpts PlanOptions, applyOpts ApplyOptions) (*SetupPlan, *RunReport, error) {
	prepared, err := e.Plan(planOpts)
	if err != nil {
		return nil, nil, err
	}
	if applyOpts.PlanOnly {
		report := e.reportFromPlan(prepared, "setup_plan")
		return &prepared.Plan, report, nil
	}
	if err := e.requireConsent(prepared, applyOpts); err != nil {
		return &prepared.Plan, nil, err
	}
	report, err := e.ApplyPrepared(ctx, prepared, applyOpts)
	return &prepared.Plan, report, err
}

// PlanReport is the secret-free JSON view of a plan. Planning itself writes nothing.
func (e *Engine) PlanReport(prepared *PreparedSetup) *RunReport {
	return e.reportFromPlan(prepared, "setup_plan")
}

func (e *Engine) requireConsent(prepared *PreparedSetup, applyOpts ApplyOptions) error {
	if applyOpts.Yes || applyOpts.Interactive {
		for _, provider := range prepared.Providers {
			if !provider.Selected || !provider.MachineWide {
				continue
			}
			named := false
			for _, row := range prepared.Plan.Providers {
				if row.Target == provider.Target && row.Selected && row.MachineWide {
					named = true
				}
			}
			if !named {
				return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("%s writes machine-wide state and must be named explicitly", provider.Target))
			}
			if applyOpts.Yes && !applyOpts.AllowMachineWide {
				return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("--yes alone is not consent for machine-wide %s; name it with --agents", provider.Target))
			}
		}
		return nil
	}
	return apperr.New(apperr.CodeInvalidInput, "noninteractive setup requires --yes or --plan")
}

func (e *Engine) reportFromPlan(prepared *PreparedSetup, kind string) *RunReport {
	reports := make([]ProviderReport, 0, len(prepared.Providers))
	for _, provider := range prepared.Providers {
		if !provider.Selected {
			continue
		}
		reports = append(reports, ProviderReport{
			Target:           provider.Target,
			Selected:         true,
			MachineWide:      provider.MachineWide,
			NoOp:             provider.Plan.NoOp,
			IntegrationState: provider.ResultingState,
		})
	}
	return &RunReport{
		Kind:                  kind,
		Status:                deriveRunStatus(reports),
		WorkspaceID:           e.WorkspaceID,
		WorkspaceRoot:         e.WorkspaceRoot,
		Providers:             reports,
		ManagedMode:           prepared.Plan.ManagedMode,
		Backup:                prepared.Plan.Backup,
		RemainingDependencies: prepared.Plan.RemainingDependencies,
		Fingerprint:           prepared.Plan.Fingerprint,
	}
}

// ApplyPrepared applies an already-built plan under the setup lock.
func (e *Engine) ApplyPrepared(ctx context.Context, prepared *PreparedSetup, applyOpts ApplyOptions) (*RunReport, error) {
	if allNoOp(prepared) {
		report := e.reportFromPlan(prepared, "setup_result")
		e.applyBackupGroup(ctx, prepared, report)
		report.Status = deriveRunStatus(report.Providers)
		return report, errorForRunStatus(report.Status)
	}
	fresh, err := e.Plan(PlanOptions{
		Agents:       selectedTargets(prepared),
		Mode:         modeFromPlan(prepared),
		Backup:       prepared.Plan.Backup != nil,
		BackupTarget: backupTarget(prepared),
		Team:         teamFromPlan(prepared),
	})
	if err != nil {
		return nil, err
	}
	if fresh.Plan.Fingerprint != prepared.Plan.Fingerprint {
		return nil, apperr.New(apperr.CodeConflict, "stale plan: the workspace changed after planning")
	}
	if !applyOpts.AlreadyLocked {
		release, err := AcquireSetupLock(e.StateDir, "tracker setup")
		if err != nil {
			return nil, err
		}
		defer func() { _ = release() }()
	}
	if err := e.recoverInFlight(ctx); err != nil && !errors.Is(err, ErrInjectedCrash) {
		return nil, err
	}
	reports := []ProviderReport{}
	for _, provider := range prepared.Providers {
		if !provider.Selected {
			continue
		}
		report, applyErr := e.applyProvider(ctx, provider)
		if applyErr != nil && !errors.Is(applyErr, ErrInjectedCrash) && report.OperationState == "" {
			report.RepairReason = applyErr.Error()
			report.OperationState = StateRolledBack
		}
		reports = append(reports, report)
		if errors.Is(applyErr, ErrInjectedCrash) {
			out := &RunReport{Kind: "setup_result", WorkspaceID: e.WorkspaceID, WorkspaceRoot: e.WorkspaceRoot, Providers: reports, RemainingDependencies: prepared.Plan.RemainingDependencies}
			out.Status = deriveRunStatus(reports)
			return out, applyErr
		}
	}
	if prepared.Plan.ManagedMode != nil && !prepared.Plan.ManagedMode.NoOp && len(prepared.ModeBody) > 0 {
		if err := e.applyManagedMode(prepared.Plan.ManagedMode.Path, prepared.ModeBody, prepared.Plan.ManagedMode.Policy); err != nil {
			reports = append(reports, ProviderReport{Target: "managed_mode", Selected: true, OperationState: StateFailed, RepairReason: err.Error()})
		} else {
			reports = append(reports, ProviderReport{Target: "managed_mode", Selected: true, OperationState: StateConnected, IntegrationState: adapter.StateConnected})
		}
	}
	if prepared.Plan.Team != nil && prepared.Plan.Team.Writes {
		if err := e.applyTeam(ctx, prepared.Plan.Team); err != nil {
			reports = append(reports, ProviderReport{Target: "team", Selected: true, OperationState: StateFailed, RepairReason: err.Error()})
		} else {
			reports = append(reports, ProviderReport{Target: "team", Selected: true, OperationState: StateConnected, IntegrationState: adapter.StateConnected})
		}
	}
	report := &RunReport{
		Kind:                  "setup_result",
		WorkspaceID:           e.WorkspaceID,
		WorkspaceRoot:         e.WorkspaceRoot,
		Providers:             reports,
		ManagedMode:           prepared.Plan.ManagedMode,
		Backup:                prepared.Plan.Backup,
		RemainingDependencies: prepared.Plan.RemainingDependencies,
		Fingerprint:           prepared.Plan.Fingerprint,
	}
	e.applyBackupGroup(ctx, prepared, report)
	report.Status = deriveRunStatus(report.Providers)
	if report.Status == RunStatusConnected && hasUnverified(report.Providers) {
		report.Status = RunStatusUnverified
	}
	return report, errorForRunStatus(report.Status)
}

func hasUnverified(reports []ProviderReport) bool {
	for _, report := range reports {
		switch report.IntegrationState {
		case adapter.StateConfiguredUnverified, adapter.StatePortableReady, adapter.StateUnsupportedClientVersion:
			return true
		}
	}
	return false
}

func allNoOp(prepared *PreparedSetup) bool {
	for _, provider := range prepared.Providers {
		if provider.Selected && !provider.Plan.NoOp {
			return false
		}
	}
	if prepared.Plan.ManagedMode != nil && !prepared.Plan.ManagedMode.NoOp {
		return false
	}
	if prepared.Plan.Team != nil && prepared.Plan.Team.Writes {
		return false
	}
	return true
}

func (e *Engine) applyBackupGroup(ctx context.Context, prepared *PreparedSetup, report *RunReport) {
	if report == nil || prepared == nil || prepared.Plan.Backup == nil || !prepared.Plan.Backup.Requested {
		return
	}
	targetID := prepared.Plan.Backup.TargetID
	fail := func(err error) {
		report.Providers = append(report.Providers, ProviderReport{Target: "backup", Selected: true, OperationState: StateRolledBack, RepairReason: err.Error()})
		report.BackupWorker = "failed"
	}
	ok := func(verifiedID string) {
		report.Providers = append(report.Providers, ProviderReport{Target: "backup", Selected: true, OperationState: StateConnected})
		report.BackupWorker = "enabled"
		if verifiedID != "" {
			report.LastVerified = verifiedID
		}
	}
	if e.Hooks.BackupApply != nil {
		if err := e.Hooks.BackupApply(); err != nil {
			fail(err)
			return
		}
		ok("")
		return
	}
	if e.Hooks.BackupFirstCheckpoint != nil {
		result, err := e.Hooks.BackupFirstCheckpoint(ctx, targetID)
		if err != nil {
			fail(err)
			return
		}
		if result.Verified {
			ok(result.CheckpointID)
			return
		}
		report.Providers = append(report.Providers, ProviderReport{
			Target:           "backup",
			Selected:         true,
			OperationState:   StatePendingApproval,
			IntegrationState: adapter.StateConfiguredUnverified,
		})
		report.BackupWorker = "enabled"
		return
	}
	if err := e.enableConfiguredBackup(targetID); err != nil {
		fail(err)
		return
	}
	ok("")
}

func (e *Engine) enableConfiguredBackup(targetID string) error {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return apperr.New(apperr.CodeInvalidInput, "backup target id is required")
	}
	path := filepath.Join(e.StateDir, "backups", e.WorkspaceID, "targets.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return apperr.New(apperr.CodeNotFound, "backup target is not configured; run tracker backup target add")
	}
	var store struct {
		Targets []struct {
			TargetID string `json:"target_id"`
		} `json:"targets"`
	}
	if err := json.Unmarshal(raw, &store); err != nil {
		return err
	}
	found := false
	for _, target := range store.Targets {
		if target.TargetID == targetID {
			found = true
			break
		}
	}
	if !found {
		return apperr.New(apperr.CodeNotFound, "backup target not found: "+targetID)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(map[string]any{
		"format": "atlas_backup_auto_v1", "enabled": true, "default_target_id": targetID,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "auto.json"), append(body, '\n'), 0o600)
}

func selectedTargets(prepared *PreparedSetup) []integrations.Target {
	var out []integrations.Target
	for _, provider := range prepared.Providers {
		if provider.Selected {
			out = append(out, provider.Target)
		}
	}
	return out
}

func modeFromPlan(prepared *PreparedSetup) contracts.ManagedMode {
	if prepared.Plan.ManagedMode == nil {
		return ""
	}
	return prepared.Plan.ManagedMode.Policy.Mode
}

func backupTarget(prepared *PreparedSetup) string {
	if prepared.Plan.Backup == nil {
		return ""
	}
	return prepared.Plan.Backup.TargetID
}

func teamFromPlan(prepared *PreparedSetup) string {
	if prepared.Plan.Team == nil {
		return ""
	}
	return prepared.Plan.Team.Requested
}

func (e *Engine) applyManagedMode(path string, body []byte, policy contracts.ManagedModePolicy) error {
	opID := fmt.Sprintf("managed-mode-%d", e.now().UnixNano())
	entry := &JournalEntry{
		OperationID:   opID,
		Kind:          JournalKindManagedMode,
		WorkspaceID:   e.WorkspaceID,
		WorkspaceRoot: e.WorkspaceRoot,
		State:         StatePlanned,
		CreatedAt:     e.now(),
		Steps: []journalStepRecord{{
			StepID:   "managed-mode-file",
			Kind:     adapter.StepWriteManagedFile,
			Path:     path,
			Rollback: &adapter.RollbackAction{Kind: adapter.RollbackRestoreSnapshot, Path: path},
		}},
	}
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		entry.Steps[0].Rollback = &adapter.RollbackAction{Kind: adapter.RollbackDeleteCreated, Path: path}
	}
	if err := writeJournal(e.StateDir, entry); err != nil {
		return err
	}
	if err := markState(entry, StateApplying); err != nil {
		return err
	}
	if err := writeJournal(e.StateDir, entry); err != nil {
		return err
	}
	before, err := InspectFile(path, e.currentUID)
	if err != nil {
		return err
	}
	entry.Steps[0].Before = &before
	entry.Steps[0].Created = !before.Exists
	entry.Steps[0].Started = true
	if before.Exists {
		if err := os.MkdirAll(rollbackDir(e.StateDir, opID), dirPerm); err != nil {
			return err
		}
		if err := copyFile(path, filepath.Join(rollbackDir(e.StateDir, opID), "managed-mode-file"), filePerm); err != nil {
			return err
		}
	}
	if err := writeJournal(e.StateDir, entry); err != nil {
		return err
	}
	prepared := preparedProvider{Plan: adapter.IntegrationPlan{Steps: stepsFromJournal(entry)}}
	if err := e.withWorkspaceLock(context.Background(), path, true, func() error {
		return atomicWriteFile(path, body, 0o644)
	}); err != nil {
		return e.failToRollback(context.Background(), entry, prepared, err)
	}
	after, err := InspectFile(path, e.currentUID)
	if err != nil {
		return e.failToRollback(context.Background(), entry, prepared, err)
	}
	entry.Steps[0].After = &after
	entry.Steps[0].Done = true
	if err := markState(entry, StateApplied); err != nil {
		return err
	}
	if err := writeJournal(e.StateDir, entry); err != nil {
		return err
	}
	if err := markState(entry, StateVerifying); err != nil {
		return err
	}
	current, err := os.ReadFile(path)
	if err != nil || string(current) != string(body) {
		return e.failToRollback(context.Background(), entry, prepared, fmt.Errorf("managed-mode file did not match the planned policy"))
	}
	if err := markState(entry, StateConnected); err != nil {
		return err
	}
	if err := writeJournal(e.StateDir, entry); err != nil {
		return err
	}
	manifest, err := loadManifest(e.StateDir, e.WorkspaceID)
	if err != nil {
		return err
	}
	if manifest == nil {
		manifest = emptyManifest(e.WorkspaceID, e.WorkspaceRoot, trackerIdentity(e.TrackerPath, e.TrackerVersion))
	}
	manifest.ManagedMode = ManifestManagedMode{
		Enabled:       policy.Mode.TracksWork(),
		CapturePolicy: string(policy.CapturePolicy),
		StatusSource:  string(policy.StatusPolicy),
		DeclaredMode:  string(policy.Mode),
	}
	if err := saveManifest(e.StateDir, manifest); err != nil {
		return err
	}
	_ = os.RemoveAll(rollbackDir(e.StateDir, opID))
	return nil
}

func (e *Engine) recoverManagedMode(ctx context.Context, entry *JournalEntry) error {
	if len(entry.Steps) == 0 || entry.Steps[0].Path == "" {
		return fmt.Errorf("managed-mode journal is missing its path")
	}
	path := entry.Steps[0].Path
	raw, err := os.ReadFile(path)
	if err != nil {
		return e.failToRollback(ctx, entry, preparedProvider{Plan: adapter.IntegrationPlan{Steps: stepsFromJournal(entry)}}, err)
	}
	policy, err := contracts.ParseManagedModePolicy(raw)
	if err != nil {
		return e.failToRollback(ctx, entry, preparedProvider{Plan: adapter.IntegrationPlan{Steps: stepsFromJournal(entry)}}, err)
	}
	manifest, err := loadManifest(e.StateDir, e.WorkspaceID)
	if err != nil {
		return err
	}
	if manifest == nil {
		manifest = emptyManifest(e.WorkspaceID, e.WorkspaceRoot, trackerIdentity(e.TrackerPath, e.TrackerVersion))
	}
	manifest.ManagedMode = ManifestManagedMode{
		Enabled:       policy.Mode.TracksWork(),
		CapturePolicy: string(policy.CapturePolicy),
		StatusSource:  string(policy.StatusPolicy),
		DeclaredMode:  string(policy.Mode),
	}
	if err := saveManifest(e.StateDir, manifest); err != nil {
		return err
	}
	entry.Integration = adapter.StateConnected
	if err := markState(entry, StateConnected); err != nil {
		return err
	}
	if err := writeJournal(e.StateDir, entry); err != nil {
		return err
	}
	_ = os.RemoveAll(rollbackDir(e.StateDir, entry.OperationID))
	return nil
}

func (e *Engine) recoverInFlight(ctx context.Context) error {
	entries, err := listJournals(e.StateDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.State.InFlight() {
			continue
		}
		target, err := entry.State.RecoveryTarget()
		if err != nil {
			return err
		}
		if err := markState(entry, target); err != nil {
			return err
		}
		if err := writeJournal(e.StateDir, entry); err != nil {
			return err
		}
		action, err := entry.State.RecoveryAction()
		if err != nil {
			return err
		}
		prepared := preparedProvider{Target: entry.Target, Plan: adapter.IntegrationPlan{Steps: stepsFromJournal(entry)}}
		switch action {
		case RecoveryRollBack, RecoveryResumeRollback:
			if err := e.rollback(ctx, entry, prepared); err != nil {
				return err
			}
		case RecoveryVerify:
			if err := e.recoverVerify(ctx, entry); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *Engine) recoverVerify(ctx context.Context, entry *JournalEntry) error {
	if entry.Kind == JournalKindManagedMode {
		return e.recoverManagedMode(ctx, entry)
	}
	if entry.Kind != JournalKindProvider {
		state := entry.Integration
		if state == "" {
			state = adapter.StateConfiguredUnverified
		}
		outcome, err := OutcomeFor(state)
		if err != nil {
			return err
		}
		entry.Integration = state
		if err := markState(entry, outcome); err != nil {
			return err
		}
		return writeJournal(e.StateDir, entry)
	}
	inspection, err := inspectWorkspace(e.WorkspaceRoot, e.Home, e.StateDir, e.lookPath(), e.getenv())
	if err != nil {
		return err
	}
	var existing *adapter.IntegrationState
	if inspection.Manifest != nil {
		if row, ok := inspection.Manifest.integration(entry.Target); ok {
			existing = row.Record
		}
	}
	prepared, err := e.planTarget(entry.Target, detectionFor(inspection, entry.Target), existing, contracts.Actor("agent:"+string(entry.Target)), e.now())
	if err != nil {
		return e.rollback(ctx, entry, preparedProvider{Target: entry.Target, Plan: adapter.IntegrationPlan{Steps: stepsFromJournal(entry)}})
	}
	state, err := e.verifyPrepared(prepared)
	if err != nil {
		return e.failToRollback(ctx, entry, prepared, err)
	}
	outcome, err := OutcomeFor(state)
	if err != nil {
		return e.failToRollback(ctx, entry, prepared, err)
	}
	if outcome == StateRollingBack {
		return e.failToRollback(ctx, entry, prepared, fmt.Errorf("verification reported failed"))
	}
	entry.Integration = state
	if err := markState(entry, outcome); err != nil {
		return err
	}
	if err := writeJournal(e.StateDir, entry); err != nil {
		return err
	}
	if outcome.KeepsWrites() {
		if err := e.commitProvider(entry, prepared, state); err != nil {
			return err
		}
		_ = os.RemoveAll(rollbackDir(e.StateDir, entry.OperationID))
	}
	return nil
}

func stepsFromJournal(entry *JournalEntry) []adapter.PlanStep {
	out := make([]adapter.PlanStep, 0, len(entry.Steps))
	for _, rec := range entry.Steps {
		out = append(out, adapter.PlanStep{StepID: rec.StepID, Kind: rec.Kind, Path: rec.Path, Rollback: rec.Rollback})
	}
	return out
}

// StatusReport reads the manifest, journal, and live inspection. It does not
// recover in-flight journals; that is repair/apply under the setup lock.
func (e *Engine) StatusReport() (*RunReport, error) {
	inspection, err := inspectWorkspace(e.WorkspaceRoot, e.Home, e.StateDir, e.lookPath(), e.getenv())
	if err != nil {
		return nil, err
	}
	journals, err := listJournals(e.StateDir)
	if err != nil {
		return nil, err
	}
	inFlight := map[integrations.Target]*JournalEntry{}
	for _, entry := range journals {
		if entry.Kind == JournalKindProvider && entry.State.InFlight() {
			inFlight[entry.Target] = entry
		}
	}
	reports := []ProviderReport{}
	for _, target := range integrations.DetectableTargets() {
		report := ProviderReport{Target: target}
		if row, ok := inspection.Manifest.integration(target); ok {
			report.Selected = true
			report.IntegrationState = row.State
			report.SkillVersion = row.SkillVersion
			report.ManagedBlock = row.ManagedBlock
			report.MCPProfile = row.MCPProfile
			report.WorkspaceBinding = row.WorkspaceBinding
			report.Actor = string(row.Actor)
			report.RepairReason = row.RepairReason
			if row.Record != nil {
				report.ProviderVersion = row.Record.ClientVersion.Raw
				if row.Record.Registration != nil {
					report.MCPRegistration = row.Record.Registration.ServerName
				}
			}
			report.OperationState = StateConnected
			if !row.State.Verified() {
				report.OperationState = StatePendingApproval
			}
		}
		if entry, ok := inFlight[target]; ok {
			report.Selected = true
			report.Interrupted = true
			report.OperationState = entry.State
			if action, err := entry.State.RecoveryAction(); err == nil {
				report.Recovery = action
			}
			report.RepairReason = "interrupted, will recover on next run"
		}
		if report.Selected {
			reports = append(reports, report)
		}
	}
	var modePlan *ManagedModePlan
	if raw, err := os.ReadFile(managedModePath(e.WorkspaceRoot)); err == nil {
		if policy, err := contracts.ParseManagedModePolicy(raw); err == nil {
			modePlan = &ManagedModePlan{NoOp: true, Path: managedModePath(e.WorkspaceRoot), Policy: policy, Declared: string(policy.Mode)}
		}
	}
	status := deriveRunStatus(reports)
	if len(reports) == 0 {
		status = RunStatusUnverified
	}
	repair := ""
	manifest := inspection.Manifest
	if manifest == nil {
		manifest, _ = loadManifest(e.StateDir, e.WorkspaceID)
	}
	if manifest != nil && manifest.TrackerBinary.SHA256 != "" {
		current := trackerIdentity(e.TrackerPath, e.TrackerVersion)
		if current.SHA256 != "" && current.SHA256 != manifest.TrackerBinary.SHA256 {
			repair = "binary relocation: tracker executable hash changed"
			status = RunStatusPartial
		}
		if manifest.WorkspacePath != "" && manifest.WorkspacePath != e.WorkspaceRoot {
			repair = "workspace relocation: registered path does not match this directory"
			status = RunStatusPartial
		}
	}
	if repair == "" {
		if reason := e.unmanagedMCPReason(inspection); reason != "" {
			repair = reason
		}
	}
	worker, lastVerified := e.backupStatusFields()
	return &RunReport{
		Kind:          "setup_status",
		Status:        status,
		WorkspaceID:   e.WorkspaceID,
		WorkspaceRoot: e.WorkspaceRoot,
		Providers:     reports,
		ManagedMode:   modePlan,
		RepairReason:  repair,
		BackupWorker:  worker,
		LastVerified:  lastVerified,
	}, nil
}

func (e *Engine) backupStatusFields() (string, string) {
	if e.WorkspaceID == "" || e.StateDir == "" {
		return "not_configured", ""
	}
	dir := filepath.Join(e.StateDir, "backups", e.WorkspaceID)
	if _, err := os.Stat(filepath.Join(dir, "auto.json")); err != nil {
		if _, err := os.Stat(filepath.Join(dir, "targets.json")); err != nil {
			return "not_configured", ""
		}
		return "configured", ""
	}
	raw, err := os.ReadFile(filepath.Join(dir, "ledger.json"))
	if err != nil {
		return "enabled", ""
	}
	var ledger struct {
		LastRemoteCheckpointID string    `json:"last_remote_checkpoint_id"`
		LastVerifiedCommit     string    `json:"last_verified_commit"`
		LastVerifiedAt         time.Time `json:"last_verified_at"`
	}
	if err := json.Unmarshal(raw, &ledger); err != nil {
		return "enabled", ""
	}
	if ledger.LastVerifiedCommit == "" || ledger.LastVerifiedAt.IsZero() {
		return "enabled", ""
	}
	if ledger.LastRemoteCheckpointID != "" {
		return "enabled", ledger.LastRemoteCheckpointID
	}
	return "enabled", ledger.LastVerifiedCommit
}

func (e *Engine) unmanagedMCPReason(inspection Inspection) string {
	for _, sighting := range inspection.MCPSightings {
		if !sighting.Present {
			continue
		}
		owned := false
		if inspection.Manifest != nil {
			if row, ok := inspection.Manifest.integration(sighting.Target); ok && row.Record != nil && row.Record.Registration != nil {
				owned = true
			}
		}
		if !owned {
			return "unmanaged MCP registration named atlas is present; repair will not rewrite it"
		}
	}
	return ""
}

// Repair recovers in-flight journals and refreshes drifted skill files.
func (e *Engine) Repair(ctx context.Context, target integrations.Target, yes bool) (*RunReport, error) {
	release, err := AcquireSetupLock(e.StateDir, "tracker setup repair")
	if err != nil {
		return nil, err
	}
	defer func() { _ = release() }()
	if err := e.recoverInFlight(ctx); err != nil {
		return nil, err
	}
	opts := PlanOptions{}
	if target != "" {
		opts.Agents = []integrations.Target{target}
	} else {
		opts.AgentsAll = true
	}
	prepared, err := e.Plan(opts)
	if err != nil {
		return nil, err
	}
	if !yes {
		report := e.reportFromPlan(prepared, "setup_repair_plan")
		if reason := e.relocationReason(); reason != "" {
			report.RepairReason = reason
			report.Status = RunStatusPartial
		}
		return report, nil
	}
	report, err := e.ApplyPrepared(ctx, prepared, ApplyOptions{Yes: true, AlreadyLocked: true, AllowMachineWide: target == integrations.TargetOpenClaw || opts.AgentsAll})
	if bindErr := e.refreshManifestBinding(); bindErr != nil && err == nil {
		return report, bindErr
	}
	if report != nil {
		if reason := e.relocationReason(); reason != "" {
			report.RepairReason = reason
		} else {
			report.RepairReason = ""
		}
	}
	return report, err
}

func (e *Engine) relocationReason() string {
	manifest, err := loadManifest(e.StateDir, e.WorkspaceID)
	if err != nil || manifest == nil {
		return ""
	}
	current := trackerIdentity(e.TrackerPath, e.TrackerVersion)
	if manifest.TrackerBinary.SHA256 != "" && current.SHA256 != "" && current.SHA256 != manifest.TrackerBinary.SHA256 {
		return "binary relocation: tracker executable hash changed"
	}
	if manifest.WorkspacePath != "" && manifest.WorkspacePath != e.WorkspaceRoot {
		return "workspace relocation: registered path does not match this directory"
	}
	return ""
}

func (e *Engine) refreshManifestBinding() error {
	manifest, err := loadManifest(e.StateDir, e.WorkspaceID)
	if err != nil {
		return err
	}
	if manifest == nil {
		manifest = emptyManifest(e.WorkspaceID, e.WorkspaceRoot, trackerIdentity(e.TrackerPath, e.TrackerVersion))
	}
	manifest.TrackerBinary = trackerIdentity(e.TrackerPath, e.TrackerVersion)
	manifest.WorkspacePath = e.WorkspaceRoot
	if err := saveManifest(e.StateDir, manifest); err != nil {
		return err
	}
	e.rememberWorkspace()
	return nil
}

// Disconnect removes Atlas-owned files for one target.
func (e *Engine) Disconnect(ctx context.Context, target integrations.Target, yes bool) (*RunReport, error) {
	if target == "" {
		return nil, apperr.New(apperr.CodeInvalidInput, "disconnect requires a target")
	}
	inspection, err := inspectWorkspace(e.WorkspaceRoot, e.Home, e.StateDir, e.lookPath(), e.getenv())
	if err != nil {
		return nil, err
	}
	var existing *adapter.IntegrationState
	if inspection.Manifest != nil {
		if row, ok := inspection.Manifest.integration(target); ok {
			existing = row.Record
		}
	}
	prepared, err := e.planDisconnect(target, detectionFor(inspection, target), existing, yes)
	if err != nil {
		return nil, err
	}
	prepared.Selected = true
	if !yes {
		return &RunReport{
			Kind:          "integrations_disconnect_plan",
			Status:        RunStatusUnverified,
			WorkspaceID:   e.WorkspaceID,
			WorkspaceRoot: e.WorkspaceRoot,
			Providers:     []ProviderReport{{Target: target, Selected: true, NoOp: prepared.Plan.NoOp}},
		}, nil
	}
	release, err := AcquireSetupLock(e.StateDir, "tracker integrations disconnect")
	if err != nil {
		return nil, err
	}
	defer func() { _ = release() }()
	report, err := e.applyProvider(ctx, prepared)
	if err != nil {
		return nil, err
	}
	if manifest, loadErr := loadManifest(e.StateDir, e.WorkspaceID); loadErr == nil && manifest != nil {
		delete(manifest.Integrations, integrationKey(target))
		_ = saveManifest(e.StateDir, manifest)
	}
	return &RunReport{
		Kind:          "integrations_disconnect",
		Status:        RunStatusUnverified,
		WorkspaceID:   e.WorkspaceID,
		WorkspaceRoot: e.WorkspaceRoot,
		Providers:     []ProviderReport{report},
	}, nil
}

func encodeReport(report *RunReport) []byte {
	raw, _ := json.MarshalIndent(report, "", "  ")
	return raw
}
