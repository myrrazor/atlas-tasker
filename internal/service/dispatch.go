package service

import (
	"context"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func (s *QueryService) DispatchSuggest(ctx context.Context, ticketID string) (DispatchSuggestion, error) {
	ticket, err := s.Tickets.GetTicket(ctx, ticketID)
	if err != nil {
		return DispatchSuggestion{}, err
	}
	report, err := s.AgentEligibility(ctx, ticketID)
	if err != nil {
		return DispatchSuggestion{}, err
	}
	commonReasons, err := s.dispatchBlockers(ctx, ticket)
	if err != nil {
		return DispatchSuggestion{}, err
	}
	eligibleCount := 0
	autoRoute := ""
	for i := range report.Entries {
		for _, code := range commonReasons {
			report.Entries[i].Eligible = false
			report.Entries[i].ReasonCodes = appendReasonCode(report.Entries[i].ReasonCodes, code)
		}
		if report.Entries[i].Eligible {
			eligibleCount++
			autoRoute = report.Entries[i].Agent.AgentID
		}
	}
	if eligibleCount != 1 {
		autoRoute = ""
	}
	return DispatchSuggestion{TicketID: ticket.ID, GeneratedAt: s.now(), AutoRouteAgentID: autoRoute, Suggestions: report.Entries}, nil
}

func (s *QueryService) DispatchQueue(ctx context.Context) (DispatchQueueView, error) {
	tickets, err := s.Tickets.ListTickets(ctx, contracts.TicketListOptions{IncludeArchived: false})
	if err != nil {
		return DispatchQueueView{}, err
	}
	entries := make([]DispatchQueueEntry, 0, len(tickets))
	for _, ticket := range tickets {
		if ticket.Archived || ticket.Status == contracts.StatusDone || ticket.Status == contracts.StatusCanceled {
			continue
		}
		suggestion, err := s.DispatchSuggest(ctx, ticket.ID)
		if err != nil {
			return DispatchQueueView{}, err
		}
		entries = append(entries, DispatchQueueEntry{Ticket: ticket, Suggestion: suggestion, GeneratedAt: s.now()})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Ticket.Priority != entries[j].Ticket.Priority {
			return priorityWeight(entries[i].Ticket.Priority) > priorityWeight(entries[j].Ticket.Priority)
		}
		if !entries[i].Ticket.UpdatedAt.Equal(entries[j].Ticket.UpdatedAt) {
			return entries[i].Ticket.UpdatedAt.Before(entries[j].Ticket.UpdatedAt)
		}
		return entries[i].Ticket.ID < entries[j].Ticket.ID
	})
	return DispatchQueueView{GeneratedAt: s.now(), Entries: entries}, nil
}

func (s *ActionService) AutoDispatchRun(ctx context.Context, ticketID string, actor contracts.Actor, reason string) (DispatchResult, error) {
	suggestion, err := NewQueryService(s.Root, s.Projects, s.Tickets, s.Events, s.Projection, s.Clock).DispatchSuggest(ctx, ticketID)
	if err != nil {
		return DispatchResult{}, err
	}
	if strings.TrimSpace(suggestion.AutoRouteAgentID) == "" {
		return DispatchResult{TicketID: ticketID, ReasonCodes: collectDispatchReasons(suggestion), GeneratedAt: s.now()}, apperr.New(apperr.CodeConflict, "dispatch requires exactly one eligible agent")
	}
	return s.DispatchRun(ctx, ticketID, suggestion.AutoRouteAgentID, contracts.RunKindWork, actor, reason)
}

func (s *QueryService) resolveRunbookForAgent(ctx context.Context, ticket contracts.TicketSnapshot, agent contracts.AgentProfile) (contracts.Runbook, string, error) {
	candidates := make([]string, 0, 4)
	if strings.TrimSpace(ticket.Runbook) != "" {
		candidates = append(candidates, ticket.Runbook)
	}
	if strings.TrimSpace(agent.DefaultRunbook) != "" {
		candidates = append(candidates, agent.DefaultRunbook)
	}
	project, err := s.Projects.GetProject(ctx, ticket.Project)
	if err == nil {
		for _, mapping := range project.Defaults.RunbookMappings {
			if mapping.TicketType == "" || mapping.TicketType == ticket.Type {
				candidates = append(candidates, mapping.Runbook)
			}
		}
	}
	candidates = append(candidates, defaultRunbookName(ticket.Type))
	seen := map[string]struct{}{}
	for _, raw := range candidates {
		name := sanitizeRunbookName(raw)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		runbook, err := s.Runbooks.LoadRunbook(ctx, name)
		if err != nil {
			continue
		}
		if !runbookAppliesToTicket(runbook, ticket.Type) {
			continue
		}
		stage := runbook.DefaultInitialStage
		if stage == "" && len(runbook.Stages) > 0 {
			stage = runbook.Stages[0].Key
		}
		return runbook, stage, nil
	}
	return contracts.Runbook{}, "", apperr.New(apperr.CodeInvalidInput, "no runbook matches this ticket")
}

func (s *QueryService) dispatchBlockers(ctx context.Context, ticket contracts.TicketSnapshot) ([]string, error) {
	codes := make([]string, 0, 4)
	if ticket.Archived || ticket.Status == contracts.StatusDone || ticket.Status == contracts.StatusCanceled {
		codes = appendReasonCode(codes, "ticket_status_ineligible")
	}
	boardStatus, err := s.BoardStatus(ctx, ticket)
	if err != nil {
		return nil, err
	}
	if boardStatus == contracts.StatusBlocked {
		codes = appendReasonCode(codes, "blocked_dependency")
	}
	if len(ticket.OpenGateIDs) > 0 {
		codes = appendReasonCode(codes, "open_gate_prevents_dispatch")
	}
	return codes, nil
}

func appendReasonCode(values []string, code string) []string {
	code = strings.TrimSpace(code)
	if code == "" {
		return values
	}
	for _, value := range values {
		if value == code {
			return values
		}
	}
	return append(values, code)
}

func runbookAppliesToTicket(runbook contracts.Runbook, ticketType contracts.TicketType) bool {
	if len(runbook.AppliesToTicketTypes) == 0 {
		return true
	}
	for _, kind := range runbook.AppliesToTicketTypes {
		if kind == ticketType {
			return true
		}
	}
	return false
}

func defaultRunbookName(ticketType contracts.TicketType) string {
	switch ticketType {
	case contracts.TicketTypeEpic:
		return "plan"
	case contracts.TicketTypeTask, contracts.TicketTypeBug, contracts.TicketTypeSubtask:
		return "implement"
	default:
		return "implement"
	}
}

func priorityWeight(priority contracts.Priority) int {
	switch priority {
	case contracts.PriorityCritical:
		return 4
	case contracts.PriorityHigh:
		return 3
	case contracts.PriorityMedium:
		return 2
	case contracts.PriorityLow:
		return 1
	default:
		return 0
	}
}

func collectDispatchReasons(suggestion DispatchSuggestion) []string {
	codes := make([]string, 0)
	for _, entry := range suggestion.Suggestions {
		for _, code := range entry.ReasonCodes {
			codes = appendReasonCode(codes, code)
		}
	}
	return codes
}
