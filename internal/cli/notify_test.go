package cli

import (
	"encoding/json"
	"testing"
)

func TestNotifyCommands(t *testing.T) {
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
	must("config", "set", "notifications.file_enabled", "true")
	must("config", "set", "notifications.file_path", ".tracker/ops-notify.log")
	must("notify", "send", "--event-type", "ticket.approved", "--project", "APP", "--ticket", "APP-1", "--reason", "smoke")

	logOut := must("notify", "log", "--json")
	var deliveries []map[string]any
	if err := json.Unmarshal([]byte(logOut), &deliveries); err != nil {
		t.Fatalf("parse notify log: %v\nraw=%s", err, logOut)
	}
	if len(deliveries) == 0 {
		t.Fatalf("expected notify log entries, got %s", logOut)
	}

	deadOut := must("notify", "dead-letter", "--json")
	var deadLetters []map[string]any
	if err := json.Unmarshal([]byte(deadOut), &deadLetters); err != nil {
		t.Fatalf("parse dead letters: %v\nraw=%s", err, deadOut)
	}
	if len(deadLetters) != 0 {
		t.Fatalf("expected no dead letters, got %s", deadOut)
	}
}
