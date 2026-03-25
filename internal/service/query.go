package service

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type QueryService struct {
	Root       string
	Projects   contracts.ProjectStore
	Tickets    contracts.TicketStore
	Agents     contracts.AgentStore
	Runs       contracts.RunStore
	Runbooks   contracts.RunbookStore
	Events     contracts.EventLog
	Projection contracts.ProjectionStore
	Views      ViewStore
	Clock      func() time.Time
}

func NewQueryService(root string, projects contracts.ProjectStore, tickets contracts.TicketStore, events contracts.EventLog, projection contracts.ProjectionStore, clock func() time.Time) *QueryService {
	return &QueryService{Root: root, Projects: projects, Tickets: tickets, Agents: AgentStore{Root: root}, Runs: RunStore{Root: root}, Runbooks: RunbookStore{Root: root}, Events: events, Projection: projection, Views: ViewStore{Root: root}, Clock: clock}
}

func (s *QueryService) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

func (s *QueryService) Board(ctx context.Context, opts contracts.BoardQueryOptions) (BoardViewModel, error) {
	board, err := s.Projection.QueryBoard(ctx, opts)
	if err != nil {
		return BoardViewModel{}, err
	}
	return BoardViewModel{Board: board}, nil
}

func (s *QueryService) Search(ctx context.Context, query contracts.SearchQuery) ([]contracts.TicketSnapshot, error) {
	return s.Projection.QuerySearch(ctx, query)
}

func (s *QueryService) ListSavedViews() ([]contracts.SavedView, error) {
	return s.Views.ListViews()
}

func (s *QueryService) ListAgents(ctx context.Context) ([]AgentDetailView, error) {
	profiles, err := s.Agents.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	runs, err := s.Runs.ListRuns(ctx, "")
	if err != nil {
		return nil, err
	}
	activeCounts := activeRunCountsByAgent(runs)
	items := make([]AgentDetailView, 0, len(profiles))
	for _, profile := range profiles {
		items = append(items, AgentDetailView{
			Profile:     profile,
			ActiveRuns:  activeCounts[profile.AgentID],
			GeneratedAt: s.now(),
		})
	}
	return items, nil
}

func (s *QueryService) AgentDetail(ctx context.Context, agentID string) (AgentDetailView, error) {
	profile, err := s.Agents.LoadAgent(ctx, agentID)
	if err != nil {
		return AgentDetailView{}, err
	}
	runs, err := s.Runs.ListRuns(ctx, "")
	if err != nil {
		return AgentDetailView{}, err
	}
	return AgentDetailView{Profile: profile, ActiveRuns: activeRunCountsByAgent(runs)[profile.AgentID], GeneratedAt: s.now()}, nil
}

func (s *QueryService) AgentEligibility(ctx context.Context, ticketID string) (AgentEligibilityReport, error) {
	ticket, err := s.Tickets.GetTicket(ctx, ticketID)
	if err != nil {
		return AgentEligibilityReport{}, err
	}
	profiles, err := s.Agents.ListAgents(ctx)
	if err != nil {
		return AgentEligibilityReport{}, err
	}
	runs, err := s.Runs.ListRuns(ctx, "")
	if err != nil {
		return AgentEligibilityReport{}, err
	}
	activeCounts := activeRunCountsByAgent(runs)
	ticketRuns, err := s.Runs.ListRuns(ctx, ticket.ID)
	if err != nil {
		return AgentEligibilityReport{}, err
	}
	hasActiveRun := activeRunCountForTicket(ticketRuns) > 0
	items := make([]AgentEligibilityEntry, 0, len(profiles))
	for _, profile := range profiles {
		activeRuns := activeCounts[profile.AgentID]
		entry := AgentEligibilityEntry{
			Agent:       profile,
			Eligible:    true,
			ReasonCodes: []string{},
			ActiveRuns:  activeRuns,
		}
		if !profile.Enabled {
			entry.Eligible = false
			entry.ReasonCodes = append(entry.ReasonCodes, "agent_disabled")
		}
		if profile.MaxActiveRuns > 0 && activeRuns >= profile.MaxActiveRuns {
			entry.Eligible = false
			entry.ReasonCodes = append(entry.ReasonCodes, "agent_at_capacity")
		}
		if len(profile.AllowedTicketTypes) > 0 && !containsTicketType(profile.AllowedTicketTypes, ticket.Type) {
			entry.Eligible = false
			entry.ReasonCodes = append(entry.ReasonCodes, "disallowed_worker")
		}
		if missing := missingCapabilities(profile.Capabilities, ticket.RequiredCapabilities); len(missing) > 0 {
			entry.Eligible = false
			entry.ReasonCodes = append(entry.ReasonCodes, "missing_capability")
		}
		if runbook, stage, runbookErr := s.resolveRunbookForAgent(ctx, ticket, profile); runbookErr == nil {
			entry.Runbook = runbook.Name
			entry.Stage = stage
		} else {
			entry.Eligible = false
			entry.ReasonCodes = append(entry.ReasonCodes, "runbook_requirement_unsatisfied")
		}
		if hasActiveRun && !ticket.AllowParallelRuns {
			entry.Eligible = false
			entry.ReasonCodes = append(entry.ReasonCodes, "parallel_runs_disabled")
		}
		items = append(items, entry)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Eligible != items[j].Eligible {
			return items[i].Eligible
		}
		if items[i].Agent.RoutingWeight != items[j].Agent.RoutingWeight {
			return items[i].Agent.RoutingWeight > items[j].Agent.RoutingWeight
		}
		return items[i].Agent.AgentID < items[j].Agent.AgentID
	})
	for i := range items {
		items[i].Rank = i + 1
	}
	return AgentEligibilityReport{TicketID: ticketID, GeneratedAt: s.now(), Entries: items}, nil
}

func (s *QueryService) NotificationLog(limit int) ([]NotificationDelivery, error) {
	cfg, err := config.Load(s.Root)
	if err != nil {
		return nil, err
	}
	records, err := ReadNotificationLog(s.Root, cfg)
	if err != nil {
		return nil, err
	}
	return tailNotificationRecords(records, limit), nil
}

func (s *QueryService) DeadLetters(limit int) ([]NotificationDelivery, error) {
	cfg, err := config.Load(s.Root)
	if err != nil {
		return nil, err
	}
	records, err := ReadDeadLetters(s.Root, cfg)
	if err != nil {
		return nil, err
	}
	return tailNotificationRecords(records, limit), nil
}

func tailNotificationRecords(records []NotificationDelivery, limit int) []NotificationDelivery {
	if limit <= 0 || len(records) <= limit {
		return records
	}
	return append([]NotificationDelivery{}, records[len(records)-limit:]...)
}

func (s *QueryService) AutomationRules() ([]contracts.AutomationRule, error) {
	return (AutomationEngine{Store: AutomationStore{Root: s.Root}}).ListRules()
}

func (s *QueryService) ExplainAutomationRules(ctx context.Context, ticketID string) ([]AutomationResult, error) {
	if strings.TrimSpace(ticketID) == "" {
		return []AutomationResult{}, nil
	}
	rules, err := s.AutomationRules()
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		return []AutomationResult{}, nil
	}
	detail, err := s.TicketDetail(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	eventType := contracts.EventTicketUpdated
	actor := contracts.Actor("human:owner")
	if len(detail.History) > 0 {
		last := detail.History[len(detail.History)-1]
		eventType = last.Type
		actor = last.Actor
	}
	event := contracts.Event{
		EventID:       1,
		Timestamp:     s.now(),
		Actor:         actor,
		Type:          eventType,
		Project:       detail.Ticket.Project,
		TicketID:      detail.Ticket.ID,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	engine := AutomationEngine{Store: AutomationStore{Root: s.Root}}
	results := make([]AutomationResult, 0, len(rules))
	for _, rule := range rules {
		result, err := engine.Explain(ctx, s, rule, event, ticketID)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Matched != results[j].Matched {
			return results[i].Matched
		}
		return results[i].Rule.Name < results[j].Rule.Name
	})
	return results, nil
}

func (s *QueryService) SavedView(name string) (contracts.SavedView, error) {
	return s.Views.LoadView(name)
}

func (s *QueryService) ListSubscriptions(ctx context.Context, actor contracts.Actor) ([]SubscriptionView, error) {
	subscriptions, err := SubscriptionStore{Root: s.Root}.ListSubscriptions()
	if err != nil {
		return nil, err
	}
	items := make([]SubscriptionView, 0, len(subscriptions))
	if actor == "" {
		for _, subscription := range subscriptions {
			items = append(items, s.subscriptionView(ctx, subscription))
		}
		return items, nil
	}
	for _, subscription := range subscriptions {
		if subscription.Actor != actor {
			continue
		}
		items = append(items, s.subscriptionView(ctx, subscription))
	}
	return items, nil
}

func (s *QueryService) subscriptionView(ctx context.Context, subscription contracts.Subscription) SubscriptionView {
	view := SubscriptionView{Subscription: subscription, Active: true}
	switch subscription.TargetKind {
	case contracts.SubscriptionTargetTicket:
		ticket, err := s.Tickets.GetTicket(ctx, subscription.Target)
		if err != nil {
			view.Active = false
			view.InactiveReason = "missing_ticket"
			return view
		}
		if ticket.Archived || ticket.Status == contracts.StatusCanceled {
			view.Active = false
			view.InactiveReason = "ticket_inactive"
		}
	case contracts.SubscriptionTargetProject:
		if _, err := s.Projects.GetProject(ctx, subscription.Target); err != nil {
			view.Active = false
			view.InactiveReason = "missing_project"
		}
	case contracts.SubscriptionTargetSavedView:
		if _, err := s.Views.LoadView(subscription.Target); err != nil {
			view.Active = false
			view.InactiveReason = "missing_saved_view"
		}
	}
	return view
}

func (s *QueryService) History(ctx context.Context, ticketID string) (HistoryView, error) {
	events, err := s.Projection.QueryHistory(ctx, ticketID)
	if err != nil {
		return HistoryView{}, err
	}
	return HistoryView{TicketID: ticketID, Events: events}, nil
}

func (s *QueryService) TicketDetail(ctx context.Context, ticketID string) (TicketDetailView, error) {
	ticket, err := s.Projection.QueryTicket(ctx, ticketID)
	if err != nil {
		ticket, err = s.Tickets.GetTicket(ctx, ticketID)
		if err != nil {
			return TicketDetailView{}, err
		}
	}
	history, err := s.History(ctx, ticketID)
	if err != nil {
		return TicketDetailView{}, err
	}
	comments := make([]string, 0)
	for _, event := range history.Events {
		if event.Type != contracts.EventTicketCommented {
			continue
		}
		if payloadMap, ok := event.Payload.(map[string]any); ok {
			if body, ok := payloadMap["body"].(string); ok && strings.TrimSpace(body) != "" {
				comments = append(comments, strings.TrimSpace(body))
			}
		}
	}
	policy, err := s.EffectivePolicy(ctx, ticket)
	if err != nil {
		return TicketDetailView{}, err
	}
	ticket, err = s.withProgress(ctx, ticket)
	if err != nil {
		return TicketDetailView{}, err
	}
	gitView, err := SCMService{Root: s.Root}.ContextForTicket(ctx, ticket)
	if err != nil {
		return TicketDetailView{}, err
	}
	return TicketDetailView{Ticket: ticket, Comments: comments, History: history.Events, EffectivePolicy: policy, Git: gitView}, nil
}

func (s *QueryService) InspectTicket(ctx context.Context, ticketID string, actor contracts.Actor) (InspectView, error) {
	detail, err := s.TicketDetail(ctx, ticketID)
	if err != nil {
		return InspectView{}, err
	}
	view := InspectView{
		Ticket:          detail.Ticket,
		BoardStatus:     contracts.BoardStatus(detail.Ticket),
		LeaseActive:     detail.Ticket.Lease.Active(s.now()),
		EffectivePolicy: detail.EffectivePolicy,
		History:         detail.History,
		Git:             detail.Git,
	}
	if actor == "" {
		return view, nil
	}
	queue, err := s.Queue(ctx, actor)
	if err != nil {
		return InspectView{}, err
	}
	for category, entries := range queue.Categories {
		for _, entry := range entries {
			if entry.Ticket.ID == ticketID {
				view.QueueCategories = append(view.QueueCategories, category)
				break
			}
		}
	}
	sort.Slice(view.QueueCategories, func(i, j int) bool {
		return view.QueueCategories[i] < view.QueueCategories[j]
	})
	return view, nil
}

func missingCapabilities(have []string, want []string) []string {
	if len(want) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(have))
	for _, capability := range have {
		key := strings.TrimSpace(strings.ToLower(capability))
		if key == "" {
			continue
		}
		set[key] = struct{}{}
	}
	var missing []string
	for _, capability := range want {
		key := strings.TrimSpace(strings.ToLower(capability))
		if key == "" {
			continue
		}
		if _, ok := set[key]; !ok {
			missing = append(missing, key)
		}
	}
	return missing
}

func containsTicketType(items []contracts.TicketType, candidate contracts.TicketType) bool {
	for _, item := range items {
		if item == candidate {
			return true
		}
	}
	return false
}

func (s *QueryService) EffectivePolicy(ctx context.Context, ticket contracts.TicketSnapshot) (EffectivePolicyView, error) {
	return resolveEffectivePolicy(ctx, s.Root, s.Projects, s.Tickets, ticket)
}

func (s *QueryService) Queue(ctx context.Context, actor contracts.Actor) (QueueView, error) {
	if actor == "" {
		resolved, err := s.ResolveActor(ctx, "")
		if err != nil {
			return QueueView{}, err
		}
		actor = resolved
	}
	tickets, err := s.Tickets.ListTickets(ctx, contracts.TicketListOptions{IncludeArchived: false})
	if err != nil {
		return QueueView{}, err
	}
	now := s.now()
	repo, err := SCMService{Root: s.Root}.RepoStatus(ctx)
	if err != nil {
		return QueueView{}, err
	}
	view := QueueView{Actor: actor, GeneratedAt: now, Categories: map[QueueCategory][]QueueEntry{}}
	for _, ticket := range tickets {
		policy, err := s.EffectivePolicy(ctx, ticket)
		if err != nil {
			return QueueView{}, err
		}
		if len(policy.AllowedWorkers) > 0 && !actorInList(actor, policy.AllowedWorkers) {
			view.Categories[QueuePolicyViolations] = append(view.Categories[QueuePolicyViolations], QueueEntry{Ticket: ticket, Reason: "actor not allowed by effective policy"})
		}
		entryHint := queueGitHint(repo, ticket, SCMService{Root: s.Root}.SuggestedBranch(ticket))
		switch {
		case ticket.Lease.Actor != "" && !ticket.Lease.ExpiresAt.IsZero() && !ticket.Lease.Active(now):
			view.Categories[QueueStaleClaims] = append(view.Categories[QueueStaleClaims], QueueEntry{Ticket: ticket, Reason: "lease expired", GitHint: entryHint})
		case ticket.Lease.Active(now) && ticket.Lease.Actor == actor:
			view.Categories[QueueClaimedByMe] = append(view.Categories[QueueClaimedByMe], QueueEntry{Ticket: ticket, Reason: "active lease owned by actor", GitHint: entryHint})
		case ticket.Status == contracts.StatusReady && (ticket.Assignee == "" || ticket.Assignee == actor):
			view.Categories[QueueReadyForMe] = append(view.Categories[QueueReadyForMe], QueueEntry{Ticket: ticket, Reason: "ready and assignable", GitHint: entryHint})
		case contracts.BoardStatus(ticket) == contracts.StatusBlocked && (ticket.Assignee == "" || ticket.Assignee == actor):
			view.Categories[QueueBlockedForMe] = append(view.Categories[QueueBlockedForMe], QueueEntry{Ticket: ticket, Reason: "ticket is blocked", GitHint: entryHint})
		}
		if ticket.Status == contracts.StatusInReview && (ticket.Reviewer == actor || actor == contracts.Actor("human:owner")) {
			view.Categories[QueueNeedsReview] = append(view.Categories[QueueNeedsReview], QueueEntry{Ticket: ticket, Reason: "waiting for review", GitHint: entryHint})
		}
		if policy.CompletionMode == contracts.CompletionModeDualGate && ticket.ReviewState == contracts.ReviewStateApproved {
			view.Categories[QueueAwaitingOwner] = append(view.Categories[QueueAwaitingOwner], QueueEntry{Ticket: ticket, Reason: "approved and waiting for owner completion", GitHint: entryHint})
		}
	}
	for category := range view.Categories {
		sortQueueEntries(view.Categories[category])
	}
	return view, nil
}

func queueGitHint(repo GitRepoView, ticket contracts.TicketSnapshot, suggested string) string {
	if !repo.Present {
		return ""
	}
	branch := strings.ToLower(strings.TrimSpace(repo.Branch))
	id := strings.ToLower(ticket.ID)
	switch {
	case branch != "" && strings.Contains(branch, id) && repo.Dirty:
		return "current branch matches ticket; repo has uncommitted changes"
	case branch != "" && strings.Contains(branch, id):
		return "current branch matches ticket"
	case repo.Dirty:
		return fmt.Sprintf("suggested branch %s; repo has uncommitted changes", suggested)
	default:
		return fmt.Sprintf("suggested branch %s", suggested)
	}
}

func (s *QueryService) Who(ctx context.Context) ([]contracts.TicketSnapshot, error) {
	tickets, err := s.Tickets.ListTickets(ctx, contracts.TicketListOptions{IncludeArchived: false})
	if err != nil {
		return nil, err
	}
	now := s.now()
	active := make([]contracts.TicketSnapshot, 0)
	for _, ticket := range tickets {
		if ticket.Lease.Actor == "" {
			continue
		}
		if ticket.Lease.Active(now) || (!ticket.Lease.ExpiresAt.IsZero() && ticket.Lease.ExpiresAt.Before(now)) {
			active = append(active, ticket)
		}
	}
	sort.Slice(active, func(i, j int) bool {
		if active[i].Lease.ExpiresAt.Equal(active[j].Lease.ExpiresAt) {
			return active[i].ID < active[j].ID
		}
		if active[i].Lease.ExpiresAt.IsZero() {
			return false
		}
		if active[j].Lease.ExpiresAt.IsZero() {
			return true
		}
		return active[i].Lease.ExpiresAt.Before(active[j].Lease.ExpiresAt)
	})
	return active, nil
}

func (s *QueryService) ResolveActor(ctx context.Context, explicit contracts.Actor) (contracts.Actor, error) {
	if explicit != "" {
		if !explicit.IsValid() {
			return "", fmt.Errorf("invalid actor: %s", explicit)
		}
		return explicit, nil
	}
	if envActor := strings.TrimSpace(os.Getenv("TRACKER_ACTOR")); envActor != "" {
		actor := contracts.Actor(envActor)
		if !actor.IsValid() {
			return "", fmt.Errorf("invalid TRACKER_ACTOR: %s", envActor)
		}
		return actor, nil
	}
	cfg, err := config.Load(s.Root)
	if err != nil {
		return "", err
	}
	if cfg.Actor.Default != "" {
		return cfg.Actor.Default, nil
	}
	return "", fmt.Errorf("actor is required: pass --actor, set TRACKER_ACTOR, or configure actor.default")
}

func (s *QueryService) Next(ctx context.Context, actor contracts.Actor) (NextView, error) {
	resolved, err := s.ResolveActor(ctx, actor)
	if err != nil {
		return NextView{}, err
	}
	queue, err := s.Queue(ctx, resolved)
	if err != nil {
		return NextView{}, err
	}
	order := []QueueCategory{
		QueueReadyForMe,
		QueueClaimedByMe,
		QueueNeedsReview,
		QueueAwaitingOwner,
		QueueBlockedForMe,
		QueueStaleClaims,
		QueuePolicyViolations,
	}
	view := NextView{Actor: resolved}
	for _, category := range order {
		for _, entry := range queue.Categories[category] {
			view.Entries = append(view.Entries, NextEntry{Category: category, Entry: entry})
		}
	}
	return view, nil
}

func (s *QueryService) RunSavedView(ctx context.Context, name string, actorOverride contracts.Actor) (SavedViewResult, error) {
	view, err := s.SavedView(name)
	if err != nil {
		return SavedViewResult{}, err
	}
	result := SavedViewResult{View: view}
	switch view.Kind {
	case contracts.SavedViewKindBoard:
		board, err := s.Board(ctx, contracts.BoardQueryOptions{
			Project:  strings.TrimSpace(view.Project),
			Assignee: view.Assignee,
			Type:     view.Type,
		})
		if err != nil {
			return SavedViewResult{}, err
		}
		filtered := board.Board
		if len(view.Board.Columns) > 0 {
			filtered = filterBoardColumns(board.Board, view.Board.Columns)
		}
		result.Board = &BoardViewModel{Board: filtered}
	case contracts.SavedViewKindSearch:
		query, err := contracts.ParseSearchQuery(strings.TrimSpace(view.Query))
		if err != nil {
			return SavedViewResult{}, err
		}
		tickets, err := s.Search(ctx, query)
		if err != nil {
			return SavedViewResult{}, err
		}
		result.Tickets = tickets
	case contracts.SavedViewKindQueue:
		actor, err := s.resolveSavedViewActor(ctx, view, actorOverride)
		if err != nil {
			return SavedViewResult{}, err
		}
		queue, err := s.Queue(ctx, actor)
		if err != nil {
			return SavedViewResult{}, err
		}
		if len(view.Queue.Categories) > 0 {
			queue.Categories = filterQueueView(queue.Categories, view.Queue.Categories)
		}
		result.Actor = actor
		result.Queue = &queue
	case contracts.SavedViewKindNext:
		actor, err := s.resolveSavedViewActor(ctx, view, actorOverride)
		if err != nil {
			return SavedViewResult{}, err
		}
		next, err := s.Next(ctx, actor)
		if err != nil {
			return SavedViewResult{}, err
		}
		if len(view.Queue.Categories) > 0 {
			next.Entries = filterNextEntries(next.Entries, view.Queue.Categories)
		}
		result.Actor = actor
		result.Next = &next
	default:
		return SavedViewResult{}, fmt.Errorf("unsupported saved view kind: %s", view.Kind)
	}
	return result, nil
}

func applyPolicy(view *EffectivePolicyView, policy contracts.TicketPolicy) {
	if policy.CompletionMode != "" {
		view.CompletionMode = policy.CompletionMode
	}
	if len(policy.AllowedWorkers) > 0 {
		view.AllowedWorkers = append([]contracts.Actor{}, policy.AllowedWorkers...)
	}
	if policy.RequiredReviewer != "" {
		view.RequiredReviewer = policy.RequiredReviewer
	}
}

func actorInList(actor contracts.Actor, values []contracts.Actor) bool {
	for _, value := range values {
		if value == actor {
			return true
		}
	}
	return false
}

func sortQueueEntries(entries []QueueEntry) {
	priorityRank := map[contracts.Priority]int{
		contracts.PriorityCritical: 4,
		contracts.PriorityHigh:     3,
		contracts.PriorityMedium:   2,
		contracts.PriorityLow:      1,
	}
	sort.Slice(entries, func(i, j int) bool {
		left := priorityRank[entries[i].Ticket.Priority]
		right := priorityRank[entries[j].Ticket.Priority]
		if left != right {
			return left > right
		}
		if !entries[i].Ticket.UpdatedAt.Equal(entries[j].Ticket.UpdatedAt) {
			return entries[i].Ticket.UpdatedAt.Before(entries[j].Ticket.UpdatedAt)
		}
		return entries[i].Ticket.ID < entries[j].Ticket.ID
	})
}

func filterBoardColumns(board contracts.BoardView, columns []contracts.Status) contracts.BoardView {
	filtered := contracts.BoardView{Columns: map[contracts.Status][]contracts.TicketSnapshot{}}
	for _, column := range columns {
		filtered.Columns[column] = append([]contracts.TicketSnapshot{}, board.Columns[column]...)
	}
	return filtered
}

func filterQueueView(categories map[QueueCategory][]QueueEntry, wanted []string) map[QueueCategory][]QueueEntry {
	filtered := map[QueueCategory][]QueueEntry{}
	for _, raw := range wanted {
		category := QueueCategory(strings.TrimSpace(raw))
		if category == "" {
			continue
		}
		filtered[category] = append([]QueueEntry{}, categories[category]...)
	}
	return filtered
}

func filterNextEntries(entries []NextEntry, wanted []string) []NextEntry {
	allowed := make(map[QueueCategory]struct{}, len(wanted))
	for _, raw := range wanted {
		category := QueueCategory(strings.TrimSpace(raw))
		if category == "" {
			continue
		}
		allowed[category] = struct{}{}
	}
	if len(allowed) == 0 {
		return entries
	}
	filtered := make([]NextEntry, 0, len(entries))
	for _, entry := range entries {
		if _, ok := allowed[entry.Category]; ok {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func (s *QueryService) resolveSavedViewActor(ctx context.Context, view contracts.SavedView, actorOverride contracts.Actor) (contracts.Actor, error) {
	if actorOverride != "" {
		return s.ResolveActor(ctx, actorOverride)
	}
	if view.Actor != "" {
		return s.ResolveActor(ctx, view.Actor)
	}
	return s.ResolveActor(ctx, "")
}

func (s *QueryService) withProgress(ctx context.Context, ticket contracts.TicketSnapshot) (contracts.TicketSnapshot, error) {
	children, err := s.Tickets.ListTickets(ctx, contracts.TicketListOptions{Project: ticket.Project, IncludeArchived: false})
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}
	total := 0
	done := 0
	blocked := 0
	for _, child := range children {
		if child.Parent != ticket.ID {
			continue
		}
		total++
		if contracts.IsTerminalStatus(child.Status) {
			done++
		}
		if contracts.BoardStatus(child) == contracts.StatusBlocked {
			blocked++
		}
	}
	if total == 0 {
		return ticket, nil
	}
	ticket.Progress = contracts.ProgressSummary{
		TotalChildren:   total,
		DoneChildren:    done,
		BlockedChildren: blocked,
		Percent:         (done * 100) / total,
	}
	return ticket, nil
}
