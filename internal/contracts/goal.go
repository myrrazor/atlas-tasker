package contracts

import (
	"fmt"
	"strings"
	"time"
)

type GoalTargetKind string

const (
	GoalTargetTicket GoalTargetKind = "ticket"
	GoalTargetRun    GoalTargetKind = "run"
)

func (k GoalTargetKind) IsValid() bool {
	return k == GoalTargetTicket || k == GoalTargetRun
}

var GoalManifestSectionOrder = []string{
	"Goal",
	"Objective",
	"Current ticket/run",
	"Acceptance criteria",
	"Constraints",
	"Required gates",
	"Evidence needed",
	"Allowed actions",
	"Do not do",
	"Current blockers",
	"Context links",
	"Done when",
}

type GoalBrief struct {
	TargetKind    GoalTargetKind `json:"target_kind" yaml:"target_kind"`
	TargetID      string         `json:"target_id" yaml:"target_id"`
	Objective     string         `json:"objective" yaml:"objective"`
	Sections      []GoalSection  `json:"sections,omitempty" yaml:"sections,omitempty"`
	GeneratedAt   time.Time      `json:"generated_at" yaml:"generated_at"`
	SchemaVersion int            `json:"schema_version" yaml:"schema_version"`
}

func (b GoalBrief) Validate() error {
	if !b.TargetKind.IsValid() || strings.TrimSpace(b.TargetID) == "" {
		return fmt.Errorf("valid goal target kind and id are required")
	}
	if strings.TrimSpace(b.Objective) == "" {
		return fmt.Errorf("objective is required")
	}
	if b.GeneratedAt.IsZero() {
		return fmt.Errorf("generated_at is required")
	}
	for _, section := range b.Sections {
		if err := section.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type GoalManifest struct {
	ManifestID         string              `json:"manifest_id" yaml:"manifest_id"`
	TargetKind         GoalTargetKind      `json:"target_kind" yaml:"target_kind"`
	TargetID           string              `json:"target_id" yaml:"target_id"`
	Objective          string              `json:"objective" yaml:"objective"`
	Sections           []GoalSection       `json:"sections" yaml:"sections"`
	PolicySnapshotHash string              `json:"policy_snapshot_hash,omitempty" yaml:"policy_snapshot_hash,omitempty"`
	TrustSnapshotHash  string              `json:"trust_snapshot_hash,omitempty" yaml:"trust_snapshot_hash,omitempty"`
	SourceHash         string              `json:"source_hash" yaml:"source_hash"`
	RedactionPreviewID string              `json:"redaction_preview_id,omitempty" yaml:"redaction_preview_id,omitempty"`
	SignatureEnvelopes []SignatureEnvelope `json:"signature_envelopes,omitempty" yaml:"signature_envelopes,omitempty" atlasc14n:"-"`
	GeneratedAt        time.Time           `json:"generated_at" yaml:"generated_at"`
	GeneratedBy        Actor               `json:"generated_by" yaml:"generated_by"`
	Reason             string              `json:"reason" yaml:"reason"`
	SchemaVersion      int                 `json:"schema_version" yaml:"schema_version"`
}

func (m GoalManifest) Validate() error {
	if strings.TrimSpace(m.ManifestID) == "" {
		return fmt.Errorf("manifest_id is required")
	}
	if !m.TargetKind.IsValid() || strings.TrimSpace(m.TargetID) == "" {
		return fmt.Errorf("valid goal target kind and id are required")
	}
	if strings.TrimSpace(m.Objective) == "" {
		return fmt.Errorf("objective is required")
	}
	if strings.TrimSpace(m.SourceHash) == "" {
		return fmt.Errorf("source_hash is required")
	}
	if m.GeneratedAt.IsZero() {
		return fmt.Errorf("generated_at is required")
	}
	if !m.GeneratedBy.IsValid() {
		return fmt.Errorf("invalid generated_by actor: %s", m.GeneratedBy)
	}
	if strings.TrimSpace(m.Reason) == "" {
		return fmt.Errorf("reason is required")
	}
	if len(m.Sections) != len(GoalManifestSectionOrder) {
		return fmt.Errorf("goal manifest must include %d sections", len(GoalManifestSectionOrder))
	}
	for _, section := range m.Sections {
		if err := section.Validate(); err != nil {
			return err
		}
	}
	for i, heading := range GoalManifestSectionOrder {
		if m.Sections[i].Heading != heading {
			return fmt.Errorf("goal manifest section %d must be %q", i+1, heading)
		}
	}
	for _, sig := range m.SignatureEnvelopes {
		if err := validateSignatureEnvelopeForArtifact(sig, ArtifactKindGoalManifest, m.ManifestID); err != nil {
			return err
		}
	}
	return nil
}

type GoalSection struct {
	Heading string   `json:"heading" yaml:"heading"`
	Items   []string `json:"items,omitempty" yaml:"items,omitempty"`
	Body    string   `json:"body,omitempty" yaml:"body,omitempty"`
}

func (s GoalSection) Validate() error {
	if strings.TrimSpace(s.Heading) == "" {
		return fmt.Errorf("goal section heading is required")
	}
	if len(s.Items) == 0 && strings.TrimSpace(s.Body) == "" {
		return fmt.Errorf("goal section %q must include body or items", s.Heading)
	}
	return nil
}
