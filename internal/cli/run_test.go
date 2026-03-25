package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDispatchLifecycleAttachAndCleanup(t *testing.T) {
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
	must("ticket", "create", "--project", "APP", "--title", "Build runner", "--type", "task", "--actor", "human:owner")
	must("agent", "create", "builder-1", "--name", "Builder One", "--provider", "codex", "--capability", "go", "--actor", "human:owner")

	dispatchOut := must("run", "dispatch", "APP-1", "--agent", "builder-1", "--actor", "human:owner", "--json")
	var dispatch struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Payload       struct {
			RunID        string `json:"run_id"`
			AgentID      string `json:"agent_id"`
			WorktreePath string `json:"worktree_path"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(dispatchOut), &dispatch); err != nil {
		t.Fatalf("parse dispatch output: %v\nraw=%s", err, dispatchOut)
	}
	if dispatch.FormatVersion != jsonFormatVersion || dispatch.Kind != "run_dispatch_result" || dispatch.Payload.RunID == "" {
		t.Fatalf("unexpected dispatch payload: %#v", dispatch)
	}
	if dispatch.Payload.AgentID != "builder-1" {
		t.Fatalf("unexpected agent: %#v", dispatch)
	}
	if dispatch.Payload.WorktreePath == "" {
		t.Fatalf("expected managed worktree path, got %#v", dispatch)
	}
	if _, err := os.Stat(dispatch.Payload.WorktreePath); err != nil {
		t.Fatalf("expected worktree to exist: %v", err)
	}

	if out, err := runCLI(t, "run", "dispatch", "APP-1", "--agent", "builder-1", "--actor", "human:owner"); err == nil || !strings.Contains(err.Error(), "parallel_runs_disabled") {
		t.Fatalf("expected second dispatch rejection, err=%v out=%s", err, out)
	}

	must("run", "start", dispatch.Payload.RunID, "--actor", "human:owner")
	must("run", "attach", dispatch.Payload.RunID, "--provider", "codex", "--session-ref", "sess-1", "--actor", "human:owner")
	must("run", "attach", dispatch.Payload.RunID, "--provider", "codex", "--session-ref", "sess-1", "--actor", "human:owner")
	if out, err := runCLI(t, "run", "attach", dispatch.Payload.RunID, "--provider", "codex", "--session-ref", "sess-2", "--actor", "human:owner"); err == nil || !strings.Contains(err.Error(), "already attached") {
		t.Fatalf("expected conflicting attach failure, err=%v out=%s", err, out)
	}

	if err := os.WriteFile(filepath.Join(dispatch.Payload.WorktreePath, "dirty.txt"), []byte("oops\n"), 0o644); err != nil {
		t.Fatalf("write dirty worktree file: %v", err)
	}
	must("run", "abort", dispatch.Payload.RunID, "--actor", "human:owner", "--summary", "stopping here")
	if out, err := runCLI(t, "run", "cleanup", dispatch.Payload.RunID, "--actor", "human:owner"); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected dirty cleanup failure, err=%v out=%s", err, out)
	}
	must("run", "cleanup", dispatch.Payload.RunID, "--actor", "human:owner", "--force")
	if _, err := os.Stat(dispatch.Payload.WorktreePath); !os.IsNotExist(err) {
		t.Fatalf("expected cleaned worktree to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(".tracker", "runtime", dispatch.Payload.RunID)); !os.IsNotExist(err) {
		t.Fatalf("expected runtime dir cleanup, err=%v", err)
	}
}

func TestReindexAndRepairDoNotRecreateWorktrees(t *testing.T) {
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
	must("ticket", "create", "--project", "APP", "--title", "Repair runner", "--type", "task", "--actor", "human:owner")
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
	if err := os.RemoveAll(dispatch.Payload.WorktreePath); err != nil {
		t.Fatalf("remove worktree: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(".tracker", "runtime", dispatch.Payload.RunID)); err != nil {
		t.Fatalf("remove runtime dir: %v", err)
	}

	must("reindex")
	must("doctor", "--repair")

	worktreeOut := must("worktree", "view", dispatch.Payload.RunID, "--json")
	var detail struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Payload       struct {
			Present bool   `json:"present"`
			Path    string `json:"path"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(worktreeOut), &detail); err != nil {
		t.Fatalf("parse worktree detail: %v\nraw=%s", err, worktreeOut)
	}
	if detail.Kind != "worktree_detail" || detail.Payload.Present {
		t.Fatalf("expected worktree drift to remain drift, got %#v", detail)
	}
	if _, err := os.Stat(dispatch.Payload.WorktreePath); !os.IsNotExist(err) {
		t.Fatalf("expected reindex/repair not to recreate worktree, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(".tracker", "runtime", dispatch.Payload.RunID)); !os.IsNotExist(err) {
		t.Fatalf("expected reindex/repair not to recreate runtime dir, err=%v", err)
	}
}
