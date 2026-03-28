package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type changeUnlinkResult struct {
	Ticket contracts.TicketSnapshot
	Change contracts.ChangeRef
}

func (s *ActionService) LinkChange(ctx context.Context, ticketID string, change contracts.ChangeRef, actor contracts.Actor, reason string) (contracts.ChangeRef, error) {
	return withWriteLock(ctx, s.LockManager, "link change", func(ctx context.Context) (contracts.ChangeRef, error) {
		if !actor.IsValid() {
			return contracts.ChangeRef{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		ticket, err := s.Tickets.GetTicket(ctx, ticketID)
		if err != nil {
			return contracts.ChangeRef{}, err
		}
		change = normalizeChangeRef(change)
		if strings.TrimSpace(change.ChangeID) == "" {
			change.ChangeID = "change_" + NewOpaqueID()
		}
		if existing, err := s.Changes.LoadChange(ctx, change.ChangeID); err == nil {
			if strings.TrimSpace(change.TicketID) == "" {
				change.TicketID = existing.TicketID
			}
			if change.RunID == "" {
				change.RunID = existing.RunID
			}
			if change.BranchName == "" {
				change.BranchName = existing.BranchName
			}
			if change.BaseBranch == "" {
				change.BaseBranch = existing.BaseBranch
			}
			if change.HeadRef == "" {
				change.HeadRef = existing.HeadRef
			}
			if change.URL == "" {
				change.URL = existing.URL
			}
			if change.ExternalID == "" {
				change.ExternalID = existing.ExternalID
			}
			if strings.TrimSpace(change.ReviewSummary) == "" {
				change.ReviewSummary = existing.ReviewSummary
			}
			if len(change.ReviewRequestedFrom) == 0 {
				change.ReviewRequestedFrom = existing.ReviewRequestedFrom
			}
			change.CreatedAt = existing.CreatedAt
		}
		change.TicketID = ticket.ID
		if strings.TrimSpace(change.RunID) != "" {
			run, err := s.Runs.LoadRun(ctx, change.RunID)
			if err != nil {
				return contracts.ChangeRef{}, err
			}
			if run.TicketID != ticket.ID {
				return contracts.ChangeRef{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("run %s does not belong to ticket %s", run.RunID, ticket.ID))
			}
			if change.BranchName == "" {
				change.BranchName = run.BranchName
			}
		}
		if err := change.Validate(); err != nil {
			return contracts.ChangeRef{}, err
		}
		now := s.now()
		change.UpdatedAt = now
		if !containsStringID(ticket.ChangeIDs, change.ChangeID) {
			ticket.ChangeIDs = append(ticket.ChangeIDs, change.ChangeID)
		}
		ticket.UpdatedAt = now
		if err := s.previewTicketChangeState(ctx, &ticket, &change, nil); err != nil {
			return contracts.ChangeRef{}, err
		}
		event, err := s.newEvent(ctx, ticket.Project, now, actor, reason, contracts.EventChangeLinked, ticket.ID, map[string]any{
			"ticket": ticket,
			"change": change,
		})
		if err != nil {
			return contracts.ChangeRef{}, err
		}
		if err := s.commitMutation(ctx, "link change", "change_ref", event, func(ctx context.Context) error {
			if err := s.Changes.SaveChange(ctx, change); err != nil {
				return err
			}
			return s.UpdateTicket(ctx, ticket)
		}); err != nil {
			return contracts.ChangeRef{}, err
		}
		return change, nil
	})
}

func (s *ActionService) UnlinkChange(ctx context.Context, ticketID string, changeID string, actor contracts.Actor, reason string) (contracts.TicketSnapshot, contracts.ChangeRef, error) {
	result, err := withWriteLock(ctx, s.LockManager, "unlink change", func(ctx context.Context) (changeUnlinkResult, error) {
		if !actor.IsValid() {
			return changeUnlinkResult{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		ticket, err := s.Tickets.GetTicket(ctx, ticketID)
		if err != nil {
			return changeUnlinkResult{}, err
		}
		change, err := s.Changes.LoadChange(ctx, changeID)
		if err != nil {
			return changeUnlinkResult{}, err
		}
		filtered := make([]string, 0, len(ticket.ChangeIDs))
		for _, linkedID := range ticket.ChangeIDs {
			if linkedID != changeID {
				filtered = append(filtered, linkedID)
			}
		}
		if len(filtered) == len(ticket.ChangeIDs) {
			return changeUnlinkResult{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("change %s is not linked to %s", changeID, ticketID))
		}
		now := s.now()
		ticket.ChangeIDs = filtered
		ticket.UpdatedAt = now
		if err := s.refreshTicketChangeState(ctx, &ticket); err != nil {
			return changeUnlinkResult{}, err
		}
		event, err := s.newEvent(ctx, ticket.Project, now, actor, reason, contracts.EventChangeUnlinked, ticket.ID, map[string]any{
			"ticket":    ticket,
			"change":    change,
			"change_id": changeID,
		})
		if err != nil {
			return changeUnlinkResult{}, err
		}
		if err := s.commitMutation(ctx, "unlink change", "ticket_snapshot", event, func(ctx context.Context) error {
			return s.UpdateTicket(ctx, ticket)
		}); err != nil {
			return changeUnlinkResult{}, err
		}
		return changeUnlinkResult{Ticket: ticket, Change: change}, nil
	})
	if err != nil {
		return contracts.TicketSnapshot{}, contracts.ChangeRef{}, err
	}
	return result.Ticket, result.Change, nil
}

func (s *ActionService) RecordCheck(ctx context.Context, check contracts.CheckResult, actor contracts.Actor, reason string) (contracts.CheckResult, error) {
	return withWriteLock(ctx, s.LockManager, "record check", func(ctx context.Context) (contracts.CheckResult, error) {
		if !actor.IsValid() {
			return contracts.CheckResult{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		check = normalizeCheckResult(check)
		if strings.TrimSpace(check.CheckID) == "" {
			check.CheckID = "check_" + NewOpaqueID()
		}
		if existing, err := s.Checks.LoadCheck(ctx, check.CheckID); err == nil {
			if check.Provider == "" {
				check.Provider = existing.Provider
			}
			if check.ExternalID == "" {
				check.ExternalID = existing.ExternalID
			}
			if check.URL == "" {
				check.URL = existing.URL
			}
			if check.Summary == "" {
				check.Summary = existing.Summary
			}
			if check.StartedAt.IsZero() {
				check.StartedAt = existing.StartedAt
			}
		}
		if check.Status == contracts.CheckStatusRunning && check.StartedAt.IsZero() {
			check.StartedAt = s.now()
		}
		if check.Status == contracts.CheckStatusCompleted {
			if check.StartedAt.IsZero() {
				check.StartedAt = s.now()
			}
			if check.CompletedAt.IsZero() {
				check.CompletedAt = s.now()
			}
		}
		if err := check.Validate(); err != nil {
			return contracts.CheckResult{}, err
		}
		var (
			ticket     contracts.TicketSnapshot
			change     contracts.ChangeRef
			run        contracts.RunSnapshot
			err        error
			ticketSeen bool
			changeSeen bool
			runSeen    bool
		)
		switch check.Scope {
		case contracts.CheckScopeTicket:
			ticket, err = s.Tickets.GetTicket(ctx, check.ScopeID)
			if err != nil {
				return contracts.CheckResult{}, err
			}
			ticketSeen = true
		case contracts.CheckScopeChange:
			change, err = s.Changes.LoadChange(ctx, check.ScopeID)
			if err != nil {
				return contracts.CheckResult{}, err
			}
			changeSeen = true
			if check.Provider == "" {
				check.Provider = change.Provider
			}
			ticket, err = s.Tickets.GetTicket(ctx, change.TicketID)
			if err != nil {
				return contracts.CheckResult{}, err
			}
			ticketSeen = true
		case contracts.CheckScopeRun:
			run, err = s.Runs.LoadRun(ctx, check.ScopeID)
			if err != nil {
				return contracts.CheckResult{}, err
			}
			runSeen = true
		default:
			return contracts.CheckResult{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid check scope: %s", check.Scope))
		}
		now := s.now()
		check.UpdatedAt = now
		eventType := contracts.EventCheckRecorded
		if _, err := s.Checks.LoadCheck(ctx, check.CheckID); err == nil {
			eventType = contracts.EventCheckUpdated
		}
		if ticketSeen {
			ticket.UpdatedAt = now
			if changeSeen {
				change.UpdatedAt = now
			}
			if err := s.previewTicketChangeState(ctx, &ticket, func() *contracts.ChangeRef {
				if changeSeen {
					return &change
				}
				return nil
			}(), &check); err != nil {
				return contracts.CheckResult{}, err
			}
		}
		if changeSeen {
			changeChecks, err := s.checksForScopeWithPending(ctx, contracts.CheckScopeChange, change.ChangeID, &check)
			if err != nil {
				return contracts.CheckResult{}, err
			}
			change.ChecksStatus = aggregateChecks(changeChecks)
			change.UpdatedAt = now
		}
		payload := map[string]any{"check": check}
		if ticketSeen {
			payload["ticket"] = ticket
		}
		if changeSeen {
			payload["change"] = change
		}
		if runSeen {
			payload["run"] = run
		}
		project := workspaceProjectKey
		ticketIDForEvent := ""
		if ticketSeen {
			project = ticket.Project
			ticketIDForEvent = ticket.ID
		} else if runSeen {
			project = run.Project
			ticketIDForEvent = run.TicketID
		}
		event, err := s.newEvent(ctx, project, now, actor, reason, eventType, ticketIDForEvent, payload)
		if err != nil {
			return contracts.CheckResult{}, err
		}
		if err := s.commitMutation(ctx, "record check", "check_result", event, func(ctx context.Context) error {
			if err := s.Checks.SaveCheck(ctx, check); err != nil {
				return err
			}
			if changeSeen {
				if err := s.Changes.SaveChange(ctx, change); err != nil {
					return err
				}
			}
			if ticketSeen {
				if err := s.UpdateTicket(ctx, ticket); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return contracts.CheckResult{}, err
		}
		return check, nil
	})
}

func (s *ActionService) refreshTicketChangeState(ctx context.Context, ticket *contracts.TicketSnapshot) error {
	return s.previewTicketChangeState(ctx, ticket, nil, nil)
}

func (s *ActionService) previewTicketChangeState(ctx context.Context, ticket *contracts.TicketSnapshot, pendingChange *contracts.ChangeRef, pendingCheck *contracts.CheckResult) error {
	changes, err := s.Changes.ListChanges(ctx, ticket.ID)
	if err != nil {
		return err
	}
	if pendingChange != nil {
		replaced := false
		for i := range changes {
			if changes[i].ChangeID == pendingChange.ChangeID {
				changes[i] = *pendingChange
				replaced = true
				break
			}
		}
		if !replaced {
			changes = append(changes, *pendingChange)
		}
	}
	linked := linkedChanges(*ticket, changes)
	relevantChecks, err := s.checksForScopeWithPending(ctx, contracts.CheckScopeTicket, ticket.ID, pendingCheck)
	if err != nil {
		return err
	}
	for _, change := range linked {
		changeChecks, err := s.checksForScopeWithPending(ctx, contracts.CheckScopeChange, change.ChangeID, pendingCheck)
		if err != nil {
			return err
		}
		relevantChecks = append(relevantChecks, changeChecks...)
	}
	ticket.ChangeReadyState, ticket.ChangeReadyReasons = evaluateChangeReadiness(*ticket, changes, relevantChecks)
	return nil
}

func (s *ActionService) checksForScopeWithPending(ctx context.Context, scope contracts.CheckScope, scopeID string, pending *contracts.CheckResult) ([]contracts.CheckResult, error) {
	checks, err := s.Checks.ListChecks(ctx, scope, scopeID)
	if err != nil {
		return nil, err
	}
	if pending == nil || pending.Scope != scope || pending.ScopeID != scopeID {
		return checks, nil
	}
	replaced := false
	for i := range checks {
		if checks[i].CheckID == pending.CheckID {
			checks[i] = *pending
			replaced = true
			break
		}
	}
	if !replaced {
		checks = append(checks, *pending)
	}
	return checks, nil
}
