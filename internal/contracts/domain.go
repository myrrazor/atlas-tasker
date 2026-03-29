package contracts

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	SchemaVersionV1      = 1
	SchemaVersionV2      = 2
	SchemaVersionV3      = 3
	SchemaVersionV4      = 4
	SchemaVersionV5      = 5
	CurrentSchemaVersion = SchemaVersionV5
	DefaultLeaseTTL      = 60 * time.Minute
)

// TicketType is the canonical issue type for v1.
type TicketType string

const (
	TicketTypeEpic    TicketType = "epic"
	TicketTypeTask    TicketType = "task"
	TicketTypeBug     TicketType = "bug"
	TicketTypeSubtask TicketType = "subtask"
)

// Status is the workflow state stored in markdown and projection.
type Status string

const (
	StatusBacklog    Status = "backlog"
	StatusReady      Status = "ready"
	StatusInProgress Status = "in_progress"
	StatusInReview   Status = "in_review"
	StatusBlocked    Status = "blocked"
	StatusDone       Status = "done"
	StatusCanceled   Status = "canceled"
)

// Priority is the urgency marker for sorting and triage.
type Priority string

const (
	PriorityLow      Priority = "low"
	PriorityMedium   Priority = "medium"
	PriorityHigh     Priority = "high"
	PriorityCritical Priority = "critical"
)

// CompletionMode defines who can move in_review to done.
type CompletionMode string

const (
	CompletionModeOpen       CompletionMode = "open"
	CompletionModeOwnerGate  CompletionMode = "owner_gate"
	CompletionModeReviewGate CompletionMode = "review_gate"
	CompletionModeDualGate   CompletionMode = "dual_gate"
)

// ReviewState captures explicit review workflow state.
type ReviewState string

const (
	ReviewStateNone             ReviewState = "none"
	ReviewStatePending          ReviewState = "pending"
	ReviewStateApproved         ReviewState = "approved"
	ReviewStateChangesRequested ReviewState = "changes_requested"
)

// LeaseKind distinguishes work leases from review leases.
type LeaseKind string

const (
	LeaseKindNone   LeaseKind = ""
	LeaseKindWork   LeaseKind = "work"
	LeaseKindReview LeaseKind = "review"
)

// Actor is the mutation identity format (e.g. human:owner, agent:builder-1).
type Actor string

var validTicketTypes = map[TicketType]struct{}{
	TicketTypeEpic: {}, TicketTypeTask: {}, TicketTypeBug: {}, TicketTypeSubtask: {},
}

var validStatuses = map[Status]struct{}{
	StatusBacklog: {}, StatusReady: {}, StatusInProgress: {}, StatusInReview: {},
	StatusBlocked: {}, StatusDone: {}, StatusCanceled: {},
}

var validPriorities = map[Priority]struct{}{
	PriorityLow: {}, PriorityMedium: {}, PriorityHigh: {}, PriorityCritical: {},
}

var validCompletionModes = map[CompletionMode]struct{}{
	CompletionModeOpen: {}, CompletionModeOwnerGate: {}, CompletionModeReviewGate: {}, CompletionModeDualGate: {},
}

var validReviewStates = map[ReviewState]struct{}{
	ReviewStateNone: {}, ReviewStatePending: {}, ReviewStateApproved: {}, ReviewStateChangesRequested: {},
}

var validLeaseKinds = map[LeaseKind]struct{}{
	LeaseKindNone: {}, LeaseKindWork: {}, LeaseKindReview: {},
}

func (t TicketType) IsValid() bool {
	_, ok := validTicketTypes[t]
	return ok
}

func (s Status) IsValid() bool {
	_, ok := validStatuses[s]
	return ok
}

func (p Priority) IsValid() bool {
	_, ok := validPriorities[p]
	return ok
}

func (m CompletionMode) IsValid() bool {
	_, ok := validCompletionModes[m]
	return ok
}

func (r ReviewState) IsValid() bool {
	if r == "" {
		return true
	}
	_, ok := validReviewStates[r]
	return ok
}

func (k LeaseKind) IsValid() bool {
	_, ok := validLeaseKinds[k]
	return ok
}

func (a Actor) IsValid() bool {
	parts := strings.SplitN(string(a), ":", 2)
	if len(parts) != 2 {
		return false
	}
	if parts[0] != "human" && parts[0] != "agent" {
		return false
	}
	return strings.TrimSpace(parts[1]) != ""
}

// Project represents a tracked project namespace.
type Project struct {
	Key           string          `json:"key"`
	Name          string          `json:"name"`
	CreatedAt     time.Time       `json:"created_at"`
	SchemaVersion int             `json:"schema_version"`
	Defaults      ProjectDefaults `json:"defaults"`
}

func (p Project) Validate() error {
	if strings.TrimSpace(p.Key) == "" {
		return fmt.Errorf("project key is required")
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("project name is required")
	}
	if p.SchemaVersion != 0 && p.SchemaVersion != SchemaVersionV1 && p.SchemaVersion != SchemaVersionV2 && p.SchemaVersion != SchemaVersionV3 && p.SchemaVersion != SchemaVersionV4 && p.SchemaVersion != SchemaVersionV5 {
		return fmt.Errorf("invalid project schema version: %d", p.SchemaVersion)
	}
	if err := p.Defaults.Validate(); err != nil {
		return err
	}
	return nil
}

// WorkflowConfig captures project/workspace workflow policy.
type WorkflowConfig struct {
	CompletionMode CompletionMode `json:"completion_mode"`
}

func (w WorkflowConfig) Validate() error {
	if !w.CompletionMode.IsValid() {
		return fmt.Errorf("invalid completion mode: %s", w.CompletionMode)
	}
	return nil
}

// TrackerConfig defines top-level runtime config contracts.
type TrackerConfig struct {
	Workflow      WorkflowConfig      `json:"workflow"`
	Actor         ActorConfig         `json:"actor"`
	Notifications NotificationsConfig `json:"notifications"`
	Provider      ProviderConfig      `json:"provider,omitempty"`
	ImportExport  ImportExportConfig  `json:"import_export,omitempty"`
	Release       ReleaseConfig       `json:"release,omitempty"`
}

func (c TrackerConfig) Validate() error {
	if err := c.Workflow.Validate(); err != nil {
		return err
	}
	if err := c.Actor.Validate(); err != nil {
		return err
	}
	if err := c.Notifications.Validate(); err != nil {
		return err
	}
	if err := c.Provider.Validate(); err != nil {
		return err
	}
	if err := c.ImportExport.Validate(); err != nil {
		return err
	}
	return c.Release.Validate()
}

// ActorConfig holds local actor defaults for CLI/TUI convenience.
type ActorConfig struct {
	Default Actor `json:"default,omitempty"`
}

func (c ActorConfig) Validate() error {
	if c.Default != "" && !c.Default.IsValid() {
		return fmt.Errorf("invalid default actor: %s", c.Default)
	}
	return nil
}

// NotificationsConfig controls the built-in v1.2 notifier sinks.
const (
	DefaultWebhookTimeoutSeconds = 3
	MaxWebhookTimeoutSeconds     = 30
	MaxWebhookRetries            = 5
)

type NotificationsConfig struct {
	Terminal              bool   `json:"terminal"`
	FileEnabled           bool   `json:"file_enabled,omitempty"`
	FilePath              string `json:"file_path,omitempty"`
	WebhookURL            string `json:"webhook_url,omitempty"`
	WebhookTimeoutSeconds int    `json:"webhook_timeout_seconds,omitempty"`
	WebhookRetries        int    `json:"webhook_retries,omitempty"`
	DeliveryLogPath       string `json:"delivery_log_path,omitempty"`
	DeadLetterPath        string `json:"dead_letter_path,omitempty"`
}

func (c NotificationsConfig) Validate() error {
	if c.FileEnabled && strings.TrimSpace(c.FilePath) == "" {
		return fmt.Errorf("notifications.file_path is required when file notifications are enabled")
	}
	if strings.TrimSpace(c.WebhookURL) != "" {
		if _, err := url.ParseRequestURI(strings.TrimSpace(c.WebhookURL)); err != nil {
			return fmt.Errorf("invalid notifications.webhook_url: %w", err)
		}
	}
	if c.WebhookTimeoutSeconds < 0 || c.WebhookTimeoutSeconds > MaxWebhookTimeoutSeconds {
		return fmt.Errorf("notifications.webhook_timeout_seconds must be between 0 and %d", MaxWebhookTimeoutSeconds)
	}
	if c.WebhookRetries < 0 || c.WebhookRetries > MaxWebhookRetries {
		return fmt.Errorf("notifications.webhook_retries must be between 0 and %d", MaxWebhookRetries)
	}
	if strings.TrimSpace(c.DeliveryLogPath) == "" {
		return fmt.Errorf("notifications.delivery_log_path is required")
	}
	if strings.TrimSpace(c.DeadLetterPath) == "" {
		return fmt.Errorf("notifications.dead_letter_path is required")
	}
	return nil
}

type ProviderConfig struct {
	DefaultSCMProvider ChangeProvider `json:"default_scm_provider,omitempty"`
	DefaultBaseBranch  string         `json:"default_base_branch,omitempty"`
	GitHubRepo         string         `json:"github_repo,omitempty"`
}

func (c ProviderConfig) Validate() error {
	if c.DefaultSCMProvider != "" && !c.DefaultSCMProvider.IsValid() {
		return fmt.Errorf("invalid provider.default_scm_provider: %s", c.DefaultSCMProvider)
	}
	return nil
}

type ImportExportConfig struct {
	MaxBundleSizeMB     int  `json:"max_bundle_size_mb,omitempty"`
	RequireVerification bool `json:"require_verification,omitempty"`
	AllowUpdateExisting bool `json:"allow_update_existing,omitempty"`
}

func (c ImportExportConfig) Validate() error {
	if c.MaxBundleSizeMB < 0 {
		return fmt.Errorf("import_export.max_bundle_size_mb must be >= 0")
	}
	return nil
}

type ReleaseConfig struct {
	BaseMarker         string `json:"base_marker,omitempty"`
	BaseSHA            string `json:"base_sha,omitempty"`
	VerifyChecksums    bool   `json:"verify_checksums,omitempty"`
	VerifyAttestations bool   `json:"verify_attestations,omitempty"`
}

func (c ReleaseConfig) Validate() error { return nil }

// ProjectDefaults captures project-level policy defaults introduced in v1.2.
type ProjectDefaults struct {
	CompletionMode      CompletionMode  `json:"completion_mode,omitempty"`
	LeaseTTLMinutes     int             `json:"lease_ttl_minutes,omitempty"`
	AllowedWorkers      []Actor         `json:"allowed_workers,omitempty"`
	RequiredReviewer    Actor           `json:"required_reviewer,omitempty"`
	TemplatesPath       string          `json:"templates_path,omitempty"`
	HooksEnabled        bool            `json:"hooks_enabled,omitempty"`
	Worktrees           WorktreeConfig  `json:"worktrees,omitempty"`
	RunbookMappings     []RunbookMap    `json:"runbook_mappings,omitempty"`
	RoutingHints        []RoutingHint   `json:"routing_hints,omitempty"`
	GateTemplates       []GateTemplate  `json:"gate_templates,omitempty"`
	ExecutionSafety     ExecutionSafety `json:"execution_safety,omitempty"`
	PermissionProfiles  []string        `json:"permission_profiles,omitempty"`
	SCMProvider         ChangeProvider  `json:"scm_provider,omitempty"`
	SCMBaseBranch       string          `json:"scm_base_branch,omitempty"`
	SCMRepo             string          `json:"scm_repo,omitempty"`
	RetentionPolicies   []string        `json:"retention_policies,omitempty"`
	ImportTemplate      string          `json:"import_template,omitempty"`
	ReleaseVerification string          `json:"release_verification,omitempty"`
}

func (p ProjectDefaults) Validate() error {
	if p.CompletionMode != "" && !p.CompletionMode.IsValid() {
		return fmt.Errorf("invalid project completion mode: %s", p.CompletionMode)
	}
	if p.LeaseTTLMinutes < 0 {
		return fmt.Errorf("lease_ttl_minutes must be >= 0")
	}
	if err := validateActors(p.AllowedWorkers); err != nil {
		return fmt.Errorf("invalid allowed_workers: %w", err)
	}
	if p.RequiredReviewer != "" && !p.RequiredReviewer.IsValid() {
		return fmt.Errorf("invalid required reviewer: %s", p.RequiredReviewer)
	}
	if err := p.Worktrees.Validate(); err != nil {
		return err
	}
	for _, mapping := range p.RunbookMappings {
		if err := mapping.Validate(); err != nil {
			return err
		}
	}
	for _, hint := range p.RoutingHints {
		if err := hint.Validate(); err != nil {
			return err
		}
	}
	for _, template := range p.GateTemplates {
		if err := template.Validate(); err != nil {
			return err
		}
	}
	if p.SCMProvider != "" && !p.SCMProvider.IsValid() {
		return fmt.Errorf("invalid project scm_provider: %s", p.SCMProvider)
	}
	if err := p.ExecutionSafety.Validate(); err != nil {
		return err
	}
	return nil
}

// TicketPolicy stores ticket or epic-level overrides for work policy.
type TicketPolicy struct {
	Inherit          bool           `json:"inherit"`
	CompletionMode   CompletionMode `json:"completion_mode,omitempty"`
	AllowedWorkers   []Actor        `json:"allowed_workers,omitempty"`
	RequiredReviewer Actor          `json:"required_reviewer,omitempty"`
	OwnerOverride    bool           `json:"owner_override,omitempty"`
}

func (p TicketPolicy) Validate() error {
	if p.CompletionMode != "" && !p.CompletionMode.IsValid() {
		return fmt.Errorf("invalid ticket completion mode: %s", p.CompletionMode)
	}
	if err := validateActors(p.AllowedWorkers); err != nil {
		return fmt.Errorf("invalid allowed_workers: %w", err)
	}
	if p.RequiredReviewer != "" && !p.RequiredReviewer.IsValid() {
		return fmt.Errorf("invalid required reviewer: %s", p.RequiredReviewer)
	}
	return nil
}

func (p TicketPolicy) HasOverrides() bool {
	return p.CompletionMode != "" || len(p.AllowedWorkers) > 0 || p.RequiredReviewer != "" || p.OwnerOverride
}

// LeaseState stores active work/review ownership for a ticket.
type LeaseState struct {
	Actor           Actor     `json:"actor,omitempty"`
	Kind            LeaseKind `json:"kind,omitempty"`
	AcquiredAt      time.Time `json:"acquired_at,omitempty"`
	ExpiresAt       time.Time `json:"expires_at,omitempty"`
	LastHeartbeatAt time.Time `json:"last_heartbeat_at,omitempty"`
}

func (l LeaseState) Validate() error {
	if !l.Kind.IsValid() {
		return fmt.Errorf("invalid lease kind: %s", l.Kind)
	}
	if l.Kind == LeaseKindNone {
		if l.Actor != "" || !l.AcquiredAt.IsZero() || !l.ExpiresAt.IsZero() || !l.LastHeartbeatAt.IsZero() {
			return fmt.Errorf("empty lease kind cannot carry active lease fields")
		}
		return nil
	}
	if !l.Actor.IsValid() {
		return fmt.Errorf("invalid lease actor: %s", l.Actor)
	}
	if l.AcquiredAt.IsZero() {
		return fmt.Errorf("lease acquired_at is required")
	}
	if l.ExpiresAt.IsZero() {
		return fmt.Errorf("lease expires_at is required")
	}
	if l.ExpiresAt.Before(l.AcquiredAt) {
		return fmt.Errorf("lease expires_at must be >= acquired_at")
	}
	if !l.LastHeartbeatAt.IsZero() && l.LastHeartbeatAt.Before(l.AcquiredAt) {
		return fmt.Errorf("last heartbeat must be >= acquired_at")
	}
	return nil
}

func (l LeaseState) Active(now time.Time) bool {
	if l.Kind == LeaseKindNone || l.Actor == "" {
		return false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return !l.ExpiresAt.IsZero() && now.Before(l.ExpiresAt)
}

// ProgressSummary holds derived rollup information for parent tickets.
type ProgressSummary struct {
	TotalChildren   int `json:"total_children,omitempty"`
	DoneChildren    int `json:"done_children,omitempty"`
	BlockedChildren int `json:"blocked_children,omitempty"`
	Percent         int `json:"percent,omitempty"`
}

func (p ProgressSummary) Validate() error {
	if p.TotalChildren < 0 || p.DoneChildren < 0 || p.BlockedChildren < 0 {
		return fmt.Errorf("progress counters must be >= 0")
	}
	if p.DoneChildren > p.TotalChildren {
		return fmt.Errorf("done_children cannot exceed total_children")
	}
	if p.BlockedChildren > p.TotalChildren {
		return fmt.Errorf("blocked_children cannot exceed total_children")
	}
	if p.Percent < 0 || p.Percent > 100 {
		return fmt.Errorf("progress percent must be between 0 and 100")
	}
	return nil
}

// TicketSnapshot mirrors v1 ticket markdown frontmatter plus body sections.
type TicketSnapshot struct {
	ID                   string           `json:"id"`
	TicketUID            string           `json:"ticket_uid,omitempty"`
	Project              string           `json:"project"`
	Title                string           `json:"title"`
	Type                 TicketType       `json:"type"`
	Status               Status           `json:"status"`
	Priority             Priority         `json:"priority"`
	Parent               string           `json:"parent,omitempty"`
	Labels               []string         `json:"labels"`
	Assignee             Actor            `json:"assignee,omitempty"`
	Reviewer             Actor            `json:"reviewer,omitempty"`
	BlockedBy            []string         `json:"blocked_by"`
	Blocks               []string         `json:"blocks"`
	CreatedAt            time.Time        `json:"created_at"`
	UpdatedAt            time.Time        `json:"updated_at"`
	SchemaVersion        int              `json:"schema_version"`
	Archived             bool             `json:"archived"`
	Policy               TicketPolicy     `json:"policy,omitempty"`
	ReviewState          ReviewState      `json:"review_state,omitempty"`
	Lease                LeaseState       `json:"lease,omitempty"`
	Template             string           `json:"template,omitempty"`
	SkillHint            string           `json:"skill_hint,omitempty"`
	Blueprint            string           `json:"blueprint,omitempty"`
	Progress             ProgressSummary  `json:"progress,omitempty"`
	RequiredCapabilities []string         `json:"required_capabilities,omitempty"`
	DispatchMode         DispatchMode     `json:"dispatch_mode,omitempty"`
	AllowParallelRuns    bool             `json:"allow_parallel_runs,omitempty"`
	Runbook              string           `json:"runbook,omitempty"`
	LatestRunID          string           `json:"latest_run_id,omitempty"`
	LatestHandoffID      string           `json:"latest_handoff_id,omitempty"`
	OpenGateIDs          []string         `json:"open_gate_ids,omitempty"`
	LastDispatchAt       time.Time        `json:"last_dispatch_at,omitempty"`
	ChangeIDs            []string         `json:"change_ids,omitempty"`
	ChangeReadyState     ChangeReadyState `json:"change_ready_state,omitempty"`
	ChangeReadyReasons   []string         `json:"change_ready_reasons,omitempty"`
	PermissionProfiles   []string         `json:"permission_profiles,omitempty"`
	Protected            bool             `json:"protected,omitempty"`
	Sensitive            bool             `json:"sensitive,omitempty"`

	Summary            string   `json:"summary,omitempty"`
	Description        string   `json:"description,omitempty"`
	AcceptanceCriteria []string `json:"acceptance_criteria,omitempty"`
	Notes              string   `json:"notes,omitempty"`
}

func (t TicketSnapshot) ValidateForCreate() error {
	t = NormalizeTicketSnapshot(t)
	if strings.TrimSpace(t.ID) == "" {
		return fmt.Errorf("ticket id is required")
	}
	if strings.TrimSpace(t.Project) == "" {
		return fmt.Errorf("project is required")
	}
	if strings.TrimSpace(t.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if !t.Type.IsValid() {
		return fmt.Errorf("invalid ticket type: %s", t.Type)
	}
	if !t.Status.IsValid() {
		return fmt.Errorf("invalid status: %s", t.Status)
	}
	if !t.Priority.IsValid() {
		return fmt.Errorf("invalid priority: %s", t.Priority)
	}
	if t.Assignee != "" && !t.Assignee.IsValid() {
		return fmt.Errorf("invalid assignee actor: %s", t.Assignee)
	}
	if t.Reviewer != "" && !t.Reviewer.IsValid() {
		return fmt.Errorf("invalid reviewer actor: %s", t.Reviewer)
	}
	if t.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("schema_version must be %d", CurrentSchemaVersion)
	}
	if t.CreatedAt.IsZero() {
		return fmt.Errorf("created_at is required")
	}
	if t.UpdatedAt.IsZero() {
		return fmt.Errorf("updated_at is required")
	}
	if err := t.Policy.Validate(); err != nil {
		return err
	}
	if !t.ReviewState.IsValid() {
		return fmt.Errorf("invalid review state: %s", t.ReviewState)
	}
	if err := t.Lease.Validate(); err != nil {
		return err
	}
	if err := t.Progress.Validate(); err != nil {
		return err
	}
	if t.DispatchMode != "" && !t.DispatchMode.IsValid() {
		return fmt.Errorf("invalid dispatch mode: %s", t.DispatchMode)
	}
	if t.ChangeReadyState != "" && !t.ChangeReadyState.IsValid() {
		return fmt.Errorf("invalid change_ready_state: %s", t.ChangeReadyState)
	}
	for _, capability := range t.RequiredCapabilities {
		if strings.TrimSpace(capability) == "" {
			return fmt.Errorf("required_capabilities cannot contain blanks")
		}
	}
	return nil
}

func IsTerminalStatus(status Status) bool {
	return status == StatusDone || status == StatusCanceled
}

// BoardStatus returns the status bucket used by board-style views.
func BoardStatus(ticket TicketSnapshot) Status {
	if IsTerminalStatus(ticket.Status) {
		return StatusDone
	}
	if ticket.Status != StatusBlocked && len(ticket.BlockedBy) > 0 {
		return StatusBlocked
	}
	return ticket.Status
}

func NormalizeProject(project Project) Project {
	originalSchema := project.SchemaVersion
	if originalSchema == 0 {
		project.Defaults.CompletionMode = firstCompletionMode(project.Defaults.CompletionMode, CompletionModeOpen)
		project.SchemaVersion = CurrentSchemaVersion
	}
	if project.Defaults.LeaseTTLMinutes == 0 {
		project.Defaults.LeaseTTLMinutes = int(DefaultLeaseTTL / time.Minute)
	}
	if project.Defaults.Worktrees.DefaultMode == "" {
		project.Defaults.Worktrees.DefaultMode = WorktreeModePerRun
	}
	if project.SchemaVersion == 0 {
		project.SchemaVersion = SchemaVersionV1
	}
	if originalSchema != SchemaVersionV1 && project.SchemaVersion < CurrentSchemaVersion {
		project.SchemaVersion = CurrentSchemaVersion
	}
	return project
}

func NormalizeTicketSnapshot(ticket TicketSnapshot) TicketSnapshot {
	if ticket.SchemaVersion == 0 {
		ticket.SchemaVersion = SchemaVersionV1
	}
	if ticket.TicketUID == "" {
		ticket.TicketUID = TicketUID(ticket.Project, ticket.ID)
	}
	if ticket.Labels == nil {
		ticket.Labels = []string{}
	}
	if ticket.BlockedBy == nil {
		ticket.BlockedBy = []string{}
	}
	if ticket.Blocks == nil {
		ticket.Blocks = []string{}
	}
	if ticket.AcceptanceCriteria == nil {
		ticket.AcceptanceCriteria = []string{}
	}
	if ticket.RequiredCapabilities == nil {
		ticket.RequiredCapabilities = []string{}
	}
	if ticket.OpenGateIDs == nil {
		ticket.OpenGateIDs = []string{}
	}
	if ticket.ChangeIDs == nil {
		ticket.ChangeIDs = []string{}
	}
	if ticket.ChangeReadyReasons == nil {
		ticket.ChangeReadyReasons = []string{}
	}
	if ticket.PermissionProfiles == nil {
		ticket.PermissionProfiles = []string{}
	}
	if ticket.ReviewState == "" {
		ticket.ReviewState = ReviewStateNone
	}
	if ticket.DispatchMode == "" {
		ticket.DispatchMode = DispatchModeManual
	}
	if ticket.SchemaVersion < SchemaVersionV2 && !ticket.Policy.HasOverrides() {
		ticket.Policy.Inherit = true
	}
	if ticket.SchemaVersion < CurrentSchemaVersion {
		ticket.SchemaVersion = CurrentSchemaVersion
	}
	return ticket
}

func validateActors(actors []Actor) error {
	for _, actor := range actors {
		if !actor.IsValid() {
			return fmt.Errorf("%s", actor)
		}
	}
	return nil
}

func firstCompletionMode(values ...CompletionMode) CompletionMode {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
