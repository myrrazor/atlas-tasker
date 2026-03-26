package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

type runMutationPayload struct {
	Run      contracts.RunSnapshot    `json:"run"`
	Ticket   contracts.TicketSnapshot `json:"ticket,omitempty"`
	Gate     contracts.GateSnapshot   `json:"gate,omitempty"`
	Gates    []contracts.GateSnapshot `json:"gates,omitempty"`
	Evidence contracts.EvidenceItem   `json:"evidence,omitempty"`
	Handoff  contracts.HandoffPacket  `json:"handoff,omitempty"`
}

func (s *ActionService) DispatchRun(ctx context.Context, ticketID string, agentID string, kind contracts.RunKind, actor contracts.Actor, reason string) (DispatchResult, error) {
	return withWriteLock(ctx, s.LockManager, "dispatch run", func(ctx context.Context) (DispatchResult, error) {
		if !actor.IsValid() {
			return DispatchResult{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		ticket, err := s.Tickets.GetTicket(ctx, ticketID)
		if err != nil {
			return DispatchResult{}, err
		}
		agent, err := s.Agents.LoadAgent(ctx, agentID)
		if err != nil {
			return DispatchResult{}, err
		}
		if kind == "" {
			kind = contracts.RunKindWork
		}
		if !kind.IsValid() {
			return DispatchResult{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid run kind: %s", kind))
		}
		if _, err := s.requirePermission(ctx, permissionEvalInput{
			Action:            contracts.PermissionActionDispatch,
			Actor:             actor,
			Ticket:            ticket,
			ActorAgent:        &agent,
			Runbook:           permissionRunbook(ticket, nil),
			ChangedFilesKnown: false,
		}); err != nil {
			return DispatchResult{}, err
		}
		entry, err := s.agentEligibility(ctx, ticket, agent.AgentID)
		if err != nil {
			return DispatchResult{}, err
		}
		if !entry.Eligible {
			return DispatchResult{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("agent %s is not eligible for %s (%s)", agent.AgentID, ticket.ID, strings.Join(entry.ReasonCodes, ", ")))
		}
		runs, err := s.Runs.ListRuns(ctx, ticket.ID)
		if err != nil {
			return DispatchResult{}, err
		}
		if !ticket.AllowParallelRuns {
			for _, existing := range runs {
				if runIsActive(existing.Status) {
					return DispatchResult{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("ticket %s already has active run %s", ticket.ID, existing.RunID))
				}
			}
		}
		runID, err := nextRunID(ctx, s.Runs, s.now())
		if err != nil {
			return DispatchResult{}, err
		}
		run := normalizeRunSnapshot(contracts.RunSnapshot{
			RunID:     runID,
			TicketID:  ticket.ID,
			Project:   ticket.Project,
			AgentID:   agent.AgentID,
			Provider:  agent.Provider,
			Status:    contracts.RunStatusDispatched,
			Kind:      kind,
			CreatedAt: s.now(),
		})
		project, err := s.Projects.GetProject(ctx, ticket.Project)
		if err != nil {
			project = contracts.Project{Key: ticket.Project}
		}
		worktrees := WorktreeManager{Root: s.Root}
		run, err = worktrees.Prepare(ctx, project, ticket, run)
		if err != nil {
			return DispatchResult{}, err
		}
		runbook, stage, err := NewQueryService(s.Root, s.Projects, s.Tickets, s.Events, s.Projection, s.Clock).resolveRunbookForAgent(ctx, ticket, agent)
		if err != nil {
			return DispatchResult{}, err
		}
		run.BlueprintStage = stage
		if strings.TrimSpace(ticket.Runbook) == "" {
			ticket.Runbook = runbook.Name
		}
		ticket.LatestRunID = run.RunID
		ticket.LastDispatchAt = s.now()
		payload := runMutationPayload{Run: run, Ticket: ticket}
		event, err := s.newEvent(ctx, ticket.Project, s.now(), actor, reason, contracts.EventRunDispatched, ticket.ID, payload)
		if err != nil {
			return DispatchResult{}, err
		}
		originalTicket, err := s.Tickets.GetTicket(ctx, ticket.ID)
		if err != nil {
			return DispatchResult{}, err
		}
		runtimeDir := storage.RuntimeDir(s.Root, run.RunID)
		if err := s.commitMutation(ctx, "dispatch run", "run_snapshot", event, func(ctx context.Context) error {
			var worktreeCreated bool
			var runtimeCreated bool
			rollback := func() {
				_ = RunStore{Root: s.Root}.DeleteRun(ctx, run.RunID)
				if runtimeCreated {
					_ = os.RemoveAll(runtimeDir)
				}
				if worktreeCreated {
					_ = worktrees.Remove(ctx, run, true)
				}
				_ = s.UpdateTicket(ctx, originalTicket)
			}
			if err := s.Runs.SaveRun(ctx, run); err != nil {
				return err
			}
			if testBeforeRunWorktreeCreateHook != nil {
				if err := testBeforeRunWorktreeCreateHook(run); err != nil {
					rollback()
					return err
				}
			}
			if err := worktrees.Create(ctx, run); err != nil {
				rollback()
				return err
			}
			if strings.TrimSpace(run.WorktreePath) != "" {
				worktreeCreated = true
			}
			if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
				rollback()
				return fmt.Errorf("create runtime dir: %w", err)
			}
			runtimeCreated = true
			if err := s.UpdateTicket(ctx, ticket); err != nil {
				rollback()
				return err
			}
			return nil
		}); err != nil {
			return DispatchResult{}, err
		}
		return DispatchResult{TicketID: ticket.ID, RunID: run.RunID, AgentID: agent.AgentID, Runbook: ticket.Runbook, Stage: run.BlueprintStage, WorktreePath: run.WorktreePath, GeneratedAt: s.now()}, nil
	})
}

func (s *ActionService) StartRun(ctx context.Context, runID string, actor contracts.Actor, reason string, summary string) (contracts.RunSnapshot, error) {
	return s.transitionRun(ctx, runID, actor, reason, contracts.EventRunStarted, contracts.RunStatusActive, func(run *contracts.RunSnapshot) {
		if run.StartedAt.IsZero() {
			run.StartedAt = s.now()
		}
		if strings.TrimSpace(summary) != "" {
			run.Summary = strings.TrimSpace(summary)
		}
	})
}

func (s *ActionService) AttachRun(ctx context.Context, runID string, provider contracts.AgentProvider, sessionRef string, replace bool, actor contracts.Actor, reason string) (contracts.RunSnapshot, error) {
	return withWriteLock(ctx, s.LockManager, "attach run session", func(ctx context.Context) (contracts.RunSnapshot, error) {
		if !actor.IsValid() {
			return contracts.RunSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		provider = contracts.AgentProvider(strings.TrimSpace(string(provider)))
		if !provider.IsValid() {
			return contracts.RunSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid provider: %s", provider))
		}
		sessionRef = strings.TrimSpace(sessionRef)
		if sessionRef == "" {
			return contracts.RunSnapshot{}, apperr.New(apperr.CodeInvalidInput, "session_ref is required")
		}
		run, err := s.Runs.LoadRun(ctx, runID)
		if err != nil {
			return contracts.RunSnapshot{}, err
		}
		if run.SessionProvider == provider && run.SessionRef == sessionRef {
			if run.Status == contracts.RunStatusAttached || run.Status == contracts.RunStatusActive || run.Status == contracts.RunStatusHandoffReady || run.Status == contracts.RunStatusAwaitingReview || run.Status == contracts.RunStatusAwaitingOwner {
				return run, nil
			}
		} else if run.SessionRef != "" && !replace {
			return contracts.RunSnapshot{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("run %s already attached to %s/%s", run.RunID, run.SessionProvider, run.SessionRef))
		}
		if run.Status == contracts.RunStatusDispatched {
			run.Status = contracts.RunStatusAttached
		} else if !run.Status.Allows(contracts.RunStatusAttached) && run.Status != contracts.RunStatusAttached && run.Status != contracts.RunStatusActive && run.Status != contracts.RunStatusHandoffReady && run.Status != contracts.RunStatusAwaitingReview && run.Status != contracts.RunStatusAwaitingOwner {
			return contracts.RunSnapshot{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("run %s cannot move from %s to attached", run.RunID, run.Status))
		}
		run.SessionProvider = provider
		run.SessionRef = sessionRef
		event, err := s.newEvent(ctx, run.Project, s.now(), actor, reason, contracts.EventRunAttached, run.TicketID, runMutationPayload{Run: run})
		if err != nil {
			return contracts.RunSnapshot{}, err
		}
		if err := s.commitMutation(ctx, "attach run", "run_snapshot", event, func(ctx context.Context) error {
			return s.Runs.SaveRun(ctx, run)
		}); err != nil {
			return contracts.RunSnapshot{}, err
		}
		return run, nil
	})
}

func (s *ActionService) CompleteRun(ctx context.Context, runID string, actor contracts.Actor, reason string, summary string) (contracts.RunSnapshot, error) {
	return s.finishRun(ctx, runID, actor, reason, contracts.EventRunCompleted, contracts.RunStatusCompleted, "completed", summary)
}

func (s *ActionService) FailRun(ctx context.Context, runID string, actor contracts.Actor, reason string, summary string) (contracts.RunSnapshot, error) {
	return s.finishRun(ctx, runID, actor, reason, contracts.EventRunFailed, contracts.RunStatusFailed, "failed", summary)
}

func (s *ActionService) AbortRun(ctx context.Context, runID string, actor contracts.Actor, reason string, summary string) (contracts.RunSnapshot, error) {
	return s.finishRun(ctx, runID, actor, reason, contracts.EventRunAborted, contracts.RunStatusAborted, "aborted", summary)
}

func (s *ActionService) CleanupRun(ctx context.Context, runID string, force bool, actor contracts.Actor, reason string) (contracts.RunSnapshot, error) {
	return withWriteLock(ctx, s.LockManager, "cleanup run", func(ctx context.Context) (contracts.RunSnapshot, error) {
		if !actor.IsValid() {
			return contracts.RunSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		run, err := s.Runs.LoadRun(ctx, runID)
		if err != nil {
			return contracts.RunSnapshot{}, err
		}
		if run.Status != contracts.RunStatusCompleted && run.Status != contracts.RunStatusFailed && run.Status != contracts.RunStatusAborted {
			return contracts.RunSnapshot{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("run %s must be completed, failed, or aborted before cleanup", run.RunID))
		}
		worktrees := WorktreeManager{Root: s.Root}
		if err := worktrees.Remove(ctx, run, force); err != nil {
			return contracts.RunSnapshot{}, err
		}
		if err := os.RemoveAll(storage.RuntimeDir(s.Root, run.RunID)); err != nil {
			return contracts.RunSnapshot{}, fmt.Errorf("remove runtime dir: %w", err)
		}
		run.Status = contracts.RunStatusCleanedUp
		event, err := s.newEvent(ctx, run.Project, s.now(), actor, reason, contracts.EventRunCleanedUp, run.TicketID, runMutationPayload{Run: run})
		if err != nil {
			return contracts.RunSnapshot{}, err
		}
		if err := s.commitMutation(ctx, "cleanup run", "run_snapshot", event, func(ctx context.Context) error {
			return s.Runs.SaveRun(ctx, run)
		}); err != nil {
			return contracts.RunSnapshot{}, err
		}
		return run, nil
	})
}

func (s *ActionService) transitionRun(ctx context.Context, runID string, actor contracts.Actor, reason string, eventType contracts.EventType, next contracts.RunStatus, mutate func(*contracts.RunSnapshot)) (contracts.RunSnapshot, error) {
	return withWriteLock(ctx, s.LockManager, "transition run", func(ctx context.Context) (contracts.RunSnapshot, error) {
		if !actor.IsValid() {
			return contracts.RunSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		run, err := s.Runs.LoadRun(ctx, runID)
		if err != nil {
			return contracts.RunSnapshot{}, err
		}
		if next == contracts.RunStatusActive {
			ticket, err := s.Tickets.GetTicket(ctx, run.TicketID)
			if err != nil {
				return contracts.RunSnapshot{}, err
			}
			if len(ticket.OpenGateIDs) > 0 {
				return contracts.RunSnapshot{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("run %s cannot resume while gates are open", run.RunID))
			}
		}
		if run.Status != next && !run.Status.Allows(next) {
			return contracts.RunSnapshot{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("run %s cannot move from %s to %s", run.RunID, run.Status, next))
		}
		run.Status = next
		if mutate != nil {
			mutate(&run)
		}
		event, err := s.newEvent(ctx, run.Project, s.now(), actor, reason, eventType, run.TicketID, runMutationPayload{Run: run})
		if err != nil {
			return contracts.RunSnapshot{}, err
		}
		if err := s.commitMutation(ctx, "transition run", "run_snapshot", event, func(ctx context.Context) error {
			return s.Runs.SaveRun(ctx, run)
		}); err != nil {
			return contracts.RunSnapshot{}, err
		}
		return run, nil
	})
}

func (s *ActionService) finishRun(ctx context.Context, runID string, actor contracts.Actor, reason string, eventType contracts.EventType, next contracts.RunStatus, result string, summary string) (contracts.RunSnapshot, error) {
	return withWriteLock(ctx, s.LockManager, "finish run", func(ctx context.Context) (contracts.RunSnapshot, error) {
		run, err := s.Runs.LoadRun(ctx, runID)
		if err != nil {
			return contracts.RunSnapshot{}, err
		}
		if next == contracts.RunStatusCompleted {
			ticket, err := s.Tickets.GetTicket(ctx, run.TicketID)
			if err != nil {
				return contracts.RunSnapshot{}, err
			}
			if len(ticket.OpenGateIDs) > 0 {
				return contracts.RunSnapshot{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("run %s cannot complete while gates are open", run.RunID))
			}
			agent, _ := s.Agents.LoadAgent(ctx, run.AgentID)
			changedFiles, known := permissionChangedFilesForRun(ctx, s.Runs, s.Root, run)
			if _, err := s.requirePermission(ctx, permissionEvalInput{
				Action:            contracts.PermissionActionRunComplete,
				Actor:             actor,
				Ticket:            ticket,
				Run:               &run,
				ActorAgent:        maybeAgentProfile(agent),
				Runbook:           permissionRunbook(ticket, &run),
				ChangedFiles:      changedFiles,
				ChangedFilesKnown: known,
			}); err != nil {
				return contracts.RunSnapshot{}, err
			}
		}
		return s.transitionRun(ctx, runID, actor, reason, eventType, next, func(run *contracts.RunSnapshot) {
			run.CompletedAt = s.now()
			run.Result = result
			if strings.TrimSpace(summary) != "" {
				run.Summary = strings.TrimSpace(summary)
			}
		})
	})
}

func (s *ActionService) agentEligibility(ctx context.Context, ticket contracts.TicketSnapshot, agentID string) (AgentEligibilityEntry, error) {
	report, err := NewQueryService(s.Root, s.Projects, s.Tickets, s.Events, s.Projection, s.Clock).AgentEligibility(ctx, ticket.ID)
	if err != nil {
		return AgentEligibilityEntry{}, err
	}
	for _, entry := range report.Entries {
		if entry.Agent.AgentID == agentID {
			return entry, nil
		}
	}
	return AgentEligibilityEntry{}, apperr.New(apperr.CodeNotFound, fmt.Sprintf("agent %s not found in eligibility report", agentID))
}

func nextRunID(ctx context.Context, store contracts.RunStore, now time.Time) (string, error) {
	base := fmt.Sprintf("run_%x", now.UTC().UnixNano())
	if store == nil {
		return base, nil
	}
	runs, err := store.ListRuns(ctx, "")
	if err != nil {
		return "", err
	}
	seen := make(map[string]struct{}, len(runs))
	for _, run := range runs {
		seen[run.RunID] = struct{}{}
	}
	if _, ok := seen[base]; !ok {
		return base, nil
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s_%d", base, i)
		if _, ok := seen[candidate]; !ok {
			return candidate, nil
		}
	}
}

func runIsActive(status contracts.RunStatus) bool {
	switch status {
	case contracts.RunStatusDispatched,
		contracts.RunStatusAttached,
		contracts.RunStatusActive,
		contracts.RunStatusHandoffReady,
		contracts.RunStatusAwaitingReview,
		contracts.RunStatusAwaitingOwner:
		return true
	default:
		return false
	}
}

func sortRuns(runs []contracts.RunSnapshot) {
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].CreatedAt.Equal(runs[j].CreatedAt) {
			return runs[i].RunID < runs[j].RunID
		}
		return runs[i].CreatedAt.Before(runs[j].CreatedAt)
	})
}

func removeRunRuntimeArtifacts(root string, runID string) error {
	path := storage.RuntimeDir(root, runID)
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove runtime dir %s: %w", filepath.Base(path), err)
	}
	return nil
}
