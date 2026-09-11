package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

// ErrInjectedCrash stops an operation at a journaled crash-injection point
// so tests can resume from the persisted state.
var ErrInjectedCrash = errors.New("injected setup crash")

// Hooks injects crash and observation points for AT114-103.
type Hooks struct {
	BeforeFirstWrite func() error
	AfterSnapshot    func(step adapter.PlanStep) error
	AfterStep        func(step adapter.PlanStep) error
	AfterVerify      func() error
	DuringRollback   func(step adapter.PlanStep) error
	// BackupApply is optional. Sprint 114.1 never writes backup state; tests
	// inject a failure here to prove agents stay connected (AT114-104).
	BackupApply func() error
}

type CommandRunner func(cmd adapter.Command) (int, error)

func (e *Engine) applyProvider(ctx context.Context, prepared preparedProvider) (ProviderReport, error) {
	report := ProviderReport{
		Target:           prepared.Target,
		Selected:         true,
		MachineWide:      prepared.MachineWide,
		IntegrationState: prepared.ResultingState,
	}
	if prepared.Plan.NoOp {
		report.NoOp = true
		report.OperationState = ""
		return report, nil
	}
	opID := fmt.Sprintf("%s-%s-%d", prepared.Target, prepared.Plan.Operation, e.now().UnixNano())
	fp, err := prepared.Plan.Fingerprint()
	if err != nil {
		return report, err
	}
	entry := &JournalEntry{
		OperationID:     opID,
		Kind:            JournalKindProvider,
		Target:          prepared.Target,
		WorkspaceID:     e.WorkspaceID,
		WorkspaceRoot:   e.WorkspaceRoot,
		PlanFingerprint: fp,
		State:           StatePlanned,
		CreatedAt:       e.now(),
		Steps:           journalStepsFromPlan(prepared.Plan),
	}
	if err := writeJournal(e.StateDir, entry); err != nil {
		return report, err
	}
	if err := e.runOperation(ctx, entry, prepared); err != nil {
		report.OperationState = entry.State
		report.IntegrationState = entry.Integration
		report.RepairReason = err.Error()
		if errors.Is(err, ErrInjectedCrash) {
			return report, err
		}
		return report, err
	}
	report.OperationState = entry.State
	report.IntegrationState = entry.Integration
	return report, nil
}

func journalStepsFromPlan(plan adapter.IntegrationPlan) []journalStepRecord {
	out := make([]journalStepRecord, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		out = append(out, journalStepRecord{
			StepID:   step.StepID,
			Kind:     step.Kind,
			Path:     step.Path,
			Rollback: step.Rollback,
		})
	}
	return out
}

func (e *Engine) runOperation(ctx context.Context, entry *JournalEntry, prepared preparedProvider) error {
	if err := markState(entry, StateApplying); err != nil {
		return err
	}
	if err := writeJournal(e.StateDir, entry); err != nil {
		return err
	}
	firstWrite := true
	for i := range prepared.Plan.Steps {
		step := prepared.Plan.Steps[i]
		if !step.Writes() {
			continue
		}
		if firstWrite && e.Hooks.BeforeFirstWrite != nil {
			if err := e.Hooks.BeforeFirstWrite(); err != nil {
				return e.failToRollback(ctx, entry, prepared, err)
			}
		}
		firstWrite = false
		if err := e.applyStep(ctx, entry, prepared, i, step); err != nil {
			return e.failToRollback(ctx, entry, prepared, err)
		}
	}
	if err := markState(entry, StateApplied); err != nil {
		return err
	}
	if err := writeJournal(e.StateDir, entry); err != nil {
		return err
	}
	if err := markState(entry, StateVerifying); err != nil {
		return err
	}
	if err := writeJournal(e.StateDir, entry); err != nil {
		return err
	}
	state, err := e.verifyPrepared(prepared)
	if err != nil {
		return e.failToRollback(ctx, entry, prepared, err)
	}
	if state == "" && prepared.Plan.Operation == adapter.PlanOperationRemove {
		state = adapter.StateConnected
	}
	if e.Hooks.AfterVerify != nil {
		if err := e.Hooks.AfterVerify(); err != nil {
			return e.failToRollback(ctx, entry, prepared, err)
		}
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
		if prepared.Plan.Operation != adapter.PlanOperationRemove {
			if err := e.commitProvider(entry, prepared, state); err != nil {
				return err
			}
		}
		_ = os.RemoveAll(rollbackDir(e.StateDir, entry.OperationID))
	}
	return nil
}

func (e *Engine) applyStep(ctx context.Context, entry *JournalEntry, prepared preparedProvider, index int, step adapter.PlanStep) error {
	rec := &entry.Steps[index]
	if rec.Done {
		return nil
	}
	if step.Path != "" {
		if err := e.validateLivePath(step.Path, isLocalStateStep(step.Kind)); err != nil {
			return err
		}
		before, err := InspectFile(step.Path, e.currentUID)
		if err != nil {
			return err
		}
		rec.Before = &before
		rec.Created = !before.Exists
		if before.Exists {
			if err := os.MkdirAll(rollbackDir(e.StateDir, entry.OperationID), dirPerm); err != nil {
				return err
			}
			if err := copyFile(step.Path, filepath.Join(rollbackDir(e.StateDir, entry.OperationID), rec.StepID), filePerm); err != nil {
				return err
			}
		}
	}
	rec.Started = true
	if err := writeJournal(e.StateDir, entry); err != nil {
		return err
	}
	if e.Hooks.AfterSnapshot != nil {
		if err := e.Hooks.AfterSnapshot(step); err != nil {
			return err
		}
	}
	if err := e.performStep(ctx, step, prepared.Payloads[step.StepID]); err != nil {
		rec.Error = err.Error()
		_ = writeJournal(e.StateDir, entry)
		return err
	}
	if step.Path != "" {
		after, err := InspectFile(step.Path, e.currentUID)
		if err != nil {
			return err
		}
		rec.After = &after
	}
	rec.Done = true
	if err := writeJournal(e.StateDir, entry); err != nil {
		return err
	}
	if e.Hooks.AfterStep != nil {
		return e.Hooks.AfterStep(step)
	}
	return nil
}

func (e *Engine) performStep(ctx context.Context, step adapter.PlanStep, payload []byte) error {
	insideWorkspace := step.Path != "" && isPathWithin(e.WorkspaceRoot, step.Path)
	if insideWorkspace && step.Writes() {
		locks := service.FileLockManager{Root: e.WorkspaceRoot, Wait: time.Millisecond}
		release, err := locks.Acquire(ctx, "setup-step")
		if err != nil {
			return apperr.Wrap(apperr.CodeBusy, err, "workspace lock busy during setup")
		}
		defer func() { _ = release() }()
	}
	switch step.Kind {
	case adapter.StepWriteManagedFile, adapter.StepUpdateManagedBlock, adapter.StepRecordLocalState:
		if len(payload) == 0 && step.Kind != adapter.StepRecordLocalState {
			return fmt.Errorf("step %s has no payload", step.StepID)
		}
		if step.Kind == adapter.StepRecordLocalState && len(payload) == 0 {
			payload = e.localStatePayload(step)
		}
		return atomicWriteFile(step.Path, payload, os.FileMode(step.Mode))
	case adapter.StepRemoveManagedFile, adapter.StepRemoveLocalState:
		if err := os.Remove(step.Path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	case adapter.StepRemoveManagedBlock:
		if payload == nil {
			return fmt.Errorf("step %s has no stripped payload", step.StepID)
		}
		return atomicWriteFile(step.Path, payload, 0o644)
	case adapter.StepRunClientCommand:
		if step.Command == nil {
			return fmt.Errorf("command step missing command")
		}
		_, err := e.runCommand(*step.Command)
		return err
	default:
		return fmt.Errorf("unsupported step kind %s", step.Kind)
	}
}

func (e *Engine) localStatePayload(step adapter.PlanStep) []byte {
	state := adapter.IntegrationState{
		Target:          integrations.Target(filepath.Base(filepath.Dir(step.Path))),
		ContractVersion: adapter.ContractVersion,
		State:           adapter.StateConfiguredUnverified,
		Scope:           adapter.ScopeProjectShared,
		WorkspaceID:     e.WorkspaceID,
		WorkspaceRoot:   e.WorkspaceRoot,
		UpdatedAt:       e.now(),
	}
	// Target is taken from the filename (codex.json) by the caller when it
	// supplies a payload. This fallback is only for crash-recovery tests.
	if base := filepath.Base(step.Path); len(base) > 5 {
		state.Target = integrations.Target(base[:len(base)-len(".json")])
	}
	raw, _ := json.MarshalIndent(state, "", "  ")
	return append(raw, '\n')
}

func (e *Engine) failToRollback(ctx context.Context, entry *JournalEntry, prepared preparedProvider, cause error) error {
	if errors.Is(cause, ErrInjectedCrash) {
		_ = writeJournal(e.StateDir, entry)
		return cause
	}
	if err := markState(entry, StateRollingBack); err != nil {
		return err
	}
	if err := writeJournal(e.StateDir, entry); err != nil {
		return err
	}
	if err := e.rollback(ctx, entry, prepared); err != nil {
		return err
	}
	return cause
}

func (e *Engine) rollback(ctx context.Context, entry *JournalEntry, prepared preparedProvider) error {
	left := []string{}
	for i := len(entry.Steps) - 1; i >= 0; i-- {
		rec := &entry.Steps[i]
		if !rec.Started || rec.RollbackDone {
			continue
		}
		var step adapter.PlanStep
		if i < len(prepared.Plan.Steps) {
			step = prepared.Plan.Steps[i]
		} else {
			step = adapter.PlanStep{StepID: rec.StepID, Kind: rec.Kind, Path: rec.Path, Rollback: rec.Rollback}
		}
		if e.Hooks.DuringRollback != nil {
			if err := e.Hooks.DuringRollback(step); err != nil {
				_ = writeJournal(e.StateDir, entry)
				return err
			}
		}
		if err := e.rollbackStep(entry.OperationID, rec); err != nil {
			rec.Error = err.Error()
			left = append(left, rec.Path)
			_ = writeJournal(e.StateDir, entry)
			continue
		}
		rec.RollbackDone = true
		if err := writeJournal(e.StateDir, entry); err != nil {
			return err
		}
	}
	entry.LeftBehind = left
	if len(left) > 0 {
		if err := markState(entry, StateFailed); err != nil {
			return err
		}
		return writeJournal(e.StateDir, entry)
	}
	if err := markState(entry, StateRolledBack); err != nil {
		return err
	}
	if err := writeJournal(e.StateDir, entry); err != nil {
		return err
	}
	_ = os.RemoveAll(rollbackDir(e.StateDir, entry.OperationID))
	return nil
}

func (e *Engine) rollbackStep(operationID string, rec *journalStepRecord) error {
	if rec.Rollback == nil {
		return nil
	}
	switch rec.Rollback.Kind {
	case adapter.RollbackRestoreSnapshot:
		current, err := InspectFile(rec.Path, e.currentUID)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if rec.Before != nil && identitiesEqual(current, *rec.Before) {
			return nil
		}
		if rec.After != nil && current.Exists && !identitiesEqual(current, *rec.After) {
			return fmt.Errorf("file %s changed after Atlas wrote it; not overwriting", rec.Path)
		}
		if rec.Before == nil || !rec.Before.Exists {
			if err := os.Remove(rec.Path); err != nil && !os.IsNotExist(err) {
				return err
			}
			return nil
		}
		return copyFile(filepath.Join(rollbackDir(e.StateDir, operationID), rec.StepID), rec.Path, os.FileMode(rec.Before.Mode))
	case adapter.RollbackDeleteCreated:
		_, err := os.Lstat(rec.Path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		current, err := InspectFile(rec.Path, e.currentUID)
		if err != nil {
			return err
		}
		if rec.After != nil && !identitiesEqual(current, *rec.After) {
			return fmt.Errorf("file %s is not the Atlas-created identity; not deleting", rec.Path)
		}
		return os.Remove(rec.Path)
	case adapter.RollbackRunCommand:
		if rec.Rollback.Command == nil {
			return fmt.Errorf("run_command rollback missing command")
		}
		if rec.Rollback.Command.Purpose == adapter.CommandPurposeRemove {
			absent, err := e.probeAbsent(*rec.Rollback.Command)
			if err != nil {
				return err
			}
			if absent {
				return nil
			}
		}
		_, err := e.runCommand(*rec.Rollback.Command)
		return err
	default:
		return fmt.Errorf("unknown rollback kind %s", rec.Rollback.Kind)
	}
}

func (e *Engine) probeAbsent(cmd adapter.Command) (bool, error) {
	if e.ProbeAbsent != nil {
		return e.ProbeAbsent(cmd)
	}
	return false, nil
}

func (e *Engine) runCommand(cmd adapter.Command) (int, error) {
	if e.Runner != nil {
		return e.Runner(cmd)
	}
	ctx := context.Background()
	if cmd.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cmd.Timeout)
		defer cancel()
	}
	execCmd := exec.CommandContext(ctx, cmd.Executable, cmd.Args...)
	if cmd.Dir != "" {
		execCmd.Dir = cmd.Dir
	}
	err := execCmd.Run()
	if err == nil {
		return 0, nil
	}
	if exit, ok := err.(*exec.ExitError); ok {
		return exit.ExitCode(), err
	}
	return -1, err
}

func (e *Engine) validateLivePath(path string, localState bool) error {
	if err := refuseSymlinkPath(path); err != nil {
		return err
	}
	parent := filepath.Dir(path)
	info, err := os.Lstat(parent)
	if os.IsNotExist(err) {
		if localState || isPathWithin(e.WorkspaceRoot, path) {
			return os.MkdirAll(parent, 0o755)
		}
		return fmt.Errorf("parent of %s does not exist", path)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("refusing symlinked parent %s", parent))
	}
	return nil
}

func refuseSymlinkPath(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("refusing symlinked path %s", path))
	}
	return nil
}

func isLocalStateStep(kind adapter.StepKind) bool {
	return kind == adapter.StepRecordLocalState || kind == adapter.StepRemoveLocalState
}

func isRemovalStep(kind adapter.StepKind) bool {
	switch kind {
	case adapter.StepRemoveManagedFile, adapter.StepRemoveManagedBlock, adapter.StepRemoveConfigEntry, adapter.StepRemoveLocalState:
		return true
	default:
		return false
	}
}

func isPathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	cleaned := filepath.Clean(rel)
	if cleaned == "." || cleaned == ".." {
		return false
	}
	return !hasDotDot(cleaned)
}

func hasDotDot(rel string) bool {
	return rel == ".." || len(rel) >= 3 && rel[:3] == ".."+string(os.PathSeparator)
}

func (e *Engine) verifyPrepared(prepared preparedProvider) (adapter.State, error) {
	if prepared.Plan.Operation == adapter.PlanOperationRemove {
		for _, step := range prepared.Plan.Steps {
			if step.Path == "" {
				continue
			}
			if step.Kind == adapter.StepRemoveManagedFile || step.Kind == adapter.StepRemoveLocalState {
				if _, err := os.Lstat(step.Path); err == nil {
					return adapter.StateRepairRequired, fmt.Errorf("file %s still exists after removal", step.Path)
				} else if !os.IsNotExist(err) {
					return adapter.StateRepairRequired, err
				}
			}
		}
		return adapter.StateConnected, nil
	}
	if e.Registry != nil {
		if item, ok := e.Registry.Lookup(prepared.Target); ok {
			state := adapter.IntegrationState{
				Target:          prepared.Target,
				ContractVersion: adapter.ContractVersion,
				State:           prepared.ResultingState,
				Scope:           prepared.Plan.Scope,
				WorkspaceID:     e.WorkspaceID,
				WorkspaceRoot:   e.WorkspaceRoot,
				UpdatedAt:       e.now(),
			}
			verification, err := item.Verify(context.Background(), state)
			if err != nil {
				return adapter.StateFailed, err
			}
			return verification.State, nil
		}
	}
	for _, step := range prepared.Plan.Steps {
		if step.Path == "" || !step.Writes() || isRemovalStep(step.Kind) {
			continue
		}
		payload := prepared.Payloads[step.StepID]
		if len(payload) == 0 {
			continue
		}
		current, err := os.ReadFile(step.Path)
		if err != nil {
			return adapter.StateRepairRequired, err
		}
		if string(current) != string(payload) && step.Kind != adapter.StepRecordLocalState {
			return adapter.StateRepairRequired, fmt.Errorf("file %s does not match the applied payload", step.Path)
		}
	}
	return prepared.ResultingState, nil
}

func (e *Engine) commitProvider(entry *JournalEntry, prepared preparedProvider, state adapter.State) error {
	manifest, err := loadManifest(e.StateDir, e.WorkspaceID)
	if err != nil {
		return err
	}
	if manifest == nil {
		manifest = emptyManifest(e.WorkspaceID, e.WorkspaceRoot, trackerIdentity(e.TrackerPath, e.TrackerVersion))
	}
	record := &adapter.IntegrationState{
		Target:          prepared.Target,
		ContractVersion: adapter.ContractVersion,
		State:           state,
		Scope:           prepared.Plan.Scope,
		WorkspaceID:     e.WorkspaceID,
		WorkspaceRoot:   e.WorkspaceRoot,
		UpdatedAt:       e.now(),
	}
	if payload, ok := prepared.Payloads["state-"+string(prepared.Target)]; ok {
		var fromDisk adapter.IntegrationState
		if err := json.Unmarshal(payload, &fromDisk); err == nil {
			record.SkillVersion = fromDisk.SkillVersion
			record.ManagedBlockVersion = fromDisk.ManagedBlockVersion
		}
	}
	manifest.putIntegration(prepared.Target, ManifestIntegration{
		State:            state,
		AdapterVersion:   adapter.ContractVersion,
		SkillVersion:     record.SkillVersion,
		ManagedBlock:     record.ManagedBlockVersion,
		WorkspaceBinding: e.WorkspaceID,
		Record:           record,
	})
	if err := saveManifest(e.StateDir, manifest); err != nil {
		return err
	}
	e.rememberWorkspace()
	_ = entry
	return nil
}

func (e *Engine) rememberWorkspace() {
	reg, err := loadRegistry(e.StateDir)
	if err != nil {
		return
	}
	dev, ino, _ := fileDevIno(e.WorkspaceRoot)
	reg.put(RegistryEntry{
		WorkspaceID:   e.WorkspaceID,
		CanonicalPath: e.WorkspaceRoot,
		Dev:           dev,
		Ino:           ino,
		RegisteredAt:  e.now(),
		VerifiedAt:    e.now(),
	})
	_ = saveRegistry(e.StateDir, reg)
}
