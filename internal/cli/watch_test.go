package cli

import (
	"encoding/json"
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
	var subscriptions []map[string]any
	if err := json.Unmarshal([]byte(watchList), &subscriptions); err != nil {
		t.Fatalf("parse watch list json: %v\nraw=%s", err, watchList)
	}
	if len(subscriptions) != 2 {
		t.Fatalf("expected two subscriptions, got %#v", subscriptions)
	}

	must("ticket", "comment", "APP-1", "--body", "ping", "--actor", "human:owner")
	logOut := must("notify", "log", "--json")
	var deliveries []map[string]any
	if err := json.Unmarshal([]byte(logOut), &deliveries); err != nil {
		t.Fatalf("parse notify log: %v\nraw=%s", err, logOut)
	}
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
	if err := json.Unmarshal([]byte(logOut), &deliveries); err != nil {
		t.Fatalf("parse notify log after unwatch: %v\nraw=%s", err, logOut)
	}
	if len(deliveries) != beforeCount {
		t.Fatalf("expected no new delivery after unwatch, got %#v", deliveries)
	}
}
