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
	refs := decodeJSONList[map[string]any](t, refsOut)
	if len(refs) != 1 {
		t.Fatalf("unexpected refs payload: %s", refsOut)
	}

	statusOut := must("git", "status", "--json")
	var status struct {
		FormatVersion string `json:"format_version"`
		Present       bool   `json:"present"`
	}
	if err := json.Unmarshal([]byte(statusOut), &status); err != nil {
		t.Fatalf("parse git status: %v\nraw=%s", err, statusOut)
	}
	if status.FormatVersion != jsonFormatVersion || !status.Present {
		t.Fatalf("unexpected git status: %#v", status)
	}

	writeGitFile(t, "feature-2.txt", "second\n")
	gitRunCLI(t, "add", "feature-2.txt")
	if _, err := runCLI(t, "git", "commit", "APP-1", "--message", "ship it"); err != nil {
		t.Fatalf("git commit command failed: %v", err)
	}

	writeGitFile(t, ".tracker/local-only.log", "leave me out\n")
	trackedFiles := gitOutput(t, "show", "--name-only", "--format=", "HEAD")
	if !strings.Contains(trackedFiles, "feature-2.txt") {
		t.Fatalf("expected staged file in commit, got %s", trackedFiles)
	}
	if strings.Contains(trackedFiles, ".tracker/local-only.log") {
		t.Fatalf("expected unstaged tracker metadata to stay out of commit, got %s", trackedFiles)
	}
	statusPretty := must("git", "status", "--pretty")
	if !strings.Contains(statusPretty, "dirty=true") {
		t.Fatalf("expected unstaged file to keep repo dirty, got %s", statusPretty)
	}
}

func TestGitCommitRejectsDetachedHeadAndNestedRepos(t *testing.T) {
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

	writeGitFile(t, "detached.txt", "hi\n")
	gitRunCLI(t, "add", "detached.txt")
	gitRunCLI(t, "checkout", "--detach", "HEAD")
	if out, err := runCLI(t, "git", "commit", "APP-1", "--message", "detached"); err == nil || !strings.Contains(err.Error(), "checked-out branch") {
		t.Fatalf("expected detached head failure, got err=%v out=%s", err, out)
	}

	gitRunCLI(t, "checkout", "main")
	writeGitFile(t, "nested-ready.txt", "hi\n")
	gitRunCLI(t, "add", "nested-ready.txt")
	if err := os.MkdirAll("nested-repo", 0o755); err != nil {
		t.Fatalf("mkdir nested repo: %v", err)
	}
	gitRunCLI(t, "-C", "nested-repo", "init", "-b", "main")
	if out, err := runCLI(t, "git", "commit", "APP-1", "--message", "nested"); err == nil || !strings.Contains(err.Error(), "nested git repo detected") {
		t.Fatalf("expected nested repo failure, got err=%v out=%s", err, out)
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

func gitOutput(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
	return string(output)
}

func writeGitFile(t *testing.T, name string, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(".", name), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
