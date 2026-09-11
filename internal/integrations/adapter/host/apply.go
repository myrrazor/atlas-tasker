package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
	"github.com/myrrazor/atlas-tasker/internal/mcp/selfprobe"
	"github.com/myrrazor/atlas-tasker/internal/mcp/testhost"
)

func (a *Adapter) FilePayloads(plan adapter.IntegrationPlan) (map[string][]byte, error) {
	input := adapter.PlanInput{
		WorkspaceRoot:  plan.WorkspaceRoot,
		WorkspaceID:    plan.WorkspaceID,
		Home:           plan.Home,
		TrackerPath:    "/usr/local/bin/tracker",
		Detection:      plan.Detection,
		Scope:          plan.Scope,
		ConsentedRoots: plan.ConsentedRoots,
	}
	if plan.Registration != nil {
		input.TrackerPath = plan.Registration.Command
		if plan.Registration.Portable {
			input.TrackerPath = "/usr/local/bin/tracker"
		}
		input.ActorHint = plan.Registration.ActorHint
	}
	if a.stateDir == "" && plan.LocalStateRoot != "" {
		a.stateDir = plan.LocalStateRoot
	}
	prepared, err := a.Prepare(context.Background(), input)
	if err != nil {
		return nil, err
	}
	return prepared.Payloads, nil
}

func (a *Adapter) Apply(ctx context.Context, plan adapter.IntegrationPlan) (adapter.ApplyResult, error) {
	if err := plan.Validate(); err != nil {
		return adapter.ApplyResult{}, err
	}
	result := adapter.ApplyResult{PlanID: plan.PlanID, Target: plan.Target, State: plan.ResultingState}
	if result.State == "" && plan.Operation == adapter.PlanOperationRemove {
		result.State = adapter.StateConfiguredUnverified
	}
	if result.State.Verified() {
		result.State = adapter.StateConfiguredUnverified
		if plan.Target == integrations.TargetGeneric {
			result.State = adapter.StatePortableReady
		}
	}
	if plan.NoOp {
		result.Record = &adapter.IntegrationState{
			Target: plan.Target, ContractVersion: adapter.ContractVersion, State: result.State,
			Scope: plan.Scope, WorkspaceID: plan.WorkspaceID, WorkspaceRoot: plan.WorkspaceRoot,
			Registration: plan.Registration, UpdatedAt: time.Now().UTC(),
		}
		if plan.Registration != nil {
			result.Record.EntryFingerprint = plan.Registration.Fingerprint()
		}
		return result, result.Validate()
	}
	payloads := a.lastPayloads
	if payloads == nil || missingPayloads(plan, payloads) {
		if plan.Operation != adapter.PlanOperationSetup {
			result.State = adapter.StateFailed
			result.Error = "missing payloads for " + string(plan.Operation)
			return result, fmt.Errorf("%s", result.Error)
		}
		regenerated, err := a.FilePayloads(plan)
		if err != nil {
			result.State = adapter.StateFailed
			result.Error = err.Error()
			return result, nil
		}
		payloads = regenerated
	}
	for _, step := range plan.Steps {
		if !step.Writes() {
			result.Outcomes = append(result.Outcomes, adapter.StepOutcome{StepID: step.StepID, Status: adapter.StepSkipped})
			continue
		}
		if err := applyStep(step, payloads[step.StepID], a.runner); err != nil {
			result.State = adapter.StateFailed
			result.Error = err.Error()
			result.Record = nil
			result.Outcomes = append(result.Outcomes, adapter.StepOutcome{StepID: step.StepID, Status: adapter.StepFailed, Error: err.Error()})
			return result, result.Validate()
		}
		result.Outcomes = append(result.Outcomes, adapter.StepOutcome{StepID: step.StepID, Status: adapter.StepApplied})
	}
	record := adapter.IntegrationState{
		Target: plan.Target, ContractVersion: adapter.ContractVersion, State: result.State,
		Scope: plan.Scope, WorkspaceID: plan.WorkspaceID, WorkspaceRoot: plan.WorkspaceRoot,
		Registration: plan.Registration, ClientExecutable: plan.Detection.ExecutablePath,
		UpdatedAt: time.Now().UTC(),
	}
	if plan.Registration != nil {
		record.EntryFingerprint = plan.Registration.Fingerprint()
	}
	result.Record = &record
	return result, result.Validate()
}

func applyStep(step adapter.PlanStep, payload []byte, runner adapter.CommandRunner) error {
	switch step.Kind {
	case adapter.StepWriteManagedFile, adapter.StepUpdateManagedBlock, adapter.StepWriteConfigEntry, adapter.StepRecordLocalState:
		if len(payload) == 0 && step.Kind != adapter.StepRecordLocalState {
			return fmt.Errorf("step %s has no payload", step.StepID)
		}
		if err := os.MkdirAll(filepath.Dir(step.Path), 0o755); err != nil {
			return err
		}
		mode := os.FileMode(step.Mode)
		if mode == 0 {
			mode = 0o644
		}
		return os.WriteFile(step.Path, payload, mode)
	case adapter.StepRemoveManagedBlock:
		if err := os.MkdirAll(filepath.Dir(step.Path), 0o755); err != nil {
			return err
		}
		return os.WriteFile(step.Path, payload, 0o644)
	case adapter.StepRemoveManagedFile, adapter.StepRemoveLocalState, adapter.StepRemoveConfigEntry:
		if len(payload) > 0 {
			return os.WriteFile(step.Path, payload, 0o644)
		}
		if err := os.Remove(step.Path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	case adapter.StepRunClientCommand:
		if step.Command == nil {
			return fmt.Errorf("missing command")
		}
		if runner == nil {
			runner = DefaultRunner{}
		}
		_, err := runner.Run(context.Background(), *step.Command)
		return err
	default:
		return fmt.Errorf("unsupported step %s", step.Kind)
	}
}

func (a *Adapter) LastPayloads() map[string][]byte { return a.lastPayloads }

func (a *Adapter) Verify(ctx context.Context, state adapter.IntegrationState) (adapter.Verification, error) {
	now := time.Now().UTC()
	if a.now != nil {
		now = a.now()
	}
	v := adapter.Verification{Target: a.target, State: state.State, CheckedAt: now, ClientExecutable: state.ClientExecutable}
	if state.State == "" {
		v.State = adapter.StateConfiguredUnverified
	}
	if a.target == integrations.TargetGeneric && !v.State.Verified() {
		v.State = adapter.StatePortableReady
	}

	selfOK := false
	nativeOK := false
	doctorOK := false
	conformanceOK := false
	if state.Registration != nil {
		exe := lookupTracker(state)
		if exe != "" && isAbs(exe) {
			v.ServerExecutable = exe
			cmd, err := state.Registration.ServeCommand(exe, state.WorkspaceRoot, 20*time.Second)
			if err == nil {
				report, err := selfprobe.Run(ctx, selfprobe.Options{Command: cmd})
				v.Probes = append(v.Probes, adapter.ProbeRecord{Command: cmd, Summary: report.Detail})
				selfOK = err == nil && report.Passed()
				v.Checks = append(v.Checks, adapter.VerificationCheck{Name: "self_probe", Method: adapter.VerificationSelfProbe, Passed: selfOK, Detail: report.Detail})
			} else {
				v.Checks = append(v.Checks, adapter.VerificationCheck{Name: "self_probe", Method: adapter.VerificationSelfProbe, Passed: false, Detail: err.Error()})
			}
		}
		if state.ClientExecutable != "" && isAbs(state.ClientExecutable) {
			v.ClientExecutable = state.ClientExecutable
			nativeOK, doctorOK = a.runClientNativeChecks(ctx, &v, state)
		}
		if a.target == integrations.TargetGeneric {
			conformanceOK = a.runGenericConformance(ctx, &v, state, exe)
		}
	}

	pending := state.State == adapter.StatePendingWorkspaceTrust || state.State == adapter.StatePendingMCPApproval
	switch {
	case pending:
		v.State = state.State
		v.ConnectionKind = ""
	case a.target == integrations.TargetGeneric && conformanceOK:
		v.State = adapter.StateConnected
		v.ConnectionKind = adapter.ConnectionKindStandardConfig
	case a.target == integrations.TargetGeneric:
		v.State = adapter.StatePortableReady
		v.ConnectionKind = ""
	case a.target == integrations.TargetOpenClaw && doctorOK:
		v.State = adapter.StateConnected
		v.ConnectionKind = adapter.ConnectionKindClientNative
	case a.target == integrations.TargetOpenClaw && selfOK:
		v.State = adapter.StateConnectedRestartRequired
		v.ConnectionKind = adapter.ConnectionKindSelfProbe
	case nativeOK:
		v.State = adapter.StateConnected
		v.ConnectionKind = adapter.ConnectionKindClientNative
	case selfOK && a.target == integrations.TargetCursor:
		v.State = adapter.StateConnectedRestartRequired
		v.ConnectionKind = adapter.ConnectionKindSelfProbe
	case selfOK:
		v.State = adapter.StateConnected
		v.ConnectionKind = adapter.ConnectionKindSelfProbe
	default:
		if v.State.Verified() {
			v.State = adapter.StateConfiguredUnverified
		}
		if a.target == integrations.TargetGeneric {
			v.State = adapter.StatePortableReady
		}
		v.ConnectionKind = ""
	}

	if err := v.Validate(); err != nil {
		v.State = adapter.StateConfiguredUnverified
		if a.target == integrations.TargetGeneric {
			v.State = adapter.StatePortableReady
		}
		v.ConnectionKind = ""
		if err := v.Validate(); err != nil {
			return v, err
		}
	}
	return v, nil
}

func (a *Adapter) runClientNativeChecks(ctx context.Context, v *adapter.Verification, state adapter.IntegrationState) (nativeOK, doctorOK bool) {
	runner := a.runner
	if runner == nil {
		runner = DefaultRunner{}
	}
	name := ""
	if state.Registration != nil {
		name = state.Registration.ServerName
	}
	for _, cmd := range clientInspectCommands(a.target, state.ClientExecutable, name, state.WorkspaceRoot) {
		if err := cmd.Validate(); err != nil {
			continue
		}
		result, err := runner.Run(ctx, cmd)
		detail := strings.TrimSpace(string(result.Stdout) + " " + string(result.Stderr))
		if err != nil && detail == "" {
			detail = err.Error()
		}
		exitedOK := err == nil && result.ExitCode == 0 && !result.TimedOut
		method := methodForInspect(cmd)
		mentioned := mentionsServer(detail, name)
		checkPassed := exitedOK && mentioned
		v.Checks = append(v.Checks, adapter.VerificationCheck{Name: string(method), Method: method, Passed: checkPassed, Detail: truncateDetail(detail)})
		v.Probes = append(v.Probes, adapter.ProbeRecord{Command: cmd, ExitCode: result.ExitCode, TimedOut: result.TimedOut, Summary: truncateDetail(detail)})
		proves, isDoctor := nativeProof(a.target, method, exitedOK, mentioned, detail)
		if proves {
			nativeOK = true
		}
		if isDoctor {
			doctorOK = true
		}
	}
	return nativeOK, doctorOK
}

func (a *Adapter) runGenericConformance(ctx context.Context, v *adapter.Verification, state adapter.IntegrationState, exe string) bool {
	desc := state.ConfigPath
	if desc == "" {
		desc = filepath.Join(state.WorkspaceRoot, ".tracker", "integrations", "atlas-mcp.json")
	}
	if _, err := testhost.Load(desc); err != nil {
		v.Checks = append(v.Checks, adapter.VerificationCheck{Name: "portable_descriptor", Method: adapter.VerificationConformanceHost, Passed: false, Detail: err.Error()})
		return false
	}
	if exe == "" || !isAbs(exe) {
		// Discovery only. A loadable file is not a live connection.
		return false
	}
	name := ""
	if state.Registration != nil {
		name = state.Registration.ServerName
	}
	report, err := testhost.Probe(ctx, testhost.ProbeOptions{
		DescriptorPath: desc,
		ServerName:     name,
		Executable:     exe,
		WorkspaceRoot:  state.WorkspaceRoot,
	})
	passed := err == nil && report.Passed()
	detail := report.Detail
	if err != nil && detail == "" {
		detail = err.Error()
	}
	v.Checks = append(v.Checks, adapter.VerificationCheck{Name: "conformance_host", Method: adapter.VerificationConformanceHost, Passed: passed, Detail: detail})
	return passed
}

func truncateDetail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 400 {
		return s
	}
	return s[:400]
}

func missingPayloads(plan adapter.IntegrationPlan, payloads map[string][]byte) bool {
	for _, step := range plan.Steps {
		if !step.Writes() {
			continue
		}
		switch step.Kind {
		case adapter.StepRunClientCommand, adapter.StepRemoveManagedFile, adapter.StepRemoveLocalState, adapter.StepRemoveManagedBlock:
			continue
		}
		if len(payloads[step.StepID]) == 0 {
			return true
		}
	}
	return false
}

func lookupTracker(state adapter.IntegrationState) string {
	if state.Registration != nil && isAbs(state.Registration.Command) {
		return state.Registration.Command
	}
	return ""
}

func isAbs(path string) bool {
	return len(path) > 0 && path[0] == '/'
}

func (a *Adapter) Repair(ctx context.Context, state adapter.IntegrationState) (adapter.RepairPlan, error) {
	input := adapter.PlanInput{
		WorkspaceRoot: state.WorkspaceRoot,
		WorkspaceID:   state.WorkspaceID,
		Home:          a.effectiveHome(),
		TrackerPath:   firstAbs(state),
		ActorHint:     state.ActorHint,
		Detection: adapter.Detection{
			Target: a.target, Installed: state.ClientExecutable != "",
			ExecutablePath: state.ClientExecutable, Version: state.ClientVersion,
			VersionSupport: adapter.VersionUnknown,
		},
		Scope:    state.Scope,
		Existing: &state,
	}
	if input.Detection.Installed && input.Detection.Version.Known {
		input.Detection.VersionSupport = adapter.VersionSupported
	}
	if input.TrackerPath == "" {
		input.TrackerPath = "/usr/local/bin/tracker"
	}
	if a.stateDir == "" {
		return adapter.RepairPlan{}, fmt.Errorf("state dir is required for repair")
	}
	prepared, err := a.Prepare(ctx, input)
	if err != nil {
		return adapter.RepairPlan{}, err
	}
	prepared.Plan.Operation = adapter.PlanOperationRepair
	prepared.Plan.PlanID = "repair-" + string(a.target)
	if err := prepared.Plan.Validate(); err != nil {
		return adapter.RepairPlan{}, err
	}
	reason := state.RepairReason
	if reason == "" {
		reason = "refresh Atlas-owned skill and MCP registration"
	}
	return adapter.RepairPlan{Reason: reason, Plan: prepared.Plan}, nil
}

func (a *Adapter) Remove(ctx context.Context, state adapter.IntegrationState) (adapter.RemovalPlan, error) {
	caps := a.Capabilities()
	detection := adapter.Detection{Target: a.target, VersionSupport: adapter.VersionUnknown}
	if state.ClientExecutable != "" {
		detection.Installed = true
		detection.ExecutablePath = state.ClientExecutable
		detection.Version = state.ClientVersion
		if !detection.Version.Known {
			detection.Version = adapter.ClientVersion{Raw: "recorded", Known: true}
		}
		detection.VersionSupport = adapter.VersionSupported
	}
	steps := []adapter.PlanStep{}
	payloads := map[string][]byte{}
	installer := installerPreview(state.WorkspaceRoot, a.target)
	ownership := true
	for _, file := range installer {
		if _, err := os.Lstat(file.Path); err != nil {
			continue
		}
		if IsSymlinkPath(file.Path, os.Lstat) {
			ownership = false
			continue
		}
		current, err := os.ReadFile(file.Path)
		if err != nil {
			return adapter.RemovalPlan{}, err
		}
		if file.Kind == "instruction" {
			begin, end, err := instructionMarkers(a.target)
			if err != nil {
				return adapter.RemovalPlan{}, err
			}
			stripped, changed := stripBlock(string(current), begin, end)
			if !changed {
				continue
			}
			id := "remove-block-" + shortHash([]byte(file.Path))
			steps = append(steps, adapter.PlanStep{
				StepID: id, Kind: adapter.StepRemoveManagedBlock,
				Description: "remove Atlas managed instruction block", Path: file.Path,
				Reversibility: adapter.Reversible, Rollback: &adapter.RollbackAction{Kind: adapter.RollbackRestoreSnapshot, Path: file.Path},
			})
			payloads[id] = []byte(stripped)
			continue
		}
		if string(current) != file.Body {
			ownership = false
		}
		steps = append(steps, adapter.PlanStep{
			StepID: "remove-" + shortHash([]byte(file.Path)), Kind: adapter.StepRemoveManagedFile,
			Description: "remove Atlas-owned " + file.Kind, Path: file.Path,
			Reversibility: adapter.Reversible, Rollback: &adapter.RollbackAction{Kind: adapter.RollbackRestoreSnapshot, Path: file.Path},
		})
	}
	if contextPath := generatedContextPath(state.WorkspaceRoot, caps.SkillDir); contextPath != "" {
		if raw, err := os.ReadFile(contextPath); err == nil && strings.Contains(string(raw), "Atlas provider context") {
			steps = append(steps, adapter.PlanStep{
				StepID: "remove-context-" + string(a.target), Kind: adapter.StepRemoveManagedFile,
				Description: "remove generated provider/actor context", Path: contextPath,
				Reversibility: adapter.Reversible, Rollback: &adapter.RollbackAction{Kind: adapter.RollbackRestoreSnapshot, Path: contextPath},
			})
		}
	}
	cliRemoved := false
	if preferred, ok := caps.Preferred(); ok && preferred.WriteMethod == adapter.WriteMethodClientCLI && state.Registration != nil {
		if remove := clientRemoveCommand(a.target, state.ClientExecutable, state.Registration.ServerName, state.WorkspaceRoot); remove != nil && remove.Validate() == nil {
			steps = append(steps, adapter.PlanStep{
				StepID: "remove-cli-" + state.Registration.ServerName, Kind: adapter.StepRunClientCommand,
				Description: "remove Atlas-owned MCP server via client CLI", Command: remove,
				Reversibility:      adapter.Irreversible,
				IrreversibleReason: "client MCP removal is applied through the provider CLI; re-adding requires a new setup",
			})
			cliRemoved = true
		}
	}
	if !cliRemoved {
		if state.ConfigPath == "" {
			if preferred, ok := caps.Preferred(); ok {
				if path, ok := preferred.ResolvePath(state.WorkspaceRoot, a.effectiveHome()); ok {
					state.ConfigPath = path
				}
			} else if len(caps.Scopes) > 0 {
				if path, ok := caps.Scopes[0].ResolvePath(state.WorkspaceRoot, a.effectiveHome()); ok {
					state.ConfigPath = path
				}
			}
		}
		if state.ConfigPath != "" {
			if raw, err := os.ReadFile(state.ConfigPath); err == nil && state.Registration != nil {
				owned := false
				method := writeMethodForPath(caps, state.ConfigPath, state.WorkspaceRoot, a.effectiveHome())
				switch {
				case looksJSON(state.ConfigPath) || method == adapter.WriteMethodPortableOnly:
					_, owned, _ = DecodeAtlasEntry(raw, *state.Registration)
				case strings.HasSuffix(state.ConfigPath, ".toml"):
					owned = tomlEntryOwned(raw, state.Registration.ServerName, state.WorkspaceID)
				default:
					owned = state.NativeEntryFingerprint == "" || NativeFingerprint(extractNative(raw, state.Registration.ServerName)) == state.NativeEntryFingerprint
				}
				if owned && method == adapter.WriteMethodPortableOnly {
					steps = append(steps, adapter.PlanStep{
						StepID: "remove-portable-" + state.Registration.ServerName, Kind: adapter.StepRemoveManagedFile,
						Description: "remove portable Atlas MCP descriptor", Path: state.ConfigPath,
						Reversibility: adapter.Reversible, Rollback: &adapter.RollbackAction{Kind: adapter.RollbackRestoreSnapshot, Path: state.ConfigPath},
					})
				} else if owned && method == adapter.WriteMethodAtlasFileEdit {
					var next []byte
					var err error
					if looksJSON(state.ConfigPath) {
						next, err = RemoveJSONServer(raw, state.Registration.ServerName)
					} else {
						next, err = RemoveTOMLServer(raw, state.Registration.ServerName)
					}
					if err != nil {
						ownership = false
					} else {
						id := "remove-config-" + state.Registration.ServerName
						steps = append(steps, adapter.PlanStep{
							StepID: id, Kind: adapter.StepRemoveConfigEntry, Scope: state.Scope,
							Description: "remove Atlas MCP entry", Path: state.ConfigPath,
							Reversibility: adapter.Reversible, Rollback: &adapter.RollbackAction{Kind: adapter.RollbackRestoreSnapshot, Path: state.ConfigPath},
						})
						payloads[id] = next
					}
				} else if !owned {
					ownership = false
				}
			}
		}
	}
	statePath := filepath.Join(a.stateDir, "workspaces", state.WorkspaceID, string(a.target)+".json")
	if _, err := os.Lstat(statePath); err == nil {
		steps = append(steps, adapter.PlanStep{
			StepID: "remove-state-" + string(a.target), Kind: adapter.StepRemoveLocalState,
			Description: "remove local integration state", Path: statePath,
			Reversibility: adapter.Reversible, Rollback: &adapter.RollbackAction{Kind: adapter.RollbackRestoreSnapshot, Path: statePath},
		})
	}
	plan := adapter.IntegrationPlan{
		PlanID: "remove-" + string(a.target), ContractVersion: adapter.ContractVersion,
		Operation: adapter.PlanOperationRemove, Target: a.target,
		WorkspaceID: state.WorkspaceID, WorkspaceRoot: state.WorkspaceRoot,
		Home: a.effectiveHome(), LocalStateRoot: a.stateDir, Scope: state.Scope,
		Detection: detection, Steps: steps, GeneratedAt: time.Now().UTC(), NoOp: len(steps) == 0,
		ConsentedRoots: nil,
	}
	if err := plan.Validate(); err != nil {
		return adapter.RemovalPlan{}, err
	}
	a.lastPayloads = payloads
	return adapter.RemovalPlan{Plan: plan, OwnershipVerified: ownership, RequiresConfirmation: !ownership}, nil
}

var _ adapter.AgentIntegrationAdapter = (*Adapter)(nil)

func (a *Adapter) effectiveHome() string {
	return a.home
}

func generatedContextPath(workspaceRoot, skillDir string) string {
	if workspaceRoot == "" || skillDir == "" {
		return ""
	}
	return filepath.Join(workspaceRoot, filepath.FromSlash(skillDir), "references", "atlas-context.md")
}

func writeMethodForPath(caps adapter.Capabilities, path, root, home string) adapter.WriteMethod {
	for _, scope := range caps.Scopes {
		if resolved, ok := scope.ResolvePath(root, home); ok && resolved == path {
			return scope.WriteMethod
		}
	}
	if strings.Contains(filepath.ToSlash(path), "/.tracker/integrations/") {
		return adapter.WriteMethodPortableOnly
	}
	return adapter.WriteMethodAtlasFileEdit
}

func tomlEntryOwned(raw []byte, name, workspaceID string) bool {
	for _, found := range serversFromTOML(raw) {
		if found.Name == name {
			return found.OK && atlasOwnedEntry(found.Entry, workspaceID)
		}
	}
	return false
}

func firstAbs(state adapter.IntegrationState) string {
	if state.Registration != nil && isAbs(state.Registration.Command) {
		return state.Registration.Command
	}
	return "/usr/local/bin/tracker"
}

func looksJSON(path string) bool { return len(path) >= 5 && path[len(path)-5:] == ".json" }

func extractNative(raw []byte, name string) []byte { return raw }
