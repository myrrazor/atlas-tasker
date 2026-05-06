package contracts

import (
	"fmt"
	"strings"
	"time"
)

type ClassificationLevel string

const (
	ClassificationPublic       ClassificationLevel = "public"
	ClassificationInternal     ClassificationLevel = "internal"
	ClassificationConfidential ClassificationLevel = "confidential"
	ClassificationRestricted   ClassificationLevel = "restricted"
)

var classificationRank = map[ClassificationLevel]int{
	ClassificationPublic: 0, ClassificationInternal: 1, ClassificationConfidential: 2, ClassificationRestricted: 3,
}

func (l ClassificationLevel) IsValid() bool {
	_, ok := classificationRank[l]
	return ok
}

func HigherClassification(a ClassificationLevel, b ClassificationLevel) ClassificationLevel {
	if !a.IsValid() || !b.IsValid() {
		return ClassificationRestricted
	}
	if classificationRank[a] >= classificationRank[b] {
		return a
	}
	return b
}

type ClassifiedEntityKind string

const (
	ClassifiedEntityWorkspace ClassifiedEntityKind = "workspace"
	ClassifiedEntityProject   ClassifiedEntityKind = "project"
	ClassifiedEntityTicket    ClassifiedEntityKind = "ticket"
	ClassifiedEntityRun       ClassifiedEntityKind = "run"
	ClassifiedEntityEvidence  ClassifiedEntityKind = "evidence"
	ClassifiedEntityHandoff   ClassifiedEntityKind = "handoff"
	ClassifiedEntityAudit     ClassifiedEntityKind = "audit"
	ClassifiedEntityBackup    ClassifiedEntityKind = "backup"
)

var validClassifiedEntityKinds = map[ClassifiedEntityKind]struct{}{
	ClassifiedEntityWorkspace: {}, ClassifiedEntityProject: {}, ClassifiedEntityTicket: {}, ClassifiedEntityRun: {},
	ClassifiedEntityEvidence: {}, ClassifiedEntityHandoff: {}, ClassifiedEntityAudit: {}, ClassifiedEntityBackup: {},
}

func (k ClassifiedEntityKind) IsValid() bool {
	_, ok := validClassifiedEntityKinds[k]
	return ok
}

type ClassificationLabel struct {
	ClassificationID string               `json:"classification_id" yaml:"classification_id" toml:"classification_id"`
	EntityKind       ClassifiedEntityKind `json:"entity_kind" yaml:"entity_kind" toml:"entity_kind"`
	EntityID         string               `json:"entity_id" yaml:"entity_id" toml:"entity_id"`
	Level            ClassificationLevel  `json:"level" yaml:"level" toml:"level"`
	AppliedBy        Actor                `json:"applied_by" yaml:"applied_by" toml:"applied_by"`
	Reason           string               `json:"reason" yaml:"reason" toml:"reason"`
	CreatedAt        time.Time            `json:"created_at" yaml:"created_at" toml:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at" yaml:"updated_at" toml:"updated_at"`
	SchemaVersion    int                  `json:"schema_version" yaml:"schema_version" toml:"schema_version"`
}

func (l ClassificationLabel) Validate() error {
	if strings.TrimSpace(l.ClassificationID) == "" {
		return fmt.Errorf("classification_id is required")
	}
	if !l.EntityKind.IsValid() || strings.TrimSpace(l.EntityID) == "" {
		return fmt.Errorf("valid entity kind and id are required")
	}
	if !l.Level.IsValid() {
		return fmt.Errorf("invalid classification level: %s", l.Level)
	}
	if !l.AppliedBy.IsValid() {
		return fmt.Errorf("invalid applied_by actor: %s", l.AppliedBy)
	}
	if strings.TrimSpace(l.Reason) == "" {
		return fmt.Errorf("reason is required")
	}
	return nil
}

type RedactionAction string

const (
	RedactionInclude           RedactionAction = "include"
	RedactionOmit              RedactionAction = "omit"
	RedactionMask              RedactionAction = "mask"
	RedactionHash              RedactionAction = "hash"
	RedactionReplaceWithMarker RedactionAction = "replace_with_marker"
)

var validRedactionActions = map[RedactionAction]struct{}{
	RedactionInclude: {}, RedactionOmit: {}, RedactionMask: {}, RedactionHash: {}, RedactionReplaceWithMarker: {},
}

func (a RedactionAction) IsValid() bool {
	_, ok := validRedactionActions[a]
	return ok
}

type RedactionTarget string

const (
	RedactionTargetExport RedactionTarget = "export"
	RedactionTargetSync   RedactionTarget = "sync"
	RedactionTargetAudit  RedactionTarget = "audit"
	RedactionTargetBackup RedactionTarget = "backup"
	RedactionTargetGoal   RedactionTarget = "goal"
)

var validRedactionTargets = map[RedactionTarget]struct{}{
	RedactionTargetExport: {}, RedactionTargetSync: {}, RedactionTargetAudit: {}, RedactionTargetBackup: {}, RedactionTargetGoal: {},
}

func (t RedactionTarget) IsValid() bool {
	_, ok := validRedactionTargets[t]
	return ok
}

type RedactionRule struct {
	RuleID        string               `json:"rule_id" yaml:"rule_id" toml:"rule_id"`
	Target        RedactionTarget      `json:"target" yaml:"target" toml:"target"`
	EntityKind    ClassifiedEntityKind `json:"entity_kind,omitempty" yaml:"entity_kind,omitempty" toml:"entity_kind,omitempty"`
	FieldPath     string               `json:"field_path" yaml:"field_path" toml:"field_path"`
	MinLevel      ClassificationLevel  `json:"min_level" yaml:"min_level" toml:"min_level"`
	Action        RedactionAction      `json:"action" yaml:"action" toml:"action"`
	Marker        string               `json:"marker,omitempty" yaml:"marker,omitempty" toml:"marker,omitempty"`
	Reason        string               `json:"reason" yaml:"reason" toml:"reason"`
	SchemaVersion int                  `json:"schema_version" yaml:"schema_version" toml:"schema_version"`
}

func (r RedactionRule) Validate() error {
	if strings.TrimSpace(r.RuleID) == "" || strings.TrimSpace(r.FieldPath) == "" {
		return fmt.Errorf("rule_id and field_path are required")
	}
	if !r.Target.IsValid() {
		return fmt.Errorf("invalid redaction target: %s", r.Target)
	}
	if r.EntityKind != "" && !r.EntityKind.IsValid() {
		return fmt.Errorf("invalid classified entity kind: %s", r.EntityKind)
	}
	if !r.MinLevel.IsValid() {
		return fmt.Errorf("invalid classification level: %s", r.MinLevel)
	}
	if !r.Action.IsValid() {
		return fmt.Errorf("invalid redaction action: %s", r.Action)
	}
	if r.Action == RedactionReplaceWithMarker && strings.TrimSpace(r.Marker) == "" {
		return fmt.Errorf("replace_with_marker requires marker")
	}
	if strings.TrimSpace(r.Reason) == "" {
		return fmt.Errorf("reason is required")
	}
	return nil
}

type RedactionPreview struct {
	PreviewID          string            `json:"preview_id" yaml:"preview_id" toml:"preview_id"`
	Scope              string            `json:"scope" yaml:"scope" toml:"scope"`
	Target             RedactionTarget   `json:"target" yaml:"target" toml:"target"`
	Actor              Actor             `json:"actor" yaml:"actor" toml:"actor"`
	PolicyVersionHash  string            `json:"policy_version_hash" yaml:"policy_version_hash" toml:"policy_version_hash"`
	ClassificationHash string            `json:"classification_hash" yaml:"classification_hash" toml:"classification_hash"`
	SourceStateHash    string            `json:"source_state_hash" yaml:"source_state_hash" toml:"source_state_hash"`
	CommandTarget      string            `json:"command_target" yaml:"command_target" toml:"command_target"`
	CreatedAt          time.Time         `json:"created_at" yaml:"created_at" toml:"created_at"`
	ExpiresAt          time.Time         `json:"expires_at" yaml:"expires_at" toml:"expires_at"`
	Items              []RedactionResult `json:"items,omitempty" yaml:"items,omitempty" toml:"items,omitempty"`
	SchemaVersion      int               `json:"schema_version" yaml:"schema_version" toml:"schema_version"`
}

func (p RedactionPreview) Validate() error {
	if strings.TrimSpace(p.PreviewID) == "" || strings.TrimSpace(p.Scope) == "" {
		return fmt.Errorf("preview_id and scope are required")
	}
	if !p.Target.IsValid() {
		return fmt.Errorf("invalid redaction target: %s", p.Target)
	}
	if !p.Actor.IsValid() {
		return fmt.Errorf("invalid preview actor: %s", p.Actor)
	}
	if strings.TrimSpace(p.PolicyVersionHash) == "" || strings.TrimSpace(p.ClassificationHash) == "" || strings.TrimSpace(p.SourceStateHash) == "" {
		return fmt.Errorf("policy, classification, and source hashes are required")
	}
	if strings.TrimSpace(p.CommandTarget) == "" {
		return fmt.Errorf("command_target is required")
	}
	if p.CreatedAt.IsZero() || p.ExpiresAt.IsZero() {
		return fmt.Errorf("preview created_at and expires_at are required")
	}
	if !p.ExpiresAt.After(p.CreatedAt) {
		return fmt.Errorf("preview expires_at must be after created_at")
	}
	for _, item := range p.Items {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type RedactionResult struct {
	EntityKind  ClassifiedEntityKind `json:"entity_kind" yaml:"entity_kind" toml:"entity_kind"`
	EntityID    string               `json:"entity_id" yaml:"entity_id" toml:"entity_id"`
	FieldPath   string               `json:"field_path" yaml:"field_path" toml:"field_path"`
	Level       ClassificationLevel  `json:"level" yaml:"level" toml:"level"`
	Action      RedactionAction      `json:"action" yaml:"action" toml:"action"`
	ReasonCodes []string             `json:"reason_codes,omitempty" yaml:"reason_codes,omitempty" toml:"reason_codes,omitempty"`
}

func (r RedactionResult) Validate() error {
	if !r.EntityKind.IsValid() || strings.TrimSpace(r.EntityID) == "" || strings.TrimSpace(r.FieldPath) == "" {
		return fmt.Errorf("valid entity kind, entity id, and field path are required")
	}
	if !r.Level.IsValid() {
		return fmt.Errorf("invalid classification level: %s", r.Level)
	}
	if !r.Action.IsValid() {
		return fmt.Errorf("invalid redaction action: %s", r.Action)
	}
	return nil
}
