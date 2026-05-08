package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectRequiredReviewerAndDependenciesAuthFlow(t *testing.T) {
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
	must("project", "create", "AUTH", "Auth")
	must("project", "policy", "set", "AUTH", "--completion-mode", "review_gate", "--required-reviewer", "agent:reviewer-1", "--actor", "human:owner", "--reason", "default reviewer")
	must("ticket", "create", "--project", "AUTH", "--title", "Logout timeout", "--type", "task", "--assignee", "agent:builder-1", "--actor", "human:owner")

	viewOut := must("ticket", "view", "AUTH-1", "--json")
	var view struct {
		BoardStatus       string `json:"board_status"`
		EffectiveReviewer string `json:"effective_reviewer"`
		Ticket            struct {
			Reviewer string `json:"reviewer"`
		} `json:"ticket"`
		EffectivePolicy struct {
			RequiredReviewer string `json:"required_reviewer"`
		} `json:"effective_policy"`
	}
	if err := json.Unmarshal([]byte(viewOut), &view); err != nil {
		t.Fatalf("parse ticket view: %v\n%s", err, viewOut)
	}
	if view.Ticket.Reviewer != "agent:reviewer-1" || view.EffectivePolicy.RequiredReviewer != "agent:reviewer-1" || view.EffectiveReviewer != "agent:reviewer-1" {
		t.Fatalf("project required reviewer was not visible/persisted: %#v", view)
	}

	must("ticket", "move", "AUTH-1", "ready", "--actor", "agent:builder-1")
	must("ticket", "move", "AUTH-1", "in_progress", "--actor", "agent:builder-1")
	must("ticket", "request-review", "AUTH-1", "--actor", "agent:builder-1", "--reason", "ready")
	if out, err := runCLI(t, "ticket", "approve", "AUTH-1", "--actor", "agent:builder-1", "--reason", "self approve"); err == nil || !strings.Contains(out+err.Error(), "only the assigned reviewer") {
		t.Fatalf("expected builder approval to fail, err=%v out=%s", err, out)
	}
	approved := must("ticket", "approve", "AUTH-1", "--actor", "agent:reviewer-1", "--reason", "reviewed", "--json")
	if !strings.Contains(approved, `"status": "done"`) {
		t.Fatalf("review_gate approval should complete ticket: %s", approved)
	}

	must("ticket", "create", "--project", "AUTH", "--title", "Blocker", "--type", "task", "--status", "in_progress", "--actor", "human:owner")
	must("ticket", "create", "--project", "AUTH", "--title", "Dependent", "--type", "task", "--status", "ready", "--actor", "human:owner")
	must("ticket", "link", "AUTH-3", "--blocked-by", "AUTH-2", "--actor", "human:owner", "--reason", "depends on blocker")
	if out, err := runCLI(t, "ticket", "move", "AUTH-3", "in_progress", "--actor", "agent:builder-1", "--json"); err == nil || !strings.Contains(out+err.Error(), "dependency_blocked") {
		t.Fatalf("expected unresolved dependency to block progress, err=%v out=%s", err, out)
	}
	boardBlocked := must("board", "--project", "AUTH", "--pretty")
	if !strings.Contains(boardBlocked, "Blocked (1)") || !strings.Contains(boardBlocked, "AUTH-3 [task] [blocked]") {
		t.Fatalf("expected unresolved dependency to derive blocked bucket:\n%s", boardBlocked)
	}

	must("ticket", "request-review", "AUTH-2", "--actor", "agent:builder-1", "--reason", "blocker ready")
	must("ticket", "approve", "AUTH-2", "--actor", "agent:reviewer-1", "--reason", "blocker reviewed")
	must("ticket", "move", "AUTH-3", "in_progress", "--actor", "agent:builder-1")
	boardUnblocked := must("board", "--project", "AUTH", "--pretty")
	if !strings.Contains(boardUnblocked, "Blocked (0)") || !strings.Contains(boardUnblocked, "AUTH-3 [task] [in_progress]") {
		t.Fatalf("expected board to clear derived blocked bucket after blocker done:\n%s", boardUnblocked)
	}
	if err := os.Remove(filepath.Join(".tracker", "index.sqlite")); err != nil {
		t.Fatalf("remove projection: %v", err)
	}
	must("reindex")
	reindexed := must("board", "--project", "AUTH", "--pretty")
	if !strings.Contains(reindexed, "Blocked (0)") || !strings.Contains(reindexed, "AUTH-3 [task] [in_progress]") {
		t.Fatalf("expected reindex to preserve unblocked board state:\n%s", reindexed)
	}
}

func TestTicketApproveRejectsSelfApproval(t *testing.T) {
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
	must("ticket", "create", "--project", "APP", "--title", "Do not self approve", "--type", "task", "--status", "in_progress", "--assignee", "agent:builder-1", "--reviewer", "agent:builder-1", "--actor", "human:owner")
	must("ticket", "request-review", "APP-1", "--actor", "agent:builder-1", "--reason", "ready")
	if out, err := runCLI(t, "ticket", "approve", "APP-1", "--actor", "agent:builder-1", "--reason", "approve myself"); err == nil || !strings.Contains(out+err.Error(), "self_approval_denied") {
		t.Fatalf("expected self approval denial, err=%v out=%s", err, out)
	}
	must("ticket", "create", "--project", "APP", "--title", "Separate reviewer", "--type", "task", "--status", "in_progress", "--assignee", "agent:builder-1", "--actor", "human:owner")
	must("ticket", "request-review", "APP-2", "--reviewer", "agent:reviewer-1", "--actor", "agent:builder-1", "--reason", "ready")
	if out := must("ticket", "approve", "APP-2", "--actor", "agent:reviewer-1", "--reason", "reviewed"); !strings.Contains(out, "approved APP-2") {
		t.Fatalf("expected separate reviewer approval, got %s", out)
	}
}

func TestTicketApproveCanBeProtectedByGovernanceSoD(t *testing.T) {
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
	created := must("governance", "pack", "create", "approval-sod", "--protected-action", "ticket_approve", "--separation-event", "ticket.moved", "--actor", "human:owner", "--reason", "protect approval", "--json")
	if !strings.Contains(created, `"ticket_approve"`) {
		t.Fatalf("governance pack did not include ticket_approve: %s", created)
	}
	must("governance", "pack", "apply", "approval-sod", "--scope", "project:APP", "--actor", "human:owner", "--reason", "apply approval sod")
	must("ticket", "create", "--project", "APP", "--title", "Needs SoD", "--type", "task", "--status", "ready", "--reviewer", "agent:builder-1", "--actor", "human:owner")
	must("ticket", "move", "APP-1", "in_progress", "--actor", "agent:builder-1")
	must("ticket", "request-review", "APP-1", "--actor", "agent:builder-1", "--reason", "ready")
	if out, err := runCLI(t, "ticket", "approve", "APP-1", "--actor", "agent:builder-1", "--reason", "same actor"); err == nil || !strings.Contains(out+err.Error(), "separation_of_duties_violation") {
		t.Fatalf("expected governance SoD denial, err=%v out=%s", err, out)
	}
}

func TestRequestReviewReviewerFlagAndShellParity(t *testing.T) {
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
	must("ticket", "create", "--project", "APP", "--title", "Review via flag", "--type", "task", "--status", "in_progress", "--actor", "human:owner")
	must("ticket", "request-review", "APP-1", "--reviewer", "agent:reviewer-1", "--actor", "agent:builder-1", "--reason", "ready")
	viewOut := must("ticket", "view", "APP-1", "--json")
	if !strings.Contains(viewOut, `"reviewer": "agent:reviewer-1"`) || !strings.Contains(viewOut, `"status": "in_review"`) {
		t.Fatalf("request-review --reviewer should set reviewer and move to review: %s", viewOut)
	}

	must("ticket", "create", "--project", "APP", "--title", "Shell review flag", "--type", "task", "--status", "in_progress", "--actor", "human:owner")
	runSlashShell(t, `/ticket request-review APP-2 --reviewer agent:reviewer-2 --actor agent:builder-1 --reason "shell ready"`)
	shellView := must("ticket", "view", "APP-2", "--json")
	if !strings.Contains(shellView, `"reviewer": "agent:reviewer-2"`) || !strings.Contains(shellView, `"status": "in_review"`) {
		t.Fatalf("slash shell request-review --reviewer drifted: %s", shellView)
	}

	must("project", "create", "POL", "Policy")
	must("project", "policy", "set", "POL", "--required-reviewer", "agent:reviewer-1", "--actor", "human:owner", "--reason", "required reviewer")
	must("ticket", "create", "--project", "POL", "--title", "Reject wrong reviewer", "--type", "task", "--status", "in_progress", "--actor", "human:owner")
	if out, err := runCLI(t, "ticket", "request-review", "POL-1", "--reviewer", "agent:reviewer-2", "--actor", "agent:builder-1", "--reason", "wrong reviewer"); err == nil || !strings.Contains(out+err.Error(), "reviewer_policy_mismatch") {
		t.Fatalf("expected reviewer policy mismatch before transition, err=%v out=%s", err, out)
	}
	policyView := must("ticket", "view", "POL-1", "--json")
	if !strings.Contains(policyView, `"status": "in_progress"`) {
		t.Fatalf("invalid reviewer should not change state: %s", policyView)
	}
}
