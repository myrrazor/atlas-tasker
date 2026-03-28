package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
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
	must("ticket", "approve", "APP-2", "--actor", "human:owner")
	must("ticket", "move", "APP-2", "done", "--actor", "human:owner")

	must("ticket", "comment", "APP-4", "--body", "first", "--actor", "agent:builder-1")
	must("ticket", "comment", "APP-4", "--body", "second", "--actor", "agent:builder-1")

	board := must("board", "--project", "APP", "--pretty")
	if !strings.Contains(board, "APP-2") || !strings.Contains(board, "Done") {
		t.Fatalf("board output missing expected ticket/state: %s", board)
	}
	if !strings.Contains(board, "Blocked (1)") || !strings.Contains(board, "APP-4 Bug") {
		t.Fatalf("board output missing blocked ticket placement: %s", board)
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
	if !strings.Contains(reindexedBoard, "Blocked (1)") || !strings.Contains(reindexedBoard, "APP-4 Bug") {
		t.Fatalf("reindex board missing blocked placement: %s", reindexedBoard)
	}
}

func TestShellParityForCoreCommands(t *testing.T) {
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
		runSlashShell(t, slash)
	}
}

func TestShellParityForOrchestrationCommands(t *testing.T) {
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
			t.Fatalf("command failed %v: %v\noutput=%s", args, err, out)
		}
		return out
	}

	must("init")
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Shell runner", "--type", "task", "--reviewer", "agent:reviewer-1", "--actor", "human:owner")
	must("ticket", "move", "APP-1", "ready", "--actor", "human:owner")

	runSlashShell(t, `/agent create builder-1 --name "Builder One" --provider codex --capability go --actor human:owner`)
	runSlashShell(t, `/agent list`)
	runSlashShell(t, `/agent view builder-1`)
	runSlashShell(t, `/permission-profile create audit-ops --name "Audit Ops" --workspace-default --actor human:owner`)
	runSlashShell(t, `/permission-profile list`)
	runSlashShell(t, `/permission-profile view audit-ops`)
	runSlashShell(t, `/permission-profile bind audit-ops --project APP --actor human:owner`)
	runSlashShell(t, `/permissions view APP-1 --actor human:owner --action dispatch`)
	runSlashShell(t, `/permission-profile unbind audit-ops --project APP --actor human:owner`)
	runSlashShell(t, `/agent eligible APP-1`)
	runSlashShell(t, `/dispatch suggest APP-1`)
	runSlashShell(t, `/dispatch queue`)
	runSlashShell(t, `/dispatch run APP-1 --agent builder-1 --actor human:owner`)

	runListOut := must("run", "list", "--json")
	var runList struct {
		Items []struct {
			RunID string `json:"run_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(runListOut), &runList); err != nil {
		t.Fatalf("parse run list: %v\nraw=%s", err, runListOut)
	}
	if len(runList.Items) != 1 || runList.Items[0].RunID == "" {
		t.Fatalf("expected one dispatched run, got %#v", runList.Items)
	}
	runID := runList.Items[0].RunID

	if err := os.WriteFile("proof.log", []byte("shell proof\n"), 0o644); err != nil {
		t.Fatalf("write proof artifact: %v", err)
	}

	runSlashShell(t, "/run view "+runID)
	runSlashShell(t, "/run start "+runID+" --actor human:owner")
	runSlashShell(t, "/run attach "+runID+" --provider codex --session-ref sess-1 --actor human:owner")
	runSlashShell(t, "/run open "+runID)
	runSlashShell(t, "/run launch "+runID+" --actor human:owner")
	runSlashShell(t, `/run checkpoint `+runID+` --title "shell checkpoint" --body "runtime ready" --actor human:owner`)
	runSlashShell(t, `/run evidence add `+runID+` --type note --title "shell evidence" --body "captured from slash shell" --artifact proof.log --actor human:owner`)
	runSlashShell(t, `/evidence list `+runID)

	evidenceListOut := must("evidence", "list", runID, "--json")
	var evidenceList struct {
		Items []struct {
			EvidenceID string `json:"evidence_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(evidenceListOut), &evidenceList); err != nil {
		t.Fatalf("parse evidence list: %v\nraw=%s", err, evidenceListOut)
	}
	if len(evidenceList.Items) < 2 || evidenceList.Items[0].EvidenceID == "" {
		t.Fatalf("expected evidence items, got %#v", evidenceList.Items)
	}
	evidenceID := evidenceList.Items[0].EvidenceID
	runSlashShell(t, "/evidence view "+evidenceID)

	runSlashShell(t, `/run handoff `+runID+` --next-actor agent:reviewer-1 --next-gate review --actor human:owner`)
	runSlashShell(t, `/approvals`)
	runSlashShell(t, `/inbox`)

	approvalsOut := must("approvals", "--json")
	var approvals struct {
		Items []struct {
			Gate struct {
				GateID string `json:"gate_id"`
			} `json:"gate"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(approvalsOut), &approvals); err != nil {
		t.Fatalf("parse approvals: %v\nraw=%s", err, approvalsOut)
	}
	if len(approvals.Items) != 1 || approvals.Items[0].Gate.GateID == "" {
		t.Fatalf("expected one approval gate, got %#v", approvals.Items)
	}
	gateID := approvals.Items[0].Gate.GateID

	runViewOut := must("run", "view", runID, "--json")
	var runView struct {
		Payload struct {
			Handoffs []struct {
				HandoffID string `json:"handoff_id"`
			} `json:"handoffs"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(runViewOut), &runView); err != nil {
		t.Fatalf("parse run view: %v\nraw=%s", err, runViewOut)
	}
	if len(runView.Payload.Handoffs) != 1 || runView.Payload.Handoffs[0].HandoffID == "" {
		t.Fatalf("expected one handoff, got %#v", runView.Payload.Handoffs)
	}
	handoffID := runView.Payload.Handoffs[0].HandoffID

	runSlashShell(t, "/gate view "+gateID)
	runSlashShell(t, "/inbox view gate:"+gateID)
	runSlashShell(t, "/handoff view "+handoffID)
	runSlashShell(t, "/handoff export "+handoffID)
	runSlashShell(t, "/gate approve "+gateID+" --actor agent:reviewer-1")
	runSlashShell(t, "/run complete "+runID+` --summary "shell done" --actor human:owner`)
	runSlashShell(t, "/worktree view "+runID)
	runSlashShell(t, "/worktree list")
	runSlashShell(t, "/run cleanup "+runID+" --actor human:owner")
	runSlashShell(t, "/worktree repair")
	runSlashShell(t, "/worktree prune")

	historyOut := must("ticket", "history", "APP-1", "--json")
	var history struct {
		Events []struct {
			Type     string `json:"type"`
			Metadata struct {
				Surface contracts.EventSurface `json:"surface"`
			} `json:"metadata"`
		} `json:"events"`
	}
	if err := json.Unmarshal([]byte(historyOut), &history); err != nil {
		t.Fatalf("parse ticket history: %v\nraw=%s", err, historyOut)
	}
	var sawShellRun bool
	for _, event := range history.Events {
		if event.Metadata.Surface == contracts.EventSurfaceShell && strings.HasPrefix(event.Type, "run.") {
			sawShellRun = true
			break
		}
	}
	if !sawShellRun {
		t.Fatalf("expected slash shell mutations to persist shell surface metadata on run events, got %s", historyOut)
	}
}

func runSlashShell(t *testing.T, slash string) {
	t.Helper()
	args, err := ParseSlashCommand(slash)
	if err != nil {
		t.Fatalf("parse slash command %q failed: %v", slash, err)
	}
	if err := executeArgsWithSurface(args, contracts.EventSurfaceShell); err != nil {
		t.Fatalf("execute slash command %q failed: %v", slash, err)
	}
}
