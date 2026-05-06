package cli

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
)

func TestAutomationCLIWorkflow(t *testing.T) {
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
	must("ticket", "create", "--project", "APP", "--title", "Automate me", "--type", "task", "--actor", "human:owner")
	must("automation", "create", "review-ready", "--on", "ticket.moved", "--action", "request_review")

	listOut := must("automation", "list", "--json")
	rules := decodeJSONList[contracts.AutomationRule](t, listOut)
	if len(rules) != 1 || rules[0].Name != "review-ready" {
		t.Fatalf("unexpected rules: %#v", rules)
	}

	explainOut := must("automation", "explain", "review-ready", "--ticket", "APP-1", "--event-type", "ticket.moved", "--actor", "agent:builder-1", "--json")
	var result struct {
		FormatVersion string   `json:"format_version"`
		Matched       bool     `json:"matched"`
		Actions       []string `json:"actions"`
	}
	if err := json.Unmarshal([]byte(explainOut), &result); err != nil {
		t.Fatalf("parse automation explain: %v\nraw=%s", err, explainOut)
	}
	if result.FormatVersion != jsonFormatVersion {
		t.Fatalf("unexpected format version: %s", result.FormatVersion)
	}
	if !result.Matched || len(result.Actions) != 1 {
		t.Fatalf("unexpected explain result: %#v", result)
	}
}

func TestAutomationCreateEditSemantics(t *testing.T) {
	withTempWorkspace(t)

	must := func(args ...string) {
		t.Helper()
		if _, err := runCLI(t, args...); err != nil {
			t.Fatalf("%v failed: %v", args, err)
		}
	}

	must("init")
	if _, err := runCLI(t, "automation", "edit", "missing", "--on", "ticket.moved", "--action", "notify:hi"); err == nil {
		t.Fatal("expected automation edit to fail for missing rule")
	}
	must("automation", "create", "existing", "--on", "ticket.moved", "--action", "notify:hi")
	if _, err := runCLI(t, "automation", "create", "existing", "--on", "ticket.moved", "--action", "notify:again"); err == nil {
		t.Fatal("expected automation create to fail for duplicate rule")
	}
}

func TestShellSurfaceMetadataPersistsToTicketHistory(t *testing.T) {
	withTempWorkspace(t)

	must := func(args ...string) {
		t.Helper()
		if err := executeArgsWithSurface(args, contracts.EventSurfaceCLI); err != nil {
			t.Fatalf("%v failed: %v", args, err)
		}
	}

	must("init")
	must("project", "create", "APP", "App Project")
	if err := executeArgsWithSurface([]string{"ticket", "create", "--project", "APP", "--title", "Shell ticket", "--type", "task", "--actor", "human:owner"}, contracts.EventSurfaceShell); err != nil {
		t.Fatalf("shell ticket create failed: %v", err)
	}

	root, err := openWorkspace()
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	defer root.close()
	history, err := root.queries.History(context.Background(), "APP-1")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history.Events) == 0 {
		t.Fatal("expected ticket history")
	}
	if history.Events[0].Metadata.Surface != contracts.EventSurfaceShell {
		t.Fatalf("expected shell event surface, got %#v", history.Events[0].Metadata)
	}
	store := mdstore.TicketStore{RootDir: root.root}
	if _, err := store.GetTicket(context.Background(), "APP-1"); err != nil {
		t.Fatalf("ticket persisted: %v", err)
	}
}
