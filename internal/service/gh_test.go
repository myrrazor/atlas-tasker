package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
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
    echo '[{"number":42,"title":"APP-1: tighten locks","url":"https://github.com/myrrazor/atlas-tasker/pull/42","state":"OPEN","isDraft":false,"headRefName":"ticket/app-1-tighten-locks","baseRefName":"testing"}]'
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
	if len(pulls) != 1 || pulls[0].Number != 42 {
		t.Fatalf("unexpected pull requests: %#v", pulls)
	}
}

func TestGHCreatePullRequest(t *testing.T) {
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
  "pr create")
    echo 'https://github.com/myrrazor/atlas-tasker/pull/43'
    ;;
  "pr view")
    echo '{"number":43,"title":"APP-1: tighten locks","url":"https://github.com/myrrazor/atlas-tasker/pull/43","state":"OPEN","isDraft":true,"headRefName":"ticket/app-1-tighten-locks","baseRefName":"testing"}'
    ;;
  *)
    echo "unexpected gh args: $*" >&2
    exit 1
    ;;
esac
`)

	pr, err := (GHService{Root: root}).CreatePullRequest(context.Background(), contracts.TicketSnapshot{
		ID:          "APP-1",
		Title:       "tighten locks",
		Description: "lock the write path",
	}, "", "", "testing", true)
	if err != nil {
		t.Fatalf("create pr: %v", err)
	}
	if pr.Number != 43 || !pr.Draft || pr.BaseRef != "testing" {
		t.Fatalf("unexpected pr payload: %#v", pr)
	}
}

func TestGHImportReferenceURL(t *testing.T) {
	service := GHService{}
	valid, err := service.ImportReferenceURL("https://github.com/myrrazor/atlas-tasker/pull/44")
	if err != nil {
		t.Fatalf("import url: %v", err)
	}
	if valid == "" {
		t.Fatal("expected validated GitHub URL")
	}
	if _, err := service.ImportReferenceURL("https://example.com/not-github"); err == nil {
		t.Fatal("expected non-github url to fail")
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
