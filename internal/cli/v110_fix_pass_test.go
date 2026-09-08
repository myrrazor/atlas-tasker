package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTicketShowAliasAndUnknownSubcommand(t *testing.T) {
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
	must("project", "create", "APP", "App")
	must("ticket", "create", "--project", "APP", "--title", "Show me", "--type", "task", "--actor", "human:owner")
	if out := must("ticket", "show", "APP-1", "--json"); !strings.Contains(out, `"id": "APP-1"`) {
		t.Fatalf("show alias failed: %s", out)
	}
	if out, err := runCLI(t, "ticket", "nosuch"); err == nil || !strings.Contains(out+err.Error(), "unknown command") {
		t.Fatalf("expected unknown subcommand error, err=%v out=%s", err, out)
	}
}

func TestSameColumnMoveIsNoOpAndBulkConflictExits(t *testing.T) {
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
	must("config", "set", "actor.default", "human:owner")
	must("project", "create", "APP", "App")
	must("ticket", "create", "--project", "APP", "--title", "Ready", "--type", "task", "--status", "ready", "--actor", "human:owner")
	if out := must("ticket", "move", "APP-1", "ready", "--actor", "human:owner"); !strings.Contains(out, "moved APP-1 to ready") {
		t.Fatalf("same-column move should no-op successfully: %s", out)
	}
	var stdout, stderr bytes.Buffer
	exit := Execute([]string{"bulk", "move", "in_progress", "--ticket", "APP-1", "--yes", "--json"}, &stdout, &stderr)
	if exit != 0 {
		// APP-1 is ready -> in_progress is valid; create a conflict case instead
	}
	must("ticket", "move", "APP-1", "in_progress", "--actor", "human:owner")
	stdout.Reset()
	stderr.Reset()
	exit = Execute([]string{"bulk", "move", "ready", "--ticket", "APP-1", "--ticket", "APP-99", "--yes", "--json"}, &stdout, &stderr)
	if exit == 0 {
		t.Fatalf("bulk with failures should exit non-zero; stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"failed"`) {
		t.Fatalf("stdout should still carry the bulk envelope: %s", stdout.String())
	}
}

func TestNestedInitRefusedAndReInitMessage(t *testing.T) {
	withTempWorkspace(t)
	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
		return out
	}
	if out := must("init"); !strings.Contains(out, "initialized") {
		t.Fatalf("first init message: %s", out)
	}
	if out := must("init"); !strings.Contains(out, "already bootstrapped") {
		t.Fatalf("re-init message: %s", out)
	}
	must("project", "create", "APP", "App")
	nested := filepath.Join("projects", "APP")
	if err := os.Chdir(nested); err != nil {
		t.Fatalf("chdir nested: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir("../..") })
	if out, err := runCLI(t, "init"); err == nil || !strings.Contains(out+err.Error(), "inside existing Atlas workspace") {
		t.Fatalf("nested init should refuse, err=%v out=%s", err, out)
	}
}

func TestTicketCreateRequiresTypeAndActorResolution(t *testing.T) {
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
	must("project", "create", "APP", "App")
	if out, err := runCLI(t, "ticket", "create", "--project", "APP", "--title", "No type", "--actor", "human:owner"); err == nil || !strings.Contains(out+err.Error(), "--type") {
		t.Fatalf("expected missing type error, err=%v out=%s", err, out)
	}
	if out, err := runCLI(t, "ticket", "create", "--project", "APP", "--title", "No actor", "--type", "task"); err == nil || !strings.Contains(out+err.Error(), "actor is required") {
		t.Fatalf("expected missing actor error, err=%v out=%s", err, out)
	}
	if out, err := runCLI(t, "ticket", "move", "APP-1", "ready"); err == nil || !strings.Contains(out+err.Error(), "actor is required") {
		// ticket does not exist yet / actor required first
		if err == nil || !strings.Contains(out+err.Error(), "actor is required") {
			t.Fatalf("expected move without actor to require actor, err=%v out=%s", err, out)
		}
	}
}

func TestOpenModeSoloCompleteFromInProgress(t *testing.T) {
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
	must("project", "create", "APP", "App")
	must("ticket", "create", "--project", "APP", "--title", "Solo close", "--type", "task", "--status", "in_progress", "--assignee", "agent:builder-1", "--actor", "human:owner")
	out := must("ticket", "complete", "APP-1", "--actor", "agent:builder-1", "--reason", "done", "--json")
	var ticket struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(out), &ticket); err != nil {
		t.Fatalf("parse complete: %v\n%s", err, out)
	}
	if ticket.Status != "done" {
		t.Fatalf("expected done, got %s", out)
	}
}
