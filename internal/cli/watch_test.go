package cli

import (
	"strings"
	"testing"
)

func TestWatchCommandsDriveNotifyRecipients(t *testing.T) {
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
	must("config", "set", "actor.default", "agent:builder-1")
	must("config", "set", "notifications.file_enabled", "true")
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Watch me", "--type", "task", "--actor", "human:owner")
	must("views", "save", "ready-search", "--kind", "search", "--query", "status=backlog")

	listOut := must("watch", "list", "--pretty")
	if !strings.Contains(listOut, "watchers:") {
		t.Fatalf("unexpected empty watcher list output: %s", listOut)
	}

	must("watch", "ticket", "APP-1")
	must("watch", "view", "ready-search", "--actor", "human:owner", "--event", "ticket.commented")

	watchList := must("watch", "list", "--json")
	subscriptions := decodeJSONList[struct {
		Subscription struct {
			Actor      string   `json:"actor"`
			TargetKind string   `json:"target_kind"`
			Target     string   `json:"target"`
			EventTypes []string `json:"event_types"`
		} `json:"subscription"`
		Active         bool   `json:"active"`
		InactiveReason string `json:"inactive_reason"`
	}](t, watchList)
	if len(subscriptions) != 2 {
		t.Fatalf("expected two subscriptions, got %#v", subscriptions)
	}
	if !subscriptions[0].Active || !subscriptions[1].Active {
		t.Fatalf("expected active subscriptions, got %#v", subscriptions)
	}

	must("ticket", "comment", "APP-1", "--body", "ping", "--actor", "human:owner")
	logOut := must("notify", "log", "--json")
	deliveries := decodeJSONList[map[string]any](t, logOut)
	if len(deliveries) == 0 {
		t.Fatalf("expected notification deliveries, got %s", logOut)
	}
	recipients, ok := deliveries[len(deliveries)-1]["recipients"].([]any)
	if !ok || len(recipients) == 0 {
		t.Fatalf("expected recipients in delivery record, got %#v", deliveries[len(deliveries)-1])
	}
	beforeCount := len(deliveries)

	must("unwatch", "ticket", "APP-1")
	must("unwatch", "view", "ready-search", "--actor", "human:owner")
	must("ticket", "comment", "APP-1", "--body", "still quiet", "--actor", "human:owner")
	logOut = must("notify", "log", "--json")
	deliveries = decodeJSONList[map[string]any](t, logOut)
	if len(deliveries) != beforeCount {
		t.Fatalf("expected no new delivery after unwatch, got %#v", deliveries)
	}
}

func TestWatchListMarksInactiveTargetsAndSurvivesReindex(t *testing.T) {
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
	must("config", "set", "actor.default", "agent:builder-1")
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Watch me", "--type", "task", "--actor", "human:owner")
	must("views", "save", "ready-search", "--kind", "search", "--query", "status=backlog")
	must("watch", "ticket", "APP-1")
	must("watch", "view", "ready-search", "--actor", "human:owner")
	must("views", "delete", "ready-search")

	before := decodeJSONList[struct {
		Subscription struct {
			TargetKind string `json:"target_kind"`
			Target     string `json:"target"`
		} `json:"subscription"`
		Active         bool   `json:"active"`
		InactiveReason string `json:"inactive_reason"`
	}](t, must("watch", "list", "--json"))

	if len(before) != 2 {
		t.Fatalf("expected two watchers, got %#v", before)
	}
	if !before[0].Active && before[0].InactiveReason == "" {
		t.Fatalf("expected inactive watcher reason, got %#v", before[0])
	}
	if !before[1].Active && before[1].InactiveReason == "" {
		t.Fatalf("expected inactive watcher reason, got %#v", before[1])
	}

	var sawInactiveView bool
	for _, entry := range before {
		if entry.Subscription.TargetKind == "saved_view" {
			sawInactiveView = !entry.Active && entry.InactiveReason == "missing_saved_view"
		}
	}
	if !sawInactiveView {
		t.Fatalf("expected deleted saved view watcher to be inactive, got %#v", before)
	}

	must("reindex")
	after := decodeJSONList[map[string]any](t, must("watch", "list", "--json"))
	if len(after) != len(before) {
		t.Fatalf("expected watcher count to survive reindex, before=%#v after=%#v", before, after)
	}
}
