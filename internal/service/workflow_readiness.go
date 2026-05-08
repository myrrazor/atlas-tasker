package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

const dependencyBlockedReason = "dependency_blocked"

func effectiveReviewer(ticket contracts.TicketSnapshot, policy EffectivePolicyView) contracts.Actor {
	if ticket.Reviewer != "" {
		return ticket.Reviewer
	}
	return policy.RequiredReviewer
}

func validateRequestedReviewer(ticket contracts.TicketSnapshot, policy EffectivePolicyView, reviewer contracts.Actor) error {
	if reviewer == "" {
		return nil
	}
	if !reviewer.IsValid() {
		return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid reviewer actor: %s", reviewer))
	}
	if policy.RequiredReviewer != "" && reviewer != policy.RequiredReviewer {
		return apperr.New(apperr.CodePermissionDenied, fmt.Sprintf("reviewer_policy_mismatch: effective policy requires %s", policy.RequiredReviewer))
	}
	return nil
}

func unsafeDependencyProgress(status contracts.Status) bool {
	switch status {
	case contracts.StatusInProgress, contracts.StatusInReview, contracts.StatusDone:
		return true
	default:
		return false
	}
}

func boardStatusForDependencies(ticket contracts.TicketSnapshot, hasUnresolvedBlocker bool) contracts.Status {
	if contracts.IsTerminalStatus(ticket.Status) {
		return contracts.StatusDone
	}
	if ticket.Status == contracts.StatusBlocked || hasUnresolvedBlocker {
		return contracts.StatusBlocked
	}
	return ticket.Status
}

func unresolvedBlockersFromStore(ctx context.Context, tickets contracts.TicketStore, ticket contracts.TicketSnapshot) ([]string, error) {
	unresolved := make([]string, 0, len(ticket.BlockedBy))
	for _, blockerID := range ticket.BlockedBy {
		blockerID = strings.TrimSpace(blockerID)
		if blockerID == "" {
			continue
		}
		blocker, err := tickets.GetTicket(ctx, blockerID)
		if err != nil {
			unresolved = append(unresolved, blockerID)
			continue
		}
		if blocker.Status != contracts.StatusDone {
			unresolved = append(unresolved, blocker.ID)
		}
	}
	return unresolved, nil
}

func dependencyBlockedError(ticket contracts.TicketSnapshot, blockers []string) error {
	return apperr.New(apperr.CodeConflict, fmt.Sprintf("%s: ticket %s is blocked by unresolved tickets: %s", dependencyBlockedReason, ticket.ID, strings.Join(blockers, ", ")))
}

func (s *ActionService) requireNoUnresolvedDependencies(ctx context.Context, ticket contracts.TicketSnapshot) error {
	blockers, err := unresolvedBlockersFromStore(ctx, s.Tickets, ticket)
	if err != nil {
		return err
	}
	if len(blockers) > 0 {
		return dependencyBlockedError(ticket, blockers)
	}
	return nil
}

func (s *QueryService) BoardStatus(ctx context.Context, ticket contracts.TicketSnapshot) (contracts.Status, error) {
	blockers, err := unresolvedBlockersFromStore(ctx, s.Tickets, ticket)
	if err != nil {
		return "", err
	}
	return boardStatusForDependencies(ticket, len(blockers) > 0), nil
}
