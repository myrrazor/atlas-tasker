package contracts

import (
	"fmt"
	"strings"
	"time"
)

type AuditScopeKind string

const (
	AuditScopeWorkspace AuditScopeKind = "workspace"
	AuditScopeProject   AuditScopeKind = "project"
	AuditScopeTicket    AuditScopeKind = "ticket"
	AuditScopeRun       AuditScopeKind = "run"
	AuditScopeChange    AuditScopeKind = "change"
	AuditScopeRelease   AuditScopeKind = "release"
	AuditScopeIncident  AuditScopeKind = "incident"
)

var validAuditScopeKinds = map[AuditScopeKind]struct{}{
	AuditScopeWorkspace: {}, AuditScopeProject: {}, AuditScopeTicket: {}, AuditScopeRun: {},
	AuditScopeChange: {}, AuditScopeRelease: {}, AuditScopeIncident: {},
}

func (k AuditScopeKind) IsValid() bool {
	_, ok := validAuditScopeKinds[k]
	return ok
}

type AuditFindingSeverity string

const (
	AuditFindingInfo     AuditFindingSeverity = "info"
	AuditFindingWarning  AuditFindingSeverity = "warning"
	AuditFindingCritical AuditFindingSeverity = "critical"
)

var validAuditFindingSeverities = map[AuditFindingSeverity]struct{}{
	AuditFindingInfo: {}, AuditFindingWarning: {}, AuditFindingCritical: {},
}

func (s AuditFindingSeverity) IsValid() bool {
	_, ok := validAuditFindingSeverities[s]
	return ok
}

type AuditReport struct {
	AuditReportID          string              `json:"audit_report_id" yaml:"audit_report_id"`
	ScopeKind              AuditScopeKind      `json:"scope_kind" yaml:"scope_kind"`
	ScopeID                string              `json:"scope_id,omitempty" yaml:"scope_id,omitempty"`
	GeneratedAt            time.Time           `json:"generated_at" yaml:"generated_at"`
	GeneratedBy            Actor               `json:"generated_by" yaml:"generated_by"`
	EventRange             EventRange          `json:"event_range" yaml:"event_range"`
	PolicySnapshotHash     string              `json:"policy_snapshot_hash" yaml:"policy_snapshot_hash"`
	TrustSnapshotHash      string              `json:"trust_snapshot_hash" yaml:"trust_snapshot_hash"`
	RedactionPreviewID     string              `json:"redaction_preview_id,omitempty" yaml:"redaction_preview_id,omitempty"`
	IncludedArtifactHashes []ArtifactHash      `json:"included_artifact_hashes,omitempty" yaml:"included_artifact_hashes,omitempty"`
	Findings               []AuditFinding      `json:"findings,omitempty" yaml:"findings,omitempty"`
	SignatureEnvelopes     []SignatureEnvelope `json:"signature_envelopes,omitempty" yaml:"signature_envelopes,omitempty" atlasc14n:"-"`
	SchemaVersion          int                 `json:"schema_version" yaml:"schema_version"`
}

func (r AuditReport) Validate() error {
	if strings.TrimSpace(r.AuditReportID) == "" {
		return fmt.Errorf("audit_report_id is required")
	}
	if !r.ScopeKind.IsValid() {
		return fmt.Errorf("valid audit scope kind is required")
	}
	if r.ScopeKind == AuditScopeWorkspace && strings.TrimSpace(r.ScopeID) != "" {
		return fmt.Errorf("workspace audit scope must omit scope_id")
	}
	if r.ScopeKind != AuditScopeWorkspace && strings.TrimSpace(r.ScopeID) == "" {
		return fmt.Errorf("audit scope id is required for %s scope", r.ScopeKind)
	}
	if !r.GeneratedBy.IsValid() {
		return fmt.Errorf("invalid generated_by actor: %s", r.GeneratedBy)
	}
	if r.GeneratedAt.IsZero() {
		return fmt.Errorf("generated_at is required")
	}
	if strings.TrimSpace(r.PolicySnapshotHash) == "" || strings.TrimSpace(r.TrustSnapshotHash) == "" {
		return fmt.Errorf("policy and trust snapshot hashes are required")
	}
	if err := r.EventRange.Validate(); err != nil {
		return err
	}
	for _, artifact := range r.IncludedArtifactHashes {
		if err := artifact.Validate(); err != nil {
			return err
		}
	}
	for _, finding := range r.Findings {
		if err := finding.Validate(); err != nil {
			return err
		}
	}
	for _, sig := range r.SignatureEnvelopes {
		if err := validateSignatureEnvelopeForArtifact(sig, ArtifactKindAuditReport, r.AuditReportID); err != nil {
			return err
		}
	}
	return nil
}

type EventRange struct {
	FromEventID int64     `json:"from_event_id" yaml:"from_event_id"`
	ToEventID   int64     `json:"to_event_id" yaml:"to_event_id"`
	FromTime    time.Time `json:"from_time,omitempty" yaml:"from_time,omitempty"`
	ToTime      time.Time `json:"to_time,omitempty" yaml:"to_time,omitempty"`
}

func (r EventRange) Validate() error {
	if r.FromEventID < 0 || r.ToEventID < 0 {
		return fmt.Errorf("event range ids cannot be negative")
	}
	if r.ToEventID != 0 && r.FromEventID > r.ToEventID {
		return fmt.Errorf("from_event_id cannot be greater than to_event_id")
	}
	if !r.FromTime.IsZero() && !r.ToTime.IsZero() && r.FromTime.After(r.ToTime) {
		return fmt.Errorf("event range from_time cannot be after to_time")
	}
	return nil
}

type ArtifactHash struct {
	Kind   ArtifactKind `json:"kind" yaml:"kind"`
	UID    string       `json:"uid" yaml:"uid"`
	SHA256 string       `json:"sha256" yaml:"sha256"`
}

func (h ArtifactHash) Validate() error {
	if !h.Kind.IsValid() {
		return fmt.Errorf("invalid artifact kind: %s", h.Kind)
	}
	if strings.TrimSpace(h.UID) == "" || strings.TrimSpace(h.SHA256) == "" {
		return fmt.Errorf("artifact uid and sha256 are required")
	}
	return nil
}

type AuditFinding struct {
	FindingID   string               `json:"finding_id" yaml:"finding_id"`
	Severity    AuditFindingSeverity `json:"severity" yaml:"severity"`
	Code        string               `json:"code" yaml:"code"`
	Message     string               `json:"message" yaml:"message"`
	ArtifactUID string               `json:"artifact_uid,omitempty" yaml:"artifact_uid,omitempty"`
}

func (f AuditFinding) Validate() error {
	if strings.TrimSpace(f.FindingID) == "" || strings.TrimSpace(f.Code) == "" || strings.TrimSpace(f.Message) == "" {
		return fmt.Errorf("finding id, code, and message are required")
	}
	if !f.Severity.IsValid() {
		return fmt.Errorf("invalid audit finding severity: %s", f.Severity)
	}
	return nil
}

type AuditPacket struct {
	PacketID           string                  `json:"packet_id" yaml:"packet_id"`
	Report             AuditReport             `json:"report" yaml:"report"`
	Canonicalization   CanonicalizationVersion `json:"canonicalization" yaml:"canonicalization"`
	PacketHash         string                  `json:"packet_hash" yaml:"packet_hash"`
	SignatureEnvelopes []SignatureEnvelope     `json:"signature_envelopes,omitempty" yaml:"signature_envelopes,omitempty" atlasc14n:"-"`
	SchemaVersion      int                     `json:"schema_version" yaml:"schema_version"`
}

func (p AuditPacket) Validate() error {
	if strings.TrimSpace(p.PacketID) == "" || strings.TrimSpace(p.PacketHash) == "" {
		return fmt.Errorf("packet_id and packet_hash are required")
	}
	if !p.Canonicalization.IsValid() {
		return fmt.Errorf("invalid canonicalization: %s", p.Canonicalization)
	}
	if err := p.Report.Validate(); err != nil {
		return err
	}
	for _, sig := range p.SignatureEnvelopes {
		if err := validateSignatureEnvelopeForArtifact(sig, ArtifactKindAuditPacket, p.PacketID); err != nil {
			return err
		}
	}
	return nil
}
