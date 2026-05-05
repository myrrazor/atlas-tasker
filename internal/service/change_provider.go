package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type scmDefaults struct {
	BaseBranch string
	Provider   contracts.ChangeProvider
	Repo       string
}

type changeObserver struct {
	projects contracts.ProjectStore
	root     string
	runs     contracts.RunStore
}

func (s *ActionService) CreateChange(ctx context.Context, runID string, actor contracts.Actor, reason string) (ChangeCreateResultView, error) {
	return withWriteLock(ctx, s.LockManager, "create change", func(ctx context.Context) (ChangeCreateResultView, error) {
		if !actor.IsValid() {
			return ChangeCreateResultView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		run, err := s.Runs.LoadRun(ctx, runID)
		if err != nil {
			return ChangeCreateResultView{}, err
		}
		ticket, err := s.Tickets.GetTicket(ctx, run.TicketID)
		if err != nil {
			return ChangeCreateResultView{}, err
		}
		defaults, err := resolveSCMDefaults(ctx, s.Root, s.Projects, ticket.Project)
		if err != nil {
			return ChangeCreateResultView{}, err
		}
		scm, branch, _, err := changeSCMTarget(ctx, s.Runs, s.Root, contracts.ChangeRef{RunID: run.RunID, BranchName: run.BranchName})
		if err != nil {
			return ChangeCreateResultView{}, err
		}
		if strings.TrimSpace(branch) == "" {
			return ChangeCreateResultView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("run %s does not have a checked-out branch", run.RunID))
		}
		agent, _ := s.Agents.LoadAgent(ctx, run.AgentID)
		changedFiles, known := permissionChangedFilesForRun(ctx, s.Runs, s.Root, run)
		if _, err := s.requirePermission(ctx, permissionEvalInput{
			Action:            contracts.PermissionActionChangeCreate,
			Actor:             actor,
			Ticket:            ticket,
			Run:               &run,
			ActorAgent:        maybeAgentProfile(agent),
			Runbook:           permissionRunbook(ticket, &run),
			ChangedFiles:      changedFiles,
			ChangedFilesKnown: known,
		}); err != nil {
			return ChangeCreateResultView{}, err
		}
		existing, found, err := linkedChangeForRun(ctx, s.Changes, ticket, run.RunID)
		if err != nil {
			return ChangeCreateResultView{}, err
		}
		change := existing
		if !found {
			change = contracts.ChangeRef{
				ChangeID:      "change_" + NewOpaqueID(),
				CreatedAt:     s.now(),
				SchemaVersion: contracts.CurrentSchemaVersion,
				Status:        contracts.ChangeStatusLocalOnly,
			}
		}
		change.Provider = firstNonEmptyProvider(change.Provider, defaults.Provider)
		change.TicketID = ticket.ID
		change.RunID = run.RunID
		change.BranchName = firstNonEmpty(change.BranchName, branch)
		change.HeadRef = firstNonEmpty(change.HeadRef, branch)
		change.BaseBranch = firstNonEmpty(change.BaseBranch, defaults.BaseBranch)
		if change.Status == "" {
			change.Status = contracts.ChangeStatusLocalOnly
		}
		if _, err := scm.RepoStatus(ctx); err != nil {
			return ChangeCreateResultView{}, err
		}
		eventType := contracts.EventChangeCreated
		purpose := "create change"
		if found {
			eventType = contracts.EventChangeUpdated
			purpose = "refresh change"
		}
		saved, ticket, err := s.upsertLinkedChangeLocked(ctx, ticket, change, actor, reason, eventType, purpose)
		if err != nil {
			return ChangeCreateResultView{}, err
		}
		reasons := []string{}
		if found {
			reasons = append(reasons, "existing_change_reused")
		}
		return ChangeCreateResultView{
			Change:      saved,
			Created:     !found,
			ReasonCodes: reasons,
			Ticket:      ticket,
			GeneratedAt: s.now(),
		}, nil
	})
}

func (s *ActionService) ImportChangeURL(ctx context.Context, ticketID string, rawURL string, actor contracts.Actor, reason string) (ChangeCreateResultView, error) {
	return withWriteLock(ctx, s.LockManager, "import change url", func(ctx context.Context) (ChangeCreateResultView, error) {
		if !actor.IsValid() {
			return ChangeCreateResultView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		ticket, err := s.Tickets.GetTicket(ctx, ticketID)
		if err != nil {
			return ChangeCreateResultView{}, err
		}
		defaults, err := resolveSCMDefaults(ctx, s.Root, s.Projects, ticket.Project)
		if err != nil {
			return ChangeCreateResultView{}, err
		}
		gh := GHService{Root: s.Root, Repo: defaults.Repo}
		cleanURL, number, err := gh.ImportPullRequestURL(rawURL)
		if err != nil {
			return ChangeCreateResultView{}, apperr.New(apperr.CodeInvalidInput, err.Error())
		}
		reasons := []string{}
		change, found, err := linkedGitHubChange(ctx, s.Changes, ticket, cleanURL, number)
		if err != nil {
			return ChangeCreateResultView{}, err
		}
		if !found {
			change = contracts.ChangeRef{
				ChangeID:      "change_" + NewOpaqueID(),
				CreatedAt:     s.now(),
				SchemaVersion: contracts.CurrentSchemaVersion,
			}
		}
		change.Provider = contracts.ChangeProviderGitHub
		change.TicketID = ticket.ID
		change.URL = cleanURL
		change.ExternalID = strconv.Itoa(number)
		change.BaseBranch = firstNonEmpty(change.BaseBranch, defaults.BaseBranch)
		capability, err := gh.Capability(ctx)
		if err != nil {
			return ChangeCreateResultView{}, err
		}
		switch {
		case !capability.Installed:
			reasons = append(reasons, "provider_unavailable")
			change.Status = firstNonEmptyChangeStatus(change.Status, contracts.ChangeStatusOpen)
		case !capability.Authenticated:
			reasons = append(reasons, "provider_unauthenticated")
			change.Status = firstNonEmptyChangeStatus(change.Status, contracts.ChangeStatusOpen)
		case true:
			if pr, err := gh.PullRequestView(ctx, cleanURL); err == nil {
				change.BranchName = firstNonEmpty(change.BranchName, pr.HeadRef)
				change.HeadRef = firstNonEmpty(change.HeadRef, pr.HeadRef)
				change.BaseBranch = firstNonEmpty(change.BaseBranch, pr.BaseRef)
				checkViews, checksErr := gh.PullRequestChecks(ctx, cleanURL)
				checkAggregate := contracts.CheckAggregateUnknown
				if checksErr == nil {
					checkAggregate = aggregateGitHubChecks(checkViews)
				} else if isBenignGHRepoError(checksErr) {
					reasons = append(reasons, "provider_checks_unavailable")
				} else {
					return ChangeCreateResultView{}, checksErr
				}
				change.Status = observedStatusFromGitHub(change, pr, checkAggregate)
				change.ChecksStatus = checkAggregate
			} else if isBenignGHRepoError(err) {
				reasons = append(reasons, "provider_pull_missing")
				change.Status = firstNonEmptyChangeStatus(change.Status, contracts.ChangeStatusOpen)
			} else {
				return ChangeCreateResultView{}, err
			}
		}
		eventType := contracts.EventChangeCreated
		purpose := "import change url"
		if found {
			eventType = contracts.EventChangeUpdated
			purpose = "refresh imported change url"
		}
		saved, ticket, err := s.upsertLinkedChangeLocked(ctx, ticket, change, actor, reason, eventType, purpose)
		if err != nil {
			return ChangeCreateResultView{}, err
		}
		if found {
			reasons = append(reasons, "existing_change_reused")
		}
		return ChangeCreateResultView{
			Change:      saved,
			Created:     !found,
			ReasonCodes: dedupeStrings(reasons),
			Ticket:      ticket,
			GeneratedAt: s.now(),
		}, nil
	})
}

func (s *ActionService) SyncChange(ctx context.Context, changeID string, actor contracts.Actor, reason string) (ChangeStatusView, error) {
	return withWriteLock(ctx, s.LockManager, "sync change", func(ctx context.Context) (ChangeStatusView, error) {
		if !actor.IsValid() {
			return ChangeStatusView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		change, ticket, observed, err := s.observeChange(ctx, changeID)
		if err != nil {
			return ChangeStatusView{}, err
		}
		nextStatus, drifted := reconcileObservedChangeStatus(change.Status, observed.ObservedStatus)
		if drifted {
			observed.ReasonCodes = append(observed.ReasonCodes, "provider_state_conflict")
			nextStatus = contracts.ChangeStatusExternalDrifted
		}
		change.Status = nextStatus
		change.ChecksStatus = observed.ObservedChecksStatus
		change.BranchName = firstNonEmpty(change.BranchName, observed.CurrentBranch)
		if observed.PullRequest != nil {
			change.URL = firstNonEmpty(change.URL, observed.PullRequest.URL)
			change.ExternalID = firstNonEmpty(change.ExternalID, strconv.Itoa(observed.PullRequest.Number))
			change.HeadRef = firstNonEmpty(change.HeadRef, observed.PullRequest.HeadRef)
			change.BranchName = firstNonEmpty(change.BranchName, observed.PullRequest.HeadRef)
			change.BaseBranch = firstNonEmpty(change.BaseBranch, observed.PullRequest.BaseRef)
		}
		eventType := contracts.EventChangeSynced
		purpose := "sync change"
		if drifted {
			eventType = contracts.EventChangeExternalDrifted
			purpose = "record change drift"
		}
		saved, updatedTicket, err := s.upsertLinkedChangeLocked(ctx, ticket, change, actor, reason, eventType, purpose)
		if err != nil {
			return ChangeStatusView{}, err
		}
		observed.Change = saved
		observed.Ticket = updatedTicket
		observed.GeneratedAt = s.now()
		observed.ReasonCodes = dedupeStrings(observed.ReasonCodes)
		return observed, nil
	})
}

func (s *ActionService) SyncChangeChecks(ctx context.Context, changeID string, actor contracts.Actor, reason string) (CheckSyncResultView, error) {
	return withWriteLock(ctx, s.LockManager, "sync change checks", func(ctx context.Context) (CheckSyncResultView, error) {
		if !actor.IsValid() {
			return CheckSyncResultView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		change, ticket, observed, err := s.observeChange(ctx, changeID)
		if err != nil {
			return CheckSyncResultView{}, err
		}
		if change.Provider != contracts.ChangeProviderGitHub || observed.PullRequest == nil {
			return CheckSyncResultView{
				Aggregate:   change.ChecksStatus,
				Change:      change,
				ReasonCodes: dedupeStrings(append(observed.ReasonCodes, "provider_pull_missing")),
				GeneratedAt: s.now(),
			}, nil
		}
		defaults, err := resolveSCMDefaults(ctx, s.Root, s.Projects, ticket.Project)
		if err != nil {
			return CheckSyncResultView{}, err
		}
		scmRoot, _, _, err := changeSCMTarget(ctx, s.Runs, s.Root, change)
		if err != nil {
			return CheckSyncResultView{}, err
		}
		gh := GHService{Root: scmRoot.Root, Repo: defaults.Repo}
		checkViews, err := gh.PullRequestChecks(ctx, observed.PullRequest.URL)
		if err != nil {
			if isBenignGHRepoError(err) {
				return CheckSyncResultView{
					Aggregate:   change.ChecksStatus,
					Change:      change,
					ReasonCodes: dedupeStrings(append(observed.ReasonCodes, "provider_checks_unavailable")),
					GeneratedAt: s.now(),
				}, nil
			}
			return CheckSyncResultView{}, err
		}
		now := s.now()
		checks := make([]contracts.CheckResult, 0, len(checkViews))
		for _, item := range checkViews {
			checks = append(checks, providerCheckResult(change, item, now))
		}
		aggregate := aggregateChecks(checks)
		nextStatus, drifted := reconcileObservedChangeStatus(change.Status, observedStatusFromGitHub(change, *observed.PullRequest, aggregate))
		if drifted {
			nextStatus = contracts.ChangeStatusExternalDrifted
		}
		change.Status = nextStatus
		change.ChecksStatus = aggregate
		change.UpdatedAt = now
		ticket.UpdatedAt = now
		if err := s.previewTicketChangeStateWithPendingChecks(ctx, &ticket, &change, checks); err != nil {
			return CheckSyncResultView{}, err
		}
		event, err := s.newEvent(ctx, ticket.Project, now, actor, reason, contracts.EventCheckSynced, ticket.ID, map[string]any{
			"ticket": ticket,
			"change": change,
			"checks": checks,
		})
		if err != nil {
			return CheckSyncResultView{}, err
		}
		if err := s.commitMutation(ctx, "sync change checks", "check_result", event, func(ctx context.Context) error {
			for _, check := range checks {
				if err := s.Checks.SaveCheck(ctx, check); err != nil {
					return err
				}
			}
			if err := s.Changes.SaveChange(ctx, change); err != nil {
				return err
			}
			return s.UpdateTicket(ctx, ticket)
		}); err != nil {
			return CheckSyncResultView{}, err
		}
		return CheckSyncResultView{
			Aggregate:   aggregate,
			Change:      change,
			Checks:      checks,
			ReasonCodes: dedupeStrings(observed.ReasonCodes),
			GeneratedAt: now,
		}, nil
	})
}

func (s *ActionService) RequestChangeReview(ctx context.Context, changeID string, actor contracts.Actor, reason string) (ChangeStatusView, error) {
	return withWriteLock(ctx, s.LockManager, "request change review", func(ctx context.Context) (ChangeStatusView, error) {
		if !actor.IsValid() {
			return ChangeStatusView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		change, ticket, observed, err := s.observeChange(ctx, changeID)
		if err != nil {
			return ChangeStatusView{}, err
		}
		if err := requireChangeReviewAuthority(ctx, s.Runs, change, actor); err != nil {
			return ChangeStatusView{}, err
		}
		if change.Provider != contracts.ChangeProviderGitHub {
			return ChangeStatusView{}, apperr.New(apperr.CodeConflict, "change review request requires a github-backed change")
		}
		if observed.PullRequest == nil {
			return ChangeStatusView{}, apperr.New(apperr.CodeConflict, "change review request blocked: provider_pull_missing")
		}
		switch observed.ObservedStatus {
		case contracts.ChangeStatusMerged, contracts.ChangeStatusClosed, contracts.ChangeStatusSuperseded:
			return ChangeStatusView{}, apperr.New(apperr.CodeConflict, "change review request blocked: change_not_reviewable")
		}
		reviewTargets := reviewRequestTargets(ticket)
		if len(reviewTargets) > 0 {
			change.ReviewRequestedFrom = reviewTargets
		}
		change.ReviewSummary = strings.TrimSpace(firstNonEmpty(change.ReviewSummary, reason))

		defaults, err := resolveSCMDefaults(ctx, s.Root, s.Projects, ticket.Project)
		if err != nil {
			return ChangeStatusView{}, err
		}
		scmRoot, _, _, err := changeSCMTarget(ctx, s.Runs, s.Root, change)
		if err != nil {
			return ChangeStatusView{}, err
		}
		gh := GHService{Root: scmRoot.Root, Repo: defaults.Repo}
		if _, err := gh.RequestPullRequestReview(ctx, observed.PullRequest.URL); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "not authenticated") {
				return ChangeStatusView{}, apperr.New(apperr.CodeConflict, "change review request blocked: provider_unauthenticated")
			}
			if strings.Contains(strings.ToLower(err.Error()), "not installed") {
				return ChangeStatusView{}, apperr.New(apperr.CodeConflict, "change review request blocked: provider_unavailable")
			}
			return ChangeStatusView{}, err
		}
		return s.persistObservedChangeAfterProviderWrite(ctx, change, ticket, actor, reason, contracts.EventChangeReviewRequested, "request change review", "review_requested")
	})
}

func (s *ActionService) MergeChange(ctx context.Context, changeID string, actor contracts.Actor, reason string) (ChangeStatusView, error) {
	return withWriteLock(ctx, s.LockManager, "merge change", func(ctx context.Context) (ChangeStatusView, error) {
		if !actor.IsValid() {
			return ChangeStatusView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		change, ticket, observed, err := s.observeChange(ctx, changeID)
		if err != nil {
			return ChangeStatusView{}, err
		}
		if err := requireChangeMergeAuthority(ticket, actor); err != nil {
			return ChangeStatusView{}, err
		}
		if change.Provider != contracts.ChangeProviderGitHub {
			return ChangeStatusView{}, apperr.New(apperr.CodeConflict, "change merge requires a github-backed change")
		}
		if observed.PullRequest == nil {
			return ChangeStatusView{}, apperr.New(apperr.CodeConflict, "change merge blocked: provider_pull_missing")
		}
		var run *contracts.RunSnapshot
		if strings.TrimSpace(change.RunID) != "" {
			loadedRun, err := s.Runs.LoadRun(ctx, change.RunID)
			if err == nil {
				run = &loadedRun
			}
		}
		var relevantAgent *contracts.AgentProfile
		if run != nil {
			loadedAgent, err := s.Agents.LoadAgent(ctx, run.AgentID)
			if err == nil {
				relevantAgent = &loadedAgent
			}
		}
		changedFiles, known := permissionChangedFilesForChange(ctx, s.Runs, s.Root, change)
		if _, err := s.requirePermission(ctx, permissionEvalInput{
			Action:            contracts.PermissionActionChangeMerge,
			Actor:             actor,
			Ticket:            ticket,
			Run:               run,
			Change:            &change,
			ActorAgent:        relevantAgent,
			Runbook:           permissionRunbook(ticket, run),
			ChangedFiles:      changedFiles,
			ChangedFilesKnown: known,
		}); err != nil {
			return ChangeStatusView{}, err
		}
		candidateTicket := ticket
		candidateChange := change
		candidateChange.Status = observed.ObservedStatus
		candidateChange.ChecksStatus = observed.ObservedChecksStatus
		if err := s.previewTicketChangeState(ctx, &candidateTicket, &candidateChange, nil); err != nil {
			return ChangeStatusView{}, err
		}
		if observed.ObservedStatus != contracts.ChangeStatusMerged && candidateTicket.ChangeReadyState != contracts.ChangeReadyMergeReady {
			return ChangeStatusView{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("change merge blocked: %s", candidateTicket.ChangeReadyState))
		}
		if observed.ObservedStatus == contracts.ChangeStatusMerged {
			return s.persistObservedChangeAfterProviderWrite(ctx, change, ticket, actor, reason, contracts.EventChangeMerged, "record merged change", "already_merged")
		}

		defaults, err := resolveSCMDefaults(ctx, s.Root, s.Projects, ticket.Project)
		if err != nil {
			return ChangeStatusView{}, err
		}
		scmRoot, _, _, err := changeSCMTarget(ctx, s.Runs, s.Root, change)
		if err != nil {
			return ChangeStatusView{}, err
		}
		gh := GHService{Root: scmRoot.Root, Repo: defaults.Repo}
		if _, err := gh.MergePullRequest(ctx, observed.PullRequest.URL); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "not authenticated") {
				return ChangeStatusView{}, apperr.New(apperr.CodeConflict, "change merge blocked: provider_unauthenticated")
			}
			if strings.Contains(strings.ToLower(err.Error()), "not installed") {
				return ChangeStatusView{}, apperr.New(apperr.CodeConflict, "change merge blocked: provider_unavailable")
			}
			return ChangeStatusView{}, err
		}
		return s.persistObservedChangeAfterProviderWrite(ctx, change, ticket, actor, reason, contracts.EventChangeMerged, "merge change", "merged")
	})
}

func (s *QueryService) ChangeStatus(ctx context.Context, changeID string) (ChangeStatusView, error) {
	change, err := s.Changes.LoadChange(ctx, changeID)
	if err != nil {
		return ChangeStatusView{}, err
	}
	ticket, err := s.Tickets.GetTicket(ctx, change.TicketID)
	if err != nil {
		return ChangeStatusView{}, err
	}
	observer := changeObserver{root: s.Root, projects: s.Projects, runs: s.Runs}
	view, err := observer.observe(ctx, change, ticket)
	if err != nil {
		return ChangeStatusView{}, err
	}
	view.GeneratedAt = s.now()
	return view, nil
}

func (s *ActionService) observeChange(ctx context.Context, changeID string) (contracts.ChangeRef, contracts.TicketSnapshot, ChangeStatusView, error) {
	change, err := s.Changes.LoadChange(ctx, changeID)
	if err != nil {
		return contracts.ChangeRef{}, contracts.TicketSnapshot{}, ChangeStatusView{}, err
	}
	ticket, err := s.Tickets.GetTicket(ctx, change.TicketID)
	if err != nil {
		return contracts.ChangeRef{}, contracts.TicketSnapshot{}, ChangeStatusView{}, err
	}
	observer := changeObserver{root: s.Root, projects: s.Projects, runs: s.Runs}
	view, err := observer.observe(ctx, change, ticket)
	if err != nil {
		return contracts.ChangeRef{}, contracts.TicketSnapshot{}, ChangeStatusView{}, err
	}
	return change, ticket, view, nil
}

func (s *ActionService) persistObservedChangeAfterProviderWrite(ctx context.Context, change contracts.ChangeRef, ticket contracts.TicketSnapshot, actor contracts.Actor, reason string, eventType contracts.EventType, purpose string, extraReasons ...string) (ChangeStatusView, error) {
	observer := changeObserver{root: s.Root, projects: s.Projects, runs: s.Runs}
	view, err := observer.observe(ctx, change, ticket)
	if err != nil {
		return ChangeStatusView{}, err
	}
	nextStatus, drifted := reconcileObservedChangeStatus(change.Status, view.ObservedStatus)
	if drifted {
		view.ReasonCodes = append(view.ReasonCodes, "provider_state_conflict")
		nextStatus = contracts.ChangeStatusExternalDrifted
		eventType = contracts.EventChangeExternalDrifted
		purpose = "record change drift"
	}
	change.Status = nextStatus
	change.ChecksStatus = view.ObservedChecksStatus
	change.UpdatedAt = s.now()
	if view.PullRequest != nil {
		change.URL = firstNonEmpty(strings.TrimSpace(view.PullRequest.URL), change.URL)
		change.ExternalID = firstNonEmpty(strconv.Itoa(view.PullRequest.Number), change.ExternalID)
		change.HeadRef = firstNonEmpty(strings.TrimSpace(view.PullRequest.HeadRef), change.HeadRef)
		change.BranchName = firstNonEmpty(strings.TrimSpace(view.PullRequest.HeadRef), change.BranchName)
		change.BaseBranch = firstNonEmpty(strings.TrimSpace(view.PullRequest.BaseRef), change.BaseBranch)
	}
	saved, updatedTicket, err := s.upsertLinkedChangeLocked(ctx, ticket, change, actor, reason, eventType, purpose)
	if err != nil {
		return ChangeStatusView{}, err
	}
	view.Change = saved
	view.Ticket = updatedTicket
	view.GeneratedAt = s.now()
	view.ReasonCodes = dedupeStrings(append(view.ReasonCodes, extraReasons...))
	return view, nil
}

func (o changeObserver) observe(ctx context.Context, change contracts.ChangeRef, ticket contracts.TicketSnapshot) (ChangeStatusView, error) {
	defaults, err := resolveSCMDefaults(ctx, o.root, o.projects, ticket.Project)
	if err != nil {
		return ChangeStatusView{}, err
	}
	scm, branch, currentBranch, err := changeSCMTarget(ctx, o.runs, o.root, change)
	if err != nil {
		return ChangeStatusView{}, err
	}
	gitContext, err := scm.ContextForTicket(ctx, ticket)
	if err != nil {
		return ChangeStatusView{}, err
	}
	changedFiles, err := observedChangedFiles(ctx, scm, change, branch, currentBranch)
	if err != nil {
		return ChangeStatusView{}, err
	}
	branchExists, err := scm.BranchExists(ctx, branch)
	if err != nil {
		return ChangeStatusView{}, err
	}
	reasons := []string{}
	detached := false
	if gitContext.Repo.Present && strings.TrimSpace(currentBranch) == "" {
		detached = true
		reasons = append(reasons, "detached_head")
	}
	if gitContext.Repo.Present {
		if dirty, err := worktreeDirtySourceRepo(ctx, scm); err == nil && dirty {
			reasons = append(reasons, "dirty_repo")
		}
		if err := scm.ensureNoNestedRepoAmbiguity(gitContext.Repo.Root); err != nil {
			reasons = append(reasons, "nested_repo_ambiguity")
		}
	} else {
		reasons = append(reasons, "repo_missing")
	}
	if branch != "" && !branchExists {
		reasons = append(reasons, "local_branch_missing")
	}
	observedStatus := firstNonEmptyChangeStatus(change.Status, contracts.ChangeStatusLocalOnly)
	observedChecks := firstNonEmptyCheckAggregate(change.ChecksStatus, contracts.CheckAggregateUnknown)
	var pull *GitHubPRView
	if firstNonEmptyProvider(change.Provider, defaults.Provider) == contracts.ChangeProviderGitHub || change.URL != "" || change.ExternalID != "" {
		gh := GHService{Root: scm.Root, Repo: defaults.Repo}
		capability, err := gh.Capability(ctx)
		if err != nil {
			return ChangeStatusView{}, err
		}
		gitContext.GitHub.Capability = capability
		gitContext.GitHub.SuggestedTitle = fmt.Sprintf("%s: %s", ticket.ID, strings.TrimSpace(ticket.Title))
		switch {
		case !capability.Installed:
			reasons = append(reasons, "provider_unavailable")
		case !capability.Authenticated:
			reasons = append(reasons, "provider_unauthenticated")
		default:
			pr, matches, err := gh.lookupPullRequest(ctx, change, ticket.ID, branch)
			if err != nil {
				if !isBenignGHRepoError(err) {
					return ChangeStatusView{}, err
				}
			} else {
				gitContext.GitHub.PullRequests = matches
				if pr != nil {
					pull = pr
					checkViews, err := gh.PullRequestChecks(ctx, pr.URL)
					if err == nil {
						observedChecks = aggregateGitHubChecks(checkViews)
					} else if !isBenignGHRepoError(err) {
						return ChangeStatusView{}, err
					} else {
						reasons = append(reasons, "provider_checks_unavailable")
					}
					observedStatus = observedStatusFromGitHub(change, *pr, observedChecks)
				} else {
					reasons = append(reasons, "provider_pull_missing")
				}
			}
		}
	}
	return ChangeStatusView{
		Change:               change,
		ChangedFiles:         changedFiles,
		CurrentBranch:        currentBranch,
		DetachedHEAD:         detached,
		Git:                  gitContext,
		LocalBranchExists:    branch == "" || branchExists,
		ObservedChecksStatus: observedChecks,
		ObservedStatus:       observedStatus,
		PullRequest:          pull,
		ReasonCodes:          dedupeStrings(reasons),
		Ticket:               ticket,
	}, nil
}

func observedChangedFiles(ctx context.Context, scm SCMService, change contracts.ChangeRef, branch string, currentBranch string) ([]string, error) {
	// Imported external PRs should not borrow whatever happens to be dirty in the
	// local workspace root. Only surface local file drift when the change has a
	// dedicated run/worktree or the matching branch is actually checked out here.
	if strings.TrimSpace(change.RunID) == "" {
		if strings.TrimSpace(branch) == "" || strings.TrimSpace(currentBranch) == "" || strings.TrimSpace(branch) != strings.TrimSpace(currentBranch) {
			return []string{}, nil
		}
	}
	return scm.ChangedFiles(ctx)
}

func resolveSCMDefaults(ctx context.Context, root string, projects contracts.ProjectStore, projectKey string) (scmDefaults, error) {
	cfg, err := config.Load(root)
	if err != nil {
		return scmDefaults{}, err
	}
	defaults := scmDefaults{
		Provider:   firstNonEmptyProvider(cfg.Provider.DefaultSCMProvider, contracts.ChangeProviderLocal),
		BaseBranch: firstNonEmpty(strings.TrimSpace(cfg.Provider.DefaultBaseBranch), "main"),
		Repo:       strings.TrimSpace(cfg.Provider.GitHubRepo),
	}
	if strings.TrimSpace(projectKey) == "" {
		return defaults, nil
	}
	project, err := projects.GetProject(ctx, projectKey)
	if err != nil {
		return scmDefaults{}, err
	}
	if project.Defaults.SCMProvider != "" {
		defaults.Provider = project.Defaults.SCMProvider
	}
	if strings.TrimSpace(project.Defaults.SCMBaseBranch) != "" {
		defaults.BaseBranch = strings.TrimSpace(project.Defaults.SCMBaseBranch)
	}
	if strings.TrimSpace(project.Defaults.SCMRepo) != "" {
		defaults.Repo = strings.TrimSpace(project.Defaults.SCMRepo)
	}
	return defaults, nil
}

func changeSCMTarget(ctx context.Context, runs contracts.RunStore, root string, change contracts.ChangeRef) (SCMService, string, string, error) {
	scmRoot := root
	branch := strings.TrimSpace(change.BranchName)
	if strings.TrimSpace(change.RunID) != "" {
		run, err := runs.LoadRun(ctx, change.RunID)
		if err != nil {
			return SCMService{}, "", "", err
		}
		if strings.TrimSpace(run.WorktreePath) != "" {
			scmRoot = run.WorktreePath
		}
		branch = firstNonEmpty(branch, run.BranchName)
	}
	scm := SCMService{Root: scmRoot}
	currentBranch, err := scm.currentBranch(ctx)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "not a git repository") {
		return SCMService{}, "", "", err
	}
	branch = firstNonEmpty(branch, currentBranch)
	return scm, branch, strings.TrimSpace(currentBranch), nil
}

func linkedChangeForRun(ctx context.Context, store contracts.ChangeStore, ticket contracts.TicketSnapshot, runID string) (contracts.ChangeRef, bool, error) {
	changes, err := store.ListChanges(ctx, ticket.ID)
	if err != nil {
		return contracts.ChangeRef{}, false, err
	}
	for _, change := range linkedChanges(ticket, changes) {
		if change.RunID == runID {
			return change, true, nil
		}
	}
	return contracts.ChangeRef{}, false, nil
}

func linkedGitHubChange(ctx context.Context, store contracts.ChangeStore, ticket contracts.TicketSnapshot, changeURL string, number int) (contracts.ChangeRef, bool, error) {
	changes, err := store.ListChanges(ctx, ticket.ID)
	if err != nil {
		return contracts.ChangeRef{}, false, err
	}
	numberText := strconv.Itoa(number)
	for _, change := range linkedChanges(ticket, changes) {
		if strings.EqualFold(strings.TrimSpace(change.URL), strings.TrimSpace(changeURL)) || strings.TrimSpace(change.ExternalID) == numberText {
			return change, true, nil
		}
	}
	return contracts.ChangeRef{}, false, nil
}

func (s GHService) lookupPullRequest(ctx context.Context, change contracts.ChangeRef, ticketID string, branch string) (*GitHubPRView, []GitHubPRView, error) {
	ref := strings.TrimSpace(change.URL)
	switch {
	case ref != "":
		pr, err := s.PullRequestView(ctx, ref)
		if err != nil {
			return nil, nil, err
		}
		return &pr, []GitHubPRView{pr}, nil
	case strings.TrimSpace(change.ExternalID) != "":
		pr, err := s.PullRequestView(ctx, strings.TrimSpace(change.ExternalID))
		if err != nil {
			if !isBenignGHRepoError(err) {
				return nil, nil, err
			}
		} else {
			return &pr, []GitHubPRView{pr}, nil
		}
	}
	matches, err := s.PullRequests(ctx, ticketID, branch)
	if err != nil {
		return nil, nil, err
	}
	if len(matches) == 0 {
		return nil, nil, nil
	}
	if branch != "" {
		for i := range matches {
			if matches[i].HeadRef == branch {
				return &matches[i], matches, nil
			}
		}
	}
	return &matches[0], matches, nil
}

func aggregateGitHubChecks(checks []GitHubCheckView) contracts.CheckAggregateState {
	if len(checks) == 0 {
		return contracts.CheckAggregateUnknown
	}
	pending := false
	failing := false
	for _, check := range checks {
		switch strings.ToLower(strings.TrimSpace(check.Bucket)) {
		case "pending":
			pending = true
		case "fail", "cancel":
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

func observedStatusFromGitHub(change contracts.ChangeRef, pr GitHubPRView, checks contracts.CheckAggregateState) contracts.ChangeStatus {
	if !pr.MergedAt.IsZero() {
		return contracts.ChangeStatusMerged
	}
	switch strings.ToLower(strings.TrimSpace(pr.State)) {
	case "closed", "merged":
		if !pr.MergedAt.IsZero() || strings.EqualFold(pr.State, "merged") {
			return contracts.ChangeStatusMerged
		}
		return contracts.ChangeStatusClosed
	}
	if pr.Draft {
		return contracts.ChangeStatusDraft
	}
	switch strings.ToLower(strings.TrimSpace(pr.ReviewDecision)) {
	case "changes_requested":
		return contracts.ChangeStatusChangesRequested
	case "approved":
		if checks == contracts.CheckAggregatePassing {
			return contracts.ChangeStatusMergeReady
		}
		return contracts.ChangeStatusApproved
	case "review_required":
		return contracts.ChangeStatusReviewRequested
	}
	if len(change.ReviewRequestedFrom) > 0 {
		return contracts.ChangeStatusReviewRequested
	}
	return contracts.ChangeStatusOpen
}

func reconcileObservedChangeStatus(current contracts.ChangeStatus, observed contracts.ChangeStatus) (contracts.ChangeStatus, bool) {
	if observed == "" {
		if current == "" {
			return contracts.ChangeStatusLocalOnly, false
		}
		return current, false
	}
	if current == "" || current == observed {
		return observed, false
	}
	switch current {
	case contracts.ChangeStatusMerged, contracts.ChangeStatusClosed, contracts.ChangeStatusSuperseded:
		return contracts.ChangeStatusExternalDrifted, true
	default:
		return observed, false
	}
}

func providerCheckResult(change contracts.ChangeRef, view GitHubCheckView, now time.Time) contracts.CheckResult {
	name := strings.TrimSpace(view.Name)
	if workflow := strings.TrimSpace(view.Workflow); workflow != "" {
		name = workflow + " / " + name
	}
	status := contracts.CheckStatusCompleted
	conclusion := contracts.CheckConclusionSuccess
	switch strings.ToLower(strings.TrimSpace(view.Bucket)) {
	case "pending":
		status = contracts.CheckStatusRunning
		conclusion = contracts.CheckConclusionUnknown
	case "fail":
		conclusion = contracts.CheckConclusionFailure
	case "cancel":
		conclusion = contracts.CheckConclusionCancelled
	case "skipping":
		conclusion = contracts.CheckConclusionSkipped
	}
	check := contracts.CheckResult{
		CheckID:    stableProviderCheckID(change.ChangeID, view.Workflow, view.Name),
		Source:     contracts.CheckSourceProvider,
		Provider:   contracts.ChangeProviderGitHub,
		Scope:      contracts.CheckScopeChange,
		ScopeID:    change.ChangeID,
		Name:       name,
		Status:     status,
		Conclusion: conclusion,
		Summary:    strings.TrimSpace(view.Description),
		URL:        strings.TrimSpace(view.Link),
		ExternalID: firstNonEmpty(strings.TrimSpace(view.Link), strings.TrimSpace(name)),
		UpdatedAt:  now,
	}
	if !view.StartedAt.IsZero() {
		check.StartedAt = view.StartedAt
	}
	if !view.CompletedAt.IsZero() {
		check.CompletedAt = view.CompletedAt
	}
	if status == contracts.CheckStatusRunning && check.StartedAt.IsZero() {
		check.StartedAt = now
	}
	if status == contracts.CheckStatusCompleted && check.CompletedAt.IsZero() {
		check.CompletedAt = now
	}
	return normalizeCheckResult(check)
}

func stableProviderCheckID(changeID string, workflow string, name string) string {
	parts := []string{"github", slugify(changeID)}
	if workflow = slugify(workflow); workflow != "" {
		parts = append(parts, workflow)
	}
	if name = slugify(name); name != "" {
		parts = append(parts, name)
	}
	return "check_" + strings.Join(parts, "_")
}

func requireChangeReviewAuthority(ctx context.Context, runs contracts.RunStore, change contracts.ChangeRef, actor contracts.Actor) error {
	if actor == contracts.Actor("human:owner") {
		return nil
	}
	if strings.TrimSpace(change.RunID) == "" {
		return apperr.New(apperr.CodePermissionDenied, "change review request blocked: review_request_not_authorized")
	}
	run, err := runs.LoadRun(ctx, change.RunID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(run.AgentID) == "" || actor != contracts.Actor(run.AgentID) {
		return apperr.New(apperr.CodePermissionDenied, "change review request blocked: review_request_not_authorized")
	}
	return nil
}

func requireChangeMergeAuthority(ticket contracts.TicketSnapshot, actor contracts.Actor) error {
	if actor == contracts.Actor("human:owner") {
		return nil
	}
	if ticket.Reviewer != "" && actor == ticket.Reviewer {
		return nil
	}
	return apperr.New(apperr.CodePermissionDenied, "change merge blocked: merge_not_authorized")
}

func reviewRequestTargets(ticket contracts.TicketSnapshot) []contracts.Actor {
	if ticket.Reviewer != "" {
		return []contracts.Actor{ticket.Reviewer}
	}
	return []contracts.Actor{contracts.Actor("human:owner")}
}

func firstNonEmptyProvider(values ...contracts.ChangeProvider) contracts.ChangeProvider {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return contracts.ChangeProviderLocal
}

func firstNonEmptyChangeStatus(values ...contracts.ChangeStatus) contracts.ChangeStatus {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return contracts.ChangeStatusLocalOnly
}

func firstNonEmptyCheckAggregate(values ...contracts.CheckAggregateState) contracts.CheckAggregateState {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return contracts.CheckAggregateUnknown
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(values))
	compacted := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		compacted = append(compacted, value)
	}
	sort.Strings(compacted)
	return compacted
}
