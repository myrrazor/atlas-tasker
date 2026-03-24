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

type BoardViewModel struct {
	Board contracts.BoardView `json:"board"`
}

type TicketDetailView struct {
	Ticket          contracts.TicketSnapshot `json:"ticket"`
	Comments        []string                 `json:"comments"`
	History         []contracts.Event        `json:"history"`
	EffectivePolicy EffectivePolicyView      `json:"effective_policy"`
	Git             GitContextView           `json:"git"`
}

type HistoryView struct {
	TicketID string            `json:"ticket_id"`
	Events   []contracts.Event `json:"events"`
}

type InspectView struct {
	Ticket          contracts.TicketSnapshot `json:"ticket"`
	BoardStatus     contracts.Status         `json:"board_status"`
	LeaseActive     bool                     `json:"lease_active"`
	EffectivePolicy EffectivePolicyView      `json:"effective_policy"`
	History         []contracts.Event        `json:"history"`
	Git             GitContextView           `json:"git"`
	QueueCategories []QueueCategory          `json:"queue_categories,omitempty"`
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

type GitContextView struct {
	CurrentBranchMatches bool            `json:"current_branch_matches"`
	Repo                 GitRepoView     `json:"repo"`
	SuggestedBranch      string          `json:"suggested_branch,omitempty"`
	Refs                 []GitCommitView `json:"refs,omitempty"`
}
