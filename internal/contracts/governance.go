package contracts

import (
	"fmt"
	"strings"
	"time"
)

type ProtectedAction string

const (
	ProtectedActionSyncImportApply   ProtectedAction = "sync_import_apply"
	ProtectedActionBundleImportApply ProtectedAction = "bundle_import_apply"
	ProtectedActionImportApply       ProtectedAction = "import_apply"
	ProtectedActionGateApprove       ProtectedAction = "gate_approve"
	ProtectedActionGateWaive         ProtectedAction = "gate_waive"
	ProtectedActionTicketComplete    ProtectedAction = "ticket_complete"
	ProtectedActionRunComplete       ProtectedAction = "run_complete"
	ProtectedActionChangeMerge       ProtectedAction = "change_merge"
	ProtectedActionExportCreate      ProtectedAction = "export_create"
	ProtectedActionArchiveApply      ProtectedAction = "archive_apply"
	ProtectedActionArchiveRestore    ProtectedAction = "archive_restore"
	ProtectedActionBackupRestore     ProtectedAction = "backup_restore"
	ProtectedActionTrustKey          ProtectedAction = "trust_key"
	ProtectedActionRevokeKey         ProtectedAction = "revoke_key"
	ProtectedActionRedactionOverride ProtectedAction = "redaction_override"
	ProtectedActionOwnerOverride     ProtectedAction = "owner_override"
)

var validProtectedActions = map[ProtectedAction]struct{}{
	ProtectedActionSyncImportApply: {}, ProtectedActionBundleImportApply: {}, ProtectedActionImportApply: {}, ProtectedActionGateApprove: {},
	ProtectedActionGateWaive: {}, ProtectedActionTicketComplete: {}, ProtectedActionRunComplete: {},
	ProtectedActionChangeMerge: {}, ProtectedActionExportCreate: {}, ProtectedActionArchiveRestore: {},
	ProtectedActionArchiveApply: {}, ProtectedActionBackupRestore: {}, ProtectedActionTrustKey: {}, ProtectedActionRevokeKey: {},
	ProtectedActionRedactionOverride: {}, ProtectedActionOwnerOverride: {},
}

func (a ProtectedAction) IsValid() bool {
	_, ok := validProtectedActions[a]
	return ok
}

type PolicyScopeKind string

const (
	PolicyScopeWorkspace      PolicyScopeKind = "workspace"
	PolicyScopeProject        PolicyScopeKind = "project"
	PolicyScopeRunbook        PolicyScopeKind = "runbook"
	PolicyScopeClassification PolicyScopeKind = "classification"
	PolicyScopeTicketType     PolicyScopeKind = "ticket_type"
)

var validPolicyScopeKinds = map[PolicyScopeKind]struct{}{
	PolicyScopeWorkspace: {}, PolicyScopeProject: {}, PolicyScopeRunbook: {}, PolicyScopeClassification: {}, PolicyScopeTicketType: {},
}

func (k PolicyScopeKind) IsValid() bool {
	_, ok := validPolicyScopeKinds[k]
	return ok
}

type GovernancePolicy struct {
	PolicyID                string                   `json:"policy_id" yaml:"policy_id" toml:"policy_id"`
	Name                    string                   `json:"name" yaml:"name" toml:"name"`
	Description             string                   `json:"description,omitempty" yaml:"description,omitempty" toml:"description,omitempty"`
	ScopeKind               PolicyScopeKind          `json:"scope_kind" yaml:"scope_kind" toml:"scope_kind"`
	ScopeID                 string                   `json:"scope_id,omitempty" yaml:"scope_id,omitempty" toml:"scope_id,omitempty"`
	ProtectedActions        []ProtectedAction        `json:"protected_actions,omitempty" yaml:"protected_actions,omitempty" toml:"protected_actions,omitempty"`
	RequiredSignatures      int                      `json:"required_signatures,omitempty" yaml:"required_signatures,omitempty" toml:"required_signatures,omitempty"`
	QuorumRules             []QuorumRule             `json:"quorum_rules,omitempty" yaml:"quorum_rules,omitempty" toml:"quorum_rules,omitempty"`
	SeparationOfDutiesRules []SeparationOfDutiesRule `json:"separation_of_duties_rules,omitempty" yaml:"separation_of_duties_rules,omitempty" toml:"separation_of_duties_rules,omitempty"`
	ClassificationRules     []string                 `json:"classification_rules,omitempty" yaml:"classification_rules,omitempty" toml:"classification_rules,omitempty"`
	OverrideRules           []OverrideRule           `json:"override_rules,omitempty" yaml:"override_rules,omitempty" toml:"override_rules,omitempty"`
	CreatedAt               time.Time                `json:"created_at" yaml:"created_at" toml:"created_at"`
	UpdatedAt               time.Time                `json:"updated_at" yaml:"updated_at" toml:"updated_at"`
	SchemaVersion           int                      `json:"schema_version" yaml:"schema_version" toml:"schema_version"`
}

func (p GovernancePolicy) Validate() error {
	if strings.TrimSpace(p.PolicyID) == "" || strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("policy_id and name are required")
	}
	if !p.ScopeKind.IsValid() {
		return fmt.Errorf("invalid policy scope kind: %s", p.ScopeKind)
	}
	if p.ScopeKind == PolicyScopeWorkspace && strings.TrimSpace(p.ScopeID) != "" {
		return fmt.Errorf("workspace policies must omit scope_id")
	}
	if p.ScopeKind != PolicyScopeWorkspace && strings.TrimSpace(p.ScopeID) == "" {
		return fmt.Errorf("%s policies require scope_id", p.ScopeKind)
	}
	if p.RequiredSignatures < 0 {
		return fmt.Errorf("required_signatures cannot be negative")
	}
	for _, action := range p.ProtectedActions {
		if !action.IsValid() {
			return fmt.Errorf("invalid protected action: %s", action)
		}
	}
	for _, rule := range p.QuorumRules {
		if err := rule.Validate(); err != nil {
			return err
		}
	}
	for _, rule := range p.SeparationOfDutiesRules {
		if err := rule.Validate(); err != nil {
			return err
		}
	}
	for _, rule := range p.OverrideRules {
		if err := rule.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type PolicyPack struct {
	PackID        string             `json:"pack_id" yaml:"pack_id" toml:"pack_id"`
	Name          string             `json:"name" yaml:"name" toml:"name"`
	Description   string             `json:"description,omitempty" yaml:"description,omitempty" toml:"description,omitempty"`
	Policies      []GovernancePolicy `json:"policies,omitempty" yaml:"policies,omitempty" toml:"policies,omitempty"`
	CreatedAt     time.Time          `json:"created_at" yaml:"created_at" toml:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at" yaml:"updated_at" toml:"updated_at"`
	SchemaVersion int                `json:"schema_version" yaml:"schema_version" toml:"schema_version"`
}

func (p PolicyPack) Validate() error {
	if strings.TrimSpace(p.PackID) == "" || strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("pack_id and name are required")
	}
	for _, policy := range p.Policies {
		if err := policy.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type QuorumRule struct {
	RuleID                       string           `json:"rule_id" yaml:"rule_id" toml:"rule_id"`
	ActionKind                   ProtectedAction  `json:"action_kind" yaml:"action_kind" toml:"action_kind"`
	RequiredCount                int              `json:"required_count" yaml:"required_count" toml:"required_count"`
	AllowedRoles                 []MembershipRole `json:"allowed_roles,omitempty" yaml:"allowed_roles,omitempty" toml:"allowed_roles,omitempty"`
	AllowedCollaborators         []string         `json:"allowed_collaborators,omitempty" yaml:"allowed_collaborators,omitempty" toml:"allowed_collaborators,omitempty"`
	DisallowActorFromPriorRoles  []string         `json:"disallow_actor_from_prior_roles,omitempty" yaml:"disallow_actor_from_prior_roles,omitempty" toml:"disallow_actor_from_prior_roles,omitempty"`
	RequireDistinctCollaborators bool             `json:"require_distinct_collaborators" yaml:"require_distinct_collaborators" toml:"require_distinct_collaborators"`
	RequireTrustedSignatures     bool             `json:"require_trusted_signatures" yaml:"require_trusted_signatures" toml:"require_trusted_signatures"`
}

func (r QuorumRule) Validate() error {
	if strings.TrimSpace(r.RuleID) == "" {
		return fmt.Errorf("quorum rule_id is required")
	}
	if !r.ActionKind.IsValid() {
		return fmt.Errorf("invalid quorum action: %s", r.ActionKind)
	}
	if r.RequiredCount < 1 {
		return fmt.Errorf("quorum required_count must be positive")
	}
	for _, role := range r.AllowedRoles {
		if !role.IsValid() {
			return fmt.Errorf("invalid quorum role: %s", role)
		}
	}
	return nil
}

type SeparationOfDutiesRule struct {
	RuleID                      string          `json:"rule_id" yaml:"rule_id" toml:"rule_id"`
	ActionKind                  ProtectedAction `json:"action_kind" yaml:"action_kind" toml:"action_kind"`
	ForbiddenActorRelationships []string        `json:"forbidden_actor_relationships,omitempty" yaml:"forbidden_actor_relationships,omitempty" toml:"forbidden_actor_relationships,omitempty"`
	LookbackEventTypes          []EventType     `json:"lookback_event_types,omitempty" yaml:"lookback_event_types,omitempty" toml:"lookback_event_types,omitempty"`
	LookbackScope               string          `json:"lookback_scope" yaml:"lookback_scope" toml:"lookback_scope"`
}

func (r SeparationOfDutiesRule) Validate() error {
	if strings.TrimSpace(r.RuleID) == "" {
		return fmt.Errorf("separation rule_id is required")
	}
	if !r.ActionKind.IsValid() {
		return fmt.Errorf("invalid separation action: %s", r.ActionKind)
	}
	for _, eventType := range r.LookbackEventTypes {
		if !eventType.IsValid() {
			return fmt.Errorf("invalid lookback event type: %s", eventType)
		}
	}
	switch r.LookbackScope {
	case "ticket", "run", "change", "project":
		return nil
	default:
		return fmt.Errorf("invalid lookback scope: %s", r.LookbackScope)
	}
}

type OverrideRule struct {
	RuleID                  string          `json:"rule_id" yaml:"rule_id" toml:"rule_id"`
	ActionKind              ProtectedAction `json:"action_kind" yaml:"action_kind" toml:"action_kind"`
	Allowed                 bool            `json:"allowed" yaml:"allowed" toml:"allowed"`
	RequireReason           bool            `json:"require_reason" yaml:"require_reason" toml:"require_reason"`
	RequireTrustedSignature bool            `json:"require_trusted_signature" yaml:"require_trusted_signature" toml:"require_trusted_signature"`
}

func (r OverrideRule) Validate() error {
	if strings.TrimSpace(r.RuleID) == "" {
		return fmt.Errorf("override rule_id is required")
	}
	if !r.ActionKind.IsValid() {
		return fmt.Errorf("invalid override action: %s", r.ActionKind)
	}
	return nil
}

type GovernanceExplanation struct {
	Target          string            `json:"target"`
	Action          ProtectedAction   `json:"action"`
	Actor           Actor             `json:"actor"`
	Allowed         bool              `json:"allowed"`
	MatchedPolicies []string          `json:"matched_policies,omitempty"`
	ReasonCodes     []string          `json:"reason_codes,omitempty"`
	Inputs          map[string]string `json:"inputs,omitempty"`
	GeneratedAt     time.Time         `json:"generated_at"`
	SchemaVersion   int               `json:"schema_version"`
}

type GovernanceSimulationResult struct {
	Explanation GovernanceExplanation `json:"explanation"`
	DryRun      bool                  `json:"dry_run"`
}
