package setup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
)

type versionRunner struct {
	stdout string
}

func (v versionRunner) Run(context.Context, adapter.Command) (adapter.CommandResult, error) {
	return adapter.CommandResult{Stdout: []byte(v.stdout)}, nil
}

func TestInspectReportsNativeAndLegacySkillsSeparately(t *testing.T) {
	engine := testEngine(t)
	native := filepath.Join(engine.WorkspaceRoot, filepath.FromSlash(integrations.AgentsRootSkillDir), "SKILL.md")
	legacy := filepath.Join(engine.WorkspaceRoot, filepath.FromSlash(integrations.CodexLegacySkillDir), "SKILL.md")
	for _, path := range []string{native, legacy} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(mustSkill(t)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	inspection, err := inspectWorkspace(engine.WorkspaceRoot, engine.Home, engine.StateDir, engine.LookPath, os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	var nativeHit, legacyHit bool
	for _, item := range inspection.SkillSightings {
		if item.Target != integrations.TargetCodex || !item.Present {
			continue
		}
		switch item.Kind {
		case "native":
			nativeHit = item.AtlasManaged
		case "legacy":
			legacyHit = item.AtlasManaged
		}
	}
	if !nativeHit || !legacyHit {
		t.Fatalf("expected native and legacy Codex skill sightings: %+v", inspection.SkillSightings)
	}
}

func TestSetupGrokArgvAgreesWithInitPortableFlag(t *testing.T) {
	engine := testEngine(t)
	exe := filepath.Join(t.TempDir(), "grok")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	engine.LookPath = func(name string) (string, error) {
		if name == "grok" {
			return exe, nil
		}
		return "", os.ErrNotExist
	}
	engine.ClientRunner = versionRunner{stdout: "1.0.30"}
	prepared, err := engine.Plan(PlanOptions{Agents: []integrations.Target{integrations.TargetGrok}})
	if err != nil {
		t.Fatal(err)
	}
	var grok adapter.IntegrationPlan
	for _, item := range prepared.Providers {
		if item.Target == integrations.TargetGrok {
			grok = item.Plan
		}
	}
	if grok.Registration == nil {
		t.Fatal("expected Grok registration")
	}
	joined := strings.Join(grok.Registration.Args, " ")
	if !strings.Contains(joined, "--tool-profile") || !strings.Contains(joined, "workflow") {
		t.Fatalf("setup grok argv missing workflow profile: %v", grok.Registration.Args)
	}
	if !strings.Contains(joined, "--tool-name-style") || !strings.Contains(joined, "portable") {
		t.Fatalf("setup grok argv missing portable names: %v", grok.Registration.Args)
	}
	if strings.Contains(joined, "--global") {
		t.Fatalf("project setup must not use init's --global argv: %v", grok.Registration.Args)
	}
	if grok.ResultingState.Verified() {
		t.Fatalf("setup must not claim live verified from a plan, got %s", grok.ResultingState)
	}
}

func TestSetupCodexKeepsWorkspaceTrust(t *testing.T) {
	engine := testEngine(t)
	prepared, err := engine.Plan(PlanOptions{Agents: []integrations.Target{integrations.TargetCodex}})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range prepared.Providers {
		if item.Target != integrations.TargetCodex {
			continue
		}
		if item.ResultingState != adapter.StatePendingWorkspaceTrust && item.Plan.ResultingState != adapter.StatePendingWorkspaceTrust {
			if item.Plan.ResultingState == adapter.StateConfiguredUnverified {
				return
			}
			t.Fatalf("codex must not skip workspace trust, got %s", item.Plan.ResultingState)
		}
		if item.Plan.ResultingState.Verified() {
			t.Fatal("codex plan claimed verified without a live probe")
		}
	}
}

func mustSkill(t *testing.T) string {
	t.Helper()
	installer := integrations.Installer{Root: t.TempDir()}
	files, err := installer.Preview(integrations.TargetCodex)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.Kind == "skill" && filepath.Base(file.Path) == "SKILL.md" {
			return file.Body
		}
	}
	t.Fatal("missing skill preview")
	return ""
}
