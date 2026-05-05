package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitCommands(t *testing.T) {
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
	must("ticket", "create", "--project", "APP", "--title", "Git me", "--type", "task", "--actor", "human:owner")

	branch := must("git", "branch-name", "APP-1", "--pretty")
	if !strings.Contains(branch, "ticket/app-1-git-me") {
		t.Fatalf("unexpected branch suggestion: %s", branch)
	}

	gitRunCLI(t, "checkout", "-b", "ticket/app-1-git-me")
	writeGitFile(t, "feature.txt", "hi\n")
	gitRunCLI(t, "add", "feature.txt")
	gitRunCLI(t, "commit", "-m", "APP-1: add feature")

	refsOut := must("git", "refs", "APP-1", "--json")
	var refs []map[string]any
	if err := json.Unmarshal([]byte(refsOut), &refs); err != nil {
		t.Fatalf("parse refs json: %v\nraw=%s", err, refsOut)
	}
	if len(refs) != 1 {
		t.Fatalf("unexpected refs payload: %s", refsOut)
	}

	statusOut := must("git", "status", "--json")
	if !strings.Contains(statusOut, "\"present\": true") {
		t.Fatalf("unexpected git status: %s", statusOut)
	}
}

func gitRunCLI(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}

func writeGitFile(t *testing.T, name string, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(".", name), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
