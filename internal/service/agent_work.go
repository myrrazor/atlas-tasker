package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

const (
	AgentWorkReasonDependencyBlocked = "dependency_blocked"
	AgentWorkReasonWaitingReview     = "waiting_for_review"
	AgentWorkReasonWaitingOwner      = "waiting_for_owner"
	AgentWorkReasonNotReadyStatus    = "not_ready_status"
	AgentWorkReasonClaimedByOther    = "claimed_by_other"
	AgentWorkReasonPolicyBlocked     = "policy_blocked"
	AgentWorkReasonAgentDisabled     = "agent_disabled"
	AgentWorkReasonAgentAtCapacity   = "agent_at_capacity"
	AgentWorkReasonMissingCapability = "missing_capability"
	AgentWorkReasonActiveRunExists   = "active_run_exists"
	AgentWorkReasonOpenGate          = "open_gate"
)

func (s *QueryService) AgentAvailable(ctx context.Context, actor contracts.Actor) (AgentWorkView, error) {
	view, err := s.AgentWork(ctx, actor)
	if err != nil {
		return AgentWorkView{}, err
	}
	view.Pending = nil
	return view, nil
}

func (s *QueryService) AgentPending(ctx context.Context, actor contracts.Actor) (AgentWorkView, error) {
	view, err := s.AgentWork(ctx, actor)
	if err != nil {
		return AgentWorkView{}, err
	}
	view.Available = nil
	return view, nil
}

func (s *QueryService) AgentWork(ctx context.Context, actor contracts.Actor) (AgentWorkView, error) {
	resolved, err := s.ResolveActor(ctx, actor)
	if err != nil {
		return AgentWorkView{}, err
	}
	agentID := agentIDFromActor(resolved)
	var profile *contracts.AgentProfile
	if agentID != "" {
		loaded, loadErr := s.Agents.LoadAgent(ctx, agentID)
		if loadErr == nil {
			profile = &loaded
		}
	}
	tickets, err := s.Tickets.ListTickets(ctx, contracts.TicketListOptions{IncludeArchived: false})
	if err != nil {
		return AgentWorkView{}, err
	}
	runs, err := s.Runs.ListRuns(ctx, "")
	if err != nil {
		return AgentWorkView{}, err
	}
	activeByTicket := activeRunsByTicket(runs)
	activeCounts := activeRunCountsByAgent(runs)
	repo, _ := SCMService{Root: s.Root}.RepoStatus(ctx)

	view := AgentWorkView{
		Actor:       resolved,
		AgentID:     agentID,
		GeneratedAt: s.now(),
		Available:   []AgentWorkEntry{},
		Pending:     []AgentWorkEntry{},
	}
	for _, ticket := range tickets {
		if ticket.Archived || contracts.IsTerminalStatus(ticket.Status) {
			continue
		}
		entry, relevant, classifyErr := s.classifyAgentWork(ctx, ticket, resolved, profile, activeByTicket[ticket.ID], activeCounts, repo)
		if classifyErr != nil {
			return AgentWorkView{}, classifyErr
		}
		if !relevant {
			continue
		}
		if entry.State == AgentWorkAvailable {
			view.Available = append(view.Available, entry)
		} else {
			view.Pending = append(view.Pending, entry)
		}
	}
	sortAgentWorkEntries(view.Available)
	sortAgentWorkEntries(view.Pending)
	return view, nil
}

func (s *QueryService) classifyAgentWork(ctx context.Context, ticket contracts.TicketSnapshot, actor contracts.Actor, profile *contracts.AgentProfile, activeRuns []contracts.RunSnapshot, activeCounts map[string]int, repo GitRepoView) (AgentWorkEntry, bool, error) {
	policy, err := s.EffectivePolicy(ctx, ticket)
	if err != nil {
		return AgentWorkEntry{}, false, err
	}
	boardStatus, err := s.BoardStatus(ctx, ticket)
	if err != nil {
		return AgentWorkEntry{}, false, err
	}
	blockers, err := unresolvedBlockersFromStore(ctx, s.Tickets, ticket)
	if err != nil {
		return AgentWorkEntry{}, false, err
	}
	effectiveReviewActor := effectiveReviewer(ticket, policy)
	claimable := ticket.Assignee == "" && (ticket.Status == contracts.StatusReady || ticket.Status == contracts.StatusBacklog || ticket.Status == contracts.StatusBlocked)
	relevant := claimable || ticket.Assignee == actor || ticket.Lease.Actor == actor || effectiveReviewActor == actor || actor == contracts.Actor("human:owner")
	if !relevant {
		return AgentWorkEntry{}, false, nil
	}

	entry := AgentWorkEntry{
		Ticket:         ticket,
		State:          AgentWorkPending,
		ReasonCodes:    []string{},
		UnresolvedDeps: blockers,
		GitHint:        queueGitHint(repo, ticket, SCMService{Root: s.Root}.SuggestedBranch(ticket)),
	}
	if len(activeRuns) > 0 {
		entry.RunID = activeRuns[0].RunID
	}
	add := func(code string) {
		entry.ReasonCodes = appendReasonCode(entry.ReasonCodes, code)
	}

	if len(blockers) > 0 || boardStatus == contracts.StatusBlocked {
		add(AgentWorkReasonDependencyBlocked)
	}
	if ticket.Lease.Active(s.now()) && ticket.Lease.Actor != "" && ticket.Lease.Actor != actor {
		add(AgentWorkReasonClaimedByOther)
	}
	if len(policy.AllowedWorkers) > 0 && !actorInList(actor, policy.AllowedWorkers) {
		add(AgentWorkReasonPolicyBlocked)
	}
	if len(ticket.OpenGateIDs) > 0 && ticket.Status != contracts.StatusInReview {
		add(AgentWorkReasonOpenGate)
	}
	if profile != nil {
		if !profile.Enabled {
			add(AgentWorkReasonAgentDisabled)
		}
		if profile.MaxActiveRuns > 0 && activeCounts[profile.AgentID] >= profile.MaxActiveRuns && !activeRunOwnedBy(activeRuns, profile.AgentID) {
			add(AgentWorkReasonAgentAtCapacity)
		}
		if len(profile.AllowedTicketTypes) > 0 && !containsTicketType(profile.AllowedTicketTypes, ticket.Type) {
			add(AgentWorkReasonPolicyBlocked)
		}
		if missing := missingCapabilities(profile.Capabilities, ticket.RequiredCapabilities); len(missing) > 0 {
			add(AgentWorkReasonMissingCapability)
		}
	}
	if len(activeRuns) > 0 && !activeRunOwnedBy(activeRuns, agentIDFromActor(actor)) && !ticket.AllowParallelRuns {
		add(AgentWorkReasonActiveRunExists)
	}

	switch ticket.Status {
	case contracts.StatusReady:
		entry.Action = "start"
		if len(entry.ReasonCodes) == 0 {
			entry.State = AgentWorkAvailable
			entry.Reason = "ready for this agent"
			entry.Suggested = suggestedWorkCommands(ticket.ID, actor)
		}
	case contracts.StatusInProgress:
		entry.Action = "continue"
		if ticket.Assignee == actor || ticket.Lease.Actor == actor || activeRunOwnedBy(activeRuns, agentIDFromActor(actor)) {
			if len(entry.ReasonCodes) == 0 || onlyCapacityFromOwnRun(entry.ReasonCodes) {
				entry.State = AgentWorkAvailable
				entry.ReasonCodes = removeReasonCode(entry.ReasonCodes, AgentWorkReasonAgentAtCapacity)
				entry.Reason = "continue active work"
				entry.Suggested = suggestedContinueCommands(ticket.ID, actor)
			}
		} else if len(entry.ReasonCodes) == 0 {
			add(AgentWorkReasonNotReadyStatus)
		}
	case contracts.StatusInReview:
		entry.Action = "review"
		if effectiveReviewActor == actor || actor == contracts.Actor("human:owner") {
			if len(blockers) == 0 {
				entry.State = AgentWorkAvailable
				entry.Reason = "waiting for this reviewer"
				entry.Suggested = suggestedReviewCommands(ticket.ID, actor)
			}
		} else {
			add(AgentWorkReasonWaitingReview)
		}
	case contracts.StatusBacklog, contracts.StatusBlocked:
		entry.Action = "wait"
		if len(entry.ReasonCodes) == 0 {
			add(AgentWorkReasonNotReadyStatus)
		}
	default:
		entry.Action = "wait"
		if policy.CompletionMode == contracts.CompletionModeDualGate && ticket.ReviewState == contracts.ReviewStateApproved && actor == contracts.Actor("human:owner") {
			entry.Action = "complete"
			entry.State = AgentWorkAvailable
			entry.Reason = "approved and waiting for owner completion"
			entry.Suggested = []string{fmt.Sprintf("tracker ticket complete %s --actor %s --reason \"owner completion\"", ticket.ID, actor)}
		} else {
			add(AgentWorkReasonNotReadyStatus)
		}
	}
	if entry.State == AgentWorkPending && entry.Reason == "" {
		entry.Reason = strings.Join(entry.ReasonCodes, ",")
	}
	return entry, true, nil
}

func agentIDFromActor(actor contracts.Actor) string {
	raw := strings.TrimSpace(string(actor))
	if strings.HasPrefix(raw, "agent:") {
		return strings.TrimPrefix(raw, "agent:")
	}
	return ""
}

func activeRunsByTicket(runs []contracts.RunSnapshot) map[string][]contracts.RunSnapshot {
	byTicket := map[string][]contracts.RunSnapshot{}
	for _, run := range runs {
		if runIsActive(run.Status) {
			byTicket[run.TicketID] = append(byTicket[run.TicketID], run)
		}
	}
	for ticketID := range byTicket {
		sortRuns(byTicket[ticketID])
	}
	return byTicket
}

func activeRunOwnedBy(runs []contracts.RunSnapshot, agentID string) bool {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return false
	}
	for _, run := range runs {
		if run.AgentID == agentID && runIsActive(run.Status) {
			return true
		}
	}
	return false
}

func suggestedWorkCommands(ticketID string, actor contracts.Actor) []string {
	return []string{
		fmt.Sprintf("tracker ticket claim %s --actor %s --reason \"start work\"", ticketID, actor),
		fmt.Sprintf("tracker ticket move %s in_progress --actor %s --reason \"start work\"", ticketID, actor),
	}
}

func suggestedContinueCommands(ticketID string, actor contracts.Actor) []string {
	return []string{
		fmt.Sprintf("tracker ticket view %s --actor %s", ticketID, actor),
		fmt.Sprintf("tracker run list --ticket %s --json", ticketID),
	}
}

func suggestedReviewCommands(ticketID string, actor contracts.Actor) []string {
	return []string{
		fmt.Sprintf("tracker ticket approve %s --actor %s --reason \"review passed\"", ticketID, actor),
		fmt.Sprintf("tracker ticket reject %s --actor %s --reason \"changes requested\"", ticketID, actor),
	}
}

func onlyCapacityFromOwnRun(codes []string) bool {
	for _, code := range codes {
		if code != AgentWorkReasonAgentAtCapacity {
			return false
		}
	}
	return len(codes) > 0
}

func removeReasonCode(codes []string, target string) []string {
	filtered := codes[:0]
	for _, code := range codes {
		if code != target {
			filtered = append(filtered, code)
		}
	}
	return filtered
}

func sortAgentWorkEntries(entries []AgentWorkEntry) {
	sort.Slice(entries, func(i, j int) bool {
		left := entries[i].Ticket
		right := entries[j].Ticket
		if priorityWeight(left.Priority) != priorityWeight(right.Priority) {
			return priorityWeight(left.Priority) > priorityWeight(right.Priority)
		}
		if !left.UpdatedAt.Equal(right.UpdatedAt) {
			return left.UpdatedAt.Before(right.UpdatedAt)
		}
		return left.ID < right.ID
	})
}
