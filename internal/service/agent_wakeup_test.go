package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

func setupAgentWakeupTest(t *testing.T) (string, context.Context, *ActionService, *QueryService, mdstore.TicketStore, time.Time) {
	t.Helper()
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 5, 9, 16, 0, 0, 0, time.UTC)
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
	actions := NewActionService(root, projects, tickets, events, projection, func() time.Time { return now }, FileLockManager{Root: root}, nil, nil)
	queries := NewQueryService(root, projects, tickets, events, projection, func() time.Time { return now })
	return root, ctx, actions, queries, tickets, now
}

func TestAgentWakeupCreatedWhenBlockerReachesDone(t *testing.T) {
	_, ctx, actions, queries, tickets, now := setupAgentWakeupTest(t)
	blocker := testAgentWorkTicket("APP-1", "Blocker", contracts.StatusInReview, now)
	blocker.ReviewState = contracts.ReviewStateApproved
	dependent := testAgentWorkTicket("APP-2", "Dependent", contracts.StatusReady, now)
	dependent.Assignee = contracts.Actor("agent:builder-1")
	dependent.BlockedBy = []string{blocker.ID}
	for _, ticket := range []contracts.TicketSnapshot{blocker, dependent} {
		if err := tickets.CreateTicket(ctx, ticket); err != nil {
			t.Fatalf("create %s: %v", ticket.ID, err)
		}
	}
	if _, err := actions.MoveTicket(ctx, blocker.ID, contracts.StatusDone, contracts.Actor("human:owner"), "blocker done"); err != nil {
		t.Fatalf("complete blocker: %v", err)
	}
	wakeups, err := queries.AgentWakeups(ctx, "builder-1")
	if err != nil {
		t.Fatalf("list wakeups: %v", err)
	}
	if len(wakeups) != 1 {
		t.Fatalf("expected one wakeup, got %#v", wakeups)
	}
	if wakeups[0].TicketID != dependent.ID || wakeups[0].BlockerTicketID != blocker.ID || wakeups[0].State != AgentWakeupPending {
		t.Fatalf("unexpected wakeup: %#v", wakeups[0])
	}
	events, err := actions.Events.StreamEvents(ctx, "APP", 0)
	if err != nil {
		t.Fatalf("stream events: %v", err)
	}
	found := false
	for _, event := range events {
		if event.Type == contracts.EventAgentWorkAvailable {
			found = true
			if event.Actor != contracts.ActorAtlasSystem {
				t.Fatalf("expected system actor %s, got %s", contracts.ActorAtlasSystem, event.Actor)
			}
		}
	}
	if !found {
		t.Fatalf("expected agent.work_available event, got %#v", events)
	}
}

func TestAgentWakeupCommandModeLaunchesArgvWithoutShell(t *testing.T) {
	root, ctx, actions, queries, tickets, now := setupAgentWakeupTest(t)
	marker := filepath.Join(root, "launched.marker")
	if _, err := actions.SetAgentAuto(ctx, "builder-1", AgentAutoModeCommand, []string{"/usr/bin/touch", marker}, contracts.Actor("human:owner"), "enable command mode"); err != nil {
		t.Fatalf("set auto: %v", err)
	}
	blocker := testAgentWorkTicket("APP-1", "Blocker", contracts.StatusInReview, now)
	blocker.ReviewState = contracts.ReviewStateApproved
	dependent := testAgentWorkTicket("APP-2", "Dependent", contracts.StatusReady, now)
	dependent.Assignee = contracts.Actor("agent:builder-1")
	dependent.BlockedBy = []string{blocker.ID}
	for _, ticket := range []contracts.TicketSnapshot{blocker, dependent} {
		if err := tickets.CreateTicket(ctx, ticket); err != nil {
			t.Fatalf("create %s: %v", ticket.ID, err)
		}
	}
	if _, err := actions.MoveTicket(ctx, blocker.ID, contracts.StatusDone, contracts.Actor("human:owner"), "blocker done"); err != nil {
		t.Fatalf("complete blocker: %v", err)
	}
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected command mode marker: %v", err)
	}
	wakeups, err := queries.AgentWakeups(ctx, "builder-1")
	if err != nil {
		t.Fatalf("list wakeups: %v", err)
	}
	if len(wakeups) != 1 || wakeups[0].State != AgentWakeupLaunched || len(wakeups[0].Command) != 2 {
		t.Fatalf("expected launched wakeup with command, got %#v", wakeups)
	}
}

func TestAgentAutoRejectsShellInterpreter(t *testing.T) {
	_, ctx, actions, _, _, _ := setupAgentWakeupTest(t)
	_, err := actions.SetAgentAuto(ctx, "builder-1", AgentAutoModeCommand, []string{"sh", "-c", "echo no"}, contracts.Actor("human:owner"), "bad command")
	if err == nil {
		t.Fatal("expected shell interpreter to be rejected")
	}
}

func TestAgentWakeupCreatedForBacklogDependent(t *testing.T) {
	// freshly created tickets default to backlog; the agent should still get
	// poked when the last blocker lands, and the ticket promoted for it
	_, ctx, actions, queries, tickets, now := setupAgentWakeupTest(t)
	blocker := testAgentWorkTicket("APP-1", "Blocker", contracts.StatusInReview, now)
	blocker.ReviewState = contracts.ReviewStateApproved
	dependent := testAgentWorkTicket("APP-2", "Dependent", contracts.StatusBacklog, now)
	dependent.Assignee = contracts.Actor("agent:builder-1")
	dependent.BlockedBy = []string{blocker.ID}
	for _, ticket := range []contracts.TicketSnapshot{blocker, dependent} {
		if err := tickets.CreateTicket(ctx, ticket); err != nil {
			t.Fatalf("create %s: %v", ticket.ID, err)
		}
	}
	if _, err := actions.MoveTicket(ctx, blocker.ID, contracts.StatusDone, contracts.Actor("human:owner"), "blocker done"); err != nil {
		t.Fatalf("complete blocker: %v", err)
	}
	wakeups, err := queries.AgentWakeups(ctx, "builder-1")
	if err != nil {
		t.Fatalf("list wakeups: %v", err)
	}
	if len(wakeups) != 1 {
		t.Fatalf("expected a wakeup for the backlog dependent, got %#v", wakeups)
	}
	if wakeups[0].TicketID != dependent.ID || wakeups[0].BlockerTicketID != blocker.ID || wakeups[0].State != AgentWakeupPending {
		t.Fatalf("unexpected wakeup: %#v", wakeups[0])
	}
	if wakeups[0].Reason != wakeupReasonPromoted {
		t.Fatalf("backlog dependent wakeup should announce the promotion, got %q", wakeups[0].Reason)
	}
	if got, err := tickets.GetTicket(ctx, dependent.ID); err != nil || got.Status != contracts.StatusReady {
		t.Fatalf("expected dependent promoted to ready, got %s (%v)", got.Status, err)
	}
}

func TestAgentWakeupCreatedForBlockedStatusDependent(t *testing.T) {
	_, ctx, actions, queries, tickets, now := setupAgentWakeupTest(t)
	blocker := testAgentWorkTicket("APP-1", "Blocker", contracts.StatusInReview, now)
	blocker.ReviewState = contracts.ReviewStateApproved
	dependent := testAgentWorkTicket("APP-2", "Dependent", contracts.StatusBlocked, now)
	dependent.Assignee = contracts.Actor("agent:builder-1")
	dependent.BlockedBy = []string{blocker.ID}
	for _, ticket := range []contracts.TicketSnapshot{blocker, dependent} {
		if err := tickets.CreateTicket(ctx, ticket); err != nil {
			t.Fatalf("create %s: %v", ticket.ID, err)
		}
	}
	if _, err := actions.MoveTicket(ctx, blocker.ID, contracts.StatusDone, contracts.Actor("human:owner"), "blocker done"); err != nil {
		t.Fatalf("complete blocker: %v", err)
	}
	wakeups, err := queries.AgentWakeups(ctx, "builder-1")
	if err != nil {
		t.Fatalf("list wakeups: %v", err)
	}
	if len(wakeups) != 1 {
		t.Fatalf("expected a wakeup for the blocked dependent, got %#v", wakeups)
	}
	if wakeups[0].TicketID != dependent.ID || wakeups[0].State != AgentWakeupPending {
		t.Fatalf("unexpected wakeup: %#v", wakeups[0])
	}
}

func TestAgentWakeupSkipsBacklogDependentWhenAgentDisabled(t *testing.T) {
	// status is the only obstacle a wakeup may overlook; a disabled agent
	// profile still suppresses it
	root, ctx, actions, queries, tickets, now := setupAgentWakeupTest(t)
	if err := (AgentStore{Root: root}).SaveAgent(ctx, contracts.AgentProfile{AgentID: "builder-1", DisplayName: "Builder", Provider: contracts.AgentProviderCodex, Enabled: false}); err != nil {
		t.Fatalf("disable agent: %v", err)
	}
	blocker := testAgentWorkTicket("APP-1", "Blocker", contracts.StatusInReview, now)
	blocker.ReviewState = contracts.ReviewStateApproved
	dependent := testAgentWorkTicket("APP-2", "Dependent", contracts.StatusBacklog, now)
	dependent.Assignee = contracts.Actor("agent:builder-1")
	dependent.BlockedBy = []string{blocker.ID}
	for _, ticket := range []contracts.TicketSnapshot{blocker, dependent} {
		if err := tickets.CreateTicket(ctx, ticket); err != nil {
			t.Fatalf("create %s: %v", ticket.ID, err)
		}
	}
	if _, err := actions.MoveTicket(ctx, blocker.ID, contracts.StatusDone, contracts.Actor("human:owner"), "blocker done"); err != nil {
		t.Fatalf("complete blocker: %v", err)
	}
	wakeups, err := queries.AgentWakeups(ctx, "builder-1")
	if err != nil {
		t.Fatalf("list wakeups: %v", err)
	}
	if len(wakeups) != 0 {
		t.Fatalf("expected no wakeups for disabled agent, got %#v", wakeups)
	}
}

type wakeupRejectingEventLog struct {
	*eventstore.Log
}

func (l *wakeupRejectingEventLog) AppendEvent(ctx context.Context, event contracts.Event) error {
	if event.Type == contracts.EventAgentWorkAvailable {
		return fmt.Errorf("event log unavailable")
	}
	return l.Log.AppendEvent(ctx, event)
}

func TestAgentWakeupFailureLeavesVisibleRecord(t *testing.T) {
	// wakeups are post-commit side effects, so a failure must never poison the
	// mutation -- but it can't just vanish either
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 5, 9, 16, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	projects := mdstore.ProjectStore{RootDir: root}
	tickets := mdstore.TicketStore{RootDir: root, Clock: clock}
	events := &wakeupRejectingEventLog{Log: &eventstore.Log{RootDir: root}}
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
	actions := NewActionService(root, projects, tickets, events, projection, clock, FileLockManager{Root: root}, nil, nil)
	queries := NewQueryService(root, projects, tickets, events, projection, clock)

	blocker := testAgentWorkTicket("APP-1", "Blocker", contracts.StatusInReview, now)
	blocker.ReviewState = contracts.ReviewStateApproved
	dependent := testAgentWorkTicket("APP-2", "Dependent", contracts.StatusReady, now)
	dependent.Assignee = contracts.Actor("agent:builder-1")
	dependent.BlockedBy = []string{blocker.ID}
	for _, ticket := range []contracts.TicketSnapshot{blocker, dependent} {
		if err := tickets.CreateTicket(ctx, ticket); err != nil {
			t.Fatalf("create %s: %v", ticket.ID, err)
		}
	}
	if _, err := actions.MoveTicket(ctx, blocker.ID, contracts.StatusDone, contracts.Actor("human:owner"), "blocker done"); err != nil {
		t.Fatalf("blocker completion must not fail when wakeup recording fails: %v", err)
	}
	wakeups, err := queries.AgentWakeups(ctx, "builder-1")
	if err != nil {
		t.Fatalf("list wakeups: %v", err)
	}
	if len(wakeups) != 1 {
		t.Fatalf("expected one visible failed wakeup, got %#v", wakeups)
	}
	if wakeups[0].State != AgentWakeupFailed || strings.TrimSpace(wakeups[0].Error) == "" {
		t.Fatalf("expected visible failure record with a reason, got %#v", wakeups[0])
	}
}

// pulls the wakeup + promotion events for a dependent out of the APP stream
func dependentPromotionEvents(t *testing.T, ctx context.Context, actions *ActionService, dependentID string) (promotion contracts.Event, cause contracts.Event) {
	t.Helper()
	events, err := actions.Events.StreamEvents(ctx, "APP", 0)
	if err != nil {
		t.Fatalf("stream events: %v", err)
	}
	byID := map[int64]contracts.Event{}
	found := false
	for _, event := range events {
		byID[event.EventID] = event
		if event.Type == contracts.EventTicketMoved && event.TicketID == dependentID {
			promotion = event
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a ticket.moved for %s, got %#v", dependentID, events)
	}
	cause, ok := byID[promotion.Metadata.CausationEventID]
	if !ok {
		t.Fatalf("promotion causation %d does not point at a logged event: %#v", promotion.Metadata.CausationEventID, promotion)
	}
	return promotion, cause
}

func TestBlockerCompletionPromotesBacklogDependentToReady(t *testing.T) {
	// the wakeup used to point at a ticket no query would show; the dependent
	// stayed in backlog and next/available/queue all came back empty
	_, ctx, actions, queries, tickets, now := setupAgentWakeupTest(t)
	blocker := testAgentWorkTicket("APP-1", "Blocker", contracts.StatusInReview, now)
	blocker.ReviewState = contracts.ReviewStateApproved
	dependent := testAgentWorkTicket("APP-2", "Dependent", contracts.StatusBacklog, now)
	dependent.Assignee = contracts.Actor("agent:builder-1")
	dependent.BlockedBy = []string{blocker.ID}
	for _, ticket := range []contracts.TicketSnapshot{blocker, dependent} {
		if err := tickets.CreateTicket(ctx, ticket); err != nil {
			t.Fatalf("create %s: %v", ticket.ID, err)
		}
	}
	if _, err := actions.MoveTicket(ctx, blocker.ID, contracts.StatusDone, contracts.Actor("human:owner"), "blocker done"); err != nil {
		t.Fatalf("complete blocker: %v", err)
	}

	got, err := tickets.GetTicket(ctx, dependent.ID)
	if err != nil {
		t.Fatalf("get dependent: %v", err)
	}
	if got.Status != contracts.StatusReady {
		t.Fatalf("expected dependent promoted to ready, got %s", got.Status)
	}

	promotion, cause := dependentPromotionEvents(t, ctx, actions, dependent.ID)
	if promotion.Actor != contracts.ActorAtlasSystem {
		t.Fatalf("promotion should be recorded by %s, got %s", contracts.ActorAtlasSystem, promotion.Actor)
	}
	if !strings.Contains(promotion.Reason, blocker.ID) || !strings.Contains(promotion.Reason, "human:owner") {
		t.Fatalf("promotion reason should name the blocker and who completed it, got %q", promotion.Reason)
	}
	raw, _ := json.Marshal(promotion.Payload)
	var payload struct {
		From contracts.Status `json:"from"`
		To   contracts.Status `json:"to"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode promotion payload: %v", err)
	}
	if payload.From != contracts.StatusBacklog || payload.To != contracts.StatusReady {
		t.Fatalf("expected backlog -> ready payload, got %s -> %s", payload.From, payload.To)
	}
	if cause.TicketID != blocker.ID || !ticketCompletionEvent(cause) {
		t.Fatalf("promotion causation should be the blocker completion, got %#v", cause)
	}
	if promotion.Metadata.CorrelationID != cause.Metadata.CorrelationID {
		t.Fatalf("promotion should share the completion correlation id: %q vs %q", promotion.Metadata.CorrelationID, cause.Metadata.CorrelationID)
	}

	wakeups, err := queries.AgentWakeups(ctx, "builder-1")
	if err != nil {
		t.Fatalf("list wakeups: %v", err)
	}
	if len(wakeups) != 1 {
		t.Fatalf("expected one wakeup, got %#v", wakeups)
	}
	if wakeups[0].Reason != "dependency completed; ticket promoted to ready" || wakeups[0].Metadata["promoted"] != "true" {
		t.Fatalf("wakeup should record the promotion, got %#v", wakeups[0])
	}

	view, err := queries.AgentWork(ctx, contracts.Actor("agent:builder-1"))
	if err != nil {
		t.Fatalf("agent work: %v", err)
	}
	if len(view.Available) != 1 || view.Available[0].Ticket.ID != dependent.ID || view.Available[0].Action != "start" {
		t.Fatalf("woken agent should see the dependent as startable, got %#v", view.Available)
	}
}

func TestBlockerCompletionLeavesPersistedBlockedDependentAlone(t *testing.T) {
	// a manual `blocked` may be deliberate (waiting on something outside the
	// tracker), so the agent gets poked but nothing moves
	_, ctx, actions, queries, tickets, now := setupAgentWakeupTest(t)
	blocker := testAgentWorkTicket("APP-1", "Blocker", contracts.StatusInReview, now)
	blocker.ReviewState = contracts.ReviewStateApproved
	dependent := testAgentWorkTicket("APP-2", "Dependent", contracts.StatusBlocked, now)
	dependent.Assignee = contracts.Actor("agent:builder-1")
	dependent.BlockedBy = []string{blocker.ID}
	for _, ticket := range []contracts.TicketSnapshot{blocker, dependent} {
		if err := tickets.CreateTicket(ctx, ticket); err != nil {
			t.Fatalf("create %s: %v", ticket.ID, err)
		}
	}
	if _, err := actions.MoveTicket(ctx, blocker.ID, contracts.StatusDone, contracts.Actor("human:owner"), "blocker done"); err != nil {
		t.Fatalf("complete blocker: %v", err)
	}
	got, err := tickets.GetTicket(ctx, dependent.ID)
	if err != nil {
		t.Fatalf("get dependent: %v", err)
	}
	if got.Status != contracts.StatusBlocked {
		t.Fatalf("persisted blocked ticket must not be promoted, got %s", got.Status)
	}
	wakeups, err := queries.AgentWakeups(ctx, "builder-1")
	if err != nil {
		t.Fatalf("list wakeups: %v", err)
	}
	if len(wakeups) != 1 || wakeups[0].TicketID != dependent.ID {
		t.Fatalf("expected a wakeup for the blocked dependent, got %#v", wakeups)
	}
	if _, ok := wakeups[0].Metadata["promoted"]; ok {
		t.Fatalf("no promotion was attempted, metadata should not say otherwise: %#v", wakeups[0].Metadata)
	}
}

func TestBlockerCompletionDoesNotPromoteWhileAnotherBlockerRemains(t *testing.T) {
	_, ctx, actions, queries, tickets, now := setupAgentWakeupTest(t)
	first := testAgentWorkTicket("APP-1", "First blocker", contracts.StatusInReview, now)
	first.ReviewState = contracts.ReviewStateApproved
	second := testAgentWorkTicket("APP-3", "Second blocker", contracts.StatusInProgress, now)
	dependent := testAgentWorkTicket("APP-2", "Dependent", contracts.StatusBacklog, now)
	dependent.Assignee = contracts.Actor("agent:builder-1")
	dependent.BlockedBy = []string{first.ID, second.ID}
	for _, ticket := range []contracts.TicketSnapshot{first, second, dependent} {
		if err := tickets.CreateTicket(ctx, ticket); err != nil {
			t.Fatalf("create %s: %v", ticket.ID, err)
		}
	}
	if _, err := actions.MoveTicket(ctx, first.ID, contracts.StatusDone, contracts.Actor("human:owner"), "first blocker done"); err != nil {
		t.Fatalf("complete first blocker: %v", err)
	}
	got, err := tickets.GetTicket(ctx, dependent.ID)
	if err != nil {
		t.Fatalf("get dependent: %v", err)
	}
	if got.Status != contracts.StatusBacklog {
		t.Fatalf("dependent still has an open blocker, expected backlog, got %s", got.Status)
	}
	wakeups, err := queries.AgentWakeups(ctx, "builder-1")
	if err != nil {
		t.Fatalf("list wakeups: %v", err)
	}
	if len(wakeups) != 0 {
		t.Fatalf("expected no wakeup while %s is open, got %#v", second.ID, wakeups)
	}
}

// refuses only the dependent's own move so the blocker completion and the
// wakeup event still land
type promotionRejectingEventLog struct {
	*eventstore.Log
	dependentID string
}

func (l *promotionRejectingEventLog) AppendEvent(ctx context.Context, event contracts.Event) error {
	if event.Type == contracts.EventTicketMoved && event.TicketID == l.dependentID {
		return fmt.Errorf("event log unavailable")
	}
	return l.Log.AppendEvent(ctx, event)
}

func TestPromotionFailureStillCreatesWakeup(t *testing.T) {
	// promotion is best-effort: the completion already committed, so a failed
	// nested move must degrade to the plain wakeup and say what went wrong
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 5, 9, 16, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	projects := mdstore.ProjectStore{RootDir: root}
	tickets := mdstore.TicketStore{RootDir: root, Clock: clock}
	events := &promotionRejectingEventLog{Log: &eventstore.Log{RootDir: root}, dependentID: "APP-2"}
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
	actions := NewActionService(root, projects, tickets, events, projection, clock, FileLockManager{Root: root}, nil, nil)
	queries := NewQueryService(root, projects, tickets, events, projection, clock)

	blocker := testAgentWorkTicket("APP-1", "Blocker", contracts.StatusInReview, now)
	blocker.ReviewState = contracts.ReviewStateApproved
	dependent := testAgentWorkTicket("APP-2", "Dependent", contracts.StatusBacklog, now)
	dependent.Assignee = contracts.Actor("agent:builder-1")
	dependent.BlockedBy = []string{blocker.ID}
	for _, ticket := range []contracts.TicketSnapshot{blocker, dependent} {
		if err := tickets.CreateTicket(ctx, ticket); err != nil {
			t.Fatalf("create %s: %v", ticket.ID, err)
		}
	}
	if _, err := actions.MoveTicket(ctx, blocker.ID, contracts.StatusDone, contracts.Actor("human:owner"), "blocker done"); err != nil {
		t.Fatalf("blocker completion must not fail when promotion fails: %v", err)
	}

	// the move wrote the canonical file before the append died, so its journal
	// entry is the only thing that lets doctor finish the job. The wakeup that
	// follows gets the same event id and must not reuse the file.
	journal := MutationJournal{Root: root, Clock: clock}
	entries, err := journal.List()
	if err != nil {
		t.Fatalf("list journal: %v", err)
	}
	if len(entries) != 1 || entries[0].Event.Type != contracts.EventTicketMoved || entries[0].Event.TicketID != dependent.ID {
		t.Fatalf("half-applied promotion must stay in the journal for doctor, got %#v", entries)
	}

	wakeups, err := queries.AgentWakeups(ctx, "builder-1")
	if err != nil {
		t.Fatalf("list wakeups: %v", err)
	}
	if len(wakeups) != 1 {
		t.Fatalf("expected one visible wakeup despite the failed promotion, got %#v", wakeups)
	}
	got := wakeups[0]
	if got.State != AgentWakeupFailed || !strings.Contains(got.Error, "doctor --repair") {
		t.Fatalf("wakeup can't commit on top of a pending journal; expected a failed record naming doctor --repair, got %#v", got)
	}
	if got.Reason != "dependency completed; assigned work is available" {
		t.Fatalf("fallback wakeup should keep the plain reason, got %q", got.Reason)
	}
	if got.Metadata["promoted"] != "false" || !strings.Contains(got.Metadata["promotion_error"], "event log unavailable") {
		t.Fatalf("wakeup should say the promotion failed and carry the real cause, got %#v", got.Metadata)
	}

	// log comes back, doctor replays the move, everything agrees again
	report, err := RepairWorkspace(ctx, root, clock, events.Log, projection)
	if err != nil {
		t.Fatalf("repair workspace: %v", err)
	}
	if report.Pending != 1 {
		t.Fatalf("expected doctor to see the pending promotion, got %#v", report)
	}
	stream, err := events.Log.StreamEvents(ctx, "APP", 0)
	if err != nil {
		t.Fatalf("stream events: %v", err)
	}
	replayed := false
	for _, event := range stream {
		if event.Type == contracts.EventTicketMoved && event.TicketID == dependent.ID {
			replayed = true
		}
	}
	if !replayed {
		t.Fatalf("repair should have replayed the promotion, got %#v", stream)
	}
	after, err := tickets.GetTicket(ctx, dependent.ID)
	if err != nil {
		t.Fatalf("get dependent: %v", err)
	}
	if after.Status != contracts.StatusReady {
		t.Fatalf("expected dependent ready after repair, got %s", after.Status)
	}
}
