package service

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func (s *QueryService) Dashboard(ctx context.Context, collaboratorID string) (DashboardSummaryView, error) {
	filterID := strings.TrimSpace(collaboratorID)
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

	view := DashboardSummaryView{GeneratedAt: s.now(), CollaboratorFilter: filterID}
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

	approvals, err := s.Approvals(ctx, filterID)
	if err != nil {
		return DashboardSummaryView{}, err
	}
	inbox, err := s.Inbox(ctx, filterID)
	if err != nil {
		return DashboardSummaryView{}, err
	}
	mentions, err := s.Mentions.ListMentions(ctx, filterID)
	if err != nil {
		return DashboardSummaryView{}, err
	}
	view.CollaboratorWorkload, err = s.dashboardCollaboratorWorkload(ctx, filterID, approvals, inbox, mentions)
	if err != nil {
		return DashboardSummaryView{}, err
	}
	view.MentionQueue = mentionQueueEntries(mentions)

	conflicts, err := s.Conflicts.ListConflicts(ctx)
	if err != nil {
		return DashboardSummaryView{}, err
	}
	view.ConflictQueue, err = s.dashboardConflictQueue(ctx, conflicts)
	if err != nil {
		return DashboardSummaryView{}, err
	}

	remotes, err := s.SyncRemotes.ListSyncRemotes(ctx)
	if err != nil {
		return DashboardSummaryView{}, err
	}
	jobs, err := s.SyncJobs.ListSyncJobs(ctx, "")
	if err != nil {
		return DashboardSummaryView{}, err
	}
	view.RemoteHealth, view.FailedSyncJobs, err = s.dashboardRemoteHealth(ctx, remotes, jobs)
	if err != nil {
		return DashboardSummaryView{}, err
	}
	view.ProviderMappingWarnings, err = s.dashboardProviderWarnings(ctx)
	if err != nil {
		return DashboardSummaryView{}, err
	}

	sort.Strings(view.StaleWorktrees)
	sort.Strings(view.RetentionTargets)
	sort.Strings(view.FailedSyncJobs)
	view.ProviderMappingWarnings = uniqueStrings(view.ProviderMappingWarnings)
	return view, nil
}

func (s *QueryService) Timeline(ctx context.Context, ticketID string, collaboratorID string) (TimelineView, error) {
	filterID := strings.TrimSpace(collaboratorID)
	workspaceID, err := loadWorkspaceIdentity(s.Root)
	if err != nil {
		return TimelineView{}, err
	}
	detail, err := s.TicketDetail(ctx, ticketID)
	if err != nil {
		return TimelineView{}, err
	}
	entries := make([]TimelineEntry, 0, len(detail.History)+8)
	for _, event := range detail.History {
		entries = append(entries, TimelineEntry{
			Timestamp:  event.Timestamp,
			EventID:    event.EventID,
			Type:       event.Type,
			Kind:       "event",
			Actor:      event.Actor,
			TicketID:   event.TicketID,
			Summary:    summarizeTimelineEvent(event),
			Provenance: timelineProvenance(event.OriginWorkspaceID, workspaceID),
		})
	}
	extra, err := s.timelineCollaborationEntries(ctx, detail, filterID)
	if err != nil {
		return TimelineView{}, err
	}
	entries = append(entries, extra...)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Timestamp.Equal(entries[j].Timestamp) {
			if entries[i].EventID == entries[j].EventID {
				if entries[i].Kind == entries[j].Kind {
					return entries[i].Summary < entries[j].Summary
				}
				return entries[i].Kind < entries[j].Kind
			}
			return entries[i].EventID < entries[j].EventID
		}
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})
	return TimelineView{
		TicketID:           ticketID,
		CollaboratorFilter: filterID,
		GeneratedAt:        s.now(),
		Entries:            entries,
		ChangeReady:        detail.Ticket.ChangeReadyState,
		OpenGateIDs:        append([]string{}, detail.Ticket.OpenGateIDs...),
	}, nil
}

func (s *QueryService) dashboardCollaboratorWorkload(ctx context.Context, filterID string, approvals []ApprovalItemView, inbox []InboxItemView, mentions []contracts.Mention) ([]CollaboratorWorkloadView, error) {
	collaborators, err := s.Collaborators.ListCollaborators(ctx)
	if err != nil {
		return nil, err
	}
	counts := map[string]*CollaboratorWorkloadView{}
	seed := func(id string) *CollaboratorWorkloadView {
		if count, ok := counts[id]; ok {
			return count
		}
		count := &CollaboratorWorkloadView{CollaboratorID: id}
		counts[id] = count
		return count
	}
	for _, collaborator := range collaborators {
		if filterID != "" && collaborator.CollaboratorID != filterID {
			continue
		}
		if collaborator.Status == contracts.CollaboratorStatusRemoved {
			continue
		}
		seed(collaborator.CollaboratorID)
	}
	for _, item := range approvals {
		for _, collaboratorID := range item.CollaboratorIDs {
			seed(collaboratorID).Approvals++
		}
	}
	for _, item := range inbox {
		for _, collaboratorID := range item.CollaboratorIDs {
			counter := seed(collaboratorID)
			counter.InboxItems++
			if item.Kind == "handoff" {
				counter.Handoffs++
			}
		}
	}
	for _, mention := range mentions {
		seed(mention.CollaboratorID).Mentions++
	}
	items := make([]CollaboratorWorkloadView, 0, len(counts))
	for _, item := range counts {
		if filterID == "" && item.Approvals == 0 && item.InboxItems == 0 && item.Mentions == 0 && item.Handoffs == 0 {
			continue
		}
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool {
		left := items[i].Approvals + items[i].InboxItems + items[i].Mentions + items[i].Handoffs
		right := items[j].Approvals + items[j].InboxItems + items[j].Mentions + items[j].Handoffs
		if left == right {
			return items[i].CollaboratorID < items[j].CollaboratorID
		}
		return left > right
	})
	return items, nil
}

func mentionQueueEntries(mentions []contracts.Mention) []MentionQueueEntry {
	items := make([]MentionQueueEntry, 0, len(mentions))
	for _, mention := range mentions {
		items = append(items, MentionQueueEntry{
			MentionUID:      mention.MentionUID,
			CollaboratorID:  mention.CollaboratorID,
			TicketID:        mention.TicketID,
			SourceKind:      mention.SourceKind,
			SourceID:        mention.SourceID,
			Summary:         fmt.Sprintf("@%s mentioned in %s %s", mention.CollaboratorID, mention.SourceKind, mention.SourceID),
			OriginWorkspace: mention.OriginWorkspaceID,
			GeneratedAt:     mention.CreatedAt,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].GeneratedAt.Equal(items[j].GeneratedAt) {
			return items[i].MentionUID < items[j].MentionUID
		}
		return items[i].GeneratedAt.After(items[j].GeneratedAt)
	})
	return items
}

func (s *QueryService) dashboardConflictQueue(ctx context.Context, conflicts []contracts.ConflictRecord) ([]ConflictQueueEntry, error) {
	lookup, err := s.ticketIDLookup(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]ConflictQueueEntry, 0)
	for _, conflict := range conflicts {
		if conflict.Status != contracts.ConflictStatusOpen {
			continue
		}
		items = append(items, ConflictQueueEntry{
			ConflictID:   conflict.ConflictID,
			EntityKind:   conflict.EntityKind,
			EntityUID:    conflict.EntityUID,
			ConflictType: conflict.ConflictType,
			Status:       conflict.Status,
			TicketID:     lookup[conflict.EntityUID],
			OpenedByJob:  conflict.OpenedByJob,
			GeneratedAt:  conflict.OpenedAt,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].GeneratedAt.Equal(items[j].GeneratedAt) {
			return items[i].ConflictID < items[j].ConflictID
		}
		return items[i].GeneratedAt.After(items[j].GeneratedAt)
	})
	return items, nil
}

func (s *QueryService) dashboardRemoteHealth(ctx context.Context, remotes []contracts.SyncRemote, jobs []contracts.SyncJob) ([]RemoteHealthView, []string, error) {
	failedJobs := make([]string, 0)
	items := make([]RemoteHealthView, 0, len(remotes))
	now := s.now()
	for _, remote := range remotes {
		publications, err := cachedRemotePublications(s.Root, remote.RemoteID)
		if err != nil {
			return nil, nil, err
		}
		view := RemoteHealthView{RemoteID: remote.RemoteID, Enabled: remote.Enabled, DefaultAction: string(remote.DefaultAction), PublicationCount: len(publications), LastSuccessAt: remote.LastSuccessAt}
		for _, publication := range publications {
			when := publication.FetchedAt
			if when.IsZero() {
				when = publication.CreatedAt
			}
			if when.After(view.LatestPublicationAt) {
				view.LatestPublicationAt = when
			}
		}
		for _, job := range jobs {
			if job.RemoteID != remote.RemoteID {
				continue
			}
			if job.State == contracts.SyncJobStateFailed {
				view.FailedJobs++
				failedJobs = append(failedJobs, job.JobID)
			}
		}
		switch {
		case !remote.Enabled:
			view.State = "disabled"
		case view.FailedJobs > 0:
			view.State = "failed"
		case view.LastSuccessAt.IsZero() && view.PublicationCount == 0:
			view.State = "idle"
		case remote.Enabled && freshnessTimestamp(view).Before(now.Add(-24*time.Hour)):
			view.State = "stale"
		default:
			view.State = "healthy"
		}
		items = append(items, view)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].RemoteID < items[j].RemoteID })
	return items, uniqueStrings(failedJobs), nil
}

func freshnessTimestamp(view RemoteHealthView) time.Time {
	when := view.LastSuccessAt
	if view.LatestPublicationAt.After(when) {
		return view.LatestPublicationAt
	}
	return when
}

func (s *QueryService) dashboardProviderWarnings(ctx context.Context) ([]string, error) {
	projects, err := s.Projects.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	warnings := make([]string, 0)
	for _, project := range projects {
		codeowners, err := s.CodeownersPreview(ctx, project.Key)
		if err != nil {
			return nil, err
		}
		for _, warning := range codeowners.Warnings {
			warnings = append(warnings, project.Key+":"+warning)
		}
		rules, err := s.ProviderRulesPreview(ctx, project.Key)
		if err != nil {
			return nil, err
		}
		for _, warning := range rules.Warnings {
			warnings = append(warnings, project.Key+":"+warning)
		}
	}
	return uniqueStrings(warnings), nil
}

func (s *QueryService) timelineCollaborationEntries(ctx context.Context, detail TicketDetailView, collaboratorID string) ([]TimelineEntry, error) {
	filterID := strings.TrimSpace(collaboratorID)
	entries := make([]TimelineEntry, 0)

	approvals, err := s.Approvals(ctx, filterID)
	if err != nil {
		return nil, err
	}
	for _, approval := range approvals {
		if approval.Gate.TicketID != detail.Ticket.ID {
			continue
		}
		entries = append(entries, TimelineEntry{
			Timestamp:       approval.Gate.CreatedAt,
			Type:            contracts.EventType("approval.route"),
			Kind:            "approval",
			TicketID:        detail.Ticket.ID,
			CollaboratorIDs: append([]string{}, approval.CollaboratorIDs...),
			Summary:         approval.Summary,
			Provenance:      "local",
		})
	}
	inbox, err := s.Inbox(ctx, filterID)
	if err != nil {
		return nil, err
	}
	for _, item := range inbox {
		if item.TicketID != detail.Ticket.ID || item.Kind != "handoff" {
			continue
		}
		entries = append(entries, TimelineEntry{
			Timestamp:       item.GeneratedAt,
			Type:            contracts.EventType("handoff.route"),
			Kind:            "handoff",
			TicketID:        detail.Ticket.ID,
			CollaboratorIDs: append([]string{}, item.CollaboratorIDs...),
			Summary:         item.Summary,
			Provenance:      item.Provenance,
		})
	}
	entityUIDs, err := s.ticketEntityUIDs(ctx, detail.Ticket.ID)
	if err != nil {
		return nil, err
	}
	conflicts, err := s.Conflicts.ListConflicts(ctx)
	if err != nil {
		return nil, err
	}
	for _, conflict := range conflicts {
		if _, ok := entityUIDs[conflict.EntityUID]; !ok {
			continue
		}
		entries = append(entries, TimelineEntry{
			Timestamp:  conflict.OpenedAt,
			Type:       contracts.EventConflictOpened,
			Kind:       "conflict",
			TicketID:   detail.Ticket.ID,
			Summary:    fmt.Sprintf("conflict opened: %s %s", conflict.EntityKind, conflict.ConflictType),
			Provenance: "sync",
		})
		if !conflict.ResolvedAt.IsZero() {
			entries = append(entries, TimelineEntry{
				Timestamp:  conflict.ResolvedAt,
				Type:       contracts.EventConflictResolved,
				Kind:       "conflict",
				Actor:      conflict.ResolvedBy,
				TicketID:   detail.Ticket.ID,
				Summary:    fmt.Sprintf("conflict resolved: %s", conflict.Resolution),
				Provenance: "sync",
			})
		}
	}
	jobs, err := s.SyncJobs.ListSyncJobs(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, job := range jobs {
		if !syncJobTouchesTicket(job, detail.Ticket) {
			continue
		}
		at := job.FinishedAt
		if at.IsZero() {
			at = job.StartedAt
		}
		eventType := contracts.EventSyncCompleted
		if job.State == contracts.SyncJobStateFailed {
			eventType = contracts.EventSyncFailed
		}
		summary := fmt.Sprintf("sync %s %s", job.Mode, job.State)
		if strings.TrimSpace(job.RemoteID) != "" {
			summary += " via " + job.RemoteID
		}
		entries = append(entries, TimelineEntry{
			Timestamp:  at,
			Type:       eventType,
			Kind:       "sync_job",
			TicketID:   detail.Ticket.ID,
			Summary:    summary,
			Provenance: "sync",
		})
	}
	return entries, nil
}

func (s *QueryService) ticketEntityUIDs(ctx context.Context, ticketID string) (map[string]struct{}, error) {
	ticket, err := s.Tickets.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	uids := map[string]struct{}{ticket.TicketUID: {}}
	runs, err := s.Runs.ListRuns(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		uids[run.RunUID] = struct{}{}
		evidence, err := s.Evidence.ListEvidence(ctx, run.RunID)
		if err != nil {
			return nil, err
		}
		for _, item := range evidence {
			uids[item.EvidenceUID] = struct{}{}
		}
	}
	gates, err := s.Gates.ListGates(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	for _, gate := range gates {
		uids[gate.GateUID] = struct{}{}
	}
	handoffs, err := s.Handoffs.ListHandoffs(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	for _, handoff := range handoffs {
		uids[handoff.HandoffUID] = struct{}{}
	}
	changes, err := s.Changes.ListChanges(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	for _, change := range changes {
		uids[change.ChangeUID] = struct{}{}
		checks, err := s.Checks.ListChecks(ctx, contracts.CheckScopeChange, change.ChangeID)
		if err != nil {
			return nil, err
		}
		for _, check := range checks {
			uids[check.CheckUID] = struct{}{}
		}
	}
	ticketChecks, err := s.Checks.ListChecks(ctx, contracts.CheckScopeTicket, ticketID)
	if err != nil {
		return nil, err
	}
	for _, check := range ticketChecks {
		uids[check.CheckUID] = struct{}{}
	}
	return uids, nil
}

func (s *QueryService) ticketIDLookup(ctx context.Context) (map[string]string, error) {
	lookup := map[string]string{}
	tickets, err := s.Tickets.ListTickets(ctx, contracts.TicketListOptions{IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	for _, ticket := range tickets {
		lookup[ticket.TicketUID] = ticket.ID
	}
	runs, err := s.Runs.ListRuns(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		lookup[run.RunUID] = run.TicketID
	}
	gates, err := s.Gates.ListGates(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, gate := range gates {
		lookup[gate.GateUID] = gate.TicketID
	}
	handoffs, err := s.Handoffs.ListHandoffs(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, handoff := range handoffs {
		lookup[handoff.HandoffUID] = handoff.TicketID
	}
	changes, err := s.Changes.ListChanges(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, change := range changes {
		lookup[change.ChangeUID] = change.TicketID
	}
	checks, err := s.Checks.ListChecks(ctx, "", "")
	if err != nil {
		return nil, err
	}
	for _, check := range checks {
		scopeID := strings.TrimSpace(check.ScopeID)
		switch check.Scope {
		case contracts.CheckScopeTicket:
			lookup[check.CheckUID] = scopeID
		case contracts.CheckScopeRun:
			for _, run := range runs {
				if run.RunID == scopeID {
					lookup[check.CheckUID] = run.TicketID
					break
				}
			}
		case contracts.CheckScopeChange:
			for _, change := range changes {
				if change.ChangeID == scopeID {
					lookup[check.CheckUID] = change.TicketID
					break
				}
			}
		}
	}
	return lookup, nil
}

func syncJobTouchesTicket(job contracts.SyncJob, ticket contracts.TicketSnapshot) bool {
	if strings.TrimSpace(job.BundleRef) == "" {
		return false
	}
	manifest, _, err := loadManifestFromArchive(job.BundleRef)
	if err != nil {
		return false
	}
	ticketPath := filepath.ToSlash(filepath.Join(storage.ProjectsDirName, ticket.Project, "tickets", ticket.ID+".md"))
	eventPath := filepath.ToSlash(filepath.Join(storage.TrackerDirName, "events", ticket.Project+".jsonl"))
	for _, item := range manifest.Files {
		path := filepath.ToSlash(strings.TrimSpace(item.Path))
		if path == ticketPath || path == eventPath {
			return true
		}
	}
	return false
}

func timelineProvenance(originWorkspaceID string, workspaceID string) string {
	origin := strings.TrimSpace(originWorkspaceID)
	if origin == "" || origin == strings.TrimSpace(workspaceID) {
		return "local"
	}
	return "synced"
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
	switch event.Type {
	case contracts.EventMentionRecorded:
		var mention contracts.Mention
		if decodeEventPayload(event.Payload, &mention) == nil && mention.CollaboratorID != "" {
			return fmt.Sprintf("@%s mentioned in %s %s", mention.CollaboratorID, mention.SourceKind, mention.SourceID)
		}
	}
	return fmt.Sprintf("%s by %s", event.Type, event.Actor)
}

func decodeEventPayload(raw any, target any) error {
	payload, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, target)
}
