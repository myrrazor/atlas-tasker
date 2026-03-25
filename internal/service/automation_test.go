package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

func setupAutomationTest(t *testing.T) (context.Context, *ActionService, *QueryService, *sqlitestore.Store, contracts.TicketSnapshot, *[]contracts.Event) {
	t.Helper()
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = projection.Close() })
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	project := contracts.Project{Key: "APP", Name: "App", CreatedAt: now}
	if err := projectStore.CreateProject(ctx, project); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket := contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Watch me",
		Summary:       "Watch me",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusInProgress,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	seed := contracts.Event{
		EventID:       1,
		Timestamp:     now,
		Actor:         contracts.Actor("human:owner"),
		Type:          contracts.EventTicketCreated,
		Project:       "APP",
		TicketID:      ticket.ID,
		Payload:       ticket,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := eventsLog.AppendEvent(ctx, seed); err != nil {
		t.Fatalf("append seed event: %v", err)
	}
	if err := projection.ApplyEvent(ctx, seed); err != nil {
		t.Fatalf("project seed event: %v", err)
	}
	var notifications []contracts.Event
	notifier := notifierFunc(func(_ context.Context, event contracts.Event) error {
		notifications = append(notifications, event)
		return nil
	})
	automation := &AutomationEngine{
		Store:    AutomationStore{Root: root},
		Notifier: notifier,
	}
	actions := NewActionService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now }, FileLockManager{Root: root}, notifier, automation)
	queries := NewQueryService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now })
	return ctx, actions, queries, projection, ticket, &notifications
}

func TestAutomationDryRunDoesNotMutate(t *testing.T) {
	ctx, actions, queries, projection, ticket, _ := setupAutomationTest(t)
	rule := contracts.AutomationRule{
		Name:    "dry-run-review",
		Enabled: true,
		Trigger: contracts.AutomationTrigger{EventTypes: []contracts.EventType{contracts.EventTicketMoved}},
		Conditions: contracts.AutomationCondition{
			Status: contracts.StatusInProgress,
		},
		Actions: []contracts.AutomationAction{{Kind: contracts.AutomationActionRequestReview}},
	}
	if err := actions.Automation.SaveRule(rule); err != nil {
		t.Fatalf("save rule: %v", err)
	}
	before, err := projection.QueryHistory(ctx, ticket.ID)
	if err != nil {
		t.Fatalf("history before: %v", err)
	}
	event := contracts.Event{
		EventID:       99,
		Timestamp:     time.Now().UTC(),
		Actor:         contracts.Actor("agent:builder-1"),
		Type:          contracts.EventTicketMoved,
		Project:       ticket.Project,
		TicketID:      ticket.ID,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	result, err := actions.Automation.DryRun(ctx, queries, rule, event, ticket.ID)
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if !result.Matched || len(result.Actions) != 1 {
		t.Fatalf("unexpected dry-run result: %#v", result)
	}
	after, err := projection.QueryHistory(ctx, ticket.ID)
	if err != nil {
		t.Fatalf("history after: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("dry-run should not mutate history: before=%d after=%d", len(before), len(after))
	}
}

func TestAutomationCommentActionRunsOnceAndKeepsMetadata(t *testing.T) {
	ctx, actions, _, projection, ticket, _ := setupAutomationTest(t)
	rule := contracts.AutomationRule{
		Name:    "comment-on-comment",
		Enabled: true,
		Trigger: contracts.AutomationTrigger{EventTypes: []contracts.EventType{contracts.EventTicketCommented}},
		Actions: []contracts.AutomationAction{{Kind: contracts.AutomationActionComment, Body: "automation follow-up"}},
	}
	if err := actions.Automation.SaveRule(rule); err != nil {
		t.Fatalf("save rule: %v", err)
	}
	ctx = WithEventMetadata(ctx, EventMetaContext{Surface: contracts.EventSurfaceShell})
	if err := actions.CommentTicket(ctx, ticket.ID, "human note", contracts.Actor("agent:builder-1"), "manual comment"); err != nil {
		t.Fatalf("comment ticket: %v", err)
	}
	history, err := projection.QueryHistory(ctx, ticket.ID)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != 3 {
		t.Fatalf("expected create + manual comment + automation comment, got %#v", history)
	}
	last := history[len(history)-1]
	if last.Type != contracts.EventTicketCommented {
		t.Fatalf("expected automation comment event, got %#v", last)
	}
	if last.Actor != contracts.Actor("agent:automation") {
		t.Fatalf("expected automation actor, got %#v", last)
	}
	if last.Metadata.Surface != contracts.EventSurfaceAutomation {
		t.Fatalf("expected automation surface, got %#v", last.Metadata)
	}
	if last.Metadata.CausationEventID != history[len(history)-2].EventID {
		t.Fatalf("expected causation to point at manual comment: %#v", last.Metadata)
	}
	if last.Metadata.CorrelationID != history[len(history)-2].Metadata.CorrelationID {
		t.Fatalf("expected automation correlation to match parent: %#v", last.Metadata)
	}
}

func TestAutomationNotifyActionUsesNotifier(t *testing.T) {
	ctx, actions, _, _, ticket, notifications := setupAutomationTest(t)
	rule := contracts.AutomationRule{
		Name:    "notify-review",
		Enabled: true,
		Trigger: contracts.AutomationTrigger{EventTypes: []contracts.EventType{contracts.EventTicketReviewRequested}},
		Actions: []contracts.AutomationAction{{Kind: contracts.AutomationActionNotify, Message: "review inbox"}},
	}
	if err := actions.Automation.SaveRule(rule); err != nil {
		t.Fatalf("save rule: %v", err)
	}
	if _, err := actions.RequestReview(ctx, ticket.ID, contracts.Actor("agent:builder-1"), "send to review"); err != nil {
		t.Fatalf("request review: %v", err)
	}
	if len(*notifications) == 0 {
		t.Fatal("expected notifier to receive automation event")
	}
	last := (*notifications)[len(*notifications)-1]
	if last.Type != contracts.EventOwnerAttentionRaised || last.Reason != "review inbox" {
		t.Fatalf("unexpected notifier event: %#v", last)
	}
}
