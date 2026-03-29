package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func (s *QueryService) CodeownersPreview(ctx context.Context, projectKey string) (CodeownersPreviewView, error) {
	project, err := s.Projects.GetProject(ctx, projectKey)
	if err != nil {
		return CodeownersPreviewView{}, err
	}
	repoRoot, outputPath, err := scaffoldRepoRoot(ctx, s.Root)
	if err != nil {
		return CodeownersPreviewView{}, err
	}
	rules, warnings, err := s.resolveCodeownersRules(ctx, project)
	if err != nil {
		return CodeownersPreviewView{}, err
	}
	return CodeownersPreviewView{
		Project:     project.Key,
		RepoRoot:    repoRoot,
		Path:        outputPath,
		Content:     renderCodeowners(rules),
		Rules:       rules,
		Warnings:    warnings,
		GeneratedAt: s.now(),
	}, nil
}

func (s *ActionService) WriteCodeowners(ctx context.Context, projectKey string, actor contracts.Actor, reason string) (CodeownersPreviewView, error) {
	return withWriteLock(ctx, s.LockManager, "write codeowners scaffold", func(ctx context.Context) (CodeownersPreviewView, error) {
		if !actor.IsValid() {
			return CodeownersPreviewView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		view, err := NewQueryService(s.Root, s.Projects, s.Tickets, s.Events, s.Projection, s.Clock).CodeownersPreview(ctx, projectKey)
		if err != nil {
			return CodeownersPreviewView{}, err
		}
		if strings.TrimSpace(view.Path) == "" {
			return CodeownersPreviewView{}, apperr.New(apperr.CodeConflict, "codeowners write requires a git repository root")
		}
		eventPayload := map[string]any{
			"project":  projectKey,
			"path":     view.Path,
			"content":  view.Content,
			"rules":    view.Rules,
			"warnings": view.Warnings,
		}
		event, err := s.newEvent(ctx, projectKey, s.now(), actor, reason, contracts.EventCodeownersWritten, "", eventPayload)
		if err != nil {
			return CodeownersPreviewView{}, err
		}
		if err := s.commitMutation(ctx, "write codeowners scaffold", "project_policy", event, func(ctx context.Context) error {
			return os.WriteFile(view.Path, []byte(view.Content), 0o644)
		}); err != nil {
			return CodeownersPreviewView{}, err
		}
		view.GeneratedAt = s.now()
		return view, nil
	})
}

func (s *QueryService) ProviderRulesPreview(ctx context.Context, projectKey string) (ProviderRulesPreviewView, error) {
	project, err := s.Projects.GetProject(ctx, projectKey)
	if err != nil {
		return ProviderRulesPreviewView{}, err
	}
	rules, warnings, err := s.resolveProviderRules(ctx, project)
	if err != nil {
		return ProviderRulesPreviewView{}, err
	}
	return ProviderRulesPreviewView{Project: project.Key, Rules: rules, Warnings: warnings, GeneratedAt: s.now()}, nil
}

func scaffoldRepoRoot(ctx context.Context, root string) (string, string, error) {
	repo, err := (SCMService{Root: root}).RepoStatus(ctx)
	if err != nil {
		return "", "", err
	}
	if !repo.Present || strings.TrimSpace(repo.Root) == "" {
		return "", "", nil
	}
	return repo.Root, filepath.Join(repo.Root, "CODEOWNERS"), nil
}

func (s *QueryService) resolveCodeownersRules(ctx context.Context, project contracts.Project) ([]CodeownersRulePreview, []string, error) {
	rules := make([]CodeownersRulePreview, 0)
	warnings := make([]string, 0)
	if len(project.Defaults.CodeownersRules) == 0 {
		owners, ownerWarnings, err := s.derivedProjectOwners(ctx, project.Key)
		if err != nil {
			return nil, nil, err
		}
		warnings = append(warnings, ownerWarnings...)
		if len(owners) == 0 {
			warnings = append(warnings, "no project collaborators with github handles found for CODEOWNERS")
			return rules, dedupeStrings(warnings), nil
		}
		rules = append(rules, CodeownersRulePreview{Pattern: "*", Owners: owners})
		return rules, dedupeStrings(warnings), nil
	}

	seenPatterns := map[string]struct{}{}
	for _, rule := range project.Defaults.CodeownersRules {
		if _, ok := seenPatterns[rule.Pattern]; ok {
			warnings = append(warnings, "duplicate_codeowners_pattern:"+rule.Pattern)
		}
		seenPatterns[rule.Pattern] = struct{}{}
		owners, ownerWarnings, err := s.resolveRuleOwners(ctx, project, rule.Collaborators, rule.Teams)
		if err != nil {
			return nil, nil, err
		}
		warnings = append(warnings, ownerWarnings...)
		if len(owners) == 0 {
			warnings = append(warnings, "codeowners_rule_without_owners:"+rule.Pattern)
			continue
		}
		rules = append(rules, CodeownersRulePreview{Pattern: rule.Pattern, Owners: owners})
	}
	if _, ok := seenPatterns["*"]; ok && len(seenPatterns) > 1 {
		warnings = append(warnings, "wildcard_codeowners_rule_overlaps_specific_paths")
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].Pattern < rules[j].Pattern })
	return rules, dedupeStrings(warnings), nil
}

func (s *QueryService) resolveProviderRules(ctx context.Context, project contracts.Project) ([]ProviderRulePreview, []string, error) {
	warnings := make([]string, 0)
	rules := make([]ProviderRulePreview, 0)
	if len(project.Defaults.ProviderRules) == 0 {
		codeowners, codeownerWarnings, err := s.resolveCodeownersRules(ctx, project)
		if err != nil {
			return nil, nil, err
		}
		warnings = append(warnings, codeownerWarnings...)
		for _, rule := range codeowners {
			rules = append(rules, ProviderRulePreview{
				Name:              "codeowners:" + rule.Pattern,
				Paths:             []string{rule.Pattern},
				Reviewers:         append([]string{}, rule.Owners...),
				RequiredApprovals: 1,
			})
		}
		return rules, dedupeStrings(warnings), nil
	}
	for _, rule := range project.Defaults.ProviderRules {
		reviewers, ruleWarnings, err := s.resolveRuleOwners(ctx, project, rule.Collaborators, rule.Teams)
		if err != nil {
			return nil, nil, err
		}
		warnings = append(warnings, ruleWarnings...)
		rules = append(rules, ProviderRulePreview{
			Name:              rule.Name,
			Paths:             append([]string{}, rule.Paths...),
			Reviewers:         reviewers,
			RequiredApprovals: maxInt(rule.RequiredApprovals, 1),
		})
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].Name < rules[j].Name })
	return rules, dedupeStrings(warnings), nil
}

func (s *QueryService) resolveRuleOwners(ctx context.Context, project contracts.Project, collaboratorIDs []string, teamAliases []string) ([]string, []string, error) {
	owners := make([]string, 0, len(collaboratorIDs)+len(teamAliases))
	warnings := make([]string, 0)
	for _, collaboratorID := range collaboratorIDs {
		collaborator, err := s.Collaborators.LoadCollaborator(ctx, collaboratorID)
		if err != nil {
			warnings = append(warnings, "unknown_collaborator:"+collaboratorID)
			continue
		}
		handle := normalizeProviderOwner(collaborator.ProviderHandles[string(contracts.ChangeProviderGitHub)])
		if handle == "" {
			warnings = append(warnings, "missing_github_handle:"+collaboratorID)
			continue
		}
		owners = append(owners, handle)
	}
	for _, alias := range teamAliases {
		handle := ""
		for _, team := range project.Defaults.ProviderTeams {
			if team.Alias == alias && team.Provider == contracts.ChangeProviderGitHub {
				handle = normalizeProviderOwner(team.Handle)
				break
			}
		}
		if handle == "" {
			warnings = append(warnings, "unknown_provider_team:"+alias)
			continue
		}
		owners = append(owners, handle)
	}
	return dedupeStrings(owners), warnings, nil
}

func (s *QueryService) derivedProjectOwners(ctx context.Context, projectKey string) ([]string, []string, error) {
	collaborators, err := s.Collaborators.ListCollaborators(ctx)
	if err != nil {
		return nil, nil, err
	}
	owners := make([]string, 0)
	warnings := make([]string, 0)
	for _, collaborator := range collaborators {
		if collaborator.Status != contracts.CollaboratorStatusActive {
			continue
		}
		bindings, err := s.Memberships.ListMemberships(ctx, collaborator.CollaboratorID)
		if err != nil {
			return nil, nil, err
		}
		include := false
		for _, membership := range activeMembershipsForProject(bindings, projectKey) {
			switch membership.Role {
			case contracts.MembershipRoleReviewer, contracts.MembershipRoleMaintainer, contracts.MembershipRoleOwner:
				include = true
			}
		}
		if !include {
			continue
		}
		handle := normalizeProviderOwner(collaborator.ProviderHandles[string(contracts.ChangeProviderGitHub)])
		if handle == "" {
			warnings = append(warnings, "missing_github_handle:"+collaborator.CollaboratorID)
			continue
		}
		owners = append(owners, handle)
	}
	return dedupeStrings(owners), warnings, nil
}

func renderCodeowners(rules []CodeownersRulePreview) string {
	lines := []string{
		"# generated by tracker project codeowners render",
		"# review before writing",
	}
	if len(rules) == 0 {
		lines = append(lines, "", "# no CODEOWNERS entries configured")
		return strings.Join(lines, "\n") + "\n"
	}
	lines = append(lines, "")
	for _, rule := range rules {
		lines = append(lines, rule.Pattern+" "+strings.Join(rule.Owners, " "))
	}
	return strings.Join(lines, "\n") + "\n"
}

func normalizeProviderOwner(handle string) string {
	handle = strings.TrimSpace(handle)
	if handle == "" {
		return ""
	}
	if strings.HasPrefix(handle, "@") {
		return handle
	}
	return "@" + handle
}

func maxInt(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}
