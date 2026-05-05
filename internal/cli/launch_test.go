package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRunOpenAndLaunchManageRuntimeArtifacts(t *testing.T) {
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
	must("ticket", "create", "--project", "APP", "--title", "Launch manifests", "--type", "task", "--actor", "human:owner")
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

	openOut := must("run", "open", dispatch.Payload.RunID, "--json")
	var openView struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Payload       struct {
			RunID            string   `json:"run_id"`
			RuntimeDir       string   `json:"runtime_dir"`
			BriefPath        string   `json:"brief_path"`
			ContextPath      string   `json:"context_path"`
			CodexLaunchPath  string   `json:"codex_launch_path"`
			ClaudeLaunchPath string   `json:"claude_launch_path"`
			Created          []string `json:"created"`
			Updated          []string `json:"updated"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(openOut), &openView); err != nil {
		t.Fatalf("parse open output: %v\nraw=%s", err, openOut)
	}
	if openView.FormatVersion != jsonFormatVersion || openView.Kind != "run_launch_manifest" {
		t.Fatalf("unexpected open envelope: %#v", openView)
	}
	if openView.Payload.RunID != dispatch.Payload.RunID || openView.Payload.BriefPath == "" || openView.Payload.ContextPath == "" {
		t.Fatalf("unexpected open payload: %#v", openView)
	}
	if len(openView.Payload.Created) != 0 || len(openView.Payload.Updated) != 0 {
		t.Fatalf("run open should not claim file mutations: %#v", openView.Payload)
	}
	if _, err := os.Stat(openView.Payload.RuntimeDir); err != nil {
		t.Fatalf("expected runtime dir placeholder to exist after dispatch: %v", err)
	}
	for _, path := range []string{
		openView.Payload.BriefPath,
		openView.Payload.ContextPath,
		openView.Payload.CodexLaunchPath,
		openView.Payload.ClaudeLaunchPath,
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("run open should not create runtime file %s, err=%v", path, err)
		}
	}

	launchOut := must("run", "launch", dispatch.Payload.RunID, "--actor", "human:owner", "--json")
	var launchView struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Payload       struct {
			RuntimeDir       string   `json:"runtime_dir"`
			BriefPath        string   `json:"brief_path"`
			ContextPath      string   `json:"context_path"`
			CodexLaunchPath  string   `json:"codex_launch_path"`
			ClaudeLaunchPath string   `json:"claude_launch_path"`
			Created          []string `json:"created"`
			Updated          []string `json:"updated"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(launchOut), &launchView); err != nil {
		t.Fatalf("parse launch output: %v\nraw=%s", err, launchOut)
	}
	if launchView.Kind != "run_launch_manifest" || len(launchView.Payload.Created) != 4 || len(launchView.Payload.Updated) != 0 {
		t.Fatalf("unexpected launch payload: %#v", launchView)
	}
	for _, path := range []string{
		launchView.Payload.BriefPath,
		launchView.Payload.ContextPath,
		launchView.Payload.CodexLaunchPath,
		launchView.Payload.ClaudeLaunchPath,
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected runtime file %s to exist: %v", path, err)
		}
	}

	secondOut := must("run", "launch", dispatch.Payload.RunID, "--actor", "human:owner", "--json")
	launchView = struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Payload       struct {
			RuntimeDir       string   `json:"runtime_dir"`
			BriefPath        string   `json:"brief_path"`
			ContextPath      string   `json:"context_path"`
			CodexLaunchPath  string   `json:"codex_launch_path"`
			ClaudeLaunchPath string   `json:"claude_launch_path"`
			Created          []string `json:"created"`
			Updated          []string `json:"updated"`
		} `json:"payload"`
	}{}
	if err := json.Unmarshal([]byte(secondOut), &launchView); err != nil {
		t.Fatalf("parse second launch output: %v\nraw=%s", err, secondOut)
	}
	if len(launchView.Payload.Created) != 0 || len(launchView.Payload.Updated) != 0 {
		t.Fatalf("expected idempotent launch without refresh, got %#v", launchView.Payload)
	}

	if err := os.WriteFile(filepath.Join(launchView.Payload.RuntimeDir, "brief.md"), []byte("stale\n"), 0o644); err != nil {
		t.Fatalf("overwrite brief: %v", err)
	}
	refreshOut := must("run", "launch", dispatch.Payload.RunID, "--refresh", "--actor", "human:owner", "--json")
	launchView = struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Payload       struct {
			RuntimeDir       string   `json:"runtime_dir"`
			BriefPath        string   `json:"brief_path"`
			ContextPath      string   `json:"context_path"`
			CodexLaunchPath  string   `json:"codex_launch_path"`
			ClaudeLaunchPath string   `json:"claude_launch_path"`
			Created          []string `json:"created"`
			Updated          []string `json:"updated"`
		} `json:"payload"`
	}{}
	if err := json.Unmarshal([]byte(refreshOut), &launchView); err != nil {
		t.Fatalf("parse refresh output: %v\nraw=%s", err, refreshOut)
	}
	if len(launchView.Payload.Updated) == 0 {
		t.Fatalf("expected refresh to report updated files, got %#v", launchView.Payload)
	}

	contextBody, err := os.ReadFile(launchView.Payload.ContextPath)
	if err != nil {
		t.Fatalf("read context json: %v", err)
	}
	if !json.Valid(contextBody) {
		t.Fatalf("expected valid context json: %s", string(contextBody))
	}
}
