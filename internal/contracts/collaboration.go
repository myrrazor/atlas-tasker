package contracts

import (
	"fmt"
	"strings"
	"time"
)

type CollaboratorStatus string

const (
	CollaboratorStatusActive    CollaboratorStatus = "active"
	CollaboratorStatusSuspended CollaboratorStatus = "suspended"
	CollaboratorStatusRemoved   CollaboratorStatus = "removed"
)

var validCollaboratorStatuses = map[CollaboratorStatus]struct{}{
	CollaboratorStatusActive:    {},
	CollaboratorStatusSuspended: {},
	CollaboratorStatusRemoved:   {},
}

func (s CollaboratorStatus) IsValid() bool {
	_, ok := validCollaboratorStatuses[s]
	return ok
}

type CollaboratorTrustState string

const (
	CollaboratorTrustStateUntrusted CollaboratorTrustState = "untrusted"
	CollaboratorTrustStateTrusted   CollaboratorTrustState = "trusted"
)

var validCollaboratorTrustStates = map[CollaboratorTrustState]struct{}{
	CollaboratorTrustStateUntrusted: {},
	CollaboratorTrustStateTrusted:   {},
}

func (s CollaboratorTrustState) IsValid() bool {
	_, ok := validCollaboratorTrustStates[s]
	return ok
}

type MembershipScopeKind string

const (
	MembershipScopeWorkspace MembershipScopeKind = "workspace"
	MembershipScopeProject   MembershipScopeKind = "project"
)

var validMembershipScopeKinds = map[MembershipScopeKind]struct{}{
	MembershipScopeWorkspace: {},
	MembershipScopeProject:   {},
}

func (k MembershipScopeKind) IsValid() bool {
	_, ok := validMembershipScopeKinds[k]
	return ok
}

type MembershipRole string

const (
	MembershipRoleOwner       MembershipRole = "owner"
	MembershipRoleMaintainer  MembershipRole = "maintainer"
	MembershipRoleContributor MembershipRole = "contributor"
	MembershipRoleReviewer    MembershipRole = "reviewer"
	MembershipRoleObserver    MembershipRole = "observer"
)

var validMembershipRoles = map[MembershipRole]struct{}{
	MembershipRoleOwner:       {},
	MembershipRoleMaintainer:  {},
	MembershipRoleContributor: {},
	MembershipRoleReviewer:    {},
	MembershipRoleObserver:    {},
}

func (r MembershipRole) IsValid() bool {
	_, ok := validMembershipRoles[r]
	return ok
}

type MembershipStatus string

const (
	MembershipStatusActive  MembershipStatus = "active"
	MembershipStatusUnbound MembershipStatus = "unbound"
)

var validMembershipStatuses = map[MembershipStatus]struct{}{
	MembershipStatusActive:  {},
	MembershipStatusUnbound: {},
}

func (s MembershipStatus) IsValid() bool {
	_, ok := validMembershipStatuses[s]
	return ok
}

type SyncRemoteKind string

const (
	SyncRemoteKindGit  SyncRemoteKind = "git"
	SyncRemoteKindPath SyncRemoteKind = "path"
)

var validSyncRemoteKinds = map[SyncRemoteKind]struct{}{
	SyncRemoteKindGit:  {},
	SyncRemoteKindPath: {},
}

func (k SyncRemoteKind) IsValid() bool {
	_, ok := validSyncRemoteKinds[k]
	return ok
}

type SyncDefaultAction string

const (
	SyncDefaultActionFetch SyncDefaultAction = "fetch"
	SyncDefaultActionPull  SyncDefaultAction = "pull"
	SyncDefaultActionPush  SyncDefaultAction = "push"
)

var validSyncDefaultActions = map[SyncDefaultAction]struct{}{
	SyncDefaultActionFetch: {},
	SyncDefaultActionPull:  {},
	SyncDefaultActionPush:  {},
}

func (a SyncDefaultAction) IsValid() bool {
	_, ok := validSyncDefaultActions[a]
	return ok
}

type SyncJobMode string

const (
	SyncJobModeStatus       SyncJobMode = "status"
	SyncJobModeFetch        SyncJobMode = "fetch"
	SyncJobModePull         SyncJobMode = "pull"
	SyncJobModePush         SyncJobMode = "push"
	SyncJobModeRun          SyncJobMode = "run"
	SyncJobModeBundleCreate SyncJobMode = "bundle_create"
	SyncJobModeBundleImport SyncJobMode = "bundle_import"
	SyncJobModeBundleVerify SyncJobMode = "bundle_verify"
)

var validSyncJobModes = map[SyncJobMode]struct{}{
	SyncJobModeStatus:       {},
	SyncJobModeFetch:        {},
	SyncJobModePull:         {},
	SyncJobModePush:         {},
	SyncJobModeRun:          {},
	SyncJobModeBundleCreate: {},
	SyncJobModeBundleImport: {},
	SyncJobModeBundleVerify: {},
}

func (m SyncJobMode) IsValid() bool {
	_, ok := validSyncJobModes[m]
	return ok
}

type SyncJobState string

const (
	SyncJobStatePlanned     SyncJobState = "planned"
	SyncJobStateScanning    SyncJobState = "scanning"
	SyncJobStateVerifying   SyncJobState = "verifying"
	SyncJobStateReconciling SyncJobState = "reconciling"
	SyncJobStateApplying    SyncJobState = "applying"
	SyncJobStatePublishing  SyncJobState = "publishing"
	SyncJobStateCompleted   SyncJobState = "completed"
	SyncJobStateFailed      SyncJobState = "failed"
	SyncJobStateCanceled    SyncJobState = "canceled"
)

var validSyncJobStates = map[SyncJobState]struct{}{
	SyncJobStatePlanned:     {},
	SyncJobStateScanning:    {},
	SyncJobStateVerifying:   {},
	SyncJobStateReconciling: {},
	SyncJobStateApplying:    {},
	SyncJobStatePublishing:  {},
	SyncJobStateCompleted:   {},
	SyncJobStateFailed:      {},
	SyncJobStateCanceled:    {},
}

func (s SyncJobState) IsValid() bool {
	_, ok := validSyncJobStates[s]
	return ok
}

type ConflictType string

const (
	ConflictTypeScalarDivergence        ConflictType = "scalar_divergence"
	ConflictTypeTerminalStateDivergence ConflictType = "terminal_state_divergence"
	ConflictTypeUIDCollision            ConflictType = "uid_collision"
	ConflictTypeTrustStateDivergence    ConflictType = "trust_state_divergence"
	ConflictTypeMembershipDivergence    ConflictType = "membership_divergence"
	ConflictTypeGateDivergence          ConflictType = "gate_divergence"
	ConflictTypeRunStateDivergence      ConflictType = "run_state_divergence"
	ConflictTypeChangeDivergence        ConflictType = "change_divergence"
	ConflictTypeCheckDivergence         ConflictType = "check_divergence"
)

var validConflictTypes = map[ConflictType]struct{}{
	ConflictTypeScalarDivergence:        {},
	ConflictTypeTerminalStateDivergence: {},
	ConflictTypeUIDCollision:            {},
	ConflictTypeTrustStateDivergence:    {},
	ConflictTypeMembershipDivergence:    {},
	ConflictTypeGateDivergence:          {},
	ConflictTypeRunStateDivergence:      {},
	ConflictTypeChangeDivergence:        {},
	ConflictTypeCheckDivergence:         {},
}

func (t ConflictType) IsValid() bool {
	_, ok := validConflictTypes[t]
	return ok
}

type ConflictStatus string

const (
	ConflictStatusOpen       ConflictStatus = "open"
	ConflictStatusResolved   ConflictStatus = "resolved"
	ConflictStatusSuperseded ConflictStatus = "superseded"
)

var validConflictStatuses = map[ConflictStatus]struct{}{
	ConflictStatusOpen:       {},
	ConflictStatusResolved:   {},
	ConflictStatusSuperseded: {},
}

func (s ConflictStatus) IsValid() bool {
	_, ok := validConflictStatuses[s]
	return ok
}

type ConflictResolution string

const (
	ConflictResolutionUseLocal  ConflictResolution = "use_local"
	ConflictResolutionUseRemote ConflictResolution = "use_remote"
)

var validConflictResolutions = map[ConflictResolution]struct{}{
	ConflictResolutionUseLocal:  {},
	ConflictResolutionUseRemote: {},
}

func (r ConflictResolution) IsValid() bool {
	_, ok := validConflictResolutions[r]
	return ok
}

type CollaboratorProfile struct {
	CollaboratorID  string                 `json:"collaborator_id" yaml:"collaborator_id"`
	DisplayName     string                 `json:"display_name,omitempty" yaml:"display_name,omitempty"`
	Status          CollaboratorStatus     `json:"status" yaml:"status"`
	TrustState      CollaboratorTrustState `json:"trust_state" yaml:"trust_state"`
	AtlasActors     []Actor                `json:"atlas_actors,omitempty" yaml:"atlas_actors,omitempty"`
	ProviderHandles map[string]string      `json:"provider_handles,omitempty" yaml:"provider_handles,omitempty"`
	CreatedAt       time.Time              `json:"created_at" yaml:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at" yaml:"updated_at"`
	SchemaVersion   int                    `json:"schema_version" yaml:"schema_version"`
}

func (c CollaboratorProfile) Validate() error {
	if strings.TrimSpace(c.CollaboratorID) == "" {
		return fmt.Errorf("collaborator_id is required")
	}
	if !c.Status.IsValid() {
		return fmt.Errorf("invalid collaborator status: %s", c.Status)
	}
	if !c.TrustState.IsValid() {
		return fmt.Errorf("invalid collaborator trust_state: %s", c.TrustState)
	}
	for _, actor := range c.AtlasActors {
		if !actor.IsValid() {
			return fmt.Errorf("invalid collaborator atlas_actor: %s", actor)
		}
	}
	return nil
}

type MembershipBinding struct {
	MembershipUID             string              `json:"membership_uid" yaml:"membership_uid"`
	CollaboratorID            string              `json:"collaborator_id" yaml:"collaborator_id"`
	ScopeKind                 MembershipScopeKind `json:"scope_kind" yaml:"scope_kind"`
	ScopeID                   string              `json:"scope_id" yaml:"scope_id"`
	Role                      MembershipRole      `json:"role" yaml:"role"`
	Status                    MembershipStatus    `json:"status" yaml:"status"`
	DefaultPermissionProfiles []string            `json:"default_permission_profiles,omitempty" yaml:"default_permission_profiles,omitempty"`
	CreatedAt                 time.Time           `json:"created_at" yaml:"created_at"`
	UpdatedAt                 time.Time           `json:"updated_at" yaml:"updated_at"`
	EndedAt                   time.Time           `json:"ended_at,omitempty" yaml:"ended_at,omitempty"`
}

func (m MembershipBinding) Validate() error {
	if strings.TrimSpace(m.MembershipUID) == "" {
		return fmt.Errorf("membership_uid is required")
	}
	if strings.TrimSpace(m.CollaboratorID) == "" {
		return fmt.Errorf("collaborator_id is required")
	}
	if !m.ScopeKind.IsValid() {
		return fmt.Errorf("invalid membership scope_kind: %s", m.ScopeKind)
	}
	if strings.TrimSpace(m.ScopeID) == "" {
		return fmt.Errorf("scope_id is required")
	}
	if !m.Role.IsValid() {
		return fmt.Errorf("invalid membership role: %s", m.Role)
	}
	if !m.Status.IsValid() {
		return fmt.Errorf("invalid membership status: %s", m.Status)
	}
	return nil
}

type Mention struct {
	MentionUID        string    `json:"mention_uid" yaml:"mention_uid"`
	CollaboratorID    string    `json:"collaborator_id" yaml:"collaborator_id"`
	SourceKind        string    `json:"source_kind" yaml:"source_kind"`
	SourceID          string    `json:"source_id" yaml:"source_id"`
	SourceEventUID    string    `json:"source_event_uid" yaml:"source_event_uid"`
	TicketID          string    `json:"ticket_id,omitempty" yaml:"ticket_id,omitempty"`
	OriginWorkspaceID string    `json:"origin_workspace_id,omitempty" yaml:"origin_workspace_id,omitempty"`
	CreatedAt         time.Time `json:"created_at" yaml:"created_at"`
}

func (m Mention) Validate() error {
	if strings.TrimSpace(m.MentionUID) == "" {
		return fmt.Errorf("mention_uid is required")
	}
	if strings.TrimSpace(m.CollaboratorID) == "" {
		return fmt.Errorf("collaborator_id is required")
	}
	if strings.TrimSpace(m.SourceKind) == "" {
		return fmt.Errorf("source_kind is required")
	}
	if strings.TrimSpace(m.SourceID) == "" {
		return fmt.Errorf("source_id is required")
	}
	if strings.TrimSpace(m.SourceEventUID) == "" {
		return fmt.Errorf("source_event_uid is required")
	}
	return nil
}

type SyncRemote struct {
	RemoteID      string            `json:"remote_id" yaml:"remote_id"`
	Kind          SyncRemoteKind    `json:"kind" yaml:"kind"`
	Location      string            `json:"location" yaml:"location"`
	Enabled       bool              `json:"enabled" yaml:"enabled"`
	DefaultAction SyncDefaultAction `json:"default_action,omitempty" yaml:"default_action,omitempty"`
	LastSuccessAt time.Time         `json:"last_success_at,omitempty" yaml:"last_success_at,omitempty"`
	LastJobID     string            `json:"last_job_id,omitempty" yaml:"last_job_id,omitempty"`
	CreatedAt     time.Time         `json:"created_at" yaml:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at" yaml:"updated_at"`
}

func (r SyncRemote) Validate() error {
	if strings.TrimSpace(r.RemoteID) == "" {
		return fmt.Errorf("remote_id is required")
	}
	if !r.Kind.IsValid() {
		return fmt.Errorf("invalid remote kind: %s", r.Kind)
	}
	if strings.TrimSpace(r.Location) == "" {
		return fmt.Errorf("location is required")
	}
	if r.DefaultAction != "" && !r.DefaultAction.IsValid() {
		return fmt.Errorf("invalid default_action: %s", r.DefaultAction)
	}
	return nil
}

type SyncJob struct {
	JobID         string         `json:"job_id" yaml:"job_id"`
	RemoteID      string         `json:"remote_id,omitempty" yaml:"remote_id,omitempty"`
	BundleRef     string         `json:"bundle_ref,omitempty" yaml:"bundle_ref,omitempty"`
	Mode          SyncJobMode    `json:"mode" yaml:"mode"`
	State         SyncJobState   `json:"state" yaml:"state"`
	StartedAt     time.Time      `json:"started_at" yaml:"started_at"`
	FinishedAt    time.Time      `json:"finished_at,omitempty" yaml:"finished_at,omitempty"`
	Warnings      []string       `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	ReasonCodes   []string       `json:"reason_codes,omitempty" yaml:"reason_codes,omitempty"`
	Counts        map[string]int `json:"counts,omitempty" yaml:"counts,omitempty"`
	ConflictIDs   []string       `json:"conflict_ids,omitempty" yaml:"conflict_ids,omitempty"`
	SchemaVersion int            `json:"schema_version" yaml:"schema_version"`
}

func (j SyncJob) Validate() error {
	if strings.TrimSpace(j.JobID) == "" {
		return fmt.Errorf("job_id is required")
	}
	if !j.Mode.IsValid() {
		return fmt.Errorf("invalid sync job mode: %s", j.Mode)
	}
	if !j.State.IsValid() {
		return fmt.Errorf("invalid sync job state: %s", j.State)
	}
	return nil
}

type ConflictRecord struct {
	ConflictID    string             `json:"conflict_id" yaml:"conflict_id"`
	EntityKind    string             `json:"entity_kind" yaml:"entity_kind"`
	EntityUID     string             `json:"entity_uid" yaml:"entity_uid"`
	ConflictType  ConflictType       `json:"conflict_type" yaml:"conflict_type"`
	LocalRef      string             `json:"local_ref,omitempty" yaml:"local_ref,omitempty"`
	RemoteRef     string             `json:"remote_ref,omitempty" yaml:"remote_ref,omitempty"`
	Status        ConflictStatus     `json:"status" yaml:"status"`
	OpenedByJob   string             `json:"opened_by_job,omitempty" yaml:"opened_by_job,omitempty"`
	OpenedAt      time.Time          `json:"opened_at" yaml:"opened_at"`
	ResolvedAt    time.Time          `json:"resolved_at,omitempty" yaml:"resolved_at,omitempty"`
	ResolvedBy    Actor              `json:"resolved_by,omitempty" yaml:"resolved_by,omitempty"`
	Resolution    ConflictResolution `json:"resolution,omitempty" yaml:"resolution,omitempty"`
	SchemaVersion int                `json:"schema_version" yaml:"schema_version"`
}

func (c ConflictRecord) Validate() error {
	if strings.TrimSpace(c.ConflictID) == "" {
		return fmt.Errorf("conflict_id is required")
	}
	if strings.TrimSpace(c.EntityKind) == "" {
		return fmt.Errorf("entity_kind is required")
	}
	if strings.TrimSpace(c.EntityUID) == "" {
		return fmt.Errorf("entity_uid is required")
	}
	if !c.ConflictType.IsValid() {
		return fmt.Errorf("invalid conflict_type: %s", c.ConflictType)
	}
	if !c.Status.IsValid() {
		return fmt.Errorf("invalid conflict status: %s", c.Status)
	}
	if c.ResolvedBy != "" && !c.ResolvedBy.IsValid() {
		return fmt.Errorf("invalid conflict resolved_by: %s", c.ResolvedBy)
	}
	if c.Resolution != "" && !c.Resolution.IsValid() {
		return fmt.Errorf("invalid conflict resolution: %s", c.Resolution)
	}
	return nil
}

type TeamInboxItem struct {
	CollaboratorID    string    `json:"collaborator_id"`
	ItemKind          string    `json:"item_kind"`
	ItemID            string    `json:"item_id"`
	TicketID          string    `json:"ticket_id,omitempty"`
	OriginWorkspaceID string    `json:"origin_workspace_id,omitempty"`
	Summary           string    `json:"summary,omitempty"`
	GeneratedAt       time.Time `json:"generated_at"`
}
