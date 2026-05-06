package service

import (
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func aggregateChecks(checks []contracts.CheckResult) contracts.CheckAggregateState {
	if len(checks) == 0 {
		return contracts.CheckAggregateUnknown
	}
	pending := false
	failing := false
	for _, check := range checks {
		if check.Status != contracts.CheckStatusCompleted || check.Conclusion == contracts.CheckConclusionUnknown {
			pending = true
			continue
		}
		switch check.Conclusion {
		case contracts.CheckConclusionFailure, contracts.CheckConclusionTimedOut, contracts.CheckConclusionCancelled:
			failing = true
		}
	}
	if pending {
		return contracts.CheckAggregatePending
	}
	if failing {
		return contracts.CheckAggregateFailing
	}
	return contracts.CheckAggregatePassing
}

// evaluateChangeReadiness stays intentionally narrow. The ticket snapshot stores
// the current readiness rollup, and every surface reads that same rollup back.
func evaluateChangeReadiness(ticket contracts.TicketSnapshot, changes []contracts.ChangeRef, checks []contracts.CheckResult) (contracts.ChangeReadyState, []string) {
	active := linkedChanges(ticket, changes)
	if len(active) == 0 {
		return contracts.ChangeReadyNoLinkedChange, []string{string(contracts.ChangeReadyNoLinkedChange)}
	}
	for _, change := range active {
		switch change.Status {
		case contracts.ChangeStatusLocalOnly, contracts.ChangeStatusDraft:
			return contracts.ChangeReadyLinkedDraft, []string{string(contracts.ChangeReadyLinkedDraft)}
		}
	}
	for _, change := range active {
		if change.Status == contracts.ChangeStatusChangesRequested {
			return contracts.ChangeReadyChangesRequested, []string{string(contracts.ChangeReadyChangesRequested)}
		}
	}
	for _, change := range active {
		switch change.Status {
		case contracts.ChangeStatusOpen, contracts.ChangeStatusReviewRequested, contracts.ChangeStatusExternalDrifted, contracts.ChangeStatusClosed, contracts.ChangeStatusSuperseded:
			return contracts.ChangeReadyReviewPending, []string{string(contracts.ChangeReadyReviewPending)}
		}
	}
	switch aggregateChecks(checks) {
	case contracts.CheckAggregatePending, contracts.CheckAggregateUnknown:
		return contracts.ChangeReadyChecksPending, []string{string(contracts.ChangeReadyChecksPending)}
	case contracts.CheckAggregateFailing:
		return contracts.ChangeReadyChecksFailing, []string{string(contracts.ChangeReadyChecksFailing)}
	}
	if len(ticket.OpenGateIDs) > 0 {
		return contracts.ChangeReadyMergeBlockedByOpenGate, []string{string(contracts.ChangeReadyMergeBlockedByOpenGate)}
	}
	return contracts.ChangeReadyMergeReady, []string{string(contracts.ChangeReadyMergeReady)}
}

func linkedChanges(ticket contracts.TicketSnapshot, changes []contracts.ChangeRef) []contracts.ChangeRef {
	if len(changes) == 0 || len(ticket.ChangeIDs) == 0 {
		return []contracts.ChangeRef{}
	}
	linkedIDs := make(map[string]struct{}, len(ticket.ChangeIDs))
	for _, changeID := range ticket.ChangeIDs {
		if strings.TrimSpace(changeID) == "" {
			continue
		}
		linkedIDs[changeID] = struct{}{}
	}
	filtered := make([]contracts.ChangeRef, 0, len(ticket.ChangeIDs))
	for _, change := range changes {
		if _, ok := linkedIDs[change.ChangeID]; ok {
			filtered = append(filtered, change)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].CreatedAt.Equal(filtered[j].CreatedAt) {
			return filtered[i].ChangeID < filtered[j].ChangeID
		}
		return filtered[i].CreatedAt.Before(filtered[j].CreatedAt)
	})
	return filtered
}

func containsStringID(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
