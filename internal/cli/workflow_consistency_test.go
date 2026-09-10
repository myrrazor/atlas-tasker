package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
)

func TestMutationActorResolutionIsConsistent(t *testing.T) {
	withTempWorkspace(t)
	t.Setenv("TRACKER_ACTOR", "")
	for _, args := range [][]string{{"init"}, {"project", "create", "APP", "App"}, {"ticket", "create", "--project", "APP", "--title", "Work", "--type", "task", "--actor", "human:owner"}} {
		if out, err := runCLI(t, args...); err != nil {
			t.Fatalf("%v: %v %s", args, err, out)
		}
	}
	at := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	for _, args := range [][]string{
		{"bulk", "move", "ready", "--ticket", "APP-1", "--yes"},
		{"ticket", "assign", "APP-1", "agent:builder-1"},
		{"team", "apply", "pair"},
		{"schedule", "set", "APP-1", "--at", at, "--runner", "human:alex", "--reason", "plan"},
	} {
		if out, err := runCLI(t, args...); apperr.CodeOf(err) != apperr.CodeInvalidInput || !strings.Contains(err.Error(), "actor is required") {
			t.Fatalf("%v defaulted actor: %v %s", args, err, out)
		}
	}
	root, _ := os.Getwd()
	ticket, err := (mdstore.TicketStore{RootDir: root}).GetTicket(context.Background(), "APP-1")
	if err != nil || ticket.Status != contracts.StatusBacklog || ticket.Assignee != "" || ticket.Schedule != nil {
		t.Fatalf("missing actor mutated ticket: %#v %v", ticket, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".tracker", "agents", "builder-1.toml")); !os.IsNotExist(err) {
		t.Fatalf("missing actor applied team: %v", err)
	}
	if out, err := runCLI(t, "config", "set", "actor.default", "human:configured"); err != nil {
		t.Fatalf("configure actor: %v %s", err, out)
	}
	for _, tc := range []struct{ env, explicit, want string }{
		{"", "", "human:configured"}, {"human:environment", "", "human:environment"}, {"human:environment", "human:explicit", "human:explicit"},
	} {
		t.Setenv("TRACKER_ACTOR", tc.env)
		args := []string{"bulk", "move", "ready", "--ticket", "APP-1", "--dry-run", "--json"}
		if tc.explicit != "" {
			args = append(args, "--actor", tc.explicit)
		}
		if out, err := runCLI(t, args...); err != nil || !strings.Contains(out, `"actor": "`+tc.want+`"`) {
			t.Fatalf("actor precedence %v: %v %s", tc, err, out)
		}
	}
}

func TestClaimWrongAssigneeReturnsExitFour(t *testing.T) {
	withTempWorkspace(t)
	for _, args := range [][]string{{"init"}, {"project", "create", "APP", "App"}, {"ticket", "create", "--project", "APP", "--title", "Assigned", "--type", "task", "--status", "ready", "--assignee", "agent:builder-1", "--actor", "human:owner"}} {
		if out, err := runCLI(t, args...); err != nil {
			t.Fatalf("setup: %v %s", err, out)
		}
	}
	for _, args := range [][]string{
		{"ticket", "claim", "APP-1", "--actor", "agent:builder-2"},
		{"bulk", "claim", "--ticket", "APP-1", "--actor", "agent:builder-2", "--yes"},
	} {
		var stdout, stderr bytes.Buffer
		if exit := Execute(args, &stdout, &stderr); exit != 4 {
			t.Fatalf("%v: exit=%d out=%s err=%s", args, exit, &stdout, &stderr)
		}
	}
}

func TestUnmanagedProjectFolderDoesNotBreakDoctor(t *testing.T) {
	withTempWorkspace(t)
	if out, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init: %v %s", err, out)
	}
	root, _ := os.Getwd()
	nested := filepath.Join(root, "projects", "foo")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}
	_, initErr := runCLI(t, "init")
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	if initErr == nil {
		t.Fatal("nested init succeeded")
	}
	if out, err := runCLI(t, "doctor"); err != nil {
		t.Fatalf("unmanaged directory broke doctor: %v %s", err, out)
	}
	marker := filepath.Join(nested, "project.md")
	if err := os.WriteFile(marker, []byte("invalid managed project"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := runCLI(t, "project", "list"); err == nil || !strings.Contains(err.Error(), marker) {
		t.Fatalf("invalid managed project must remain visible: %v %s", err, out)
	}
}

func TestTicketParentHelpNamesShowAlias(t *testing.T) {
	if out, err := runCLI(t, "ticket", "--help"); err != nil || !strings.Contains(out, "alias: show") {
		t.Fatalf("show alias hidden: %v %s", err, out)
	}
}
