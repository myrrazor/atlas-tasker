package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/service"
)

func TestScheduleCLISetListTickHistoryAndClear(t *testing.T) {
	withTempWorkspace(t)
	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
		return out
	}
	must("init")
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Scheduled report", "--type", "task", "--status", "ready", "--actor", "human:owner")
	at := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	must("schedule", "set", "APP-1", "--at", at.Format(time.RFC3339), "--runner", "human:alex", "--actor", "human:owner", "--reason", "plan report")

	listRaw := must("schedule", "list", "--project", "APP", "--json")
	items := decodeJSONList[service.ScheduleEntry](t, listRaw)
	if len(items) != 1 || items[0].Ticket.ID != "APP-1" || items[0].Runner != "human:alex" || items[0].State != service.ScheduleStateScheduled {
		t.Fatalf("unexpected schedule list: %#v", items)
	}

	tickRaw := must("schedule", "tick", "--now", at.Format(time.RFC3339), "--actor", "human:owner", "--reason", "scheduled tick", "--json")
	var tick struct {
		FormatVersion string                      `json:"format_version"`
		Entries       []service.ScheduleTickEntry `json:"entries"`
	}
	if err := json.Unmarshal([]byte(tickRaw), &tick); err != nil {
		t.Fatalf("parse tick json: %v\n%s", err, tickRaw)
	}
	if tick.FormatVersion != jsonFormatVersion || len(tick.Entries) != 1 || tick.Entries[0].State != service.ScheduleStateNotified {
		t.Fatalf("unexpected tick payload: %#v", tick)
	}

	must("ticket", "move", "APP-1", "in_progress", "--actor", "human:alex", "--reason", "start work")
	must("ticket", "request-review", "APP-1", "--actor", "human:alex", "--reason", "ready")
	must("ticket", "approve", "APP-1", "--actor", "human:owner", "--reason", "looks good")
	must("ticket", "complete", "APP-1", "--actor", "human:owner", "--reason", "shipped")
	history := must("schedule", "history", "--project", "APP", "--pretty")
	if !strings.Contains(history, "APP-1") || !strings.Contains(history, "human:owner") {
		t.Fatalf("completion history missing ticket: %s", history)
	}

	must("schedule", "clear", "APP-1", "--actor", "human:owner", "--reason", "archive reminder")
	if got := must("schedule", "list", "--project", "APP", "--pretty"); !strings.Contains(got, "no scheduled tickets") {
		t.Fatalf("expected empty schedule after clear: %s", got)
	}
}

func TestScheduleCLIRequiresTimestampTimezoneAndReason(t *testing.T) {
	withTempWorkspace(t)
	_, _ = runCLI(t, "init")
	_, _ = runCLI(t, "project", "create", "APP", "App Project")
	_, _ = runCLI(t, "ticket", "create", "--project", "APP", "--title", "Task", "--type", "task", "--actor", "human:owner")
	if _, err := runCLI(t, "schedule", "set", "APP-1", "--at", "2026-08-07 10:00", "--runner", "human:owner", "--actor", "human:owner", "--reason", "test"); err == nil {
		t.Fatal("expected timezone-free timestamp to fail")
	}
	if _, err := runCLI(t, "schedule", "set", "APP-1", "--at", time.Now().Add(time.Hour).Format(time.RFC3339), "--runner", "human:owner", "--actor", "human:owner"); err == nil {
		t.Fatal("expected missing reason to fail")
	}
}
