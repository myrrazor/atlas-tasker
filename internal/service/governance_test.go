package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
	"github.com/pelletier/go-toml/v2"
)

func newGovernanceHarness(t *testing.T) (context.Context, *ActionService, contracts.TicketSnapshot) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	now := time.Date(2026, 5, 6, 18, 30, 0, 0, time.UTC)
	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = projection.Close() })
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket := contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Govern me",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusInReview,
		ReviewState:   contracts.ReviewStateApproved,
		Priority:      contracts.PriorityMedium,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	actions := NewActionService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now }, FileLockManager{Root: root}, nil, nil)
	if err := actions.AppendAndProject(ctx, contracts.Event{EventID: 1, Timestamp: now, Actor: contracts.Actor("agent:builder-1"), Type: contracts.EventTicketCreated, Project: "APP", TicketID: ticket.ID, Payload: ticket, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("append create event: %v", err)
	}
	return ctx, actions, ticket
}

func TestGovernanceQuorumEvaluatesActiveCollaboratorsAtActionTime(t *testing.T) {
	ctx, actions, ticket := newGovernanceHarness(t)
	for _, spec := range []struct {
		id    string
		actor contracts.Actor
	}{
		{"reviewer-1", contracts.Actor("agent:reviewer-1")},
		{"reviewer-2", contracts.Actor("agent:reviewer-2")},
	} {
		if _, err := actions.AddCollaborator(ctx, contracts.CollaboratorProfile{
			CollaboratorID: spec.id,
			Status:         contracts.CollaboratorStatusActive,
			TrustState:     contracts.CollaboratorTrustStateTrusted,
			AtlasActors:    []contracts.Actor{spec.actor},
		}, contracts.Actor("human:owner"), "add reviewer"); err != nil {
			t.Fatalf("add collaborator %s: %v", spec.id, err)
		}
		if _, err := actions.BindMembership(ctx, contracts.MembershipBinding{
			CollaboratorID: spec.id,
			ScopeKind:      contracts.MembershipScopeProject,
			ScopeID:        "APP",
			Role:           contracts.MembershipRoleReviewer,
			Status:         contracts.MembershipStatusActive,
		}, contracts.Actor("human:owner"), "bind reviewer"); err != nil {
			t.Fatalf("bind membership %s: %v", spec.id, err)
		}
	}
	policy := contracts.GovernancePolicy{
		PolicyID:         "release-quorum",
		Name:             "Release quorum",
		ScopeKind:        contracts.PolicyScopeProject,
		ScopeID:          "APP",
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionTicketComplete},
		QuorumRules: []contracts.QuorumRule{{
			RuleID:                       "reviewer-quorum",
			ActionKind:                   contracts.ProtectedActionTicketComplete,
			RequiredCount:                2,
			AllowedRoles:                 []contracts.MembershipRole{contracts.MembershipRoleReviewer},
			RequireDistinctCollaborators: true,
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := actions.GovernancePolicies.SaveGovernancePolicy(ctx, policy); err != nil {
		t.Fatalf("save governance policy: %v", err)
	}
	allowed, err := actions.ExplainGovernance(ctx, GovernanceEvaluationInput{
		Action:         contracts.ProtectedActionTicketComplete,
		Target:         "ticket:" + ticket.ID,
		Actor:          contracts.Actor("human:owner"),
		ApprovalActors: []contracts.Actor{contracts.Actor("agent:reviewer-1"), contracts.Actor("agent:reviewer-2")},
	})
	if err != nil {
		t.Fatalf("explain governance: %v", err)
	}
	if !allowed.Allowed {
		t.Fatalf("two active reviewer approvals should satisfy quorum: %#v", allowed)
	}
	if _, err := actions.SetCollaboratorStatus(ctx, "reviewer-2", contracts.CollaboratorStatusSuspended, contracts.Actor("human:owner"), "suspend reviewer"); err != nil {
		t.Fatalf("suspend reviewer: %v", err)
	}
	blocked, err := actions.ExplainGovernance(ctx, GovernanceEvaluationInput{
		Action:         contracts.ProtectedActionTicketComplete,
		Target:         "ticket:" + ticket.ID,
		Actor:          contracts.Actor("human:owner"),
		ApprovalActors: []contracts.Actor{contracts.Actor("agent:reviewer-1"), contracts.Actor("agent:reviewer-2")},
	})
	if err != nil {
		t.Fatalf("explain suspended governance: %v", err)
	}
	reasons := strings.Join(blocked.ReasonCodes, ",")
	if blocked.Allowed || !strings.Contains(reasons, "quorum_unsatisfied") || !strings.Contains(reasons, "quorum_approval_inactive") {
		t.Fatalf("suspended approver should not satisfy action-time quorum: %#v", blocked)
	}
}

func TestGovernanceSeparationAndSignatureBypassProtection(t *testing.T) {
	ctx, actions, ticket := newGovernanceHarness(t)
	signaturePolicy := contracts.GovernancePolicy{
		PolicyID:           "signed-bundle-import",
		Name:               "Signed bundle import",
		ScopeKind:          contracts.PolicyScopeWorkspace,
		ProtectedActions:   []contracts.ProtectedAction{contracts.ProtectedActionBundleImportApply},
		RequiredSignatures: 1,
		OverrideRules: []contracts.OverrideRule{{
			RuleID:        "owner-override",
			ActionKind:    contracts.ProtectedActionBundleImportApply,
			Allowed:       true,
			RequireReason: true,
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := actions.GovernancePolicies.SaveGovernancePolicy(ctx, signaturePolicy); err != nil {
		t.Fatalf("save signature governance policy: %v", err)
	}
	blocked, err := actions.ExplainGovernance(ctx, GovernanceEvaluationInput{
		Action: contracts.ProtectedActionBundleImportApply,
		Target: "workspace",
		Actor:  contracts.Actor("human:owner"),
		Reason: "override import",
	})
	if err != nil {
		t.Fatalf("explain signature governance: %v", err)
	}
	if blocked.Allowed || !strings.Contains(strings.Join(blocked.ReasonCodes, ","), "owner_override_cannot_bypass_signature") {
		t.Fatalf("owner override must not bypass missing trusted signatures: %#v", blocked)
	}

	separationPolicy := contracts.GovernancePolicy{
		PolicyID:         "release-separation",
		Name:             "Release separation",
		ScopeKind:        contracts.PolicyScopeProject,
		ScopeID:          "APP",
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionTicketComplete},
		SeparationOfDutiesRules: []contracts.SeparationOfDutiesRule{{
			RuleID:                      "creator-cannot-complete",
			ActionKind:                  contracts.ProtectedActionTicketComplete,
			ForbiddenActorRelationships: []string{"same_actor"},
			LookbackEventTypes:          []contracts.EventType{contracts.EventTicketCreated},
			LookbackScope:               "ticket",
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := actions.GovernancePolicies.SaveGovernancePolicy(ctx, separationPolicy); err != nil {
		t.Fatalf("save separation governance policy: %v", err)
	}
	separation, err := actions.ExplainGovernance(ctx, GovernanceEvaluationInput{
		Action: contracts.ProtectedActionTicketComplete,
		Target: "ticket:" + ticket.ID,
		Actor:  contracts.Actor("agent:builder-1"),
	})
	if err != nil {
		t.Fatalf("explain separation governance: %v", err)
	}
	if separation.Allowed || !strings.Contains(strings.Join(separation.ReasonCodes, ","), "separation_of_duties_violation") {
		t.Fatalf("same actor should violate separation: %#v", separation)
	}
}

func TestGovernanceSeparationHonorsNamedRelationships(t *testing.T) {
	ctx, actions, ticket := newGovernanceHarness(t)
	if err := actions.AppendAndProject(ctx, contracts.Event{
		EventID:       2,
		Timestamp:     time.Date(2026, 5, 6, 18, 31, 0, 0, time.UTC),
		Actor:         contracts.Actor("agent:builder-1"),
		Type:          contracts.EventRunCompleted,
		Project:       "APP",
		TicketID:      ticket.ID,
		Payload:       map[string]string{"run_id": "run_implemented"},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("append run completed event: %v", err)
	}
	if err := actions.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:         "implementer-cannot-merge",
		Name:             "Implementer cannot merge",
		ScopeKind:        contracts.PolicyScopeProject,
		ScopeID:          "APP",
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionChangeMerge},
		SeparationOfDutiesRules: []contracts.SeparationOfDutiesRule{{
			RuleID:                      "implemented-work",
			ActionKind:                  contracts.ProtectedActionChangeMerge,
			ForbiddenActorRelationships: []string{"implemented"},
			LookbackEventTypes:          []contracts.EventType{contracts.EventRunCompleted},
			LookbackScope:               "ticket",
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save named separation policy: %v", err)
	}
	explanation, err := actions.ExplainGovernance(ctx, GovernanceEvaluationInput{
		Action: contracts.ProtectedActionChangeMerge,
		Target: "ticket:" + ticket.ID,
		Actor:  contracts.Actor("agent:builder-1"),
	})
	if err != nil {
		t.Fatalf("explain named separation governance: %v", err)
	}
	if explanation.Allowed || !strings.Contains(strings.Join(explanation.ReasonCodes, ","), "separation_of_duties_violation:implemented-work") {
		t.Fatalf("implemented relationship should block the same root actor: %#v", explanation)
	}
}

func TestGovernanceSignedSyncBundleImportSatisfiesSignaturePolicy(t *testing.T) {
	ctx := context.Background()
	_, sourceActions, _, sourceProjects, _, _ := newImportExportHarness(t)
	seedSyncWorkspace(t, ctx, sourceActions, sourceProjects)
	key := generateTrustedSigningKey(t, ctx, sourceActions, "alice")
	secondKey := generateTrustedSigningKey(t, ctx, sourceActions, "bob")
	bundle, err := sourceActions.CreateSyncBundle(ctx, contracts.Actor("human:owner"), "create signed sync bundle")
	if err != nil {
		t.Fatalf("create sync bundle: %v", err)
	}
	if _, err := sourceActions.SignSyncPublication(ctx, bundle.Job.BundleRef, key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "sign publication"); err != nil {
		t.Fatalf("sign sync publication: %v", err)
	}
	publicationPath := strings.TrimSuffix(bundle.Job.BundleRef, ".tar.gz") + ".publication.json"
	duplicatedPublication, err := readSyncPublication(publicationPath)
	if err != nil {
		t.Fatalf("read signed publication: %v", err)
	}
	duplicatedPublication.SignatureEnvelopes = append(duplicatedPublication.SignatureEnvelopes, duplicatedPublication.SignatureEnvelopes[0])
	if err := writeSyncPublication(publicationPath, duplicatedPublication); err != nil {
		t.Fatalf("write duplicated publication signature: %v", err)
	}
	_, duplicateTarget, _, _, _, _ := newImportExportHarness(t)
	if err := duplicateTarget.SecurityKeys.SavePublicKey(ctx, key.PublicKey); err != nil {
		t.Fatalf("seed duplicate target key: %v", err)
	}
	if _, err := duplicateTarget.BindTrust(ctx, "alice", key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "trust signer"); err != nil {
		t.Fatalf("trust duplicate target signer: %v", err)
	}
	if err := duplicateTarget.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:           "signed-bundle-import",
		Name:               "Signed bundle import",
		ScopeKind:          contracts.PolicyScopeWorkspace,
		ProtectedActions:   []contracts.ProtectedAction{contracts.ProtectedActionBundleImportApply},
		RequiredSignatures: 2,
		SchemaVersion:      contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save duplicate target governance policy: %v", err)
	}
	if _, err := duplicateTarget.ImportSyncBundle(ctx, bundle.Job.BundleRef, contracts.Actor("human:owner"), "import duplicated signature bundle"); err == nil || !strings.Contains(err.Error(), "trusted_signature_required") {
		t.Fatalf("duplicated envelope should not satisfy multi-signature governance, got %v", err)
	}
	if _, err := sourceActions.SignSyncPublication(ctx, bundle.Job.BundleRef, secondKey.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "co-sign publication"); err != nil {
		t.Fatalf("co-sign sync publication: %v", err)
	}

	_, quorumTarget, _, _, _, _ := newImportExportHarness(t)
	for _, spec := range []struct {
		id  string
		key KeyDetailView
	}{
		{"alice", key},
		{"bob", secondKey},
	} {
		if err := quorumTarget.SecurityKeys.SavePublicKey(ctx, spec.key.PublicKey); err != nil {
			t.Fatalf("seed quorum target public key %s: %v", spec.id, err)
		}
		if _, err := quorumTarget.BindTrust(ctx, spec.id, spec.key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "trust quorum signer"); err != nil {
			t.Fatalf("trust quorum signer %s: %v", spec.id, err)
		}
	}
	if err := quorumTarget.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:         "signed-bundle-quorum",
		Name:             "Signed bundle quorum",
		ScopeKind:        contracts.PolicyScopeWorkspace,
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionBundleImportApply},
		QuorumRules: []contracts.QuorumRule{{
			RuleID:                   "two-trusted-signers",
			ActionKind:               contracts.ProtectedActionBundleImportApply,
			RequiredCount:            2,
			RequireTrustedSignatures: true,
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save signature-backed quorum policy: %v", err)
	}
	if _, err := quorumTarget.ImportSyncBundle(ctx, bundle.Job.BundleRef, contracts.Actor("human:owner"), "import trusted signed bundle with signature quorum"); err != nil {
		t.Fatalf("trusted signed bundle should satisfy signature-backed quorum: %v", err)
	}

	_, targetActions, _, _, _, _ := newImportExportHarness(t)
	for _, spec := range []struct {
		id  string
		key KeyDetailView
	}{
		{"alice", key},
		{"bob", secondKey},
	} {
		if err := targetActions.SecurityKeys.SavePublicKey(ctx, spec.key.PublicKey); err != nil {
			t.Fatalf("seed target public key %s: %v", spec.id, err)
		}
		if _, err := targetActions.BindTrust(ctx, spec.id, spec.key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "trust copied signer"); err != nil {
			t.Fatalf("trust copied signer %s: %v", spec.id, err)
		}
	}
	if err := targetActions.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:           "signed-bundle-import",
		Name:               "Signed bundle import",
		ScopeKind:          contracts.PolicyScopeWorkspace,
		ProtectedActions:   []contracts.ProtectedAction{contracts.ProtectedActionBundleImportApply},
		RequiredSignatures: 2,
		SchemaVersion:      contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save governance policy: %v", err)
	}
	if _, err := targetActions.ImportSyncBundle(ctx, bundle.Job.BundleRef, contracts.Actor("human:owner"), "import trusted signed bundle"); err != nil {
		t.Fatalf("trusted signed bundle should satisfy governance signature policy: %v", err)
	}

	_, unsignedTarget, _, _, _, _ := newImportExportHarness(t)
	if err := unsignedTarget.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:           "signed-bundle-import",
		Name:               "Signed bundle import",
		ScopeKind:          contracts.PolicyScopeWorkspace,
		ProtectedActions:   []contracts.ProtectedAction{contracts.ProtectedActionBundleImportApply},
		RequiredSignatures: 2,
		SchemaVersion:      contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save unsigned target governance policy: %v", err)
	}
	if _, err := unsignedTarget.ImportSyncBundle(ctx, bundle.Job.BundleRef, contracts.Actor("human:owner"), "import without trust"); err == nil || !strings.Contains(err.Error(), "trusted_signature_required") {
		t.Fatalf("untrusted signed bundle should not satisfy governance, got %v", err)
	}
}

func TestGovernanceStructuredImportUsesImportApplyAction(t *testing.T) {
	ctx := context.Background()
	root, actions, _, _, _, _ := newImportExportHarness(t)
	sourcePath := filepath.Join(root, "jira.csv")
	if err := os.WriteFile(sourcePath, []byte("Issue key,Summary,Issue Type,Status,Priority\nAPP-90,Imported ticket,Task,Ready,High\n"), 0o644); err != nil {
		t.Fatalf("write jira csv: %v", err)
	}
	preview, err := actions.PreviewImport(ctx, sourcePath, contracts.Actor("human:owner"), "preview structured import")
	if err != nil {
		t.Fatalf("preview structured import: %v", err)
	}
	if err := actions.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:           "signed-sync-import",
		Name:               "Signed sync import",
		ScopeKind:          contracts.PolicyScopeWorkspace,
		ProtectedActions:   []contracts.ProtectedAction{contracts.ProtectedActionSyncImportApply},
		RequiredSignatures: 1,
		SchemaVersion:      contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save sync import governance policy: %v", err)
	}
	if _, err := actions.ApplyImport(ctx, preview.Job.JobID, contracts.Actor("human:owner"), "apply structured import"); err != nil {
		t.Fatalf("structured import should not be governed as signed sync import: %v", err)
	}
	if _, err := actions.Tickets.GetTicket(ctx, "APP-90"); err != nil {
		t.Fatalf("structured import should create ticket: %v", err)
	}
}

func TestGovernanceExportCreateGuardsDirectCreatePaths(t *testing.T) {
	ctx := context.Background()
	root, actions, _, projects, _, _ := newImportExportHarness(t)
	seedSyncWorkspace(t, ctx, actions, projects)
	if err := actions.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:         "export-create-quorum",
		Name:             "Export create quorum",
		ScopeKind:        contracts.PolicyScopeWorkspace,
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionExportCreate},
		QuorumRules: []contracts.QuorumRule{{
			RuleID:                       "export-approval",
			ActionKind:                   contracts.ProtectedActionExportCreate,
			RequiredCount:                1,
			RequireDistinctCollaborators: true,
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save export governance policy: %v", err)
	}

	if _, err := actions.CreateExportBundle(ctx, "workspace", contracts.Actor("human:owner"), "export workspace"); err == nil || !strings.Contains(err.Error(), "quorum_unsatisfied") {
		t.Fatalf("direct export create should enforce governance before writing, got %v", err)
	}
	if _, err := actions.CreateSyncBundle(ctx, contracts.Actor("human:owner"), "create sync bundle"); err == nil || !strings.Contains(err.Error(), "quorum_unsatisfied") {
		t.Fatalf("direct sync bundle create should enforce governance before writing, got %v", err)
	}
	if _, err := os.Stat(syncMigrationPath(root)); err == nil || !os.IsNotExist(err) {
		t.Fatalf("denied sync bundle create should not write migration state, got %v", err)
	}
}

func TestGovernancePackApplyCreatesScopedPolicyInstances(t *testing.T) {
	ctx, actions, _ := newGovernanceHarness(t)
	created, err := actions.CreateGovernancePack(ctx, GovernancePackCreateOptions{
		Name:             "Release quorum",
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionTicketComplete},
		QuorumCount:      1,
	}, contracts.Actor("human:owner"), "create reusable pack")
	if err != nil {
		t.Fatalf("create governance pack: %v", err)
	}
	if _, err := actions.ApplyGovernancePack(ctx, created.Pack.PackID, "project:APP", contracts.Actor("human:owner"), "apply app"); err != nil {
		t.Fatalf("apply APP scope: %v", err)
	}
	if _, err := actions.ApplyGovernancePack(ctx, created.Pack.PackID, "project:WEB", contracts.Actor("human:owner"), "apply WEB scope"); err != nil {
		t.Fatalf("apply WEB scope: %v", err)
	}
	policies, err := actions.GovernancePolicies.ListGovernancePolicies(ctx)
	if err != nil {
		t.Fatalf("list applied policies: %v", err)
	}
	if len(policies) != 2 {
		t.Fatalf("reapplying a pack to another scope should preserve both policies: %#v", policies)
	}
	ids := []string{policies[0].PolicyID, policies[1].PolicyID}
	if ids[0] == ids[1] || !strings.Contains(strings.Join(ids, ","), "project-app") || !strings.Contains(strings.Join(ids, ","), "project-web") {
		t.Fatalf("scoped policy ids should be distinct and scope-bound: %#v", ids)
	}
}

func TestGovernancePolicyStoreLoadsSnakeCaseTOML(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(storage.GovernancePoliciesDir(root), 0o755); err != nil {
		t.Fatalf("mkdir governance policies: %v", err)
	}
	raw := []byte(`
policy_id = "release-quorum"
name = "Release quorum"
scope_kind = "project"
scope_id = "APP"
protected_actions = ["ticket_complete"]
schema_version = 1

[[quorum_rules]]
rule_id = "reviewer-quorum"
action_kind = "ticket_complete"
required_count = 2
allowed_roles = ["reviewer"]
require_distinct_collaborators = true
`)
	if err := os.WriteFile(governancePolicyPath(root, "release-quorum"), raw, 0o644); err != nil {
		t.Fatalf("write snake-case governance policy: %v", err)
	}
	policy, err := (GovernancePolicyStore{Root: root}).LoadGovernancePolicy(ctx, "release-quorum")
	if err != nil {
		t.Fatalf("load snake-case governance policy: %v", err)
	}
	if policy.PolicyID != "release-quorum" || policy.ScopeKind != contracts.PolicyScopeProject || policy.ScopeID != "APP" {
		t.Fatalf("snake-case policy fields not decoded: %#v", policy)
	}
	if len(policy.ProtectedActions) != 1 || policy.ProtectedActions[0] != contracts.ProtectedActionTicketComplete {
		t.Fatalf("snake-case protected actions not decoded: %#v", policy.ProtectedActions)
	}
	if len(policy.QuorumRules) != 1 || policy.QuorumRules[0].RequiredCount != 2 || len(policy.QuorumRules[0].AllowedRoles) != 1 || policy.QuorumRules[0].AllowedRoles[0] != contracts.MembershipRoleReviewer {
		t.Fatalf("snake-case quorum rules not decoded: %#v", policy.QuorumRules)
	}
}

func TestGovernanceRejectsUnsupportedSignatureRequirements(t *testing.T) {
	ctx, actions, _ := newGovernanceHarness(t)
	err := actions.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:           "impossible-ticket-signature",
		Name:               "Impossible ticket signature",
		ScopeKind:          contracts.PolicyScopeWorkspace,
		ProtectedActions:   []contracts.ProtectedAction{contracts.ProtectedActionTicketComplete},
		RequiredSignatures: 1,
		SchemaVersion:      contracts.CurrentSchemaVersion,
	})
	if err == nil || !strings.Contains(err.Error(), "required_signatures is not supported") {
		t.Fatalf("ticket completion should not accept an impossible signature policy yet, got %v", err)
	}
}

func TestGovernanceValidateRunsRuntimeChecks(t *testing.T) {
	ctx, actions, _ := newGovernanceHarness(t)
	policy := normalizeGovernancePolicy(contracts.GovernancePolicy{
		PolicyID:           "impossible-ticket-signature",
		Name:               "Impossible ticket signature",
		ScopeKind:          contracts.PolicyScopeWorkspace,
		ProtectedActions:   []contracts.ProtectedAction{contracts.ProtectedActionTicketComplete},
		RequiredSignatures: 1,
		SchemaVersion:      contracts.CurrentSchemaVersion,
	})
	raw, err := toml.Marshal(policy)
	if err != nil {
		t.Fatalf("marshal hand-edited policy: %v", err)
	}
	if err := os.MkdirAll(storage.GovernancePoliciesDir(actions.Root), 0o755); err != nil {
		t.Fatalf("create governance policy dir: %v", err)
	}
	if err := os.WriteFile(governancePolicyPath(actions.Root, policy.PolicyID), raw, 0o644); err != nil {
		t.Fatalf("write hand-edited policy: %v", err)
	}
	view, err := actions.ValidateGovernance(ctx)
	if err != nil {
		t.Fatalf("validate governance: %v", err)
	}
	if view.Valid || !strings.Contains(strings.Join(view.Errors, ","), "required_signatures is not supported") {
		t.Fatalf("runtime-invalid policy should fail governance validate: %#v", view)
	}

	rawInvalid := []byte(`policy_id = "bad-action"
name = "Bad action"
scope_kind = "workspace"
protected_actions = ["not_a_real_action"]
schema_version = 2
`)
	if err := os.WriteFile(governancePolicyPath(actions.Root, "bad-action"), rawInvalid, 0o644); err != nil {
		t.Fatalf("write contract-invalid policy: %v", err)
	}
	view, err = actions.ValidateGovernance(ctx)
	if err != nil {
		t.Fatalf("validate governance with contract-invalid policy should still return a report: %v", err)
	}
	if view.Valid || !strings.Contains(strings.Join(view.Errors, ","), "bad-action") || !strings.Contains(strings.Join(view.Errors, ","), "invalid protected action") {
		t.Fatalf("contract-invalid policy should be reported in governance validation output: %#v", view)
	}
}

func TestGovernanceGateRejectDoesNotUseApprovalPolicy(t *testing.T) {
	ctx, actions, ticket := newGovernanceHarness(t)
	gate := seedOpenReviewGate(t, ctx, actions, ticket)
	if err := actions.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:         "gate-approval-quorum",
		Name:             "Gate approval quorum",
		ScopeKind:        contracts.PolicyScopeWorkspace,
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionGateApprove},
		QuorumRules: []contracts.QuorumRule{{
			RuleID:                       "reviewer-approval",
			ActionKind:                   contracts.ProtectedActionGateApprove,
			RequiredCount:                1,
			RequireDistinctCollaborators: true,
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save gate approval policy: %v", err)
	}
	rejected, err := actions.RejectGate(ctx, gate.GateID, contracts.Actor("human:owner"), "reject incomplete work")
	if err != nil {
		t.Fatalf("gate rejection should not be blocked by gate approval governance: %v", err)
	}
	if rejected.State != contracts.GateStateRejected {
		t.Fatalf("expected rejected gate, got %#v", rejected)
	}
}

func TestGovernanceOwnerOverrideMustCoverEveryFailedPolicy(t *testing.T) {
	ctx, actions, ticket := newGovernanceHarness(t)
	if err := actions.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:         "owner-can-override-workspace",
		Name:             "Owner can override workspace",
		ScopeKind:        contracts.PolicyScopeWorkspace,
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionTicketComplete},
		QuorumRules: []contracts.QuorumRule{{
			RuleID:                       "workspace-review",
			ActionKind:                   contracts.ProtectedActionTicketComplete,
			RequiredCount:                1,
			RequireDistinctCollaborators: true,
		}},
		OverrideRules: []contracts.OverrideRule{{
			RuleID:        "owner-override",
			ActionKind:    contracts.ProtectedActionTicketComplete,
			Allowed:       true,
			RequireReason: true,
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save override policy: %v", err)
	}
	if err := actions.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:         "strict-project-quorum",
		Name:             "Strict project quorum",
		ScopeKind:        contracts.PolicyScopeProject,
		ScopeID:          "APP",
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionTicketComplete},
		QuorumRules: []contracts.QuorumRule{{
			RuleID:                       "project-review",
			ActionKind:                   contracts.ProtectedActionTicketComplete,
			RequiredCount:                2,
			RequireDistinctCollaborators: true,
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save strict policy: %v", err)
	}
	explanation, err := actions.ExplainGovernance(ctx, GovernanceEvaluationInput{
		Action: contracts.ProtectedActionTicketComplete,
		Target: "ticket:" + ticket.ID,
		Actor:  contracts.Actor("human:owner"),
		Reason: "emergency override",
	})
	if err != nil {
		t.Fatalf("explain governance: %v", err)
	}
	reasons := strings.Join(explanation.ReasonCodes, ",")
	if explanation.Allowed || !strings.Contains(reasons, "owner_override_not_allowed:strict-project-quorum") {
		t.Fatalf("override from one matching policy must not bypass another failed policy: %#v", explanation)
	}
}

func TestGovernanceOwnerOverridePoliciesAreEnforced(t *testing.T) {
	ctx, actions, ticket := newGovernanceHarness(t)
	if err := actions.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:         "ticket-quorum-with-override",
		Name:             "Ticket quorum with override",
		ScopeKind:        contracts.PolicyScopeProject,
		ScopeID:          "APP",
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionTicketComplete},
		QuorumRules: []contracts.QuorumRule{{
			RuleID:                       "release-review",
			ActionKind:                   contracts.ProtectedActionTicketComplete,
			RequiredCount:                1,
			AllowedRoles:                 []contracts.MembershipRole{contracts.MembershipRoleReviewer},
			RequireDistinctCollaborators: true,
		}},
		OverrideRules: []contracts.OverrideRule{{
			RuleID:        "owner-override",
			ActionKind:    contracts.ProtectedActionTicketComplete,
			Allowed:       true,
			RequireReason: true,
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save overridable policy: %v", err)
	}
	if err := actions.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:         "owner-override-quorum",
		Name:             "Owner override quorum",
		ScopeKind:        contracts.PolicyScopeWorkspace,
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionOwnerOverride},
		QuorumRules: []contracts.QuorumRule{{
			RuleID:                       "owner-override-approval",
			ActionKind:                   contracts.ProtectedActionOwnerOverride,
			RequiredCount:                1,
			RequireDistinctCollaborators: true,
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save owner override policy: %v", err)
	}
	blocked, err := actions.ExplainGovernance(ctx, GovernanceEvaluationInput{
		Action: contracts.ProtectedActionTicketComplete,
		Target: "ticket:" + ticket.ID,
		Actor:  contracts.Actor("human:owner"),
		Reason: "emergency override",
	})
	if err != nil {
		t.Fatalf("explain blocked override: %v", err)
	}
	reasons := strings.Join(blocked.ReasonCodes, ",")
	if blocked.Allowed || !strings.Contains(reasons, "owner_override_policy_unsatisfied:owner-override-quorum") || !strings.Contains(reasons, "quorum_unsatisfied:owner-override-approval") {
		t.Fatalf("owner override policy should block the override: %#v", blocked)
	}
	allowed, err := actions.ExplainGovernance(ctx, GovernanceEvaluationInput{
		Action:         contracts.ProtectedActionTicketComplete,
		Target:         "ticket:" + ticket.ID,
		Actor:          contracts.Actor("human:owner"),
		Reason:         "emergency override",
		ApprovalActors: []contracts.Actor{contracts.Actor("human:owner")},
	})
	if err != nil {
		t.Fatalf("explain approved override: %v", err)
	}
	if !allowed.Allowed || !strings.Contains(strings.Join(allowed.ReasonCodes, ","), "owner_override_applied") {
		t.Fatalf("satisfied owner override policy should permit override: %#v", allowed)
	}
}

func TestGovernanceClassificationScopeRequiresRestrictedLegacyFlag(t *testing.T) {
	ctx, actions, ticket := newGovernanceHarness(t)
	ticket.Protected = true
	if err := actions.UpdateTicket(ctx, ticket); err != nil {
		t.Fatalf("mark ticket protected: %v", err)
	}
	if err := actions.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:         "public-ticket-quorum",
		Name:             "Public ticket quorum",
		ScopeKind:        contracts.PolicyScopeClassification,
		ScopeID:          string(contracts.ClassificationPublic),
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionTicketComplete},
		QuorumRules: []contracts.QuorumRule{{
			RuleID:                       "public-review",
			ActionKind:                   contracts.ProtectedActionTicketComplete,
			RequiredCount:                1,
			RequireDistinctCollaborators: true,
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save public classification policy: %v", err)
	}
	public, err := actions.ExplainGovernance(ctx, GovernanceEvaluationInput{
		Action: contracts.ProtectedActionTicketComplete,
		Target: "ticket:" + ticket.ID,
		Actor:  contracts.Actor("human:owner"),
	})
	if err != nil {
		t.Fatalf("explain public classification governance: %v", err)
	}
	if !public.Allowed {
		t.Fatalf("classification:public policy must not match protected legacy tickets: %#v", public)
	}
	if err := actions.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:         "restricted-ticket-quorum",
		Name:             "Restricted ticket quorum",
		ScopeKind:        contracts.PolicyScopeClassification,
		ScopeID:          string(contracts.ClassificationRestricted),
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionTicketComplete},
		QuorumRules: []contracts.QuorumRule{{
			RuleID:                       "restricted-review",
			ActionKind:                   contracts.ProtectedActionTicketComplete,
			RequiredCount:                1,
			RequireDistinctCollaborators: true,
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save restricted classification policy: %v", err)
	}
	restricted, err := actions.ExplainGovernance(ctx, GovernanceEvaluationInput{
		Action: contracts.ProtectedActionTicketComplete,
		Target: "ticket:" + ticket.ID,
		Actor:  contracts.Actor("human:owner"),
	})
	if err != nil {
		t.Fatalf("explain restricted classification governance: %v", err)
	}
	if restricted.Allowed || !strings.Contains(strings.Join(restricted.ReasonCodes, ","), "quorum_unsatisfied:restricted-review") {
		t.Fatalf("classification:restricted policy should match protected legacy tickets: %#v", restricted)
	}
}

func TestGovernanceSyncPullDoesNotRequireManualBundlePolicy(t *testing.T) {
	ctx := context.Background()
	_, sourceActions, _, sourceProjects, _, _ := newImportExportHarness(t)
	seedSyncWorkspace(t, ctx, sourceActions, sourceProjects)
	remoteDir := filepath.Join(t.TempDir(), "path-remote")
	actor := contracts.Actor("human:owner")
	remote, err := sourceActions.AddSyncRemote(ctx, contracts.SyncRemote{
		RemoteID:      "origin",
		Kind:          contracts.SyncRemoteKindPath,
		Location:      remoteDir,
		DefaultAction: contracts.SyncDefaultActionPush,
		Enabled:       true,
	}, actor, "seed path remote")
	if err != nil {
		t.Fatalf("add source remote: %v", err)
	}
	pushView, err := sourceActions.SyncPush(ctx, remote.RemoteID, actor, "push workspace")
	if err != nil {
		t.Fatalf("sync push: %v", err)
	}
	_, targetActions, targetQueries, _, _, _ := newImportExportHarness(t)
	if _, err := targetActions.AddSyncRemote(ctx, contracts.SyncRemote{
		RemoteID:      "origin",
		Kind:          contracts.SyncRemoteKindPath,
		Location:      remoteDir,
		DefaultAction: contracts.SyncDefaultActionPull,
		Enabled:       true,
	}, actor, "seed target remote"); err != nil {
		t.Fatalf("add target remote: %v", err)
	}
	if err := targetActions.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:         "manual-bundle-import-quorum",
		Name:             "Manual bundle import quorum",
		ScopeKind:        contracts.PolicyScopeWorkspace,
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionBundleImportApply},
		QuorumRules: []contracts.QuorumRule{{
			RuleID:                       "manual-import-review",
			ActionKind:                   contracts.ProtectedActionBundleImportApply,
			RequiredCount:                1,
			RequireDistinctCollaborators: true,
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save manual bundle governance policy: %v", err)
	}
	if _, err := targetActions.SyncPull(ctx, "origin", pushView.Publication.WorkspaceID, actor, "pull workspace"); err != nil {
		t.Fatalf("sync pull should not be blocked by manual bundle import governance: %v", err)
	}
	if _, err := targetQueries.TicketDetail(ctx, "APP-1"); err != nil {
		t.Fatalf("expected synced ticket detail: %v", err)
	}
	if _, err := targetActions.ImportSyncBundle(ctx, pushView.Job.BundleRef, actor, "manual import"); err == nil || !strings.Contains(err.Error(), "quorum_unsatisfied") {
		t.Fatalf("manual bundle import policy should still block direct imports, got %v", err)
	}
}

func TestGovernanceDeniedBundleImportDoesNotStampMigration(t *testing.T) {
	ctx := context.Background()
	_, sourceActions, _, sourceProjects, _, _ := newImportExportHarness(t)
	seedSyncWorkspace(t, ctx, sourceActions, sourceProjects)
	bundle, err := sourceActions.CreateSyncBundle(ctx, contracts.Actor("human:owner"), "create sync bundle")
	if err != nil {
		t.Fatalf("create sync bundle: %v", err)
	}
	targetRoot, targetActions, _, _, _, _ := newImportExportHarness(t)
	if err := targetActions.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:         "manual-bundle-import-quorum",
		Name:             "Manual bundle import quorum",
		ScopeKind:        contracts.PolicyScopeWorkspace,
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionBundleImportApply},
		QuorumRules: []contracts.QuorumRule{{
			RuleID:                       "manual-import-review",
			ActionKind:                   contracts.ProtectedActionBundleImportApply,
			RequiredCount:                1,
			RequireDistinctCollaborators: true,
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save manual bundle governance policy: %v", err)
	}
	if _, err := targetActions.ImportSyncBundle(ctx, bundle.Job.BundleRef, contracts.Actor("human:owner"), "manual import"); err == nil || !strings.Contains(err.Error(), "quorum_unsatisfied") {
		t.Fatalf("manual bundle import policy should block before migration, got %v", err)
	}
	if _, err := os.Stat(syncMigrationPath(targetRoot)); err == nil || !os.IsNotExist(err) {
		t.Fatalf("denied bundle import should not write migration state, got %v", err)
	}
}

func TestGovernanceDeniedSyncPullDoesNotPersistMirror(t *testing.T) {
	ctx := context.Background()
	_, sourceActions, _, sourceProjects, _, _ := newImportExportHarness(t)
	seedSyncWorkspace(t, ctx, sourceActions, sourceProjects)
	remoteDir := filepath.Join(t.TempDir(), "path-remote")
	actor := contracts.Actor("human:owner")
	remote, err := sourceActions.AddSyncRemote(ctx, contracts.SyncRemote{
		RemoteID:      "origin",
		Kind:          contracts.SyncRemoteKindPath,
		Location:      remoteDir,
		DefaultAction: contracts.SyncDefaultActionPush,
		Enabled:       true,
	}, actor, "seed path remote")
	if err != nil {
		t.Fatalf("add source remote: %v", err)
	}
	pushView, err := sourceActions.SyncPush(ctx, remote.RemoteID, actor, "push workspace")
	if err != nil {
		t.Fatalf("sync push: %v", err)
	}
	targetRoot, targetActions, _, _, _, _ := newImportExportHarness(t)
	if _, err := targetActions.AddSyncRemote(ctx, contracts.SyncRemote{
		RemoteID:      "origin",
		Kind:          contracts.SyncRemoteKindPath,
		Location:      remoteDir,
		DefaultAction: contracts.SyncDefaultActionPull,
		Enabled:       true,
	}, actor, "seed target remote"); err != nil {
		t.Fatalf("add target remote: %v", err)
	}
	if err := targetActions.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:         "sync-import-quorum",
		Name:             "Sync import quorum",
		ScopeKind:        contracts.PolicyScopeWorkspace,
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionSyncImportApply},
		QuorumRules: []contracts.QuorumRule{{
			RuleID:                       "sync-review",
			ActionKind:                   contracts.ProtectedActionSyncImportApply,
			RequiredCount:                1,
			RequireDistinctCollaborators: true,
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save sync import governance policy: %v", err)
	}
	if _, err := targetActions.SyncPull(ctx, "origin", pushView.Publication.WorkspaceID, actor, "pull workspace"); err == nil || !strings.Contains(err.Error(), "quorum_unsatisfied") {
		t.Fatalf("sync pull should be denied by sync import governance, got %v", err)
	}
	mirrorDir := storage.SyncMirrorRemoteDir(targetRoot, "origin")
	entries, err := os.ReadDir(mirrorDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read mirror dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("denied sync pull must not persist fetched publications in durable mirror: %#v", entries)
	}
}

func TestGovernanceDeniedGitSyncPullDoesNotPersistMirrorCache(t *testing.T) {
	ctx := context.Background()
	_, sourceActions, _, sourceProjects, _, _ := newImportExportHarness(t)
	seedSyncWorkspace(t, ctx, sourceActions, sourceProjects)
	gitRemote := filepath.Join(t.TempDir(), "sync-remote.git")
	gitRun(t, t.TempDir(), "init", "--bare", gitRemote)
	actor := contracts.Actor("human:owner")
	remote, err := sourceActions.AddSyncRemote(ctx, contracts.SyncRemote{
		RemoteID:      "origin",
		Kind:          contracts.SyncRemoteKindGit,
		Location:      gitRemote,
		DefaultAction: contracts.SyncDefaultActionPush,
		Enabled:       true,
	}, actor, "seed git remote")
	if err != nil {
		t.Fatalf("add source git remote: %v", err)
	}
	pushView, err := sourceActions.SyncPush(ctx, remote.RemoteID, actor, "push workspace")
	if err != nil {
		t.Fatalf("sync git push: %v", err)
	}
	targetRoot, targetActions, _, _, _, _ := newImportExportHarness(t)
	if _, err := targetActions.AddSyncRemote(ctx, contracts.SyncRemote{
		RemoteID:      "origin",
		Kind:          contracts.SyncRemoteKindGit,
		Location:      gitRemote,
		DefaultAction: contracts.SyncDefaultActionPull,
		Enabled:       true,
	}, actor, "seed target git remote"); err != nil {
		t.Fatalf("add target git remote: %v", err)
	}
	if err := targetActions.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:         "sync-import-quorum",
		Name:             "Sync import quorum",
		ScopeKind:        contracts.PolicyScopeWorkspace,
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionSyncImportApply},
		QuorumRules: []contracts.QuorumRule{{
			RuleID:                       "sync-review",
			ActionKind:                   contracts.ProtectedActionSyncImportApply,
			RequiredCount:                1,
			RequireDistinctCollaborators: true,
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save sync import governance policy: %v", err)
	}
	if _, err := targetActions.SyncPull(ctx, "origin", pushView.Publication.WorkspaceID, actor, "pull workspace"); err == nil || !strings.Contains(err.Error(), "quorum_unsatisfied") {
		t.Fatalf("git sync pull should be denied by sync import governance, got %v", err)
	}
	mirrorDir := storage.SyncMirrorRemoteDir(targetRoot, "origin")
	entries, err := os.ReadDir(mirrorDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read mirror dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("denied git sync pull must not persist publications or git cache in durable mirror: %#v", entries)
	}
}

func TestGovernanceOverrideIsRecordedOnlyAfterSuccess(t *testing.T) {
	ctx := context.Background()
	_, sourceActions, _, sourceProjects, _, _ := newImportExportHarness(t)
	seedSyncWorkspace(t, ctx, sourceActions, sourceProjects)
	bundle, err := sourceActions.CreateSyncBundle(ctx, contracts.Actor("human:owner"), "create sync bundle")
	if err != nil {
		t.Fatalf("create sync bundle: %v", err)
	}
	checksumPath := strings.TrimSuffix(bundle.Job.BundleRef, ".tar.gz") + ".sha256"
	if err := os.WriteFile(checksumPath, []byte("0000  "+filepath.Base(bundle.Job.BundleRef)+"\n"), 0o644); err != nil {
		t.Fatalf("tamper sync checksum: %v", err)
	}
	targetRoot, targetActions, _, _, _, targetEvents := newImportExportHarness(t)
	if err := targetActions.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:         "manual-import-override",
		Name:             "Manual import override",
		ScopeKind:        contracts.PolicyScopeWorkspace,
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionBundleImportApply},
		QuorumRules: []contracts.QuorumRule{{
			RuleID:                       "manual-import-review",
			ActionKind:                   contracts.ProtectedActionBundleImportApply,
			RequiredCount:                1,
			RequireDistinctCollaborators: true,
		}},
		OverrideRules: []contracts.OverrideRule{{
			RuleID:        "owner-override",
			ActionKind:    contracts.ProtectedActionBundleImportApply,
			Allowed:       true,
			RequireReason: true,
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save import override governance policy: %v", err)
	}
	if _, err := targetActions.ImportSyncBundle(ctx, bundle.Job.BundleRef, contracts.Actor("human:owner"), "emergency import override"); err == nil || !strings.Contains(err.Error(), "sync bundle verification failed") {
		t.Fatalf("import should fail after owner override is accepted, got %v", err)
	}
	events, err := targetEvents.StreamEvents(ctx, workspaceEventProject, 0)
	if err != nil {
		t.Fatalf("stream workspace events in %s: %v", targetRoot, err)
	}
	for _, event := range events {
		if event.Type == contracts.EventGovernanceOverrideRecorded {
			t.Fatalf("failed protected action must not record owner override: %#v", event)
		}
	}
}

func TestGovernanceArchiveUsesProjectScopedTargets(t *testing.T) {
	ctx := context.Background()
	root, actions, _, projectStore, ticketStore, _ := newImportExportHarness(t)
	seedGovernanceArchiveCandidate(t, ctx, root, actions, projectStore, ticketStore, "run_archive_apply")
	if err := actions.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:         "app-archive-apply",
		Name:             "APP archive apply",
		ScopeKind:        contracts.PolicyScopeProject,
		ScopeID:          "APP",
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionArchiveApply},
		QuorumRules: []contracts.QuorumRule{{
			RuleID:                       "archive-approval",
			ActionKind:                   contracts.ProtectedActionArchiveApply,
			RequiredCount:                1,
			RequireDistinctCollaborators: true,
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save archive apply policy: %v", err)
	}
	if _, err := actions.ApplyArchive(ctx, contracts.RetentionTargetRuntime, "APP", true, contracts.Actor("human:owner"), "archive runtime"); err == nil || !strings.Contains(err.Error(), "quorum_unsatisfied") {
		t.Fatalf("project-scoped archive apply policy should block APP archive, got %v", err)
	}

	root, actions, _, projectStore, ticketStore, _ = newImportExportHarness(t)
	seedGovernanceArchiveCandidate(t, ctx, root, actions, projectStore, ticketStore, "run_archive_restore")
	applied, err := actions.ApplyArchive(ctx, contracts.RetentionTargetRuntime, "APP", true, contracts.Actor("human:owner"), "archive runtime")
	if err != nil {
		t.Fatalf("seed archive: %v", err)
	}
	if err := actions.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:         "app-archive-restore",
		Name:             "APP archive restore",
		ScopeKind:        contracts.PolicyScopeProject,
		ScopeID:          "APP",
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionArchiveRestore},
		QuorumRules: []contracts.QuorumRule{{
			RuleID:                       "restore-approval",
			ActionKind:                   contracts.ProtectedActionArchiveRestore,
			RequiredCount:                1,
			RequireDistinctCollaborators: true,
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save archive restore policy: %v", err)
	}
	if _, err := actions.RestoreArchive(ctx, applied.Record.ArchiveID, contracts.Actor("human:owner"), "restore runtime"); err == nil || !strings.Contains(err.Error(), "quorum_unsatisfied") {
		t.Fatalf("project-scoped archive restore policy should block APP restore, got %v", err)
	}
}

func TestGovernanceRevokeTrustUsesProtectedRevokeKey(t *testing.T) {
	ctx, _, actions := newSecurityTestActions(t)
	key, err := actions.GenerateKey(ctx, KeyGenerateOptions{Scope: contracts.KeyScopeCollaborator, OwnerID: "alice"}, contracts.Actor("human:owner"), "create alice key")
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if _, err := actions.BindTrust(ctx, "alice", key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "trust alice key"); err != nil {
		t.Fatalf("bind trust: %v", err)
	}
	if err := actions.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:         "key-revoke-quorum",
		Name:             "Key revoke quorum",
		ScopeKind:        contracts.PolicyScopeWorkspace,
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionRevokeKey},
		QuorumRules: []contracts.QuorumRule{{
			RuleID:                       "trust-revoke-review",
			ActionKind:                   contracts.ProtectedActionRevokeKey,
			RequiredCount:                1,
			RequireDistinctCollaborators: true,
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save revoke governance policy: %v", err)
	}
	if _, err := actions.RevokeTrustForKey(ctx, key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "local distrust ceremony"); err == nil || !strings.Contains(err.Error(), "quorum_unsatisfied") {
		t.Fatalf("trust revoke should honor revoke_key governance, got %v", err)
	}
	bindings, err := actions.TrustBindings.ListTrustBindingsForKey(ctx, key.PublicKey.PublicKeyID)
	if err != nil {
		t.Fatalf("list trust bindings: %v", err)
	}
	if len(bindings) != 1 || bindings[0].TrustLevel != contracts.TrustLevelTrusted {
		t.Fatalf("blocked trust revoke should leave binding trusted: %#v", bindings)
	}
}

func seedGovernanceArchiveCandidate(t *testing.T, ctx context.Context, root string, actions *ActionService, projectStore contracts.ProjectStore, ticketStore contracts.TicketStore, runID string) {
	t.Helper()
	now := actions.now()
	old := now.AddDate(0, 0, -10)
	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := ticketStore.CreateTicket(ctx, contracts.TicketSnapshot{ID: "APP-1", Project: "APP", Title: "Archive runtime", Type: contracts.TicketTypeTask, Status: contracts.StatusDone, Priority: contracts.PriorityHigh, CreatedAt: old, UpdatedAt: old, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	run := normalizeRunSnapshot(contracts.RunSnapshot{RunID: runID, TicketID: "APP-1", Project: "APP", Status: contracts.RunStatusCompleted, Kind: contracts.RunKindWork, CreatedAt: old, CompletedAt: old, SchemaVersion: contracts.CurrentSchemaVersion})
	if err := (RunStore{Root: root}).SaveRun(ctx, run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	for _, path := range []string{storage.RuntimeBriefFile(root, run.RunID), storage.RuntimeContextFile(root, run.RunID), storage.RuntimeLaunchFile(root, run.RunID, "codex"), storage.RuntimeLaunchFile(root, run.RunID, "claude")} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir runtime dir: %v", err)
		}
		if err := os.WriteFile(path, []byte("runtime"), 0o644); err != nil {
			t.Fatalf("write runtime artifact: %v", err)
		}
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatalf("chtimes %s: %v", path, err)
		}
	}
	if err := os.Chtimes(storage.RuntimeDir(root, run.RunID), old, old); err != nil {
		t.Fatalf("chtimes runtime dir: %v", err)
	}
}
