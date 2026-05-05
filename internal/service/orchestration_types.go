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

type ApprovalItemView struct {
	Gate        contracts.GateSnapshot   `json:"gate"`
	Ticket      contracts.TicketSnapshot `json:"ticket"`
	Summary     string                   `json:"summary"`
	GeneratedAt time.Time                `json:"generated_at"`
}

type AgentEligibilityEntry struct {
	Agent       contracts.AgentProfile `json:"agent"`
	Eligible    bool                   `json:"eligible"`
	ReasonCodes []string               `json:"reason_codes,omitempty"`
	ActiveRuns  int                    `json:"active_runs"`
	Runbook     string                 `json:"runbook,omitempty"`
	Stage       string                 `json:"stage,omitempty"`
	Rank        int                    `json:"rank,omitempty"`
}

type AgentEligibilityReport struct {
	TicketID    string                  `json:"ticket_id"`
	GeneratedAt time.Time               `json:"generated_at"`
	Entries     []AgentEligibilityEntry `json:"entries"`
}

type RunDetailView struct {
	Run         contracts.RunSnapshot     `json:"run"`
	Ticket      contracts.TicketSnapshot  `json:"ticket,omitempty"`
	Gates       []contracts.GateSnapshot  `json:"gates,omitempty"`
	Changes     []contracts.ChangeRef     `json:"changes,omitempty"`
	Checks      []contracts.CheckResult   `json:"checks,omitempty"`
	Evidence    []contracts.EvidenceItem  `json:"evidence,omitempty"`
	Handoffs    []contracts.HandoffPacket `json:"handoffs,omitempty"`
	GeneratedAt time.Time                 `json:"generated_at"`
}

type DispatchSuggestion struct {
	TicketID         string                  `json:"ticket_id"`
	GeneratedAt      time.Time               `json:"generated_at"`
	AutoRouteAgentID string                  `json:"auto_route_agent_id,omitempty"`
	Suggestions      []AgentEligibilityEntry `json:"suggestions"`
}

type DispatchQueueEntry struct {
	Ticket      contracts.TicketSnapshot `json:"ticket"`
	Suggestion  DispatchSuggestion       `json:"suggestion"`
	GeneratedAt time.Time                `json:"generated_at"`
}

type DispatchQueueView struct {
	GeneratedAt time.Time            `json:"generated_at"`
	Entries     []DispatchQueueEntry `json:"entries"`
}

type DispatchResult struct {
	TicketID     string    `json:"ticket_id"`
	RunID        string    `json:"run_id,omitempty"`
	AgentID      string    `json:"agent_id,omitempty"`
	Runbook      string    `json:"runbook,omitempty"`
	Stage        string    `json:"stage,omitempty"`
	ReasonCodes  []string  `json:"reason_codes,omitempty"`
	WorktreePath string    `json:"worktree_path,omitempty"`
	GeneratedAt  time.Time `json:"generated_at"`
}

type RunLaunchManifestView struct {
	RunID            string    `json:"run_id"`
	TicketID         string    `json:"ticket_id"`
	AgentID          string    `json:"agent_id"`
	RuntimeDir       string    `json:"runtime_dir"`
	WorktreePath     string    `json:"worktree_path,omitempty"`
	EvidenceDir      string    `json:"evidence_dir"`
	BriefPath        string    `json:"brief_path"`
	ContextPath      string    `json:"context_path"`
	CodexLaunchPath  string    `json:"codex_launch_path"`
	ClaudeLaunchPath string    `json:"claude_launch_path"`
	Created          []string  `json:"created,omitempty"`
	Updated          []string  `json:"updated,omitempty"`
	GeneratedAt      time.Time `json:"generated_at"`
}

type WorktreeStatusView struct {
	RunID         string    `json:"run_id"`
	TicketID      string    `json:"ticket_id,omitempty"`
	Path          string    `json:"path,omitempty"`
	BranchName    string    `json:"branch_name,omitempty"`
	Present       bool      `json:"present"`
	Dirty         bool      `json:"dirty"`
	LastCheckedAt time.Time `json:"last_checked_at"`
}

type InboxItemView struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	TicketID    string    `json:"ticket_id"`
	RunID       string    `json:"run_id,omitempty"`
	GateID      string    `json:"gate_id,omitempty"`
	HandoffID   string    `json:"handoff_id,omitempty"`
	Summary     string    `json:"summary"`
	State       string    `json:"state"`
	GeneratedAt time.Time `json:"generated_at"`
}

type InboxDetailView struct {
	Item      InboxItemView            `json:"item"`
	Ticket    contracts.TicketSnapshot `json:"ticket,omitempty"`
	Run       contracts.RunSnapshot    `json:"run,omitempty"`
	Gate      contracts.GateSnapshot   `json:"gate,omitempty"`
	Handoff   contracts.HandoffPacket  `json:"handoff,omitempty"`
	Generated time.Time                `json:"generated_at"`
}
