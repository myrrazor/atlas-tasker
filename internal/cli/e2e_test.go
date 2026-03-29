package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
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

func TestShellParityForCollaboratorMembershipAndMentionsCommands(t *testing.T) {
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
	must("ticket", "create", "--project", "APP", "--title", "Mentions", "--type", "task", "--actor", "human:owner")

	runSlashShell(t, `/collaborator add rev-1 --name "Rev One" --actor-map agent:reviewer-1 --actor human:owner`)
	runSlashShell(t, `/collaborator list`)
	runSlashShell(t, `/collaborator view rev-1`)
	runSlashShell(t, `/membership bind rev-1 --scope-kind project --scope-id APP --role reviewer --actor human:owner`)
	runSlashShell(t, `/membership list --collaborator rev-1`)

	must("ticket", "comment", "APP-1", "--body", "review with @rev-1", "--actor", "human:owner")

	mentionsOut := must("mentions", "list", "--collaborator", "rev-1", "--json")
	var mentions struct {
		Items []struct {
			MentionUID string `json:"mention_uid"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(mentionsOut), &mentions); err != nil {
		t.Fatalf("parse mentions list: %v\nraw=%s", err, mentionsOut)
	}
	if len(mentions.Items) != 1 || mentions.Items[0].MentionUID == "" {
		t.Fatalf("expected one mention, got %#v", mentions.Items)
	}

	runSlashShell(t, `/mentions list --collaborator rev-1`)
	runSlashShell(t, `/mentions view `+mentions.Items[0].MentionUID)
}

func TestShellParityForImportExportCommands(t *testing.T) {
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
	must("ticket", "create", "--project", "APP", "--title", "Ship import/export", "--type", "task", "--actor", "human:owner")

	runSlashShell(t, "/export create --scope workspace --actor human:owner")
	runSlashShell(t, "/export list")

	exportListOut := must("export", "list", "--json")
	var exportList struct {
		Items []struct {
			BundleID string `json:"bundle_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(exportListOut), &exportList); err != nil {
		t.Fatalf("parse export list: %v\nraw=%s", err, exportListOut)
	}
	if len(exportList.Items) != 1 || exportList.Items[0].BundleID == "" {
		t.Fatalf("expected one export bundle, got %#v", exportList.Items)
	}
	bundleID := exportList.Items[0].BundleID

	runSlashShell(t, "/export view "+bundleID)
	runSlashShell(t, "/export verify "+bundleID+" --actor human:owner")

	csvPath := filepath.Join(".", "shell-import.csv")
	csv := "Project Key,Project Name,Issue Key,Summary,Issue Type,Status,Priority\nSHP,Shell Imports,SHP-1,Shell imported task,task,ready,medium\n"
	if err := os.WriteFile(csvPath, []byte(csv), 0o644); err != nil {
		t.Fatalf("write shell import csv: %v", err)
	}

	runSlashShell(t, "/import preview "+csvPath+" --actor human:owner")
	runSlashShell(t, "/import list")

	importListOut := must("import", "list", "--json")
	var importList struct {
		Items []struct {
			JobID string `json:"job_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(importListOut), &importList); err != nil {
		t.Fatalf("parse import list: %v\nraw=%s", err, importListOut)
	}
	if len(importList.Items) != 1 || importList.Items[0].JobID == "" {
		t.Fatalf("expected one import job, got %#v", importList.Items)
	}
	jobID := importList.Items[0].JobID

	runSlashShell(t, "/import view "+jobID)
	runSlashShell(t, "/import apply "+jobID+" --actor human:owner")

	ticketViewOut := must("ticket", "view", "SHP-1", "--json")
	if !json.Valid([]byte(ticketViewOut)) {
		t.Fatalf("expected imported ticket json view, got %s", ticketViewOut)
	}
}

func TestShellParityForChangeAndChecksCommands(t *testing.T) {
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
	must("config", "set", "provider.default_scm_provider", "github")
	must("config", "set", "provider.github_repo", "myrrazor/atlas-tasker")
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Shell change parity", "--type", "task", "--actor", "human:owner")
	must("ticket", "create", "--project", "APP", "--title", "Shell import parity", "--type", "task", "--actor", "human:owner")
	must("agent", "create", "builder-1", "--name", "Builder One", "--provider", "codex", "--capability", "go", "--actor", "human:owner")

	dispatchOut := must("run", "dispatch", "APP-1", "--agent", "builder-1", "--actor", "human:owner", "--json")
	var dispatch struct {
		Payload struct {
			RunID        string `json:"run_id"`
			WorktreePath string `json:"worktree_path"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(dispatchOut), &dispatch); err != nil {
		t.Fatalf("parse dispatch output: %v\nraw=%s", err, dispatchOut)
	}
	must("run", "start", dispatch.Payload.RunID, "--actor", "human:owner")
	if err := os.MkdirAll(filepath.Join(dispatch.Payload.WorktreePath, "pkg"), 0o755); err != nil {
		t.Fatalf("mkdir worktree dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dispatch.Payload.WorktreePath, "pkg", "feature.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write worktree file: %v", err)
	}

	runViewOut := must("run", "view", dispatch.Payload.RunID, "--json")
	var runView struct {
		Payload struct {
			Run struct {
				BranchName string `json:"branch_name"`
			} `json:"run"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(runViewOut), &runView); err != nil {
		t.Fatalf("parse run view: %v\nraw=%s", err, runViewOut)
	}
	installFakeGHProviderForCLI(t, runView.Payload.Run.BranchName)

	runSlashShell(t, "/change create "+dispatch.Payload.RunID+" --actor human:owner")
	changeListOut := must("change", "list", "--ticket", "APP-1", "--json")
	var changeList struct {
		Items []struct {
			ChangeID string `json:"change_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(changeListOut), &changeList); err != nil {
		t.Fatalf("parse change list: %v\nraw=%s", err, changeListOut)
	}
	if len(changeList.Items) != 1 || changeList.Items[0].ChangeID == "" {
		t.Fatalf("expected one change, got %#v", changeList.Items)
	}
	changeID := changeList.Items[0].ChangeID

	runSlashShell(t, "/change list --ticket APP-1")
	runSlashShell(t, "/change view "+changeID)
	runSlashShell(t, "/change status "+changeID)
	runSlashShell(t, "/change sync "+changeID+" --actor human:owner")
	runSlashShell(t, `/checks record --scope run --id `+dispatch.Payload.RunID+` --name "smoke" --status completed --conclusion success --actor human:owner`)
	runSlashShell(t, "/checks sync "+changeID+" --actor human:owner")
	runSlashShell(t, "/checks list --scope change --id "+changeID)

	checkListOut := must("checks", "list", "--scope", "run", "--id", dispatch.Payload.RunID, "--json")
	var checkList struct {
		Items []struct {
			CheckID string `json:"check_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(checkListOut), &checkList); err != nil {
		t.Fatalf("parse check list: %v\nraw=%s", err, checkListOut)
	}
	if len(checkList.Items) == 0 || checkList.Items[0].CheckID == "" {
		t.Fatalf("expected recorded run check, got %#v", checkList.Items)
	}
	runSlashShell(t, "/checks view "+checkList.Items[0].CheckID)

	runSlashShell(t, "/change import-url APP-2 --url https://github.com/myrrazor/atlas-tasker/pull/43 --actor human:owner")
	importedListOut := must("change", "list", "--ticket", "APP-2", "--json")
	if err := json.Unmarshal([]byte(importedListOut), &changeList); err != nil {
		t.Fatalf("parse imported change list: %v\nraw=%s", err, importedListOut)
	}
	if len(changeList.Items) != 1 || changeList.Items[0].ChangeID == "" {
		t.Fatalf("expected imported change, got %#v", changeList.Items)
	}
	importedChangeID := changeList.Items[0].ChangeID
	runSlashShell(t, "/change review-request "+importedChangeID+" --actor human:owner")
	runSlashShell(t, "/change merge "+changeID+" --actor human:owner")
}

func TestShellParityForArchiveCommands(t *testing.T) {
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
	must("ticket", "create", "--project", "APP", "--title", "Archive parity", "--type", "task", "--actor", "human:owner")

	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	old := time.Now().UTC().AddDate(0, 0, -10)
	run := contracts.RunSnapshot{
		RunID:         "run_shell_archive",
		TicketID:      "APP-1",
		Project:       "APP",
		Status:        contracts.RunStatusCompleted,
		Kind:          contracts.RunKindWork,
		CreatedAt:     old,
		CompletedAt:   old,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := (service.RunStore{Root: root}).SaveRun(commandContext(nil), run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	for _, path := range []string{
		storage.RuntimeBriefFile(root, run.RunID),
		storage.RuntimeContextFile(root, run.RunID),
		storage.RuntimeLaunchFile(root, run.RunID, "codex"),
		storage.RuntimeLaunchFile(root, run.RunID, "claude"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir runtime dir: %v", err)
		}
		if err := os.WriteFile(path, []byte("runtime"), 0o644); err != nil {
			t.Fatalf("write runtime artifact: %v", err)
		}
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatalf("chtimes %s: %v", path, err)
		}
	}
	if err := os.Chtimes(storage.RuntimeDir(root, run.RunID), old, old); err != nil {
		t.Fatalf("chtimes runtime dir: %v", err)
	}

	runSlashShell(t, "/archive plan --target runtime --project APP")
	runSlashShell(t, "/archive apply --target runtime --project APP --yes --actor human:owner")
	runSlashShell(t, "/archive list --target runtime")

	listOut := must("archive", "list", "--target", "runtime", "--json")
	var listed struct {
		Items []struct {
			ArchiveID string `json:"archive_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(listOut), &listed); err != nil {
		t.Fatalf("parse archive list: %v\nraw=%s", err, listOut)
	}
	if len(listed.Items) != 1 || listed.Items[0].ArchiveID == "" {
		t.Fatalf("expected one archive record, got %#v", listed.Items)
	}

	runSlashShell(t, "/archive restore "+listed.Items[0].ArchiveID+" --actor human:owner")
}

func TestShellParityForCompactCommands(t *testing.T) {
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
	must("ticket", "create", "--project", "APP", "--title", "Compact parity", "--type", "task", "--actor", "human:owner")

	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	old := time.Now().UTC().AddDate(0, 0, -10)
	run := contracts.RunSnapshot{
		RunID:         "run_shell_compact",
		TicketID:      "APP-1",
		Project:       "APP",
		Status:        contracts.RunStatusCompleted,
		Kind:          contracts.RunKindWork,
		CreatedAt:     old,
		CompletedAt:   old,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := (service.RunStore{Root: root}).SaveRun(commandContext(nil), run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	for _, path := range []string{
		storage.RuntimeBriefFile(root, run.RunID),
		storage.RuntimeLaunchFile(root, run.RunID, "codex"),
		storage.RuntimeLaunchFile(root, run.RunID, "claude"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir runtime dir: %v", err)
		}
		if err := os.WriteFile(path, []byte("runtime"), 0o644); err != nil {
			t.Fatalf("write runtime artifact: %v", err)
		}
	}

	runSlashShell(t, "/compact --yes --actor human:owner")
}

func TestShellParityForDashboardAndTimelineCommands(t *testing.T) {
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
	must("ticket", "create", "--project", "APP", "--title", "Timeline parity", "--type", "task", "--actor", "human:owner")
	must("ticket", "comment", "APP-1", "--body", "shell timeline note", "--actor", "agent:builder-1")

	runSlashShell(t, "/dashboard")
	runSlashShell(t, "/timeline APP-1")
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
