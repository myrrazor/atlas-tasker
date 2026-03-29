package service

import (
	"context"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func (s *QueryService) ListCollaborators(ctx context.Context) ([]contracts.CollaboratorProfile, error) {
	return s.Collaborators.ListCollaborators(ctx)
}

func (s *QueryService) CollaboratorDetail(ctx context.Context, collaboratorID string) (CollaboratorDetailView, error) {
	collaborator, err := s.Collaborators.LoadCollaborator(ctx, collaboratorID)
	if err != nil {
		return CollaboratorDetailView{}, err
	}
	memberships, err := s.Memberships.ListMemberships(ctx, collaboratorID)
	if err != nil {
		return CollaboratorDetailView{}, err
	}
	mentions, err := s.Mentions.ListMentions(ctx, collaboratorID)
	if err != nil {
		return CollaboratorDetailView{}, err
	}
	return CollaboratorDetailView{Collaborator: collaborator, Memberships: memberships, Mentions: mentions, GeneratedAt: s.now()}, nil
}

func (s *QueryService) ListMemberships(ctx context.Context, collaboratorID string) ([]contracts.MembershipBinding, error) {
	return s.Memberships.ListMemberships(ctx, collaboratorID)
}

func (s *QueryService) ListMentions(ctx context.Context, collaboratorID string) ([]contracts.Mention, error) {
	return s.Mentions.ListMentions(ctx, collaboratorID)
}

func (s *QueryService) MentionDetail(ctx context.Context, mentionUID string) (MentionDetailView, error) {
	mention, err := s.Mentions.LoadMention(ctx, mentionUID)
	if err != nil {
		return MentionDetailView{}, err
	}
	view := MentionDetailView{Mention: mention, GeneratedAt: s.now()}
	collaborator, err := s.Collaborators.LoadCollaborator(ctx, mention.CollaboratorID)
	if err == nil {
		view.Collaborator = collaborator
	}
	return view, nil
}

func (s *QueryService) collaboratorByID(ctx context.Context, collaboratorID string) (contracts.CollaboratorProfile, []contracts.MembershipBinding, error) {
	collaborator, err := s.Collaborators.LoadCollaborator(ctx, collaboratorID)
	if err != nil {
		return contracts.CollaboratorProfile{}, nil, err
	}
	memberships, err := s.Memberships.ListMemberships(ctx, collaboratorID)
	if err != nil {
		return contracts.CollaboratorProfile{}, nil, err
	}
	return collaborator, memberships, nil
}

func (s *QueryService) collaboratorIDsForGate(ctx context.Context, gate contracts.GateSnapshot, projectKey string) ([]string, error) {
	collaborators, err := s.Collaborators.ListCollaborators(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]string, 0)
	for _, collaborator := range collaborators {
		if collaborator.Status != contracts.CollaboratorStatusActive {
			continue
		}
		memberships, err := s.Memberships.ListMemberships(ctx, collaborator.CollaboratorID)
		if err != nil {
			return nil, err
		}
		if collaboratorMatchesGate(collaborator, memberships, strings.TrimSpace(projectKey), gate) {
			items = append(items, collaborator.CollaboratorID)
		}
	}
	sort.Strings(items)
	return items, nil
}

func collaboratorMatchesGate(collaborator contracts.CollaboratorProfile, memberships []contracts.MembershipBinding, projectKey string, gate contracts.GateSnapshot) bool {
	relevantMemberships := activeMembershipsForProject(memberships, projectKey)
	if len(relevantMemberships) == 0 {
		return false
	}
	if gate.RequiredAgentID != "" {
		for _, actor := range collaborator.AtlasActors {
			if string(actor) == gate.RequiredAgentID {
				return true
			}
		}
	}
	if gate.RequiredRole == "" {
		return false
	}
	for _, membership := range relevantMemberships {
		if membershipRoleMatchesGate(membership.Role, gate.RequiredRole) {
			return true
		}
	}
	return false
}

func membershipRoleMatchesGate(role contracts.MembershipRole, required contracts.AgentRole) bool {
	switch required {
	case contracts.AgentRoleReviewer:
		return role == contracts.MembershipRoleReviewer || role == contracts.MembershipRoleOwner || role == contracts.MembershipRoleMaintainer
	case contracts.AgentRoleOwnerDelegate:
		return role == contracts.MembershipRoleOwner || role == contracts.MembershipRoleMaintainer
	case contracts.AgentRoleQA:
		return role == contracts.MembershipRoleReviewer || role == contracts.MembershipRoleMaintainer
	default:
		return false
	}
}

func (s *QueryService) collaboratorIDsForHandoff(ctx context.Context, handoff contracts.HandoffPacket, projectKey string) ([]string, error) {
	ids := make([]string, 0)
	collaborators, err := s.Collaborators.ListCollaborators(ctx)
	if err != nil {
		return nil, err
	}
	for _, collaborator := range collaborators {
		if collaborator.Status != contracts.CollaboratorStatusActive {
			continue
		}
		memberships, err := s.Memberships.ListMemberships(ctx, collaborator.CollaboratorID)
		if err != nil {
			return nil, err
		}
		if len(activeMembershipsForProject(memberships, projectKey)) == 0 {
			continue
		}
		if strings.TrimSpace(handoff.SuggestedNextActor) != "" {
			for _, actor := range collaborator.AtlasActors {
				if string(actor) == handoff.SuggestedNextActor {
					ids = append(ids, collaborator.CollaboratorID)
					break
				}
			}
		}
	}
	mentions, err := s.Mentions.ListMentions(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, mention := range mentions {
		if mention.SourceKind == "handoff" && mention.SourceID == handoff.HandoffID {
			ids = append(ids, mention.CollaboratorID)
		}
	}
	return uniqueStrings(ids), nil
}

func activeMembershipsForProject(memberships []contracts.MembershipBinding, projectKey string) []contracts.MembershipBinding {
	projectKey = strings.TrimSpace(projectKey)
	items := make([]contracts.MembershipBinding, 0, len(memberships))
	for _, membership := range memberships {
		if membership.Status != contracts.MembershipStatusActive {
			continue
		}
		switch membership.ScopeKind {
		case contracts.MembershipScopeWorkspace:
			items = append(items, membership)
		case contracts.MembershipScopeProject:
			if projectKey != "" && membership.ScopeID == projectKey {
				items = append(items, membership)
			}
		}
	}
	return items
}
