package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGateLifecycleApprovalsAndInbox(t *testing.T) {
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
	must("ticket", "create", "--project", "APP", "--title", "Review me", "--type", "task", "--reviewer", "agent:reviewer-1", "--actor", "human:owner")
	must("agent", "create", "reviewer-1", "--name", "Reviewer One", "--provider", "claude", "--default-runbook", "review", "--actor", "human:owner")

	dispatchOut := must("run", "dispatch", "APP-1", "--agent", "reviewer-1", "--kind", "review", "--actor", "human:owner", "--json")
	var dispatch struct {
		Payload struct {
			RunID string `json:"run_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(dispatchOut), &dispatch); err != nil {
		t.Fatalf("parse dispatch output: %v\nraw=%s", err, dispatchOut)
	}
	must("run", "start", dispatch.Payload.RunID, "--actor", "human:owner")

	handoffOut := must("run", "handoff", dispatch.Payload.RunID, "--actor", "human:owner", "--json")
	var handoff struct {
		Payload struct {
			HandoffID string `json:"handoff_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(handoffOut), &handoff); err != nil {
		t.Fatalf("parse handoff output: %v\nraw=%s", err, handoffOut)
	}

	gateListOut := must("gate", "list", "--ticket", "APP-1", "--json")
	var gateList struct {
		Kind  string `json:"kind"`
		Items []struct {
			GateID          string `json:"gate_id"`
			State           string `json:"state"`
			Kind            string `json:"kind"`
			RequiredAgentID string `json:"required_agent_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(gateListOut), &gateList); err != nil {
		t.Fatalf("parse gate list: %v\nraw=%s", err, gateListOut)
	}
	if gateList.Kind != "gate_list" || len(gateList.Items) != 1 || gateList.Items[0].Kind != "review" || gateList.Items[0].RequiredAgentID != "agent:reviewer-1" {
		t.Fatalf("unexpected gate list payload: %#v", gateList)
	}
	firstGateID := gateList.Items[0].GateID

	if out, err := runCLI(t, "run", "complete", dispatch.Payload.RunID, "--actor", "human:owner"); err == nil || !strings.Contains(err.Error(), "gates are open") {
		t.Fatalf("expected run completion to be blocked, err=%v out=%s", err, out)
	}
	if out, err := runCLI(t, "gate", "approve", firstGateID, "--actor", "agent:reviewer-2"); err == nil || !strings.Contains(err.Error(), "cannot decide") {
		t.Fatalf("expected wrong reviewer rejection, err=%v out=%s", err, out)
	}

	must("gate", "reject", firstGateID, "--actor", "agent:reviewer-1", "--reason", "needs another pass")
	runAfterReject := must("run", "view", dispatch.Payload.RunID, "--json")
	var rejectedRun struct {
		Payload struct {
			Run struct {
				Status string `json:"status"`
			} `json:"run"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(runAfterReject), &rejectedRun); err != nil {
		t.Fatalf("parse run after reject: %v\nraw=%s", err, runAfterReject)
	}
	if rejectedRun.Payload.Run.Status != "active" {
		t.Fatalf("expected rejected gate to push run back to active, got %#v", rejectedRun)
	}

	must("run", "handoff", dispatch.Payload.RunID, "--actor", "human:owner")
	secondGateListOut := must("gate", "list", "--ticket", "APP-1", "--json")
	var secondGateList struct {
		Items []struct {
			GateID         string `json:"gate_id"`
			State          string `json:"state"`
			ReplacesGateID string `json:"replaces_gate_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(secondGateListOut), &secondGateList); err != nil {
		t.Fatalf("parse second gate list: %v\nraw=%s", err, secondGateListOut)
	}
	if len(secondGateList.Items) != 2 || secondGateList.Items[1].State != "open" || secondGateList.Items[1].ReplacesGateID != firstGateID {
		t.Fatalf("expected reopened gate to replace the rejected gate, got %#v", secondGateList.Items)
	}

	secondGateID := secondGateList.Items[1].GateID
	must("gate", "approve", secondGateID, "--actor", "agent:reviewer-1")

	approvalsOut := must("approvals", "--json")
	var approvals struct {
		Kind  string `json:"kind"`
		Items []any  `json:"items"`
	}
	if err := json.Unmarshal([]byte(approvalsOut), &approvals); err != nil {
		t.Fatalf("parse approvals: %v\nraw=%s", err, approvalsOut)
	}
	if approvals.Kind != "approvals_list" || len(approvals.Items) != 0 {
		t.Fatalf("expected approvals to clear, got %#v", approvals)
	}

	inboxOut := must("inbox", "--json")
	var inbox struct {
		Kind  string `json:"kind"`
		Items []struct {
			ID        string `json:"id"`
			Kind      string `json:"kind"`
			HandoffID string `json:"handoff_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(inboxOut), &inbox); err != nil {
		t.Fatalf("parse inbox: %v\nraw=%s", err, inboxOut)
	}
	if inbox.Kind != "inbox_list" || len(inbox.Items) != 1 || inbox.Items[0].Kind != "handoff" || !strings.HasPrefix(inbox.Items[0].ID, "handoff:") {
		t.Fatalf("expected handoff inbox item after approval, got %#v", inbox)
	}
	if inbox.Items[0].HandoffID == "" {
		t.Fatalf("expected handoff id in inbox payload, got %#v", inbox)
	}

	detailOut := must("inbox", "view", inbox.Items[0].ID, "--json")
	var detail struct {
		Kind    string `json:"kind"`
		Payload struct {
			Item struct {
				ID string `json:"id"`
			} `json:"item"`
			Handoff struct {
				HandoffID string `json:"handoff_id"`
			} `json:"handoff"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(detailOut), &detail); err != nil {
		t.Fatalf("parse inbox detail: %v\nraw=%s", err, detailOut)
	}
	if detail.Kind != "inbox_detail" || detail.Payload.Handoff.HandoffID == "" {
		t.Fatalf("unexpected inbox detail payload: %#v", detail)
	}

	must("reindex")
	postReindexGates := must("gate", "list", "--ticket", "APP-1", "--json")
	var persisted struct {
		Items []struct {
			State string `json:"state"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(postReindexGates), &persisted); err != nil {
		t.Fatalf("parse post-reindex gates: %v\nraw=%s", err, postReindexGates)
	}
	if len(persisted.Items) != 2 || persisted.Items[0].State != "rejected" || persisted.Items[1].State != "approved" {
		t.Fatalf("expected gate history to survive reindex, got %#v", persisted.Items)
	}
}
