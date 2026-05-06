package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/domain"
)

type ActionService struct {
	Root               string
	Projects           contracts.ProjectStore
	Tickets            contracts.TicketStore
	Collaborators      contracts.CollaboratorStore
	Memberships        contracts.MembershipStore
	Mentions           contracts.MentionStore
	SyncRemotes        contracts.SyncRemoteStore
	SyncJobs           contracts.SyncJobStore
	Conflicts          contracts.ConflictStore
	Agents             contracts.AgentStore
	PermissionProfiles contracts.PermissionProfileStore
	Runs               contracts.RunStore
	Runbooks           contracts.RunbookStore
	Gates              contracts.GateStore
	Evidence           contracts.EvidenceStore
	Handoffs           contracts.HandoffStore
	Changes            contracts.ChangeStore
	Checks             contracts.CheckStore
	ImportJobs         contracts.ImportJobStore
	ExportBundles      contracts.ExportBundleStore
	RetentionPolicies  contracts.RetentionPolicyStore
	Archives           contracts.ArchiveRecordStore
	Events             contracts.EventLog
	Projection         contracts.ProjectionStore
	Clock              func() time.Time
	LockManager        WriteLockManager
	Notifier           Notifier
	Automation         *AutomationEngine
}

func NewActionService(root string, projects contracts.ProjectStore, tickets contracts.TicketStore, events contracts.EventLog, projection contracts.ProjectionStore, clock func() time.Time, locks WriteLockManager, notifier Notifier, automation *AutomationEngine) *ActionService {
	canonicalRoot, err := CanonicalWorkspaceRoot(root)
	if err != nil {
		canonicalRoot = root
	}
	if fm, ok := locks.(FileLockManager); ok {
		fm.Root = canonicalRoot
		locks = fm
	}
	return &ActionService{Root: canonicalRoot, Projects: projects, Tickets: tickets, Collaborators: CollaboratorStore{Root: canonicalRoot}, Memberships: MembershipStore{Root: canonicalRoot}, Mentions: MentionStore{Root: canonicalRoot}, SyncRemotes: SyncRemoteStore{Root: canonicalRoot}, SyncJobs: SyncJobStore{Root: canonicalRoot}, Conflicts: ConflictStore{Root: canonicalRoot}, Agents: AgentStore{Root: canonicalRoot}, PermissionProfiles: PermissionProfileStore{Root: canonicalRoot}, Runs: RunStore{Root: canonicalRoot}, Runbooks: RunbookStore{Root: canonicalRoot}, Gates: GateStore{Root: canonicalRoot}, Evidence: EvidenceStore{Root: canonicalRoot}, Handoffs: HandoffStore{Root: canonicalRoot}, Changes: ChangeStore{Root: canonicalRoot}, Checks: CheckStore{Root: canonicalRoot}, ImportJobs: ImportJobStore{Root: canonicalRoot}, ExportBundles: ExportBundleStore{Root: canonicalRoot}, RetentionPolicies: RetentionPolicyStore{Root: canonicalRoot}, Archives: ArchiveRecordStore{Root: canonicalRoot}, Events: events, Projection: projection, Clock: clock, LockManager: locks, Notifier: notifier, Automation: automation}
}

func (s *ActionService) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

func (s *ActionService) journal() MutationJournal {
	return MutationJournal{Root: s.Root, Clock: s.Clock}
}

func (s *ActionService) newEvent(ctx context.Context, project string, at time.Time, actor contracts.Actor, reason string, eventType contracts.EventType, ticketID string, payload any) (contracts.Event, error) {
	if !lockHeld(ctx) {
		return contracts.Event{}, apperr.New(apperr.CodeInternal, "event allocation requires workspace write lock")
	}
	workspaceID, err := ensureWorkspaceIdentity(s.Root)
	if err != nil {
		return contracts.Event{}, err
	}
	eventID, err := s.NextEventID(ctx, project)
	if err != nil {
		return contracts.Event{}, err
	}
	logicalClock, err := s.NextLogicalClock(ctx)
	if err != nil {
		return contracts.Event{}, err
	}
	return contracts.NormalizeEvent(contracts.Event{
		EventID:           eventID,
		EventUID:          contracts.CanonicalEventUID(contracts.Event{EventID: eventID, Timestamp: at.UTC(), OriginWorkspaceID: workspaceID, LogicalClock: logicalClock, Type: eventType, Project: project, TicketID: ticketID, SchemaVersion: contracts.CurrentSchemaVersion}),
		Timestamp:         at.UTC(),
		OriginWorkspaceID: workspaceID,
		LogicalClock:      logicalClock,
		Actor:             actor,
		Reason:            reason,
		Type:              eventType,
		Project:           project,
		TicketID:          ticketID,
		Payload:           payload,
		Metadata:          eventMetadataFromContext(ctx, actor),
		SchemaVersion:     contracts.CurrentSchemaVersion,
	}), nil
}

func (s *ActionService) commitMutation(ctx context.Context, purpose string, canonicalKind string, event contracts.Event, writeCanonical func(context.Context) error) error {
	ctx = contextWithDefaultReplayMode(ctx)
	if lockHeld(ctx) {
		normalized, err := s.normalizeAppendOnlyEvent(ctx, event)
		if err != nil {
			return err
		}
		event = normalized
	} else {
		event = contracts.NormalizeEvent(event)
	}
	journal, err := s.journal().Begin(purpose, canonicalKind, event)
	if err != nil {
		return err
	}
	if writeCanonical != nil {
		if err := writeCanonical(ctx); err != nil {
			_ = s.journal().Complete(journal.ID)
			return err
		}
		updated, markErr := s.journal().Mark(journal, MutationStageCanonicalWritten, "")
		if markErr != nil {
			return apperr.Wrap(apperr.CodeRepairNeeded, markErr, "record canonical mutation stage")
		}
		journal = updated
	}
	if err := s.Events.AppendEvent(ctx, event); err != nil {
		_, _ = s.journal().Mark(journal, journal.Stage, err.Error())
		if writeCanonical != nil {
			return apperr.Wrap(apperr.CodeRepairNeeded, err, "event append failed after canonical write")
		}
		return err
	}
	updated, markErr := s.journal().Mark(journal, MutationStageEventAppended, "")
	if markErr != nil {
		return apperr.Wrap(apperr.CodeRepairNeeded, markErr, "record event append stage")
	}
	journal = updated
	if s.Projection != nil {
		if err := s.Projection.ApplyEvent(ctx, event); err != nil {
			_, _ = s.journal().Mark(journal, journal.Stage, err.Error())
			return apperr.Wrap(apperr.CodeRepairNeeded, err, "projection apply failed after event commit")
		}
		updated, markErr = s.journal().Mark(journal, MutationStageProjectionApplied, "")
		if markErr != nil {
			return apperr.Wrap(apperr.CodeRepairNeeded, markErr, "record projection stage")
		}
		journal = updated
	}
	if !historicalReplay(ctx) && s.Notifier != nil {
		_ = s.Notifier.Notify(ctx, event)
	}
	if err := s.journal().Complete(journal.ID); err != nil {
		return apperr.Wrap(apperr.CodeRepairNeeded, err, "finalize mutation journal")
	}
	if !historicalReplay(ctx) && s.Automation != nil {
		_, _ = s.Automation.Run(ctx, s, NewQueryService(s.Root, s.Projects, s.Tickets, s.Events, s.Projection, s.Clock), event)
	}
	return nil
}

func (s *ActionService) commitTicketSnapshotEvent(ctx context.Context, purpose string, ticket contracts.TicketSnapshot, actor contracts.Actor, reason string, eventType contracts.EventType, payload any) error {
	if payload == nil {
		payload = ticket
	}
	event, err := s.newEvent(ctx, ticket.Project, ticket.UpdatedAt, actor, reason, eventType, ticket.ID, payload)
	if err != nil {
		return err
	}
	return s.commitMutation(ctx, purpose, "ticket_snapshot", event, func(ctx context.Context) error {
		return s.UpdateTicket(ctx, ticket)
	})
}

func (s *ActionService) NextEventID(ctx context.Context, project string) (int64, error) {
	return withWriteLock(ctx, s.LockManager, "allocate event id", func(ctx context.Context) (int64, error) {
		events, err := s.Events.StreamEvents(ctx, project, 0)
		if err != nil {
			return 0, err
		}
		var maxID int64
		for _, event := range events {
			if event.EventID > maxID {
				maxID = event.EventID
			}
		}
		return maxID + 1, nil
	})
}

func (s *ActionService) NextLogicalClock(ctx context.Context) (int64, error) {
	return withWriteLock(ctx, s.LockManager, "allocate logical clock", func(ctx context.Context) (int64, error) {
		events, err := s.Events.StreamEvents(ctx, "", 0)
		if err != nil {
			return 0, err
		}
		var maxClock int64
		for _, event := range events {
			if event.LogicalClock > maxClock {
				maxClock = event.LogicalClock
			}
			if event.LogicalClock == 0 && event.EventID > maxClock {
				maxClock = event.EventID
			}
		}
		return maxClock + 1, nil
	})
}

func (s *ActionService) CreateProject(ctx context.Context, project contracts.Project) error {
	_, err := withWriteLock(ctx, s.LockManager, "create project", func(ctx context.Context) (struct{}, error) {
		return struct{}{}, s.Projects.CreateProject(ctx, contracts.NormalizeProject(project))
	})
	return err
}

func (s *ActionService) UpdateProject(ctx context.Context, project contracts.Project) error {
	_, err := withWriteLock(ctx, s.LockManager, "update project", func(ctx context.Context) (struct{}, error) {
		return struct{}{}, s.Projects.UpdateProject(ctx, contracts.NormalizeProject(project))
	})
	return err
}

func (s *ActionService) SaveAgentProfile(ctx context.Context, profile contracts.AgentProfile, actor contracts.Actor, reason string) (contracts.AgentProfile, error) {
	return withWriteLock(ctx, s.LockManager, "save agent profile", func(ctx context.Context) (contracts.AgentProfile, error) {
		if !actor.IsValid() {
			return contracts.AgentProfile{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		profile.AgentID = sanitizeAgentID(profile.AgentID)
		existing, err := s.Agents.LoadAgent(ctx, profile.AgentID)
		eventType := contracts.EventAgentCreated
		if err == nil {
			eventType = contracts.EventAgentUpdated
			if strings.TrimSpace(profile.DisplayName) == "" {
				profile.DisplayName = existing.DisplayName
			}
		}
		if !profile.Enabled && eventType == contracts.EventAgentCreated {
			profile.Enabled = true
		}
		event, err := s.newEvent(ctx, workspaceProjectKey, s.now(), actor, reason, eventType, "", profile)
		if err != nil {
			return contracts.AgentProfile{}, err
		}
		if err := s.commitMutation(ctx, "save agent profile", "agent_profile", event, func(ctx context.Context) error {
			return s.Agents.SaveAgent(ctx, profile)
		}); err != nil {
			return contracts.AgentProfile{}, err
		}
		return profile, nil
	})
}

func (s *ActionService) SetAgentEnabled(ctx context.Context, agentID string, enabled bool, actor contracts.Actor, reason string) (contracts.AgentProfile, error) {
	return withWriteLock(ctx, s.LockManager, "set agent enabled", func(ctx context.Context) (contracts.AgentProfile, error) {
		if !actor.IsValid() {
			return contracts.AgentProfile{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		profile, err := s.Agents.LoadAgent(ctx, agentID)
		if err != nil {
			return contracts.AgentProfile{}, err
		}
		profile.Enabled = enabled
		eventType := contracts.EventAgentDisabled
		if enabled {
			eventType = contracts.EventAgentEnabled
		}
		event, err := s.newEvent(ctx, workspaceProjectKey, s.now(), actor, reason, eventType, "", profile)
		if err != nil {
			return contracts.AgentProfile{}, err
		}
		if err := s.commitMutation(ctx, "set agent enabled", "agent_profile", event, func(ctx context.Context) error {
			return s.Agents.SaveAgent(ctx, profile)
		}); err != nil {
			return contracts.AgentProfile{}, err
		}
		return profile, nil
	})
}

func (s *ActionService) CreateTicket(ctx context.Context, ticket contracts.TicketSnapshot) error {
	_, err := withWriteLock(ctx, s.LockManager, "create ticket snapshot", func(ctx context.Context) (struct{}, error) {
		return struct{}{}, s.Tickets.CreateTicket(ctx, contracts.NormalizeTicketSnapshot(ticket))
	})
	return err
}

func (s *ActionService) UpdateTicket(ctx context.Context, ticket contracts.TicketSnapshot) error {
	_, err := withWriteLock(ctx, s.LockManager, "update ticket snapshot", func(ctx context.Context) (struct{}, error) {
		return struct{}{}, s.Tickets.UpdateTicket(ctx, contracts.NormalizeTicketSnapshot(ticket))
	})
	return err
}

func (s *ActionService) SoftDeleteTicket(ctx context.Context, id string, actor contracts.Actor, reason string) error {
	_, err := withWriteLock(ctx, s.LockManager, "soft delete ticket", func(ctx context.Context) (struct{}, error) {
		return struct{}{}, s.Tickets.SoftDeleteTicket(ctx, id, actor, reason)
	})
	return err
}

func (s *ActionService) AppendAndProject(ctx context.Context, event contracts.Event) error {
	_, err := withWriteLock(ctx, s.LockManager, "append and project event", func(ctx context.Context) (struct{}, error) {
		normalized, err := s.normalizeAppendOnlyEvent(ctx, event)
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, s.commitMutation(ctx, "append and project event", "event_only", normalized, nil)
	})
	return err
}

func (s *ActionService) normalizeAppendOnlyEvent(ctx context.Context, event contracts.Event) (contracts.Event, error) {
	event = contracts.NormalizeEvent(event)
	if event.OriginWorkspaceID == "" {
		workspaceID, err := ensureWorkspaceIdentity(s.Root)
		if err != nil {
			return contracts.Event{}, err
		}
		event.OriginWorkspaceID = workspaceID
	}
	if event.LogicalClock == 0 {
		next, err := s.NextLogicalClock(ctx)
		if err != nil {
			return contracts.Event{}, err
		}
		event.LogicalClock = next
	}
	if event.EventUID == "" {
		event.EventUID = contracts.CanonicalEventUID(event)
	}
	return contracts.NormalizeEvent(event), nil
}

func (s *ActionService) AllocateTicketID(ctx context.Context, project string) (string, error) {
	return withWriteLock(ctx, s.LockManager, "allocate ticket id", func(ctx context.Context) (string, error) {
		tickets, err := s.Tickets.ListTickets(ctx, contracts.TicketListOptions{Project: strings.TrimSpace(project), IncludeArchived: true})
		if err != nil {
			return "", err
		}
		max := 0
		prefix := strings.TrimSpace(project) + "-"
		for _, ticket := range tickets {
			if !strings.HasPrefix(ticket.ID, prefix) {
				continue
			}
			raw := strings.TrimPrefix(ticket.ID, prefix)
			n, err := strconv.Atoi(raw)
			if err == nil && n > max {
				max = n
			}
		}
		return fmt.Sprintf("%s-%d", strings.TrimSpace(project), max+1), nil
	})
}

func (s *ActionService) CreateTrackedTicket(ctx context.Context, ticket contracts.TicketSnapshot, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	return withWriteLock(ctx, s.LockManager, "create tracked ticket", func(ctx context.Context) (contracts.TicketSnapshot, error) {
		if !actor.IsValid() {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if _, err := s.Projects.GetProject(ctx, ticket.Project); err != nil {
			return contracts.TicketSnapshot{}, err
		}
		normalized := contracts.NormalizeTicketSnapshot(ticket)
		if strings.TrimSpace(normalized.ID) == "" {
			id, err := s.AllocateTicketID(ctx, normalized.Project)
			if err != nil {
				return contracts.TicketSnapshot{}, err
			}
			normalized.ID = id
		}
		if normalized.SchemaVersion == 0 {
			normalized.SchemaVersion = contracts.CurrentSchemaVersion
		}
		event, err := s.newEvent(ctx, normalized.Project, normalized.UpdatedAt, actor, reason, contracts.EventTicketCreated, normalized.ID, normalized)
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
		if err := s.commitMutation(ctx, "create tracked ticket", "ticket_snapshot", event, func(ctx context.Context) error {
			return s.CreateTicket(ctx, normalized)
		}); err != nil {
			return contracts.TicketSnapshot{}, err
		}
		return normalized, nil
	})
}

func (s *ActionService) SaveTrackedTicket(ctx context.Context, ticket contracts.TicketSnapshot, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	return withWriteLock(ctx, s.LockManager, "save tracked ticket", func(ctx context.Context) (contracts.TicketSnapshot, error) {
		if !actor.IsValid() {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		normalized := contracts.NormalizeTicketSnapshot(ticket)
		event, err := s.newEvent(ctx, normalized.Project, normalized.UpdatedAt, actor, reason, contracts.EventTicketUpdated, normalized.ID, normalized)
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
		if err := s.commitMutation(ctx, "save tracked ticket", "ticket_snapshot", event, func(ctx context.Context) error {
			return s.UpdateTicket(ctx, normalized)
		}); err != nil {
			return contracts.TicketSnapshot{}, err
		}
		return normalized, nil
	})
}

func (s *ActionService) MutateTrackedTicket(ctx context.Context, ticketID string, actor contracts.Actor, reason string, purpose string, mutate func(*contracts.TicketSnapshot) error) (contracts.TicketSnapshot, error) {
	if strings.TrimSpace(purpose) == "" {
		purpose = "mutate tracked ticket"
	}
	return withWriteLock(ctx, s.LockManager, purpose, func(ctx context.Context) (contracts.TicketSnapshot, error) {
		if !actor.IsValid() {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		ticket, err := s.Tickets.GetTicket(ctx, ticketID)
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
		if err := mutate(&ticket); err != nil {
			return contracts.TicketSnapshot{}, err
		}
		ticket.UpdatedAt = s.now()
		return s.SaveTrackedTicket(ctx, ticket, actor, reason)
	})
}

func (s *ActionService) DeleteTrackedTicket(ctx context.Context, ticketID string, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	return withWriteLock(ctx, s.LockManager, "delete tracked ticket", func(ctx context.Context) (contracts.TicketSnapshot, error) {
		if !actor.IsValid() {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		ticket, err := s.Tickets.GetTicket(ctx, ticketID)
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
		now := s.now()
		ticket.Status = contracts.StatusCanceled
		ticket.Archived = true
		ticket.UpdatedAt = now
		auditLine := fmt.Sprintf("Archived by %s at %s", actor, now.Format(time.RFC3339))
		if strings.TrimSpace(reason) != "" {
			auditLine += " — " + strings.TrimSpace(reason)
		}
		if strings.TrimSpace(ticket.Notes) == "" {
			ticket.Notes = auditLine
		} else {
			ticket.Notes = strings.TrimSpace(ticket.Notes) + "\n\n" + auditLine
		}
		event, err := s.newEvent(ctx, ticket.Project, now, actor, reason, contracts.EventTicketClosed, ticket.ID, ticket)
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
		if err := s.commitMutation(ctx, "delete tracked ticket", "ticket_snapshot", event, func(ctx context.Context) error {
			return s.UpdateTicket(ctx, ticket)
		}); err != nil {
			return contracts.TicketSnapshot{}, err
		}
		return ticket, nil
	})
}

func (s *ActionService) AssignTicket(ctx context.Context, ticketID string, assignee contracts.Actor, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	return withWriteLock(ctx, s.LockManager, "assign ticket", func(ctx context.Context) (contracts.TicketSnapshot, error) {
		if !actor.IsValid() {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if assignee != "" && !assignee.IsValid() {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid assignee actor: %s", assignee))
		}
		ticket, err := s.Tickets.GetTicket(ctx, ticketID)
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
		ticket.Assignee = assignee
		ticket.UpdatedAt = s.now()
		return s.SaveTrackedTicket(ctx, ticket, actor, reason)
	})
}

func (s *ActionService) CommentTicket(ctx context.Context, ticketID string, body string, actor contracts.Actor, reason string) error {
	_, err := withWriteLock(ctx, s.LockManager, "comment ticket", func(ctx context.Context) (struct{}, error) {
		if !actor.IsValid() {
			return struct{}{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		body = strings.TrimSpace(body)
		if body == "" {
			return struct{}{}, apperr.New(apperr.CodeInvalidInput, "comment body is required in v1 non-interactive mode")
		}
		ticket, err := s.Tickets.GetTicket(ctx, ticketID)
		if err != nil {
			return struct{}{}, err
		}
		now := s.now()
		event, err := s.newEvent(ctx, ticket.Project, now, actor, reason, contracts.EventTicketCommented, ticket.ID, map[string]any{"body": body})
		if err != nil {
			return struct{}{}, err
		}
		mentions, err := s.extractMentions(ctx, event, "ticket_comment", event.EventUID, ticket.ID, body)
		if err != nil {
			return struct{}{}, err
		}
		if err := s.commitMutation(ctx, "comment ticket", "comment", event, func(ctx context.Context) error {
			for _, mention := range mentions.Mentions {
				if err := s.Mentions.SaveMention(ctx, mention); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return struct{}{}, err
		}
		if err := s.recordMentionEvents(ctx, ticket.Project, actor, reason, mentions.Mentions); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, nil
	})
	return err
}

func (s *ActionService) LinkTickets(ctx context.Context, id string, otherID string, kind domain.LinkKind, actor contracts.Actor, reason string) (contracts.Event, error) {
	return withWriteLock(ctx, s.LockManager, "link tickets", func(ctx context.Context) (contracts.Event, error) {
		if !actor.IsValid() {
			return contracts.Event{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		mapped, err := s.loadTicketsMap(ctx)
		if err != nil {
			return contracts.Event{}, err
		}
		if err := domain.ApplyLink(mapped, id, otherID, kind); err != nil {
			return contracts.Event{}, err
		}
		now := s.now()
		trimmedOther := strings.TrimSpace(otherID)
		for _, ticketID := range []string{strings.TrimSpace(id), trimmedOther} {
			ticket := mapped[ticketID]
			ticket.UpdatedAt = now
		}
		event, err := s.newEvent(ctx, mapped[strings.TrimSpace(id)].Project, now, actor, reason, contracts.EventTicketLinked, strings.TrimSpace(id), map[string]any{
			"id":           strings.TrimSpace(id),
			"other_id":     trimmedOther,
			"kind":         kind,
			"ticket":       mapped[strings.TrimSpace(id)],
			"other_ticket": mapped[trimmedOther],
		})
		if err != nil {
			return contracts.Event{}, err
		}
		if err := s.commitMutation(ctx, "link tickets", "multi_ticket_snapshot", event, func(ctx context.Context) error {
			for _, ticketID := range []string{strings.TrimSpace(id), trimmedOther} {
				if err := s.UpdateTicket(ctx, mapped[ticketID]); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return contracts.Event{}, err
		}
		return event, nil
	})
}

func (s *ActionService) UnlinkTickets(ctx context.Context, id string, otherID string, actor contracts.Actor, reason string) (contracts.Event, error) {
	return withWriteLock(ctx, s.LockManager, "unlink tickets", func(ctx context.Context) (contracts.Event, error) {
		if !actor.IsValid() {
			return contracts.Event{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		mapped, err := s.loadTicketsMap(ctx)
		if err != nil {
			return contracts.Event{}, err
		}
		if err := domain.RemoveLink(mapped, id, otherID); err != nil {
			return contracts.Event{}, err
		}
		now := s.now()
		trimmedID := strings.TrimSpace(id)
		trimmedOther := strings.TrimSpace(otherID)
		for _, ticketID := range []string{trimmedID, trimmedOther} {
			ticket := mapped[ticketID]
			ticket.UpdatedAt = now
		}
		event, err := s.newEvent(ctx, mapped[trimmedID].Project, now, actor, reason, contracts.EventTicketUnlinked, trimmedID, map[string]any{
			"id":           trimmedID,
			"other_id":     trimmedOther,
			"ticket":       mapped[trimmedID],
			"other_ticket": mapped[trimmedOther],
		})
		if err != nil {
			return contracts.Event{}, err
		}
		if err := s.commitMutation(ctx, "unlink tickets", "multi_ticket_snapshot", event, func(ctx context.Context) error {
			for _, ticketID := range []string{trimmedID, trimmedOther} {
				if err := s.UpdateTicket(ctx, mapped[ticketID]); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return contracts.Event{}, err
		}
		return event, nil
	})
}

func (s *ActionService) ClaimTicket(ctx context.Context, ticketID string, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	return withWriteLock(ctx, s.LockManager, "claim ticket", func(ctx context.Context) (contracts.TicketSnapshot, error) {
		if !actor.IsValid() {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		ticket, err := s.Tickets.GetTicket(ctx, ticketID)
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
		if ticket.Lease.Actor != "" && !ticket.Lease.Active(s.now()) {
			if _, err := s.expireLease(ctx, &ticket, "lease expired before claim"); err != nil {
				return contracts.TicketSnapshot{}, err
			}
		}
		if ticket.Lease.Active(s.now()) && ticket.Lease.Actor == actor {
			return ticket, nil
		}
		if ticket.Lease.Active(s.now()) && ticket.Lease.Actor != actor {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("ticket %s is already claimed by %s", ticket.ID, ticket.Lease.Actor))
		}
		policy, err := resolveEffectivePolicy(ctx, s.Root, s.Projects, s.Tickets, ticket)
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
		if len(policy.AllowedWorkers) > 0 && !actorInList(actor, policy.AllowedWorkers) && actor != contracts.Actor("human:owner") {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodePermissionDenied, fmt.Sprintf("actor %s is not allowed by effective policy", actor))
		}

		kind := contracts.LeaseKindWork
		if ticket.Status == contracts.StatusInReview {
			if actor != ticket.Reviewer && actor != contracts.Actor("human:owner") {
				return contracts.TicketSnapshot{}, apperr.New(apperr.CodePermissionDenied, "review claims must belong to the reviewer or owner")
			}
			kind = contracts.LeaseKindReview
		}
		now := s.now()
		ticket.Lease = contracts.LeaseState{
			Actor:           actor,
			Kind:            kind,
			AcquiredAt:      now,
			ExpiresAt:       now.Add(policy.LeaseTTL),
			LastHeartbeatAt: now,
		}
		ticket.UpdatedAt = now
		if err := s.commitTicketSnapshotEvent(ctx, "claim ticket", ticket, actor, reason, contracts.EventTicketClaimed, ticket); err != nil {
			return contracts.TicketSnapshot{}, err
		}
		return ticket, nil
	})
}

func (s *ActionService) ReleaseTicket(ctx context.Context, ticketID string, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	return withWriteLock(ctx, s.LockManager, "release ticket", func(ctx context.Context) (contracts.TicketSnapshot, error) {
		if !actor.IsValid() {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		ticket, err := s.Tickets.GetTicket(ctx, ticketID)
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
		if ticket.Lease.Actor == "" {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("ticket %s is not claimed", ticket.ID))
		}
		if ticket.Lease.Actor != actor && actor != contracts.Actor("human:owner") {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodePermissionDenied, fmt.Sprintf("ticket %s is claimed by %s", ticket.ID, ticket.Lease.Actor))
		}
		now := s.now()
		ticket.Lease = contracts.LeaseState{}
		ticket.UpdatedAt = now
		if err := s.commitTicketSnapshotEvent(ctx, "release ticket", ticket, actor, reason, contracts.EventTicketReleased, ticket); err != nil {
			return contracts.TicketSnapshot{}, err
		}
		return ticket, nil
	})
}

func (s *ActionService) HeartbeatTicket(ctx context.Context, ticketID string, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	return withWriteLock(ctx, s.LockManager, "heartbeat ticket", func(ctx context.Context) (contracts.TicketSnapshot, error) {
		if !actor.IsValid() {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		ticket, err := s.Tickets.GetTicket(ctx, ticketID)
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
		if ticket.Lease.Actor == "" || ticket.Lease.Actor != actor || !ticket.Lease.Active(s.now()) {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodePermissionDenied, fmt.Sprintf("actor %s does not hold an active lease on %s", actor, ticket.ID))
		}
		policy, err := resolveEffectivePolicy(ctx, s.Root, s.Projects, s.Tickets, ticket)
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
		now := s.now()
		ticket.Lease.LastHeartbeatAt = now
		ticket.Lease.ExpiresAt = now.Add(policy.LeaseTTL)
		ticket.UpdatedAt = now
		if err := s.commitTicketSnapshotEvent(ctx, "heartbeat ticket", ticket, actor, reason, contracts.EventTicketHeartbeat, ticket); err != nil {
			return contracts.TicketSnapshot{}, err
		}
		return ticket, nil
	})
}

func (s *ActionService) SweepExpiredClaims(ctx context.Context, actor contracts.Actor, reason string) ([]contracts.TicketSnapshot, error) {
	return withWriteLock(ctx, s.LockManager, "sweep expired claims", func(ctx context.Context) ([]contracts.TicketSnapshot, error) {
		if !actor.IsValid() {
			return nil, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		tickets, err := s.Tickets.ListTickets(ctx, contracts.TicketListOptions{IncludeArchived: false})
		if err != nil {
			return nil, err
		}
		expired := make([]contracts.TicketSnapshot, 0)
		for _, ticket := range tickets {
			if ticket.Lease.Actor == "" || ticket.Lease.Active(s.now()) || ticket.Lease.ExpiresAt.IsZero() {
				continue
			}
			updated, err := s.expireLease(ctx, &ticket, reason)
			if err != nil {
				return nil, err
			}
			expired = append(expired, updated)
		}
		return expired, nil
	})
}

func (s *ActionService) MoveTicket(ctx context.Context, ticketID string, to contracts.Status, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	return withWriteLock(ctx, s.LockManager, "move ticket", func(ctx context.Context) (contracts.TicketSnapshot, error) {
		if !actor.IsValid() {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		ticket, err := s.Tickets.GetTicket(ctx, ticketID)
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
		if to == contracts.StatusDone {
			return s.CompleteTicket(ctx, ticketID, actor, reason)
		}
		policy, err := resolveEffectivePolicy(ctx, s.Root, s.Projects, s.Tickets, ticket)
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
		if err := domain.ValidateTransition(ticket.Status, to); err != nil {
			return contracts.TicketSnapshot{}, err
		}
		now := s.now()
		from := ticket.Status
		priorReviewState := ticket.ReviewState
		ticket.Status = to
		if to == contracts.StatusInProgress && from == contracts.StatusInReview && priorReviewState == contracts.ReviewStateApproved && policy.CompletionMode != contracts.CompletionModeReviewGate {
			ticket.ReviewState = contracts.ReviewStateChangesRequested
		} else if to != contracts.StatusInReview {
			ticket.ReviewState = contracts.ReviewStateNone
		}
		if to == contracts.StatusInReview && priorReviewState == contracts.ReviewStateNone {
			ticket.ReviewState = contracts.ReviewStatePending
		}
		ticket.UpdatedAt = now
		payload := map[string]any{"from": from, "to": to, "ticket": ticket}
		if err := s.commitTicketSnapshotEvent(ctx, "move ticket", ticket, actor, reason, contracts.EventTicketMoved, payload); err != nil {
			return contracts.TicketSnapshot{}, err
		}
		return ticket, nil
	})
}

func (s *ActionService) RequestReview(ctx context.Context, ticketID string, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	return withWriteLock(ctx, s.LockManager, "request review", func(ctx context.Context) (contracts.TicketSnapshot, error) {
		if !actor.IsValid() {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		ticket, err := s.Tickets.GetTicket(ctx, ticketID)
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
		if err := domain.ValidateTransition(ticket.Status, contracts.StatusInReview); err != nil {
			return contracts.TicketSnapshot{}, err
		}
		now := s.now()
		ticket.Status = contracts.StatusInReview
		ticket.ReviewState = contracts.ReviewStatePending
		if ticket.Lease.Kind == contracts.LeaseKindWork {
			ticket.Lease = contracts.LeaseState{}
		}
		ticket.UpdatedAt = now
		if err := s.commitTicketSnapshotEvent(ctx, "request review", ticket, actor, reason, contracts.EventTicketReviewRequested, ticket); err != nil {
			return contracts.TicketSnapshot{}, err
		}
		return ticket, nil
	})
}

func (s *ActionService) ApproveTicket(ctx context.Context, ticketID string, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	return withWriteLock(ctx, s.LockManager, "approve ticket", func(ctx context.Context) (contracts.TicketSnapshot, error) {
		if !actor.IsValid() {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		ticket, err := s.Tickets.GetTicket(ctx, ticketID)
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
		if ticket.Status != contracts.StatusInReview {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("ticket %s is not in review", ticket.ID))
		}
		if actor != contracts.Actor("human:owner") && actor != ticket.Reviewer {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodePermissionDenied, "only the assigned reviewer or human:owner can approve")
		}
		policy, err := resolveEffectivePolicy(ctx, s.Root, s.Projects, s.Tickets, ticket)
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
		now := s.now()
		ticket.ReviewState = contracts.ReviewStateApproved
		ticket.UpdatedAt = now
		if policy.CompletionMode == contracts.CompletionModeReviewGate {
			ticket.Status = contracts.StatusDone
			ticket.Lease = contracts.LeaseState{}
		}
		if err := s.commitTicketSnapshotEvent(ctx, "approve ticket", ticket, actor, reason, contracts.EventTicketApproved, ticket); err != nil {
			return contracts.TicketSnapshot{}, err
		}
		return ticket, nil
	})
}

func (s *ActionService) RejectTicket(ctx context.Context, ticketID string, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	return withWriteLock(ctx, s.LockManager, "reject ticket", func(ctx context.Context) (contracts.TicketSnapshot, error) {
		if !actor.IsValid() {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if strings.TrimSpace(reason) == "" {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeInvalidInput, "reject requires a reason")
		}
		ticket, err := s.Tickets.GetTicket(ctx, ticketID)
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
		if ticket.Status != contracts.StatusInReview {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("ticket %s is not in review", ticket.ID))
		}
		if actor != contracts.Actor("human:owner") && actor != ticket.Reviewer {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodePermissionDenied, "only the assigned reviewer or human:owner can reject")
		}
		now := s.now()
		ticket.Status = contracts.StatusInProgress
		ticket.ReviewState = contracts.ReviewStateChangesRequested
		if ticket.Lease.Kind == contracts.LeaseKindReview {
			ticket.Lease = contracts.LeaseState{}
		}
		ticket.UpdatedAt = now
		if err := s.commitTicketSnapshotEvent(ctx, "reject ticket", ticket, actor, reason, contracts.EventTicketRejected, ticket); err != nil {
			return contracts.TicketSnapshot{}, err
		}
		return ticket, nil
	})
}

func (s *ActionService) CompleteTicket(ctx context.Context, ticketID string, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	return withWriteLock(ctx, s.LockManager, "complete ticket", func(ctx context.Context) (contracts.TicketSnapshot, error) {
		if !actor.IsValid() {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		ticket, err := s.Tickets.GetTicket(ctx, ticketID)
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
		if ticket.Status != contracts.StatusInReview {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("ticket %s must be in_review to complete", ticket.ID))
		}
		policy, err := resolveEffectivePolicy(ctx, s.Root, s.Projects, s.Tickets, ticket)
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
		if ticket.ReviewState != contracts.ReviewStateApproved {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("ticket %s must be approved before completion", ticket.ID))
		}
		if len(ticket.OpenGateIDs) > 0 {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("ticket %s cannot complete while gates are open", ticket.ID))
		}
		actorAgent, _ := actorAgentProfile(ctx, s.Agents, actor)
		changedFiles, known := permissionChangedFilesForTicket(ctx, s.Runs, s.Changes, s.Root, ticket)
		if _, err := s.requirePermission(ctx, permissionEvalInput{
			Action:            contracts.PermissionActionTicketComplete,
			Actor:             actor,
			Ticket:            ticket,
			ActorAgent:        actorAgent,
			Runbook:           permissionRunbook(ticket, nil),
			ChangedFiles:      changedFiles,
			ChangedFilesKnown: known,
		}); err != nil {
			return contracts.TicketSnapshot{}, err
		}
		if err := domain.CheckCompletionPermission(policy.CompletionMode, actor, ticket.Reviewer); err != nil {
			return contracts.TicketSnapshot{}, &apperr.Error{Code: apperr.CodePermissionDenied, Message: err.Error(), Cause: err}
		}
		now := s.now()
		from := ticket.Status
		ticket.Status = contracts.StatusDone
		ticket.Lease = contracts.LeaseState{}
		ticket.UpdatedAt = now
		payload := map[string]any{"from": from, "to": contracts.StatusDone, "ticket": ticket}
		if err := s.commitTicketSnapshotEvent(ctx, "complete ticket", ticket, actor, reason, contracts.EventTicketMoved, payload); err != nil {
			return contracts.TicketSnapshot{}, err
		}
		return ticket, nil
	})
}

func (s *ActionService) GetTicketPolicy(ctx context.Context, ticketID string) (contracts.TicketPolicy, EffectivePolicyView, error) {
	ticket, err := s.Tickets.GetTicket(ctx, ticketID)
	if err != nil {
		return contracts.TicketPolicy{}, EffectivePolicyView{}, err
	}
	effective, err := resolveEffectivePolicy(ctx, s.Root, s.Projects, s.Tickets, ticket)
	if err != nil {
		return contracts.TicketPolicy{}, EffectivePolicyView{}, err
	}
	return ticket.Policy, effective, nil
}

func (s *ActionService) SetTicketPolicy(ctx context.Context, ticketID string, policy contracts.TicketPolicy, actor contracts.Actor, reason string) (contracts.TicketSnapshot, error) {
	return withWriteLock(ctx, s.LockManager, "set ticket policy", func(ctx context.Context) (contracts.TicketSnapshot, error) {
		if !actor.IsValid() {
			return contracts.TicketSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if err := policy.Validate(); err != nil {
			return contracts.TicketSnapshot{}, err
		}
		ticket, err := s.Tickets.GetTicket(ctx, ticketID)
		if err != nil {
			return contracts.TicketSnapshot{}, err
		}
		ticket.Policy = policy
		ticket.UpdatedAt = s.now()
		if err := s.commitTicketSnapshotEvent(ctx, "set ticket policy", ticket, actor, reason, contracts.EventTicketPolicyUpdated, ticket); err != nil {
			return contracts.TicketSnapshot{}, err
		}
		return ticket, nil
	})
}

func (s *ActionService) GetProjectPolicy(ctx context.Context, key string) (contracts.ProjectDefaults, error) {
	project, err := s.Projects.GetProject(ctx, key)
	if err != nil {
		return contracts.ProjectDefaults{}, err
	}
	return project.Defaults, nil
}

func (s *ActionService) SetProjectPolicy(ctx context.Context, key string, defaults contracts.ProjectDefaults, actor contracts.Actor, reason string) (contracts.Project, error) {
	return withWriteLock(ctx, s.LockManager, "set project policy", func(ctx context.Context) (contracts.Project, error) {
		if !actor.IsValid() {
			return contracts.Project{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if err := defaults.Validate(); err != nil {
			return contracts.Project{}, err
		}
		project, err := s.Projects.GetProject(ctx, key)
		if err != nil {
			return contracts.Project{}, err
		}
		project.Defaults = defaults
		event, err := s.newEvent(ctx, key, s.now(), actor, reason, contracts.EventProjectPolicyUpdated, "", project)
		if err != nil {
			return contracts.Project{}, err
		}
		if err := s.commitMutation(ctx, "set project policy", "project_snapshot", event, func(ctx context.Context) error {
			return s.UpdateProject(ctx, project)
		}); err != nil {
			return contracts.Project{}, err
		}
		return project, nil
	})
}

func (s *ActionService) expireLease(ctx context.Context, ticket *contracts.TicketSnapshot, reason string) (contracts.TicketSnapshot, error) {
	now := s.now()
	expiredActor := ticket.Lease.Actor
	ticket.Lease = contracts.LeaseState{}
	ticket.UpdatedAt = now
	eventID, err := s.NextEventID(ctx, ticket.Project)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}
	why := strings.TrimSpace(reason)
	if why == "" {
		why = "lease expired"
	}
	event := contracts.Event{
		EventID:       eventID,
		Timestamp:     now,
		Actor:         expiredActor,
		Reason:        why,
		Type:          contracts.EventTicketLeaseExpired,
		Project:       ticket.Project,
		TicketID:      ticket.ID,
		Payload:       *ticket,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := s.commitMutation(ctx, "expire lease", "ticket_snapshot", event, func(ctx context.Context) error {
		return s.UpdateTicket(ctx, *ticket)
	}); err != nil {
		return contracts.TicketSnapshot{}, err
	}
	return *ticket, nil
}

func (s *ActionService) loadTicketsMap(ctx context.Context) (map[string]contracts.TicketSnapshot, error) {
	tickets, err := s.Tickets.ListTickets(ctx, contracts.TicketListOptions{IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	mapped := make(map[string]contracts.TicketSnapshot, len(tickets))
	for _, ticket := range tickets {
		mapped[ticket.ID] = ticket
	}
	return mapped, nil
}
