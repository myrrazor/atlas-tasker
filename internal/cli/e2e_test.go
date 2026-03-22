package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcceptanceFlowWithRecovery(t *testing.T) {
	withTempWorkspace(t)

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("command failed %v: %v\noutput=%s", args, err, out)
		}
		return out
	}

	must("init")
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Epic", "--type", "epic", "--actor", "human:owner")
	must("ticket", "create", "--project", "APP", "--title", "Task One", "--type", "task", "--parent", "APP-1", "--actor", "human:owner")
	must("ticket", "create", "--project", "APP", "--title", "Task Two", "--type", "task", "--parent", "APP-1", "--actor", "human:owner")
	must("ticket", "create", "--project", "APP", "--title", "Bug", "--type", "bug", "--actor", "human:owner")
	must("ticket", "link", "APP-4", "--blocked-by", "APP-2", "--actor", "human:owner")
	must("ticket", "assign", "APP-2", "agent:builder-1", "--actor", "human:owner")

	must("ticket", "move", "APP-2", "ready", "--actor", "agent:builder-1")
	must("ticket", "move", "APP-2", "in_progress", "--actor", "agent:builder-1")
	must("ticket", "move", "APP-2", "in_review", "--actor", "agent:builder-1")

	must("config", "set", "workflow.completion_mode", "owner_gate")
	if _, err := runCLI(t, "ticket", "move", "APP-2", "done", "--actor", "agent:builder-1"); err == nil {
		t.Fatal("expected owner_gate to block agent completion")
	}
	must("ticket", "move", "APP-2", "done", "--actor", "human:owner")

	must("ticket", "comment", "APP-4", "--body", "first", "--actor", "agent:builder-1")
	must("ticket", "comment", "APP-4", "--body", "second", "--actor", "agent:builder-1")

	board := must("board", "--project", "APP", "--pretty")
	if !strings.Contains(board, "APP-2") || !strings.Contains(board, "Done") {
		t.Fatalf("board output missing expected ticket/state: %s", board)
	}

	history := must("ticket", "history", "APP-2", "--pretty")
	if !strings.Contains(history, "ticket.moved") {
		t.Fatalf("history missing move events: %s", history)
	}

	if err := os.Remove(filepath.Join(".tracker", "index.sqlite")); err != nil {
		t.Fatalf("failed to remove sqlite: %v", err)
	}
	must("reindex")
	reindexedBoard := must("board", "--project", "APP", "--pretty")
	if !strings.Contains(reindexedBoard, "APP-2") {
		t.Fatalf("reindex board missing APP-2: %s", reindexedBoard)
	}
}

func TestSlashParityForCoreCommands(t *testing.T) {
	withTempWorkspace(t)
	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	commands := []string{
		"/project create APP \"App Project\"",
		"/project list",
		"/ticket create --project APP --title \"Task\" --type task --actor human:owner",
		"/ticket view APP-1",
		"/ticket move APP-1 ready --actor human:owner",
		"/ticket history APP-1",
	}
	for _, slash := range commands {
		args, err := ParseSlashCommand(slash)
		if err != nil {
			t.Fatalf("parse slash command failed: %v", err)
		}
		if err := executeArgs(args); err != nil {
			t.Fatalf("execute parsed slash args failed for %q: %v", slash, err)
		}
	}
}
