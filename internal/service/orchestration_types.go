package service

import (
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type AgentDetailView struct {
	Profile     contracts.AgentProfile `json:"profile"`
	ActiveRuns  int                    `json:"active_runs"`
	GeneratedAt time.Time              `json:"generated_at"`
}

type AgentEligibilityEntry struct {
	Agent       contracts.AgentProfile `json:"agent"`
	Eligible    bool                   `json:"eligible"`
	ReasonCodes []string               `json:"reason_codes,omitempty"`
	Rank        int                    `json:"rank,omitempty"`
}

type AgentEligibilityReport struct {
	TicketID     string                  `json:"ticket_id"`
	GeneratedAt  time.Time               `json:"generated_at"`
	Entries      []AgentEligibilityEntry `json:"entries"`
}

type RunDetailView struct {
	Run         contracts.RunSnapshot   `json:"run"`
	Ticket      contracts.TicketSnapshot `json:"ticket,omitempty"`
	Gates       []contracts.GateSnapshot `json:"gates,omitempty"`
	Evidence    []contracts.EvidenceItem `json:"evidence,omitempty"`
	Handoffs    []contracts.HandoffPacket `json:"handoffs,omitempty"`
	GeneratedAt time.Time                `json:"generated_at"`
}

type DispatchSuggestion struct {
	TicketID     string                    `json:"ticket_id"`
	GeneratedAt  time.Time                 `json:"generated_at"`
	Suggestions  []AgentEligibilityEntry   `json:"suggestions"`
}

type DispatchResult struct {
	TicketID       string              `json:"ticket_id"`
	RunID          string              `json:"run_id,omitempty"`
	AgentID        string              `json:"agent_id,omitempty"`
	ReasonCodes    []string            `json:"reason_codes,omitempty"`
	WorktreePath   string              `json:"worktree_path,omitempty"`
	GeneratedAt    time.Time           `json:"generated_at"`
}

type WorktreeStatusView struct {
	RunID          string    `json:"run_id"`
	TicketID       string    `json:"ticket_id,omitempty"`
	Path           string    `json:"path,omitempty"`
	BranchName     string    `json:"branch_name,omitempty"`
	Present        bool      `json:"present"`
	Dirty          bool      `json:"dirty"`
	LastCheckedAt  time.Time `json:"last_checked_at"`
}

type InboxItemView struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	TicketID    string    `json:"ticket_id"`
	RunID       string    `json:"run_id,omitempty"`
	GateID      string    `json:"gate_id,omitempty"`
	Summary     string    `json:"summary"`
	State       string    `json:"state"`
	GeneratedAt time.Time `json:"generated_at"`
}
