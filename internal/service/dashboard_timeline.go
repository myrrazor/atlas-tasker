package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func (s *QueryService) Dashboard(ctx context.Context) (DashboardSummaryView, error) {
	tickets, err := s.Tickets.ListTickets(ctx, contracts.TicketListOptions{IncludeArchived: false})
	if err != nil {
		return DashboardSummaryView{}, err
	}
	runs, err := s.Runs.ListRuns(ctx, "")
	if err != nil {
		return DashboardSummaryView{}, err
	}
	gates, err := s.Gates.ListGates(ctx, "")
	if err != nil {
		return DashboardSummaryView{}, err
	}
	worktrees, err := s.WorktreeList(ctx)
	if err != nil {
		return DashboardSummaryView{}, err
	}

	runsByTicket := map[string][]contracts.RunSnapshot{}
	for _, run := range runs {
		runsByTicket[run.TicketID] = append(runsByTicket[run.TicketID], run)
	}
	openGatesByTicket := map[string][]contracts.GateSnapshot{}
	for _, gate := range gates {
		if gate.State == contracts.GateStateOpen {
			openGatesByTicket[gate.TicketID] = append(openGatesByTicket[gate.TicketID], gate)
		}
	}

	view := DashboardSummaryView{GeneratedAt: s.now()}
	for _, run := range runs {
		if run.Status == contracts.RunStatusDispatched || run.Status == contracts.RunStatusAttached || run.Status == contracts.RunStatusActive || run.Status == contracts.RunStatusHandoffReady || run.Status == contracts.RunStatusAwaitingReview || run.Status == contracts.RunStatusAwaitingOwner {
			view.ActiveRuns++
		}
	}
	for _, ticket := range tickets {
		if isAwaitingReviewTicket(ticket, runsByTicket[ticket.ID], openGatesByTicket[ticket.ID]) {
			view.AwaitingReview = appendDashboardBucket(view.AwaitingReview, ticket.ID)
		}
		if isAwaitingOwnerTicket(ticket, runsByTicket[ticket.ID], openGatesByTicket[ticket.ID]) {
			view.AwaitingOwner = appendDashboardBucket(view.AwaitingOwner, ticket.ID)
		}
		if ticket.ChangeReadyState == contracts.ChangeReadyMergeReady {
			view.MergeReady = appendDashboardBucket(view.MergeReady, ticket.ID)
		}
		if ticket.ChangeReadyState == contracts.ChangeReadyChecksPending || ticket.ChangeReadyState == contracts.ChangeReadyChecksFailing {
			view.BlockedByChecks = appendDashboardBucket(view.BlockedByChecks, ticket.ID)
		}
	}
	for _, item := range worktrees {
		if strings.TrimSpace(item.Path) == "" {
			continue
		}
		if !item.Present || item.Dirty {
			view.StaleWorktrees = append(view.StaleWorktrees, item.RunID)
		}
	}
	for _, target := range []contracts.RetentionTarget{contracts.RetentionTargetRuntime, contracts.RetentionTargetEvidenceArtifacts, contracts.RetentionTargetExportBundles, contracts.RetentionTargetLogs} {
		plan, err := s.ArchivePlan(ctx, target, "")
		if err != nil {
			return DashboardSummaryView{}, err
		}
		if len(plan.Items) > 0 {
			view.RetentionTargets = append(view.RetentionTargets, string(target))
		}
	}
	sort.Strings(view.StaleWorktrees)
	sort.Strings(view.RetentionTargets)
	return view, nil
}

func (s *QueryService) Timeline(ctx context.Context, ticketID string) (TimelineView, error) {
	detail, err := s.TicketDetail(ctx, ticketID)
	if err != nil {
		return TimelineView{}, err
	}
	entries := make([]TimelineEntry, 0, len(detail.History))
	for _, event := range detail.History {
		entries = append(entries, TimelineEntry{
			Timestamp: event.Timestamp,
			EventID:   event.EventID,
			Type:      event.Type,
			Actor:     event.Actor,
			TicketID:  event.TicketID,
			Summary:   summarizeTimelineEvent(event),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Timestamp.Equal(entries[j].Timestamp) {
			if entries[i].EventID == entries[j].EventID {
				return entries[i].Type < entries[j].Type
			}
			return entries[i].EventID < entries[j].EventID
		}
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})
	return TimelineView{TicketID: ticketID, GeneratedAt: s.now(), Entries: entries, ChangeReady: detail.Ticket.ChangeReadyState, OpenGateIDs: append([]string{}, detail.Ticket.OpenGateIDs...)}, nil
}

func appendDashboardBucket(bucket DashboardBucket, ticketID string) DashboardBucket {
	bucket.Count++
	if len(bucket.TicketIDs) < 5 {
		bucket.TicketIDs = append(bucket.TicketIDs, ticketID)
	}
	return bucket
}

func isAwaitingReviewTicket(ticket contracts.TicketSnapshot, runs []contracts.RunSnapshot, gates []contracts.GateSnapshot) bool {
	if ticket.Status == contracts.StatusInReview {
		return true
	}
	for _, gate := range gates {
		if gate.Kind == contracts.GateKindReview {
			return true
		}
	}
	for _, run := range runs {
		if run.Status == contracts.RunStatusAwaitingReview {
			return true
		}
	}
	return false
}

func isAwaitingOwnerTicket(ticket contracts.TicketSnapshot, runs []contracts.RunSnapshot, gates []contracts.GateSnapshot) bool {
	for _, gate := range gates {
		if gate.Kind == contracts.GateKindOwner || gate.Kind == contracts.GateKindRelease {
			return true
		}
	}
	for _, run := range runs {
		if run.Status == contracts.RunStatusAwaitingOwner {
			return true
		}
	}
	return false
}

func summarizeTimelineEvent(event contracts.Event) string {
	summary := strings.TrimSpace(event.Reason)
	if summary != "" {
		return summary
	}
	return fmt.Sprintf("%s by %s", event.Type, event.Actor)
}
