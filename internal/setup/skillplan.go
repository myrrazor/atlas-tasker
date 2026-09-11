package setup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter/all"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter/host"
)

const remainingAdapterDependency = "AT114-201..AT114-208: provider MCP adapters are not registered"

func (e *Engine) ensureRegistry() error {
	if e.Registry != nil {
		return nil
	}
	reg, err := all.New(all.Options{StateDir: e.StateDir})
	if err != nil {
		return err
	}
	e.Registry = reg
	return nil
}

func (e *Engine) planTarget(target integrations.Target, found integrations.Detection, existing *adapter.IntegrationState, hint contracts.Actor, now time.Time) (preparedProvider, error) {
	if err := e.ensureRegistry(); err != nil {
		return preparedProvider{}, err
	}
	item, ok := e.Registry.Lookup(target)
	if !ok {
		return planSkillOnly(e.WorkspaceRoot, e.WorkspaceID, e.Home, e.StateDir, e.TrackerPath, target, found, existing, now, e.lookPath())
	}
	detectIn := adapter.DetectInput{
		WorkspaceRoot: e.WorkspaceRoot,
		Home:          e.Home,
		LookPath:      e.lookPath(),
		Getenv:        e.getenv(),
		Runner:        host.DefaultRunner{},
	}
	detection := item.Detect(context.Background(), detectIn)
	if detection.Target == "" {
		detection = toAdapterDetection(target, found, e.lookPath())
	}
	input := adapter.PlanInput{
		WorkspaceRoot: e.WorkspaceRoot,
		WorkspaceID:   e.WorkspaceID,
		Home:          e.Home,
		TrackerPath:   e.TrackerPath,
		ActorHint:     hint,
		Detection:     detection,
		Existing:      existing,
	}
	if prep, ok := item.(interface {
		Prepare(context.Context, adapter.PlanInput) (host.Prepared, error)
	}); ok {
		prepared, err := prep.Prepare(context.Background(), input)
		if err != nil {
			return preparedProvider{}, err
		}
		caps, _ := adapter.CapabilitiesFor(target)
		return preparedProvider{
			Target:         target,
			MachineWide:    caps.PreferredScope == adapter.ScopeGateway || caps.PreferredScope == adapter.ScopeUser,
			Plan:           prepared.Plan,
			Payloads:       prepared.Payloads,
			ResultingState: prepared.Plan.ResultingState,
		}, nil
	}
	plan, err := item.Plan(context.Background(), input)
	if err != nil {
		return preparedProvider{}, err
	}
	payloads := map[string][]byte{}
	if src, ok := item.(interface {
		LastPayloads() map[string][]byte
	}); ok {
		payloads = src.LastPayloads()
	}
	caps, _ := adapter.CapabilitiesFor(target)
	return preparedProvider{
		Target:         target,
		MachineWide:    caps.PreferredScope == adapter.ScopeGateway || caps.PreferredScope == adapter.ScopeUser,
		Plan:           plan,
		Payloads:       payloads,
		ResultingState: plan.ResultingState,
	}, nil
}

type preparedProvider struct {
	Target              integrations.Target
	Selected            bool
	MachineWide         bool
	Plan                adapter.IntegrationPlan
	Payloads            map[string][]byte
	AdapterMissing      bool
	RemainingDependency string
	ResultingState      adapter.State
}

func planSkillOnly(workspaceRoot, workspaceID, home, stateDir, trackerPath string, target integrations.Target, detection integrations.Detection, existing *adapter.IntegrationState, now time.Time, lookPath func(string) (string, error)) (preparedProvider, error) {
	caps, err := adapter.CapabilitiesFor(target)
	if err != nil {
		return preparedProvider{}, err
	}
	installer := integrations.Installer{Root: workspaceRoot}
	files, err := installer.Preview(target)
	if err != nil {
		return preparedProvider{}, err
	}
	adapterDetection := toAdapterDetection(target, detection, lookPath)
	scope := caps.PreferredScope
	machineWide := scope == adapter.ScopeGateway || scope == adapter.ScopeUser
	resulting := adapter.StateConfiguredUnverified
	if target == integrations.TargetGeneric {
		resulting = adapter.StatePortableReady
	}
	if adapterDetection.Installed && adapterDetection.VersionSupport != adapter.VersionSupported && target != integrations.TargetGeneric {
		resulting = adapter.StateUnsupportedClientVersion
	}
	localStateRoot := filepath.Clean(stateDir)
	statePath := filepath.Join(localStateRoot, "workspaces", workspaceID, string(target)+".json")
	steps := make([]adapter.PlanStep, 0, len(files)+1)
	payloads := map[string][]byte{}
	writes := 0
	var skillVersion, blockVersion string
	for _, file := range files {
		if file.Kind == "skill" && filepath.Base(file.Path) == "SKILL.md" && skillVersion == "" {
			skillVersion = shortHash([]byte(file.Body))
		}
		if file.Kind == "instruction" {
			blockVersion = shortHash([]byte(file.Body))
		}
		if file.Change == integrations.InstallChangeNone {
			continue
		}
		kind := adapter.StepWriteManagedFile
		if file.Kind == "instruction" && file.Change == integrations.InstallChangeUpdate {
			kind = adapter.StepUpdateManagedBlock
		}
		stepID := "file-" + shortHash([]byte(file.Path))
		rollback := &adapter.RollbackAction{Kind: adapter.RollbackRestoreSnapshot, Path: file.Path}
		if file.Change == integrations.InstallChangeCreate {
			rollback = &adapter.RollbackAction{Kind: adapter.RollbackDeleteCreated, Path: file.Path}
		}
		steps = append(steps, adapter.PlanStep{
			StepID:        stepID,
			Kind:          kind,
			Description:   "install Atlas-owned " + file.Kind + " for " + string(target),
			Path:          file.Path,
			Reversibility: adapter.Reversible,
			Rollback:      rollback,
			Mode:          0o644,
		})
		payloads[stepID] = []byte(file.Body)
		writes++
	}
	needState := existing == nil || existing.SkillVersion != skillVersion || existing.ManagedBlockVersion != blockVersion || existing.State != resulting
	if writes > 0 || needState {
		stepID := "state-" + string(target)
		created := true
		if _, err := os.Lstat(statePath); err == nil {
			created = false
		}
		rollback := &adapter.RollbackAction{Kind: adapter.RollbackRestoreSnapshot, Path: statePath}
		if created {
			rollback = &adapter.RollbackAction{Kind: adapter.RollbackDeleteCreated, Path: statePath}
		}
		steps = append(steps, adapter.PlanStep{
			StepID:        stepID,
			Kind:          adapter.StepRecordLocalState,
			Description:   "record local integration state for " + string(target),
			Path:          statePath,
			Reversibility: adapter.Reversible,
			Rollback:      rollback,
			Mode:          0o600,
		})
		record := adapter.IntegrationState{
			Target:              target,
			ContractVersion:     adapter.ContractVersion,
			State:               resulting,
			Scope:               scope,
			WorkspaceID:         workspaceID,
			WorkspaceRoot:       workspaceRoot,
			SkillVersion:        skillVersion,
			ManagedBlockVersion: blockVersion,
			UpdatedAt:           now,
		}
		raw, err := json.MarshalIndent(record, "", "  ")
		if err != nil {
			return preparedProvider{}, err
		}
		payloads[stepID] = append(raw, '\n')
		writes++
	}
	noOp := writes == 0
	plan := adapter.IntegrationPlan{
		PlanID:          "setup-" + string(target),
		ContractVersion: adapter.ContractVersion,
		Operation:       adapter.PlanOperationSetup,
		Target:          target,
		WorkspaceID:     workspaceID,
		WorkspaceRoot:   workspaceRoot,
		Home:            home,
		LocalStateRoot:  localStateRoot,
		Scope:           scope,
		Detection:       adapterDetection,
		Steps:           steps,
		ResultingState:  resulting,
		GeneratedAt:     now,
		NoOp:            noOp,
		Warnings:        []string{remainingAdapterDependency},
	}
	if err := plan.Validate(); err != nil {
		return preparedProvider{}, fmt.Errorf("skill-only plan for %s: %w", target, err)
	}
	return preparedProvider{
		Target:              target,
		MachineWide:         machineWide,
		Plan:                plan,
		Payloads:            payloads,
		AdapterMissing:      true,
		RemainingDependency: remainingAdapterDependency,
		ResultingState:      resulting,
	}, nil
}

func (e *Engine) planDisconnect(target integrations.Target, found integrations.Detection, existing *adapter.IntegrationState, confirmDrift bool) (preparedProvider, error) {
	if err := e.ensureRegistry(); err != nil {
		return preparedProvider{}, err
	}
	if item, ok := e.Registry.Lookup(target); ok && existing != nil {
		if existing.WorkspaceRoot == "" {
			existing.WorkspaceRoot = e.WorkspaceRoot
		}
		if existing.WorkspaceID == "" {
			existing.WorkspaceID = e.WorkspaceID
		}
		removal, err := item.Remove(context.Background(), *existing)
		if err == nil {
			payloads := map[string][]byte{}
			if src, ok := item.(interface{ LastPayloads() map[string][]byte }); ok {
				payloads = src.LastPayloads()
			}
			if !removal.OwnershipVerified && !confirmDrift {
				return preparedProvider{}, fmt.Errorf("ambiguous ownership: %s removal requires confirmation", target)
			}
			return preparedProvider{Target: target, Plan: removal.Plan, Payloads: payloads}, nil
		}
	}
	return planSkillRemoval(e.WorkspaceRoot, e.WorkspaceID, e.Home, e.StateDir, target, found, existing, e.now(), e.lookPath(), confirmDrift)
}

func planSkillRemoval(workspaceRoot, workspaceID, home, stateDir string, target integrations.Target, detection integrations.Detection, existing *adapter.IntegrationState, now time.Time, lookPath func(string) (string, error), confirmDrift bool) (preparedProvider, error) {
	caps, err := adapter.CapabilitiesFor(target)
	if err != nil {
		return preparedProvider{}, err
	}
	installer := integrations.Installer{Root: workspaceRoot}
	files, err := installer.Preview(target)
	if err != nil {
		return preparedProvider{}, err
	}
	adapterDetection := toAdapterDetection(target, detection, lookPath)
	localStateRoot := filepath.Clean(stateDir)
	statePath := filepath.Join(localStateRoot, "workspaces", workspaceID, string(target)+".json")
	steps := []adapter.PlanStep{}
	payloads := map[string][]byte{}
	for _, file := range files {
		info, err := os.Lstat(file.Path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return preparedProvider{}, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return preparedProvider{}, fmt.Errorf("ambiguous ownership: %s is a symlink", file.Path)
		}
		current, err := os.ReadFile(file.Path)
		if err != nil {
			return preparedProvider{}, err
		}
		if file.Kind == "instruction" {
			begin, end, err := integrations.InstructionMarkers(target)
			if err != nil {
				return preparedProvider{}, err
			}
			stripped, changed := stripManagedBlock(string(current), begin, end)
			if !changed {
				continue
			}
			stepID := "remove-block-" + shortHash([]byte(file.Path))
			steps = append(steps, adapter.PlanStep{
				StepID:        stepID,
				Kind:          adapter.StepRemoveManagedBlock,
				Description:   "remove Atlas managed instruction block for " + string(target),
				Path:          file.Path,
				Reversibility: adapter.Reversible,
				Rollback:      &adapter.RollbackAction{Kind: adapter.RollbackRestoreSnapshot, Path: file.Path},
			})
			payloads[stepID] = []byte(stripped)
			continue
		}
		if string(current) != file.Body && !confirmDrift {
			return preparedProvider{}, fmt.Errorf("ambiguous ownership: %s was changed after Atlas wrote it; removal requires confirmation", file.Path)
		}
		stepID := "remove-" + shortHash([]byte(file.Path))
		steps = append(steps, adapter.PlanStep{
			StepID:        stepID,
			Kind:          adapter.StepRemoveManagedFile,
			Description:   "remove Atlas-owned " + file.Kind + " for " + string(target),
			Path:          file.Path,
			Reversibility: adapter.Reversible,
			Rollback:      &adapter.RollbackAction{Kind: adapter.RollbackRestoreSnapshot, Path: file.Path},
		})
	}
	if _, err := os.Lstat(statePath); err == nil {
		steps = append(steps, adapter.PlanStep{
			StepID:        "remove-state-" + string(target),
			Kind:          adapter.StepRemoveLocalState,
			Description:   "remove local integration state for " + string(target),
			Path:          statePath,
			Reversibility: adapter.Reversible,
			Rollback:      &adapter.RollbackAction{Kind: adapter.RollbackRestoreSnapshot, Path: statePath},
		})
	}
	plan := adapter.IntegrationPlan{
		PlanID:          "remove-" + string(target),
		ContractVersion: adapter.ContractVersion,
		Operation:       adapter.PlanOperationRemove,
		Target:          target,
		WorkspaceID:     workspaceID,
		WorkspaceRoot:   workspaceRoot,
		Home:            home,
		LocalStateRoot:  localStateRoot,
		Scope:           caps.PreferredScope,
		Detection:       adapterDetection,
		Steps:           steps,
		GeneratedAt:     now,
		NoOp:            len(steps) == 0,
	}
	if err := plan.Validate(); err != nil {
		return preparedProvider{}, fmt.Errorf("removal plan for %s: %w", target, err)
	}
	_ = existing
	return preparedProvider{Target: target, Plan: plan, Payloads: payloads, AdapterMissing: true}, nil
}

func toAdapterDetection(target integrations.Target, found integrations.Detection, lookPath func(string) (string, error)) adapter.Detection {
	detection := adapter.Detection{
		Target:         target,
		VersionSupport: adapter.VersionUnknown,
		Reasons:        found.Reasons,
	}
	name := clientExecutableName(target)
	if name != "" && lookPath != nil {
		if exe, err := lookPath(name); err == nil && exe != "" {
			if abs, err := filepath.Abs(exe); err == nil {
				cleaned := filepath.Clean(abs)
				if filepath.IsAbs(cleaned) {
					detection.Installed = true
					detection.ExecutablePath = cleaned
				}
			}
		}
	}
	return detection
}

func clientExecutableName(target integrations.Target) string {
	switch target {
	case integrations.TargetCodex:
		return "codex"
	case integrations.TargetClaude:
		return "claude"
	case integrations.TargetCursor:
		return "cursor"
	case integrations.TargetOpenClaw:
		return "openclaw"
	case integrations.TargetGrok:
		return "grok"
	default:
		return ""
	}
}

func shortHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:12]
}

func stripManagedBlock(body, begin, end string) (string, bool) {
	start := strings.Index(body, begin)
	stop := strings.Index(body, end)
	if start < 0 || stop < 0 || stop < start {
		return body, false
	}
	stop += len(end)
	if stop < len(body) && body[stop] == '\n' {
		stop++
	}
	updated := body[:start] + body[stop:]
	return updated, updated != body
}
