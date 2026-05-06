package contracts

import (
	"fmt"
	"strings"
	"time"
)

type ChangeProvider string

const (
	ChangeProviderLocal  ChangeProvider = "local"
	ChangeProviderGitHub ChangeProvider = "github"
)

var validChangeProviders = map[ChangeProvider]struct{}{
	ChangeProviderLocal:  {},
	ChangeProviderGitHub: {},
}

func (p ChangeProvider) IsValid() bool {
	_, ok := validChangeProviders[p]
	return ok
}

type ChangeStatus string

const (
	ChangeStatusLocalOnly        ChangeStatus = "local_only"
	ChangeStatusDraft            ChangeStatus = "draft"
	ChangeStatusOpen             ChangeStatus = "open"
	ChangeStatusReviewRequested  ChangeStatus = "review_requested"
	ChangeStatusApproved         ChangeStatus = "approved"
	ChangeStatusChangesRequested ChangeStatus = "changes_requested"
	ChangeStatusMergeReady       ChangeStatus = "merge_ready"
	ChangeStatusMerged           ChangeStatus = "merged"
	ChangeStatusClosed           ChangeStatus = "closed"
	ChangeStatusSuperseded       ChangeStatus = "superseded"
	ChangeStatusExternalDrifted  ChangeStatus = "external_drifted"
)

var validChangeStatuses = map[ChangeStatus]struct{}{
	ChangeStatusLocalOnly:        {},
	ChangeStatusDraft:            {},
	ChangeStatusOpen:             {},
	ChangeStatusReviewRequested:  {},
	ChangeStatusApproved:         {},
	ChangeStatusChangesRequested: {},
	ChangeStatusMergeReady:       {},
	ChangeStatusMerged:           {},
	ChangeStatusClosed:           {},
	ChangeStatusSuperseded:       {},
	ChangeStatusExternalDrifted:  {},
}

func (s ChangeStatus) IsValid() bool {
	_, ok := validChangeStatuses[s]
	return ok
}

func (s ChangeStatus) Allows(next ChangeStatus) bool {
	allowed := map[ChangeStatus][]ChangeStatus{
		ChangeStatusLocalOnly:        {ChangeStatusDraft, ChangeStatusOpen, ChangeStatusClosed, ChangeStatusSuperseded, ChangeStatusExternalDrifted},
		ChangeStatusDraft:            {ChangeStatusOpen, ChangeStatusReviewRequested, ChangeStatusClosed, ChangeStatusSuperseded, ChangeStatusExternalDrifted},
		ChangeStatusOpen:             {ChangeStatusReviewRequested, ChangeStatusApproved, ChangeStatusChangesRequested, ChangeStatusMergeReady, ChangeStatusClosed, ChangeStatusSuperseded, ChangeStatusExternalDrifted},
		ChangeStatusReviewRequested:  {ChangeStatusApproved, ChangeStatusChangesRequested, ChangeStatusMergeReady, ChangeStatusClosed, ChangeStatusSuperseded, ChangeStatusExternalDrifted},
		ChangeStatusApproved:         {ChangeStatusChangesRequested, ChangeStatusMergeReady, ChangeStatusMerged, ChangeStatusClosed, ChangeStatusSuperseded, ChangeStatusExternalDrifted},
		ChangeStatusChangesRequested: {ChangeStatusDraft, ChangeStatusOpen, ChangeStatusReviewRequested, ChangeStatusClosed, ChangeStatusSuperseded, ChangeStatusExternalDrifted},
		ChangeStatusMergeReady:       {ChangeStatusMerged, ChangeStatusChangesRequested, ChangeStatusClosed, ChangeStatusSuperseded, ChangeStatusExternalDrifted},
		ChangeStatusExternalDrifted:  {ChangeStatusOpen, ChangeStatusReviewRequested, ChangeStatusApproved, ChangeStatusChangesRequested, ChangeStatusMergeReady, ChangeStatusMerged, ChangeStatusClosed, ChangeStatusSuperseded},
	}
	for _, candidate := range allowed[s] {
		if candidate == next {
			return true
		}
	}
	return false
}

type CheckAggregateState string

const (
	CheckAggregateUnknown CheckAggregateState = "unknown"
	CheckAggregatePending CheckAggregateState = "pending"
	CheckAggregatePassing CheckAggregateState = "passing"
	CheckAggregateFailing CheckAggregateState = "failing"
)

var validCheckAggregateStates = map[CheckAggregateState]struct{}{
	CheckAggregateUnknown: {},
	CheckAggregatePending: {},
	CheckAggregatePassing: {},
	CheckAggregateFailing: {},
}

func (s CheckAggregateState) IsValid() bool {
	_, ok := validCheckAggregateStates[s]
	return ok
}

type ChangeReadyState string

const (
	ChangeReadyUnknown                ChangeReadyState = "unknown"
	ChangeReadyNoLinkedChange         ChangeReadyState = "no_linked_change"
	ChangeReadyLinkedDraft            ChangeReadyState = "linked_change_draft"
	ChangeReadyReviewPending          ChangeReadyState = "review_pending"
	ChangeReadyChangesRequested       ChangeReadyState = "changes_requested"
	ChangeReadyChecksPending          ChangeReadyState = "checks_pending"
	ChangeReadyChecksFailing          ChangeReadyState = "checks_failing"
	ChangeReadyMergeBlockedByOpenGate ChangeReadyState = "merge_blocked_by_open_gate"
	ChangeReadyMergeReady             ChangeReadyState = "merge_ready"
)

var validChangeReadyStates = map[ChangeReadyState]struct{}{
	ChangeReadyUnknown:                {},
	ChangeReadyNoLinkedChange:         {},
	ChangeReadyLinkedDraft:            {},
	ChangeReadyReviewPending:          {},
	ChangeReadyChangesRequested:       {},
	ChangeReadyChecksPending:          {},
	ChangeReadyChecksFailing:          {},
	ChangeReadyMergeBlockedByOpenGate: {},
	ChangeReadyMergeReady:             {},
}

func (s ChangeReadyState) IsValid() bool {
	_, ok := validChangeReadyStates[s]
	return ok
}

type ChangeRef struct {
	ChangeID            string              `json:"change_id" yaml:"change_id"`
	ChangeUID           string              `json:"change_uid,omitempty" yaml:"change_uid,omitempty"`
	Provider            ChangeProvider      `json:"provider" yaml:"provider"`
	TicketID            string              `json:"ticket_id" yaml:"ticket_id"`
	RunID               string              `json:"run_id,omitempty" yaml:"run_id,omitempty"`
	BranchName          string              `json:"branch_name,omitempty" yaml:"branch_name,omitempty"`
	BaseBranch          string              `json:"base_branch,omitempty" yaml:"base_branch,omitempty"`
	HeadRef             string              `json:"head_ref,omitempty" yaml:"head_ref,omitempty"`
	URL                 string              `json:"url,omitempty" yaml:"url,omitempty"`
	ExternalID          string              `json:"external_id,omitempty" yaml:"external_id,omitempty"`
	Status              ChangeStatus        `json:"status" yaml:"status"`
	ChecksStatus        CheckAggregateState `json:"checks_status,omitempty" yaml:"checks_status,omitempty"`
	ReviewRequestedFrom []Actor             `json:"review_requested_from,omitempty" yaml:"review_requested_from,omitempty"`
	ReviewSummary       string              `json:"review_summary,omitempty" yaml:"review_summary,omitempty"`
	CreatedAt           time.Time           `json:"created_at" yaml:"created_at"`
	UpdatedAt           time.Time           `json:"updated_at" yaml:"updated_at"`
	SchemaVersion       int                 `json:"schema_version" yaml:"schema_version"`
}

func (c ChangeRef) Validate() error {
	if strings.TrimSpace(c.ChangeID) == "" {
		return fmt.Errorf("change_id is required")
	}
	if !c.Provider.IsValid() {
		return fmt.Errorf("invalid change provider: %s", c.Provider)
	}
	if strings.TrimSpace(c.TicketID) == "" {
		return fmt.Errorf("ticket_id is required")
	}
	if !c.Status.IsValid() {
		return fmt.Errorf("invalid change status: %s", c.Status)
	}
	if c.ChecksStatus != "" && !c.ChecksStatus.IsValid() {
		return fmt.Errorf("invalid checks_status: %s", c.ChecksStatus)
	}
	for _, actor := range c.ReviewRequestedFrom {
		if !actor.IsValid() {
			return fmt.Errorf("invalid review_requested_from actor: %s", actor)
		}
	}
	return nil
}

type CheckSource string

const (
	CheckSourceLocal    CheckSource = "local"
	CheckSourceProvider CheckSource = "provider"
	CheckSourceManual   CheckSource = "manual"
)

var validCheckSources = map[CheckSource]struct{}{
	CheckSourceLocal:    {},
	CheckSourceProvider: {},
	CheckSourceManual:   {},
}

func (s CheckSource) IsValid() bool {
	_, ok := validCheckSources[s]
	return ok
}

type CheckScope string

const (
	CheckScopeRun    CheckScope = "run"
	CheckScopeChange CheckScope = "change"
	CheckScopeTicket CheckScope = "ticket"
)

var validCheckScopes = map[CheckScope]struct{}{
	CheckScopeRun:    {},
	CheckScopeChange: {},
	CheckScopeTicket: {},
}

func (s CheckScope) IsValid() bool {
	_, ok := validCheckScopes[s]
	return ok
}

type CheckStatus string

const (
	CheckStatusQueued    CheckStatus = "queued"
	CheckStatusRunning   CheckStatus = "running"
	CheckStatusCompleted CheckStatus = "completed"
)

var validCheckStatuses = map[CheckStatus]struct{}{
	CheckStatusQueued:    {},
	CheckStatusRunning:   {},
	CheckStatusCompleted: {},
}

func (s CheckStatus) IsValid() bool {
	_, ok := validCheckStatuses[s]
	return ok
}

type CheckConclusion string

const (
	CheckConclusionUnknown   CheckConclusion = "unknown"
	CheckConclusionSuccess   CheckConclusion = "success"
	CheckConclusionFailure   CheckConclusion = "failure"
	CheckConclusionNeutral   CheckConclusion = "neutral"
	CheckConclusionCancelled CheckConclusion = "cancelled"
	CheckConclusionTimedOut  CheckConclusion = "timed_out"
	CheckConclusionSkipped   CheckConclusion = "skipped"
)

var validCheckConclusions = map[CheckConclusion]struct{}{
	CheckConclusionUnknown:   {},
	CheckConclusionSuccess:   {},
	CheckConclusionFailure:   {},
	CheckConclusionNeutral:   {},
	CheckConclusionCancelled: {},
	CheckConclusionTimedOut:  {},
	CheckConclusionSkipped:   {},
}

func (c CheckConclusion) IsValid() bool {
	_, ok := validCheckConclusions[c]
	return ok
}

type CheckResult struct {
	CheckID       string          `json:"check_id" yaml:"check_id"`
	CheckUID      string          `json:"check_uid,omitempty" yaml:"check_uid,omitempty"`
	Source        CheckSource     `json:"source" yaml:"source"`
	Provider      ChangeProvider  `json:"provider,omitempty" yaml:"provider,omitempty"`
	Scope         CheckScope      `json:"scope" yaml:"scope"`
	ScopeID       string          `json:"scope_id" yaml:"scope_id"`
	Name          string          `json:"name" yaml:"name"`
	Status        CheckStatus     `json:"status" yaml:"status"`
	Conclusion    CheckConclusion `json:"conclusion" yaml:"conclusion"`
	Summary       string          `json:"summary,omitempty" yaml:"summary,omitempty"`
	URL           string          `json:"url,omitempty" yaml:"url,omitempty"`
	StartedAt     time.Time       `json:"started_at,omitempty" yaml:"started_at,omitempty"`
	CompletedAt   time.Time       `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
	ExternalID    string          `json:"external_id,omitempty" yaml:"external_id,omitempty"`
	UpdatedAt     time.Time       `json:"updated_at" yaml:"updated_at"`
	SchemaVersion int             `json:"schema_version" yaml:"schema_version"`
}

func (c CheckResult) Validate() error {
	if strings.TrimSpace(c.CheckID) == "" {
		return fmt.Errorf("check_id is required")
	}
	if !c.Source.IsValid() {
		return fmt.Errorf("invalid check source: %s", c.Source)
	}
	if c.Provider != "" && !c.Provider.IsValid() {
		return fmt.Errorf("invalid check provider: %s", c.Provider)
	}
	if !c.Scope.IsValid() {
		return fmt.Errorf("invalid check scope: %s", c.Scope)
	}
	if strings.TrimSpace(c.ScopeID) == "" {
		return fmt.Errorf("scope_id is required")
	}
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if !c.Status.IsValid() {
		return fmt.Errorf("invalid check status: %s", c.Status)
	}
	if !c.Conclusion.IsValid() {
		return fmt.Errorf("invalid check conclusion: %s", c.Conclusion)
	}
	return nil
}

type PermissionAction string

const (
	PermissionActionDispatch       PermissionAction = "dispatch"
	PermissionActionRunLaunch      PermissionAction = "run_launch"
	PermissionActionChangeCreate   PermissionAction = "change_create"
	PermissionActionChangeMerge    PermissionAction = "change_merge"
	PermissionActionGateOpen       PermissionAction = "gate_open"
	PermissionActionGateApprove    PermissionAction = "gate_approve"
	PermissionActionRunComplete    PermissionAction = "run_complete"
	PermissionActionTicketComplete PermissionAction = "ticket_complete"
)

var validPermissionActions = map[PermissionAction]struct{}{
	PermissionActionDispatch:       {},
	PermissionActionRunLaunch:      {},
	PermissionActionChangeCreate:   {},
	PermissionActionChangeMerge:    {},
	PermissionActionGateOpen:       {},
	PermissionActionGateApprove:    {},
	PermissionActionRunComplete:    {},
	PermissionActionTicketComplete: {},
}

func (a PermissionAction) IsValid() bool {
	_, ok := validPermissionActions[a]
	return ok
}

type PermissionProfile struct {
	ProfileID                    string             `json:"profile_id" yaml:"profile_id"`
	DisplayName                  string             `json:"display_name,omitempty" yaml:"display_name,omitempty"`
	Priority                     int                `json:"priority,omitempty" yaml:"priority,omitempty"`
	WorkspaceDefault             bool               `json:"workspace_default,omitempty" yaml:"workspace_default,omitempty"`
	Projects                     []string           `json:"projects,omitempty" yaml:"projects,omitempty"`
	Agents                       []string           `json:"agents,omitempty" yaml:"agents,omitempty"`
	Runbooks                     []string           `json:"runbooks,omitempty" yaml:"runbooks,omitempty"`
	AllowedProjects              []string           `json:"allowed_projects,omitempty" yaml:"allowed_projects,omitempty"`
	AllowedTicketTypes           []TicketType       `json:"allowed_ticket_types,omitempty" yaml:"allowed_ticket_types,omitempty"`
	AllowedRunbooks              []string           `json:"allowed_runbooks,omitempty" yaml:"allowed_runbooks,omitempty"`
	AllowedCapabilities          []string           `json:"allowed_capabilities,omitempty" yaml:"allowed_capabilities,omitempty"`
	AllowActions                 []PermissionAction `json:"allow_actions,omitempty" yaml:"allow_actions,omitempty"`
	DenyActions                  []PermissionAction `json:"deny_actions,omitempty" yaml:"deny_actions,omitempty"`
	AllowedPaths                 []string           `json:"allowed_paths,omitempty" yaml:"allowed_paths,omitempty"`
	ForbiddenPaths               []string           `json:"forbidden_paths,omitempty" yaml:"forbidden_paths,omitempty"`
	RequiresOwnerForSensitiveOps bool               `json:"requires_owner_for_sensitive_ops,omitempty" yaml:"requires_owner_for_sensitive_ops,omitempty"`
	SchemaVersion                int                `json:"schema_version" yaml:"schema_version"`
}

func (p PermissionProfile) Validate() error {
	if strings.TrimSpace(p.ProfileID) == "" {
		return fmt.Errorf("profile_id is required")
	}
	for _, kind := range p.AllowedTicketTypes {
		if !kind.IsValid() {
			return fmt.Errorf("invalid allowed_ticket_type: %s", kind)
		}
	}
	for _, action := range p.AllowActions {
		if !action.IsValid() {
			return fmt.Errorf("invalid allow_action: %s", action)
		}
	}
	for _, action := range p.DenyActions {
		if !action.IsValid() {
			return fmt.Errorf("invalid deny_action: %s", action)
		}
	}
	return nil
}

type ImportSourceType string

const (
	ImportSourceAtlasBundle  ImportSourceType = "atlas_bundle"
	ImportSourceJiraCSV      ImportSourceType = "jira_csv"
	ImportSourceGitHubExport ImportSourceType = "github_export"
)

var validImportSourceTypes = map[ImportSourceType]struct{}{
	ImportSourceAtlasBundle:  {},
	ImportSourceJiraCSV:      {},
	ImportSourceGitHubExport: {},
}

func (s ImportSourceType) IsValid() bool {
	_, ok := validImportSourceTypes[s]
	return ok
}

type ImportJobStatus string

const (
	ImportJobPreviewed ImportJobStatus = "previewed"
	ImportJobValidated ImportJobStatus = "validated"
	ImportJobApplying  ImportJobStatus = "applying"
	ImportJobApplied   ImportJobStatus = "applied"
	ImportJobFailed    ImportJobStatus = "failed"
	ImportJobCanceled  ImportJobStatus = "canceled"
)

var validImportJobStatuses = map[ImportJobStatus]struct{}{
	ImportJobPreviewed: {},
	ImportJobValidated: {},
	ImportJobApplying:  {},
	ImportJobApplied:   {},
	ImportJobFailed:    {},
	ImportJobCanceled:  {},
}

func (s ImportJobStatus) IsValid() bool {
	_, ok := validImportJobStatuses[s]
	return ok
}

type ImportJob struct {
	JobID             string           `json:"job_id" yaml:"job_id"`
	ImportJobUID      string           `json:"import_job_uid,omitempty" yaml:"import_job_uid,omitempty"`
	SourceType        ImportSourceType `json:"source_type" yaml:"source_type"`
	Status            ImportJobStatus  `json:"status" yaml:"status"`
	SourceFingerprint string           `json:"source_fingerprint,omitempty" yaml:"source_fingerprint,omitempty"`
	Summary           string           `json:"summary,omitempty" yaml:"summary,omitempty"`
	Warnings          []string         `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	Errors            []string         `json:"errors,omitempty" yaml:"errors,omitempty"`
	ConflictLogPath   string           `json:"conflict_log_path,omitempty" yaml:"conflict_log_path,omitempty"`
	PartialApplied    bool             `json:"partial_applied,omitempty" yaml:"partial_applied,omitempty"`
	CreatedAt         time.Time        `json:"created_at" yaml:"created_at"`
	CompletedAt       time.Time        `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
	SchemaVersion     int              `json:"schema_version" yaml:"schema_version"`
}

func (j ImportJob) Validate() error {
	if strings.TrimSpace(j.JobID) == "" {
		return fmt.Errorf("job_id is required")
	}
	if !j.SourceType.IsValid() {
		return fmt.Errorf("invalid source_type: %s", j.SourceType)
	}
	if !j.Status.IsValid() {
		return fmt.Errorf("invalid import job status: %s", j.Status)
	}
	return nil
}

type ExportBundleStatus string

const (
	ExportBundleCreating ExportBundleStatus = "creating"
	ExportBundleCreated  ExportBundleStatus = "created"
	ExportBundleFailed   ExportBundleStatus = "failed"
	ExportBundleArchived ExportBundleStatus = "archived"
)

var validExportBundleStatuses = map[ExportBundleStatus]struct{}{
	ExportBundleCreating: {},
	ExportBundleCreated:  {},
	ExportBundleFailed:   {},
	ExportBundleArchived: {},
}

func (s ExportBundleStatus) IsValid() bool {
	_, ok := validExportBundleStatuses[s]
	return ok
}

type ExportBundle struct {
	BundleID           string             `json:"bundle_id" yaml:"bundle_id"`
	ExportBundleUID    string             `json:"export_bundle_uid,omitempty" yaml:"export_bundle_uid,omitempty"`
	Scope              string             `json:"scope,omitempty" yaml:"scope,omitempty"`
	Format             string             `json:"format,omitempty" yaml:"format,omitempty"`
	ArtifactPath       string             `json:"artifact_path,omitempty" yaml:"artifact_path,omitempty"`
	ManifestPath       string             `json:"manifest_path,omitempty" yaml:"manifest_path,omitempty"`
	ChecksumPath       string             `json:"checksum_path,omitempty" yaml:"checksum_path,omitempty"`
	RedactionPreviewID string             `json:"redaction_preview_id,omitempty" yaml:"redaction_preview_id,omitempty"`
	Status             ExportBundleStatus `json:"status" yaml:"status"`
	CreatedAt          time.Time          `json:"created_at" yaml:"created_at"`
	SchemaVersion      int                `json:"schema_version" yaml:"schema_version"`
}

func (b ExportBundle) Validate() error {
	if strings.TrimSpace(b.BundleID) == "" {
		return fmt.Errorf("bundle_id is required")
	}
	if !b.Status.IsValid() {
		return fmt.Errorf("invalid export bundle status: %s", b.Status)
	}
	return nil
}

type RetentionTarget string

const (
	RetentionTargetRuntime             RetentionTarget = "runtime"
	RetentionTargetEvidenceArtifacts   RetentionTarget = "evidence_artifacts"
	RetentionTargetHandoffExports      RetentionTarget = "handoff_exports"
	RetentionTargetLogs                RetentionTarget = "logs"
	RetentionTargetExportBundles       RetentionTarget = "export_bundles"
	RetentionTargetArchiveBundles      RetentionTarget = "archive_bundles"
	RetentionTargetVerificationScratch RetentionTarget = "verification_scratch"
)

var validRetentionTargets = map[RetentionTarget]struct{}{
	RetentionTargetRuntime:             {},
	RetentionTargetEvidenceArtifacts:   {},
	RetentionTargetHandoffExports:      {},
	RetentionTargetLogs:                {},
	RetentionTargetExportBundles:       {},
	RetentionTargetArchiveBundles:      {},
	RetentionTargetVerificationScratch: {},
}

func (t RetentionTarget) IsValid() bool {
	_, ok := validRetentionTargets[t]
	return ok
}

type RetentionPolicy struct {
	PolicyID               string          `json:"policy_id" yaml:"policy_id"`
	Target                 RetentionTarget `json:"target" yaml:"target"`
	MaxAgeDays             int             `json:"max_age_days,omitempty" yaml:"max_age_days,omitempty"`
	MaxTotalSizeMB         int             `json:"max_total_size_mb,omitempty" yaml:"max_total_size_mb,omitempty"`
	KeepLastN              int             `json:"keep_last_n,omitempty" yaml:"keep_last_n,omitempty"`
	ArchiveInsteadOfDelete bool            `json:"archive_instead_of_delete,omitempty" yaml:"archive_instead_of_delete,omitempty"`
	RequiresConfirmation   bool            `json:"requires_confirmation,omitempty" yaml:"requires_confirmation,omitempty"`
	SchemaVersion          int             `json:"schema_version" yaml:"schema_version"`
}

func (p RetentionPolicy) Validate() error {
	if strings.TrimSpace(p.PolicyID) == "" {
		return fmt.Errorf("policy_id is required")
	}
	if !p.Target.IsValid() {
		return fmt.Errorf("invalid retention target: %s", p.Target)
	}
	if p.MaxAgeDays < 0 {
		return fmt.Errorf("max_age_days must be >= 0")
	}
	if p.MaxTotalSizeMB < 0 {
		return fmt.Errorf("max_total_size_mb must be >= 0")
	}
	if p.KeepLastN < 0 {
		return fmt.Errorf("keep_last_n must be >= 0")
	}
	return nil
}

type ArchiveRecordState string

const (
	ArchiveRecordArchived ArchiveRecordState = "archived"
	ArchiveRecordRestored ArchiveRecordState = "restored"
)

var validArchiveRecordStates = map[ArchiveRecordState]struct{}{
	ArchiveRecordArchived: {},
	ArchiveRecordRestored: {},
}

func (s ArchiveRecordState) IsValid() bool {
	_, ok := validArchiveRecordStates[s]
	return ok
}

type ArchiveRecord struct {
	ArchiveID        string             `json:"archive_id" yaml:"archive_id"`
	ArchiveRecordUID string             `json:"archive_record_uid,omitempty" yaml:"archive_record_uid,omitempty"`
	Target           RetentionTarget    `json:"target" yaml:"target"`
	Scope            string             `json:"scope,omitempty" yaml:"scope,omitempty"`
	ProjectKey       string             `json:"project_key,omitempty" yaml:"project_key,omitempty"`
	SourcePaths      []string           `json:"source_paths,omitempty" yaml:"source_paths,omitempty"`
	PayloadDir       string             `json:"payload_dir,omitempty" yaml:"payload_dir,omitempty"`
	ItemCount        int                `json:"item_count,omitempty" yaml:"item_count,omitempty"`
	TotalBytes       int64              `json:"total_bytes,omitempty" yaml:"total_bytes,omitempty"`
	State            ArchiveRecordState `json:"state" yaml:"state"`
	Warnings         []string           `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	CreatedAt        time.Time          `json:"created_at" yaml:"created_at"`
	RestoredAt       time.Time          `json:"restored_at,omitempty" yaml:"restored_at,omitempty"`
	SchemaVersion    int                `json:"schema_version" yaml:"schema_version"`
}

func (r ArchiveRecord) Validate() error {
	if strings.TrimSpace(r.ArchiveID) == "" {
		return fmt.Errorf("archive_id is required")
	}
	if !r.Target.IsValid() {
		return fmt.Errorf("invalid archive target: %s", r.Target)
	}
	if !r.State.IsValid() {
		return fmt.Errorf("invalid archive state: %s", r.State)
	}
	if len(r.SourcePaths) == 0 {
		return fmt.Errorf("source_paths must not be empty")
	}
	for _, path := range r.SourcePaths {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("source_paths must not contain empty values")
		}
	}
	return nil
}
