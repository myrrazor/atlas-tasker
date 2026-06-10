package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func installFakeGHForGHCommands(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

const fakeGHCapabilityScript = `#!/bin/sh
set -eu
case "$1 $2" in
  "auth status")
    exit 0
    ;;
  "repo view")
    echo '{"nameWithOwner":"myrrazor/atlas-tasker","url":"https://github.com/myrrazor/atlas-tasker"}'
    ;;
  *)
    echo "unexpected gh args: $*" >&2
    exit 1
    ;;
esac
`

func TestGHStatusReportsCapabilityJSON(t *testing.T) {
	withTempWorkspace(t)
	if out, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	installFakeGHForGHCommands(t, fakeGHCapabilityScript)

	out, err := runCLI(t, "gh", "status", "--json")
	if err != nil {
		t.Fatalf("gh status failed: %v\n%s", err, out)
	}
	var status struct {
		Kind    string `json:"kind"`
		Payload struct {
			Installed     bool   `json:"installed"`
			Authenticated bool   `json:"authenticated"`
			Repo          string `json:"repo"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(out), &status); err != nil {
		t.Fatalf("parse gh status: %v\nraw=%s", err, out)
	}
	if status.Kind != "gh_status" || !status.Payload.Installed || !status.Payload.Authenticated || status.Payload.Repo != "myrrazor/atlas-tasker" {
		t.Fatalf("unexpected gh status payload: %#v", status)
	}
}

func TestGHStatusGracefulWhenGHMissing(t *testing.T) {
	withTempWorkspace(t)
	if out, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	t.Setenv("PATH", t.TempDir())

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exit := Execute([]string{"gh", "status", "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("expected gh status to exit 0 without gh, got %d\nstderr=%s", exit, stderr.String())
	}
	var status struct {
		Kind    string `json:"kind"`
		Payload struct {
			Installed     bool `json:"installed"`
			Authenticated bool `json:"authenticated"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		t.Fatalf("parse gh status: %v\nraw=%s", err, stdout.String())
	}
	if status.Kind != "gh_status" || status.Payload.Installed || status.Payload.Authenticated {
		t.Fatalf("expected installed=false authenticated=false, got %#v", status)
	}
}

const fakeGHReadScript = `#!/bin/sh
set -eu
case "$1 $2" in
  "auth status")
    exit 0
    ;;
  "repo view")
    echo '{"nameWithOwner":"myrrazor/atlas-tasker","url":"https://github.com/myrrazor/atlas-tasker"}'
    ;;
  "pr list")
    echo '[{"number":42,"title":"APP-1: Tighten locks","url":"https://github.com/myrrazor/atlas-tasker/pull/42","state":"OPEN","isDraft":false,"headRefName":"ticket/app-1-tighten-locks","baseRefName":"main","reviewDecision":"APPROVED","mergeStateStatus":"CLEAN","mergedAt":""}]'
    ;;
  "pr view")
    echo '{"number":42,"title":"APP-1: Tighten locks","url":"https://github.com/myrrazor/atlas-tasker/pull/42","state":"OPEN","isDraft":false,"headRefName":"ticket/app-1-tighten-locks","baseRefName":"main","reviewDecision":"APPROVED","mergeStateStatus":"CLEAN","mergedAt":""}'
    ;;
  "pr checks")
    echo '[{"bucket":"pass","completedAt":"2026-03-26T17:15:00Z","description":"green","link":"https://github.com/myrrazor/atlas-tasker/actions/runs/1","name":"unit","startedAt":"2026-03-26T17:10:00Z","state":"SUCCESS","workflow":"ci"}]'
    ;;
  *)
    echo "unexpected gh args: $*" >&2
    exit 1
    ;;
esac
`

func setupGHWorkspace(t *testing.T, titles ...string) {
	t.Helper()
	must := func(args ...string) {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}
	must("init")
	must("config", "set", "provider.default_scm_provider", "github")
	must("config", "set", "provider.github_repo", "myrrazor/atlas-tasker")
	must("project", "create", "APP", "App Project")
	for _, title := range titles {
		must("ticket", "create", "--project", "APP", "--title", title, "--type", "task", "--actor", "human:owner")
	}
}

func TestGHPRsListsTicketPullRequests(t *testing.T) {
	withTempWorkspace(t)
	setupGHWorkspace(t, "Tighten locks")
	installFakeGHForGHCommands(t, fakeGHReadScript)

	out, err := runCLI(t, "gh", "prs", "APP-1", "--json")
	if err != nil {
		t.Fatalf("gh prs failed: %v\n%s", err, out)
	}
	var prs struct {
		Kind    string `json:"kind"`
		Payload struct {
			TicketID        string `json:"ticket_id"`
			Repo            string `json:"repo"`
			SuggestedBranch string `json:"suggested_branch"`
			GitHub          struct {
				PullRequests []struct {
					Number         int    `json:"number"`
					ReviewDecision string `json:"review_decision"`
					URL            string `json:"url"`
				} `json:"pull_requests"`
			} `json:"github"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(out), &prs); err != nil {
		t.Fatalf("parse gh prs: %v\nraw=%s", err, out)
	}
	if prs.Kind != "gh_pr_list" || prs.Payload.TicketID != "APP-1" || prs.Payload.Repo != "myrrazor/atlas-tasker" {
		t.Fatalf("unexpected gh prs envelope: %#v", prs)
	}
	if prs.Payload.SuggestedBranch != "ticket/app-1-tighten-locks" {
		t.Fatalf("unexpected suggested branch: %q", prs.Payload.SuggestedBranch)
	}
	if len(prs.Payload.GitHub.PullRequests) != 1 || prs.Payload.GitHub.PullRequests[0].Number != 42 || prs.Payload.GitHub.PullRequests[0].ReviewDecision != "approved" {
		t.Fatalf("unexpected pull requests: %#v", prs.Payload.GitHub.PullRequests)
	}
}

func TestGHChecksAcceptsTicketAndPRRef(t *testing.T) {
	withTempWorkspace(t)
	setupGHWorkspace(t, "Tighten locks")
	installFakeGHForGHCommands(t, fakeGHReadScript)

	for _, ref := range []string{"APP-1", "42"} {
		out, err := runCLI(t, "gh", "checks", ref, "--json")
		if err != nil {
			t.Fatalf("gh checks %s failed: %v\n%s", ref, err, out)
		}
		var checks struct {
			Kind  string `json:"kind"`
			Items []struct {
				Bucket   string `json:"bucket"`
				Name     string `json:"name"`
				Workflow string `json:"workflow"`
			} `json:"items"`
		}
		if err := json.Unmarshal([]byte(out), &checks); err != nil {
			t.Fatalf("parse gh checks %s: %v\nraw=%s", ref, err, out)
		}
		if checks.Kind != "gh_check_list" || len(checks.Items) != 1 || checks.Items[0].Bucket != "pass" || checks.Items[0].Workflow != "ci" {
			t.Fatalf("unexpected gh checks payload for %s: %#v", ref, checks)
		}
	}
}

func TestGHCreatePRLinksChangeAndRecordsEvent(t *testing.T) {
	withTempWorkspace(t)
	setupGHWorkspace(t, "Ship gh adapter")

	scriptDir := t.TempDir()
	argsFile := filepath.Join(scriptDir, "create.args")
	script := `#!/bin/sh
set -eu
ARGS="` + argsFile + `"
case "$1 $2" in
  "auth status")
    exit 0
    ;;
  "repo view")
    echo '{"nameWithOwner":"myrrazor/atlas-tasker","url":"https://github.com/myrrazor/atlas-tasker"}'
    ;;
  "pr create")
    printf '%s\n' "$*" > "$ARGS"
    echo "https://github.com/myrrazor/atlas-tasker/pull/77"
    ;;
  "pr view")
    echo '{"number":77,"title":"APP-1: Ship gh adapter","url":"https://github.com/myrrazor/atlas-tasker/pull/77","state":"OPEN","isDraft":false,"headRefName":"ticket/app-1-ship-gh-adapter","baseRefName":"main","reviewDecision":"","mergeStateStatus":"CLEAN","mergedAt":""}'
    ;;
  "pr checks")
    echo '[]'
    ;;
  *)
    echo "unexpected gh args: $*" >&2
    exit 1
    ;;
esac
`
	if err := os.WriteFile(filepath.Join(scriptDir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	out, err := runCLI(t, "gh", "create-pr", "APP-1", "--body", "ready for eyes", "--actor", "human:owner", "--reason", "open pr for review", "--json")
	if err != nil {
		t.Fatalf("gh create-pr failed: %v\n%s", err, out)
	}
	var created struct {
		Kind    string `json:"kind"`
		Payload struct {
			Created bool `json:"created"`
			Change  struct {
				ChangeID   string `json:"change_id"`
				Provider   string `json:"provider"`
				URL        string `json:"url"`
				ExternalID string `json:"external_id"`
				HeadRef    string `json:"head_ref"`
				Status     string `json:"status"`
			} `json:"change"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatalf("parse gh create-pr: %v\nraw=%s", err, out)
	}
	if created.Kind != "change_create_result" || !created.Payload.Created {
		t.Fatalf("unexpected create-pr envelope: %#v", created)
	}
	change := created.Payload.Change
	if change.Provider != "github" || change.URL != "https://github.com/myrrazor/atlas-tasker/pull/77" || change.ExternalID != "77" || change.HeadRef != "ticket/app-1-ship-gh-adapter" {
		t.Fatalf("unexpected linked change: %#v", change)
	}

	// defaults: title from ticket, head from the suggested branch
	recorded, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read recorded gh args: %v", err)
	}
	for _, want := range []string{"--title APP-1: Ship gh adapter", "--head ticket/app-1-ship-gh-adapter", "--body ready for eyes"} {
		if !strings.Contains(string(recorded), want) {
			t.Fatalf("expected pr create args to contain %q, got %q", want, string(recorded))
		}
	}

	listOut, err := runCLI(t, "change", "list", "--ticket", "APP-1", "--json")
	if err != nil {
		t.Fatalf("change list failed: %v\n%s", err, listOut)
	}
	var changeList struct {
		Items []struct {
			ChangeID string `json:"change_id"`
			Provider string `json:"provider"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(listOut), &changeList); err != nil {
		t.Fatalf("parse change list: %v\nraw=%s", err, listOut)
	}
	if len(changeList.Items) != 1 || changeList.Items[0].ChangeID != change.ChangeID || changeList.Items[0].Provider != "github" {
		t.Fatalf("expected linked github change in change list, got %#v", changeList)
	}

	historyOut, err := runCLI(t, "ticket", "history", "APP-1", "--json")
	if err != nil {
		t.Fatalf("ticket history failed: %v\n%s", err, historyOut)
	}
	var history struct {
		Events []struct {
			Type   string `json:"type"`
			Actor  string `json:"actor"`
			Reason string `json:"reason"`
		} `json:"events"`
	}
	if err := json.Unmarshal([]byte(historyOut), &history); err != nil {
		t.Fatalf("parse ticket history: %v\nraw=%s", err, historyOut)
	}
	found := false
	for _, event := range history.Events {
		if event.Type == "change.created" && event.Actor == "human:owner" && event.Reason == "open pr for review" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected change.created event with actor+reason, got %#v", history.Events)
	}
}

func TestGHRequestReviewMovesDraftToReady(t *testing.T) {
	withTempWorkspace(t)
	gitRunCLI(t, "init", "-b", "main")
	gitRunCLI(t, "config", "user.email", "atlas@example.com")
	gitRunCLI(t, "config", "user.name", "Atlas")
	writeGitFile(t, "README.md", "# atlas\n")
	gitRunCLI(t, "add", "README.md")
	gitRunCLI(t, "commit", "-m", "init")
	setupGHWorkspace(t, "Review my draft", "No change here")
	installFakeGHProviderForCLI(t, "ticket/app-1-review-my-draft")

	importOut, err := runCLI(t, "gh", "import-url", "APP-1", "--url", "https://github.com/myrrazor/atlas-tasker/pull/43", "--actor", "human:owner", "--json")
	if err != nil {
		t.Fatalf("gh import-url failed: %v\n%s", err, importOut)
	}
	var imported struct {
		Payload struct {
			Change struct {
				ChangeID string `json:"change_id"`
				Status   string `json:"status"`
			} `json:"change"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(importOut), &imported); err != nil {
		t.Fatalf("parse gh import-url: %v\nraw=%s", err, importOut)
	}
	if imported.Payload.Change.Status != "draft" {
		t.Fatalf("expected imported draft PR, got %#v", imported)
	}

	// ticket ref resolves the linked github change, then the stub flips draft -> ready
	reviewOut, err := runCLI(t, "gh", "request-review", "APP-1", "--actor", "human:owner", "--reason", "ready for review", "--json")
	if err != nil {
		t.Fatalf("gh request-review failed: %v\n%s", err, reviewOut)
	}
	var review struct {
		Kind    string `json:"kind"`
		Payload struct {
			Change struct {
				ChangeID string `json:"change_id"`
				Status   string `json:"status"`
			} `json:"change"`
			ObservedStatus string `json:"observed_status"`
			PullRequest    struct {
				Draft bool `json:"draft"`
			} `json:"pull_request"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(reviewOut), &review); err != nil {
		t.Fatalf("parse gh request-review: %v\nraw=%s", err, reviewOut)
	}
	if review.Kind != "change_status" || review.Payload.Change.ChangeID != imported.Payload.Change.ChangeID {
		t.Fatalf("unexpected request-review envelope: %#v", review)
	}
	if review.Payload.Change.Status != "review_requested" || review.Payload.ObservedStatus != "review_requested" || review.Payload.PullRequest.Draft {
		t.Fatalf("expected draft PR to move to review_requested, got %#v", review.Payload)
	}

	// change id works as the ref too
	if out, err := runCLI(t, "gh", "request-review", imported.Payload.Change.ChangeID, "--actor", "human:owner", "--json"); err != nil {
		t.Fatalf("gh request-review by change id failed: %v\n%s", err, out)
	}

	// no linked change -> not_found with a pointer at create-pr/import-url
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := Execute([]string{"gh", "request-review", "APP-2", "--actor", "human:owner"}, &stdout, &stderr)
	if exit != 3 {
		t.Fatalf("expected not_found exit 3 for unlinked ticket, got %d\nstderr=%s", exit, stderr.String())
	}
	if !strings.Contains(stderr.String(), "no linked change") || !strings.Contains(stderr.String(), "tracker gh create-pr") {
		t.Fatalf("expected no-linked-change hint, got: %s", stderr.String())
	}
}

func TestGHImportURLMatchesChangeImportEnvelope(t *testing.T) {
	withTempWorkspace(t)
	setupGHWorkspace(t, "Import via gh", "Import via change")
	installFakeGHProviderForCLI(t, "ticket/app-1-import-via-gh")

	type importEnvelope struct {
		Kind    string `json:"kind"`
		Payload struct {
			Created bool `json:"created"`
			Change  struct {
				Provider   string `json:"provider"`
				URL        string `json:"url"`
				ExternalID string `json:"external_id"`
				Status     string `json:"status"`
				BaseBranch string `json:"base_branch"`
			} `json:"change"`
		} `json:"payload"`
	}
	parse := func(raw string) importEnvelope {
		t.Helper()
		var envelope importEnvelope
		if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
			t.Fatalf("parse import envelope: %v\nraw=%s", err, raw)
		}
		return envelope
	}

	ghOut, err := runCLI(t, "gh", "import-url", "APP-1", "--url", "https://github.com/myrrazor/atlas-tasker/pull/43", "--actor", "human:owner", "--json")
	if err != nil {
		t.Fatalf("gh import-url failed: %v\n%s", err, ghOut)
	}
	changeOut, err := runCLI(t, "change", "import-url", "APP-2", "--url", "https://github.com/myrrazor/atlas-tasker/pull/43", "--actor", "human:owner", "--json")
	if err != nil {
		t.Fatalf("change import-url failed: %v\n%s", err, changeOut)
	}

	viaGH := parse(ghOut)
	viaChange := parse(changeOut)
	if viaGH.Kind != "change_create_result" || viaGH.Kind != viaChange.Kind {
		t.Fatalf("expected matching envelope kinds, got %q vs %q", viaGH.Kind, viaChange.Kind)
	}
	if !viaGH.Payload.Created || viaGH.Payload.Change != viaChange.Payload.Change {
		t.Fatalf("expected gh import-url to mirror change import-url, got %#v vs %#v", viaGH.Payload, viaChange.Payload)
	}
}

func TestGHMutationRejectsInvalidActor(t *testing.T) {
	withTempWorkspace(t)
	setupGHWorkspace(t, "Actor check")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := Execute([]string{"gh", "create-pr", "APP-1", "--actor", "banana", "--json"}, &stdout, &stderr)
	if exit != 2 {
		t.Fatalf("expected invalid_input exit 2, got %d\nstderr=%s", exit, stderr.String())
	}
	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatalf("parse error envelope: %v\nraw=%s", err, stderr.String())
	}
	if envelope.OK || envelope.Error.Code != "invalid_input" {
		t.Fatalf("unexpected error envelope: %#v", envelope)
	}
}

func TestGHViewRequiresGHInstalled(t *testing.T) {
	withTempWorkspace(t)
	if out, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	t.Setenv("PATH", t.TempDir())

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := Execute([]string{"gh", "view", "42"}, &stdout, &stderr)
	if exit != 7 {
		t.Fatalf("expected repair_needed exit 7 without gh, got %d\nstderr=%s", exit, stderr.String())
	}
	if !strings.Contains(stderr.String(), "install GitHub CLI and run 'gh auth login'") {
		t.Fatalf("expected install guidance, got: %s", stderr.String())
	}
}
