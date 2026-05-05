package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type PermissionBindingResult struct {
	Profile     contracts.PermissionProfile `json:"profile"`
	Project     *contracts.Project          `json:"project,omitempty"`
	Ticket      *contracts.TicketSnapshot   `json:"ticket,omitempty"`
	TargetKind  string                      `json:"target_kind"`
	TargetID    string                      `json:"target_id,omitempty"`
	Bound       bool                        `json:"bound"`
	GeneratedAt time.Time                   `json:"generated_at"`
}

func (s *ActionService) SavePermissionProfile(ctx context.Context, profile contracts.PermissionProfile, actor contracts.Actor, reason string) (contracts.PermissionProfile, error) {
	return withWriteLock(ctx, s.LockManager, "save permission profile", func(ctx context.Context) (contracts.PermissionProfile, error) {
		if !actor.IsValid() {
			return contracts.PermissionProfile{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		profile.ProfileID = sanitizePermissionProfileID(profile.ProfileID)
		existing, err := s.PermissionProfiles.LoadPermissionProfile(ctx, profile.ProfileID)
		eventType := contracts.EventPermissionProfileCreated
		if err == nil {
			eventType = contracts.EventPermissionProfileUpdated
			if strings.TrimSpace(profile.DisplayName) == "" {
				profile.DisplayName = existing.DisplayName
			}
		}
		if profile.SchemaVersion == 0 {
			profile.SchemaVersion = contracts.CurrentSchemaVersion
		}
		event, err := s.newEvent(ctx, workspaceProjectKey, s.now(), actor, reason, eventType, "", profile)
		if err != nil {
			return contracts.PermissionProfile{}, err
		}
		if err := s.commitMutation(ctx, "save permission profile", "permission_profile", event, func(ctx context.Context) error {
			return s.PermissionProfiles.SavePermissionProfile(ctx, profile)
		}); err != nil {
			return contracts.PermissionProfile{}, err
		}
		return profile, nil
	})
}

func (s *ActionService) BindPermissionProfile(ctx context.Context, profileID string, targetKind string, targetID string, actor contracts.Actor, reason string) (PermissionBindingResult, error) {
	return s.setPermissionProfileBinding(ctx, profileID, targetKind, targetID, true, actor, reason)
}

func (s *ActionService) UnbindPermissionProfile(ctx context.Context, profileID string, targetKind string, targetID string, actor contracts.Actor, reason string) (PermissionBindingResult, error) {
	return s.setPermissionProfileBinding(ctx, profileID, targetKind, targetID, false, actor, reason)
}

func (s *ActionService) setPermissionProfileBinding(ctx context.Context, profileID string, targetKind string, targetID string, bind bool, actor contracts.Actor, reason string) (PermissionBindingResult, error) {
	return withWriteLock(ctx, s.LockManager, "set permission profile binding", func(ctx context.Context) (PermissionBindingResult, error) {
		if !actor.IsValid() {
			return PermissionBindingResult{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		profile, err := s.PermissionProfiles.LoadPermissionProfile(ctx, profileID)
		if err != nil {
			return PermissionBindingResult{}, err
		}
		result := PermissionBindingResult{Profile: profile, TargetKind: strings.TrimSpace(targetKind), TargetID: strings.TrimSpace(targetID), Bound: bind, GeneratedAt: s.now()}
		eventType := contracts.EventPermissionProfileBound
		if !bind {
			eventType = contracts.EventPermissionProfileUnbound
		}
		payload := map[string]any{"profile": profile, "target_kind": result.TargetKind, "target_id": result.TargetID, "bound": bind}
		switch result.TargetKind {
		case "workspace":
			profile.WorkspaceDefault = bind
			result.Profile = profile
			payload["profile"] = profile
		case "project":
			if strings.TrimSpace(result.TargetID) == "" {
				return PermissionBindingResult{}, apperr.New(apperr.CodeInvalidInput, "project target id is required")
			}
			profile.Projects = toggleStringBinding(profile.Projects, result.TargetID, bind)
			project, err := s.Projects.GetProject(ctx, result.TargetID)
			if err != nil {
				return PermissionBindingResult{}, err
			}
			project.Defaults.PermissionProfiles = toggleStringBinding(project.Defaults.PermissionProfiles, profile.ProfileID, bind)
			result.Project = &project
			payload["project"] = project
			result.Profile = profile
			payload["profile"] = profile
		case "agent":
			if strings.TrimSpace(result.TargetID) == "" {
				return PermissionBindingResult{}, apperr.New(apperr.CodeInvalidInput, "agent target id is required")
			}
			profile.Agents = toggleStringBinding(profile.Agents, result.TargetID, bind)
			result.Profile = profile
			payload["profile"] = profile
		case "runbook":
			if strings.TrimSpace(result.TargetID) == "" {
				return PermissionBindingResult{}, apperr.New(apperr.CodeInvalidInput, "runbook target id is required")
			}
			profile.Runbooks = toggleStringBinding(profile.Runbooks, result.TargetID, bind)
			result.Profile = profile
			payload["profile"] = profile
		case "ticket":
			if strings.TrimSpace(result.TargetID) == "" {
				return PermissionBindingResult{}, apperr.New(apperr.CodeInvalidInput, "ticket target id is required")
			}
			ticket, err := s.Tickets.GetTicket(ctx, result.TargetID)
			if err != nil {
				return PermissionBindingResult{}, err
			}
			ticket.PermissionProfiles = toggleStringBinding(ticket.PermissionProfiles, profile.ProfileID, bind)
			ticket.UpdatedAt = s.now()
			result.Ticket = &ticket
			payload["ticket"] = ticket
		default:
			return PermissionBindingResult{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("unsupported binding target: %s", result.TargetKind))
		}
		eventProject := workspaceProjectKey
		eventTicket := ""
		if result.Project != nil {
			eventProject = result.Project.Key
		}
		if result.Ticket != nil {
			eventProject = result.Ticket.Project
			eventTicket = result.Ticket.ID
		}
		event, err := s.newEvent(ctx, eventProject, s.now(), actor, reason, eventType, eventTicket, payload)
		if err != nil {
			return PermissionBindingResult{}, err
		}
		if err := s.commitMutation(ctx, "set permission profile binding", "permission_profile", event, func(ctx context.Context) error {
			if err := s.PermissionProfiles.SavePermissionProfile(ctx, profile); err != nil {
				return err
			}
			if result.Project != nil {
				return s.UpdateProject(ctx, *result.Project)
			}
			if result.Ticket != nil {
				return s.UpdateTicket(ctx, *result.Ticket)
			}
			return nil
		}); err != nil {
			return PermissionBindingResult{}, err
		}
		return result, nil
	})
}

func (s *QueryService) ListPermissionProfiles(ctx context.Context) ([]contracts.PermissionProfile, error) {
	return s.PermissionProfiles.ListPermissionProfiles(ctx)
}

func (s *QueryService) PermissionProfileDetail(ctx context.Context, profileID string) (contracts.PermissionProfile, error) {
	return s.PermissionProfiles.LoadPermissionProfile(ctx, profileID)
}

func toggleStringBinding(values []string, target string, bind bool) []string {
	target = strings.TrimSpace(target)
	if target == "" {
		return dedupeStrings(values)
	}
	result := make([]string, 0, len(values)+1)
	found := false
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if trimmed == target {
			found = true
			if !bind {
				continue
			}
		}
		result = append(result, trimmed)
	}
	if bind && !found {
		result = append(result, target)
	}
	return dedupeStrings(result)
}
