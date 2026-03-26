package cli

import (
	"encoding/json"
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
