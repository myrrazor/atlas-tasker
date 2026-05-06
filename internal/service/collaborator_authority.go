package service

import (
	"context"
	"fmt"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func collaboratorAuthorityContext(ctx context.Context, collaborators contracts.CollaboratorStore, memberships contracts.MembershipStore, actor contracts.Actor) (contracts.CollaboratorProfile, []contracts.MembershipBinding, bool, error) {
	if actor == "" || actor == contracts.Actor("human:owner") {
		return contracts.CollaboratorProfile{}, nil, false, nil
	}
	items, err := collaborators.ListCollaborators(ctx)
	if err != nil {
		return contracts.CollaboratorProfile{}, nil, false, err
	}
	for _, collaborator := range items {
		for _, mapped := range collaborator.AtlasActors {
			if mapped != actor {
				continue
			}
			bindings, err := memberships.ListMemberships(ctx, collaborator.CollaboratorID)
			if err != nil {
				return contracts.CollaboratorProfile{}, nil, false, err
			}
			return collaborator, bindings, true, nil
		}
	}
	return contracts.CollaboratorProfile{}, nil, false, nil
}

func (s *ActionService) authorizeGateDecision(ctx context.Context, actor contracts.Actor, ticket contracts.TicketSnapshot, gate contracts.GateSnapshot) error {
	if actor == contracts.Actor("human:owner") {
		return nil
	}
	if gate.RequiredAgentID != "" && actor == contracts.Actor(gate.RequiredAgentID) {
		return nil
	}
	if ticket.Reviewer != "" && gate.Kind == contracts.GateKindReview && actor == ticket.Reviewer {
		return nil
	}

	collaborator, memberships, found, err := collaboratorAuthorityContext(ctx, s.Collaborators, s.Memberships, actor)
	if err != nil {
		return err
	}
	if !found {
		return apperr.New(apperr.CodePermissionDenied, fmt.Sprintf("%s cannot decide gate %s", actor, gate.GateID))
	}
	if collaborator.Status != contracts.CollaboratorStatusActive {
		return apperr.New(apperr.CodePermissionDenied, "collaborator_suspended")
	}
	projectMemberships := activeMembershipsForProject(memberships, ticket.Project)
	if len(projectMemberships) == 0 {
		return apperr.New(apperr.CodePermissionDenied, "missing_membership")
	}
	if gate.RequiredAgentID != "" {
		for _, mapped := range collaborator.AtlasActors {
			if string(mapped) == gate.RequiredAgentID {
				return nil
			}
		}
		return apperr.New(apperr.CodePermissionDenied, fmt.Sprintf("%s cannot decide gate %s", actor, gate.GateID))
	}
	if gate.RequiredRole == "" {
		return apperr.New(apperr.CodePermissionDenied, fmt.Sprintf("%s cannot decide gate %s", actor, gate.GateID))
	}
	for _, membership := range projectMemberships {
		if membershipRoleMatchesGate(membership.Role, gate.RequiredRole) {
			return nil
		}
	}
	return apperr.New(apperr.CodePermissionDenied, fmt.Sprintf("%s cannot decide %s gate", actor, gate.Kind))
}

func requireChangeMergeAuthority(ctx context.Context, collaborators contracts.CollaboratorStore, memberships contracts.MembershipStore, ticket contracts.TicketSnapshot, actor contracts.Actor) error {
	if actor == contracts.Actor("human:owner") {
		return nil
	}
	if ticket.Reviewer != "" && actor == ticket.Reviewer {
		return nil
	}
	collaborator, bindings, found, err := collaboratorAuthorityContext(ctx, collaborators, memberships, actor)
	if err != nil {
		return err
	}
	if !found {
		return apperr.New(apperr.CodePermissionDenied, "change merge blocked: merge_not_authorized")
	}
	if collaborator.Status != contracts.CollaboratorStatusActive {
		return apperr.New(apperr.CodePermissionDenied, "collaborator_suspended")
	}
	for _, membership := range activeMembershipsForProject(bindings, ticket.Project) {
		switch membership.Role {
		case contracts.MembershipRoleReviewer, contracts.MembershipRoleMaintainer, contracts.MembershipRoleOwner:
			return nil
		}
	}
	return apperr.New(apperr.CodePermissionDenied, "change merge blocked: merge_not_authorized")
}
