package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestChangeAndChecksCommandsFlowIntoTicketRunAndHandoffViews(t *testing.T) {
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
	must("ticket", "create", "--project", "APP", "--title", "Wire change flow", "--type", "task", "--actor", "human:owner")
	must("agent", "create", "builder-1", "--name", "Builder One", "--provider", "codex", "--capability", "go", "--actor", "human:owner")

	dispatchOut := must("run", "dispatch", "APP-1", "--agent", "builder-1", "--actor", "human:owner", "--json")
	var dispatch struct {
		Payload struct {
			RunID string `json:"run_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(dispatchOut), &dispatch); err != nil {
		t.Fatalf("parse dispatch output: %v\nraw=%s", err, dispatchOut)
	}
	must("run", "start", dispatch.Payload.RunID, "--actor", "human:owner")

	changeOut := must(
		"change", "link", "APP-1",
		"--run", dispatch.Payload.RunID,
		"--status", "approved",
		"--branch", "feat/app-1",
		"--base", "main",
		"--review-summary", "Ready once checks are green.",
		"--actor", "human:owner",
		"--json",
	)
	var changeView struct {
		Kind    string `json:"kind"`
		Payload struct {
			Change struct {
				ChangeID string `json:"change_id"`
				RunID    string `json:"run_id"`
			} `json:"change"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(changeOut), &changeView); err != nil {
		t.Fatalf("parse change link output: %v\nraw=%s", err, changeOut)
	}
	if changeView.Kind != "change_detail" || changeView.Payload.Change.ChangeID == "" || changeView.Payload.Change.RunID != dispatch.Payload.RunID {
		t.Fatalf("unexpected change link payload: %#v", changeView)
	}
	changeID := changeView.Payload.Change.ChangeID

	checkOut := must(
		"checks", "record",
		"--scope", "change",
		"--id", changeID,
		"--name", "unit",
		"--status", "completed",
		"--conclusion", "success",
		"--actor", "human:owner",
		"--json",
	)
	var changeCheck struct {
		Kind    string `json:"kind"`
		Payload struct {
			CheckID string `json:"check_id"`
			Scope   string `json:"scope"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(checkOut), &changeCheck); err != nil {
		t.Fatalf("parse change check output: %v\nraw=%s", err, checkOut)
	}
	if changeCheck.Kind != "check_detail" || changeCheck.Payload.Scope != "change" {
		t.Fatalf("unexpected change check payload: %#v", changeCheck)
	}

	runCheckOut := must(
		"checks", "record",
		"--scope", "run",
		"--id", dispatch.Payload.RunID,
		"--name", "smoke",
		"--status", "completed",
		"--conclusion", "success",
		"--actor", "human:owner",
		"--json",
	)
	var runCheck struct {
		Payload struct {
			CheckID string `json:"check_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(runCheckOut), &runCheck); err != nil {
		t.Fatalf("parse run check output: %v\nraw=%s", err, runCheckOut)
	}
	if runCheck.Payload.CheckID == "" {
		t.Fatalf("expected run check id")
	}

	changeListOut := must("change", "list", "--ticket", "APP-1", "--json")
	var changeList struct {
		Kind  string `json:"kind"`
		Items []struct {
			ChangeID string `json:"change_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(changeListOut), &changeList); err != nil {
		t.Fatalf("parse change list output: %v\nraw=%s", err, changeListOut)
	}
	if changeList.Kind != "change_list" || len(changeList.Items) != 1 || changeList.Items[0].ChangeID != changeID {
		t.Fatalf("unexpected change list: %#v", changeList)
	}

	checkListOut := must("checks", "list", "--scope", "change", "--id", changeID, "--json")
	var checkList struct {
		Kind  string `json:"kind"`
		Items []struct {
			CheckID string `json:"check_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(checkListOut), &checkList); err != nil {
		t.Fatalf("parse check list output: %v\nraw=%s", err, checkListOut)
	}
	if checkList.Kind != "check_list" || len(checkList.Items) != 1 {
		t.Fatalf("unexpected check list: %#v", checkList)
	}

	ticketViewOut := must("ticket", "view", "APP-1", "--json")
	var ticketView struct {
		Ticket struct {
			ChangeReadyState string `json:"change_ready_state"`
		} `json:"ticket"`
		Changes []struct {
			ChangeID string `json:"change_id"`
		} `json:"changes"`
		Checks []struct {
			CheckID string `json:"check_id"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(ticketViewOut), &ticketView); err != nil {
		t.Fatalf("parse ticket view output: %v\nraw=%s", err, ticketViewOut)
	}
	if ticketView.Ticket.ChangeReadyState != "merge_ready" || len(ticketView.Changes) != 1 || len(ticketView.Checks) != 1 {
		t.Fatalf("unexpected ticket view payload: %#v", ticketView)
	}

	runViewOut := must("run", "view", dispatch.Payload.RunID, "--json")
	var runView struct {
		Kind    string `json:"kind"`
		Payload struct {
			Changes []struct {
				ChangeID string `json:"change_id"`
			} `json:"changes"`
			Checks []struct {
				CheckID string `json:"check_id"`
			} `json:"checks"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(runViewOut), &runView); err != nil {
		t.Fatalf("parse run view output: %v\nraw=%s", err, runViewOut)
	}
	if runView.Kind != "run_detail" || len(runView.Payload.Changes) != 1 || len(runView.Payload.Checks) != 1 {
		t.Fatalf("unexpected run view payload: %#v", runView)
	}

	handoffOut := must("run", "handoff", dispatch.Payload.RunID, "--next-actor", "agent:reviewer-1", "--next-gate", "review", "--actor", "human:owner", "--json")
	var handoff struct {
		Payload struct {
			HandoffID string `json:"handoff_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(handoffOut), &handoff); err != nil {
		t.Fatalf("parse handoff output: %v\nraw=%s", err, handoffOut)
	}

	handoffViewOut := must("handoff", "view", handoff.Payload.HandoffID, "--json")
	var handoffView struct {
		Kind    string `json:"kind"`
		Payload struct {
			HandoffID string `json:"handoff_id"`
			Changes   []struct {
				ChangeID string `json:"change_id"`
			} `json:"changes"`
			Checks []struct {
				CheckID string `json:"check_id"`
			} `json:"checks"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(handoffViewOut), &handoffView); err != nil {
		t.Fatalf("parse handoff view output: %v\nraw=%s", err, handoffViewOut)
	}
	if handoffView.Kind != "handoff_detail" || handoffView.Payload.HandoffID == "" || len(handoffView.Payload.Changes) != 1 || len(handoffView.Payload.Checks) != 1 {
		t.Fatalf("unexpected handoff view payload: %#v", handoffView)
	}

	must("change", "unlink", "APP-1", changeID, "--actor", "human:owner")
	postUnlinkOut := must("ticket", "view", "APP-1", "--json")
	var postUnlink struct {
		Ticket struct {
			ChangeReadyState string `json:"change_ready_state"`
		} `json:"ticket"`
		Changes []struct {
			ChangeID string `json:"change_id"`
		} `json:"changes"`
	}
	if err := json.Unmarshal([]byte(postUnlinkOut), &postUnlink); err != nil {
		t.Fatalf("parse post-unlink ticket view: %v\nraw=%s", err, postUnlinkOut)
	}
	if postUnlink.Ticket.ChangeReadyState != "no_linked_change" || len(postUnlink.Changes) != 0 {
		t.Fatalf("unexpected post-unlink ticket payload: %#v", postUnlink)
	}
}

func TestChangeCreateStatusSyncAndImportURLFlow(t *testing.T) {
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
	must("config", "set", "provider.default_scm_provider", "github")
	must("config", "set", "provider.github_repo", "myrrazor/atlas-tasker")
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Ship provider sync", "--type", "task", "--actor", "human:owner")
	must("ticket", "create", "--project", "APP", "--title", "Import provider URL", "--type", "task", "--actor", "human:owner")
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
	if dispatch.Payload.WorktreePath == "" {
		t.Fatalf("expected worktree path in dispatch payload: %s", dispatchOut)
	}
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

	createOut := must("change", "create", dispatch.Payload.RunID, "--actor", "human:owner", "--json")
	var create struct {
		Kind    string `json:"kind"`
		Payload struct {
			Created bool `json:"created"`
			Change  struct {
				ChangeID   string `json:"change_id"`
				Provider   string `json:"provider"`
				Status     string `json:"status"`
				BranchName string `json:"branch_name"`
			} `json:"change"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(createOut), &create); err != nil {
		t.Fatalf("parse change create output: %v\nraw=%s", err, createOut)
	}
	if create.Kind != "change_create_result" || !create.Payload.Created || create.Payload.Change.ChangeID == "" {
		t.Fatalf("unexpected create payload: %#v", create)
	}
	if create.Payload.Change.Provider != "github" || create.Payload.Change.Status != "local_only" {
		t.Fatalf("expected local-only github change, got %#v", create.Payload.Change)
	}
	changeID := create.Payload.Change.ChangeID

	changeViewOut := must("change", "view", changeID, "--json")
	var changeView struct {
		Payload struct {
			ChangedFiles []string `json:"changed_files"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(changeViewOut), &changeView); err != nil {
		t.Fatalf("parse change view output: %v\nraw=%s", err, changeViewOut)
	}
	if len(changeView.Payload.ChangedFiles) != 1 || changeView.Payload.ChangedFiles[0] != "pkg/feature.txt" {
		t.Fatalf("expected worktree change summary, got %#v", changeView.Payload.ChangedFiles)
	}

	statusOut := must("change", "status", changeID, "--json")
	var status struct {
		Kind        string   `json:"kind"`
		ReasonCodes []string `json:"reason_codes"`
		Payload     struct {
			ObservedStatus       string `json:"observed_status"`
			ObservedChecksStatus string `json:"observed_checks_status"`
			Change               struct {
				Status string `json:"status"`
			} `json:"change"`
			PullRequest struct {
				Number int    `json:"number"`
				URL    string `json:"url"`
			} `json:"pull_request"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(statusOut), &status); err != nil {
		t.Fatalf("parse change status output: %v\nraw=%s", err, statusOut)
	}
	if status.Kind != "change_status" || status.Payload.Change.Status != "local_only" || status.Payload.ObservedStatus != "merge_ready" || status.Payload.ObservedChecksStatus != "passing" || status.Payload.PullRequest.Number != 42 {
		t.Fatalf("unexpected change status payload: %#v", status)
	}

	syncOut := must("change", "sync", changeID, "--actor", "human:owner", "--json")
	var sync struct {
		Kind    string `json:"kind"`
		Payload struct {
			Change struct {
				Status       string `json:"status"`
				ChecksStatus string `json:"checks_status"`
				URL          string `json:"url"`
			} `json:"change"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(syncOut), &sync); err != nil {
		t.Fatalf("parse change sync output: %v\nraw=%s", err, syncOut)
	}
	if sync.Kind != "change_status" || sync.Payload.Change.Status != "merge_ready" || sync.Payload.Change.ChecksStatus != "passing" || sync.Payload.Change.URL == "" {
		t.Fatalf("unexpected change sync payload: %#v", sync)
	}

	checkSyncOut := must("checks", "sync", changeID, "--actor", "human:owner", "--json")
	var checkSync struct {
		Kind    string `json:"kind"`
		Payload struct {
			Aggregate string `json:"aggregate"`
			Checks    []struct {
				Source string `json:"source"`
				Scope  string `json:"scope"`
			} `json:"checks"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(checkSyncOut), &checkSync); err != nil {
		t.Fatalf("parse check sync output: %v\nraw=%s", err, checkSyncOut)
	}
	if checkSync.Kind != "check_sync_result" || checkSync.Payload.Aggregate != "passing" || len(checkSync.Payload.Checks) != 1 || checkSync.Payload.Checks[0].Source != "provider" || checkSync.Payload.Checks[0].Scope != "change" {
		t.Fatalf("unexpected check sync payload: %#v", checkSync)
	}

	ticketViewOut := must("ticket", "view", "APP-1", "--json")
	var ticketView struct {
		Ticket struct {
			ChangeReadyState string `json:"change_ready_state"`
		} `json:"ticket"`
	}
	if err := json.Unmarshal([]byte(ticketViewOut), &ticketView); err != nil {
		t.Fatalf("parse ticket view: %v\nraw=%s", err, ticketViewOut)
	}
	if ticketView.Ticket.ChangeReadyState != "merge_ready" {
		t.Fatalf("expected merge_ready after provider sync, got %#v", ticketView)
	}

	importOut := must("change", "import-url", "APP-2", "--url", "https://github.com/myrrazor/atlas-tasker/pull/43", "--actor", "human:owner", "--json")
	var imported struct {
		Payload struct {
			Change struct {
				ChangeID string `json:"change_id"`
				Status   string `json:"status"`
				URL      string `json:"url"`
			} `json:"change"`
			Ticket struct {
				ID string `json:"id"`
			} `json:"ticket"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(importOut), &imported); err != nil {
		t.Fatalf("parse import url output: %v\nraw=%s", err, importOut)
	}
	if imported.Payload.Ticket.ID != "APP-2" || imported.Payload.Change.URL != "https://github.com/myrrazor/atlas-tasker/pull/43" || imported.Payload.Change.Status != "draft" {
		t.Fatalf("unexpected import-url payload: %#v", imported)
	}

	importedViewOut := must("change", "view", imported.Payload.Change.ChangeID, "--json")
	var importedView struct {
		Payload struct {
			ChangedFiles []string `json:"changed_files"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(importedViewOut), &importedView); err != nil {
		t.Fatalf("parse imported change view: %v\nraw=%s", err, importedViewOut)
	}
	if len(importedView.Payload.ChangedFiles) != 0 {
		t.Fatalf("expected imported change without local branch to hide workspace drift, got %#v", importedView.Payload.ChangedFiles)
	}

	reviewRequestOut := must("change", "review-request", imported.Payload.Change.ChangeID, "--actor", "human:owner", "--json")
	var reviewRequested struct {
		Payload struct {
			Change struct {
				Status string `json:"status"`
			} `json:"change"`
			ObservedStatus string `json:"observed_status"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(reviewRequestOut), &reviewRequested); err != nil {
		t.Fatalf("parse review-request output: %v\nraw=%s", err, reviewRequestOut)
	}
	if reviewRequested.Payload.Change.Status != "review_requested" || reviewRequested.Payload.ObservedStatus != "review_requested" {
		t.Fatalf("expected imported change to move into review_requested, got %#v", reviewRequested)
	}

	if out, err := runCLI(t, "change", "merge", changeID, "--actor", "agent:builder-1", "--json"); err == nil {
		t.Fatalf("expected non-owner merge to fail\n%s", out)
	}

	mergeOut := must("change", "merge", changeID, "--actor", "human:owner", "--json")
	var merged struct {
		Payload struct {
			Change struct {
				Status string `json:"status"`
			} `json:"change"`
			ObservedStatus string `json:"observed_status"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(mergeOut), &merged); err != nil {
		t.Fatalf("parse merge output: %v\nraw=%s", err, mergeOut)
	}
	if merged.Payload.Change.Status != "merged" || merged.Payload.ObservedStatus != "merged" {
		t.Fatalf("expected merged provider change, got %#v", merged)
	}
}

func installFakeGHProviderForCLI(t *testing.T, branch string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "gh")
	state42 := filepath.Join(dir, "pr42.state")
	state43 := filepath.Join(dir, "pr43.state")
	if err := os.WriteFile(state42, []byte("open"), 0o644); err != nil {
		t.Fatalf("write fake pr42 state: %v", err)
	}
	if err := os.WriteFile(state43, []byte("draft"), 0o644); err != nil {
		t.Fatalf("write fake pr43 state: %v", err)
	}
	script := `#!/bin/sh
set -eu
STATE_42="` + state42 + `"
STATE_43="` + state43 + `"
case "$1 $2" in
  "auth status")
    exit 0
    ;;
  "repo view")
    echo '{"nameWithOwner":"myrrazor/atlas-tasker","url":"https://github.com/myrrazor/atlas-tasker"}'
    ;;
  "pr list")
    echo '[{"number":42,"title":"APP-1: Ship provider sync","url":"https://github.com/myrrazor/atlas-tasker/pull/42","state":"OPEN","isDraft":false,"headRefName":"` + branch + `","baseRefName":"main","reviewDecision":"APPROVED","mergeStateStatus":"CLEAN","mergedAt":""}]'
    ;;
  "pr view")
    case "$3" in
      *"/pull/42"|42)
        if [ "$(cat "$STATE_42")" = "merged" ]; then
          echo '{"number":42,"title":"APP-1: Ship provider sync","url":"https://github.com/myrrazor/atlas-tasker/pull/42","state":"CLOSED","isDraft":false,"headRefName":"` + branch + `","baseRefName":"main","reviewDecision":"APPROVED","mergeStateStatus":"MERGED","mergedAt":"2026-03-26T17:20:00Z"}'
        else
          echo '{"number":42,"title":"APP-1: Ship provider sync","url":"https://github.com/myrrazor/atlas-tasker/pull/42","state":"OPEN","isDraft":false,"headRefName":"` + branch + `","baseRefName":"main","reviewDecision":"APPROVED","mergeStateStatus":"CLEAN","mergedAt":""}'
        fi
        ;;
      *)
        if [ "$(cat "$STATE_43")" = "ready" ]; then
          echo '{"number":43,"title":"APP-2: Import provider URL","url":"https://github.com/myrrazor/atlas-tasker/pull/43","state":"OPEN","isDraft":false,"headRefName":"ticket/app-2-import-provider-url","baseRefName":"main","reviewDecision":"REVIEW_REQUIRED","mergeStateStatus":"BLOCKED","mergedAt":""}'
        else
          echo '{"number":43,"title":"APP-2: Import provider URL","url":"https://github.com/myrrazor/atlas-tasker/pull/43","state":"OPEN","isDraft":true,"headRefName":"ticket/app-2-import-provider-url","baseRefName":"main","reviewDecision":"REVIEW_REQUIRED","mergeStateStatus":"BLOCKED","mergedAt":""}'
        fi
        ;;
    esac
    ;;
  "pr checks")
    case "$3" in
      *"/pull/42"|42)
        echo '[{"bucket":"pass","completedAt":"2026-03-26T17:15:00Z","description":"green","link":"https://github.com/myrrazor/atlas-tasker/actions/runs/1","name":"unit","startedAt":"2026-03-26T17:10:00Z","state":"SUCCESS","workflow":"ci"}]'
        ;;
      *)
        echo '[]'
        ;;
    esac
    ;;
  "pr ready")
    echo ready > "$STATE_43"
    ;;
  "pr merge")
    echo merged > "$STATE_42"
    ;;
  *)
    echo "unexpected gh args: $*" >&2
    exit 1
    ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
