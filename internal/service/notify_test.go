package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestBuildNotifierWritesTerminalAndFile(t *testing.T) {
	root := t.TempDir()
	var stderr bytes.Buffer
	notifier, err := BuildNotifier(root, contracts.TrackerConfig{
		Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen},
		Notifications: contracts.NotificationsConfig{
			Terminal:              true,
			FileEnabled:           true,
			FilePath:              ".tracker/test-notify.log",
			WebhookTimeoutSeconds: 3,
			WebhookRetries:        1,
			DeliveryLogPath:       ".tracker/delivery.log",
			DeadLetterPath:        ".tracker/dead.log",
		},
	}, &stderr, SubscriptionResolver{Store: SubscriptionStore{Root: root}})
	if err != nil {
		t.Fatalf("build notifier: %v", err)
	}
	event := contracts.Event{
		EventID:       1,
		Timestamp:     time.Date(2026, 3, 23, 4, 0, 0, 0, time.UTC),
		Actor:         contracts.Actor("human:owner"),
		Type:          contracts.EventTicketApproved,
		Project:       "APP",
		TicketID:      "APP-1",
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := notifier.Notify(context.Background(), event); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if !strings.Contains(stderr.String(), "ticket.approved APP-1") {
		t.Fatalf("unexpected terminal output: %s", stderr.String())
	}
	raw, err := os.ReadFile(filepath.Join(root, ".tracker/test-notify.log"))
	if err != nil {
		t.Fatalf("read notify file: %v", err)
	}
	if !strings.Contains(string(raw), "\"ticket_id\":\"APP-1\"") {
		t.Fatalf("unexpected notify file contents: %s", string(raw))
	}
	deliveries, err := ReadNotificationLog(root, contracts.TrackerConfig{
		Notifications: contracts.NotificationsConfig{
			DeliveryLogPath: ".tracker/delivery.log",
			DeadLetterPath:  ".tracker/dead.log",
		},
	})
	if err != nil {
		t.Fatalf("read delivery log: %v", err)
	}
	if len(deliveries) < 2 {
		t.Fatalf("expected delivery records for terminal and file, got %#v", deliveries)
	}
}

func TestBuildNotifierRetriesWebhookAndWritesDeadLetter(t *testing.T) {
	root := t.TempDir()
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("nope"))
	}))
	defer server.Close()

	notifier, err := BuildNotifier(root, contracts.TrackerConfig{
		Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen},
		Notifications: contracts.NotificationsConfig{
			WebhookURL:            server.URL,
			WebhookTimeoutSeconds: 3,
			WebhookRetries:        1,
			DeliveryLogPath:       ".tracker/delivery.log",
			DeadLetterPath:        ".tracker/dead.log",
		},
	}, nil, SubscriptionResolver{Store: SubscriptionStore{Root: root}})
	if err != nil {
		t.Fatalf("build notifier: %v", err)
	}
	event := contracts.Event{
		EventID:       1,
		Timestamp:     time.Date(2026, 3, 23, 4, 0, 0, 0, time.UTC),
		Actor:         contracts.Actor("human:owner"),
		Type:          contracts.EventTicketApproved,
		Project:       "APP",
		TicketID:      "APP-1",
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := notifier.Notify(context.Background(), event); err == nil {
		t.Fatal("expected webhook failure to bubble up")
	}
	if attempts != 2 {
		t.Fatalf("expected one retry, got %d attempts", attempts)
	}
	deadLetters, err := ReadDeadLetters(root, contracts.TrackerConfig{
		Notifications: contracts.NotificationsConfig{
			DeliveryLogPath: ".tracker/delivery.log",
			DeadLetterPath:  ".tracker/dead.log",
		},
	})
	if err != nil {
		t.Fatalf("read dead letters: %v", err)
	}
	if len(deadLetters) != 1 || deadLetters[0].Sink != "webhook" || deadLetters[0].Delivered {
		t.Fatalf("unexpected dead letters: %#v", deadLetters)
	}
	if deadLetters[0].EventSummary == "" {
		t.Fatalf("expected event summary in dead letter, got %#v", deadLetters[0])
	}
}

func TestBuildNotifierWebhookPayload(t *testing.T) {
	root := t.TempDir()
	var payload notificationPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	notifier, err := BuildNotifier(root, contracts.TrackerConfig{
		Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen},
		Notifications: contracts.NotificationsConfig{
			WebhookURL:            server.URL,
			WebhookTimeoutSeconds: 3,
			WebhookRetries:        0,
			DeliveryLogPath:       ".tracker/delivery.log",
			DeadLetterPath:        ".tracker/dead.log",
		},
	}, nil, SubscriptionResolver{Store: SubscriptionStore{Root: root}})
	if err != nil {
		t.Fatalf("build notifier: %v", err)
	}
	event := contracts.Event{
		EventID:       1,
		Timestamp:     time.Date(2026, 3, 23, 4, 0, 0, 0, time.UTC),
		Actor:         contracts.Actor("human:owner"),
		Type:          contracts.EventTicketApproved,
		Project:       "APP",
		TicketID:      "APP-1",
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := notifier.Notify(context.Background(), event); err != nil {
		t.Fatalf("notify webhook: %v", err)
	}
	if payload.Event.TicketID != "APP-1" || payload.Event.Type != contracts.EventTicketApproved || payload.DeliveredAt.IsZero() {
		t.Fatalf("unexpected webhook payload: %#v", payload)
	}
}

func TestBuildNotifierGatesDeliveryWhenWatchersExist(t *testing.T) {
	root := t.TempDir()
	store := SubscriptionStore{Root: root}
	subscription := contracts.Subscription{
		Actor:      contracts.Actor("agent:builder-1"),
		TargetKind: contracts.SubscriptionTargetTicket,
		Target:     "APP-1",
	}
	if err := store.SaveSubscription(subscription); err != nil {
		t.Fatalf("save subscription: %v", err)
	}

	notifier, err := BuildNotifier(root, contracts.TrackerConfig{
		Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen},
		Notifications: contracts.NotificationsConfig{
			FileEnabled:     true,
			FilePath:        ".tracker/test-notify.log",
			DeliveryLogPath: ".tracker/delivery.log",
			DeadLetterPath:  ".tracker/dead.log",
		},
	}, nil, SubscriptionResolver{Store: store})
	if err != nil {
		t.Fatalf("build notifier: %v", err)
	}

	unmatched := contracts.Event{
		EventID:       1,
		Timestamp:     time.Date(2026, 3, 24, 5, 0, 0, 0, time.UTC),
		Actor:         contracts.Actor("human:owner"),
		Type:          contracts.EventTicketCommented,
		Project:       "APP",
		TicketID:      "APP-2",
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := notifier.Notify(context.Background(), unmatched); err != nil {
		t.Fatalf("notify unmatched: %v", err)
	}
	deliveries, err := ReadNotificationLog(root, contracts.TrackerConfig{
		Notifications: contracts.NotificationsConfig{
			DeliveryLogPath: ".tracker/delivery.log",
			DeadLetterPath:  ".tracker/dead.log",
		},
	})
	if err != nil {
		t.Fatalf("read delivery log: %v", err)
	}
	if len(deliveries) != 0 {
		t.Fatalf("expected no deliveries for unmatched watcher event, got %#v", deliveries)
	}

	matched := unmatched
	matched.EventID = 2
	matched.TicketID = "APP-1"
	if err := notifier.Notify(context.Background(), matched); err != nil {
		t.Fatalf("notify matched: %v", err)
	}
	deliveries, err = ReadNotificationLog(root, contracts.TrackerConfig{
		Notifications: contracts.NotificationsConfig{
			DeliveryLogPath: ".tracker/delivery.log",
			DeadLetterPath:  ".tracker/dead.log",
		},
	})
	if err != nil {
		t.Fatalf("read delivery log after match: %v", err)
	}
	if len(deliveries) != 1 || len(deliveries[0].Recipients) != 1 || deliveries[0].Recipients[0] != contracts.Actor("agent:builder-1") {
		t.Fatalf("unexpected watcher delivery records: %#v", deliveries)
	}
}

func TestBuildNotifierGracefullyDegradesOnResolverErrorForLegacyEvents(t *testing.T) {
	root := t.TempDir()
	subscriptionsPath := storage.SubscriptionsDir(root)
	if err := os.MkdirAll(filepath.Dir(subscriptionsPath), 0o755); err != nil {
		t.Fatalf("create tracker dir: %v", err)
	}
	if err := os.WriteFile(subscriptionsPath, []byte("not-a-dir"), 0o644); err != nil {
		t.Fatalf("write blocking subscriptions file: %v", err)
	}

	notifier, err := BuildNotifier(root, contracts.TrackerConfig{
		Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen},
		Notifications: contracts.NotificationsConfig{
			FileEnabled:     true,
			FilePath:        ".tracker/test-notify.log",
			DeliveryLogPath: ".tracker/delivery.log",
			DeadLetterPath:  ".tracker/dead.log",
		},
	}, nil, SubscriptionResolver{Store: SubscriptionStore{Root: root}})
	if err != nil {
		t.Fatalf("build notifier: %v", err)
	}
	event := contracts.Event{
		EventID:       1,
		Timestamp:     time.Date(2026, 3, 24, 5, 30, 0, 0, time.UTC),
		Actor:         contracts.Actor("human:owner"),
		Type:          contracts.EventTicketApproved,
		Project:       "APP",
		TicketID:      "APP-1",
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := notifier.Notify(context.Background(), event); err != nil {
		t.Fatalf("expected legacy delivery to ignore resolver error, got %v", err)
	}
	deliveries, err := ReadNotificationLog(root, contracts.TrackerConfig{
		Notifications: contracts.NotificationsConfig{
			DeliveryLogPath: ".tracker/delivery.log",
			DeadLetterPath:  ".tracker/dead.log",
		},
	})
	if err != nil {
		t.Fatalf("read delivery log: %v", err)
	}
	if len(deliveries) != 1 || len(deliveries[0].Recipients) != 0 {
		t.Fatalf("unexpected fallback delivery records: %#v", deliveries)
	}
}

func TestBuildNotifierClampsWebhookBoundsAndMasksErrors(t *testing.T) {
	root := t.TempDir()
	notifier, err := BuildNotifier(root, contracts.TrackerConfig{
		Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen},
		Notifications: contracts.NotificationsConfig{
			WebhookURL:            "https://bot:secret@example.com/hook?token=abc123",
			WebhookTimeoutSeconds: contracts.MaxWebhookTimeoutSeconds + 10,
			WebhookRetries:        contracts.MaxWebhookRetries + 10,
			DeliveryLogPath:       ".tracker/delivery.log",
			DeadLetterPath:        ".tracker/dead.log",
		},
	}, nil, SubscriptionResolver{Store: SubscriptionStore{Root: root}})
	if err != nil {
		t.Fatalf("build notifier: %v", err)
	}

	event := contracts.Event{
		EventID:       1,
		Timestamp:     time.Date(2026, 3, 23, 4, 0, 0, 0, time.UTC),
		Actor:         contracts.Actor("human:owner"),
		Type:          contracts.EventTicketApproved,
		Project:       "APP",
		TicketID:      "APP-1",
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := notifier.Notify(context.Background(), event); err == nil {
		t.Fatal("expected webhook notify to fail")
	}

	deliveries, err := ReadNotificationLog(root, contracts.TrackerConfig{
		Notifications: contracts.NotificationsConfig{
			DeliveryLogPath: ".tracker/delivery.log",
			DeadLetterPath:  ".tracker/dead.log",
		},
	})
	if err != nil {
		t.Fatalf("read delivery log: %v", err)
	}
	if len(deliveries) != contracts.MaxWebhookRetries+1 {
		t.Fatalf("expected retries to clamp to %d attempts, got %d", contracts.MaxWebhookRetries+1, len(deliveries))
	}
	for _, delivery := range deliveries {
		if strings.Contains(delivery.Error, "secret") || strings.Contains(delivery.Error, "abc123") {
			t.Fatalf("expected masked delivery error, got %#v", delivery)
		}
	}

	deadLetters, err := ReadDeadLetters(root, contracts.TrackerConfig{
		Notifications: contracts.NotificationsConfig{
			DeliveryLogPath: ".tracker/delivery.log",
			DeadLetterPath:  ".tracker/dead.log",
		},
	})
	if err != nil {
		t.Fatalf("read dead letters: %v", err)
	}
	if len(deadLetters) != 1 {
		t.Fatalf("expected one dead letter, got %#v", deadLetters)
	}
	if strings.Contains(deadLetters[0].Error, "secret") || strings.Contains(deadLetters[0].Error, "abc123") {
		t.Fatalf("expected masked dead letter error, got %#v", deadLetters[0])
	}
}
