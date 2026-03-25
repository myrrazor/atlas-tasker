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
	return RunDetailView{Run: run, Ticket: ticket, GeneratedAt: s.now()}, nil
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
