package service

import (
	"bytes"
	"context"
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
			Terminal:    true,
			FileEnabled: true,
			FilePath:    ".tracker/test-notify.log",
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
}
