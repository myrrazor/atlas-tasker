package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

func setupScheduleTest(t *testing.T, notifier Notifier) (string, context.Context, *ActionService, *QueryService, mdstore.TicketStore, time.Time) {
	t.Helper()
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 8, 7, 13, 30, 0, 0, time.UTC)
	projects := mdstore.ProjectStore{RootDir: root}
	tickets := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	events := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), tickets, events)
	if err != nil {
		t.Fatalf("open projection: %v", err)
	}
	t.Cleanup(func() { _ = projection.Close() })
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := projects.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := (AgentStore{Root: root}).SaveAgent(ctx, contracts.AgentProfile{AgentID: "builder-1", DisplayName: "Builder", Provider: contracts.AgentProviderCodex, Enabled: true}); err != nil {
		t.Fatalf("save agent: %v", err)
	}
	actions := NewActionService(root, projects, tickets, events, projection, func() time.Time { return now }, FileLockManager{Root: root}, notifier, nil)
	queries := NewQueryService(root, projects, tickets, events, projection, func() time.Time { return now })
	return root, ctx, actions, queries, tickets, now
}

func scheduleTestTicket(id string, status contracts.Status, now time.Time) contracts.TicketSnapshot {
	return contracts.TicketSnapshot{
		ID:            id,
		Project:       "APP",
		Title:         "Scheduled work " + id,
		Type:          contracts.TicketTypeTask,
		Status:        status,
		Priority:      contracts.PriorityMedium,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
}

func TestHumanScheduleSetTickAndClear(t *testing.T) {
	var notified []contracts.Event
	notifier := notifierFunc(func(_ context.Context, event contracts.Event) error {
		notified = append(notified, event)
		return nil
	})
	_, ctx, actions, queries, tickets, now := setupScheduleTest(t, notifier)
	ticket := scheduleTestTicket("APP-1", contracts.StatusReady, now)
	if err := tickets.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	dueAt := now.Add(45 * time.Minute)
	scheduled, err := actions.SetTicketSchedule(ctx, ticket.ID, dueAt, contracts.Actor("human:alex"), contracts.Actor("human:owner"), "plan the work")
	if err != nil {
		t.Fatalf("set schedule: %v", err)
	}
	if scheduled.Assignee != contracts.Actor("human:alex") || scheduled.Schedule == nil || !scheduled.Schedule.At.Equal(dueAt) {
		t.Fatalf("unexpected scheduled ticket: %#v", scheduled)
	}

	view, err := queries.Schedule(ctx, ScheduleQuery{Project: "APP"})
	if err != nil {
		t.Fatalf("query schedule: %v", err)
	}
	if len(view.Entries) != 1 || view.Entries[0].State != ScheduleStateScheduled || view.Entries[0].RunnerKind != "human" {
		t.Fatalf("unexpected schedule view: %#v", view.Entries)
	}
	queries.Clock = func() time.Time { return dueAt.Add(time.Minute) }
	overdue, err := queries.Schedule(ctx, ScheduleQuery{Project: "APP"})
	if err != nil || len(overdue.Entries) != 1 || overdue.Entries[0].State != ScheduleStateOverdue || !overdue.Entries[0].Overdue {
		t.Fatalf("expected overdue schedule state: %#v err=%v", overdue.Entries, err)
	}
	queries.Clock = func() time.Time { return now }

	ticked, err := actions.TickSchedules(ctx, dueAt, contracts.Actor("human:owner"), "scheduled tick")
	if err != nil {
		t.Fatalf("tick schedules: %v", err)
	}
	if len(ticked.Entries) != 1 || ticked.Entries[0].State != ScheduleStateNotified {
		t.Fatalf("unexpected tick result: %#v", ticked)
	}
	if len(notified) < 2 || notified[len(notified)-1].Type != contracts.EventTicketScheduleTriggered {
		t.Fatalf("expected triggered schedule notification, got %#v", notified)
	}

	second, err := actions.TickSchedules(ctx, dueAt.Add(time.Hour), contracts.Actor("human:owner"), "scheduled tick")
	if err != nil {
		t.Fatalf("second tick: %v", err)
	}
	if len(second.Entries) != 0 {
		t.Fatalf("expected one-time schedule to be idempotent, got %#v", second.Entries)
	}
	view, err = queries.Schedule(ctx, ScheduleQuery{Project: "APP"})
	if err != nil {
		t.Fatalf("query triggered schedule: %v", err)
	}
	if view.Entries[0].State != ScheduleStateNotified {
		t.Fatalf("expected notified state, got %#v", view.Entries[0])
	}

	cleared, err := actions.ClearTicketSchedule(ctx, ticket.ID, contracts.Actor("human:owner"), "no longer needed")
	if err != nil {
		t.Fatalf("clear schedule: %v", err)
	}
	if cleared.Schedule != nil || cleared.Assignee != contracts.Actor("human:alex") {
		t.Fatalf("clear should keep ownership and remove only schedule: %#v", cleared)
	}
}

func TestAgentScheduleLaunchesConfiguredCommand(t *testing.T) {
	root, ctx, actions, queries, tickets, now := setupScheduleTest(t, nil)
	marker := filepath.Join(root, "scheduled-agent.marker")
	if _, err := actions.SetAgentAuto(ctx, "builder-1", AgentAutoModeCommand, []string{"/usr/bin/touch", marker}, contracts.Actor("human:owner"), "launch scheduled worker"); err != nil {
		t.Fatalf("set agent auto: %v", err)
	}
	ticket := scheduleTestTicket("APP-2", contracts.StatusReady, now)
	if err := tickets.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	dueAt := now.Add(time.Hour)
	if _, err := actions.SetTicketSchedule(ctx, ticket.ID, dueAt, contracts.Actor("agent:builder-1"), contracts.Actor("human:owner"), "run the scheduled check"); err != nil {
		t.Fatalf("set schedule: %v", err)
	}
	result, err := actions.TickSchedules(ctx, dueAt, contracts.Actor("human:owner"), "scheduled tick")
	if err != nil {
		t.Fatalf("tick schedules: %v", err)
	}
	if len(result.Entries) != 1 || result.Entries[0].State != ScheduleStateAgentLaunched || result.Entries[0].WakeupID == "" {
		t.Fatalf("unexpected agent tick: %#v", result)
	}
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected scheduled command marker: %v", err)
	}
	wakeup, err := queries.AgentWakeup(ctx, result.Entries[0].WakeupID)
	if err != nil {
		t.Fatalf("load wakeup: %v", err)
	}
	if wakeup.Source != "schedule" || wakeup.State != AgentWakeupLaunched || !wakeup.ScheduledAt.Equal(dueAt) {
		t.Fatalf("unexpected scheduled wakeup: %#v", wakeup)
	}
	view, err := queries.Schedule(ctx, ScheduleQuery{Project: "APP"})
	if err != nil {
		t.Fatalf("query schedule: %v", err)
	}
	if len(view.Entries) != 1 || view.Entries[0].State != ScheduleStateAgentLaunched || view.Entries[0].Agent == nil || view.Entries[0].Agent.Provider != contracts.AgentProviderCodex {
		t.Fatalf("unexpected agent schedule view: %#v", view.Entries)
	}
}

func TestAgentScheduleDefaultsToPendingWakeup(t *testing.T) {
	_, ctx, actions, queries, tickets, now := setupScheduleTest(t, nil)
	ticket := scheduleTestTicket("APP-3", contracts.StatusBacklog, now)
	if err := tickets.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if _, err := actions.SetTicketSchedule(ctx, ticket.ID, now.Add(time.Minute), contracts.Actor("agent:builder-1"), contracts.Actor("human:owner"), "queue agent work"); err != nil {
		t.Fatalf("set schedule: %v", err)
	}
	result, err := actions.TickSchedules(ctx, now.Add(time.Minute), contracts.Actor("human:owner"), "scheduled tick")
	if err != nil {
		t.Fatalf("tick schedules: %v", err)
	}
	if len(result.Entries) != 1 || result.Entries[0].State != ScheduleStateAgentReady {
		t.Fatalf("expected pending agent wakeup, got %#v", result)
	}
	wakeup, err := queries.AgentWakeup(ctx, result.Entries[0].WakeupID)
	if err != nil || wakeup.State != AgentWakeupPending || wakeup.Mode != AgentAutoModeNotify {
		t.Fatalf("unexpected pending wakeup: %#v err=%v", wakeup, err)
	}
}

func TestAgentScheduleFailureIsDurableAndDoesNotRetry(t *testing.T) {
	_, ctx, actions, queries, tickets, now := setupScheduleTest(t, nil)
	if _, err := actions.SetAgentAuto(ctx, "builder-1", AgentAutoModeCommand, []string{"/definitely/missing/atlas-agent"}, contracts.Actor("human:owner"), "test failed launch"); err != nil {
		t.Fatalf("set agent auto: %v", err)
	}
	ticket := scheduleTestTicket("APP-4", contracts.StatusReady, now)
	if err := tickets.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if _, err := actions.SetTicketSchedule(ctx, ticket.ID, now.Add(time.Minute), contracts.Actor("agent:builder-1"), contracts.Actor("human:owner"), "launch unavailable worker"); err != nil {
		t.Fatalf("set schedule: %v", err)
	}
	result, err := actions.TickSchedules(ctx, now.Add(time.Minute), contracts.Actor("human:owner"), "scheduled tick")
	if err != nil {
		t.Fatalf("failure should be reported in result, not abort tick: %v", err)
	}
	if len(result.Entries) != 1 || result.Entries[0].State != ScheduleStateFailed || !strings.Contains(result.Entries[0].Error, "launch scheduled agent") {
		t.Fatalf("unexpected failed tick: %#v", result)
	}
	view, err := queries.Schedule(ctx, ScheduleQuery{Project: "APP"})
	if err != nil {
		t.Fatalf("query schedule: %v", err)
	}
	if len(view.Entries) != 1 || view.Entries[0].State != ScheduleStateFailed || view.Entries[0].Ticket.Schedule.Error == "" {
		t.Fatalf("expected durable failure state: %#v", view.Entries)
	}
	second, err := actions.TickSchedules(ctx, now.Add(time.Minute), contracts.Actor("human:owner"), "scheduled tick")
	if err != nil || len(second.Entries) != 0 {
		t.Fatalf("failed one-time schedule should not retry: %#v err=%v", second, err)
	}
	rescheduled, err := actions.SetTicketSchedule(ctx, ticket.ID, now.Add(time.Hour), contracts.Actor("agent:builder-1"), contracts.Actor("human:owner"), "retry after fixing command")
	if err != nil {
		t.Fatalf("reschedule failed ticket: %v", err)
	}
	if rescheduled.Schedule == nil || !rescheduled.Schedule.TriggeredAt.IsZero() || rescheduled.Schedule.Error != "" || rescheduled.Schedule.WakeupID != "" {
		t.Fatalf("reschedule should reset one-time outcome: %#v", rescheduled.Schedule)
	}
}

func TestAgentScheduleRequiresOwnerAndEnabledProfile(t *testing.T) {
	root, ctx, actions, _, tickets, now := setupScheduleTest(t, nil)
	ticket := scheduleTestTicket("APP-5", contracts.StatusReady, now)
	if err := tickets.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	_, err := actions.SetTicketSchedule(ctx, ticket.ID, now.Add(time.Minute), contracts.Actor("agent:builder-1"), contracts.Actor("agent:planner"), "delegate")
	if apperr.CodeOf(err) != apperr.CodePermissionDenied {
		t.Fatalf("expected owner-only error, got %v", err)
	}
	if err := (AgentStore{Root: root}).SaveAgent(ctx, contracts.AgentProfile{AgentID: "builder-1", DisplayName: "Builder", Provider: contracts.AgentProviderCodex, Enabled: false}); err != nil {
		t.Fatalf("disable agent: %v", err)
	}
	_, err = actions.SetTicketSchedule(ctx, ticket.ID, now.Add(time.Minute), contracts.Actor("agent:builder-1"), contracts.Actor("human:owner"), "delegate")
	if apperr.CodeOf(err) != apperr.CodeConflict {
		t.Fatalf("expected disabled-agent conflict, got %v", err)
	}
}

func TestTerminalTicketCannotBeScheduled(t *testing.T) {
	_, ctx, actions, _, tickets, now := setupScheduleTest(t, nil)
	ticket := scheduleTestTicket("APP-8", contracts.StatusDone, now)
	if err := tickets.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	_, err := actions.SetTicketSchedule(ctx, ticket.ID, now.Add(time.Hour), contracts.Actor("human:owner"), contracts.Actor("human:owner"), "too late")
	if apperr.CodeOf(err) != apperr.CodeConflict {
		t.Fatalf("expected terminal schedule conflict, got %v", err)
	}
}

func TestCompletionHistoryUsesDoneEventsAndRange(t *testing.T) {
	_, ctx, actions, queries, tickets, now := setupScheduleTest(t, nil)
	first := scheduleTestTicket("APP-6", contracts.StatusDone, now)
	second := scheduleTestTicket("APP-7", contracts.StatusDone, now)
	for _, ticket := range []contracts.TicketSnapshot{first, second} {
		if err := tickets.CreateTicket(ctx, ticket); err != nil {
			t.Fatalf("create ticket: %v", err)
		}
	}
	events := []contracts.Event{
		{EventID: 1, Timestamp: now.Add(time.Hour), Actor: contracts.Actor("human:owner"), Reason: "finished first", Type: contracts.EventTicketMoved, Project: "APP", TicketID: first.ID, Payload: map[string]any{"to": contracts.StatusDone, "ticket": first}, SchemaVersion: contracts.CurrentSchemaVersion},
		{EventID: 2, Timestamp: now.Add(2 * time.Hour), Actor: contracts.Actor("agent:reviewer"), Reason: "approved second", Type: contracts.EventTicketApproved, Project: "APP", TicketID: second.ID, Payload: map[string]any{"ticket": second}, SchemaVersion: contracts.CurrentSchemaVersion},
		{EventID: 3, Timestamp: now.Add(3 * time.Hour), Actor: contracts.Actor("human:owner"), Type: contracts.EventTicketUpdated, Project: "APP", TicketID: second.ID, Payload: second, SchemaVersion: contracts.CurrentSchemaVersion},
	}
	for _, event := range events {
		if err := actions.AppendAndProject(ctx, event); err != nil {
			t.Fatalf("append event: %v", err)
		}
	}
	history, err := queries.CompletionHistory(ctx, ScheduleQuery{Project: "APP", From: now.Add(90 * time.Minute), To: now.Add(3 * time.Hour)})
	if err != nil {
		t.Fatalf("completion history: %v", err)
	}
	if len(history) != 1 || history[0].Ticket.ID != second.ID || history[0].CompletedBy != contracts.Actor("agent:reviewer") {
		t.Fatalf("unexpected filtered completion history: %#v", history)
	}
}

func TestScheduleQueryRejectsInvertedRange(t *testing.T) {
	_, ctx, _, queries, _, now := setupScheduleTest(t, nil)
	_, err := queries.Schedule(ctx, ScheduleQuery{From: now, To: now.Add(-time.Minute)})
	if apperr.CodeOf(err) != apperr.CodeInvalidInput {
		t.Fatalf("expected invalid range, got %v", err)
	}
}

func TestScheduleTriggerAndFailureAreDefaultNotifications(t *testing.T) {
	for _, eventType := range []contracts.EventType{contracts.EventTicketScheduleTriggered, contracts.EventTicketScheduleFailed} {
		if !shouldNotify(eventType) {
			t.Fatalf("expected %s to use configured notification sinks", eventType)
		}
	}
}

func TestScheduledWakeupArgvIncludesDueInstant(t *testing.T) {
	due := time.Date(2026, 8, 7, 17, 0, 0, 0, time.FixedZone("EDT", -4*60*60))
	argv := substituteWakeupArgv([]string{"worker", "--at", "{scheduled_at}"}, AgentWakeup{ScheduledAt: due})
	if len(argv) != 3 || argv[2] != "2026-08-07T21:00:00Z" {
		t.Fatalf("unexpected scheduled_at substitution: %#v", argv)
	}
}
