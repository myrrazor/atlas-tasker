package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func (s *QueryService) GateList(ctx context.Context, ticketID string, runID string, state contracts.GateState) ([]contracts.GateSnapshot, error) {
	gates, err := s.Gates.ListGates(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	items := make([]contracts.GateSnapshot, 0, len(gates))
	for _, gate := range gates {
		if strings.TrimSpace(runID) != "" && gate.RunID != runID {
			continue
		}
		if state != "" && gate.State != state {
			continue
		}
		items = append(items, gate)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].GateID < items[j].GateID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items, nil
}

func (s *QueryService) GateDetail(ctx context.Context, gateID string) (contracts.GateSnapshot, error) {
	return s.Gates.LoadGate(ctx, gateID)
}

func (s *QueryService) Approvals(ctx context.Context, collaboratorID string) ([]ApprovalItemView, error) {
	gates, err := s.GateList(ctx, "", "", contracts.GateStateOpen)
	if err != nil {
		return nil, err
	}
	filterID := strings.TrimSpace(collaboratorID)
	items := make([]ApprovalItemView, 0, len(gates))
	for _, gate := range gates {
		ticket, err := s.Tickets.GetTicket(ctx, gate.TicketID)
		if err != nil {
			return nil, err
		}
		collaboratorIDs, err := s.collaboratorIDsForGate(ctx, gate, ticket.Project)
		if err != nil {
			return nil, err
		}
		if filterID != "" && !containsString(collaboratorIDs, filterID) {
			continue
		}
		items = append(items, ApprovalItemView{
			Gate:            gate,
			Ticket:          ticket,
			CollaboratorIDs: collaboratorIDs,
			Summary:         summarizeGate(gate, ticket),
			GeneratedAt:     gate.CreatedAt,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].GeneratedAt.Equal(items[j].GeneratedAt) {
			return items[i].Gate.GateID < items[j].Gate.GateID
		}
		return items[i].GeneratedAt.Before(items[j].GeneratedAt)
	})
	return items, nil
}

func (s *QueryService) Inbox(ctx context.Context, collaboratorID string) ([]InboxItemView, error) {
	items := make([]InboxItemView, 0)
	filterID := strings.TrimSpace(collaboratorID)
	approvals, err := s.Approvals(ctx, filterID)
	if err != nil {
		return nil, err
	}
	for _, approval := range approvals {
		items = append(items, InboxItemView{
			ID:              "gate:" + approval.Gate.GateID,
			Kind:            "gate",
			TicketID:        approval.Gate.TicketID,
			RunID:           approval.Gate.RunID,
			GateID:          approval.Gate.GateID,
			CollaboratorIDs: approval.CollaboratorIDs,
			Summary:         approval.Summary,
			State:           string(approval.Gate.State),
			Provenance:      "local",
			GeneratedAt:     approval.GeneratedAt,
		})
	}
	runs, err := s.ListRuns(ctx, "", "", "")
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		if run.Status != contracts.RunStatusHandoffReady {
			continue
		}
		handoff, ok, err := s.latestHandoffForRun(ctx, run)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		collaboratorIDs, err := s.collaboratorIDsForHandoff(ctx, handoff, run.Project)
		if err != nil {
			return nil, err
		}
		if filterID != "" && !containsString(collaboratorIDs, filterID) {
			continue
		}
		items = append(items, InboxItemView{
			ID:              "handoff:" + handoff.HandoffID,
			Kind:            "handoff",
			TicketID:        run.TicketID,
			RunID:           run.RunID,
			HandoffID:       handoff.HandoffID,
			CollaboratorIDs: collaboratorIDs,
			Summary:         fmt.Sprintf("%s is ready for handoff on %s", run.RunID, run.TicketID),
			State:           string(run.Status),
			Provenance:      "local",
			GeneratedAt:     handoff.GeneratedAt,
		})
	}
	mentions, err := s.Mentions.ListMentions(ctx, filterID)
	if err != nil {
		return nil, err
	}
	for _, mention := range mentions {
		items = append(items, InboxItemView{
			ID:              "mention:" + mention.MentionUID,
			Kind:            "mention",
			TicketID:        mention.TicketID,
			MentionUID:      mention.MentionUID,
			CollaboratorIDs: []string{mention.CollaboratorID},
			Summary:         fmt.Sprintf("@%s mentioned in %s %s", mention.CollaboratorID, mention.SourceKind, mention.SourceID),
			State:           "open",
			Provenance:      "local",
			GeneratedAt:     mention.CreatedAt,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].GeneratedAt.Equal(items[j].GeneratedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].GeneratedAt.After(items[j].GeneratedAt)
	})
	return items, nil
}

func (s *QueryService) InboxDetail(ctx context.Context, itemID string) (InboxDetailView, error) {
	items, err := s.Inbox(ctx, "")
	if err != nil {
		return InboxDetailView{}, err
	}
	for _, item := range items {
		if item.ID != itemID {
			continue
		}
		detail := InboxDetailView{Item: item, Generated: s.now()}
		if item.TicketID != "" {
			ticket, err := s.Tickets.GetTicket(ctx, item.TicketID)
			if err == nil {
				detail.Ticket = ticket
			}
		}
		if item.RunID != "" {
			run, err := s.Runs.LoadRun(ctx, item.RunID)
			if err == nil {
				detail.Run = run
			}
		}
		if item.GateID != "" {
			gate, err := s.Gates.LoadGate(ctx, item.GateID)
			if err == nil {
				detail.Gate = gate
			}
		}
		if item.HandoffID != "" {
			handoff, err := s.Handoffs.LoadHandoff(ctx, item.HandoffID)
			if err == nil {
				detail.Handoff = handoff
			}
		}
		if item.MentionUID != "" {
			mention, err := s.Mentions.LoadMention(ctx, item.MentionUID)
			if err == nil {
				detail.Mention = mention
			}
		}
		return detail, nil
	}
	return InboxDetailView{}, fmt.Errorf("inbox item %s not found", itemID)
}

func (s *QueryService) latestHandoffForRun(ctx context.Context, run contracts.RunSnapshot) (contracts.HandoffPacket, bool, error) {
	handoffs, err := s.Handoffs.ListHandoffs(ctx, run.TicketID)
	if err != nil {
		return contracts.HandoffPacket{}, false, err
	}
	var latest contracts.HandoffPacket
	found := false
	for _, handoff := range handoffs {
		if handoff.SourceRunID != run.RunID {
			continue
		}
		if !found || handoff.GeneratedAt.After(latest.GeneratedAt) {
			latest = handoff
			found = true
		}
	}
	return latest, found, nil
}

func summarizeGate(gate contracts.GateSnapshot, ticket contracts.TicketSnapshot) string {
	summary := fmt.Sprintf("%s gate open for %s", gate.Kind, ticket.ID)
	if gate.RequiredAgentID != "" {
		summary += " -> " + gate.RequiredAgentID
	} else if gate.RequiredRole != "" {
		summary += " -> " + string(gate.RequiredRole)
	}
	return summary
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
