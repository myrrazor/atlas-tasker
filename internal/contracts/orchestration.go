package contracts

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type DispatchMode string

const (
	DispatchModeManual    DispatchMode = "manual"
	DispatchModeSuggest   DispatchMode = "suggest"
	DispatchModeAutoRoute DispatchMode = "auto_route"
)

var validDispatchModes = map[DispatchMode]struct{}{
	DispatchModeManual:    {},
	DispatchModeSuggest:   {},
	DispatchModeAutoRoute: {},
}

func (m DispatchMode) IsValid() bool {
	_, ok := validDispatchModes[m]
	return ok
}

type AgentProvider string

const (
	AgentProviderCodex  AgentProvider = "codex"
	AgentProviderClaude AgentProvider = "claude"
	AgentProviderHuman  AgentProvider = "human"
	AgentProviderCustom AgentProvider = "custom"
)

var validAgentProviders = map[AgentProvider]struct{}{
	AgentProviderCodex:  {},
	AgentProviderClaude: {},
	AgentProviderHuman:  {},
	AgentProviderCustom: {},
}

func (p AgentProvider) IsValid() bool {
	_, ok := validAgentProviders[p]
	return ok
}

type AgentRole string

const (
	AgentRoleWorker        AgentRole = "worker"
	AgentRoleReviewer      AgentRole = "reviewer"
	AgentRoleOwnerDelegate AgentRole = "owner_delegate"
	AgentRoleQA            AgentRole = "qa"
	AgentRoleObserver      AgentRole = "observer"
)

var validAgentRoles = map[AgentRole]struct{}{
	AgentRoleWorker:        {},
	AgentRoleReviewer:      {},
	AgentRoleOwnerDelegate: {},
	AgentRoleQA:            {},
	AgentRoleObserver:      {},
}

func (r AgentRole) IsValid() bool {
	_, ok := validAgentRoles[r]
	return ok
}

type RunStatus string

const (
	RunStatusPlanned        RunStatus = "planned"
	RunStatusDispatched     RunStatus = "dispatched"
	RunStatusAttached       RunStatus = "attached"
	RunStatusActive         RunStatus = "active"
	RunStatusHandoffReady   RunStatus = "handoff_ready"
	RunStatusAwaitingReview RunStatus = "awaiting_review"
	RunStatusAwaitingOwner  RunStatus = "awaiting_owner"
	RunStatusCompleted      RunStatus = "completed"
	RunStatusFailed         RunStatus = "failed"
	RunStatusAborted        RunStatus = "aborted"
	RunStatusCleanedUp      RunStatus = "cleaned_up"
)

var validRunStatuses = map[RunStatus]struct{}{
	RunStatusPlanned:        {},
	RunStatusDispatched:     {},
	RunStatusAttached:       {},
	RunStatusActive:         {},
	RunStatusHandoffReady:   {},
	RunStatusAwaitingReview: {},
	RunStatusAwaitingOwner:  {},
	RunStatusCompleted:      {},
	RunStatusFailed:         {},
	RunStatusAborted:        {},
	RunStatusCleanedUp:      {},
}

func (s RunStatus) IsValid() bool {
	_, ok := validRunStatuses[s]
	return ok
}

func (s RunStatus) Allows(next RunStatus) bool {
	allowed := map[RunStatus][]RunStatus{
		RunStatusPlanned:        {RunStatusDispatched, RunStatusAborted},
		RunStatusDispatched:     {RunStatusAttached, RunStatusActive, RunStatusFailed, RunStatusAborted},
		RunStatusAttached:       {RunStatusActive, RunStatusFailed, RunStatusAborted},
		RunStatusActive:         {RunStatusHandoffReady, RunStatusAwaitingReview, RunStatusAwaitingOwner, RunStatusFailed, RunStatusAborted},
		RunStatusHandoffReady:   {RunStatusActive, RunStatusAwaitingReview, RunStatusAwaitingOwner, RunStatusCompleted, RunStatusFailed, RunStatusAborted},
		RunStatusAwaitingReview: {RunStatusActive, RunStatusHandoffReady, RunStatusAwaitingOwner, RunStatusCompleted, RunStatusFailed, RunStatusAborted},
		RunStatusAwaitingOwner:  {RunStatusActive, RunStatusHandoffReady, RunStatusCompleted, RunStatusFailed, RunStatusAborted},
		RunStatusCompleted:      {RunStatusCleanedUp},
		RunStatusFailed:         {RunStatusCleanedUp},
		RunStatusAborted:        {RunStatusCleanedUp},
	}
	for _, candidate := range allowed[s] {
		if candidate == next {
			return true
		}
	}
	return false
}

type RunKind string

const (
	RunKindWork    RunKind = "work"
	RunKindReview  RunKind = "review"
	RunKindQA      RunKind = "qa"
	RunKindRelease RunKind = "release"
)

var validRunKinds = map[RunKind]struct{}{
	RunKindWork:    {},
	RunKindReview:  {},
	RunKindQA:      {},
	RunKindRelease: {},
}

func (k RunKind) IsValid() bool {
	_, ok := validRunKinds[k]
	return ok
}

type GateKind string

const (
	GateKindReview  GateKind = "review"
	GateKindOwner   GateKind = "owner"
	GateKindQA      GateKind = "qa"
	GateKindRelease GateKind = "release"
	GateKindDesign  GateKind = "design"
)

var validGateKinds = map[GateKind]struct{}{
	GateKindReview:  {},
	GateKindOwner:   {},
	GateKindQA:      {},
	GateKindRelease: {},
	GateKindDesign:  {},
}

func (k GateKind) IsValid() bool {
	_, ok := validGateKinds[k]
	return ok
}

type GateState string

const (
	GateStateOpen     GateState = "open"
	GateStateApproved GateState = "approved"
	GateStateRejected GateState = "rejected"
	GateStateWaived   GateState = "waived"
)

var validGateStates = map[GateState]struct{}{
	GateStateOpen:     {},
	GateStateApproved: {},
	GateStateRejected: {},
	GateStateWaived:   {},
}

func (s GateState) IsValid() bool {
	_, ok := validGateStates[s]
	return ok
}

type EvidenceType string

const (
	EvidenceTypeNote               EvidenceType = "note"
	EvidenceTypeTestResult         EvidenceType = "test_result"
	EvidenceTypeFileDiffSummary    EvidenceType = "file_diff_summary"
	EvidenceTypeLogExcerpt         EvidenceType = "log_excerpt"
	EvidenceTypeScreenshot         EvidenceType = "screenshot"
	EvidenceTypeArtifactRef        EvidenceType = "artifact_ref"
	EvidenceTypeCommitRef          EvidenceType = "commit_ref"
	EvidenceTypeManualAssertion    EvidenceType = "manual_assertion"
	EvidenceTypeUnresolvedQuestion EvidenceType = "unresolved_question"
	EvidenceTypeReviewChecklist    EvidenceType = "review_checklist"
)

var validEvidenceTypes = map[EvidenceType]struct{}{
	EvidenceTypeNote:               {},
	EvidenceTypeTestResult:         {},
	EvidenceTypeFileDiffSummary:    {},
	EvidenceTypeLogExcerpt:         {},
	EvidenceTypeScreenshot:         {},
	EvidenceTypeArtifactRef:        {},
	EvidenceTypeCommitRef:          {},
	EvidenceTypeManualAssertion:    {},
	EvidenceTypeUnresolvedQuestion: {},
	EvidenceTypeReviewChecklist:    {},
}

func (t EvidenceType) IsValid() bool {
	_, ok := validEvidenceTypes[t]
	return ok
}

type WorktreeMode string

const (
	WorktreeModePerRun   WorktreeMode = "per_run"
	WorktreeModeShared   WorktreeMode = "shared"
	WorktreeModeDisabled WorktreeMode = "disabled"
)

var validWorktreeModes = map[WorktreeMode]struct{}{
	WorktreeModePerRun:   {},
	WorktreeModeShared:   {},
	WorktreeModeDisabled: {},
}

func (m WorktreeMode) IsValid() bool {
	_, ok := validWorktreeModes[m]
	return ok
}

type WorktreeConfig struct {
	Enabled          bool         `json:"enabled,omitempty" yaml:"enabled,omitempty" toml:"enabled"`
	Root             string       `json:"root,omitempty" yaml:"root,omitempty" toml:"root"`
	DefaultMode      WorktreeMode `json:"default_mode,omitempty" yaml:"default_mode,omitempty" toml:"default_mode"`
	AutoPrune        bool         `json:"auto_prune,omitempty" yaml:"auto_prune,omitempty" toml:"auto_prune"`
	RequireCleanMain bool         `json:"require_clean_main,omitempty" yaml:"require_clean_main,omitempty" toml:"require_clean_main"`

	enabledSet          bool `json:"-" yaml:"-" toml:"-"`
	autoPruneSet        bool `json:"-" yaml:"-" toml:"-"`
	requireCleanMainSet bool `json:"-" yaml:"-" toml:"-"`
}

func (c WorktreeConfig) Validate() error {
	if c.DefaultMode != "" && !c.DefaultMode.IsValid() {
		return fmt.Errorf("invalid worktree default_mode: %s", c.DefaultMode)
	}
	return nil
}

func (c WorktreeConfig) EnabledConfigured() bool {
	return c.enabledSet
}

func (c WorktreeConfig) AutoPruneConfigured() bool {
	return c.autoPruneSet
}

func (c WorktreeConfig) RequireCleanMainConfigured() bool {
	return c.requireCleanMainSet
}

type worktreeConfigYAML struct {
	Enabled          *bool        `yaml:"enabled,omitempty"`
	Root             string       `yaml:"root,omitempty"`
	DefaultMode      WorktreeMode `yaml:"default_mode,omitempty"`
	AutoPrune        *bool        `yaml:"auto_prune,omitempty"`
	RequireCleanMain *bool        `yaml:"require_clean_main,omitempty"`
}

func (c WorktreeConfig) MarshalYAML() (any, error) {
	out := worktreeConfigYAML{
		Root:        c.Root,
		DefaultMode: c.DefaultMode,
	}
	if c.Enabled || c.enabledSet {
		value := c.Enabled
		out.Enabled = &value
	}
	if c.AutoPrune || c.autoPruneSet {
		value := c.AutoPrune
		out.AutoPrune = &value
	}
	if c.RequireCleanMain || c.requireCleanMainSet {
		value := c.RequireCleanMain
		out.RequireCleanMain = &value
	}
	return out, nil
}

func (c *WorktreeConfig) UnmarshalYAML(value *yaml.Node) error {
	var raw worktreeConfigYAML
	if err := value.Decode(&raw); err != nil {
		return err
	}
	c.Root = raw.Root
	c.DefaultMode = raw.DefaultMode
	c.Enabled = false
	c.AutoPrune = false
	c.RequireCleanMain = false
	c.enabledSet = raw.Enabled != nil
	c.autoPruneSet = raw.AutoPrune != nil
	c.requireCleanMainSet = raw.RequireCleanMain != nil
	if raw.Enabled != nil {
		c.Enabled = *raw.Enabled
	}
	if raw.AutoPrune != nil {
		c.AutoPrune = *raw.AutoPrune
	}
	if raw.RequireCleanMain != nil {
		c.RequireCleanMain = *raw.RequireCleanMain
	}
	return nil
}

type RunbookMap struct {
	TicketType TicketType `json:"ticket_type,omitempty" yaml:"ticket_type,omitempty" toml:"ticket_type"`
	Runbook    string     `json:"runbook,omitempty" yaml:"runbook,omitempty" toml:"runbook"`
}

func (m RunbookMap) Validate() error {
	if m.TicketType != "" && !m.TicketType.IsValid() {
		return fmt.Errorf("invalid runbook mapping ticket_type: %s", m.TicketType)
	}
	if strings.TrimSpace(m.Runbook) == "" {
		return fmt.Errorf("runbook mapping requires runbook")
	}
	return nil
}

type RoutingHint struct {
	TicketType       TicketType `json:"ticket_type,omitempty" yaml:"ticket_type,omitempty" toml:"ticket_type"`
	Capability       string     `json:"capability,omitempty" yaml:"capability,omitempty" toml:"capability"`
	PreferredAgentID string     `json:"preferred_agent_id,omitempty" yaml:"preferred_agent_id,omitempty" toml:"preferred_agent_id"`
}

func (h RoutingHint) Validate() error {
	if h.TicketType != "" && !h.TicketType.IsValid() {
		return fmt.Errorf("invalid routing hint ticket_type: %s", h.TicketType)
	}
	if strings.TrimSpace(h.Capability) == "" && strings.TrimSpace(h.PreferredAgentID) == "" {
		return fmt.Errorf("routing hint requires capability or preferred_agent_id")
	}
	return nil
}

type GateTemplate struct {
	Kind            GateKind  `json:"kind" yaml:"kind" toml:"kind"`
	RequiredRole    AgentRole `json:"required_role,omitempty" yaml:"required_role,omitempty" toml:"required_role"`
	RequiredAgentID string    `json:"required_agent_id,omitempty" yaml:"required_agent_id,omitempty" toml:"required_agent_id"`
}

func (t GateTemplate) Validate() error {
	if !t.Kind.IsValid() {
		return fmt.Errorf("invalid gate template kind: %s", t.Kind)
	}
	if t.RequiredRole != "" && !t.RequiredRole.IsValid() {
		return fmt.Errorf("invalid gate template required_role: %s", t.RequiredRole)
	}
	return nil
}

type ExecutionSafety struct {
	RequireCleanRepo bool `json:"require_clean_repo,omitempty" yaml:"require_clean_repo,omitempty" toml:"require_clean_repo"`
}

func (c ExecutionSafety) Validate() error { return nil }

type AgentProfile struct {
	AgentID             string        `json:"agent_id" yaml:"agent_id" toml:"agent_id"`
	DisplayName         string        `json:"display_name" yaml:"display_name" toml:"display_name"`
	Provider            AgentProvider `json:"provider" yaml:"provider" toml:"provider"`
	Enabled             bool          `json:"enabled" yaml:"enabled" toml:"enabled"`
	Capabilities        []string      `json:"capabilities,omitempty" yaml:"capabilities,omitempty" toml:"capabilities"`
	AllowedTicketTypes  []TicketType  `json:"allowed_ticket_types,omitempty" yaml:"allowed_ticket_types,omitempty" toml:"allowed_ticket_types"`
	DefaultRunbook      string        `json:"default_runbook,omitempty" yaml:"default_runbook,omitempty" toml:"default_runbook"`
	MaxActiveRuns       int           `json:"max_active_runs,omitempty" yaml:"max_active_runs,omitempty" toml:"max_active_runs"`
	PreferredRoles      []AgentRole   `json:"preferred_roles,omitempty" yaml:"preferred_roles,omitempty" toml:"preferred_roles"`
	RoutingWeight       int           `json:"routing_weight,omitempty" yaml:"routing_weight,omitempty" toml:"routing_weight"`
	InstructionProfile  string        `json:"instruction_profile,omitempty" yaml:"instruction_profile,omitempty" toml:"instruction_profile"`
	LaunchTarget        string        `json:"launch_target,omitempty" yaml:"launch_target,omitempty" toml:"launch_target"`
	IntegrationTemplate string        `json:"integration_template,omitempty" yaml:"integration_template,omitempty" toml:"integration_template"`
	Notes               string        `json:"notes,omitempty" yaml:"notes,omitempty" toml:"notes"`
}

func (p AgentProfile) Validate() error {
	if strings.TrimSpace(p.AgentID) == "" {
		return fmt.Errorf("agent_id is required")
	}
	if strings.TrimSpace(p.DisplayName) == "" {
		return fmt.Errorf("display_name is required")
	}
	if !p.Provider.IsValid() {
		return fmt.Errorf("invalid agent provider: %s", p.Provider)
	}
	if p.MaxActiveRuns < 0 {
		return fmt.Errorf("max_active_runs must be >= 0")
	}
	for _, capability := range p.Capabilities {
		if strings.TrimSpace(capability) == "" {
			return fmt.Errorf("capabilities cannot contain blanks")
		}
	}
	for _, kind := range p.AllowedTicketTypes {
		if !kind.IsValid() {
			return fmt.Errorf("invalid allowed_ticket_type: %s", kind)
		}
	}
	for _, role := range p.PreferredRoles {
		if !role.IsValid() {
			return fmt.Errorf("invalid preferred_role: %s", role)
		}
	}
	return nil
}

type RunSnapshot struct {
	RunID           string        `json:"run_id" yaml:"run_id"`
	TicketID        string        `json:"ticket_id" yaml:"ticket_id"`
	Project         string        `json:"project" yaml:"project"`
	AgentID         string        `json:"agent_id,omitempty" yaml:"agent_id,omitempty"`
	Provider        AgentProvider `json:"provider,omitempty" yaml:"provider,omitempty"`
	Status          RunStatus     `json:"status" yaml:"status"`
	Kind            RunKind       `json:"kind" yaml:"kind"`
	BlueprintStage  string        `json:"blueprint_stage,omitempty" yaml:"blueprint_stage,omitempty"`
	WorktreePath    string        `json:"worktree_path,omitempty" yaml:"worktree_path,omitempty"`
	BranchName      string        `json:"branch_name,omitempty" yaml:"branch_name,omitempty"`
	CreatedAt       time.Time     `json:"created_at" yaml:"created_at"`
	StartedAt       time.Time     `json:"started_at,omitempty" yaml:"started_at,omitempty"`
	CompletedAt     time.Time     `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
	LastHeartbeatAt time.Time     `json:"last_heartbeat_at,omitempty" yaml:"last_heartbeat_at,omitempty"`
	Result          string        `json:"result,omitempty" yaml:"result,omitempty"`
	Summary         string        `json:"summary,omitempty" yaml:"summary,omitempty"`
	HandoffTo       string        `json:"handoff_to,omitempty" yaml:"handoff_to,omitempty"`
	SupersedesRunID string        `json:"supersedes_run_id,omitempty" yaml:"supersedes_run_id,omitempty"`
	EvidenceCount   int           `json:"evidence_count,omitempty" yaml:"evidence_count,omitempty"`
	SchemaVersion   int           `json:"schema_version" yaml:"schema_version"`
	SessionProvider AgentProvider `json:"session_provider,omitempty" yaml:"session_provider,omitempty"`
	SessionRef      string        `json:"session_ref,omitempty" yaml:"session_ref,omitempty"`
}

func (r RunSnapshot) Validate() error {
	if strings.TrimSpace(r.RunID) == "" {
		return fmt.Errorf("run_id is required")
	}
	if strings.TrimSpace(r.TicketID) == "" {
		return fmt.Errorf("ticket_id is required")
	}
	if strings.TrimSpace(r.Project) == "" {
		return fmt.Errorf("project is required")
	}
	if !r.Status.IsValid() {
		return fmt.Errorf("invalid run status: %s", r.Status)
	}
	if !r.Kind.IsValid() {
		return fmt.Errorf("invalid run kind: %s", r.Kind)
	}
	if r.Provider != "" && !r.Provider.IsValid() {
		return fmt.Errorf("invalid run provider: %s", r.Provider)
	}
	if r.SessionProvider != "" && !r.SessionProvider.IsValid() {
		return fmt.Errorf("invalid session provider: %s", r.SessionProvider)
	}
	return nil
}

type RunbookStage struct {
	Key                   string         `json:"key" yaml:"key" toml:"key"`
	DisplayName           string         `json:"display_name" yaml:"display_name" toml:"display_name"`
	ExpectedRole          AgentRole      `json:"expected_role,omitempty" yaml:"expected_role,omitempty" toml:"expected_role"`
	RequiredCapabilities  []string       `json:"required_capabilities,omitempty" yaml:"required_capabilities,omitempty" toml:"required_capabilities"`
	RequiredEvidenceTypes []EvidenceType `json:"required_evidence_types,omitempty" yaml:"required_evidence_types,omitempty" toml:"required_evidence_types"`
	RequiredGates         []GateKind     `json:"required_gates,omitempty" yaml:"required_gates,omitempty" toml:"required_gates"`
	SuggestedTicketStatus Status         `json:"suggested_ticket_status,omitempty" yaml:"suggested_ticket_status,omitempty" toml:"suggested_ticket_status"`
	CompletionCriteria    []string       `json:"completion_criteria,omitempty" yaml:"completion_criteria,omitempty" toml:"completion_criteria"`
	SeparateRunRequired   bool           `json:"separate_run_required,omitempty" yaml:"separate_run_required,omitempty" toml:"separate_run_required"`
}

func (s RunbookStage) Validate() error {
	if strings.TrimSpace(s.Key) == "" {
		return fmt.Errorf("runbook stage key is required")
	}
	if strings.TrimSpace(s.DisplayName) == "" {
		return fmt.Errorf("runbook stage display_name is required")
	}
	if s.ExpectedRole != "" && !s.ExpectedRole.IsValid() {
		return fmt.Errorf("invalid expected_role: %s", s.ExpectedRole)
	}
	if s.SuggestedTicketStatus != "" && !s.SuggestedTicketStatus.IsValid() {
		return fmt.Errorf("invalid suggested_ticket_status: %s", s.SuggestedTicketStatus)
	}
	for _, capability := range s.RequiredCapabilities {
		if strings.TrimSpace(capability) == "" {
			return fmt.Errorf("required_capabilities cannot contain blanks")
		}
	}
	for _, evidenceType := range s.RequiredEvidenceTypes {
		if !evidenceType.IsValid() {
			return fmt.Errorf("invalid required_evidence_type: %s", evidenceType)
		}
	}
	for _, gateKind := range s.RequiredGates {
		if !gateKind.IsValid() {
			return fmt.Errorf("invalid required_gate: %s", gateKind)
		}
	}
	return nil
}

type Runbook struct {
	Name                 string         `json:"name" yaml:"name" toml:"name"`
	DisplayName          string         `json:"display_name" yaml:"display_name" toml:"display_name"`
	AppliesToTicketTypes []TicketType   `json:"applies_to_ticket_types,omitempty" yaml:"applies_to_ticket_types,omitempty" toml:"applies_to_ticket_types"`
	DefaultInitialStage  string         `json:"default_initial_stage,omitempty" yaml:"default_initial_stage,omitempty" toml:"default_initial_stage"`
	HandoffTemplate      string         `json:"handoff_template,omitempty" yaml:"handoff_template,omitempty" toml:"handoff_template"`
	Stages               []RunbookStage `json:"stages" yaml:"stages" toml:"stages"`
}

func (r Runbook) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("runbook name is required")
	}
	if strings.TrimSpace(r.DisplayName) == "" {
		return fmt.Errorf("runbook display_name is required")
	}
	if len(r.Stages) == 0 {
		return fmt.Errorf("runbook requires at least one stage")
	}
	for _, kind := range r.AppliesToTicketTypes {
		if !kind.IsValid() {
			return fmt.Errorf("invalid runbook applies_to_ticket_type: %s", kind)
		}
	}
	for _, stage := range r.Stages {
		if err := stage.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type EvidenceItem struct {
	EvidenceID           string       `json:"evidence_id" yaml:"evidence_id"`
	RunID                string       `json:"run_id" yaml:"run_id"`
	TicketID             string       `json:"ticket_id" yaml:"ticket_id"`
	Type                 EvidenceType `json:"type" yaml:"type"`
	Title                string       `json:"title,omitempty" yaml:"title,omitempty"`
	Body                 string       `json:"body,omitempty" yaml:"body,omitempty"`
	ArtifactPath         string       `json:"artifact_path,omitempty" yaml:"artifact_path,omitempty"`
	SupersedesEvidenceID string       `json:"supersedes_evidence_id,omitempty" yaml:"supersedes_evidence_id,omitempty"`
	Actor                Actor        `json:"actor" yaml:"actor"`
	CreatedAt            time.Time    `json:"created_at" yaml:"created_at"`
	SchemaVersion        int          `json:"schema_version" yaml:"schema_version"`
}

func (e EvidenceItem) Validate() error {
	if strings.TrimSpace(e.EvidenceID) == "" {
		return fmt.Errorf("evidence_id is required")
	}
	if strings.TrimSpace(e.RunID) == "" {
		return fmt.Errorf("run_id is required")
	}
	if strings.TrimSpace(e.TicketID) == "" {
		return fmt.Errorf("ticket_id is required")
	}
	if !e.Type.IsValid() {
		return fmt.Errorf("invalid evidence type: %s", e.Type)
	}
	if !e.Actor.IsValid() {
		return fmt.Errorf("invalid evidence actor: %s", e.Actor)
	}
	if len(e.Body) > 64*1024 {
		return fmt.Errorf("evidence body exceeds 64 KiB")
	}
	return nil
}

type HandoffPacket struct {
	HandoffID                 string    `json:"handoff_id" yaml:"handoff_id"`
	SourceRunID               string    `json:"source_run_id" yaml:"source_run_id"`
	TicketID                  string    `json:"ticket_id" yaml:"ticket_id"`
	Actor                     Actor     `json:"actor" yaml:"actor"`
	StatusSummary             string    `json:"status_summary,omitempty" yaml:"status_summary,omitempty"`
	ChangedFiles              []string  `json:"changed_files,omitempty" yaml:"changed_files,omitempty"`
	CommitRefs                []string  `json:"commit_refs,omitempty" yaml:"commit_refs,omitempty"`
	Tests                     []string  `json:"tests,omitempty" yaml:"tests,omitempty"`
	EvidenceLinks             []string  `json:"evidence_links,omitempty" yaml:"evidence_links,omitempty"`
	OpenQuestions             []string  `json:"open_questions,omitempty" yaml:"open_questions,omitempty"`
	Risks                     []string  `json:"risks,omitempty" yaml:"risks,omitempty"`
	SuggestedNextActor        string    `json:"suggested_next_actor,omitempty" yaml:"suggested_next_actor,omitempty"`
	SuggestedNextGate         GateKind  `json:"suggested_next_gate,omitempty" yaml:"suggested_next_gate,omitempty"`
	SuggestedNextTicketStatus Status    `json:"suggested_next_ticket_status,omitempty" yaml:"suggested_next_ticket_status,omitempty"`
	GeneratedAt               time.Time `json:"generated_at" yaml:"generated_at"`
	SchemaVersion             int       `json:"schema_version" yaml:"schema_version"`
}

func (h HandoffPacket) Validate() error {
	if strings.TrimSpace(h.HandoffID) == "" {
		return fmt.Errorf("handoff_id is required")
	}
	if strings.TrimSpace(h.SourceRunID) == "" {
		return fmt.Errorf("source_run_id is required")
	}
	if strings.TrimSpace(h.TicketID) == "" {
		return fmt.Errorf("ticket_id is required")
	}
	if !h.Actor.IsValid() {
		return fmt.Errorf("invalid handoff actor: %s", h.Actor)
	}
	if h.SuggestedNextGate != "" && !h.SuggestedNextGate.IsValid() {
		return fmt.Errorf("invalid suggested_next_gate: %s", h.SuggestedNextGate)
	}
	if h.SuggestedNextTicketStatus != "" && !h.SuggestedNextTicketStatus.IsValid() {
		return fmt.Errorf("invalid suggested_next_ticket_status: %s", h.SuggestedNextTicketStatus)
	}
	return nil
}

type GateSnapshot struct {
	GateID               string         `json:"gate_id" yaml:"gate_id"`
	TicketID             string         `json:"ticket_id" yaml:"ticket_id"`
	RunID                string         `json:"run_id,omitempty" yaml:"run_id,omitempty"`
	Kind                 GateKind       `json:"kind" yaml:"kind"`
	State                GateState      `json:"state" yaml:"state"`
	RequiredRole         AgentRole      `json:"required_role,omitempty" yaml:"required_role,omitempty"`
	RequiredAgentID      string         `json:"required_agent_id,omitempty" yaml:"required_agent_id,omitempty"`
	CreatedBy            Actor          `json:"created_by" yaml:"created_by"`
	DecidedBy            Actor          `json:"decided_by,omitempty" yaml:"decided_by,omitempty"`
	DecisionReason       string         `json:"decision_reason,omitempty" yaml:"decision_reason,omitempty"`
	EvidenceRequirements []EvidenceType `json:"evidence_requirements,omitempty" yaml:"evidence_requirements,omitempty"`
	RelatedRunIDs        []string       `json:"related_run_ids,omitempty" yaml:"related_run_ids,omitempty"`
	ReplacesGateID       string         `json:"replaces_gate_id,omitempty" yaml:"replaces_gate_id,omitempty"`
	CreatedAt            time.Time      `json:"created_at" yaml:"created_at"`
	DecidedAt            time.Time      `json:"decided_at,omitempty" yaml:"decided_at,omitempty"`
	SchemaVersion        int            `json:"schema_version" yaml:"schema_version"`
}

func (g GateSnapshot) Validate() error {
	if strings.TrimSpace(g.GateID) == "" {
		return fmt.Errorf("gate_id is required")
	}
	if strings.TrimSpace(g.TicketID) == "" {
		return fmt.Errorf("ticket_id is required")
	}
	if !g.Kind.IsValid() {
		return fmt.Errorf("invalid gate kind: %s", g.Kind)
	}
	if !g.State.IsValid() {
		return fmt.Errorf("invalid gate state: %s", g.State)
	}
	if !g.CreatedBy.IsValid() {
		return fmt.Errorf("invalid gate created_by: %s", g.CreatedBy)
	}
	if g.DecidedBy != "" && !g.DecidedBy.IsValid() {
		return fmt.Errorf("invalid gate decided_by: %s", g.DecidedBy)
	}
	if g.RequiredRole != "" && !g.RequiredRole.IsValid() {
		return fmt.Errorf("invalid gate required_role: %s", g.RequiredRole)
	}
	for _, evidenceType := range g.EvidenceRequirements {
		if !evidenceType.IsValid() {
			return fmt.Errorf("invalid gate evidence requirement: %s", evidenceType)
		}
	}
	return nil
}
