package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/domain"
)

func (s *ActionService) RunBulk(ctx context.Context, op BulkOperation) (BulkOperationResult, error) {
	normalized, err := s.normalizeBulkOperation(op)
	if err != nil {
		return BulkOperationResult{}, err
	}
	ctx = WithEventMetadata(ctx, EventMetaContext{BatchID: normalized.BatchID})
	if normalized.OverrideDeps {
		ctx = WithDependencyOverride(ctx, normalized.Actor, normalized.Reason)
	}
	result := BulkOperationResult{
		BatchID: normalized.BatchID,
		Preview: BulkPreview{
			Kind:         normalized.Kind,
			Actor:        normalized.Actor,
			Assignee:     normalized.Assignee,
			Status:       normalized.Status,
			TicketIDs:    append([]string{}, normalized.TicketIDs...),
			TicketCount:  len(normalized.TicketIDs),
			DryRun:       normalized.DryRun,
			OverrideDeps: normalized.OverrideDeps,
		},
		Results: make([]BulkTicketResult, 0, len(normalized.TicketIDs)),
	}
	for _, ticketID := range normalized.TicketIDs {
		reason, ticket, previewErr := s.previewBulkTicket(ctx, normalized, ticketID)
		entry := BulkTicketResult{
			TicketID: ticketID,
			DryRun:   normalized.DryRun,
			Reason:   reason,
		}
		if previewErr != nil {
			entry.Code = string(apperr.CodeOf(previewErr))
			entry.Error = previewErr.Error()
			result.Summary.Failed++
			result.Results = append(result.Results, entry)
			continue
		}
		if normalized.DryRun {
			entry.OK = true
			if ticket != nil {
				copy := *ticket
				entry.Ticket = &copy
			}
			result.Summary.Skipped++
			result.Results = append(result.Results, entry)
			continue
		}
		updated, runErr := s.applyBulkTicket(ctx, normalized, ticketID)
		if runErr != nil {
			entry.Code = string(apperr.CodeOf(runErr))
			entry.Error = runErr.Error()
			result.Summary.Failed++
			result.Results = append(result.Results, entry)
			continue
		}
		entry.OK = true
		if updated != nil {
			copy := *updated
			entry.Ticket = &copy
		}
		entry.Reason = appliedBulkReason(normalized, ticketID, updated)
		result.Summary.Succeeded++
		result.Results = append(result.Results, entry)
	}
	result.Summary.Total = len(result.Results)
	return result, nil
}

func appliedBulkReason(op BulkOperation, ticketID string, ticket *contracts.TicketSnapshot) string {
	switch op.Kind {
	case BulkOperationMove:
		if ticket != nil {
			return fmt.Sprintf("moved %s to %s", ticket.ID, ticket.Status)
		}
		return fmt.Sprintf("moved %s", ticketID)
	case BulkOperationAssign:
		return fmt.Sprintf("updated %s assignee to %s", ticketID, op.Assignee)
	case BulkOperationRequestReview:
		return fmt.Sprintf("updated %s to in_review", ticketID)
	case BulkOperationComplete:
		return fmt.Sprintf("completed %s", ticketID)
	case BulkOperationClaim:
		return fmt.Sprintf("updated %s lease", ticketID)
	case BulkOperationRelease:
		return fmt.Sprintf("updated %s lease", ticketID)
	default:
		return fmt.Sprintf("updated %s", ticketID)
	}
}

func (s *ActionService) normalizeBulkOperation(op BulkOperation) (BulkOperation, error) {
	op.Kind = BulkOperationKind(strings.TrimSpace(string(op.Kind)))
	if op.Kind == "" {
		return BulkOperation{}, apperr.New(apperr.CodeInvalidInput, "bulk operation kind is required")
	}
	if !op.Actor.IsValid() {
		return BulkOperation{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", op.Actor))
	}
	switch op.Kind {
	case BulkOperationMove:
		if !op.Status.IsValid() {
			return BulkOperation{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid status: %s", op.Status))
		}
	case BulkOperationAssign:
		if op.Assignee != "" && !op.Assignee.IsValid() {
			return BulkOperation{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid assignee actor: %s", op.Assignee))
		}
	case BulkOperationRequestReview, BulkOperationComplete, BulkOperationClaim, BulkOperationRelease:
	default:
		return BulkOperation{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("unsupported bulk operation: %s", op.Kind))
	}
	op.TicketIDs = uniqueTicketIDs(op.TicketIDs)
	if len(op.TicketIDs) == 0 {
		return BulkOperation{}, apperr.New(apperr.CodeInvalidInput, "bulk operations require at least one ticket")
	}
	if !op.DryRun && !op.Confirm {
		return BulkOperation{}, apperr.New(apperr.CodeInvalidInput, "bulk mutations require --yes or --dry-run")
	}
	if op.OverrideDeps {
		if !bulkOperationCanOverrideDependencies(op) {
			return BulkOperation{}, apperr.New(apperr.CodeInvalidInput, "--override-deps only applies to unsafe bulk move, request-review, and complete operations")
		}
		if op.Actor != contracts.Actor("human:owner") {
			return BulkOperation{}, apperr.New(apperr.CodePermissionDenied, "dependency_override_requires_owner")
		}
		if strings.TrimSpace(op.Reason) == "" {
			return BulkOperation{}, apperr.New(apperr.CodeInvalidInput, "dependency_override_requires_reason")
		}
	}
	if strings.TrimSpace(op.BatchID) == "" {
		op.BatchID = NewOpaqueID()
	}
	return op, nil
}

func bulkOperationCanOverrideDependencies(op BulkOperation) bool {
	switch op.Kind {
	case BulkOperationRequestReview, BulkOperationComplete:
		return true
	case BulkOperationMove:
		return unsafeDependencyProgress(op.Status)
	default:
		return false
	}
}

func uniqueTicketIDs(ticketIDs []string) []string {
	seen := make(map[string]struct{}, len(ticketIDs))
	normalized := make([]string, 0, len(ticketIDs))
	for _, ticketID := range ticketIDs {
		trimmed := strings.TrimSpace(ticketID)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	return normalized
}

func (s *ActionService) previewBulkTicket(ctx context.Context, op BulkOperation, ticketID string) (string, *contracts.TicketSnapshot, error) {
	ticket, err := s.Tickets.GetTicket(ctx, ticketID)
	if err != nil {
		return "", nil, err
	}
	switch op.Kind {
	case BulkOperationMove:
		if op.Status == contracts.StatusDone {
			if err := s.requireNoUnresolvedDependencies(ctx, ticket); err != nil {
				return "", nil, err
			}
			if ticket.Status != contracts.StatusInReview {
				return "", nil, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("ticket %s must be in_review to complete", ticket.ID))
			}
			policy, err := resolveEffectivePolicy(ctx, s.Root, s.Projects, s.Tickets, ticket)
			if err != nil {
				return "", nil, err
			}
			if ticket.ReviewState != contracts.ReviewStateApproved {
				return "", nil, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("ticket %s must be approved before completion", ticket.ID))
			}
			if err := domain.CheckCompletionPermission(policy.CompletionMode, op.Actor, effectiveReviewer(ticket, policy)); err != nil {
				return "", nil, &apperr.Error{Code: apperr.CodePermissionDenied, Message: err.Error(), Cause: err}
			}
			return fmt.Sprintf("would complete %s", ticket.ID), &ticket, nil
		}
		if unsafeDependencyProgress(op.Status) {
			if err := s.requireNoUnresolvedDependencies(ctx, ticket); err != nil {
				return "", nil, err
			}
		}
		if err := domain.ValidateTransition(ticket.Status, op.Status); err != nil {
			return "", nil, &apperr.Error{Code: apperr.CodeInvalidInput, Message: err.Error(), Cause: err}
		}
		return fmt.Sprintf("would move %s from %s to %s", ticket.ID, ticket.Status, op.Status), &ticket, nil
	case BulkOperationAssign:
		return fmt.Sprintf("would assign %s to %s", ticket.ID, op.Assignee), &ticket, nil
	case BulkOperationRequestReview:
		if err := s.requireNoUnresolvedDependencies(ctx, ticket); err != nil {
			return "", nil, err
		}
		if err := domain.ValidateTransition(ticket.Status, contracts.StatusInReview); err != nil {
			return "", nil, &apperr.Error{Code: apperr.CodeInvalidInput, Message: err.Error(), Cause: err}
		}
		return fmt.Sprintf("would request review for %s", ticket.ID), &ticket, nil
	case BulkOperationComplete:
		if err := s.requireNoUnresolvedDependencies(ctx, ticket); err != nil {
			return "", nil, err
		}
		policy, err := resolveEffectivePolicy(ctx, s.Root, s.Projects, s.Tickets, ticket)
		if err != nil {
			return "", nil, err
		}
		if ticket.Status != contracts.StatusInReview {
			return "", nil, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("ticket %s must be in_review to complete", ticket.ID))
		}
		if ticket.ReviewState != contracts.ReviewStateApproved {
			return "", nil, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("ticket %s must be approved before completion", ticket.ID))
		}
		if err := domain.CheckCompletionPermission(policy.CompletionMode, op.Actor, effectiveReviewer(ticket, policy)); err != nil {
			return "", nil, &apperr.Error{Code: apperr.CodePermissionDenied, Message: err.Error(), Cause: err}
		}
		return fmt.Sprintf("would complete %s", ticket.ID), &ticket, nil
	case BulkOperationClaim:
		if ticket.Lease.Actor != "" && ticket.Lease.Active(s.now()) && ticket.Lease.Actor != op.Actor {
			return "", nil, apperr.New(apperr.CodeConflict, fmt.Sprintf("ticket %s is already claimed by %s", ticket.ID, ticket.Lease.Actor))
		}
		policy, err := resolveEffectivePolicy(ctx, s.Root, s.Projects, s.Tickets, ticket)
		if err != nil {
			return "", nil, err
		}
		if len(policy.AllowedWorkers) > 0 && !actorInList(op.Actor, policy.AllowedWorkers) && op.Actor != contracts.Actor("human:owner") {
			return "", nil, apperr.New(apperr.CodePermissionDenied, fmt.Sprintf("actor %s is not allowed by effective policy", op.Actor))
		}
		if ticket.Status == contracts.StatusInReview && op.Actor != effectiveReviewer(ticket, policy) && op.Actor != contracts.Actor("human:owner") {
			return "", nil, apperr.New(apperr.CodePermissionDenied, "review claims must belong to the reviewer or owner")
		}
		return fmt.Sprintf("would claim %s", ticket.ID), &ticket, nil
	case BulkOperationRelease:
		if ticket.Lease.Actor == "" {
			return "", nil, apperr.New(apperr.CodeConflict, fmt.Sprintf("ticket %s is not claimed", ticket.ID))
		}
		if ticket.Lease.Actor != op.Actor && op.Actor != contracts.Actor("human:owner") {
			return "", nil, apperr.New(apperr.CodePermissionDenied, fmt.Sprintf("ticket %s is claimed by %s", ticket.ID, ticket.Lease.Actor))
		}
		return fmt.Sprintf("would release %s", ticket.ID), &ticket, nil
	default:
		return "", nil, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("unsupported bulk operation: %s", op.Kind))
	}
}

func (s *ActionService) applyBulkTicket(ctx context.Context, op BulkOperation, ticketID string) (*contracts.TicketSnapshot, error) {
	switch op.Kind {
	case BulkOperationMove:
		ticket, err := s.MoveTicket(ctx, ticketID, op.Status, op.Actor, op.Reason)
		if err != nil {
			return nil, err
		}
		return &ticket, nil
	case BulkOperationAssign:
		ticket, err := s.AssignTicket(ctx, ticketID, op.Assignee, op.Actor, op.Reason)
		if err != nil {
			return nil, err
		}
		return &ticket, nil
	case BulkOperationRequestReview:
		ticket, err := s.RequestReview(ctx, ticketID, op.Actor, op.Reason)
		if err != nil {
			return nil, err
		}
		return &ticket, nil
	case BulkOperationComplete:
		ticket, err := s.CompleteTicket(ctx, ticketID, op.Actor, op.Reason)
		if err != nil {
			return nil, err
		}
		return &ticket, nil
	case BulkOperationClaim:
		ticket, err := s.ClaimTicket(ctx, ticketID, op.Actor, op.Reason)
		if err != nil {
			return nil, err
		}
		return &ticket, nil
	case BulkOperationRelease:
		ticket, err := s.ReleaseTicket(ctx, ticketID, op.Actor, op.Reason)
		if err != nil {
			return nil, err
		}
		return &ticket, nil
	default:
		return nil, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("unsupported bulk operation: %s", op.Kind))
	}
}
