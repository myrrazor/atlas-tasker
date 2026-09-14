package grok

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestGrokProjectUsesBareTrackerWhenPATHMatches(t *testing.T) {
	bin := t.TempDir()
	tracker := filepath.Join(bin, "tracker")
	if err := os.WriteFile(tracker, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	input, _ := grokPlanInput(t, tracker, "")
	plan, err := New().WithStateDir(t.TempDir()).Plan(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Registration == nil {
		t.Fatal("missing grok registration")
	}
	if plan.Registration.Command != adapter.PortableExecutableName || !plan.Registration.Portable {
		t.Fatalf("expected bare tracker, got command=%q portable=%v", plan.Registration.Command, plan.Registration.Portable)
	}
	if plan.Registration.Binding.Kind != adapter.WorkspaceBindingVerifiedCwd {
		t.Fatalf("binding %q", plan.Registration.Binding.Kind)
	}
	joined := strings.Join(plan.Registration.Args, " ")
	if !strings.Contains(joined, "--workspace-from-cwd") || !strings.Contains(joined, adapter.FlagToolNameStyle) {
		t.Fatalf("args %v", plan.Registration.Args)
	}
	for _, step := range plan.Steps {
		if step.Kind == adapter.StepRunClientCommand && step.Command != nil && strings.Contains(strings.Join(step.Command.Args, " "), "mcp add") {
			if !containsArg(step.Command.Args, adapter.PortableExecutableName) {
				t.Fatalf("mcp add should launch bare tracker: %v", step.Command.Args)
			}
		}
	}
}

func TestGrokProjectKeepsAbsoluteTrackerWhenNotOnPATH(t *testing.T) {
	tracker := filepath.Join(t.TempDir(), "tracker")
	if err := os.WriteFile(tracker, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	input, _ := grokPlanInput(t, tracker, "")
	plan, err := New().WithStateDir(t.TempDir()).Plan(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Registration == nil {
		t.Fatal("missing grok registration")
	}
	if plan.Registration.Command != input.TrackerPath || plan.Registration.Portable {
		t.Fatalf("expected absolute tracker, got command=%q portable=%v", plan.Registration.Command, plan.Registration.Portable)
	}
	warned := false
	for _, warning := range plan.Warnings {
		if strings.Contains(warning, "PATH does not resolve") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("missing portability warning: %v", plan.Warnings)
	}
}

func TestGrokHomePathWithoutPATHLeavesGuidancePlan(t *testing.T) {
	home := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(home); err == nil {
		home = resolved
	}
	tracker := filepath.Join(home, "bin", "tracker")
	if err := os.MkdirAll(filepath.Dir(tracker), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tracker, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	input, _ := grokPlanInput(t, tracker, home)
	plan, err := New().WithStateDir(t.TempDir()).Plan(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Registration != nil {
		t.Fatalf("home-local tracker must not be written into project config: %+v", plan.Registration)
	}
	if plan.ResultingState != adapter.StateConfiguredUnverified {
		t.Fatalf("state %s", plan.ResultingState)
	}
	warned := false
	for _, warning := range plan.Warnings {
		if strings.Contains(warning, "Put this binary on PATH") && strings.Contains(warning, "rerun setup") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("missing PATH guidance: %v", plan.Warnings)
	}
	for _, step := range plan.Steps {
		if step.Kind == adapter.StepRunClientCommand && step.Command != nil && strings.Contains(strings.Join(step.Command.Args, " "), "mcp add") {
			t.Fatalf("must not register project MCP: %v", step.Command.Args)
		}
	}
	hasSkill := false
	for _, step := range plan.Steps {
		if step.Kind == adapter.StepWriteManagedFile {
			hasSkill = true
		}
	}
	if !hasSkill {
		t.Fatal("guidance plan should still refresh the Grok skill")
	}
}

func grokPlanInput(t *testing.T, tracker, home string) (adapter.PlanInput, string) {
	t.Helper()
	root := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	if err := os.MkdirAll(storage.TrackerDir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storage.TrackerDir(root), "workspace.json"), []byte(`{"workspace_id":"ws-adapter-test"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if home == "" {
		home = t.TempDir()
	}
	if resolved, err := filepath.EvalSymlinks(home); err == nil {
		home = resolved
	}
	if resolved, err := filepath.EvalSymlinks(tracker); err == nil {
		tracker = resolved
	}
	exe := filepath.Join(t.TempDir(), "grok")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return adapter.PlanInput{
		WorkspaceRoot: root,
		WorkspaceID:   "ws-adapter-test",
		Home:          home,
		TrackerPath:   tracker,
		ActorHint:     contracts.Actor("agent:grok"),
		Detection: adapter.Detection{
			Target:         integrations.TargetGrok,
			Installed:      true,
			ExecutablePath: exe,
			VersionSupport: adapter.VersionSupported,
			Version:        adapter.ClientVersion{Raw: "1.0.30", Major: 1, Minor: 0, Patch: 30, Known: true},
		},
	}, root
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
