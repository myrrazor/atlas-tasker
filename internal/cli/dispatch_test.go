package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestDispatchSuggestQueueAndBulkAutoRoute(t *testing.T) {
	withTempWorkspace(t)
	gitRunCLI(t, "init", "-b", "main")
	gitRunCLI(t, "config", "user.email", "atlas@example.com")
	gitRunCLI(t, "config", "user.name", "Atlas")
	writeGitFile(t, "README.md", "# atlas\n")
	gitRunCLI(t, "add", "README.md")
	gitRunCLI(t, "commit", "-m", "init")

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
	must("ticket", "create", "--project", "APP", "--title", "Dispatch me", "--type", "task", "--actor", "human:owner")
	must("ticket", "create", "--project", "APP", "--title", "Dispatch me too", "--type", "task", "--actor", "human:owner")
	must("views", "save", "dispatch-backlog", "--kind", "search", "--query", "status=backlog")
	must("agent", "create", "builder-1", "--name", "Builder One", "--provider", "codex", "--default-runbook", "implement", "--actor", "human:owner")
	must("agent", "create", "builder-2", "--name", "Builder Two", "--provider", "claude", "--ticket-type", "bug", "--actor", "human:owner")

	suggestOut := must("dispatch", "suggest", "APP-1", "--json")
	var suggest struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Payload       struct {
			TicketID         string `json:"ticket_id"`
			AutoRouteAgentID string `json:"auto_route_agent_id"`
			Suggestions      []struct {
				Agent struct {
					AgentID string `json:"agent_id"`
				} `json:"agent"`
				Eligible bool   `json:"eligible"`
				Runbook  string `json:"runbook"`
			} `json:"suggestions"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(suggestOut), &suggest); err != nil {
		t.Fatalf("parse dispatch suggestion: %v\nraw=%s", err, suggestOut)
	}
	if suggest.Kind != "dispatch_suggestion" || suggest.Payload.AutoRouteAgentID != "builder-1" {
		t.Fatalf("unexpected suggestion payload: %#v", suggest)
	}
	if len(suggest.Payload.Suggestions) != 2 || !suggest.Payload.Suggestions[0].Eligible || suggest.Payload.Suggestions[0].Runbook != "implement" {
		t.Fatalf("unexpected suggestion entries: %#v", suggest.Payload.Suggestions)
	}

	queueOut := must("dispatch", "queue", "--json")
	var queue struct {
		Kind    string `json:"kind"`
		Payload struct {
			Entries []struct {
				Ticket struct {
					ID string `json:"id"`
				} `json:"ticket"`
				Suggestion struct {
					AutoRouteAgentID string `json:"auto_route_agent_id"`
				} `json:"suggestion"`
			} `json:"entries"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(queueOut), &queue); err != nil {
		t.Fatalf("parse dispatch queue: %v\nraw=%s", err, queueOut)
	}
	if queue.Kind != "dispatch_queue" || len(queue.Payload.Entries) == 0 || queue.Payload.Entries[0].Suggestion.AutoRouteAgentID != "builder-1" {
		t.Fatalf("unexpected dispatch queue: %#v", queue)
	}

	runOut := must("dispatch", "run", "APP-1", "--actor", "human:owner", "--json")
	var run struct {
		Kind    string `json:"kind"`
		Payload struct {
			TicketID string `json:"ticket_id"`
			AgentID  string `json:"agent_id"`
			RunID    string `json:"run_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(runOut), &run); err != nil {
		t.Fatalf("parse dispatch run: %v\nraw=%s", err, runOut)
	}
	if run.Kind != "run_dispatch_result" || run.Payload.AgentID != "builder-1" || run.Payload.RunID == "" {
		t.Fatalf("unexpected dispatch run payload: %#v", run)
	}

	viewOut := must("views", "run", "dispatch-backlog", "--json")
	var savedView struct {
		FormatVersion string `json:"format_version"`
		Tickets       []struct {
			ID string `json:"id"`
		} `json:"tickets"`
	}
	if err := json.Unmarshal([]byte(viewOut), &savedView); err != nil {
		t.Fatalf("parse saved view output: %v\nraw=%s", err, viewOut)
	}

	bulkOut := must("dispatch", "bulk", "--view", "dispatch-backlog", "--dry-run", "--json")
	var bulk struct {
		Kind    string `json:"kind"`
		Payload struct {
			DryRun bool `json:"dry_run"`
			Items  []struct {
				TicketID string `json:"ticket_id"`
				OK       bool   `json:"ok"`
				AgentID  string `json:"agent_id"`
			} `json:"items"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(bulkOut), &bulk); err != nil {
		t.Fatalf("parse dispatch bulk: %v\nraw=%s", err, bulkOut)
	}
	if bulk.Kind != "bulk_dispatch_result" || !bulk.Payload.DryRun || len(bulk.Payload.Items) != 2 {
		t.Fatalf("unexpected bulk dispatch payload: %#v", bulk)
	}
	if len(savedView.Tickets) != len(bulk.Payload.Items) {
		t.Fatalf("expected saved view size to match bulk preview, view=%#v bulk=%#v", savedView.Tickets, bulk.Payload.Items)
	}
	for i, ticket := range savedView.Tickets {
		if bulk.Payload.Items[i].TicketID != ticket.ID {
			t.Fatalf("expected saved-view order preservation, view=%#v bulk=%#v", savedView.Tickets, bulk.Payload.Items)
		}
	}
	for _, item := range bulk.Payload.Items {
		if item.TicketID == "APP-2" && item.AgentID != "builder-1" {
			t.Fatalf("expected APP-2 to auto-route to builder-1, got %#v", bulk.Payload.Items)
		}
		if item.TicketID == "APP-1" && item.OK {
			t.Fatalf("expected APP-1 preview to fail because it already has an active run, got %#v", bulk.Payload.Items)
		}
	}
}

func TestRunDispatchAcceptsQualifiedAgentAndAllowsSelfDispatch(t *testing.T) {
	withTempWorkspace(t)
	gitRunCLI(t, "init", "-b", "main")
	gitRunCLI(t, "config", "user.email", "atlas@example.com")
	gitRunCLI(t, "config", "user.name", "Atlas")
	writeGitFile(t, "README.md", "# atlas\n")
	gitRunCLI(t, "add", "README.md")
	gitRunCLI(t, "commit", "-m", "init")

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
	must("agent", "create", "builder-1", "--name", "Builder One", "--provider", "codex", "--default-runbook", "implement", "--actor", "human:owner")
	must("collaborator", "add", "builder", "--name", "Builder", "--actor-map", "agent:builder-1", "--actor", "human:owner")
	must("collaborator", "add", "other", "--name", "Other", "--actor-map", "agent:other-1", "--actor", "human:owner")
	must("ticket", "create", "--project", "APP", "--title", "Bare dispatch", "--type", "task", "--status", "ready", "--assignee", "agent:builder-1", "--actor", "human:owner")
	must("ticket", "create", "--project", "APP", "--title", "Qualified dispatch", "--type", "task", "--status", "ready", "--assignee", "agent:builder-1", "--actor", "human:owner")
	must("ticket", "create", "--project", "APP", "--title", "Cross dispatch", "--type", "task", "--status", "ready", "--assignee", "agent:builder-1", "--actor", "human:owner")
	must("ticket", "create", "--project", "APP", "--title", "Denied dispatch", "--type", "task", "--status", "ready", "--assignee", "agent:builder-1", "--actor", "human:owner")

	assertDispatch := func(ticketID string, agentArg string) {
		t.Helper()
		out := must("run", "dispatch", ticketID, "--agent", agentArg, "--actor", "agent:builder-1", "--json")
		var payload struct {
			Payload struct {
				AgentID string `json:"agent_id"`
				RunID   string `json:"run_id"`
			} `json:"payload"`
		}
		if err := json.Unmarshal([]byte(out), &payload); err != nil {
			t.Fatalf("parse dispatch output: %v\nraw=%s", err, out)
		}
		if payload.Payload.AgentID != "builder-1" || payload.Payload.RunID == "" {
			t.Fatalf("unexpected dispatch payload for %s: %#v", agentArg, payload.Payload)
		}
	}

	assertDispatch("APP-1", "builder-1")
	assertDispatch("APP-2", "agent:builder-1")

	if out, err := runCLI(t, "run", "dispatch", "APP-3", "--agent", "builder-1", "--actor", "agent:other-1"); err == nil || !strings.Contains(out+err.Error(), "missing_membership") {
		t.Fatalf("expected cross-agent dispatch to retain membership check, err=%v out=%s", err, out)
	}

	must("permission-profile", "create", "deny-dispatch", "--workspace-default", "--deny-action", "dispatch", "--actor", "human:owner")
	if out, err := runCLI(t, "run", "dispatch", "APP-4", "--agent", "builder-1", "--actor", "agent:builder-1"); err == nil || !strings.Contains(out+err.Error(), "permission_action_denied") {
		t.Fatalf("expected permission profile deny to block self-dispatch, err=%v out=%s", err, out)
	}
}

func TestDispatchSuggestReportsDirtyRepoAndActiveRunReasonCodes(t *testing.T) {
	withTempWorkspace(t)
	gitRunCLI(t, "init", "-b", "main")
	gitRunCLI(t, "config", "user.email", "atlas@example.com")
	gitRunCLI(t, "config", "user.name", "Atlas")
	writeGitFile(t, "README.md", "# atlas\n")
	gitRunCLI(t, "add", "README.md")
	gitRunCLI(t, "commit", "-m", "init")

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
	must("ticket", "create", "--project", "APP", "--title", "Dispatch me", "--type", "task", "--actor", "human:owner")
	must("agent", "create", "builder-1", "--name", "Builder One", "--provider", "codex", "--default-runbook", "implement", "--actor", "human:owner")

	if err := os.WriteFile("dirty.txt", []byte("repo drift\n"), 0o644); err != nil {
		t.Fatalf("write dirty repo file: %v", err)
	}

	suggestOut := must("dispatch", "suggest", "APP-1", "--json")
	var dirtySuggest struct {
		Payload struct {
			AutoRouteAgentID string `json:"auto_route_agent_id"`
			Suggestions      []struct {
				Agent struct {
					AgentID string `json:"agent_id"`
				} `json:"agent"`
				Eligible    bool     `json:"eligible"`
				ReasonCodes []string `json:"reason_codes"`
			} `json:"suggestions"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(suggestOut), &dirtySuggest); err != nil {
		t.Fatalf("parse dirty dispatch suggestion: %v\nraw=%s", err, suggestOut)
	}
	if dirtySuggest.Payload.AutoRouteAgentID != "" {
		t.Fatalf("expected dirty repo to block auto-route, got %#v", dirtySuggest.Payload)
	}
	if len(dirtySuggest.Payload.Suggestions) != 1 || dirtySuggest.Payload.Suggestions[0].Eligible {
		t.Fatalf("expected dirty repo to make builder ineligible, got %#v", dirtySuggest.Payload.Suggestions)
	}
	if !containsString(dirtySuggest.Payload.Suggestions[0].ReasonCodes, "dirty_repo") {
		t.Fatalf("expected dirty_repo reason code, got %#v", dirtySuggest.Payload.Suggestions[0].ReasonCodes)
	}

	gitRunCLI(t, "add", "dirty.txt")
	gitRunCLI(t, "commit", "-m", "clean repo")
	dispatchOut := must("dispatch", "run", "APP-1", "--actor", "human:owner", "--json")
	var dispatch struct {
		Payload struct {
			RunID string `json:"run_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(dispatchOut), &dispatch); err != nil {
		t.Fatalf("parse dispatch output: %v\nraw=%s", err, dispatchOut)
	}
	if dispatch.Payload.RunID == "" {
		t.Fatalf("expected run id after dispatch, got %#v", dispatch)
	}

	activeOut := must("dispatch", "suggest", "APP-1", "--json")
	var activeSuggest struct {
		Payload struct {
			Suggestions []struct {
				Agent struct {
					AgentID string `json:"agent_id"`
				} `json:"agent"`
				Eligible    bool     `json:"eligible"`
				ReasonCodes []string `json:"reason_codes"`
			} `json:"suggestions"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(activeOut), &activeSuggest); err != nil {
		t.Fatalf("parse active-run dispatch suggestion: %v\nraw=%s", err, activeOut)
	}
	if len(activeSuggest.Payload.Suggestions) != 1 || activeSuggest.Payload.Suggestions[0].Eligible {
		t.Fatalf("expected active run to make builder ineligible, got %#v", activeSuggest.Payload.Suggestions)
	}
	for _, code := range []string{"active_run_exists", "parallel_runs_disabled"} {
		if !containsString(activeSuggest.Payload.Suggestions[0].ReasonCodes, code) {
			t.Fatalf("expected %s reason code, got %#v", code, activeSuggest.Payload.Suggestions[0].ReasonCodes)
		}
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
