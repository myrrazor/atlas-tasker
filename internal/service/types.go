package service

import (
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type PolicySource string

const (
	PolicySourceLegacy  PolicySource = "legacy_workspace"
	PolicySourceProject PolicySource = "project_default"
	PolicySourceEpic    PolicySource = "epic"
	PolicySourceTicket  PolicySource = "ticket_override"
)

type EffectivePolicyView struct {
	CompletionMode   contracts.CompletionMode `json:"completion_mode"`
	AllowedWorkers   []contracts.Actor        `json:"allowed_workers"`
	RequiredReviewer contracts.Actor          `json:"required_reviewer"`
	LeaseTTL         time.Duration            `json:"lease_ttl"`
	Sources          []PolicySource           `json:"sources"`
}

type QueueCategory string

const (
	QueueReadyForMe       QueueCategory = "ready_for_me"
	QueueClaimedByMe      QueueCategory = "claimed_by_me"
	QueueBlockedForMe     QueueCategory = "blocked_for_me"
	QueueNeedsReview      QueueCategory = "needs_review"
	QueueAwaitingOwner    QueueCategory = "awaiting_owner"
	QueueStaleClaims      QueueCategory = "stale_claims"
	QueuePolicyViolations QueueCategory = "policy_violations"
)

type QueueEntry struct {
	Ticket  contracts.TicketSnapshot `json:"ticket"`
	Reason  string                   `json:"reason"`
	GitHint string                   `json:"git_hint,omitempty"`
}

type QueueView struct {
	Actor       contracts.Actor                `json:"actor"`
	GeneratedAt time.Time                      `json:"generated_at"`
	Categories  map[QueueCategory][]QueueEntry `json:"categories"`
}

type AgentWorkState string

const (
	AgentWorkAvailable AgentWorkState = "available"
	AgentWorkPending   AgentWorkState = "pending"
)

type AgentWorkEntry struct {
	Ticket         contracts.TicketSnapshot `json:"ticket"`
	State          AgentWorkState           `json:"state"`
	Action         string                   `json:"action"`
	ReasonCodes    []string                 `json:"reason_codes,omitempty"`
	Reason         string                   `json:"reason,omitempty"`
	Suggested      []string                 `json:"suggested_commands,omitempty"`
	UnresolvedDeps []string                 `json:"unresolved_dependencies,omitempty"`
	RunID          string                   `json:"run_id,omitempty"`
	GitHint        string                   `json:"git_hint,omitempty"`
}

type AgentWorkView struct {
	Actor       contracts.Actor  `json:"actor"`
	AgentID     string           `json:"agent_id,omitempty"`
	GeneratedAt time.Time        `json:"generated_at"`
	Available   []AgentWorkEntry `json:"available"`
	Pending     []AgentWorkEntry `json:"pending"`
}

type BoardViewModel struct {
	Board contracts.BoardView `json:"board"`
}

type TicketDetailView struct {
	Ticket            contracts.TicketSnapshot `json:"ticket"`
	BoardStatus       contracts.Status         `json:"board_status"`
	EffectiveReviewer contracts.Actor          `json:"effective_reviewer,omitempty"`
	Comments          []string                 `json:"comments"`
	Mentions          []contracts.Mention      `json:"mentions,omitempty"`
	History           []contracts.Event        `json:"history"`
	Gates             []contracts.GateSnapshot `json:"gates,omitempty"`
	Changes           []contracts.ChangeRef    `json:"changes,omitempty"`
	Checks            []contracts.CheckResult  `json:"checks,omitempty"`
	EffectivePolicy   EffectivePolicyView      `json:"effective_policy"`
	Git               GitContextView           `json:"git"`
}

type HistoryView struct {
	TicketID string            `json:"ticket_id"`
	Events   []contracts.Event `json:"events"`
}

type DashboardBucket struct {
	Count     int      `json:"count"`
	TicketIDs []string `json:"ticket_ids,omitempty"`
}

type CollaboratorWorkloadView struct {
	CollaboratorID string `json:"collaborator_id"`
	Approvals      int    `json:"approvals"`
	InboxItems     int    `json:"inbox_items"`
	Mentions       int    `json:"mentions"`
	Handoffs       int    `json:"handoffs"`
}

type MentionQueueEntry struct {
	MentionUID      string    `json:"mention_uid"`
	CollaboratorID  string    `json:"collaborator_id"`
	TicketID        string    `json:"ticket_id,omitempty"`
	SourceKind      string    `json:"source_kind"`
	SourceID        string    `json:"source_id"`
	Summary         string    `json:"summary"`
	OriginWorkspace string    `json:"origin_workspace_id,omitempty"`
	GeneratedAt     time.Time `json:"generated_at"`
}

type ConflictQueueEntry struct {
	ConflictID   string                   `json:"conflict_id"`
	EntityKind   string                   `json:"entity_kind"`
	EntityUID    string                   `json:"entity_uid"`
	ConflictType contracts.ConflictType   `json:"conflict_type"`
	Status       contracts.ConflictStatus `json:"status"`
	TicketID     string                   `json:"ticket_id,omitempty"`
	OpenedByJob  string                   `json:"opened_by_job,omitempty"`
	GeneratedAt  time.Time                `json:"generated_at"`
}

type RemoteHealthView struct {
	RemoteID            string    `json:"remote_id"`
	Enabled             bool      `json:"enabled"`
	DefaultAction       string    `json:"default_action,omitempty"`
	PublicationCount    int       `json:"publication_count"`
	LatestPublicationAt time.Time `json:"latest_publication_at,omitempty"`
	LastSuccessAt       time.Time `json:"last_success_at,omitempty"`
	FailedJobs          int       `json:"failed_jobs"`
	State               string    `json:"state"`
}

type DashboardSummaryView struct {
	GeneratedAt             time.Time                  `json:"generated_at"`
	CollaboratorFilter      string                     `json:"collaborator_filter,omitempty"`
	ActiveRuns              int                        `json:"active_runs"`
	AwaitingReview          DashboardBucket            `json:"awaiting_review"`
	AwaitingOwner           DashboardBucket            `json:"awaiting_owner"`
	MergeReady              DashboardBucket            `json:"merge_ready"`
	BlockedByChecks         DashboardBucket            `json:"blocked_by_checks"`
	StaleWorktrees          []string                   `json:"stale_worktrees,omitempty"`
	RetentionTargets        []string                   `json:"retention_targets,omitempty"`
	CollaboratorWorkload    []CollaboratorWorkloadView `json:"collaborator_workload,omitempty"`
	MentionQueue            []MentionQueueEntry        `json:"mention_queue,omitempty"`
	ConflictQueue           []ConflictQueueEntry       `json:"conflict_queue,omitempty"`
	RemoteHealth            []RemoteHealthView         `json:"remote_health,omitempty"`
	FailedSyncJobs          []string                   `json:"failed_sync_jobs,omitempty"`
	ProviderMappingWarnings []string                   `json:"provider_mapping_warnings,omitempty"`
}

type TimelineEntry struct {
	Timestamp       time.Time           `json:"timestamp"`
	EventID         int64               `json:"event_id"`
	Type            contracts.EventType `json:"type"`
	Kind            string              `json:"kind,omitempty"`
	Actor           contracts.Actor     `json:"actor"`
	TicketID        string              `json:"ticket_id,omitempty"`
	CollaboratorIDs []string            `json:"collaborator_ids,omitempty"`
	Provenance      string              `json:"provenance,omitempty"`
	Summary         string              `json:"summary"`
}

type TimelineView struct {
	TicketID           string                     `json:"ticket_id"`
	CollaboratorFilter string                     `json:"collaborator_filter,omitempty"`
	GeneratedAt        time.Time                  `json:"generated_at"`
	Entries            []TimelineEntry            `json:"entries"`
	ChangeReady        contracts.ChangeReadyState `json:"change_ready"`
	OpenGateIDs        []string                   `json:"open_gate_ids,omitempty"`
}

type InspectView struct {
	Ticket            contracts.TicketSnapshot `json:"ticket"`
	BoardStatus       contracts.Status         `json:"board_status"`
	EffectiveReviewer contracts.Actor          `json:"effective_reviewer,omitempty"`
	LeaseActive       bool                     `json:"lease_active"`
	Migration         MigrationStatusView      `json:"migration"`
	EffectivePolicy   EffectivePolicyView      `json:"effective_policy"`
	Permissions       []PermissionDecisionView `json:"permissions,omitempty"`
	History           []contracts.Event        `json:"history"`
	Gates             []contracts.GateSnapshot `json:"gates,omitempty"`
	Changes           []contracts.ChangeRef    `json:"changes,omitempty"`
	Checks            []contracts.CheckResult  `json:"checks,omitempty"`
	Git               GitContextView           `json:"git"`
	Mentions          []contracts.Mention      `json:"mentions,omitempty"`
	QueueCategories   []QueueCategory          `json:"queue_categories,omitempty"`
}

type ChangeDetailView struct {
	Change       contracts.ChangeRef      `json:"change"`
	Ticket       contracts.TicketSnapshot `json:"ticket,omitempty"`
	Checks       []contracts.CheckResult  `json:"checks,omitempty"`
	ChangedFiles []string                 `json:"changed_files,omitempty"`
	GeneratedAt  time.Time                `json:"generated_at"`
}

type HandoffContextView struct {
	Handoff     contracts.HandoffPacket `json:"handoff"`
	Changes     []contracts.ChangeRef   `json:"changes,omitempty"`
	Checks      []contracts.CheckResult `json:"checks,omitempty"`
	Mentions    []contracts.Mention     `json:"mentions,omitempty"`
	GeneratedAt time.Time               `json:"generated_at"`
}

type TemplateView struct {
	Name         string                 `json:"name"`
	Path         string                 `json:"path"`
	Type         contracts.TicketType   `json:"type,omitempty"`
	Labels       []string               `json:"labels,omitempty"`
	Reviewer     contracts.Actor        `json:"reviewer,omitempty"`
	Policy       contracts.TicketPolicy `json:"policy,omitempty"`
	Blueprint    string                 `json:"blueprint,omitempty"`
	SkillHint    string                 `json:"skill_hint,omitempty"`
	Description  string                 `json:"description,omitempty"`
	Acceptance   []string               `json:"acceptance,omitempty"`
	TemplateBody string                 `json:"template_body"`
}

type NextEntry struct {
	Category QueueCategory `json:"category"`
	Entry    QueueEntry    `json:"entry"`
}

type NextView struct {
	Actor   contracts.Actor `json:"actor"`
	Entries []NextEntry     `json:"entries"`
}

type SavedViewResult struct {
	View    contracts.SavedView        `json:"view"`
	Actor   contracts.Actor            `json:"actor,omitempty"`
	Board   *BoardViewModel            `json:"board,omitempty"`
	Queue   *QueueView                 `json:"queue,omitempty"`
	Next    *NextView                  `json:"next,omitempty"`
	Tickets []contracts.TicketSnapshot `json:"tickets,omitempty"`
}

type SubscriptionView struct {
	Subscription   contracts.Subscription `json:"subscription"`
	Active         bool                   `json:"active"`
	InactiveReason string                 `json:"inactive_reason,omitempty"`
}

type BulkOperationKind string

const (
	BulkOperationMove          BulkOperationKind = "move"
	BulkOperationAssign        BulkOperationKind = "assign"
	BulkOperationRequestReview BulkOperationKind = "request_review"
	BulkOperationComplete      BulkOperationKind = "complete"
	BulkOperationClaim         BulkOperationKind = "claim"
	BulkOperationRelease       BulkOperationKind = "release"
)

type BulkOperation struct {
	Kind      BulkOperationKind `json:"kind"`
	Actor     contracts.Actor   `json:"actor"`
	Assignee  contracts.Actor   `json:"assignee,omitempty"`
	Status    contracts.Status  `json:"status,omitempty"`
	Reason    string            `json:"reason,omitempty"`
	TicketIDs []string          `json:"ticket_ids"`
	DryRun    bool              `json:"dry_run"`
	Confirm   bool              `json:"confirm"`
	BatchID   string            `json:"batch_id,omitempty"`
}

type BulkPreview struct {
	Kind        BulkOperationKind `json:"kind"`
	Actor       contracts.Actor   `json:"actor"`
	Assignee    contracts.Actor   `json:"assignee,omitempty"`
	Status      contracts.Status  `json:"status,omitempty"`
	TicketIDs   []string          `json:"ticket_ids"`
	TicketCount int               `json:"ticket_count"`
	DryRun      bool              `json:"dry_run"`
}

type BulkTicketResult struct {
	TicketID string                    `json:"ticket_id"`
	OK       bool                      `json:"ok"`
	DryRun   bool                      `json:"dry_run"`
	Code     string                    `json:"code,omitempty"`
	Error    string                    `json:"error,omitempty"`
	Reason   string                    `json:"reason,omitempty"`
	Ticket   *contracts.TicketSnapshot `json:"ticket,omitempty"`
}

type BulkSummary struct {
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
	Total     int `json:"total"`
}

type BulkOperationResult struct {
	BatchID string             `json:"batch_id"`
	Preview BulkPreview        `json:"preview"`
	Summary BulkSummary        `json:"summary"`
	Results []BulkTicketResult `json:"results"`
}

type GitRepoView struct {
	Branch  string `json:"branch,omitempty"`
	Dirty   bool   `json:"dirty"`
	Present bool   `json:"present"`
	Root    string `json:"root,omitempty"`
}

type GitCommitView struct {
	AuthorDate time.Time `json:"author_date"`
	Hash       string    `json:"hash"`
	Subject    string    `json:"subject"`
}

type GitHubCapabilityView struct {
	Authenticated bool   `json:"authenticated"`
	Installed     bool   `json:"installed"`
	Repo          string `json:"repo,omitempty"`
}

type GitHubPRView struct {
	BaseRef          string    `json:"base_ref,omitempty"`
	Draft            bool      `json:"draft"`
	HeadRef          string    `json:"head_ref,omitempty"`
	MergeStateStatus string    `json:"merge_state_status,omitempty"`
	MergedAt         time.Time `json:"merged_at,omitempty"`
	Number           int       `json:"number"`
	ReviewDecision   string    `json:"review_decision,omitempty"`
	State            string    `json:"state,omitempty"`
	Title            string    `json:"title"`
	URL              string    `json:"url"`
}

type GitHubCheckView struct {
	Bucket      string    `json:"bucket,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	Description string    `json:"description,omitempty"`
	Link        string    `json:"link,omitempty"`
	Name        string    `json:"name"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	State       string    `json:"state,omitempty"`
	Workflow    string    `json:"workflow,omitempty"`
}

type GitHubContextView struct {
	Capability     GitHubCapabilityView `json:"capability"`
	PullRequests   []GitHubPRView       `json:"pull_requests,omitempty"`
	SuggestedTitle string               `json:"suggested_title,omitempty"`
}

type GitContextView struct {
	CurrentBranchMatches bool              `json:"current_branch_matches"`
	GitHub               GitHubContextView `json:"github,omitempty"`
	Repo                 GitRepoView       `json:"repo"`
	SuggestedBranch      string            `json:"suggested_branch,omitempty"`
	Refs                 []GitCommitView   `json:"refs,omitempty"`
}

type ChangeCreateResultView struct {
	Change      contracts.ChangeRef      `json:"change"`
	Created     bool                     `json:"created"`
	ReasonCodes []string                 `json:"reason_codes,omitempty"`
	Ticket      contracts.TicketSnapshot `json:"ticket"`
	GeneratedAt time.Time                `json:"generated_at"`
}

type ChangeStatusView struct {
	Change               contracts.ChangeRef           `json:"change"`
	ChangedFiles         []string                      `json:"changed_files,omitempty"`
	CurrentBranch        string                        `json:"current_branch,omitempty"`
	DetachedHEAD         bool                          `json:"detached_head"`
	Git                  GitContextView                `json:"git"`
	LocalBranchExists    bool                          `json:"local_branch_exists"`
	ObservedChecksStatus contracts.CheckAggregateState `json:"observed_checks_status"`
	ObservedStatus       contracts.ChangeStatus        `json:"observed_status"`
	PullRequest          *GitHubPRView                 `json:"pull_request,omitempty"`
	ReasonCodes          []string                      `json:"reason_codes,omitempty"`
	Ticket               contracts.TicketSnapshot      `json:"ticket"`
	GeneratedAt          time.Time                     `json:"generated_at"`
}

type CheckSyncResultView struct {
	Aggregate   contracts.CheckAggregateState `json:"aggregate"`
	Change      contracts.ChangeRef           `json:"change"`
	Checks      []contracts.CheckResult       `json:"checks,omitempty"`
	ReasonCodes []string                      `json:"reason_codes,omitempty"`
	GeneratedAt time.Time                     `json:"generated_at"`
}
