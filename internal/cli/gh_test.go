package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitHubCommands(t *testing.T) {
	withTempWorkspace(t)
	gitRunCLI(t, "init", "-b", "main")
	gitRunCLI(t, "config", "user.email", "atlas@example.com")
	gitRunCLI(t, "config", "user.name", "Atlas")
	writeGitFile(t, "README.md", "# atlas\n")
	gitRunCLI(t, "add", "README.md")
	gitRunCLI(t, "commit", "-m", "init")
	gitRunCLI(t, "checkout", "-b", "ticket/app-1-tighten-locks")
	installFakeGHForCLI(t)

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
	must("ticket", "create", "--project", "APP", "--title", "tighten locks", "--type", "task", "--actor", "human:owner")

	status := must("gh", "status", "--json")
	if !strings.Contains(status, "\"installed\": true") || !strings.Contains(status, "\"authenticated\": true") {
		t.Fatalf("unexpected gh status: %s", status)
	}

	prs := must("gh", "prs", "APP-1", "--json")
	var prList []map[string]any
	if err := json.Unmarshal([]byte(prs), &prList); err != nil {
		t.Fatalf("parse gh prs: %v\nraw=%s", err, prs)
	}
	if len(prList) != 1 {
		t.Fatalf("unexpected gh prs payload: %s", prs)
	}

	create := must("gh", "create-pr", "APP-1", "--draft", "--base", "testing", "--actor", "human:owner", "--json")
	if !strings.Contains(create, "\"number\": 43") {
		t.Fatalf("unexpected create-pr payload: %s", create)
	}
	history := must("ticket", "history", "APP-1", "--json")
	if !strings.Contains(history, "https://github.com/myrrazor/atlas-tasker/pull/43") {
		t.Fatalf("expected pr url in history comment: %s", history)
	}

	must("gh", "import-url", "APP-1", "--url", "https://github.com/myrrazor/atlas-tasker/issues/99", "--actor", "human:owner")
	history = must("ticket", "history", "APP-1", "--json")
	if !strings.Contains(history, "issues/99") {
		t.Fatalf("expected imported github url in history: %s", history)
	}
}

func installFakeGHForCLI(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "gh")
	script := `#!/bin/sh
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
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
