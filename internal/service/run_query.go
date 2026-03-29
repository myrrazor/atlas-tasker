package service

import (
	"context"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func (s *QueryService) ListRuns(ctx context.Context, ticketID string, agentID string, status contracts.RunStatus) ([]contracts.RunSnapshot, error) {
	runs, err := s.Runs.ListRuns(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	items := make([]contracts.RunSnapshot, 0, len(runs))
	for _, run := range runs {
		if strings.TrimSpace(agentID) != "" && run.AgentID != strings.TrimSpace(agentID) {
			continue
		}
		if status != "" && run.Status != status {
			continue
		}
		items = append(items, run)
	}
	sortRuns(items)
	return items, nil
}

func (s *QueryService) RunDetail(ctx context.Context, runID string) (RunDetailView, error) {
	run, err := s.Runs.LoadRun(ctx, runID)
	if err != nil {
		return RunDetailView{}, err
	}
	ticket, err := s.Tickets.GetTicket(ctx, run.TicketID)
	if err != nil {
		return RunDetailView{}, err
	}
	evidence, err := s.Evidence.ListEvidence(ctx, runID)
	if err != nil {
		return RunDetailView{}, err
	}
	changes, err := s.Changes.ListChanges(ctx, run.TicketID)
	if err != nil {
		return RunDetailView{}, err
	}
	filteredChanges := make([]contracts.ChangeRef, 0, len(changes))
	for _, change := range changes {
		if change.RunID == runID {
			filteredChanges = append(filteredChanges, change)
		}
	}
	checks, err := s.Checks.ListChecks(ctx, contracts.CheckScopeRun, runID)
	if err != nil {
		return RunDetailView{}, err
	}
	gates, err := s.Gates.ListGates(ctx, run.TicketID)
	if err != nil {
		return RunDetailView{}, err
	}
	filteredGates := make([]contracts.GateSnapshot, 0, len(gates))
	for _, gate := range gates {
		if gate.RunID == "" || gate.RunID == runID {
			filteredGates = append(filteredGates, gate)
		}
	}
	handoffs, err := s.Handoffs.ListHandoffs(ctx, run.TicketID)
	if err != nil {
		return RunDetailView{}, err
	}
	filteredHandoffs := make([]contracts.HandoffPacket, 0, len(handoffs))
	for _, handoff := range handoffs {
		if handoff.SourceRunID == runID {
			filteredHandoffs = append(filteredHandoffs, handoff)
		}
	}
	allMentions, err := s.Mentions.ListMentions(ctx, "")
	if err != nil {
		return RunDetailView{}, err
	}
	mentionSources := make(map[string]struct{}, len(evidence)+len(filteredHandoffs))
	for _, item := range evidence {
		mentionSources["evidence:"+item.EvidenceID] = struct{}{}
	}
	for _, handoff := range filteredHandoffs {
		mentionSources["handoff:"+handoff.HandoffID] = struct{}{}
	}
	mentions := make([]contracts.Mention, 0)
	for _, mention := range allMentions {
		key := mention.SourceKind + ":" + mention.SourceID
		if _, ok := mentionSources[key]; ok {
			mentions = append(mentions, mention)
		}
	}
	return RunDetailView{Run: run, Ticket: ticket, Gates: filteredGates, Changes: filteredChanges, Checks: checks, Evidence: evidence, Handoffs: filteredHandoffs, Mentions: mentions, GeneratedAt: s.now()}, nil
}

func (s *QueryService) WorktreeDetail(ctx context.Context, runID string) (WorktreeStatusView, error) {
	run, err := s.Runs.LoadRun(ctx, runID)
	if err != nil {
		return WorktreeStatusView{}, err
	}
	return WorktreeManager{Root: s.Root}.Inspect(ctx, run)
}

func (s *QueryService) WorktreeList(ctx context.Context) ([]WorktreeStatusView, error) {
	runs, err := s.Runs.ListRuns(ctx, "")
	if err != nil {
		return nil, err
	}
	return WorktreeManager{Root: s.Root}.List(ctx, runs)
}

func activeRunCountsByAgent(runs []contracts.RunSnapshot) map[string]int {
	counts := make(map[string]int)
	for _, run := range runs {
		if strings.TrimSpace(run.AgentID) == "" || !runIsActive(run.Status) {
			continue
		}
		counts[run.AgentID]++
	}
	return counts
}

func activeRunCountForTicket(runs []contracts.RunSnapshot) int {
	count := 0
	for _, run := range runs {
		if runIsActive(run.Status) {
			count++
		}
	}
	return count
}
