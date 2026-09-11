package host

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
)

// Prepared is a validated plan plus the private file payloads the journal needs.
type Prepared struct {
	Plan     adapter.IntegrationPlan
	Payloads map[string][]byte
	Record   adapter.IntegrationState
}

// Configurator adds provider-specific MCP registration steps.
type Configurator func(ctx *BuildContext) error

// BuildContext is the mutable plan being assembled.
type BuildContext struct {
	Input      adapter.PlanInput
	Caps       adapter.Capabilities
	Scope      adapter.ScopeCapability
	Detection  adapter.Detection
	Reg        *adapter.MCPRegistration
	Steps      []adapter.PlanStep
	Payloads   map[string][]byte
	Approvals  []adapter.ApprovalStep
	Warnings   []string
	Resulting  adapter.State
	SkillVer   string
	BlockVer   string
	Now        time.Time
	StateDir   string
	ConfigPath string
	NativeFP   string
}

// Adapter is the shared implementation used by all six Sprint 114.2 targets.
type Adapter struct {
	target       integrations.Target
	configure    Configurator
	runner       adapter.CommandRunner
	now          func() time.Time
	stateDir     string
	preferStdMCP bool
	customConfig string
	lastPayloads map[string][]byte
	home         string
}

func New(target integrations.Target, configure Configurator) *Adapter {
	return &Adapter{target: target, configure: configure}
}

func (a *Adapter) WithRunner(r adapter.CommandRunner) *Adapter { a.runner = r; return a }
func (a *Adapter) WithNow(now func() time.Time) *Adapter       { a.now = now; return a }
func (a *Adapter) WithStateDir(dir string) *Adapter            { a.stateDir = dir; return a }
func (a *Adapter) WithPreferStandardMCP(v bool) *Adapter       { a.preferStdMCP = v; return a }
func (a *Adapter) WithCustomConfig(path string) *Adapter       { a.customConfig = path; return a }
func (a *Adapter) WithHome(home string) *Adapter               { a.home = home; return a }

func (a *Adapter) Target() integrations.Target { return a.target }

func (a *Adapter) Capabilities() adapter.Capabilities {
	caps, _ := adapter.CapabilitiesFor(a.target)
	return caps
}

func (a *Adapter) Detect(ctx context.Context, input adapter.DetectInput) adapter.Detection {
	if a.runner != nil && input.Runner == nil {
		input.Runner = a.runner
	}
	detection := DetectClient(ctx, a.target, adapter.PlanInput{WorkspaceRoot: input.WorkspaceRoot, Home: input.Home}, input)
	_ = a.scanExistingServers(ctx, &detection, input)
	_ = detection.Validate()
	return detection
}

func (a *Adapter) Plan(ctx context.Context, input adapter.PlanInput) (adapter.IntegrationPlan, error) {
	prepared, err := a.Prepare(ctx, input)
	if err != nil {
		return adapter.IntegrationPlan{}, err
	}
	return prepared.Plan, nil
}

func (a *Adapter) Prepare(ctx context.Context, input adapter.PlanInput) (Prepared, error) {
	if err := input.Validate(); err != nil {
		return Prepared{}, err
	}
	if input.Detection.Target != a.target {
		return Prepared{}, fmt.Errorf("detection target %s does not match %s", input.Detection.Target, a.target)
	}
	detectIn := adapter.DetectInput{WorkspaceRoot: input.WorkspaceRoot, Home: input.Home, Runner: a.runner}
	_ = a.scanExistingServers(ctx, &input.Detection, detectIn)
	caps := a.Capabilities()
	scope, err := pickScope(caps, input.Scope)
	if err != nil {
		return Prepared{}, err
	}
	now := time.Now().UTC()
	if a.now != nil {
		now = a.now()
	}
	bc := &BuildContext{
		Input:     input,
		Caps:      caps,
		Scope:     scope,
		Detection: input.Detection,
		Payloads:  map[string][]byte{},
		Now:       now,
		StateDir:  a.stateDir,
		Resulting: adapter.StateConfiguredUnverified,
	}
	if a.target == integrations.TargetGeneric {
		bc.Resulting = adapter.StatePortableReady
	}
	unsupported := input.Detection.Installed && input.Detection.VersionSupport != adapter.VersionSupported && a.target != integrations.TargetGeneric
	if unsupported {
		bc.Resulting = adapter.StateUnsupportedClientVersion
	}

	if err := addSkillSteps(bc); err != nil {
		return Prepared{}, err
	}
	if !unsupported {
		if err := addRegistration(bc); err != nil {
			return Prepared{}, err
		}
		if a.configure != nil {
			if err := a.configure(bc); err != nil {
				return Prepared{}, err
			}
		}
	}
	if err := addStateStep(bc); err != nil {
		return Prepared{}, err
	}
	writes := 0
	for _, step := range bc.Steps {
		if step.Writes() {
			writes++
		}
	}
	plan := adapter.IntegrationPlan{
		PlanID:          "setup-" + string(a.target),
		ContractVersion: adapter.ContractVersion,
		Operation:       adapter.PlanOperationSetup,
		Target:          a.target,
		WorkspaceID:     input.WorkspaceID,
		WorkspaceRoot:   input.WorkspaceRoot,
		Home:            input.Home,
		LocalStateRoot:  filepath.Clean(bc.StateDir),
		Scope:           scope.Scope,
		Detection:       input.Detection,
		Registration:    bc.Reg,
		Steps:           bc.Steps,
		ApprovalSteps:   bc.Approvals,
		ConsentedRoots:  append([]string(nil), input.ConsentedRoots...),
		ResultingState:  bc.Resulting,
		Warnings:        bc.Warnings,
		GeneratedAt:     now,
		NoOp:            writes == 0,
	}
	if plan.LocalStateRoot == "" || plan.LocalStateRoot == "." {
		return Prepared{}, fmt.Errorf("local state root is required")
	}
	if err := plan.Validate(); err != nil {
		return Prepared{}, err
	}
	a.lastPayloads = bc.Payloads
	record := adapter.IntegrationState{
		Target:                 a.target,
		ContractVersion:        adapter.ContractVersion,
		State:                  persistableState(bc.Resulting),
		Scope:                  scope.Scope,
		WorkspaceID:            input.WorkspaceID,
		WorkspaceRoot:          input.WorkspaceRoot,
		ConfigPath:             bc.ConfigPath,
		ClientExecutable:       input.Detection.ExecutablePath,
		Registration:           bc.Reg,
		SkillVersion:           bc.SkillVer,
		ManagedBlockVersion:    bc.BlockVer,
		ClientVersion:          input.Detection.Version,
		ActorHint:              ActorHint(input),
		NativeEntryFingerprint: bc.NativeFP,
		UpdatedAt:              now,
	}
	if bc.Reg != nil {
		record.EntryFingerprint = bc.Reg.Fingerprint()
	}
	return Prepared{Plan: plan, Payloads: bc.Payloads, Record: record}, nil
}

func pickScope(caps adapter.Capabilities, requested adapter.ConfigScope) (adapter.ScopeCapability, error) {
	if requested == "" {
		scope, ok := caps.Preferred()
		if !ok {
			return adapter.ScopeCapability{}, fmt.Errorf("no preferred scope")
		}
		return scope, nil
	}
	rows := caps.ScopesFor(requested)
	if len(rows) == 0 {
		return adapter.ScopeCapability{}, fmt.Errorf("%s does not support %s", caps.Target, requested)
	}
	return rows[0], nil
}

func ActorHint(input adapter.PlanInput) contracts.Actor {
	if input.ActorHint != "" {
		return input.ActorHint
	}
	return contracts.Actor("agent:" + string(input.Detection.Target))
}

func addSkillSteps(bc *BuildContext) error {
	installer := integrations.Installer{Root: bc.Input.WorkspaceRoot}
	files, err := installer.Preview(bc.Input.Detection.Target)
	if err != nil {
		return err
	}
	contextBody := providerContext(bc)
	for _, file := range files {
		if file.Kind == "skill" && filepath.Base(file.Path) == "SKILL.md" && bc.SkillVer == "" {
			bc.SkillVer = shortHash([]byte(file.Body))
		}
		if file.Kind == "instruction" {
			bc.BlockVer = shortHash([]byte(file.Body))
		}
		if file.Change == integrations.InstallChangeNone {
			continue
		}
		kind := adapter.StepWriteManagedFile
		if file.Kind == "instruction" && file.Change == integrations.InstallChangeUpdate {
			kind = adapter.StepUpdateManagedBlock
		}
		addFileStep(bc, "file-"+shortHash([]byte(file.Path)), kind, "install Atlas-owned "+file.Kind+" for "+string(bc.Input.Detection.Target), file.Path, []byte(file.Body), file.Change == integrations.InstallChangeCreate)
	}
	if contextBody != "" {
		contextPath := filepath.Join(bc.Input.WorkspaceRoot, filepath.FromSlash(bc.Caps.SkillDir), "references", "atlas-context.md")
		current, _ := os.ReadFile(contextPath)
		if string(current) != contextBody {
			created := len(current) == 0
			addFileStep(bc, "context-"+string(bc.Input.Detection.Target), adapter.StepWriteManagedFile, "write generated provider/actor context", contextPath, []byte(contextBody), created)
		}
	}
	return nil
}

func providerContext(bc *BuildContext) string {
	hint := ActorHint(bc.Input)
	name := ""
	if bc.Reg != nil {
		name = bc.Reg.ServerName
	} else if n, err := adapter.ServerNameFor(bc.Input.WorkspaceID); err == nil {
		name = n
	}
	return fmt.Sprintf(`# Atlas provider context

- Provider: %s
- Actor hint: %s
- MCP server name: %s
- Tool profile: %s
- Workspace ID: %s

The actor hint guides this skill. It does not bypass server authorization.
Every MCP write still requires an explicit actor and reason.
This file contains no secrets or machine-local home paths.
`, bc.Input.Detection.Target, hint, name, adapter.RegistrationToolProfile, bc.Input.WorkspaceID)
}

func addRegistration(bc *BuildContext) error {
	portable := bc.Input.Detection.Target == integrations.TargetGeneric && bc.Scope.Binding == adapter.WorkspaceBindingVerifiedCwd && bc.Scope.WriteMethod == adapter.WriteMethodPortableOnly
	command := bc.Input.TrackerPath
	if portable {
		command = adapter.PortableExecutableName
	}
	binding := adapter.WorkspaceBinding{Kind: bc.Scope.Binding}
	switch bc.Scope.Binding {
	case adapter.WorkspaceBindingAbsolutePath:
		binding.WorkspaceRoot = bc.Input.WorkspaceRoot
	case adapter.WorkspaceBindingClientVariable:
		binding.Placeholder = "${workspaceFolder}"
	}
	reg, err := adapter.NewRegistration(command, bc.Input.WorkspaceID, binding, ActorHint(bc.Input), portable)
	if err != nil {
		return err
	}
	if err := reg.ValidateForScope(bc.Scope.Scope, bc.Input.Home); err != nil {
		if portable {
			return err
		}
		// Repository-carried scopes cannot embed a home-local tracker. Fall
		// back to the portable executable name with verified_cwd when the
		// preferred binding allows it.
		if bc.Scope.Scope.RepositoryCarried() && bc.Scope.Binding != adapter.WorkspaceBindingAbsolutePath {
			altBinding := adapter.WorkspaceBinding{Kind: adapter.WorkspaceBindingVerifiedCwd}
			if bc.Scope.Binding == adapter.WorkspaceBindingClientVariable {
				altBinding = binding
			}
			portableCmd := adapter.PortableExecutableName
			if bc.Scope.Binding == adapter.WorkspaceBindingClientVariable {
				// Cursor still wants an absolute-or-portable command; use system path if not under home.
				portableCmd = command
			}
			reg, err = adapter.NewRegistration(portableCmd, bc.Input.WorkspaceID, altBinding, ActorHint(bc.Input), portableCmd == adapter.PortableExecutableName)
			if err != nil {
				return err
			}
			if err := reg.ValidateForScope(bc.Scope.Scope, bc.Input.Home); err != nil {
				bc.Warnings = append(bc.Warnings, "registration not placed in repository-carried config: "+err.Error())
				return nil
			}
		} else {
			bc.Warnings = append(bc.Warnings, "registration not placed: "+err.Error())
			return nil
		}
	}
	bc.Reg = &reg
	return nil
}

func addStateStep(bc *BuildContext) error {
	if bc.StateDir == "" {
		return fmt.Errorf("state dir is required")
	}
	statePath := filepath.Join(filepath.Clean(bc.StateDir), "workspaces", bc.Input.WorkspaceID, string(bc.Input.Detection.Target)+".json")
	record := adapter.IntegrationState{
		Target:              bc.Input.Detection.Target,
		ContractVersion:     adapter.ContractVersion,
		State:               persistableState(bc.Resulting),
		Scope:               bc.Scope.Scope,
		WorkspaceID:         bc.Input.WorkspaceID,
		WorkspaceRoot:       bc.Input.WorkspaceRoot,
		ConfigPath:          bc.ConfigPath,
		ClientExecutable:    bc.Input.Detection.ExecutablePath,
		Registration:        bc.Reg,
		SkillVersion:        bc.SkillVer,
		ManagedBlockVersion: bc.BlockVer,
		ClientVersion:       bc.Input.Detection.Version,
		ActorHint:           ActorHint(bc.Input),
		UpdatedAt:           bc.Now,
	}
	if bc.Reg != nil {
		record.EntryFingerprint = bc.Reg.Fingerprint()
	}
	if bc.NativeFP != "" {
		record.NativeEntryFingerprint = bc.NativeFP
	}
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	created := true
	if _, err := os.Lstat(statePath); err == nil {
		created = false
		if current, err := os.ReadFile(statePath); err == nil && string(current) == string(raw) && len(bc.Steps) == 0 {
			return nil
		}
	}
	addFileStep(bc, "state-"+string(bc.Input.Detection.Target), adapter.StepRecordLocalState, "record local integration state for "+string(bc.Input.Detection.Target), statePath, raw, created)
	return nil
}

func AddConfigFile(bc *BuildContext, id, desc, path string, body []byte, created bool) {
	addFileStep(bc, id, adapter.StepWriteConfigEntry, desc, path, body, created)
}

func AddManagedFile(bc *BuildContext, id, desc, path string, body []byte, created bool) {
	addFileStep(bc, id, adapter.StepWriteManagedFile, desc, path, body, created)
}

func addFileStep(bc *BuildContext, id string, kind adapter.StepKind, desc, path string, body []byte, created bool) {
	rollback := &adapter.RollbackAction{Kind: adapter.RollbackRestoreSnapshot, Path: path}
	if created {
		rollback = &adapter.RollbackAction{Kind: adapter.RollbackDeleteCreated, Path: path}
	}
	mode := uint32(0o644)
	if kind == adapter.StepRecordLocalState {
		mode = 0o600
	}
	if kind == adapter.StepRemoveManagedFile || kind == adapter.StepRemoveManagedBlock || kind == adapter.StepRemoveConfigEntry || kind == adapter.StepRemoveLocalState {
		mode = 0
	}
	step := adapter.PlanStep{
		StepID:        id,
		Kind:          kind,
		Description:   desc,
		Path:          path,
		Reversibility: adapter.Reversible,
		Rollback:      rollback,
		Mode:          mode,
	}
	if kind == adapter.StepWriteConfigEntry || kind == adapter.StepRemoveConfigEntry {
		step.Scope = bc.Scope.Scope
	}
	bc.Steps = append(bc.Steps, step)
	if len(body) > 0 || kind == adapter.StepRemoveManagedBlock {
		bc.Payloads[id] = body
	}
}

func AddCommandStep(bc *BuildContext, id, desc string, cmd adapter.Command, rollback *adapter.Command) {
	step := adapter.PlanStep{
		StepID:        id,
		Kind:          adapter.StepRunClientCommand,
		Description:   desc,
		Command:       &cmd,
		Reversibility: adapter.Reversible,
	}
	if rollback != nil {
		step.Rollback = &adapter.RollbackAction{Kind: adapter.RollbackRunCommand, Command: rollback}
	}
	bc.Steps = append(bc.Steps, step)
}

func UnmanagedSameName(detection adapter.Detection, name string) bool {
	for _, existing := range detection.ExistingServers {
		if existing.Name == name && !existing.AtlasOwned {
			return true
		}
	}
	return false
}
