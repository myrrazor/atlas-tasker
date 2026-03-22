package contracts

import (
	"fmt"
	"strings"
	"time"
)

const CurrentSchemaVersion = 1

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
	CompletionModeOpen: {}, CompletionModeOwnerGate: {}, CompletionModeReviewGate: {},
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
	Key       string    `json:"key"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func (p Project) Validate() error {
	if strings.TrimSpace(p.Key) == "" {
		return fmt.Errorf("project key is required")
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("project name is required")
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
	Workflow WorkflowConfig `json:"workflow"`
}

func (c TrackerConfig) Validate() error {
	return c.Workflow.Validate()
}

// TicketSnapshot mirrors v1 ticket markdown frontmatter plus body sections.
type TicketSnapshot struct {
	ID            string     `json:"id"`
	Project       string     `json:"project"`
	Title         string     `json:"title"`
	Type          TicketType `json:"type"`
	Status        Status     `json:"status"`
	Priority      Priority   `json:"priority"`
	Parent        string     `json:"parent,omitempty"`
	Labels        []string   `json:"labels"`
	Assignee      Actor      `json:"assignee,omitempty"`
	Reviewer      Actor      `json:"reviewer,omitempty"`
	BlockedBy     []string   `json:"blocked_by"`
	Blocks        []string   `json:"blocks"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	SchemaVersion int        `json:"schema_version"`
	Archived      bool       `json:"archived"`

	Summary            string   `json:"summary,omitempty"`
	Description        string   `json:"description,omitempty"`
	AcceptanceCriteria []string `json:"acceptance_criteria,omitempty"`
	Notes              string   `json:"notes,omitempty"`
}

func (t TicketSnapshot) ValidateForCreate() error {
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
