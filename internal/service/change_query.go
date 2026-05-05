package service

import (
	"context"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func (s *QueryService) ListChanges(ctx context.Context, ticketID string) ([]contracts.ChangeRef, error) {
	changes, err := s.Changes.ListChanges(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(ticketID) == "" {
		return changes, nil
	}
	ticket, err := s.Tickets.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	return linkedChanges(ticket, changes), nil
}

func (s *QueryService) ChangeDetail(ctx context.Context, changeID string) (ChangeDetailView, error) {
	change, err := s.Changes.LoadChange(ctx, changeID)
	if err != nil {
		return ChangeDetailView{}, err
	}
	ticket, err := s.Tickets.GetTicket(ctx, change.TicketID)
	if err != nil {
		return ChangeDetailView{}, err
	}
	checks, err := s.Checks.ListChecks(ctx, contracts.CheckScopeChange, changeID)
	if err != nil {
		return ChangeDetailView{}, err
	}
	scm, branch, currentBranch, err := changeSCMTarget(ctx, s.Runs, s.Root, change)
	if err != nil {
		return ChangeDetailView{}, err
	}
	changedFiles, err := observedChangedFiles(ctx, scm, change, branch, currentBranch)
	if err != nil {
		return ChangeDetailView{}, err
	}
	return ChangeDetailView{Change: change, Ticket: ticket, Checks: checks, ChangedFiles: changedFiles, GeneratedAt: s.now()}, nil
}

func (s *QueryService) ListChecks(ctx context.Context, scope contracts.CheckScope, scopeID string) ([]contracts.CheckResult, error) {
	checks, err := s.Checks.ListChecks(ctx, scope, scopeID)
	if err != nil {
		return nil, err
	}
	sort.Slice(checks, func(i, j int) bool {
		if checks[i].UpdatedAt.Equal(checks[j].UpdatedAt) {
			return checks[i].CheckID < checks[j].CheckID
		}
		return checks[i].UpdatedAt.Before(checks[j].UpdatedAt)
	})
	return checks, nil
}

func (s *QueryService) CheckDetail(ctx context.Context, checkID string) (contracts.CheckResult, error) {
	return s.Checks.LoadCheck(ctx, checkID)
}

func (s *QueryService) HandoffView(ctx context.Context, handoffID string) (HandoffContextView, error) {
	packet, err := s.Handoffs.LoadHandoff(ctx, handoffID)
	if err != nil {
		return HandoffContextView{}, err
	}
	changes, err := s.Changes.ListChanges(ctx, packet.TicketID)
	if err != nil {
		return HandoffContextView{}, err
	}
	filteredChanges := make([]contracts.ChangeRef, 0, len(changes))
	for _, change := range changes {
		if change.RunID == packet.SourceRunID {
			filteredChanges = append(filteredChanges, change)
		}
	}
	checks, err := s.Checks.ListChecks(ctx, contracts.CheckScopeRun, packet.SourceRunID)
	if err != nil {
		return HandoffContextView{}, err
	}
	return HandoffContextView{Handoff: packet, Changes: filteredChanges, Checks: checks, GeneratedAt: s.now()}, nil
}

func (s *QueryService) ticketChangeContext(ctx context.Context, ticket contracts.TicketSnapshot) ([]contracts.ChangeRef, []contracts.CheckResult, error) {
	changes, err := s.Changes.ListChanges(ctx, ticket.ID)
	if err != nil {
		return nil, nil, err
	}
	changes = linkedChanges(ticket, changes)
	checks, err := s.Checks.ListChecks(ctx, contracts.CheckScopeTicket, ticket.ID)
	if err != nil {
		return nil, nil, err
	}
	for _, change := range changes {
		changeChecks, err := s.Checks.ListChecks(ctx, contracts.CheckScopeChange, change.ChangeID)
		if err != nil {
			return nil, nil, err
		}
		checks = append(checks, changeChecks...)
	}
	sort.Slice(checks, func(i, j int) bool {
		if checks[i].UpdatedAt.Equal(checks[j].UpdatedAt) {
			return checks[i].CheckID < checks[j].CheckID
		}
		return checks[i].UpdatedAt.Before(checks[j].UpdatedAt)
	})
	return changes, checks, nil
}
