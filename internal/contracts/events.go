package contracts

import (
	"fmt"
	"strings"
	"time"
)

// EventType captures every persisted mutation event.
type EventType string

type EventSurface string

const (
	EventSurfaceCLI        EventSurface = "cli"
	EventSurfaceShell      EventSurface = "shell"
	EventSurfaceTUI        EventSurface = "tui"
	EventSurfaceAutomation EventSurface = "automation"
	EventSurfaceRepair     EventSurface = "repair"
	EventSurfaceGit        EventSurface = "git"
	EventSurfaceGH         EventSurface = "gh"
	EventSurfaceMCP        EventSurface = "mcp"
)

var validEventSurfaces = map[EventSurface]struct{}{
	EventSurfaceCLI:        {},
	EventSurfaceShell:      {},
	EventSurfaceTUI:        {},
	EventSurfaceAutomation: {},
	EventSurfaceRepair:     {},
	EventSurfaceGit:        {},
	EventSurfaceGH:         {},
	EventSurfaceMCP:        {},
}

func (s EventSurface) IsValid() bool {
	_, ok := validEventSurfaces[s]
	return ok
}

const (
	EventTicketCreated                    EventType = "ticket.created"
	EventTicketUpdated                    EventType = "ticket.updated"
	EventTicketMoved                      EventType = "ticket.moved"
	EventTicketCommented                  EventType = "ticket.commented"
	EventTicketLinked                     EventType = "ticket.linked"
	EventTicketUnlinked                   EventType = "ticket.unlinked"
	EventTicketClosed                     EventType = "ticket.closed"
	EventTicketClaimed                    EventType = "ticket.claimed"
	EventTicketReleased                   EventType = "ticket.released"
	EventTicketHeartbeat                  EventType = "ticket.heartbeat"
	EventTicketLeaseExpired               EventType = "ticket.lease_expired"
	EventTicketReviewRequested            EventType = "ticket.review_requested"
	EventTicketApproved                   EventType = "ticket.approved"
	EventTicketRejected                   EventType = "ticket.rejected"
	EventTicketPolicyUpdated              EventType = "ticket.policy_updated"
	EventTicketTemplateApplied            EventType = "ticket.template_applied"
	EventOwnerAttentionRaised             EventType = "ticket.owner_attention_required"
	EventOwnerAttentionCleared            EventType = "ticket.owner_attention_cleared"
	EventProjectPolicyUpdated             EventType = "project.policy_updated"
	EventConfigChanged                    EventType = "config.changed"
	EventAgentCreated                     EventType = "agent.created"
	EventAgentUpdated                     EventType = "agent.updated"
	EventAgentEnabled                     EventType = "agent.enabled"
	EventAgentDisabled                    EventType = "agent.disabled"
	EventRunCreated                       EventType = "run.created"
	EventRunDispatched                    EventType = "run.dispatched"
	EventRunStarted                       EventType = "run.started"
	EventRunAttached                      EventType = "run.attached"
	EventRunCheckpointed                  EventType = "run.checkpointed"
	EventRunEvidenceAdded                 EventType = "run.evidence_added"
	EventRunHandoffRequested              EventType = "run.handoff_requested"
	EventRunCompleted                     EventType = "run.completed"
	EventRunFailed                        EventType = "run.failed"
	EventRunAborted                       EventType = "run.aborted"
	EventRunCleanedUp                     EventType = "run.cleaned_up"
	EventWorktreeCreated                  EventType = "worktree.created"
	EventWorktreePruned                   EventType = "worktree.pruned"
	EventWorktreeRepaired                 EventType = "worktree.repaired"
	EventGateOpened                       EventType = "gate.opened"
	EventGateApproved                     EventType = "gate.approved"
	EventGateRejected                     EventType = "gate.rejected"
	EventGateWaived                       EventType = "gate.waived"
	EventChangeLinked                     EventType = "change.linked"
	EventChangeCreated                    EventType = "change.created"
	EventChangeSynced                     EventType = "change.synced"
	EventChangeUpdated                    EventType = "change.updated"
	EventChangeReviewRequested            EventType = "change.review_requested"
	EventChangeMerged                     EventType = "change.merged"
	EventChangeUnlinked                   EventType = "change.unlinked"
	EventChangeSuperseded                 EventType = "change.superseded"
	EventChangeExternalDrifted            EventType = "change.external_drifted"
	EventCheckRecorded                    EventType = "check.recorded"
	EventCheckUpdated                     EventType = "check.updated"
	EventCheckSynced                      EventType = "check.synced"
	EventPermissionProfileCreated         EventType = "permission_profile.created"
	EventPermissionProfileUpdated         EventType = "permission_profile.updated"
	EventPermissionProfileBound           EventType = "permission_profile.bound"
	EventPermissionProfileUnbound         EventType = "permission_profile.unbound"
	EventPermissionProfileOverrideApplied EventType = "permission_profile.override_applied"
	EventImportPreviewed                  EventType = "import.previewed"
	EventImportValidated                  EventType = "import.validated"
	EventImportStarted                    EventType = "import.started"
	EventImportApplied                    EventType = "import.applied"
	EventImportFailed                     EventType = "import.failed"
	EventImportCanceled                   EventType = "import.canceled"
	EventExportCreated                    EventType = "export.created"
	EventExportVerified                   EventType = "export.verified"
	EventArchivePlanned                   EventType = "archive.planned"
	EventArchiveApplied                   EventType = "archive.applied"
	EventArchiveRestored                  EventType = "archive.restored"
	EventCompactCompleted                 EventType = "compact.completed"
	EventCollaboratorAdded                EventType = "collaborator.added"
	EventCollaboratorEdited               EventType = "collaborator.edited"
	EventCollaboratorTrusted              EventType = "collaborator.trusted"
	EventCollaboratorSuspended            EventType = "collaborator.suspended"
	EventCollaboratorRemoved              EventType = "collaborator.removed"
	EventMembershipBound                  EventType = "membership.bound"
	EventMembershipUnbound                EventType = "membership.unbound"
	EventMentionRecorded                  EventType = "mention.recorded"
	EventRemoteAdded                      EventType = "remote.added"
	EventRemoteEdited                     EventType = "remote.edited"
	EventRemoteRemoved                    EventType = "remote.removed"
	EventSyncStarted                      EventType = "sync.started"
	EventSyncCompleted                    EventType = "sync.completed"
	EventSyncFailed                       EventType = "sync.failed"
	EventBundleCreated                    EventType = "bundle.created"
	EventBundleImported                   EventType = "bundle.imported"
	EventBundleVerified                   EventType = "bundle.verified"
	EventConflictOpened                   EventType = "conflict.opened"
	EventConflictResolved                 EventType = "conflict.resolved"
	EventProviderMappingUpdated           EventType = "provider_mapping.updated"
	EventCodeownersRendered               EventType = "codeowners.rendered"
	EventCodeownersWritten                EventType = "codeowners.written"
	EventKeyGenerated                     EventType = "key.generated"
	EventKeyImported                      EventType = "key.imported"
	EventKeyRotated                       EventType = "key.rotated"
	EventKeyRevoked                       EventType = "key.revoked"
	EventTrustBound                       EventType = "trust.bound"
	EventTrustRevoked                     EventType = "trust.revoked"
	EventSignatureCreated                 EventType = "signature.created"
	EventSignatureVerified                EventType = "signature.verified"
	EventGovernancePackCreated            EventType = "governance.pack.created"
	EventGovernancePackApplied            EventType = "governance.pack.applied"
	EventGovernancePolicyUpdated          EventType = "governance.policy.updated"
	EventGovernanceOverrideRecorded       EventType = "governance.override.recorded"
	EventClassificationSet                EventType = "classification.set"
	EventRedactionPreviewed               EventType = "redaction.previewed"
	EventRedactionExported                EventType = "redaction.exported"
	EventAuditReportCreated               EventType = "audit.report.created"
	EventAuditReportExported              EventType = "audit.report.exported"
	EventBackupCreated                    EventType = "backup.created"
	EventBackupVerified                   EventType = "backup.verified"
	EventBackupRestorePlanned             EventType = "backup.restore_planned"
	EventBackupRestored                   EventType = "backup.restored"
	EventGoalManifestGenerated            EventType = "goal.manifest.generated"
)

var validEventTypes = map[EventType]struct{}{
	EventTicketCreated:                    {},
	EventTicketUpdated:                    {},
	EventTicketMoved:                      {},
	EventTicketCommented:                  {},
	EventTicketLinked:                     {},
	EventTicketUnlinked:                   {},
	EventTicketClosed:                     {},
	EventTicketClaimed:                    {},
	EventTicketReleased:                   {},
	EventTicketHeartbeat:                  {},
	EventTicketLeaseExpired:               {},
	EventTicketReviewRequested:            {},
	EventTicketApproved:                   {},
	EventTicketRejected:                   {},
	EventTicketPolicyUpdated:              {},
	EventTicketTemplateApplied:            {},
	EventOwnerAttentionRaised:             {},
	EventOwnerAttentionCleared:            {},
	EventProjectPolicyUpdated:             {},
	EventConfigChanged:                    {},
	EventAgentCreated:                     {},
	EventAgentUpdated:                     {},
	EventAgentEnabled:                     {},
	EventAgentDisabled:                    {},
	EventRunCreated:                       {},
	EventRunDispatched:                    {},
	EventRunStarted:                       {},
	EventRunAttached:                      {},
	EventRunCheckpointed:                  {},
	EventRunEvidenceAdded:                 {},
	EventRunHandoffRequested:              {},
	EventRunCompleted:                     {},
	EventRunFailed:                        {},
	EventRunAborted:                       {},
	EventRunCleanedUp:                     {},
	EventWorktreeCreated:                  {},
	EventWorktreePruned:                   {},
	EventWorktreeRepaired:                 {},
	EventGateOpened:                       {},
	EventGateApproved:                     {},
	EventGateRejected:                     {},
	EventGateWaived:                       {},
	EventChangeLinked:                     {},
	EventChangeCreated:                    {},
	EventChangeSynced:                     {},
	EventChangeUpdated:                    {},
	EventChangeReviewRequested:            {},
	EventChangeMerged:                     {},
	EventChangeUnlinked:                   {},
	EventChangeSuperseded:                 {},
	EventChangeExternalDrifted:            {},
	EventCheckRecorded:                    {},
	EventCheckUpdated:                     {},
	EventCheckSynced:                      {},
	EventPermissionProfileCreated:         {},
	EventPermissionProfileUpdated:         {},
	EventPermissionProfileBound:           {},
	EventPermissionProfileUnbound:         {},
	EventPermissionProfileOverrideApplied: {},
	EventImportPreviewed:                  {},
	EventImportValidated:                  {},
	EventImportStarted:                    {},
	EventImportApplied:                    {},
	EventImportFailed:                     {},
	EventImportCanceled:                   {},
	EventExportCreated:                    {},
	EventExportVerified:                   {},
	EventArchivePlanned:                   {},
	EventArchiveApplied:                   {},
	EventArchiveRestored:                  {},
	EventCompactCompleted:                 {},
	EventCollaboratorAdded:                {},
	EventCollaboratorEdited:               {},
	EventCollaboratorTrusted:              {},
	EventCollaboratorSuspended:            {},
	EventCollaboratorRemoved:              {},
	EventMembershipBound:                  {},
	EventMembershipUnbound:                {},
	EventMentionRecorded:                  {},
	EventRemoteAdded:                      {},
	EventRemoteEdited:                     {},
	EventRemoteRemoved:                    {},
	EventSyncStarted:                      {},
	EventSyncCompleted:                    {},
	EventSyncFailed:                       {},
	EventBundleCreated:                    {},
	EventBundleImported:                   {},
	EventBundleVerified:                   {},
	EventConflictOpened:                   {},
	EventConflictResolved:                 {},
	EventProviderMappingUpdated:           {},
	EventCodeownersRendered:               {},
	EventCodeownersWritten:                {},
	EventKeyGenerated:                     {},
	EventKeyImported:                      {},
	EventKeyRotated:                       {},
	EventKeyRevoked:                       {},
	EventTrustBound:                       {},
	EventTrustRevoked:                     {},
	EventSignatureCreated:                 {},
	EventSignatureVerified:                {},
	EventGovernancePackCreated:            {},
	EventGovernancePackApplied:            {},
	EventGovernancePolicyUpdated:          {},
	EventGovernanceOverrideRecorded:       {},
	EventClassificationSet:                {},
	EventRedactionPreviewed:               {},
	EventRedactionExported:                {},
	EventAuditReportCreated:               {},
	EventAuditReportExported:              {},
	EventBackupCreated:                    {},
	EventBackupVerified:                   {},
	EventBackupRestorePlanned:             {},
	EventBackupRestored:                   {},
	EventGoalManifestGenerated:            {},
}

func (t EventType) IsValid() bool {
	_, ok := validEventTypes[t]
	return ok
}

// Event is the append-only JSONL record shape.
type EventMetadata struct {
	CorrelationID    string       `json:"correlation_id,omitempty"`
	CausationEventID int64        `json:"causation_event_id,omitempty"`
	MutationID       string       `json:"mutation_id,omitempty"`
	Surface          EventSurface `json:"surface,omitempty"`
	BatchID          string       `json:"batch_id,omitempty"`
	RootActor        Actor        `json:"root_actor,omitempty"`
}

type Event struct {
	EventID           int64     `json:"event_id"`
	EventUID          string    `json:"event_uid,omitempty"`
	Timestamp         time.Time `json:"timestamp"`
	OriginWorkspaceID string    `json:"origin_workspace_id,omitempty"`
	LogicalClock      int64     `json:"logical_clock,omitempty"`
	Actor             Actor     `json:"actor"`
	Reason            string    `json:"reason,omitempty"`
	Type              EventType `json:"type"`
	Project           string    `json:"project"`
	TicketID          string    `json:"ticket_id,omitempty"`
	// Payload is event-log context, not a signed artifact contract. If a future
	// release signs events, promote this to a typed payload first.
	Payload       any           `json:"payload,omitempty"`
	Metadata      EventMetadata `json:"metadata,omitempty"`
	SchemaVersion int           `json:"schema_version"`
}

func (e Event) Validate() error {
	if e.EventID <= 0 {
		return fmt.Errorf("event_id must be positive")
	}
	if e.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}
	if e.LogicalClock < 0 {
		return fmt.Errorf("logical_clock must be >= 0")
	}
	if !e.Actor.IsValid() {
		return fmt.Errorf("invalid actor: %s", e.Actor)
	}
	if !e.Type.IsValid() {
		return fmt.Errorf("invalid event type: %s", e.Type)
	}
	if strings.TrimSpace(e.Project) == "" {
		return fmt.Errorf("project is required")
	}
	if e.SchemaVersion <= 0 {
		return fmt.Errorf("schema_version is required")
	}
	if e.Metadata.Surface != "" && !e.Metadata.Surface.IsValid() {
		return fmt.Errorf("invalid metadata.surface: %s", e.Metadata.Surface)
	}
	if e.Metadata.RootActor != "" && !e.Metadata.RootActor.IsValid() {
		return fmt.Errorf("invalid metadata.root_actor: %s", e.Metadata.RootActor)
	}
	return nil
}

func NormalizeEvent(event Event) Event {
	if event.SchemaVersion == 0 {
		event.SchemaVersion = SchemaVersionV1
	}
	if event.LogicalClock == 0 && event.EventID > 0 {
		event.LogicalClock = event.EventID
	}
	if event.EventUID == "" {
		event.EventUID = CanonicalEventUID(event)
	}
	return event
}
