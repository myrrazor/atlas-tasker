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
	Ticket contracts.TicketSnapshot `json:"ticket"`
	Reason string                   `json:"reason"`
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
	QueueCategories []QueueCategory          `json:"queue_categories,omitempty"`
}
