package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGHCapabilityAndPullRequests(t *testing.T) {
	root := t.TempDir()
	installFakeGH(t, `#!/bin/sh
set -eu
case "$1 $2" in
  "auth status")
    exit 0
    ;;
  "repo view")
    echo '{"nameWithOwner":"myrrazor/atlas-tasker","url":"https://github.com/myrrazor/atlas-tasker"}'
    ;;
  "pr list")
    echo '[{"number":42,"title":"APP-1: tighten locks","url":"https://github.com/myrrazor/atlas-tasker/pull/42","state":"OPEN","isDraft":false,"headRefName":"ticket/app-1-tighten-locks","baseRefName":"main","reviewDecision":"APPROVED","mergeStateStatus":"CLEAN","mergedAt":""}]'
    ;;
  *)
    echo "unexpected gh args: $*" >&2
    exit 1
    ;;
esac
`)

	service := GHService{Root: root}
	capability, err := service.Capability(context.Background())
	if err != nil {
		t.Fatalf("capability: %v", err)
	}
	if !capability.Installed || !capability.Authenticated || capability.Repo != "myrrazor/atlas-tasker" {
		t.Fatalf("unexpected capability: %#v", capability)
	}
	pulls, err := service.PullRequests(context.Background(), "APP-1", "ticket/app-1-tighten-locks")
	if err != nil {
		t.Fatalf("pull requests: %v", err)
	}
	if len(pulls) != 1 || pulls[0].Number != 42 || pulls[0].ReviewDecision != "approved" {
		t.Fatalf("unexpected pull requests: %#v", pulls)
	}
}

func TestGHPullRequestChecksAndImportURL(t *testing.T) {
	root := t.TempDir()
	installFakeGH(t, `#!/bin/sh
set -eu
case "$1 $2" in
  "auth status")
    exit 0
    ;;
  "repo view")
    echo '{"nameWithOwner":"myrrazor/atlas-tasker","url":"https://github.com/myrrazor/atlas-tasker"}'
    ;;
  "pr view")
    echo '{"number":43,"title":"APP-1: tighten locks","url":"https://github.com/myrrazor/atlas-tasker/pull/43","state":"OPEN","isDraft":true,"headRefName":"ticket/app-1-tighten-locks","baseRefName":"main","reviewDecision":"REVIEW_REQUIRED","mergeStateStatus":"BLOCKED","mergedAt":""}'
    ;;
  "pr checks")
    echo '[{"bucket":"pass","completedAt":"2026-03-26T17:15:00Z","description":"green","link":"https://github.com/myrrazor/atlas-tasker/actions/runs/1","name":"unit","startedAt":"2026-03-26T17:10:00Z","state":"SUCCESS","workflow":"ci"}]'
    ;;
  *)
    echo "unexpected gh args: $*" >&2
    exit 1
    ;;
esac
`)

	service := GHService{Root: root}
	pr, err := service.PullRequestView(context.Background(), "https://github.com/myrrazor/atlas-tasker/pull/43")
	if err != nil {
		t.Fatalf("pull request view: %v", err)
	}
	if pr.Number != 43 || !pr.Draft || pr.ReviewDecision != "review_required" {
		t.Fatalf("unexpected pr payload: %#v", pr)
	}
	checks, err := service.PullRequestChecks(context.Background(), pr.URL)
	if err != nil {
		t.Fatalf("pull request checks: %v", err)
	}
	if len(checks) != 1 || checks[0].Bucket != "pass" || checks[0].Workflow != "ci" {
		t.Fatalf("unexpected checks payload: %#v", checks)
	}
	valid, number, err := service.ImportPullRequestURL("https://github.com/myrrazor/atlas-tasker/pull/43/files#diff")
	if err != nil {
		t.Fatalf("import url: %v", err)
	}
	if valid != "https://github.com/myrrazor/atlas-tasker/pull/43" || number != 43 {
		t.Fatalf("unexpected import url result: %q %d", valid, number)
	}
	if _, _, err := service.ImportPullRequestURL("https://github.com/myrrazor/atlas-tasker/issues/99"); err == nil {
		t.Fatal("expected issue url to fail for change import")
	}
}

func TestGHCapabilityGracefullyHandlesMissingCLI(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", root)

	capability, err := (GHService{Root: root}).Capability(context.Background())
	if err != nil {
		t.Fatalf("capability: %v", err)
	}
	if capability.Installed || capability.Authenticated {
		t.Fatalf("expected missing gh to degrade gracefully, got %#v", capability)
	}
}

func installFakeGH(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "gh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
