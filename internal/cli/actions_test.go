package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
)

type jsonListEnvelope[T any] struct {
	FormatVersion string `json:"format_version"`
	Items         []T    `json:"items"`
}

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

func decodeJSONList[T any](t *testing.T, raw string) []T {
	t.Helper()
	var payload jsonListEnvelope[T]
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("parse versioned json list: %v\nraw=%s", err, raw)
	}
	if payload.FormatVersion != jsonFormatVersion {
		t.Fatalf("unexpected format version %q in %s", payload.FormatVersion, raw)
	}
	return payload.Items
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

func TestCommandOutputSanitizesTerminalControls(t *testing.T) {
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
	must("ticket", "create", "--project", "APP", "--title", "wipe\x1b[2J", "--type", "task", "--actor", "human:owner")
	out := must("ticket", "view", "APP-1")
	if strings.Contains(out, "\x1b") {
		t.Fatalf("expected CLI text output to sanitize terminal controls, got %q", out)
	}
	if !strings.Contains(out, "wipe[2J") {
		t.Fatalf("expected sanitized title to remain readable, got %q", out)
	}
}

func TestAgentAvailablePendingCommands(t *testing.T) {
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
	must("agent", "create", "builder-1", "--name", "Builder", "--provider", "codex", "--actor", "human:owner", "--reason", "test")
	must("ticket", "create", "--project", "APP", "--title", "Blocker", "--type", "task", "--status", "in_progress", "--actor", "human:owner")
	must("ticket", "create", "--project", "APP", "--title", "Ready work", "--type", "task", "--status", "ready", "--assignee", "agent:builder-1", "--actor", "human:owner")
	must("ticket", "create", "--project", "APP", "--title", "Blocked work", "--type", "task", "--status", "ready", "--assignee", "agent:builder-1", "--actor", "human:owner")
	must("ticket", "link", "APP-3", "--blocked-by", "APP-1", "--actor", "human:owner", "--reason", "test dependency")

	available := must("agent", "available", "builder-1")
	if !strings.Contains(available, "APP-2") || strings.Contains(available, "APP-3") {
		t.Fatalf("available should show only unblocked work, got:\n%s", available)
	}
	pending := must("agent", "pending", "builder-1", "--json")
	if !strings.Contains(pending, `"dependency_blocked"`) || !strings.Contains(pending, `"APP-3"`) {
		t.Fatalf("pending json should include dependency_blocked APP-3, got:\n%s", pending)
	}
}

func TestCLIRejectsPathDerivedIdentifierTraversal(t *testing.T) {
	parent := t.TempDir()
	workspaceRoot := filepath.Join(parent, "workspace")
	if err := os.Mkdir(workspaceRoot, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(workspaceRoot); err != nil {
		t.Fatalf("chdir workspace: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if out, err := runCLI(t, "project", "create", "../../etc", "Bad"); err == nil || !strings.Contains(err.Error()+out, "project key must match") {
		t.Fatalf("expected invalid project key error, got err=%v out=%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(parent, "etc")); !os.IsNotExist(err) {
		t.Fatalf("path traversal created outside project dir, stat err=%v", err)
	}
	if out, err := runCLI(t, "collaborator", "add", "../EVIL", "--name", "evil", "--actor", "human:owner", "--reason", "trav"); err == nil || !strings.Contains(err.Error()+out, "collaborator_id must match") {
		t.Fatalf("expected invalid collaborator id error, got err=%v out=%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, ".tracker", "EVIL.md")); !os.IsNotExist(err) {
		t.Fatalf("collaborator traversal escaped collaborators dir, stat err=%v", err)
	}
}

func TestTicketTitleInputNormalizesLayoutControls(t *testing.T) {
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
	must("ticket", "create", "--project", "APP", "--title", "Title\nFAKE\tLINE\u202e.exe.txt", "--type", "task", "--actor", "human:owner")
	board := must("board")
	if strings.ContainsAny(board, "\t") || strings.Contains(board, "\u202e") || strings.Contains(board, "Title\nFAKE") {
		t.Fatalf("board output should normalize user-controlled layout controls:\n%q", board)
	}
	if !strings.Contains(board, "Title FAKE LINE.exe.txt") {
		t.Fatalf("expected readable normalized title, got:\n%s", board)
	}
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
	var payload struct {
		FormatVersion string           `json:"format_version"`
		TicketID      string           `json:"ticket_id"`
		Events        []map[string]any `json:"events"`
	}
	if err := json.Unmarshal([]byte(historyJSON), &payload); err != nil {
		t.Fatalf("history json parse failed: %v\nraw=%s", err, historyJSON)
	}
	if payload.FormatVersion != jsonFormatVersion {
		t.Fatalf("unexpected format version: %s", payload.FormatVersion)
	}
	if payload.TicketID != "APP-1" {
		t.Fatalf("unexpected history ticket id: %s", payload.TicketID)
	}
	if len(payload.Events) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(payload.Events))
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

func TestBoardMarkdownOrderIsDeterministic(t *testing.T) {
	withTempWorkspace(t)
	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	out, err := runCLI(t, "board", "--md")
	if err != nil {
		t.Fatalf("board --md failed: %v", err)
	}
	expectedOrder := []string{
		"### backlog",
		"### ready",
		"### in_progress",
		"### in_review",
		"### blocked",
		"### done",
	}
	last := -1
	for _, marker := range expectedOrder {
		index := strings.Index(out, marker)
		if index == -1 {
			t.Fatalf("missing marker %q in board markdown: %s", marker, out)
		}
		if index <= last {
			t.Fatalf("marker %q out of order in board markdown: %s", marker, out)
		}
		last = index
	}
}

func TestClaimQueueAndSweepCommands(t *testing.T) {
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
	must("ticket", "create", "--project", "APP", "--title", "Queue me", "--type", "task", "--priority", "high", "--actor", "human:owner")
	must("ticket", "move", "APP-1", "ready", "--actor", "human:owner")
	claimOut := must("ticket", "claim", "APP-1")
	if !strings.Contains(claimOut, "claimed APP-1") {
		t.Fatalf("unexpected claim output: %s", claimOut)
	}
	queueOut := must("queue", "--json")
	if !strings.Contains(queueOut, "claimed_by_me") {
		t.Fatalf("queue output missing claimed_by_me: %s", queueOut)
	}
	heartbeatOut := must("ticket", "heartbeat", "APP-1")
	if !strings.Contains(heartbeatOut, "heartbeat APP-1") {
		t.Fatalf("unexpected heartbeat output: %s", heartbeatOut)
	}
	whoOut := must("who", "--pretty")
	if !strings.Contains(whoOut, "APP-1") || !strings.Contains(whoOut, "agent:builder-1") || !strings.Contains(whoOut, "[active]") {
		t.Fatalf("unexpected who output: %s", whoOut)
	}
	must("ticket", "release", "APP-1")
	queueOut = must("queue", "--pretty")
	if strings.Contains(queueOut, "claimed_by_me:\n  - APP-1") {
		t.Fatalf("ticket should not remain claimed after release: %s", queueOut)
	}

	must("ticket", "create", "--project", "APP", "--title", "Review me", "--type", "task", "--reviewer", "agent:builder-1", "--actor", "human:owner")
	must("ticket", "move", "APP-2", "ready", "--actor", "human:owner")
	must("ticket", "move", "APP-2", "in_progress", "--actor", "human:owner")
	must("ticket", "move", "APP-2", "in_review", "--actor", "human:owner")
	reviewQueueOut := must("review-queue", "--pretty")
	if !strings.Contains(reviewQueueOut, "APP-2") {
		t.Fatalf("review-queue should include APP-2: %s", reviewQueueOut)
	}

	ownerQueueOut := must("owner-queue", "--pretty")
	if !strings.Contains(ownerQueueOut, "awaiting_owner") {
		t.Fatalf("owner-queue output missing awaiting_owner section: %s", ownerQueueOut)
	}

	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	store := mdstore.TicketStore{RootDir: root}
	ticket, err := store.GetTicket(context.Background(), "APP-1")
	if err != nil {
		t.Fatalf("load ticket for stale lease setup: %v", err)
	}
	ticket.Lease = contracts.LeaseState{
		Actor:           contracts.Actor("agent:builder-1"),
		Kind:            contracts.LeaseKindWork,
		AcquiredAt:      time.Now().UTC().Add(-2 * time.Hour),
		ExpiresAt:       time.Now().UTC().Add(-1 * time.Hour),
		LastHeartbeatAt: time.Now().UTC().Add(-90 * time.Minute),
	}
	ticket.UpdatedAt = time.Now().UTC().Add(-90 * time.Minute)
	if err := store.UpdateTicket(context.Background(), ticket); err != nil {
		t.Fatalf("update stale ticket: %v", err)
	}
	sweepOut := must("sweep", "--actor", "human:owner", "--reason", "cleanup")
	if !strings.Contains(sweepOut, "expired 1 lease(s)") {
		t.Fatalf("unexpected sweep output: %s", sweepOut)
	}
	whoOut = must("who", "--pretty")
	if strings.Contains(whoOut, "APP-1") {
		t.Fatalf("who output should not include APP-1 after sweep: %s", whoOut)
	}
}

func TestIntegrationsInstallCommands(t *testing.T) {
	withTempWorkspace(t)

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
		return out
	}

	must("integrations", "install", "codex")
	if _, err := os.Stat("AGENTS.md"); err != nil {
		t.Fatalf("expected AGENTS.md to exist: %v", err)
	}
	body, err := os.ReadFile("AGENTS.md")
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if !strings.Contains(string(body), "Atlas Tasker (Codex)") {
		t.Fatalf("unexpected AGENTS.md: %s", string(body))
	}

	must("integrations", "install", "claude")
	if _, err := os.Stat("CLAUDE.md"); err != nil {
		t.Fatalf("expected CLAUDE.md to exist: %v", err)
	}
	guide, err := os.ReadFile(".tracker/integrations/claude-guide.md")
	if err != nil {
		t.Fatalf("read claude guide: %v", err)
	}
	if !strings.Contains(string(guide), "tracker review-queue --actor agent:reviewer-1 --json") {
		t.Fatalf("unexpected claude guide: %s", string(guide))
	}
}

func TestReviewCommandsAndPolicyCommands(t *testing.T) {
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
	must("project", "policy", "set", "APP", "--completion-mode", "dual_gate", "--lease-ttl", "45", "--allowed-workers", "agent:builder-1", "--required-reviewer", "agent:reviewer-1", "--actor", "human:owner")
	projectPolicy := must("project", "policy", "get", "APP", "--json")
	if !strings.Contains(projectPolicy, "\"completion_mode\": \"dual_gate\"") {
		t.Fatalf("unexpected project policy output: %s", projectPolicy)
	}

	must("ticket", "create", "--project", "APP", "--title", "Review flow", "--type", "task", "--reviewer", "agent:reviewer-1", "--actor", "human:owner")
	must("ticket", "policy", "set", "APP-1", "--completion-mode", "dual_gate", "--allowed-workers", "agent:builder-1", "--actor", "human:owner")
	ticketPolicy := must("ticket", "policy", "get", "APP-1", "--json")
	if !strings.Contains(ticketPolicy, "\"effective_policy\"") {
		t.Fatalf("unexpected ticket policy output: %s", ticketPolicy)
	}

	must("ticket", "move", "APP-1", "ready", "--actor", "human:owner")
	must("ticket", "move", "APP-1", "in_progress", "--actor", "human:owner")
	must("ticket", "request-review", "APP-1", "--actor", "agent:builder-1")
	if _, err := runCLI(t, "ticket", "complete", "APP-1", "--actor", "human:owner"); err == nil {
		t.Fatal("expected complete to require approval first")
	}
	rejectOut := must("ticket", "reject", "APP-1", "--actor", "agent:reviewer-1", "--reason", "fix this")
	if !strings.Contains(rejectOut, "rejected APP-1") {
		t.Fatalf("unexpected reject output: %s", rejectOut)
	}
	must("ticket", "request-review", "APP-1", "--actor", "agent:builder-1")
	approveOut := must("ticket", "approve", "APP-1", "--actor", "agent:reviewer-1")
	if !strings.Contains(approveOut, "approved APP-1") {
		t.Fatalf("unexpected approve output: %s", approveOut)
	}
	ownerQueue := must("owner-queue", "--pretty")
	if !strings.Contains(ownerQueue, "APP-1") {
		t.Fatalf("owner queue should include approved dual_gate ticket: %s", ownerQueue)
	}
	if _, err := runCLI(t, "ticket", "complete", "APP-1", "--actor", "agent:reviewer-1"); err == nil {
		t.Fatal("expected dual_gate to reject reviewer completion")
	}
	completeOut := must("ticket", "complete", "APP-1", "--actor", "human:owner")
	if !strings.Contains(completeOut, "completed APP-1") {
		t.Fatalf("unexpected complete output: %s", completeOut)
	}
}

func TestDoctorRepairAndInspectCommands(t *testing.T) {
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
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Inspect me", "--type", "task", "--reviewer", "agent:reviewer-1", "--actor", "human:owner")
	must("ticket", "move", "APP-1", "ready", "--actor", "human:owner")
	must("ticket", "move", "APP-1", "in_progress", "--actor", "human:owner")
	must("ticket", "request-review", "APP-1", "--actor", "agent:builder-1", "--reason", "ready")

	inspectOut := must("inspect", "APP-1", "--actor", "agent:reviewer-1", "--json")
	if !strings.Contains(inspectOut, "\"queue_categories\"") || !strings.Contains(inspectOut, "needs_review") {
		t.Fatalf("unexpected inspect output: %s", inspectOut)
	}

	doctorOut := must("doctor", "--repair", "--json")
	if !strings.Contains(doctorOut, "\"repair_ran\": true") {
		t.Fatalf("unexpected doctor output: %s", doctorOut)
	}

	raw, err := os.ReadFile(".tracker/ops-notify.log")
	if err != nil {
		t.Fatalf("read notify log failed: %v", err)
	}
	if !strings.Contains(string(raw), "ticket.review_requested") {
		t.Fatalf("unexpected notify log contents: %s", string(raw))
	}
}

func TestTemplatesAndQueueAwareNext(t *testing.T) {
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
	templates := must("templates", "list", "--pretty")
	if !strings.Contains(templates, "design") || !strings.Contains(templates, "task") {
		t.Fatalf("unexpected templates list: %s", templates)
	}
	view := must("templates", "view", "design", "--json")
	if !strings.Contains(view, "\"blueprint\": \"design\"") {
		t.Fatalf("unexpected template view: %s", view)
	}

	must("ticket", "create", "--project", "APP", "--title", "Template task", "--template", "design", "--actor", "human:owner")
	must("ticket", "move", "APP-1", "ready", "--actor", "human:owner")
	next := must("next", "--json")
	if !strings.Contains(next, "\"category\": \"ready_for_me\"") || !strings.Contains(next, "\"reason\": \"ready and assignable\"") {
		t.Fatalf("unexpected next output: %s", next)
	}
	detail := must("ticket", "view", "APP-1", "--json")
	if !strings.Contains(detail, "\"blueprint\": \"design\"") || !strings.Contains(detail, "\"skill_hint\": \"design\"") {
		t.Fatalf("unexpected ticket detail output: %s", detail)
	}
}
