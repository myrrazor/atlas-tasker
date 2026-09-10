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

// PlanInput is what Plan receives.
type PlanInput struct {
	WorkspaceRoot string          `json:"workspace_root"`
	WorkspaceID   string          `json:"workspace_id"`
	Home          string          `json:"home,omitempty"`
	TrackerPath   string          `json:"tracker_path"`
	ActorHint     contracts.Actor `json:"actor_hint,omitempty"`
	Detection     Detection       `json:"detection"`
	// Scope overrides the adapter's preferred scope when the user selected
	// another one explicitly (for example Claude project .mcp.json).
	Scope ConfigScope `json:"scope,omitempty"`
	// ConsentedRoots are additional absolute directories the user explicitly
	// allowed Atlas to write into (generic custom config destinations).
	ConsentedRoots []string `json:"consented_roots,omitempty"`
	// Existing is the recorded state from a previous setup, if any.
	Existing *IntegrationState `json:"existing,omitempty"`
}

func (p PlanInput) Validate() error {
	if !filepath.IsAbs(p.WorkspaceRoot) || filepath.Clean(p.WorkspaceRoot) != p.WorkspaceRoot {
		return fmt.Errorf("workspace root must be a clean absolute path")
	}
	if strings.TrimSpace(p.WorkspaceID) == "" {
		return fmt.Errorf("workspace id is required")
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
		if !filepath.IsAbs(root) || filepath.Clean(root) != root {
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
	StepRenderPortableEntry StepKind = "render_portable_entry"
)

func (k StepKind) IsValid() bool {
	switch k {
	case StepWriteManagedFile, StepUpdateManagedBlock, StepWriteConfigEntry, StepRunClientCommand,
		StepRemoveManagedFile, StepRemoveManagedBlock, StepRemoveConfigEntry, StepRecordLocalState, StepRenderPortableEntry:
		return true
	default:
		return false
	}
}

// TouchesClientConfig reports whether the step changes the client's own
// configuration (as opposed to Atlas-owned instruction, skill, or state
// files). Unsupported client versions forbid these steps.
func (k StepKind) TouchesClientConfig() bool {
	switch k {
	case StepWriteConfigEntry, StepRemoveConfigEntry, StepRunClientCommand:
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
// copy of the pre-write file identified by its SHA-256.
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
	default:
		return fmt.Errorf("invalid rollback kind %q", r.Kind)
	}
	return nil
}

// PlanStep is one exact action. File steps carry Path; command steps carry
// Command; every write carries either a Rollback or an irreversible
// classification with a reason.
type PlanStep struct {
	StepID             string          `json:"step_id"`
	Kind               StepKind        `json:"kind"`
	Description        string          `json:"description"`
	Path               string          `json:"path,omitempty"`
	Scope              ConfigScope     `json:"scope,omitempty"`
	Command            *Command        `json:"command,omitempty"`
	Reversibility      Reversibility   `json:"reversibility,omitempty"`
	IrreversibleReason string          `json:"irreversible_reason,omitempty"`
	Rollback           *RollbackAction `json:"rollback,omitempty"`
	// Mode is the file mode for created files (0600 for local state, 0644 for
	// repository-carried managed files).
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
	// Home is the user's home directory, used only to refuse home-relative
	// executables in repository-carried configuration.
	Home string `json:"home,omitempty"`
	// LocalStateRoot is the private machine-local directory (0700) that
	// record_local_state steps must stay inside.
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
	if strings.TrimSpace(p.WorkspaceID) == "" {
		return fmt.Errorf("plan workspace id is required")
	}
	if !filepath.IsAbs(p.WorkspaceRoot) || filepath.Clean(p.WorkspaceRoot) != p.WorkspaceRoot {
		return fmt.Errorf("plan workspace root must be a clean absolute path")
	}
	if !p.Scope.IsValid() {
		return fmt.Errorf("plan scope %q is invalid", p.Scope)
	}
	if err := p.Detection.Validate(); err != nil {
		return fmt.Errorf("plan detection: %w", err)
	}
	if p.Detection.Target != p.Target {
		return fmt.Errorf("plan detection target %q does not match plan target %q", p.Detection.Target, p.Target)
	}
	for _, root := range p.ConsentedRoots {
		if !filepath.IsAbs(root) || filepath.Clean(root) != root {
			return fmt.Errorf("consented root must be a clean absolute path: %q", root)
		}
		if isWithin(p.WorkspaceRoot, root) || isWithin(root, p.WorkspaceRoot) {
			return fmt.Errorf("consented root %q overlaps the workspace", root)
		}
	}
	if p.Home != "" && (!filepath.IsAbs(p.Home) || filepath.Clean(p.Home) != p.Home) {
		return fmt.Errorf("plan home must be a clean absolute path")
	}
	if p.LocalStateRoot != "" {
		if !filepath.IsAbs(p.LocalStateRoot) || filepath.Clean(p.LocalStateRoot) != p.LocalStateRoot {
			return fmt.Errorf("local state root must be a clean absolute path")
		}
		if isWithin(p.WorkspaceRoot, p.LocalStateRoot) {
			return fmt.Errorf("local state root must live outside the workspace")
		}
	}
	if p.Registration != nil {
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
	} else if err := p.validateResultingState(unsupported); err != nil {
		return err
	}
	ids := map[string]struct{}{}
	writes := 0
	for i, step := range p.Steps {
		if err := p.validateStep(step, unsupported); err != nil {
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
// version and installation gates, and the rule that a pending human step can
// never coexist with a verified or portable promise.
func (p IntegrationPlan) validateResultingState(unsupported bool) error {
	if !p.ResultingState.IsValid() {
		return fmt.Errorf("plan resulting state %q is invalid", p.ResultingState)
	}
	caps, err := CapabilitiesFor(p.Target)
	if err != nil {
		return err
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
	for _, approval := range p.ApprovalSteps {
		pending, ok := approval.Requirement.PendingState()
		if !ok {
			return fmt.Errorf("approval step %q does not require a human action", approval.Requirement)
		}
		if strings.TrimSpace(approval.Instruction) == "" {
			return fmt.Errorf("approval step %s requires an instruction", approval.Requirement)
		}
		if p.ResultingState.Verified() || p.ResultingState == StatePortableReady {
			return fmt.Errorf("a plan with a pending %s step cannot promise %s; use %s", approval.Requirement, p.ResultingState, pending)
		}
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
		case StepRemoveManagedFile, StepRemoveManagedBlock, StepRemoveConfigEntry, StepRunClientCommand, StepRecordLocalState:
		default:
			return fmt.Errorf("removal plans may not contain %s steps", step.Kind)
		}
		if step.Kind == StepRunClientCommand && step.Command != nil && step.Command.Purpose == CommandPurposeRegister {
			return fmt.Errorf("removal plans may not run register commands")
		}
	}
	return nil
}

func (p IntegrationPlan) validateStep(step PlanStep, unsupported bool) error {
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
	switch step.Kind {
	case StepRunClientCommand:
		if step.Command == nil {
			return fmt.Errorf("command step requires a command")
		}
		if err := step.Command.Validate(); err != nil {
			return err
		}
		if step.Path != "" {
			return fmt.Errorf("command steps do not carry a path")
		}
		if !step.Command.Mutates() {
			return nil
		}
	case StepRecordLocalState:
		if step.Command != nil {
			return fmt.Errorf("local state steps do not carry a command")
		}
		if err := p.validateStepPath(step.Path, true); err != nil {
			return err
		}
		if step.Mode != 0o600 {
			return fmt.Errorf("local state files must be written with mode 0600")
		}
	case StepRenderPortableEntry:
		if step.Command != nil || step.Rollback != nil {
			return fmt.Errorf("portable render steps write nothing and need no rollback")
		}
		return nil
	default:
		if step.Command != nil {
			return fmt.Errorf("%s steps do not carry a command", step.Kind)
		}
		if err := p.validateStepPath(step.Path, false); err != nil {
			return err
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

// validateStepPath requires a clean absolute path contained in the workspace
// or a consented root; local state paths must stay inside LocalStateRoot.
// Symlink and ownership checks happen at apply time against the live
// filesystem; the plan proves containment.
func (p IntegrationPlan) validateStepPath(path string, localState bool) error {
	if path == "" {
		return fmt.Errorf("path is required")
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("path must be a clean absolute path: %q", path)
	}
	if strings.ContainsRune(path, 0) {
		return fmt.Errorf("path contains a NUL byte")
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
		return nil
	}
	for _, root := range p.ConsentedRoots {
		if isWithin(root, path) && path != root {
			return nil
		}
	}
	return fmt.Errorf("path %q is outside the workspace and every consented root", path)
}

// stateAtMost orders states by how much they claim so a capability cap can be
// enforced: verified states claim the most, portable_ready and unverified
// claim less, and terminal/blocked states claim nothing.
func stateAtMost(state State, cap State) bool {
	return stateRank(state) <= stateRank(cap)
}

func stateRank(state State) int {
	switch state {
	case StateConnected:
		return 4
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

// FileIdentity is the before/after record the journal keeps for every file a
// step touches. It never includes file contents.
type FileIdentity struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	Mode   uint32 `json:"mode,omitempty"`
	Size   int64  `json:"size,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
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
	Target              integrations.Target `json:"target"`
	ContractVersion     int                 `json:"contract_version"`
	State               State               `json:"state"`
	Scope               ConfigScope         `json:"scope"`
	WorkspaceID         string              `json:"workspace_id"`
	WorkspaceRoot       string              `json:"workspace_root"`
	ConfigPath          string              `json:"config_path,omitempty"`
	Registration        *MCPRegistration    `json:"registration,omitempty"`
	EntryFingerprint    string              `json:"entry_fingerprint,omitempty"`
	SkillVersion        string              `json:"skill_version,omitempty"`
	ManagedBlockVersion string              `json:"managed_block_version,omitempty"`
	ClientVersion       ClientVersion       `json:"client_version"`
	ActorHint           contracts.Actor     `json:"actor_hint,omitempty"`
	LastVerifiedAt      time.Time           `json:"last_verified_at,omitempty"`
	RepairReason        string              `json:"repair_reason,omitempty"`
	UpdatedAt           time.Time           `json:"updated_at"`
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
	if s.Registration != nil {
		if err := s.Registration.Validate(); err != nil {
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
// passed probe-class check; a manual client check can never produce one.
type Verification struct {
	Target         integrations.Target `json:"target"`
	State          State               `json:"state"`
	ConnectionKind ConnectionKind      `json:"connection_kind,omitempty"`
	Checks         []VerificationCheck `json:"checks"`
	Reasons        []string            `json:"reasons,omitempty"`
	CheckedAt      time.Time           `json:"checked_at"`
	Probes         []ProbeRecord       `json:"probes,omitempty"`
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
	probePassed := false
	for _, check := range v.Checks {
		if strings.TrimSpace(check.Name) == "" {
			return fmt.Errorf("verification check requires a name")
		}
		if check.Passed && check.Method != VerificationManualClientCheck {
			probePassed = true
		}
	}
	if v.State.Verified() {
		if !probePassed {
			return fmt.Errorf("%s requires at least one passed probe or client-native check", v.State)
		}
		if !v.ConnectionKind.IsValid() {
			return fmt.Errorf("%s requires a connection kind", v.State)
		}
		if v.Target == integrations.TargetGeneric && v.ConnectionKind != ConnectionKindStandardConfig && v.ConnectionKind != ConnectionKindCustomAdapter {
			return fmt.Errorf("generic connections must be standard_config or custom_adapter")
		}
	}
	for _, probe := range v.Probes {
		if err := probe.Command.Validate(); err != nil {
			return fmt.Errorf("verification probe: %w", err)
		}
		if probe.Command.Mutates() {
			return fmt.Errorf("verification probes must be read-only")
		}
	}
	return nil
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
