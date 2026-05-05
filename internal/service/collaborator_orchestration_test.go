package service

import (
	"context"
	"slices"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestCollaboratorGateApprovalRequiresTrustAndMatchingRole(t *testing.T) {
	ctx := context.Background()
	_, actions, queries, projectStore, _, _ := newImportExportHarness(t)
	now := actions.now()

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket, err := actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "Review me",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusInReview,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed ticket")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	gate := seedOpenReviewGate(t, ctx, actions, ticket)

	if _, err := actions.AddCollaborator(ctx, contracts.CollaboratorProfile{
		CollaboratorID: "rev-1",
		DisplayName:    "Rev One",
		AtlasActors:    []contracts.Actor{"agent:reviewer-1"},
	}, contracts.Actor("human:owner"), "seed collaborator"); err != nil {
		t.Fatalf("add collaborator: %v", err)
	}
	if _, err := actions.BindMembership(ctx, contracts.MembershipBinding{
		CollaboratorID: "rev-1",
		ScopeKind:      contracts.MembershipScopeProject,
		ScopeID:        "APP",
		Role:           contracts.MembershipRoleReviewer,
	}, contracts.Actor("human:owner"), "bind reviewer membership"); err != nil {
		t.Fatalf("bind membership: %v", err)
	}

	if _, err := actions.ApproveGate(ctx, gate.GateID, contracts.Actor("agent:reviewer-1"), "approve while untrusted"); err == nil || apperr.CodeOf(err) != apperr.CodePermissionDenied {
		t.Fatalf("expected untrusted collaborator to be blocked, got %v", err)
	}
	permissionView, err := queries.PermissionsView(ctx, "gate:"+gate.GateID, contracts.Actor("agent:reviewer-1"), contracts.PermissionActionGateApprove)
	if err != nil {
		t.Fatalf("permissions view: %v", err)
	}
	if len(permissionView.Decisions) != 1 || !slices.Contains(permissionView.Decisions[0].ReasonCodes, "collaborator_untrusted") {
		t.Fatalf("expected collaborator_untrusted reason, got %#v", permissionView.Decisions)
	}

	if _, err := actions.SetCollaboratorTrust(ctx, "rev-1", true, contracts.Actor("human:owner"), "trust reviewer"); err != nil {
		t.Fatalf("trust collaborator: %v", err)
	}
	if _, err := actions.ApproveGate(ctx, gate.GateID, contracts.Actor("agent:builder-1"), "approve without role"); err == nil || apperr.CodeOf(err) != apperr.CodePermissionDenied {
		t.Fatalf("expected unmatched actor to be blocked, got %v", err)
	}
	approved, err := actions.ApproveGate(ctx, gate.GateID, contracts.Actor("agent:reviewer-1"), "approve with reviewer role")
	if err != nil {
		t.Fatalf("approve gate with mapped reviewer: %v", err)
	}
	if approved.State != contracts.GateStateApproved {
		t.Fatalf("expected approved gate, got %#v", approved)
	}
}

func TestMembershipProfilesApplyToMappedCollaborators(t *testing.T) {
	ctx := context.Background()
	_, actions, queries, projectStore, _, _ := newImportExportHarness(t)
	now := actions.now()

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket, err := actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "Guard collaborator approvals",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusInReview,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed ticket")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	gate := seedOpenReviewGate(t, ctx, actions, ticket)

	if _, err := actions.SavePermissionProfile(ctx, contracts.PermissionProfile{
		ProfileID:     "deny-gate",
		DisplayName:   "Deny Gate Approve",
		DenyActions:   []contracts.PermissionAction{contracts.PermissionActionGateApprove},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed profile"); err != nil {
		t.Fatalf("save permission profile: %v", err)
	}
	if _, err := actions.AddCollaborator(ctx, contracts.CollaboratorProfile{
		CollaboratorID: "rev-1",
		DisplayName:    "Rev One",
		AtlasActors:    []contracts.Actor{"agent:reviewer-1"},
		TrustState:     contracts.CollaboratorTrustStateTrusted,
	}, contracts.Actor("human:owner"), "seed collaborator"); err != nil {
		t.Fatalf("add collaborator: %v", err)
	}
	if _, err := actions.BindMembership(ctx, contracts.MembershipBinding{
		CollaboratorID:            "rev-1",
		ScopeKind:                 contracts.MembershipScopeProject,
		ScopeID:                   "APP",
		Role:                      contracts.MembershipRoleReviewer,
		DefaultPermissionProfiles: []string{"deny-gate"},
	}, contracts.Actor("human:owner"), "bind reviewer membership"); err != nil {
		t.Fatalf("bind membership: %v", err)
	}

	view, err := queries.PermissionsView(ctx, "gate:"+gate.GateID, contracts.Actor("agent:reviewer-1"), contracts.PermissionActionGateApprove)
	if err != nil {
		t.Fatalf("permissions view: %v", err)
	}
	if len(view.Decisions) != 1 {
		t.Fatalf("expected one decision, got %#v", view.Decisions)
	}
	decision := view.Decisions[0]
	if decision.Allowed {
		t.Fatalf("expected membership profile to deny gate approve, got %#v", decision)
	}
	if !slices.Contains(decision.ReasonCodes, "permission_action_denied") {
		t.Fatalf("expected permission_action_denied, got %#v", decision.ReasonCodes)
	}
	foundLayer := false
	for _, match := range decision.Profiles {
		if match.Layer == PermissionLayerMembership && match.ProfileID == "deny-gate" {
			foundLayer = true
			break
		}
	}
	if !foundLayer {
		t.Fatalf("expected membership-bound profile in permission decision, got %#v", decision.Profiles)
	}
}

func TestApprovalsExcludeSuspendedCollaborator(t *testing.T) {
	ctx := context.Background()
	_, actions, queries, projectStore, _, _ := newImportExportHarness(t)
	now := actions.now()

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket, err := actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "Suspended reviewer",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusInReview,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed ticket")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	_ = seedOpenReviewGate(t, ctx, actions, ticket)

	if _, err := actions.AddCollaborator(ctx, contracts.CollaboratorProfile{
		CollaboratorID: "rev-1",
		DisplayName:    "Rev One",
		AtlasActors:    []contracts.Actor{"agent:reviewer-1"},
		TrustState:     contracts.CollaboratorTrustStateTrusted,
	}, contracts.Actor("human:owner"), "seed collaborator"); err != nil {
		t.Fatalf("add collaborator: %v", err)
	}
	if _, err := actions.BindMembership(ctx, contracts.MembershipBinding{
		CollaboratorID: "rev-1",
		ScopeKind:      contracts.MembershipScopeProject,
		ScopeID:        "APP",
		Role:           contracts.MembershipRoleReviewer,
	}, contracts.Actor("human:owner"), "bind reviewer membership"); err != nil {
		t.Fatalf("bind membership: %v", err)
	}

	items, err := queries.Approvals(ctx, "rev-1")
	if err != nil {
		t.Fatalf("approvals before suspend: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one approval before suspend, got %#v", items)
	}
	if _, err := actions.SetCollaboratorStatus(ctx, "rev-1", contracts.CollaboratorStatusSuspended, contracts.Actor("human:owner"), "suspend reviewer"); err != nil {
		t.Fatalf("suspend collaborator: %v", err)
	}
	items, err = queries.Approvals(ctx, "rev-1")
	if err != nil {
		t.Fatalf("approvals after suspend: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected suspended collaborator to disappear from approvals, got %#v", items)
	}
}

func TestRequireChangeMergeAuthorityAllowsReviewerMembership(t *testing.T) {
	ctx := context.Background()
	_, actions, _, projectStore, _, _ := newImportExportHarness(t)
	now := actions.now()

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := actions.AddCollaborator(ctx, contracts.CollaboratorProfile{
		CollaboratorID: "rev-1",
		DisplayName:    "Rev One",
		AtlasActors:    []contracts.Actor{"agent:reviewer-1"},
		TrustState:     contracts.CollaboratorTrustStateTrusted,
	}, contracts.Actor("human:owner"), "seed collaborator"); err != nil {
		t.Fatalf("add collaborator: %v", err)
	}
	if _, err := actions.BindMembership(ctx, contracts.MembershipBinding{
		CollaboratorID: "rev-1",
		ScopeKind:      contracts.MembershipScopeProject,
		ScopeID:        "APP",
		Role:           contracts.MembershipRoleReviewer,
	}, contracts.Actor("human:owner"), "bind reviewer membership"); err != nil {
		t.Fatalf("bind membership: %v", err)
	}

	ticket := contracts.NormalizeTicketSnapshot(contracts.TicketSnapshot{
		ID:            "APP-1",
		TicketUID:     contracts.TicketUID("APP", "APP-1"),
		Project:       "APP",
		Title:         "Merge me",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusInReview,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	})
	if err := requireChangeMergeAuthority(ctx, actions.Collaborators, actions.Memberships, ticket, contracts.Actor("agent:reviewer-1")); err != nil {
		t.Fatalf("expected reviewer membership to authorize merge: %v", err)
	}
	if err := requireChangeMergeAuthority(ctx, actions.Collaborators, actions.Memberships, ticket, contracts.Actor("agent:builder-1")); err == nil || apperr.CodeOf(err) != apperr.CodePermissionDenied {
		t.Fatalf("expected unmatched actor to be denied, got %v", err)
	}
}

func TestCompleteRunBlocksUntrustedMappedCollaboratorAfterRunStart(t *testing.T) {
	ctx := context.Background()
	_, actions, queries, projectStore, _, _ := newImportExportHarness(t)
	now := actions.now()

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket, err := actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "Finish run with trust revoked",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusInProgress,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed ticket")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	run := normalizeRunSnapshot(contracts.RunSnapshot{
		RunID:         "run_trust_flip",
		TicketID:      ticket.ID,
		Project:       ticket.Project,
		AgentID:       "builder-1",
		Status:        contracts.RunStatusActive,
		Kind:          contracts.RunKindWork,
		CreatedAt:     now,
		StartedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	})
	if err := actions.Runs.SaveRun(ctx, run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	if _, err := actions.AddCollaborator(ctx, contracts.CollaboratorProfile{
		CollaboratorID: "builder-1",
		DisplayName:    "Builder One",
		AtlasActors:    []contracts.Actor{"agent:builder-1"},
		TrustState:     contracts.CollaboratorTrustStateTrusted,
	}, contracts.Actor("human:owner"), "seed collaborator"); err != nil {
		t.Fatalf("add collaborator: %v", err)
	}
	if _, err := actions.BindMembership(ctx, contracts.MembershipBinding{
		CollaboratorID: "builder-1",
		ScopeKind:      contracts.MembershipScopeProject,
		ScopeID:        "APP",
		Role:           contracts.MembershipRoleContributor,
	}, contracts.Actor("human:owner"), "bind contributor membership"); err != nil {
		t.Fatalf("bind membership: %v", err)
	}
	if _, err := actions.SetCollaboratorTrust(ctx, "builder-1", false, contracts.Actor("human:owner"), "revoke trust mid-flight"); err != nil {
		t.Fatalf("revoke collaborator trust: %v", err)
	}

	if _, err := actions.CompleteRun(ctx, run.RunID, contracts.Actor("agent:builder-1"), "finish with revoked trust", "done"); err == nil || apperr.CodeOf(err) != apperr.CodePermissionDenied {
		t.Fatalf("expected run completion to be blocked, got %v", err)
	}
	view, err := queries.PermissionsView(ctx, "run:"+run.RunID, contracts.Actor("agent:builder-1"), contracts.PermissionActionRunComplete)
	if err != nil {
		t.Fatalf("permissions view: %v", err)
	}
	if len(view.Decisions) != 1 || !slices.Contains(view.Decisions[0].ReasonCodes, "collaborator_untrusted") {
		t.Fatalf("expected collaborator_untrusted reason, got %#v", view.Decisions)
	}
	stored, err := actions.Runs.LoadRun(ctx, run.RunID)
	if err != nil {
		t.Fatalf("load run after denied completion: %v", err)
	}
	if stored.Status != contracts.RunStatusActive {
		t.Fatalf("expected run to stay active after denied completion, got %#v", stored)
	}
}

func TestCompleteRunBlocksWhenMembershipIsRemovedMidFlight(t *testing.T) {
	ctx := context.Background()
	_, actions, queries, projectStore, _, _ := newImportExportHarness(t)
	now := actions.now()

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket, err := actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "Finish run with removed membership",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusInProgress,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed ticket")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	run := normalizeRunSnapshot(contracts.RunSnapshot{
		RunID:         "run_membership_flip",
		TicketID:      ticket.ID,
		Project:       ticket.Project,
		AgentID:       "builder-1",
		Status:        contracts.RunStatusActive,
		Kind:          contracts.RunKindWork,
		CreatedAt:     now,
		StartedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	})
	if err := actions.Runs.SaveRun(ctx, run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	if _, err := actions.AddCollaborator(ctx, contracts.CollaboratorProfile{
		CollaboratorID: "builder-1",
		DisplayName:    "Builder One",
		AtlasActors:    []contracts.Actor{"agent:builder-1"},
		TrustState:     contracts.CollaboratorTrustStateTrusted,
	}, contracts.Actor("human:owner"), "seed collaborator"); err != nil {
		t.Fatalf("add collaborator: %v", err)
	}
	membership, err := actions.BindMembership(ctx, contracts.MembershipBinding{
		CollaboratorID: "builder-1",
		ScopeKind:      contracts.MembershipScopeProject,
		ScopeID:        "APP",
		Role:           contracts.MembershipRoleContributor,
	}, contracts.Actor("human:owner"), "bind contributor membership")
	if err != nil {
		t.Fatalf("bind membership: %v", err)
	}
	if _, err := actions.UnbindMembership(ctx, membership.MembershipUID, contracts.Actor("human:owner"), "remove membership mid-flight"); err != nil {
		t.Fatalf("unbind membership: %v", err)
	}

	if _, err := actions.CompleteRun(ctx, run.RunID, contracts.Actor("agent:builder-1"), "finish without membership", "done"); err == nil || apperr.CodeOf(err) != apperr.CodePermissionDenied {
		t.Fatalf("expected run completion to be blocked without membership, got %v", err)
	}
	view, err := queries.PermissionsView(ctx, "run:"+run.RunID, contracts.Actor("agent:builder-1"), contracts.PermissionActionRunComplete)
	if err != nil {
		t.Fatalf("permissions view: %v", err)
	}
	if len(view.Decisions) != 1 || !slices.Contains(view.Decisions[0].ReasonCodes, "missing_membership") {
		t.Fatalf("expected missing_membership reason, got %#v", view.Decisions)
	}
}

func TestRemovedCollaboratorStaysVisibleInMentionHistory(t *testing.T) {
	ctx := context.Background()
	_, actions, queries, projectStore, _, _ := newImportExportHarness(t)
	now := actions.now()

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket, err := actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "Mention history survives removal",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed ticket")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if _, err := actions.AddCollaborator(ctx, contracts.CollaboratorProfile{
		CollaboratorID: "alana",
		DisplayName:    "Alana",
		AtlasActors:    []contracts.Actor{"agent:reviewer-1"},
		TrustState:     contracts.CollaboratorTrustStateTrusted,
	}, contracts.Actor("human:owner"), "seed collaborator"); err != nil {
		t.Fatalf("add collaborator: %v", err)
	}
	if err := actions.CommentTicket(ctx, ticket.ID, "please review @alana", contracts.Actor("human:owner"), "mention reviewer"); err != nil {
		t.Fatalf("comment ticket: %v", err)
	}
	if _, err := actions.SetCollaboratorStatus(ctx, "alana", contracts.CollaboratorStatusRemoved, contracts.Actor("human:owner"), "remove collaborator"); err != nil {
		t.Fatalf("remove collaborator: %v", err)
	}

	detail, err := queries.TicketDetail(ctx, ticket.ID)
	if err != nil {
		t.Fatalf("ticket detail: %v", err)
	}
	if len(detail.Mentions) != 1 || detail.Mentions[0].CollaboratorID != "alana" {
		t.Fatalf("expected historical mention to remain visible, got %#v", detail.Mentions)
	}
	collaborator, err := queries.CollaboratorDetail(ctx, "alana")
	if err != nil {
		t.Fatalf("collaborator detail: %v", err)
	}
	if collaborator.Collaborator.Status != contracts.CollaboratorStatusRemoved {
		t.Fatalf("expected removed collaborator status, got %#v", collaborator.Collaborator)
	}
	if len(collaborator.Mentions) != 1 || collaborator.Mentions[0].CollaboratorID != "alana" {
		t.Fatalf("expected collaborator mention history to remain visible, got %#v", collaborator.Mentions)
	}
}

func seedOpenReviewGate(t *testing.T, ctx context.Context, actions *ActionService, ticket contracts.TicketSnapshot) contracts.GateSnapshot {
	t.Helper()
	gate := contracts.GateSnapshot{
		GateID:        "gate_" + NewOpaqueID(),
		TicketID:      ticket.ID,
		Kind:          contracts.GateKindReview,
		State:         contracts.GateStateOpen,
		RequiredRole:  contracts.AgentRoleReviewer,
		CreatedBy:     contracts.Actor("human:owner"),
		CreatedAt:     actions.now(),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := actions.Gates.SaveGate(ctx, gate); err != nil {
		t.Fatalf("save gate: %v", err)
	}
	ticket.OpenGateIDs = []string{gate.GateID}
	ticket.UpdatedAt = actions.now()
	if err := actions.UpdateTicket(ctx, ticket); err != nil {
		t.Fatalf("update ticket: %v", err)
	}
	return gate
}
