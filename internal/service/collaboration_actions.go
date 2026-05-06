package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

const workspaceEventProject = "_workspace"

func (s *ActionService) AddCollaborator(ctx context.Context, collaborator contracts.CollaboratorProfile, actor contracts.Actor, reason string) (contracts.CollaboratorProfile, error) {
	return withWriteLock(ctx, s.LockManager, "add collaborator", func(ctx context.Context) (contracts.CollaboratorProfile, error) {
		if !actor.IsValid() {
			return contracts.CollaboratorProfile{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		collaborator = normalizeCollaborator(collaborator)
		if _, err := s.Collaborators.LoadCollaborator(ctx, collaborator.CollaboratorID); err == nil {
			return contracts.CollaboratorProfile{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("collaborator %s already exists", collaborator.CollaboratorID))
		}
		now := s.now()
		collaborator.CreatedAt = now
		collaborator.UpdatedAt = now
		event, err := s.newEvent(ctx, workspaceEventProject, now, actor, reason, contracts.EventCollaboratorAdded, "", collaborator)
		if err != nil {
			return contracts.CollaboratorProfile{}, err
		}
		if err := s.commitMutation(ctx, "add collaborator", "collaborator", event, func(ctx context.Context) error {
			return s.Collaborators.SaveCollaborator(ctx, collaborator)
		}); err != nil {
			return contracts.CollaboratorProfile{}, err
		}
		return collaborator, nil
	})
}

func (s *ActionService) EditCollaborator(ctx context.Context, collaboratorID string, displayName string, atlasActors []contracts.Actor, providerHandles map[string]string, actor contracts.Actor, reason string) (contracts.CollaboratorProfile, error) {
	return withWriteLock(ctx, s.LockManager, "edit collaborator", func(ctx context.Context) (contracts.CollaboratorProfile, error) {
		if !actor.IsValid() {
			return contracts.CollaboratorProfile{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		collaborator, err := s.Collaborators.LoadCollaborator(ctx, collaboratorID)
		if err != nil {
			return contracts.CollaboratorProfile{}, err
		}
		if strings.TrimSpace(displayName) != "" {
			collaborator.DisplayName = strings.TrimSpace(displayName)
		}
		if len(atlasActors) > 0 {
			collaborator.AtlasActors = append([]contracts.Actor(nil), atlasActors...)
		}
		if providerHandles != nil {
			clean := make(map[string]string, len(providerHandles))
			for provider, handle := range providerHandles {
				provider = strings.TrimSpace(provider)
				handle = strings.TrimSpace(handle)
				if provider == "" || handle == "" {
					continue
				}
				clean[provider] = handle
			}
			collaborator.ProviderHandles = clean
		}
		collaborator.UpdatedAt = s.now()
		event, err := s.newEvent(ctx, workspaceEventProject, collaborator.UpdatedAt, actor, reason, contracts.EventCollaboratorEdited, "", collaborator)
		if err != nil {
			return contracts.CollaboratorProfile{}, err
		}
		if err := s.commitMutation(ctx, "edit collaborator", "collaborator", event, func(ctx context.Context) error {
			return s.Collaborators.SaveCollaborator(ctx, collaborator)
		}); err != nil {
			return contracts.CollaboratorProfile{}, err
		}
		return collaborator, nil
	})
}

func (s *ActionService) SetCollaboratorTrust(ctx context.Context, collaboratorID string, trusted bool, actor contracts.Actor, reason string) (contracts.CollaboratorProfile, error) {
	return withWriteLock(ctx, s.LockManager, "set collaborator trust", func(ctx context.Context) (contracts.CollaboratorProfile, error) {
		if !actor.IsValid() {
			return contracts.CollaboratorProfile{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		collaborator, err := s.Collaborators.LoadCollaborator(ctx, collaboratorID)
		if err != nil {
			return contracts.CollaboratorProfile{}, err
		}
		next := contracts.CollaboratorTrustStateUntrusted
		if trusted {
			next = contracts.CollaboratorTrustStateTrusted
		}
		if collaborator.TrustState == next {
			return collaborator, nil
		}
		collaborator.TrustState = next
		collaborator.UpdatedAt = s.now()
		event, err := s.newEvent(ctx, workspaceEventProject, collaborator.UpdatedAt, actor, reason, contracts.EventCollaboratorTrusted, "", collaborator)
		if err != nil {
			return contracts.CollaboratorProfile{}, err
		}
		if err := s.commitMutation(ctx, "set collaborator trust", "collaborator", event, func(ctx context.Context) error {
			return s.Collaborators.SaveCollaborator(ctx, collaborator)
		}); err != nil {
			return contracts.CollaboratorProfile{}, err
		}
		return collaborator, nil
	})
}

func (s *ActionService) SetCollaboratorStatus(ctx context.Context, collaboratorID string, status contracts.CollaboratorStatus, actor contracts.Actor, reason string) (contracts.CollaboratorProfile, error) {
	return withWriteLock(ctx, s.LockManager, "set collaborator status", func(ctx context.Context) (contracts.CollaboratorProfile, error) {
		if !actor.IsValid() {
			return contracts.CollaboratorProfile{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if !status.IsValid() {
			return contracts.CollaboratorProfile{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid collaborator status: %s", status))
		}
		collaborator, err := s.Collaborators.LoadCollaborator(ctx, collaboratorID)
		if err != nil {
			return contracts.CollaboratorProfile{}, err
		}
		if collaborator.Status == status {
			return collaborator, nil
		}
		collaborator.Status = status
		collaborator.UpdatedAt = s.now()
		eventType := contracts.EventCollaboratorSuspended
		if status == contracts.CollaboratorStatusRemoved {
			eventType = contracts.EventCollaboratorRemoved
		}
		event, err := s.newEvent(ctx, workspaceEventProject, collaborator.UpdatedAt, actor, reason, eventType, "", collaborator)
		if err != nil {
			return contracts.CollaboratorProfile{}, err
		}
		if err := s.commitMutation(ctx, "set collaborator status", "collaborator", event, func(ctx context.Context) error {
			return s.Collaborators.SaveCollaborator(ctx, collaborator)
		}); err != nil {
			return contracts.CollaboratorProfile{}, err
		}
		return collaborator, nil
	})
}

func (s *ActionService) BindMembership(ctx context.Context, membership contracts.MembershipBinding, actor contracts.Actor, reason string) (contracts.MembershipBinding, error) {
	return withWriteLock(ctx, s.LockManager, "bind membership", func(ctx context.Context) (contracts.MembershipBinding, error) {
		if !actor.IsValid() {
			return contracts.MembershipBinding{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		membership = normalizeMembership(membership)
		if _, err := s.Collaborators.LoadCollaborator(ctx, membership.CollaboratorID); err != nil {
			return contracts.MembershipBinding{}, err
		}
		if membership.ScopeKind == contracts.MembershipScopeProject {
			if _, err := s.Projects.GetProject(ctx, membership.ScopeID); err != nil {
				return contracts.MembershipBinding{}, err
			}
		}
		if membership.ScopeKind == contracts.MembershipScopeWorkspace && strings.TrimSpace(membership.ScopeID) == "" {
			workspaceID, err := ensureWorkspaceIdentity(s.Root)
			if err != nil {
				return contracts.MembershipBinding{}, err
			}
			membership.ScopeID = workspaceID
			membership = normalizeMembership(membership)
		}
		now := s.now()
		if existing, err := s.Memberships.LoadMembership(ctx, membership.MembershipUID); err == nil {
			existing.Status = contracts.MembershipStatusActive
			existing.UpdatedAt = now
			existing.EndedAt = time.Time{}
			existing.DefaultPermissionProfiles = membership.DefaultPermissionProfiles
			membership = normalizeMembership(existing)
		} else {
			membership.CreatedAt = now
			membership.UpdatedAt = now
		}
		event, err := s.newEvent(ctx, membershipEventProject(membership), membership.UpdatedAt, actor, reason, contracts.EventMembershipBound, "", membership)
		if err != nil {
			return contracts.MembershipBinding{}, err
		}
		if err := s.commitMutation(ctx, "bind membership", "membership", event, func(ctx context.Context) error {
			return s.Memberships.SaveMembership(ctx, membership)
		}); err != nil {
			return contracts.MembershipBinding{}, err
		}
		return membership, nil
	})
}

func (s *ActionService) UnbindMembership(ctx context.Context, membershipUID string, actor contracts.Actor, reason string) (contracts.MembershipBinding, error) {
	return withWriteLock(ctx, s.LockManager, "unbind membership", func(ctx context.Context) (contracts.MembershipBinding, error) {
		if !actor.IsValid() {
			return contracts.MembershipBinding{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		membership, err := s.Memberships.LoadMembership(ctx, membershipUID)
		if err != nil {
			return contracts.MembershipBinding{}, err
		}
		if membership.Status == contracts.MembershipStatusUnbound {
			return membership, nil
		}
		now := s.now()
		membership.Status = contracts.MembershipStatusUnbound
		membership.UpdatedAt = now
		membership.EndedAt = now
		event, err := s.newEvent(ctx, membershipEventProject(membership), now, actor, reason, contracts.EventMembershipUnbound, "", membership)
		if err != nil {
			return contracts.MembershipBinding{}, err
		}
		if err := s.commitMutation(ctx, "unbind membership", "membership", event, func(ctx context.Context) error {
			return s.Memberships.SaveMembership(ctx, membership)
		}); err != nil {
			return contracts.MembershipBinding{}, err
		}
		return membership, nil
	})
}

func membershipEventProject(membership contracts.MembershipBinding) string {
	if membership.ScopeKind == contracts.MembershipScopeProject && strings.TrimSpace(membership.ScopeID) != "" {
		return membership.ScopeID
	}
	return workspaceEventProject
}
