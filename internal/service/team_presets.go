package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

// Team presets bundle the primitives a working agent team needs -- profiles,
// a runbook, separation-of-duties permissions, and the right completion mode
// -- so "give me a builder and a reviewer" is one command instead of six.
// They compose existing stores only; nothing here has its own persistence.

type TeamPreset struct {
	Name        string                        `json:"name"`
	DisplayName string                        `json:"display_name"`
	Summary     string                        `json:"summary"`
	Completion  contracts.CompletionMode      `json:"completion_mode"`
	Agents      []contracts.AgentProfile      `json:"agents"`
	Runbooks    []contracts.Runbook           `json:"runbooks,omitempty"`
	Profiles    []contracts.PermissionProfile `json:"permission_profiles,omitempty"`
	NextSteps   []string                      `json:"next_steps"`
}

type TeamApplyResult struct {
	Preset         string   `json:"preset"`
	DryRun         bool     `json:"dry_run"`
	Created        []string `json:"created"`
	Skipped        []string `json:"skipped"`
	CompletionMode string   `json:"completion_mode"`
	NextSteps      []string `json:"next_steps"`
}

func TeamPresets(provider string) ([]TeamPreset, error) {
	switch strings.TrimSpace(provider) {
	case "", "claude", "codex", "mixed":
	default:
		return nil, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid provider %q: use claude, codex, or mixed", provider))
	}
	return []TeamPreset{
		soloPreset(provider),
		pairPreset(provider),
		swarmPreset(provider),
		crossfirePreset(provider),
	}, nil
}

func TeamPresetByName(name string, provider string) (TeamPreset, error) {
	presets, err := TeamPresets(provider)
	if err != nil {
		return TeamPreset{}, err
	}
	needle := strings.ToLower(strings.TrimSpace(name))
	for _, preset := range presets {
		if preset.Name == needle {
			return preset, nil
		}
	}
	return TeamPreset{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("unknown team preset %q: run 'tracker team list'", name))
}

func (s *ActionService) ApplyTeamPreset(ctx context.Context, name string, provider string, dryRun bool, actor contracts.Actor, reason string) (TeamApplyResult, error) {
	preset, err := TeamPresetByName(name, provider)
	if err != nil {
		return TeamApplyResult{}, err
	}
	if !actor.IsValid() {
		return TeamApplyResult{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
	}
	result := TeamApplyResult{
		Preset:         preset.Name,
		DryRun:         dryRun,
		CompletionMode: string(preset.Completion),
		NextSteps:      preset.NextSteps,
	}
	for _, agent := range preset.Agents {
		label := "agent " + agent.AgentID
		if _, err := s.Agents.LoadAgent(ctx, agent.AgentID); err == nil {
			result.Skipped = append(result.Skipped, label+" (already exists)")
			continue
		}
		result.Created = append(result.Created, label)
		if dryRun {
			continue
		}
		if _, err := s.SaveAgentProfile(ctx, agent, actor, reason); err != nil {
			return result, err
		}
	}
	for _, runbook := range preset.Runbooks {
		label := "runbook " + runbook.Name
		if _, err := (RunbookStore{Root: s.Root}).LoadRunbook(ctx, runbook.Name); err == nil {
			result.Skipped = append(result.Skipped, label+" (already exists)")
			continue
		}
		result.Created = append(result.Created, label)
		if dryRun {
			continue
		}
		if err := runbook.Validate(); err != nil {
			return result, err
		}
		if err := (RunbookStore{Root: s.Root}).SaveRunbook(ctx, runbook); err != nil {
			return result, err
		}
	}
	for _, profile := range preset.Profiles {
		label := "permission profile " + profile.ProfileID
		if existing, err := (PermissionProfileStore{Root: s.Root}).ListPermissionProfiles(ctx); err == nil {
			found := false
			for _, p := range existing {
				if p.ProfileID == profile.ProfileID {
					found = true
					break
				}
			}
			if found {
				result.Skipped = append(result.Skipped, label+" (already exists)")
				continue
			}
		}
		result.Created = append(result.Created, label)
		if dryRun {
			continue
		}
		if _, err := s.SavePermissionProfile(ctx, profile, actor, reason); err != nil {
			return result, err
		}
	}
	cfg, err := config.Load(s.Root)
	if err != nil {
		return result, err
	}
	if cfg.Workflow.CompletionMode != preset.Completion {
		result.Created = append(result.Created, "completion_mode "+string(preset.Completion))
		if !dryRun {
			cfg.Workflow.CompletionMode = preset.Completion
			if err := config.Save(s.Root, cfg); err != nil {
				return result, err
			}
		}
	} else {
		result.Skipped = append(result.Skipped, "completion_mode "+string(preset.Completion)+" (already set)")
	}
	return result, nil
}

func resolvePresetProvider(flag string, fallback contracts.AgentProvider) contracts.AgentProvider {
	switch strings.TrimSpace(flag) {
	case "claude":
		return contracts.AgentProviderClaude
	case "codex":
		return contracts.AgentProviderCodex
	default:
		return fallback
	}
}

func presetAgent(id string, display string, provider contracts.AgentProvider, roles []contracts.AgentRole, runbook string, weight int, maxRuns int) contracts.AgentProfile {
	return contracts.AgentProfile{
		AgentID:        id,
		DisplayName:    display,
		Provider:       provider,
		Enabled:        true,
		Capabilities:   []string{"code"},
		DefaultRunbook: runbook,
		MaxActiveRuns:  maxRuns,
		PreferredRoles: roles,
		RoutingWeight:  weight,
		Notes:          "created by tracker team apply",
	}
}

func standardBuildRunbook() contracts.Runbook {
	return contracts.Runbook{
		Name:                 "standard-build",
		DisplayName:          "Standard Build",
		AppliesToTicketTypes: []contracts.TicketType{contracts.TicketTypeTask, contracts.TicketTypeBug},
		DefaultInitialStage:  "implement",
		Stages: []contracts.RunbookStage{
			{
				Key:                   "implement",
				DisplayName:           "Implement",
				ExpectedRole:          contracts.AgentRoleWorker,
				RequiredEvidenceTypes: []contracts.EvidenceType{contracts.EvidenceTypeTestResult},
				SuggestedTicketStatus: contracts.StatusInProgress,
				CompletionCriteria:    []string{"tests pass locally", "evidence recorded on the run"},
			},
			{
				Key:                   "review",
				DisplayName:           "Review",
				ExpectedRole:          contracts.AgentRoleReviewer,
				RequiredGates:         []contracts.GateKind{contracts.GateKindReview},
				SuggestedTicketStatus: contracts.StatusInReview,
				SeparateRunRequired:   true,
			},
		},
	}
}

func builderDutiesProfile(builders ...string) contracts.PermissionProfile {
	return contracts.PermissionProfile{
		ProfileID:     "builder-duties",
		DisplayName:   "Builders implement, reviewers approve",
		Agents:        builders,
		DenyActions:   []contracts.PermissionAction{contracts.PermissionActionGateApprove, contracts.PermissionActionTicketComplete},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
}

func presetNextSteps(provider string) []string {
	install := "tracker integrations install claude (or codex)"
	switch strings.TrimSpace(provider) {
	case "codex":
		install = "tracker integrations install codex"
	case "claude":
		install = "tracker integrations install claude"
	}
	return []string{
		"create tickets and assign them: tracker ticket assign <ID> agent:builder-1 --actor human:owner --reason \"agent work\"",
		"install agent skills: " + install,
		"optional auto-launch on unblock: tracker agent auto set builder-1 --mode command --argv <runner> --argv \"{ticket_id}\" --actor human:owner --reason \"auto pickup\"",
	}
}

func soloPreset(provider string) TeamPreset {
	p := resolvePresetProvider(provider, contracts.AgentProviderClaude)
	return TeamPreset{
		Name:        "solo",
		DisplayName: "Solo Builder",
		Summary:     "One agent works the whole board; completions stay open so it can finish its own tickets.",
		Completion:  contracts.CompletionModeOpen,
		Agents: []contracts.AgentProfile{
			presetAgent("builder-1", "Builder One", p, []contracts.AgentRole{contracts.AgentRoleWorker}, "", 1, 2),
		},
		NextSteps: presetNextSteps(provider),
	}
}

func pairPreset(provider string) TeamPreset {
	builder := resolvePresetProvider(provider, contracts.AgentProviderClaude)
	reviewer := builder
	if strings.TrimSpace(provider) == "mixed" {
		builder = contracts.AgentProviderCodex
		reviewer = contracts.AgentProviderClaude
	}
	return TeamPreset{
		Name:        "pair",
		DisplayName: "Builder + Reviewer",
		Summary:     "A builder implements, a reviewer approves; review gate enforced, builders cannot approve their own work.",
		Completion:  contracts.CompletionModeReviewGate,
		Agents: []contracts.AgentProfile{
			presetAgent("builder-1", "Builder One", builder, []contracts.AgentRole{contracts.AgentRoleWorker}, "standard-build", 2, 2),
			presetAgent("reviewer-1", "Reviewer One", reviewer, []contracts.AgentRole{contracts.AgentRoleReviewer}, "standard-build", 1, 2),
		},
		Runbooks:  []contracts.Runbook{standardBuildRunbook()},
		Profiles:  []contracts.PermissionProfile{builderDutiesProfile("builder-1")},
		NextSteps: presetNextSteps(provider),
	}
}

func swarmPreset(provider string) TeamPreset {
	base := resolvePresetProvider(provider, contracts.AgentProviderClaude)
	builders := []contracts.AgentProvider{base, base, base}
	if strings.TrimSpace(provider) == "mixed" {
		builders = []contracts.AgentProvider{contracts.AgentProviderCodex, contracts.AgentProviderClaude, contracts.AgentProviderCodex}
	}
	return TeamPreset{
		Name:        "swarm",
		DisplayName: "Builder Swarm + QA",
		Summary:     "Three builders pull from the queue by routing weight, a QA gate guards review, an owner delegate unblocks policy questions.",
		Completion:  contracts.CompletionModeReviewGate,
		Agents: []contracts.AgentProfile{
			presetAgent("builder-1", "Builder One", builders[0], []contracts.AgentRole{contracts.AgentRoleWorker}, "standard-build", 3, 1),
			presetAgent("builder-2", "Builder Two", builders[1], []contracts.AgentRole{contracts.AgentRoleWorker}, "standard-build", 2, 1),
			presetAgent("builder-3", "Builder Three", builders[2], []contracts.AgentRole{contracts.AgentRoleWorker}, "standard-build", 1, 1),
			presetAgent("qa-1", "QA One", contracts.AgentProviderHuman, []contracts.AgentRole{contracts.AgentRoleQA, contracts.AgentRoleReviewer}, "standard-build", 1, 2),
			presetAgent("owner-delegate-1", "Owner Delegate", contracts.AgentProviderHuman, []contracts.AgentRole{contracts.AgentRoleOwnerDelegate}, "", 1, 1),
		},
		Runbooks:  []contracts.Runbook{standardBuildRunbook()},
		Profiles:  []contracts.PermissionProfile{builderDutiesProfile("builder-1", "builder-2", "builder-3")},
		NextSteps: presetNextSteps(provider),
	}
}

func crossfirePreset(provider string) TeamPreset {
	// the headline act: one vendor builds, the other reviews. --provider
	// claude flips who holds the hammer.
	builder := contracts.AgentProviderCodex
	reviewer := contracts.AgentProviderClaude
	if strings.TrimSpace(provider) == "claude" {
		builder, reviewer = contracts.AgentProviderClaude, contracts.AgentProviderCodex
	}
	return TeamPreset{
		Name:        "crossfire",
		DisplayName: "Crossfire (cross-vendor review)",
		Summary:     "Codex builds, Claude reviews (or flipped) -- two different models keep each other honest across the review gate.",
		Completion:  contracts.CompletionModeReviewGate,
		Agents: []contracts.AgentProfile{
			presetAgent("builder-1", "Builder One", builder, []contracts.AgentRole{contracts.AgentRoleWorker}, "standard-build", 2, 2),
			presetAgent("reviewer-1", "Reviewer One", reviewer, []contracts.AgentRole{contracts.AgentRoleReviewer}, "standard-build", 1, 2),
		},
		Runbooks:  []contracts.Runbook{standardBuildRunbook()},
		Profiles:  []contracts.PermissionProfile{builderDutiesProfile("builder-1")},
		NextSteps: presetNextSteps(provider),
	}
}
