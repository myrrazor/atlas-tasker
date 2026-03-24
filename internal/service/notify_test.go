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
	}, &stderr)
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
	}, nil)
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
	}, nil)
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
