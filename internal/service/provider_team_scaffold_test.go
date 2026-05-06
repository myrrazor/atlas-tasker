package service

import (
	"context"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestCodeownersAndProviderRulesUseProjectPolicy(t *testing.T) {
	ctx := context.Background()
	_, actions, queries, projectStore, _, _ := newImportExportHarness(t)
	now := actions.now()

	project := contracts.Project{
		Key:           "APP",
		Name:          "App",
		CreatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
		Defaults: contracts.ProjectDefaults{
			ProviderTeams: []contracts.ProviderTeamMapping{
				{Alias: "core", Provider: contracts.ChangeProviderGitHub, Handle: "org/core"},
			},
			CodeownersRules: []contracts.CodeownersRule{
				{Pattern: "src/**", Collaborators: []string{"alice"}, Teams: []string{"core"}},
			},
			ProviderRules: []contracts.ProviderRule{
				{Name: "backend", Paths: []string{"src/**"}, Collaborators: []string{"alice"}, Teams: []string{"core"}, RequiredApprovals: 2},
			},
		},
	}
	if err := projectStore.CreateProject(ctx, project); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := actions.AddCollaborator(ctx, contracts.CollaboratorProfile{
		CollaboratorID: "alice",
		DisplayName:    "Alice",
		TrustState:     contracts.CollaboratorTrustStateTrusted,
		ProviderHandles: map[string]string{
			"github": "alice",
		},
	}, contracts.Actor("human:owner"), "seed collaborator"); err != nil {
		t.Fatalf("add collaborator: %v", err)
	}

	codeowners, err := queries.CodeownersPreview(ctx, "APP")
	if err != nil {
		t.Fatalf("codeowners preview: %v", err)
	}
	if !strings.Contains(codeowners.Content, "src/** @alice @org/core") {
		t.Fatalf("expected explicit CODEOWNERS entry, got %q", codeowners.Content)
	}

	rules, err := queries.ProviderRulesPreview(ctx, "APP")
	if err != nil {
		t.Fatalf("provider rules preview: %v", err)
	}
	if len(rules.Rules) != 1 {
		t.Fatalf("expected one provider rule, got %#v", rules.Rules)
	}
	if rules.Rules[0].Name != "backend" || rules.Rules[0].RequiredApprovals != 2 {
		t.Fatalf("unexpected provider rule preview: %#v", rules.Rules[0])
	}
	if got := strings.Join(rules.Rules[0].Reviewers, ","); got != "@alice,@org/core" && got != "@org/core,@alice" {
		t.Fatalf("unexpected provider reviewers: %#v", rules.Rules[0].Reviewers)
	}
}
