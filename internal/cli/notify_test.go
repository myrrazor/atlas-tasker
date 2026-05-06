package cli

import "testing"

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
	deliveries := decodeJSONList[map[string]any](t, logOut)
	if len(deliveries) == 0 {
		t.Fatalf("expected notify log entries, got %s", logOut)
	}
	if deliveries[0]["event_summary"] == "" {
		t.Fatalf("expected event summary in notify log, got %#v", deliveries[0])
	}

	deadOut := must("notify", "dead-letter", "--json")
	deadLetters := decodeJSONList[map[string]any](t, deadOut)
	if len(deadLetters) != 0 {
		t.Fatalf("expected no dead letters, got %s", deadOut)
	}
}
