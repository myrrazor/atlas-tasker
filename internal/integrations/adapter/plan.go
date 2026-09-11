package adapter

import (
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
)

// ContractVersion is bumped whenever a persisted IntegrationState or plan
// shape changes incompatibly. Adapters record it so repair can detect drift.
const ContractVersion = 1

// VersionSupport is the adapter's verdict on a detected client version.
type VersionSupport string

const (
	VersionSupported   VersionSupport = "supported"
	VersionUnsupported VersionSupport = "unsupported"
	VersionUnknown     VersionSupport = "unknown"
)

func (v VersionSupport) IsValid() bool {
	switch v {
	case VersionSupported, VersionUnsupported, VersionUnknown:
		return true
	default:
		return false
	}
}

// ClientVersion is the parsed client version. Raw keeps the exact probe text
// for diagnostics; Known is false when parsing failed.
type ClientVersion struct {
	Raw   string `json:"raw,omitempty"`
	Major int    `json:"major"`
	Minor int    `json:"minor"`
	Patch int    `json:"patch"`
	Known bool   `json:"known"`
}

// ConfigLocation is one client configuration path the adapter inspected.
type ConfigLocation struct {
	Scope  ConfigScope  `json:"scope"`
	Path   string       `json:"path"`
	Exists bool         `json:"exists"`
	Format ConfigFormat `json:"format"`
	// Symlink is true when the path or a parent is a symlink; adapters fail
	// closed on symlinked configuration.
	Symlink bool `json:"symlink,omitempty"`
}

// ExistingServer is an MCP server definition already present in the client.
type ExistingServer struct {
	Name        string      `json:"name"`
	Scope       ConfigScope `json:"scope"`
	AtlasOwned  bool        `json:"atlas_owned"`
	Fingerprint string      `json:"fingerprint,omitempty"`
}

// Detection is the read-only result of AgentIntegrationAdapter.Detect.
type Detection struct {
	Target          integrations.Target `json:"target"`
	Installed       bool                `json:"installed"`
	ExecutablePath  string              `json:"executable_path,omitempty"`
	Version         ClientVersion       `json:"version"`
	VersionSupport  VersionSupport      `json:"version_support"`
	ConfigLocations []ConfigLocation    `json:"config_locations,omitempty"`
	ExistingServers []ExistingServer    `json:"existing_servers,omitempty"`
	Reasons         []string            `json:"reasons,omitempty"`
	Probes          []ProbeRecord       `json:"probes,omitempty"`
}

func (d Detection) Validate() error {
	if !isKnownTarget(d.Target) {
		return fmt.Errorf("unknown detection target %q", d.Target)
	}
	if !d.VersionSupport.IsValid() {
		return fmt.Errorf("invalid version support %q", d.VersionSupport)
	}
	if d.Installed && d.Target != integrations.TargetGeneric && d.ExecutablePath == "" {
		return fmt.Errorf("installed client requires an executable path")
	}
	if d.ExecutablePath != "" {
		if err := validateAbsoluteExecutable(d.ExecutablePath); err != nil {
			return err
		}
	}
	if !d.Installed && d.VersionSupport == VersionSupported {
		return fmt.Errorf("a client that is not installed cannot have a supported version")
	}
	if d.VersionSupport == VersionSupported && !d.Version.Known {
		return fmt.Errorf("supported version requires a parsed version")
	}
	for _, probe := range d.Probes {
		if err := probe.Command.Validate(); err != nil {
			return fmt.Errorf("probe: %w", err)
		}
		if probe.Command.Mutates() {
			return fmt.Errorf("detection probes must be read-only, got %s", probe.Command.Purpose)
		}
		if d.ExecutablePath == "" || probe.Command.Executable != d.ExecutablePath {
			return fmt.Errorf("detection probes must run the detected client executable %q, got %q", d.ExecutablePath, probe.Command.Executable)
		}
	}
	return nil
}

// DetectInput is what Detect receives. Function fields are injectable so
// tests never touch the real PATH, HOME, or filesystem; nil fields fall back
// to the os defaults exactly like integrations.DetectOptions.
type DetectInput struct {
	WorkspaceRoot string
	Home          string
	LookPath      func(string) (string, error)
	Stat          func(string) (os.FileInfo, error)
	Lstat         func(string) (os.FileInfo, error)
	ReadFile      func(string) ([]byte, error)
	Getenv        func(string) string
	// Runner is optional. A nil Runner means Detect must not execute any client
	// binary and reports VersionUnknown for installed clients.
	Runner CommandRunner
}

// PlanInput is what Plan receives. Home is required: the repository-carried
// placement rules cannot be checked without it, so an engine that does not
// know the home directory cannot ask for a plan.
type PlanInput struct {
	WorkspaceRoot string          `json:"workspace_root"`
	WorkspaceID   string          `json:"workspace_id"`
	Home          string          `json:"home"`
	TrackerPath   string          `json:"tracker_path"`
	ActorHint     contracts.Actor `json:"actor_hint,omitempty"`
	Detection     Detection       `json:"detection"`
	// Scope overrides the adapter's preferred scope when the user selected
	// another one explicitly (for example Claude project .mcp.json).
	Scope ConfigScope `json:"scope,omitempty"`
	// ConsentedRoots are additional absolute directories the user explicitly
	// allowed Atlas to write into (generic custom config destinations, the
	// OpenClaw global skill directory, a named client's user-scope file).
	ConsentedRoots []string `json:"consented_roots,omitempty"`
	// Existing is the recorded state from a previous setup, if any.
	Existing *IntegrationState `json:"existing,omitempty"`
}

func (p PlanInput) Validate() error {
	if !isCleanAbsolute(p.WorkspaceRoot) {
		return fmt.Errorf("workspace root must be a clean absolute path")
	}
	if strings.TrimSpace(p.WorkspaceID) == "" {
		return fmt.Errorf("workspace id is required")
	}
	if !isCleanAbsolute(p.Home) {
		return fmt.Errorf("home must be a clean absolute path")
	}
	if err := validateAbsoluteExecutable(p.TrackerPath); err != nil {
		return fmt.Errorf("tracker path: %w", err)
	}
	if p.ActorHint != "" && !p.ActorHint.IsValid() {
		return fmt.Errorf("invalid actor hint: %s", p.ActorHint)
	}
	if err := p.Detection.Validate(); err != nil {
		return err
	}
	if p.Scope != "" && !p.Scope.IsValid() {
		return fmt.Errorf("invalid scope %q", p.Scope)
	}
	for _, root := range p.ConsentedRoots {
		if !isCleanAbsolute(root) {
			return fmt.Errorf("consented root must be a clean absolute path: %q", root)
		}
	}
	return nil
}

// StepKind is the closed set of things a plan may do.
type StepKind string

const (
	StepWriteManagedFile    StepKind = "write_managed_file"
	StepUpdateManagedBlock  StepKind = "update_managed_block"
	StepWriteConfigEntry    StepKind = "write_config_entry"
	StepRunClientCommand    StepKind = "run_client_command"
	StepRemoveManagedFile   StepKind = "remove_managed_file"
	StepRemoveManagedBlock  StepKind = "remove_managed_block"
	StepRemoveConfigEntry   StepKind = "remove_config_entry"
	StepRecordLocalState    StepKind = "record_local_state"
	StepRemoveLocalState    StepKind = "remove_local_state"
	StepRenderPortableEntry StepKind = "render_portable_entry"
)

func (k StepKind) IsValid() bool {
	switch k {
	case StepWriteManagedFile, StepUpdateManagedBlock, StepWriteConfigEntry, StepRunClientCommand,
		StepRemoveManagedFile, StepRemoveManagedBlock, StepRemoveConfigEntry,
		StepRecordLocalState, StepRemoveLocalState, StepRenderPortableEntry:
		return true
	default:
		return false
	}
}

// TouchesClientConfig reports whether the step changes the client's own
// configuration (as opposed to Atlas-owned instruction, skill, or state
// files). Unsupported client versions forbid these steps. The kind is not
// the only guard: managed-file kinds are additionally confined to Atlas-owned
// paths, so relabelling a client config write cannot slip past this gate.
func (k StepKind) TouchesClientConfig() bool {
	switch k {
	case StepWriteConfigEntry, StepRemoveConfigEntry, StepRunClientCommand:
		return true
	default:
		return false
	}
}

// managedFile reports whether the kind writes or removes an Atlas-owned file
// (instruction file, skill, command template, generated guide, descriptor).
func (k StepKind) managedFile() bool {
	switch k {
	case StepWriteManagedFile, StepUpdateManagedBlock, StepRemoveManagedFile, StepRemoveManagedBlock:
		return true
	default:
		return false
	}
}

// configEntry reports whether the kind edits a client configuration file.
func (k StepKind) configEntry() bool {
	return k == StepWriteConfigEntry || k == StepRemoveConfigEntry
}

// localState reports whether the kind touches the private local state record.
func (k StepKind) localState() bool {
	return k == StepRecordLocalState || k == StepRemoveLocalState
}

// removes reports whether the kind deletes content that existed before apply.
func (k StepKind) removes() bool {
	switch k {
	case StepRemoveManagedFile, StepRemoveManagedBlock, StepRemoveConfigEntry, StepRemoveLocalState:
		return true
	default:
		return false
	}
}

// Writes reports whether the step mutates anything. Command steps mutate only
// when their command purpose does.
func (s PlanStep) Writes() bool {
	switch s.Kind {
	case StepRenderPortableEntry:
		return false
	case StepRunClientCommand:
		return s.Command != nil && s.Command.Mutates()
	default:
		return true
	}
}

// Reversibility classifies a write for the rollback engine.
type Reversibility string

const (
	Reversible   Reversibility = "reversible"
	Irreversible Reversibility = "irreversible"
)

// RollbackKind says how a step is undone.
type RollbackKind string

const (
	RollbackRestoreSnapshot RollbackKind = "restore_snapshot"
	RollbackDeleteCreated   RollbackKind = "delete_created"
	RollbackRunCommand      RollbackKind = "run_command"
)

// RollbackAction undoes one step. Snapshot restores use the journal's private
// copy of the pre-write file identified by its SHA-256. A rollback is bound
// to its step: file rollbacks name exactly the step's path and command
// rollbacks run the step's client binary with a remove or reload purpose
// (see IntegrationPlan.Validate).
type RollbackAction struct {
	Kind    RollbackKind `json:"kind"`
	Path    string       `json:"path,omitempty"`
	Command *Command     `json:"command,omitempty"`
}

func (r RollbackAction) Validate() error {
	switch r.Kind {
	case RollbackRestoreSnapshot, RollbackDeleteCreated:
		if r.Path == "" {
			return fmt.Errorf("%s rollback requires a path", r.Kind)
		}
		if r.Command != nil {
			return fmt.Errorf("%s rollback does not take a command", r.Kind)
		}
	case RollbackRunCommand:
		if r.Command == nil {
			return fmt.Errorf("run_command rollback requires a command")
		}
		if err := r.Command.Validate(); err != nil {
			return fmt.Errorf("rollback command: %w", err)
		}
		if r.Command.Purpose != CommandPurposeRemove && r.Command.Purpose != CommandPurposeReload {
			return fmt.Errorf("run_command rollback may only remove or reload, got %s", r.Command.Purpose)
		}
	default:
		return fmt.Errorf("invalid rollback kind %q", r.Kind)
	}
	return nil
}

// PlanStep is one exact action. File steps carry Path; command steps carry
// Command; every write carries either a Rollback or an irreversible
// classification with a reason.
type PlanStep struct {
	StepID      string   `json:"step_id"`
	Kind        StepKind `json:"kind"`
	Description string   `json:"description"`
	Path        string   `json:"path,omitempty"`
	// Scope is required on config-entry steps and must equal the plan scope;
	// other steps may carry it only when it equals the plan scope.
	Scope              ConfigScope     `json:"scope,omitempty"`
	Command            *Command        `json:"command,omitempty"`
	Reversibility      Reversibility   `json:"reversibility,omitempty"`
	IrreversibleReason string          `json:"irreversible_reason,omitempty"`
	Rollback           *RollbackAction `json:"rollback,omitempty"`
	// Mode is the file mode for created or replaced files: 0600 for local
	// state, 0644 or 0600 for managed files and config entries, 0 for steps
	// that create nothing (removals, commands, portable renders).
	Mode uint32 `json:"mode,omitempty"`
}

// ApprovalStep is a human action Atlas must report and wait for. It is never
// executed by Atlas.
type ApprovalStep struct {
	Requirement ApprovalRequirement `json:"requirement"`
	Instruction string              `json:"instruction"`
}

// PlanOperation says which adapter method produced the plan. Removal plans
// carry no resulting state: after removal there is no integration to report.
type PlanOperation string

const (
	PlanOperationSetup  PlanOperation = "setup"
	PlanOperationRepair PlanOperation = "repair"
	PlanOperationRemove PlanOperation = "remove"
)

func (o PlanOperation) IsValid() bool {
	switch o {
	case PlanOperationSetup, PlanOperationRepair, PlanOperationRemove:
		return true
	default:
		return false
	}
}

// IntegrationPlan is the complete, validated, deterministic result of Plan,
// Repair, or Remove. Nothing may be written before a plan validates.
type IntegrationPlan struct {
	PlanID          string              `json:"plan_id"`
	ContractVersion int                 `json:"contract_version"`
	Operation       PlanOperation       `json:"operation"`
	Target          integrations.Target `json:"target"`
	WorkspaceID     string              `json:"workspace_id"`
	WorkspaceRoot   string              `json:"workspace_root"`
	// Home is the user's home directory. It is required whenever the plan
	// carries a non-portable registration into repository-carried
	// configuration, because that is the only way to prove the executable is
	// not a personal path.
	Home string `json:"home,omitempty"`
	// LocalStateRoot is the private machine-local directory (0700) that
	// record_local_state and remove_local_state steps must stay inside.
	LocalStateRoot string           `json:"local_state_root,omitempty"`
	Scope          ConfigScope      `json:"scope"`
	Detection      Detection        `json:"detection"`
	Registration   *MCPRegistration `json:"registration,omitempty"`
	Steps          []PlanStep       `json:"steps"`
	ApprovalSteps  []ApprovalStep   `json:"approval_steps,omitempty"`
	ConsentedRoots []string         `json:"consented_roots,omitempty"`
	ResultingState State            `json:"resulting_state"`
	Warnings       []string         `json:"warnings,omitempty"`
	GeneratedAt    time.Time        `json:"generated_at"`
	// NoOp is true when the workspace already matches the plan; Apply must
	// then write nothing.
	NoOp bool `json:"no_op,omitempty"`
}

// Validate enforces the contract rules. An adapter that produces a plan
// violating any rule has a bug; the setup engine refuses to apply it.
func (p IntegrationPlan) Validate() error {
	if p.ContractVersion != ContractVersion {
		return fmt.Errorf("plan contract version %d does not match %d", p.ContractVersion, ContractVersion)
	}
	if !p.Operation.IsValid() {
		return fmt.Errorf("plan operation %q is invalid", p.Operation)
	}
	if !isKnownTarget(p.Target) {
		return fmt.Errorf("unknown plan target %q", p.Target)
	}
	caps, err := CapabilitiesFor(p.Target)
	if err != nil {
		return err
	}
	if strings.TrimSpace(p.WorkspaceID) == "" {
		return fmt.Errorf("plan workspace id is required")
	}
	if !isCleanAbsolute(p.WorkspaceRoot) {
		return fmt.Errorf("plan workspace root must be a clean absolute path")
	}
	if !p.Scope.IsValid() {
		return fmt.Errorf("plan scope %q is invalid", p.Scope)
	}
	if len(caps.ScopesFor(p.Scope)) == 0 {
		return fmt.Errorf("%s does not support the %s scope", p.Target, p.Scope)
	}
	if err := p.Detection.Validate(); err != nil {
		return fmt.Errorf("plan detection: %w", err)
	}
	if p.Detection.Target != p.Target {
		return fmt.Errorf("plan detection target %q does not match plan target %q", p.Detection.Target, p.Target)
	}
	for _, root := range p.ConsentedRoots {
		if !isCleanAbsolute(root) {
			return fmt.Errorf("consented root must be a clean absolute path: %q", root)
		}
		if isWithin(p.WorkspaceRoot, root) || isWithin(root, p.WorkspaceRoot) {
			return fmt.Errorf("consented root %q overlaps the workspace", root)
		}
	}
	if p.Home != "" && !isCleanAbsolute(p.Home) {
		return fmt.Errorf("plan home must be a clean absolute path")
	}
	if p.LocalStateRoot != "" {
		if !isCleanAbsolute(p.LocalStateRoot) {
			return fmt.Errorf("local state root must be a clean absolute path")
		}
		if isWithin(p.WorkspaceRoot, p.LocalStateRoot) {
			return fmt.Errorf("local state root must live outside the workspace")
		}
	}
	if p.Registration != nil {
		if p.Scope.RepositoryCarried() && !p.Registration.Portable && p.Home == "" {
			return fmt.Errorf("a %s plan with a machine-local executable requires home so personal paths can be refused", p.Scope)
		}
		if err := p.Registration.ValidateForScope(p.Scope, p.Home); err != nil {
			return fmt.Errorf("plan registration: %w", err)
		}
		if p.Registration.WorkspaceID != p.WorkspaceID {
			return fmt.Errorf("plan registration is bound to another workspace")
		}
	}
	unsupported := p.Detection.Installed && p.Detection.VersionSupport != VersionSupported && p.Target != integrations.TargetGeneric
	if p.Operation == PlanOperationRemove {
		if err := p.validateRemovalShape(); err != nil {
			return err
		}
	} else if err := p.validateResultingState(caps, unsupported); err != nil {
		return err
	}
	ids := map[string]struct{}{}
	writes := 0
	for i, step := range p.Steps {
		if err := p.validateStep(step, caps, unsupported); err != nil {
			return fmt.Errorf("step %d (%s): %w", i, step.StepID, err)
		}
		if _, dup := ids[step.StepID]; dup {
			return fmt.Errorf("duplicate step id %q", step.StepID)
		}
		ids[step.StepID] = struct{}{}
		if step.Writes() {
			writes++
		}
	}
	if p.NoOp && writes > 0 {
		return fmt.Errorf("a no-op plan cannot contain %d write steps", writes)
	}
	if !p.NoOp && writes == 0 && p.ResultingState != StateUnsupportedClientVersion && p.ResultingState != StateFailed {
		return fmt.Errorf("a plan that writes nothing must be marked no_op")
	}
	for _, warning := range p.Warnings {
		if containsCredentialURL(warning) {
			return fmt.Errorf("plan warnings must not embed credentials")
		}
	}
	return nil
}

// validateResultingState applies the setup/repair promises: capability caps,
// version and installation gates, and the rule that approval steps and
// pending states imply each other exactly.
func (p IntegrationPlan) validateResultingState(caps Capabilities, unsupported bool) error {
	if !p.ResultingState.IsValid() {
		return fmt.Errorf("plan resulting state %q is invalid", p.ResultingState)
	}
	if !stateAtMost(p.ResultingState, caps.MaxPlannedState) {
		return fmt.Errorf("%s plans may not promise %s (capability cap %s)", p.Target, p.ResultingState, caps.MaxPlannedState)
	}
	if p.ResultingState.Verified() && p.Registration == nil {
		return fmt.Errorf("a %s plan requires an MCP registration", p.ResultingState)
	}
	if unsupported && p.ResultingState != StateUnsupportedClientVersion {
		return fmt.Errorf("unknown or unsupported client version must plan unsupported_client_version, got %s", p.ResultingState)
	}
	if !p.Detection.Installed && p.Target != integrations.TargetGeneric && p.ResultingState.Verified() {
		return fmt.Errorf("a client that is not installed cannot be planned as %s", p.ResultingState)
	}
	var expectedPending State
	for _, approval := range p.ApprovalSteps {
		pending, ok := approval.Requirement.PendingState()
		if !ok {
			return fmt.Errorf("approval step %q does not require a human action", approval.Requirement)
		}
		if strings.TrimSpace(approval.Instruction) == "" {
			return fmt.Errorf("approval step %s requires an instruction", approval.Requirement)
		}
		// Workspace trust is the earlier gate: until the project is trusted
		// the client does not even load the file that needs MCP approval.
		if pending == StatePendingWorkspaceTrust || expectedPending == "" {
			expectedPending = pending
		}
	}
	if expectedPending != "" && p.ResultingState != expectedPending {
		return fmt.Errorf("a plan with pending approval steps must promise %s, got %s", expectedPending, p.ResultingState)
	}
	if expectedPending == "" && (p.ResultingState == StatePendingWorkspaceTrust || p.ResultingState == StatePendingMCPApproval) {
		return fmt.Errorf("%s requires an approval step that names the outstanding human action", p.ResultingState)
	}
	return nil
}

// validateRemovalShape applies the removal-only rules: no promised state, no
// registration, no approval steps, and only removal-class steps.
func (p IntegrationPlan) validateRemovalShape() error {
	if p.ResultingState != "" {
		return fmt.Errorf("removal plans carry no resulting state, got %q", p.ResultingState)
	}
	if p.Registration != nil {
		return fmt.Errorf("removal plans do not register servers")
	}
	if len(p.ApprovalSteps) > 0 {
		return fmt.Errorf("removal plans do not wait on provider approvals")
	}
	for _, step := range p.Steps {
		switch step.Kind {
		case StepRemoveManagedFile, StepRemoveManagedBlock, StepRemoveConfigEntry, StepRunClientCommand, StepRecordLocalState, StepRemoveLocalState:
		default:
			return fmt.Errorf("removal plans may not contain %s steps", step.Kind)
		}
		if step.Kind == StepRunClientCommand && step.Command != nil && step.Command.Purpose == CommandPurposeRegister {
			return fmt.Errorf("removal plans may not run register commands")
		}
	}
	return nil
}

func (p IntegrationPlan) validateStep(step PlanStep, caps Capabilities, unsupported bool) error {
	if strings.TrimSpace(step.StepID) == "" {
		return fmt.Errorf("step id is required")
	}
	if !step.Kind.IsValid() {
		return fmt.Errorf("invalid step kind %q", step.Kind)
	}
	if strings.TrimSpace(step.Description) == "" {
		return fmt.Errorf("description is required")
	}
	if containsCredentialURL(step.Description) || containsCredentialURL(step.Path) {
		return fmt.Errorf("step must not embed credentials")
	}
	if unsupported && step.Kind.TouchesClientConfig() {
		return fmt.Errorf("client version is not verified; %s is not allowed", step.Kind)
	}
	if step.Scope != "" && step.Scope != p.Scope {
		return fmt.Errorf("step scope %q does not match plan scope %q", step.Scope, p.Scope)
	}
	switch step.Kind {
	case StepRunClientCommand:
		if step.Command == nil {
			return fmt.Errorf("command step requires a command")
		}
		if err := step.Command.Validate(); err != nil {
			return err
		}
		if step.Path != "" || step.Mode != 0 {
			return fmt.Errorf("command steps carry neither a path nor a mode")
		}
		if p.Detection.ExecutablePath == "" {
			return fmt.Errorf("command steps require a detected client executable")
		}
		if step.Command.Executable != p.Detection.ExecutablePath {
			return fmt.Errorf("command steps must run the detected client executable %q, got %q", p.Detection.ExecutablePath, step.Command.Executable)
		}
		if !step.Command.Mutates() {
			if step.Rollback != nil || step.Reversibility != "" {
				return fmt.Errorf("read-only command steps carry no rollback or reversibility")
			}
			return nil
		}
	case StepRecordLocalState, StepRemoveLocalState:
		if step.Command != nil {
			return fmt.Errorf("local state steps do not carry a command")
		}
		if err := p.validateStepPath(step.Path, true); err != nil {
			return err
		}
		if step.Kind == StepRecordLocalState && step.Mode != 0o600 {
			return fmt.Errorf("local state files must be written with mode 0600")
		}
		if step.Kind == StepRemoveLocalState && step.Mode != 0 {
			return fmt.Errorf("removal steps carry no mode")
		}
	case StepRenderPortableEntry:
		if step.Command != nil || step.Rollback != nil || step.Mode != 0 || step.Reversibility != "" {
			return fmt.Errorf("portable render steps write nothing and need no rollback, mode, or reversibility")
		}
		return nil
	default:
		if step.Command != nil {
			return fmt.Errorf("%s steps do not carry a command", step.Kind)
		}
		if err := p.validateStepPath(step.Path, false); err != nil {
			return err
		}
		if step.Kind.managedFile() {
			if err := p.validateManagedFilePath(step.Path, caps); err != nil {
				return err
			}
		}
		if step.Kind.configEntry() {
			if step.Scope == "" {
				return fmt.Errorf("%s steps must carry the plan scope", step.Kind)
			}
			if err := p.validateConfigEntryPath(step.Path, caps); err != nil {
				return err
			}
		}
		if step.Kind.removes() {
			if step.Mode != 0 {
				return fmt.Errorf("removal steps carry no mode")
			}
		} else if step.Mode != 0o644 && step.Mode != 0o600 {
			return fmt.Errorf("%s steps must use mode 0644 or 0600, got %04o", step.Kind, step.Mode)
		}
	}
	switch step.Reversibility {
	case Reversible:
		if step.Rollback == nil {
			return fmt.Errorf("reversible write requires a rollback action")
		}
		if err := step.Rollback.Validate(); err != nil {
			return err
		}
		if err := validateRollbackBinding(step, *step.Rollback); err != nil {
			return err
		}
		if step.IrreversibleReason != "" {
			return fmt.Errorf("reversible write must not carry an irreversible reason")
		}
	case Irreversible:
		if strings.TrimSpace(step.IrreversibleReason) == "" {
			return fmt.Errorf("irreversible write requires a reason")
		}
		if step.Rollback != nil {
			return fmt.Errorf("irreversible write must not carry a rollback action")
		}
	default:
		return fmt.Errorf("write step requires a reversibility classification")
	}
	return nil
}

// validateRollbackBinding ties a rollback to the step it undoes: a file step
// is undone at its own path (a removal only by restoring the snapshot), and a
// command step is undone by the same client binary with a remove or reload
// purpose. The journal engine (AT114-103) therefore never executes a rollback
// that reaches beyond what the step itself touched.
func validateRollbackBinding(step PlanStep, rollback RollbackAction) error {
	if step.Kind == StepRunClientCommand {
		if rollback.Kind != RollbackRunCommand {
			return fmt.Errorf("command steps are undone by run_command rollbacks, got %s", rollback.Kind)
		}
		if rollback.Command.Executable != step.Command.Executable {
			return fmt.Errorf("rollback command must run the step's executable %q, got %q", step.Command.Executable, rollback.Command.Executable)
		}
		return nil
	}
	if rollback.Kind == RollbackRunCommand {
		return fmt.Errorf("file steps are undone by restore_snapshot or delete_created, not run_command")
	}
	if rollback.Path != step.Path {
		return fmt.Errorf("rollback path %q must equal the step path %q", rollback.Path, step.Path)
	}
	if step.Kind.removes() && rollback.Kind != RollbackRestoreSnapshot {
		return fmt.Errorf("%s is undone by restore_snapshot, got %s", step.Kind, rollback.Kind)
	}
	return nil
}

// validateStepPath requires a clean absolute path contained in the workspace
// or a consented root; local state paths must stay inside LocalStateRoot.
// Git metadata is never writable, and the tracker's own runtime directory
// admits only its integrations subtree. Symlink and ownership checks happen
// at apply time against the live filesystem; the plan proves containment.
func (p IntegrationPlan) validateStepPath(path string, localState bool) error {
	if path == "" {
		return fmt.Errorf("path is required")
	}
	if !isCleanAbsolute(path) {
		return fmt.Errorf("path must be a clean absolute path: %q", path)
	}
	if strings.ContainsRune(path, 0) {
		return fmt.Errorf("path contains a NUL byte")
	}
	if hasPathComponent(path, gitDirName) {
		return fmt.Errorf("path %q is inside Git metadata", path)
	}
	if localState {
		if p.LocalStateRoot == "" {
			return fmt.Errorf("local state step requires local_state_root")
		}
		if !isWithin(p.LocalStateRoot, path) || path == p.LocalStateRoot {
			return fmt.Errorf("local state path %q is outside the local state root", path)
		}
		return nil
	}
	if isWithin(p.WorkspaceRoot, path) && path != p.WorkspaceRoot {
		trackerDir := filepath.Join(p.WorkspaceRoot, trackerDirName)
		integrationsDir := filepath.Join(trackerDir, trackerIntegrationsDirName)
		if isWithin(trackerDir, path) && (!isWithin(integrationsDir, path) || path == integrationsDir) {
			return fmt.Errorf("path %q is inside the tracker runtime directory; only %s/%s is writable by integrations", path, trackerDirName, trackerIntegrationsDirName)
		}
		return nil
	}
	if hasPathComponent(path, trackerDirName) {
		return fmt.Errorf("path %q is inside another tracker runtime directory", path)
	}
	for _, root := range p.ConsentedRoots {
		if isWithin(root, path) && path != root {
			return nil
		}
	}
	return fmt.Errorf("path %q is outside the workspace and every consented root", path)
}

// validateManagedFilePath confines managed-file steps to Atlas-owned
// locations inside the workspace (instruction file, skill directory, command
// directory, .tracker/integrations) or to a consented root, and never to a
// file any supported client loads as configuration. Writes outside the
// workspace require a known Home so those checks can run.
func (p IntegrationPlan) validateManagedFilePath(path string, caps Capabilities) error {
	if _, client := clientConfigPaths(p.WorkspaceRoot, p.Home)[path]; client {
		return fmt.Errorf("path %q is client configuration; use write_config_entry or remove_config_entry", path)
	}
	if !isWithin(p.WorkspaceRoot, path) {
		if strings.TrimSpace(p.Home) == "" {
			return fmt.Errorf("path %q is outside the workspace and home is unknown; refused", path)
		}
		return nil // inside a consented root, proven by validateStepPath
	}
	for _, rel := range caps.ManagedRoots() {
		root := filepath.Join(p.WorkspaceRoot, filepath.FromSlash(rel))
		if path == root && rel == caps.InstructionFile {
			return nil
		}
		if rel != caps.InstructionFile && isWithin(root, path) && path != root {
			return nil
		}
	}
	return fmt.Errorf("path %q is not an Atlas-owned location for %s (%s)", path, p.Target, strings.Join(caps.ManagedRoots(), ", "))
}

// validateConfigEntryPath allows config-entry steps only at a file the target
// documents for the plan scope with the atlas_file_edit write method, or at a
// user-selected destination inside a consented root.
func (p IntegrationPlan) validateConfigEntryPath(path string, caps Capabilities) error {
	inWorkspace := isWithin(p.WorkspaceRoot, path)
	for _, scope := range caps.ScopesFor(p.Scope) {
		if scope.WriteMethod != WriteMethodAtlasFileEdit {
			continue
		}
		if scope.UserSelected {
			if !inWorkspace {
				return nil // a consented root, proven by validateStepPath
			}
			continue
		}
		if resolved, ok := scope.ResolvePath(p.WorkspaceRoot, p.Home); ok && resolved == path {
			return nil
		}
	}
	return fmt.Errorf("path %q is not a %s configuration file %s edits in the %s scope", path, p.Target, WriteMethodAtlasFileEdit, p.Scope)
}

const (
	gitDirName                 = ".git"
	trackerDirName             = ".tracker"
	trackerIntegrationsDirName = "integrations"
)

func hasPathComponent(path string, name string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == name {
			return true
		}
	}
	return false
}

func isCleanAbsolute(path string) bool {
	return path != "" && filepath.IsAbs(path) && filepath.Clean(path) == path
}

// stateAtMost orders states by how much they claim so a capability cap can be
// enforced: connected claims the most, connected_restart_required less (the
// client still has to reload), portable_ready and configured_unverified less
// again, and terminal or blocked states claim nothing. The cap bounds what a
// plan may promise; Verification reports what a probe proved (see
// Verification.Validate).
func stateAtMost(state State, cap State) bool {
	return stateRank(state) <= stateRank(cap)
}

func stateRank(state State) int {
	switch state {
	case StateConnected:
		return 5
	case StateConnectedRestartRequired:
		return 4
	case StateConfiguredUnverified:
		return 2
	case StatePortableReady:
		return 2
	default:
		return 0
	}
}

// Fingerprint hashes the semantic content of a plan (everything except
// PlanID and GeneratedAt). Two plans from identical inputs must share it.
func (p IntegrationPlan) Fingerprint() (string, error) {
	clone := p
	clone.PlanID = ""
	clone.GeneratedAt = time.Time{}
	raw, err := json.Marshal(clone)
	if err != nil {
		return "", fmt.Errorf("fingerprint plan: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// FileOwner is the numeric owner of a file as the journal saw it. Plan
// AT114-103 records ownership with mode and hash so that an Atlas-owned file
// that changed hands is treated as drift, not silently overwritten.
type FileOwner struct {
	UID uint32 `json:"uid"`
	GID uint32 `json:"gid"`
}

// FileIdentity is the before/after record the journal keeps for every file a
// step touches. It never includes file contents. Owner is nil only on
// platforms that do not expose numeric ownership; the journal engine records
// that as a limitation rather than inventing values.
type FileIdentity struct {
	Path   string     `json:"path"`
	Exists bool       `json:"exists"`
	Mode   uint32     `json:"mode,omitempty"`
	Size   int64      `json:"size,omitempty"`
	Owner  *FileOwner `json:"owner,omitempty"`
	SHA256 string     `json:"sha256,omitempty"`
}

// StepStatus is the outcome of one applied step.
type StepStatus string

const (
	StepApplied    StepStatus = "applied"
	StepSkipped    StepStatus = "skipped"
	StepFailed     StepStatus = "failed"
	StepRolledBack StepStatus = "rolled_back"
)

func (s StepStatus) IsValid() bool {
	switch s {
	case StepApplied, StepSkipped, StepFailed, StepRolledBack:
		return true
	default:
		return false
	}
}

// StepOutcome records what happened to one step.
type StepOutcome struct {
	StepID string        `json:"step_id"`
	Status StepStatus    `json:"status"`
	Before *FileIdentity `json:"before,omitempty"`
	After  *FileIdentity `json:"after,omitempty"`
	Error  string        `json:"error,omitempty"`
}

// ApplyResult is what Apply returns. A failed apply reports the rolled-back
// steps so the operator can see exactly what was touched and undone.
type ApplyResult struct {
	PlanID   string              `json:"plan_id"`
	Target   integrations.Target `json:"target"`
	State    State               `json:"state"`
	Outcomes []StepOutcome       `json:"outcomes"`
	// Record is the local integration state to persist when State is not
	// failed.
	Record *IntegrationState `json:"record,omitempty"`
	Error  string            `json:"error,omitempty"`
}

func (r ApplyResult) Validate() error {
	if strings.TrimSpace(r.PlanID) == "" {
		return fmt.Errorf("apply result requires a plan id")
	}
	if !isKnownTarget(r.Target) {
		return fmt.Errorf("unknown apply target %q", r.Target)
	}
	if !r.State.IsValid() {
		return fmt.Errorf("invalid apply state %q", r.State)
	}
	if r.State.Verified() {
		return fmt.Errorf("apply cannot report %s; only Verify may", r.State)
	}
	for _, outcome := range r.Outcomes {
		if strings.TrimSpace(outcome.StepID) == "" || !outcome.Status.IsValid() {
			return fmt.Errorf("invalid step outcome for %q", outcome.StepID)
		}
		if outcome.Status == StepFailed && outcome.Error == "" {
			return fmt.Errorf("failed step %s requires an error", outcome.StepID)
		}
	}
	if r.State == StateFailed && r.Record != nil {
		return fmt.Errorf("a failed apply must not persist a record")
	}
	if r.State != StateFailed && r.Record == nil {
		return fmt.Errorf("a %s apply must produce a record", r.State)
	}
	if r.Record != nil {
		if err := r.Record.Validate(); err != nil {
			return err
		}
		if r.Record.State != r.State {
			return fmt.Errorf("record state %s does not match apply state %s", r.Record.State, r.State)
		}
	}
	return nil
}

// IntegrationState is the machine-local record of one provider integration.
// It lives in the private setup manifest, never in the repository, and never
// carries credentials or ticket text.
type IntegrationState struct {
	Target          integrations.Target `json:"target"`
	ContractVersion int                 `json:"contract_version"`
	State           State               `json:"state"`
	Scope           ConfigScope         `json:"scope"`
	WorkspaceID     string              `json:"workspace_id"`
	WorkspaceRoot   string              `json:"workspace_root"`
	ConfigPath      string              `json:"config_path,omitempty"`
	// ClientExecutable is the client binary detection found; repair compares
	// it and Verify may only probe with it.
	ClientExecutable string           `json:"client_executable,omitempty"`
	Registration     *MCPRegistration `json:"registration,omitempty"`
	// EntryFingerprint is MCPRegistration.Fingerprint of the entry Atlas
	// wrote: it proves command identity (name, command, args), not that the
	// native entry is byte-for-byte unchanged. Adapters that edit files also
	// record NativeEntryFingerprint so user edits to the Atlas entry are
	// detected before repair or removal touches it.
	EntryFingerprint       string          `json:"entry_fingerprint,omitempty"`
	NativeEntryFingerprint string          `json:"native_entry_fingerprint,omitempty"`
	SkillVersion           string          `json:"skill_version,omitempty"`
	ManagedBlockVersion    string          `json:"managed_block_version,omitempty"`
	ClientVersion          ClientVersion   `json:"client_version"`
	ActorHint              contracts.Actor `json:"actor_hint,omitempty"`
	LastVerifiedAt         time.Time       `json:"last_verified_at,omitempty"`
	RepairReason           string          `json:"repair_reason,omitempty"`
	UpdatedAt              time.Time       `json:"updated_at"`
}

func (s IntegrationState) Validate() error {
	if !isKnownTarget(s.Target) {
		return fmt.Errorf("unknown state target %q", s.Target)
	}
	if s.ContractVersion != ContractVersion {
		return fmt.Errorf("state contract version %d does not match %d", s.ContractVersion, ContractVersion)
	}
	if !s.State.IsValid() {
		return fmt.Errorf("invalid state %q", s.State)
	}
	if !s.Scope.IsValid() {
		return fmt.Errorf("invalid scope %q", s.Scope)
	}
	if strings.TrimSpace(s.WorkspaceID) == "" || !filepath.IsAbs(s.WorkspaceRoot) {
		return fmt.Errorf("state requires workspace id and absolute workspace root")
	}
	if s.ClientExecutable != "" {
		if err := validateAbsoluteExecutable(s.ClientExecutable); err != nil {
			return fmt.Errorf("client executable: %w", err)
		}
	}
	if s.Registration != nil {
		if err := s.Registration.ValidateForScope(s.Scope, ""); err != nil {
			return err
		}
		if s.Registration.WorkspaceID != s.WorkspaceID {
			return fmt.Errorf("state registration is bound to another workspace")
		}
		if s.EntryFingerprint == "" {
			return fmt.Errorf("a registered state requires an entry fingerprint")
		}
	}
	if s.State.Verified() && s.LastVerifiedAt.IsZero() {
		return fmt.Errorf("%s requires last_verified_at", s.State)
	}
	if s.State == StateRepairRequired && strings.TrimSpace(s.RepairReason) == "" {
		return fmt.Errorf("repair_required requires a repair reason")
	}
	if s.UpdatedAt.IsZero() {
		return fmt.Errorf("state requires updated_at")
	}
	return nil
}

// VerificationCheck is one concrete check with its result.
type VerificationCheck struct {
	Name   string             `json:"name"`
	Method VerificationMethod `json:"method"`
	Passed bool               `json:"passed"`
	Detail string             `json:"detail,omitempty"`
}

// Verification is what Verify returns. A verified state needs at least one
// passed check whose method is evidence for the reported ConnectionKind; a
// manual client check can never produce one. Verification is deliberately
// not bounded by Capabilities.MaxPlannedState: that cap limits what a plan
// may promise before anything ran, whereas Verify reports what a probe
// proved after the fact (an OpenClaw gateway that was restarted and probed
// live is connected).
type Verification struct {
	Target         integrations.Target `json:"target"`
	State          State               `json:"state"`
	ConnectionKind ConnectionKind      `json:"connection_kind,omitempty"`
	// ClientExecutable and ServerExecutable are the only binaries the probes
	// may have run: the detected client and the tracker command that was
	// actually started for the self-probe.
	ClientExecutable string              `json:"client_executable,omitempty"`
	ServerExecutable string              `json:"server_executable,omitempty"`
	Checks           []VerificationCheck `json:"checks"`
	Reasons          []string            `json:"reasons,omitempty"`
	CheckedAt        time.Time           `json:"checked_at"`
	Probes           []ProbeRecord       `json:"probes,omitempty"`
}

// connectionKindEvidence maps each connection kind to the verification methods
// that can prove it.
var connectionKindEvidence = map[ConnectionKind][]VerificationMethod{
	ConnectionKindClientNative:   {VerificationClientCLIList, VerificationClientCLIGet, VerificationClientCLIDoctor},
	ConnectionKindSelfProbe:      {VerificationSelfProbe},
	ConnectionKindStandardConfig: {VerificationConformanceHost},
	ConnectionKindCustomAdapter:  {VerificationConformanceHost},
}

func (v Verification) Validate() error {
	if !isKnownTarget(v.Target) {
		return fmt.Errorf("unknown verification target %q", v.Target)
	}
	if !v.State.IsValid() {
		return fmt.Errorf("invalid verification state %q", v.State)
	}
	if v.CheckedAt.IsZero() {
		return fmt.Errorf("verification requires checked_at")
	}
	if v.ConnectionKind != "" && !v.ConnectionKind.IsValid() {
		return fmt.Errorf("invalid connection kind %q", v.ConnectionKind)
	}
	for _, exe := range []string{v.ClientExecutable, v.ServerExecutable} {
		if exe != "" {
			if err := validateAbsoluteExecutable(exe); err != nil {
				return err
			}
		}
	}
	passedMethods := map[VerificationMethod]bool{}
	for _, check := range v.Checks {
		if strings.TrimSpace(check.Name) == "" {
			return fmt.Errorf("verification check requires a name")
		}
		if check.Passed && check.Method != VerificationManualClientCheck {
			passedMethods[check.Method] = true
		}
	}
	if v.State.Verified() {
		if len(passedMethods) == 0 {
			return fmt.Errorf("%s requires at least one passed probe or client-native check", v.State)
		}
		if !v.ConnectionKind.IsValid() {
			return fmt.Errorf("%s requires a connection kind", v.State)
		}
		generic := v.ConnectionKind == ConnectionKindStandardConfig || v.ConnectionKind == ConnectionKindCustomAdapter
		if (v.Target == integrations.TargetGeneric) != generic {
			if generic {
				return fmt.Errorf("%s is a generic connection kind; named clients report client_native or self_probe", v.ConnectionKind)
			}
			return fmt.Errorf("generic connections must be standard_config or custom_adapter")
		}
		backed := false
		for _, method := range connectionKindEvidence[v.ConnectionKind] {
			if passedMethods[method] {
				backed = true
			}
		}
		if !backed {
			return fmt.Errorf("connection kind %s needs a passed check with one of its methods %v", v.ConnectionKind, connectionKindEvidence[v.ConnectionKind])
		}
	}
	for _, probe := range v.Probes {
		if err := probe.Command.Validate(); err != nil {
			return fmt.Errorf("verification probe: %w", err)
		}
		if probe.Command.Mutates() {
			return fmt.Errorf("verification probes must be read-only")
		}
		expected := v.ClientExecutable
		if probe.Command.Purpose == CommandPurposeProbe {
			expected = v.ServerExecutable
		}
		if expected == "" || probe.Command.Executable != expected {
			return fmt.Errorf("verification probe %q must run %s (%q), got %q", probe.Command.Purpose, probeExecutableName(probe.Command.Purpose), expected, probe.Command.Executable)
		}
	}
	return nil
}

func probeExecutableName(purpose CommandPurpose) string {
	if purpose == CommandPurposeProbe {
		return "the registered server executable"
	}
	return "the detected client executable"
}

// RepairPlan wraps the plan that returns a drifted integration to its
// recorded state.
type RepairPlan struct {
	Reason string          `json:"reason"`
	Plan   IntegrationPlan `json:"plan"`
}

func (r RepairPlan) Validate() error {
	if strings.TrimSpace(r.Reason) == "" {
		return fmt.Errorf("repair plan requires a reason")
	}
	if r.Plan.Operation != PlanOperationRepair {
		return fmt.Errorf("repair plan must carry the repair operation, got %q", r.Plan.Operation)
	}
	return r.Plan.Validate()
}

// RemovalPlan removes only Atlas-owned entries. When the recorded fingerprint
// no longer matches what is on disk, ownership is ambiguous and the plan must
// require confirmation instead of deleting.
type RemovalPlan struct {
	Plan                 IntegrationPlan `json:"plan"`
	OwnershipVerified    bool            `json:"ownership_verified"`
	RequiresConfirmation bool            `json:"requires_confirmation"`
	Reasons              []string        `json:"reasons,omitempty"`
}

func (r RemovalPlan) Validate() error {
	if r.Plan.Operation != PlanOperationRemove {
		return fmt.Errorf("removal plan must carry the remove operation, got %q", r.Plan.Operation)
	}
	if err := r.Plan.Validate(); err != nil {
		return err
	}
	if !r.OwnershipVerified && !r.RequiresConfirmation {
		return fmt.Errorf("removal with unverified ownership must require confirmation")
	}
	return nil
}
