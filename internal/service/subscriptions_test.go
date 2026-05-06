package service

import (
	"context"
	"os"
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

func TestSubscriptionResolverAudience(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 24, 2, 0, 0, 0, time.UTC)

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer projection.Close()

	if err := config.Save(root, contracts.TrackerConfig{
		Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen},
		Actor:    contracts.ActorConfig{Default: contracts.Actor("agent:builder-1")},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket := contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Ready work",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	event := contracts.Event{
		EventID:       1,
		Timestamp:     now,
		Actor:         contracts.Actor("human:owner"),
		Type:          contracts.EventTicketReviewRequested,
		Project:       "APP",
		TicketID:      "APP-1",
		Payload:       ticket,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := eventsLog.AppendEvent(ctx, event); err != nil {
		t.Fatalf("append event: %v", err)
	}
	if err := projection.ApplyEvent(ctx, event); err != nil {
		t.Fatalf("apply event: %v", err)
	}

	if err := (ViewStore{Root: root}).SaveView(contracts.SavedView{
		Name:  "ready-search",
		Kind:  contracts.SavedViewKindSearch,
		Query: "status=ready",
	}); err != nil {
		t.Fatalf("save view: %v", err)
	}
	store := SubscriptionStore{Root: root}
	for _, subscription := range []contracts.Subscription{
		{Actor: contracts.Actor("agent:builder-1"), TargetKind: contracts.SubscriptionTargetTicket, Target: "APP-1"},
		{Actor: contracts.Actor("agent:reviewer-1"), TargetKind: contracts.SubscriptionTargetProject, Target: "APP"},
		{Actor: contracts.Actor("human:owner"), TargetKind: contracts.SubscriptionTargetSavedView, Target: "ready-search", EventTypes: []contracts.EventType{contracts.EventTicketReviewRequested}},
	} {
		if err := store.SaveSubscription(subscription); err != nil {
			t.Fatalf("save subscription: %v", err)
		}
	}

	queries := NewQueryService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now })
	audience, err := SubscriptionResolver{Store: store, Queries: queries}.Audience(ctx, event)
	if err != nil {
		t.Fatalf("resolve audience: %v", err)
	}
	if !audience.HasSubscriptions {
		t.Fatal("expected subscriptions to be present")
	}
	if len(audience.Recipients) != 3 {
		t.Fatalf("unexpected recipients: %#v", audience.Recipients)
	}
}

func TestBuildNotifierUsesSubscriptions(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 24, 3, 0, 0, 0, time.UTC)

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer projection.Close()

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket := contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Watch me",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityMedium,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	createEvent := contracts.Event{
		EventID:       1,
		Timestamp:     now,
		Actor:         contracts.Actor("human:owner"),
		Type:          contracts.EventTicketCreated,
		Project:       "APP",
		TicketID:      "APP-1",
		Payload:       ticket,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := eventsLog.AppendEvent(ctx, createEvent); err != nil {
		t.Fatalf("append event: %v", err)
	}
	if err := projection.ApplyEvent(ctx, createEvent); err != nil {
		t.Fatalf("apply event: %v", err)
	}

	store := SubscriptionStore{Root: root}
	subscription := contracts.Subscription{Actor: contracts.Actor("agent:builder-1"), TargetKind: contracts.SubscriptionTargetTicket, Target: "APP-1"}
	if err := store.SaveSubscription(subscription); err != nil {
		t.Fatalf("save subscription: %v", err)
	}
	queries := NewQueryService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now })
	notifier, err := BuildNotifier(root, contracts.TrackerConfig{
		Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen},
		Notifications: contracts.NotificationsConfig{
			FileEnabled:     true,
			FilePath:        ".tracker/test-notify.log",
			DeliveryLogPath: ".tracker/delivery.log",
			DeadLetterPath:  ".tracker/dead.log",
		},
	}, nil, SubscriptionResolver{Store: store, Queries: queries})
	if err != nil {
		t.Fatalf("build notifier: %v", err)
	}

	unmatched := contracts.Event{
		EventID:       2,
		Timestamp:     now,
		Actor:         contracts.Actor("human:owner"),
		Type:          contracts.EventTicketCommented,
		Project:       "APP",
		TicketID:      "APP-2",
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := notifier.Notify(ctx, unmatched); err != nil {
		t.Fatalf("notify unmatched event: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".tracker/test-notify.log")); err == nil {
		t.Fatal("expected unmatched event to skip file delivery when watchers exist")
	}

	matched := contracts.Event{
		EventID:       3,
		Timestamp:     now,
		Actor:         contracts.Actor("human:owner"),
		Type:          contracts.EventTicketApproved,
		Project:       "APP",
		TicketID:      "APP-1",
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := notifier.Notify(ctx, matched); err != nil {
		t.Fatalf("notify matched event: %v", err)
	}
	records, err := ReadNotificationLog(root, contracts.TrackerConfig{
		Notifications: contracts.NotificationsConfig{
			DeliveryLogPath: ".tracker/delivery.log",
			DeadLetterPath:  ".tracker/dead.log",
		},
	})
	if err != nil {
		t.Fatalf("read notification log: %v", err)
	}
	if len(records) != 1 || len(records[0].Recipients) != 1 || records[0].Recipients[0] != contracts.Actor("agent:builder-1") {
		t.Fatalf("unexpected delivery records: %#v", records)
	}
}

func TestSubscriptionResolverSkipsFilteredEventTypes(t *testing.T) {
	root := t.TempDir()
	store := SubscriptionStore{Root: root}
	subscription := contracts.Subscription{
		Actor:      contracts.Actor("agent:builder-1"),
		TargetKind: contracts.SubscriptionTargetTicket,
		Target:     "APP-1",
		EventTypes: []contracts.EventType{contracts.EventTicketApproved},
	}
	if err := store.SaveSubscription(subscription); err != nil {
		t.Fatalf("save subscription: %v", err)
	}
	audience, err := SubscriptionResolver{Store: store}.Audience(context.Background(), contracts.Event{
		EventID:       1,
		Timestamp:     time.Date(2026, 3, 24, 6, 0, 0, 0, time.UTC),
		Actor:         contracts.Actor("human:owner"),
		Type:          contracts.EventTicketCommented,
		Project:       "APP",
		TicketID:      "APP-1",
		SchemaVersion: contracts.CurrentSchemaVersion,
	})
	if err != nil {
		t.Fatalf("resolve audience: %v", err)
	}
	if len(audience.Recipients) != 0 {
		t.Fatalf("expected filtered event type to skip recipients, got %#v", audience)
	}
}

func TestSubscriptionResolverSkipsBrokenSavedViewWatchers(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 24, 7, 0, 0, 0, time.UTC)

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer projection.Close()

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket := contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Watch me",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityMedium,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	event := contracts.Event{
		EventID:       1,
		Timestamp:     now,
		Actor:         contracts.Actor("human:owner"),
		Type:          contracts.EventTicketCommented,
		Project:       "APP",
		TicketID:      "APP-1",
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := (ViewStore{Root: root}).SaveView(contracts.SavedView{
		Name:  "ready-search",
		Kind:  contracts.SavedViewKindSearch,
		Query: "status=ready",
	}); err != nil {
		t.Fatalf("save view: %v", err)
	}
	store := SubscriptionStore{Root: root}
	if err := store.SaveSubscription(contracts.Subscription{
		Actor:      contracts.Actor("agent:builder-1"),
		TargetKind: contracts.SubscriptionTargetTicket,
		Target:     "APP-1",
	}); err != nil {
		t.Fatalf("save ticket subscription: %v", err)
	}
	if err := store.SaveSubscription(contracts.Subscription{
		Actor:      contracts.Actor("human:owner"),
		TargetKind: contracts.SubscriptionTargetSavedView,
		Target:     "ready-search",
	}); err != nil {
		t.Fatalf("save view subscription: %v", err)
	}
	if err := os.Remove(filepath.Join(storage.ViewsDir(root), "ready-search.toml")); err != nil {
		t.Fatalf("remove saved view: %v", err)
	}

	queries := NewQueryService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now })
	audience, err := SubscriptionResolver{Store: store, Queries: queries}.Audience(ctx, event)
	if err != nil {
		t.Fatalf("resolve audience: %v", err)
	}
	if len(audience.Recipients) != 1 || audience.Recipients[0] != contracts.Actor("agent:builder-1") {
		t.Fatalf("expected healthy ticket watcher to survive broken saved view watcher, got %#v", audience)
	}
}

func TestQueryServiceListSubscriptionsMarksInactiveTargets(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 24, 8, 0, 0, 0, time.UTC)

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer projection.Close()

	project := contracts.Project{Key: "APP", Name: "App", CreatedAt: now}
	if err := projectStore.CreateProject(ctx, project); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket := contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Watch me",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityMedium,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if err := (ViewStore{Root: root}).SaveView(contracts.SavedView{
		Name:  "ready-search",
		Kind:  contracts.SavedViewKindSearch,
		Query: "status=ready",
	}); err != nil {
		t.Fatalf("save view: %v", err)
	}
	store := SubscriptionStore{Root: root}
	for _, subscription := range []contracts.Subscription{
		{Actor: contracts.Actor("agent:builder-1"), TargetKind: contracts.SubscriptionTargetTicket, Target: "APP-1"},
		{Actor: contracts.Actor("agent:reviewer-1"), TargetKind: contracts.SubscriptionTargetProject, Target: "APP"},
		{Actor: contracts.Actor("human:owner"), TargetKind: contracts.SubscriptionTargetSavedView, Target: "ready-search"},
	} {
		if err := store.SaveSubscription(subscription); err != nil {
			t.Fatalf("save subscription: %v", err)
		}
	}

	ticket.Archived = true
	if err := ticketStore.UpdateTicket(ctx, ticket); err != nil {
		t.Fatalf("archive ticket: %v", err)
	}
	if err := os.Remove(storage.ProjectFile(root, "APP")); err != nil {
		t.Fatalf("remove project file: %v", err)
	}
	if err := os.Remove(filepath.Join(storage.ViewsDir(root), "ready-search.toml")); err != nil {
		t.Fatalf("remove view file: %v", err)
	}

	queries := NewQueryService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now })
	items, err := queries.ListSubscriptions(ctx, "")
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected three subscriptions, got %#v", items)
	}

	reasons := map[contracts.SubscriptionTargetKind]string{}
	for _, item := range items {
		if item.Active {
			t.Fatalf("expected all targets to be inactive, got %#v", items)
		}
		reasons[item.Subscription.TargetKind] = item.InactiveReason
	}
	if reasons[contracts.SubscriptionTargetTicket] != "ticket_inactive" {
		t.Fatalf("unexpected ticket inactive reason: %#v", reasons)
	}
	if reasons[contracts.SubscriptionTargetProject] != "missing_project" {
		t.Fatalf("unexpected project inactive reason: %#v", reasons)
	}
	if reasons[contracts.SubscriptionTargetSavedView] != "missing_saved_view" {
		t.Fatalf("unexpected view inactive reason: %#v", reasons)
	}
}
