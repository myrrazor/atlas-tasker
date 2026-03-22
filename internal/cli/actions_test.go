package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := NewRootCommand()
	var out bytes.Buffer
	var errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	err := root.Execute()
	if errOut.Len() > 0 {
		out.WriteString(errOut.String())
	}
	return out.String(), err
}

func withTempWorkspace(t *testing.T) {
	t.Helper()
	temp := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(temp); err != nil {
		t.Fatalf("chdir temp failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})
}

func TestTicketLifecycleAndHistory(t *testing.T) {
	withTempWorkspace(t)

	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if _, err := runCLI(t, "project", "create", "APP", "App Project"); err != nil {
		t.Fatalf("project create failed: %v", err)
	}
	if _, err := runCLI(t, "ticket", "create", "--project", "APP", "--title", "First", "--type", "task", "--actor", "human:owner"); err != nil {
		t.Fatalf("ticket create failed: %v", err)
	}
	if _, err := runCLI(t, "ticket", "move", "APP-1", "ready", "--actor", "agent:builder-1"); err != nil {
		t.Fatalf("ticket move ready failed: %v", err)
	}
	if _, err := runCLI(t, "ticket", "comment", "APP-1", "--body", "looks good", "--actor", "agent:builder-1"); err != nil {
		t.Fatalf("ticket comment failed: %v", err)
	}

	historyJSON, err := runCLI(t, "ticket", "history", "APP-1", "--json")
	if err != nil {
		t.Fatalf("ticket history failed: %v", err)
	}
	var payload []map[string]any
	if err := json.Unmarshal([]byte(historyJSON), &payload); err != nil {
		t.Fatalf("history json parse failed: %v\nraw=%s", err, historyJSON)
	}
	if len(payload) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(payload))
	}
}

func TestOwnerGateBlocksAgentCompletion(t *testing.T) {
	withTempWorkspace(t)

	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if _, err := runCLI(t, "project", "create", "APP", "App Project"); err != nil {
		t.Fatalf("project create failed: %v", err)
	}
	if _, err := runCLI(t, "config", "set", "workflow.completion_mode", "owner_gate"); err != nil {
		t.Fatalf("config set owner_gate failed: %v", err)
	}
	if _, err := runCLI(t, "ticket", "create", "--project", "APP", "--title", "Flow", "--type", "task", "--actor", "human:owner"); err != nil {
		t.Fatalf("ticket create failed: %v", err)
	}
	if _, err := runCLI(t, "ticket", "move", "APP-1", "ready", "--actor", "agent:builder-1"); err != nil {
		t.Fatalf("move ready failed: %v", err)
	}
	if _, err := runCLI(t, "ticket", "move", "APP-1", "in_progress", "--actor", "agent:builder-1"); err != nil {
		t.Fatalf("move in_progress failed: %v", err)
	}
	if _, err := runCLI(t, "ticket", "move", "APP-1", "in_review", "--actor", "agent:builder-1"); err != nil {
		t.Fatalf("move in_review failed: %v", err)
	}
	if _, err := runCLI(t, "ticket", "move", "APP-1", "done", "--actor", "agent:builder-1"); err == nil {
		t.Fatal("expected owner_gate to reject agent completion")
	}
}

func TestTicketCreateRequiresExistingProject(t *testing.T) {
	withTempWorkspace(t)
	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if _, err := runCLI(t, "ticket", "create", "--project", "NOPE", "--title", "First", "--type", "task", "--actor", "human:owner"); err == nil {
		t.Fatal("expected ticket create to fail for missing project")
	}
}

func TestTicketMutationInputValidation(t *testing.T) {
	withTempWorkspace(t)
	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if _, err := runCLI(t, "project", "create", "APP", "App Project"); err != nil {
		t.Fatalf("project create failed: %v", err)
	}
	if _, err := runCLI(t, "ticket", "create", "--project", "APP", "--title", "Validate", "--type", "task", "--actor", "human:owner"); err != nil {
		t.Fatalf("ticket create failed: %v", err)
	}
	if _, err := runCLI(t, "ticket", "priority", "APP-1", "not-a-priority", "--actor", "human:owner"); err == nil {
		t.Fatal("expected invalid priority to fail")
	}
	if _, err := runCLI(t, "ticket", "assign", "APP-1", "not-an-actor", "--actor", "human:owner"); err == nil {
		t.Fatal("expected invalid assignee actor to fail")
	}
}
