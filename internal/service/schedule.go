package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

// ScheduleState is the current, derived state of a one-time ticket schedule.
type ScheduleState string

const (
	ScheduleStateScheduled     ScheduleState = "scheduled"
	ScheduleStateDue           ScheduleState = "due"
	ScheduleStateOverdue       ScheduleState = "overdue"
	ScheduleStateNotified      ScheduleState = "notified"
	ScheduleStateAgentReady    ScheduleState = "agent_ready"
	ScheduleStateAgentLaunched ScheduleState = "agent_launched"
	ScheduleStateAcknowledged  ScheduleState = "acknowledged"
	ScheduleStateFailed        ScheduleState = "failed"
	ScheduleStateCompleted     ScheduleState = "completed"
	ScheduleStateCanceled      ScheduleState = "canceled"
)

// ScheduleQuery limits schedule and completion-history reads. From is
// inclusive and To is exclusive when supplied.
type ScheduleQuery struct {
	Project string    `json:"project,omitempty"`
	From    time.Time `json:"from,omitempty"`
	To      time.Time `json:"to,omitempty"`
}

// ScheduleEntry joins a ticket schedule with its runner and wakeup state.
type ScheduleEntry struct {
	Ticket     contracts.TicketSnapshot `json:"ticket"`
	Runner     contracts.Actor          `json:"runner"`
	RunnerKind string                   `json:"runner_kind"`
	Agent      *contracts.AgentProfile  `json:"agent,omitempty"`
	Wakeup     *AgentWakeup             `json:"wakeup,omitempty"`
	State      ScheduleState            `json:"state"`
	Overdue    bool                     `json:"overdue"`
}

// CompletionEntry is a ticket completion derived from the append-only event log.
type CompletionEntry struct {
	Ticket      contracts.TicketSnapshot `json:"ticket"`
	CompletedAt time.Time                `json:"completed_at"`
	CompletedBy contracts.Actor          `json:"completed_by"`
	Reason      string                   `json:"reason,omitempty"`
	EventID     int64                    `json:"event_id"`
}

// ScheduleView contains scheduled work and completion history for the same range.
type ScheduleView struct {
	GeneratedAt time.Time         `json:"generated_at"`
	Entries     []ScheduleEntry   `json:"entries"`
	History     []CompletionEntry `json:"history"`
}

// ScheduleTickEntry reports one reminder or agent wakeup handled by a tick.
type ScheduleTickEntry struct {
	TicketID string          `json:"ticket_id"`
	Runner   contracts.Actor `json:"runner"`
	State    ScheduleState   `json:"state"`
	WakeupID string          `json:"wakeup_id,omitempty"`
	Error    string          `json:"error,omitempty"`
}

// ScheduleTickResult reports every due schedule considered by a tick.
type ScheduleTickResult struct {
	At      time.Time           `json:"at"`
	Entries []ScheduleTickEntry `json:"entries"`
}

// SetTicketSchedule creates or replaces a ticket's one-time schedule. The
// runner becomes the assignee so scheduling and workflow ownership cannot drift.
func (s *ActionService) SetTicketSchedule(ctx context.Context, ticketID string, at time.Time, runner contracts.Actor, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	return withWriteLock(ctx, s.LockManager, "set ticket schedule", func(ctx context.Context) (contracts.TicketSnapshot, error) {
		if !actor.IsValid() {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if !runner.IsValid() {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid schedule runner: %s", runner))
		}
		if strings.TrimSpace(reason) == "" {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeInvalidInput, "schedule requires a reason")
		}
		if at.IsZero() {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeInvalidInput, "schedule time is required")
		}
		ticket, err := s.Tickets.GetTicket(ctx, strings.TrimSpace(ticketID))
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
		if contracts.IsTerminalStatus(ticket.Status) {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("ticket %s is terminal and cannot be scheduled", ticket.ID))
		}
		if strings.HasPrefix(string(runner), "agent:") {
			if actor != contracts.Actor("human:owner") {
				return contracts.TicketSnapshot{}, apperr.New(apperr.CodePermissionDenied, "agent schedules can only be set by human:owner")
			}
			agent, err := s.Agents.LoadAgent(ctx, agentIDFromActor(runner))
			if err != nil {
				return contracts.TicketSnapshot{}, apperr.Wrap(apperr.CodeNotFound, err, "scheduled agent not found")
			}
			if !agent.Enabled {
				return contracts.TicketSnapshot{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("agent %s is disabled", agent.AgentID))
			}
		}
		now := s.now()
		ticket.Assignee = runner
		ticket.Schedule = &contracts.TicketSchedule{At: at.UTC(), CreatedAt: now, CreatedBy: actor}
		ticket.UpdatedAt = now
		payload := map[string]any{"ticket": ticket, "schedule": ticket.Schedule, "runner": runner}
		if err := s.commitTicketSnapshotEvent(ctx, "set ticket schedule", ticket, actor, reason, contracts.EventTicketScheduleSet, payload); err != nil {
			return contracts.TicketSnapshot{}, err
		}
		return ticket, nil
	})
}

// ClearTicketSchedule removes the current one-time schedule without changing
// the ticket assignee.
func (s *ActionService) ClearTicketSchedule(ctx context.Context, ticketID string, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	return withWriteLock(ctx, s.LockManager, "clear ticket schedule", func(ctx context.Context) (contracts.TicketSnapshot, error) {
		if !actor.IsValid() {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if strings.TrimSpace(reason) == "" {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeInvalidInput, "clearing a schedule requires a reason")
		}
		ticket, err := s.Tickets.GetTicket(ctx, strings.TrimSpace(ticketID))
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
		if ticket.Schedule == nil {
			return ticket, nil
		}
		cleared := *ticket.Schedule
		ticket.Schedule = nil
		ticket.UpdatedAt = s.now()
		payload := map[string]any{"ticket": ticket, "cleared_schedule": cleared}
		if err := s.commitTicketSnapshotEvent(ctx, "clear ticket schedule", ticket, actor, reason, contracts.EventTicketScheduleCleared, payload); err != nil {
			return contracts.TicketSnapshot{}, err
		}
		return ticket, nil
	})
}

// Schedule returns scheduled tickets and completion history in a single view.
func (s *QueryService) Schedule(ctx context.Context, query ScheduleQuery) (ScheduleView, error) {
	if err := validateScheduleQuery(query); err != nil {
		return ScheduleView{}, err
	}
	entries, err := s.scheduleEntries(ctx, query)
	if err != nil {
		return ScheduleView{}, err
	}
	history, err := s.CompletionHistory(ctx, query)
	if err != nil {
		return ScheduleView{}, err
	}
	return ScheduleView{GeneratedAt: s.now(), Entries: entries, History: history}, nil
}

func (s *QueryService) scheduleEntries(ctx context.Context, query ScheduleQuery) ([]ScheduleEntry, error) {
	tickets, err := s.Tickets.ListTickets(ctx, contracts.TicketListOptions{Project: strings.TrimSpace(query.Project), IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	entries := make([]ScheduleEntry, 0)
	wakeups := AgentWakeupStore{Root: s.Root}
	for _, ticket := range tickets {
		if ticket.Schedule == nil || !timeInScheduleRange(ticket.Schedule.At, query) {
			continue
		}
		entry := ScheduleEntry{Ticket: ticket, Runner: ticket.Assignee, RunnerKind: "human"}
		if strings.HasPrefix(string(ticket.Assignee), "agent:") {
			entry.RunnerKind = "agent"
			if agent, loadErr := s.Agents.LoadAgent(ctx, agentIDFromActor(ticket.Assignee)); loadErr == nil {
				entry.Agent = &agent
			}
			if ticket.Schedule.WakeupID != "" {
				if wakeup, loadErr := wakeups.LoadWakeup(ticket.Schedule.WakeupID); loadErr == nil {
					entry.Wakeup = &wakeup
				}
			}
		}
		entry.State, entry.Overdue = scheduleEntryState(entry, s.now())
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		left, right := entries[i].Ticket.Schedule.At, entries[j].Ticket.Schedule.At
		if left.Equal(right) {
			return entries[i].Ticket.ID < entries[j].Ticket.ID
		}
		return left.Before(right)
	})
	return entries, nil
}

// CompletionHistory derives completed tickets from immutable workflow events;
// no second completion timestamp is stored.
func (s *QueryService) CompletionHistory(ctx context.Context, query ScheduleQuery) ([]CompletionEntry, error) {
	if err := validateScheduleQuery(query); err != nil {
		return nil, err
	}
	projects, err := s.Projects.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	tickets, err := s.Tickets.ListTickets(ctx, contracts.TicketListOptions{IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	byID := make(map[string]contracts.TicketSnapshot, len(tickets))
	for _, ticket := range tickets {
		byID[ticket.ID] = ticket
	}
	completed := make(map[string]CompletionEntry)
	for _, project := range projects {
		if query.Project != "" && project.Key != strings.TrimSpace(query.Project) {
			continue
		}
		events, streamErr := s.Events.StreamEvents(ctx, project.Key, 0)
		if streamErr != nil {
			return nil, streamErr
		}
		for _, event := range events {
			if !timeInScheduleRange(event.Timestamp, query) {
				continue
			}
			ticket, ok := completedTicketFromEvent(event)
			if !ok {
				continue
			}
			if current, exists := byID[event.TicketID]; exists {
				ticket = current
			}
			completed[event.TicketID] = CompletionEntry{Ticket: ticket, CompletedAt: event.Timestamp.UTC(), CompletedBy: event.Actor, Reason: event.Reason, EventID: event.EventID}
		}
	}
	items := make([]CompletionEntry, 0, len(completed))
	for _, item := range completed {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CompletedAt.Equal(items[j].CompletedAt) {
			return items[i].Ticket.ID < items[j].Ticket.ID
		}
		return items[i].CompletedAt.After(items[j].CompletedAt)
	})
	return items, nil
}

// TickSchedules handles every untriggered schedule due at or before at. It is
// intentionally a one-shot operation so cron, launchd, or another owner-picked
// scheduler can control cadence without Atlas running a second daemon.
func (s *ActionService) TickSchedules(ctx context.Context, at time.Time, actor contracts.Actor, reason string) (ScheduleTickResult, error) {
	if !actor.IsValid() {
		return ScheduleTickResult{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
	}
	if strings.TrimSpace(reason) == "" {
		return ScheduleTickResult{}, apperr.New(apperr.CodeInvalidInput, "schedule tick requires a reason")
	}
	if at.IsZero() {
		at = s.now()
	}
	at = at.UTC()
	tickets, err := s.Tickets.ListTickets(ctx, contracts.TicketListOptions{IncludeArchived: false})
	if err != nil {
		return ScheduleTickResult{}, err
	}
	due := make([]contracts.TicketSnapshot, 0)
	for _, ticket := range tickets {
		if ticket.Schedule == nil || !ticket.Schedule.TriggeredAt.IsZero() || ticket.Schedule.At.After(at) || contracts.IsTerminalStatus(ticket.Status) {
			continue
		}
		due = append(due, ticket)
	}
	sort.Slice(due, func(i, j int) bool {
		if due[i].Schedule.At.Equal(due[j].Schedule.At) {
			return due[i].ID < due[j].ID
		}
		return due[i].Schedule.At.Before(due[j].Schedule.At)
	})
	result := ScheduleTickResult{At: at, Entries: make([]ScheduleTickEntry, 0, len(due))}
	for _, ticket := range due {
		entry, triggerErr := s.triggerTicketSchedule(ctx, ticket.ID, at, actor, reason)
		if triggerErr != nil {
			if code := apperr.CodeOf(triggerErr); code == apperr.CodePermissionDenied || code == apperr.CodeConflict {
				return ScheduleTickResult{}, triggerErr
			}
			if strings.HasPrefix(string(ticket.Assignee), "agent:") {
				failed, recordErr := s.failTicketSchedule(ctx, ticket.ID, actor, reason, triggerErr)
				if recordErr != nil {
					return ScheduleTickResult{}, recordErr
				}
				entry = failed
			} else {
				return ScheduleTickResult{}, triggerErr
			}
		}
		result.Entries = append(result.Entries, entry)
	}
	return result, nil
}

func (s *ActionService) triggerTicketSchedule(ctx context.Context, ticketID string, at time.Time, actor contracts.Actor, reason string) (ScheduleTickEntry, error) {
	return withWriteLock(ctx, s.LockManager, "trigger ticket schedule", func(ctx context.Context) (ScheduleTickEntry, error) {
		ticket, err := s.Tickets.GetTicket(ctx, ticketID)
		if err != nil {
			return ScheduleTickEntry{}, err
		}
		if ticket.Schedule == nil || !ticket.Schedule.TriggeredAt.IsZero() || ticket.Schedule.At.After(at) {
			return ScheduleTickEntry{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("ticket %s schedule is not due", ticket.ID))
		}
		if contracts.IsTerminalStatus(ticket.Status) {
			return ScheduleTickEntry{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("ticket %s is terminal", ticket.ID))
		}
		if strings.HasPrefix(string(ticket.Assignee), "agent:") && actor != contracts.Actor("human:owner") {
			return ScheduleTickEntry{}, apperr.New(apperr.CodePermissionDenied, "agent schedules can only be triggered by human:owner")
		}
		ticket.Schedule.TriggeredAt = s.now()
		ticket.Schedule.Error = ""
		ticket.UpdatedAt = s.now()
		entry := ScheduleTickEntry{TicketID: ticket.ID, Runner: ticket.Assignee, State: ScheduleStateNotified}
		payload := map[string]any{"ticket": ticket, "schedule": ticket.Schedule}
		var wakeup AgentWakeup
		var auto AgentAutoConfig
		if strings.HasPrefix(string(ticket.Assignee), "agent:") {
			auto, err = AgentAutoStore{Root: s.Root}.LoadConfig(agentIDFromActor(ticket.Assignee))
			if err != nil {
				return ScheduleTickEntry{}, err
			}
			wakeup = scheduledAgentWakeup(ticket, auto, ticket.UpdatedAt)
			ticket.Schedule.WakeupID = wakeup.WakeupID
			entry.WakeupID = wakeup.WakeupID
			entry.State = ScheduleStateAgentReady
			payload["wakeup"] = wakeup
		}
		event, err := s.newEvent(ctx, ticket.Project, ticket.UpdatedAt, actor, reason, contracts.EventTicketScheduleTriggered, ticket.ID, payload)
		if err != nil {
			return ScheduleTickEntry{}, err
		}
		if err := s.commitMutation(ctx, "trigger ticket schedule", "ticket_snapshot", event, func(ctx context.Context) error {
			if wakeup.WakeupID != "" {
				if err := (AgentWakeupStore{Root: s.Root}).SaveWakeup(wakeup); err != nil {
					return err
				}
			}
			return s.UpdateTicket(ctx, ticket)
		}); err != nil {
			return ScheduleTickEntry{}, err
		}
		if wakeup.WakeupID != "" && auto.Mode == AgentAutoModeCommand {
			wakeup = launchAgentWakeupCommand(ctx, wakeup, auto)
			_ = (AgentWakeupStore{Root: s.Root}).SaveWakeup(wakeup)
			if wakeup.State == AgentWakeupFailed {
				return ScheduleTickEntry{}, fmt.Errorf("launch scheduled agent: %s", wakeup.Error)
			}
			entry.State = ScheduleStateAgentLaunched
		}
		return entry, nil
	})
}

func (s *ActionService) failTicketSchedule(ctx context.Context, ticketID string, actor contracts.Actor, reason string, cause error) (ScheduleTickEntry, error) {
	return withWriteLock(ctx, s.LockManager, "record schedule failure", func(ctx context.Context) (ScheduleTickEntry, error) {
		ticket, err := s.Tickets.GetTicket(ctx, ticketID)
		if err != nil {
			return ScheduleTickEntry{}, err
		}
		if ticket.Schedule == nil {
			return ScheduleTickEntry{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("ticket %s has no schedule", ticket.ID))
		}
		if ticket.Schedule.TriggeredAt.IsZero() {
			ticket.Schedule.TriggeredAt = s.now()
		}
		ticket.Schedule.Error = cause.Error()
		ticket.UpdatedAt = s.now()
		payload := map[string]any{"ticket": ticket, "schedule": ticket.Schedule, "error": cause.Error()}
		if err := s.commitTicketSnapshotEvent(ctx, "record schedule failure", ticket, actor, reason, contracts.EventTicketScheduleFailed, payload); err != nil {
			return ScheduleTickEntry{}, err
		}
		return ScheduleTickEntry{TicketID: ticket.ID, Runner: ticket.Assignee, State: ScheduleStateFailed, WakeupID: ticket.Schedule.WakeupID, Error: ticket.Schedule.Error}, nil
	})
}

func scheduledAgentWakeup(ticket contracts.TicketSnapshot, auto AgentAutoConfig, at time.Time) AgentWakeup {
	id := fmt.Sprintf("wakeup_%s_schedule_%x", safeFileStem(ticket.ID), ticket.Schedule.CreatedAt.UTC().UnixNano())
	return AgentWakeup{
		WakeupID:    id,
		TicketID:    ticket.ID,
		Source:      "schedule",
		ScheduledAt: ticket.Schedule.At.UTC(),
		Actor:       ticket.Assignee,
		AgentID:     agentIDFromActor(ticket.Assignee),
		State:       AgentWakeupPending,
		Mode:        auto.Mode,
		Reason:      "scheduled ticket is due",
		CreatedAt:   at.UTC(),
	}
}

func validateScheduleQuery(query ScheduleQuery) error {
	if !query.From.IsZero() && !query.To.IsZero() && !query.From.Before(query.To) {
		return apperr.New(apperr.CodeInvalidInput, "schedule from must be before to")
	}
	return nil
}

func timeInScheduleRange(value time.Time, query ScheduleQuery) bool {
	if !query.From.IsZero() && value.Before(query.From) {
		return false
	}
	return query.To.IsZero() || value.Before(query.To)
}

func scheduleEntryState(entry ScheduleEntry, now time.Time) (ScheduleState, bool) {
	schedule := entry.Ticket.Schedule
	if entry.Ticket.Status == contracts.StatusDone {
		return ScheduleStateCompleted, false
	}
	if entry.Ticket.Status == contracts.StatusCanceled {
		return ScheduleStateCanceled, false
	}
	if schedule.Error != "" {
		return ScheduleStateFailed, false
	}
	if !schedule.TriggeredAt.IsZero() {
		if entry.RunnerKind == "human" {
			return ScheduleStateNotified, false
		}
		if entry.Wakeup != nil {
			switch entry.Wakeup.State {
			case AgentWakeupLaunched:
				return ScheduleStateAgentLaunched, false
			case AgentWakeupAcked:
				return ScheduleStateAcknowledged, false
			case AgentWakeupFailed:
				return ScheduleStateFailed, false
			}
		}
		return ScheduleStateAgentReady, false
	}
	if schedule.At.Before(now) {
		return ScheduleStateOverdue, true
	}
	if schedule.At.Equal(now) {
		return ScheduleStateDue, false
	}
	return ScheduleStateScheduled, false
}

func completedTicketFromEvent(event contracts.Event) (contracts.TicketSnapshot, bool) {
	if event.Type != contracts.EventTicketMoved && event.Type != contracts.EventTicketApproved {
		return contracts.TicketSnapshot{}, false
	}
	raw, err := json.Marshal(event.Payload)
	if err != nil {
		return contracts.TicketSnapshot{}, false
	}
	var payload struct {
		To     contracts.Status         `json:"to"`
		Ticket contracts.TicketSnapshot `json:"ticket"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return contracts.TicketSnapshot{}, false
	}
	if payload.To != contracts.StatusDone && payload.Ticket.Status != contracts.StatusDone {
		return contracts.TicketSnapshot{}, false
	}
	if payload.Ticket.ID == "" {
		payload.Ticket.ID = event.TicketID
		payload.Ticket.Project = event.Project
	}
	return payload.Ticket, true
}
