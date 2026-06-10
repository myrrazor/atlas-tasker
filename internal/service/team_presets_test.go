package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

func setupTeamPresetTest(t *testing.T) (string, context.Context, *ActionService) {
	t.Helper()
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	projects := mdstore.ProjectStore{RootDir: root}
	tickets := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	events := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), tickets, events)
	if err != nil {
		t.Fatalf("open projection: %v", err)
	}
	t.Cleanup(func() { _ = projection.Close() })
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	actions := NewActionService(root, projects, tickets, events, projection, func() time.Time { return now }, FileLockManager{Root: root}, nil, nil)
	return root, ctx, actions
}

func TestTeamPresetsListedForCLI(t *testing.T) {
	presets, err := TeamPresets("")
	if err != nil {
		t.Fatalf("list presets: %v", err)
	}
	names := make([]string, 0, len(presets))
	for _, p := range presets {
		names = append(names, p.Name)
	}
	want := []string{"solo", "pair", "swarm", "crossfire"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected preset roster: %v", names)
	}
	for _, p := range presets {
		if strings.TrimSpace(p.Summary) == "" || len(p.Agents) == 0 {
			t.Fatalf("preset %s missing summary or agents", p.Name)
		}
	}
}

func TestApplyTeamPresetPairCreatesRoster(t *testing.T) {
	root, ctx, actions := setupTeamPresetTest(t)
	result, err := actions.ApplyTeamPreset(ctx, "pair", "", false, contracts.Actor("human:owner"), "team setup")
	if err != nil {
		t.Fatalf("apply pair: %v", err)
	}
	if result.DryRun || len(result.Created) == 0 || len(result.NextSteps) == 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	builder, err := actions.Agents.LoadAgent(ctx, "builder-1")
	if err != nil {
		t.Fatalf("builder-1 missing: %v", err)
	}
	if !builder.Enabled {
		t.Fatal("builder should be enabled")
	}
	reviewer, err := actions.Agents.LoadAgent(ctx, "reviewer-1")
	if err != nil {
		t.Fatalf("reviewer-1 missing: %v", err)
	}
	hasReviewerRole := false
	for _, role := range reviewer.PreferredRoles {
		if role == contracts.AgentRoleReviewer {
			hasReviewerRole = true
		}
	}
	if !hasReviewerRole {
		t.Fatalf("reviewer-1 should prefer the reviewer role, got %#v", reviewer.PreferredRoles)
	}
	if _, err := (RunbookStore{Root: root}).LoadRunbook(ctx, "standard-build"); err != nil {
		t.Fatalf("standard-build runbook missing: %v", err)
	}
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Workflow.CompletionMode != contracts.CompletionModeReviewGate {
		t.Fatalf("expected review_gate completion, got %s", cfg.Workflow.CompletionMode)
	}
}

func TestApplyTeamPresetIsIdempotent(t *testing.T) {
	_, ctx, actions := setupTeamPresetTest(t)
	if _, err := actions.ApplyTeamPreset(ctx, "pair", "", false, contracts.Actor("human:owner"), "team setup"); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	second, err := actions.ApplyTeamPreset(ctx, "pair", "", false, contracts.Actor("human:owner"), "team setup again")
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if len(second.Created) != 0 {
		t.Fatalf("second apply should create nothing, got %v", second.Created)
	}
	if len(second.Skipped) == 0 {
		t.Fatal("second apply should report skips")
	}
}

func TestApplyTeamPresetDryRunMutatesNothing(t *testing.T) {
	root, ctx, actions := setupTeamPresetTest(t)
	result, err := actions.ApplyTeamPreset(ctx, "pair", "", true, contracts.Actor("human:owner"), "preview")
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if !result.DryRun || len(result.Created) == 0 {
		t.Fatalf("dry-run should preview creations, got %#v", result)
	}
	if _, err := actions.Agents.LoadAgent(ctx, "builder-1"); err == nil {
		t.Fatal("dry-run must not create agents")
	}
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Workflow.CompletionMode != contracts.CompletionModeOpen {
		t.Fatal("dry-run must not touch completion mode")
	}
}

func TestApplyTeamPresetCrossfireMixesProviders(t *testing.T) {
	_, ctx, actions := setupTeamPresetTest(t)
	if _, err := actions.ApplyTeamPreset(ctx, "crossfire", "", false, contracts.Actor("human:owner"), "team setup"); err != nil {
		t.Fatalf("apply crossfire: %v", err)
	}
	builder, err := actions.Agents.LoadAgent(ctx, "builder-1")
	if err != nil {
		t.Fatalf("builder-1 missing: %v", err)
	}
	reviewer, err := actions.Agents.LoadAgent(ctx, "reviewer-1")
	if err != nil {
		t.Fatalf("reviewer-1 missing: %v", err)
	}
	if builder.Provider != contracts.AgentProviderCodex || reviewer.Provider != contracts.AgentProviderClaude {
		t.Fatalf("crossfire default should be codex builder + claude reviewer, got %s/%s", builder.Provider, reviewer.Provider)
	}
}

func TestApplyTeamPresetCrossfireProviderFlip(t *testing.T) {
	_, ctx, actions := setupTeamPresetTest(t)
	if _, err := actions.ApplyTeamPreset(ctx, "crossfire", "claude", false, contracts.Actor("human:owner"), "team setup"); err != nil {
		t.Fatalf("apply crossfire claude: %v", err)
	}
	builder, _ := actions.Agents.LoadAgent(ctx, "builder-1")
	reviewer, _ := actions.Agents.LoadAgent(ctx, "reviewer-1")
	if builder.Provider != contracts.AgentProviderClaude || reviewer.Provider != contracts.AgentProviderCodex {
		t.Fatalf("crossfire --provider claude should flip the pair, got %s/%s", builder.Provider, reviewer.Provider)
	}
}

func TestApplyTeamPresetUnknownName(t *testing.T) {
	_, ctx, actions := setupTeamPresetTest(t)
	_, err := actions.ApplyTeamPreset(ctx, "galactic", "", false, contracts.Actor("human:owner"), "nope")
	if err == nil {
		t.Fatal("expected unknown preset to fail")
	}
	if apperr.CodeOf(err) != apperr.CodeInvalidInput {
		t.Fatalf("expected invalid_input, got %v", err)
	}
}
