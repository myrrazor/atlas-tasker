package contracts

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"
)

type BackupScopeKind string

const (
	BackupScopeWorkspace BackupScopeKind = "workspace"
	BackupScopeProject   BackupScopeKind = "project"
)

func (k BackupScopeKind) IsValid() bool {
	return k == BackupScopeWorkspace || k == BackupScopeProject
}

type BackupSection string

const (
	BackupSectionProjects       BackupSection = "projects"
	BackupSectionEvents         BackupSection = "events"
	BackupSectionRuns           BackupSection = "runs"
	BackupSectionGates          BackupSection = "gates"
	BackupSectionEvidence       BackupSection = "evidence"
	BackupSectionCollaboration  BackupSection = "collaboration"
	BackupSectionPublicSecurity BackupSection = "public_security"
	BackupSectionGovernance     BackupSection = "governance"
	BackupSectionClassification BackupSection = "classification"
	BackupSectionAudit          BackupSection = "audit"
)

var validBackupSections = map[BackupSection]struct{}{
	BackupSectionProjects: {}, BackupSectionEvents: {}, BackupSectionRuns: {}, BackupSectionGates: {},
	BackupSectionEvidence: {}, BackupSectionCollaboration: {}, BackupSectionPublicSecurity: {},
	BackupSectionGovernance: {}, BackupSectionClassification: {}, BackupSectionAudit: {},
}

func (s BackupSection) IsValid() bool {
	_, ok := validBackupSections[s]
	return ok
}

type BackupSnapshot struct {
	BackupID           string              `json:"backup_id" yaml:"backup_id"`
	CreatedAt          time.Time           `json:"created_at" yaml:"created_at"`
	CreatedBy          Actor               `json:"created_by" yaml:"created_by"`
	ScopeKind          BackupScopeKind     `json:"scope_kind" yaml:"scope_kind"`
	ScopeID            string              `json:"scope_id,omitempty" yaml:"scope_id,omitempty"`
	ManifestHash       string              `json:"manifest_hash" yaml:"manifest_hash"`
	IncludedSections   []BackupSection     `json:"included_sections,omitempty" yaml:"included_sections,omitempty"`
	RedactionPreviewID string              `json:"redaction_preview_id,omitempty" yaml:"redaction_preview_id,omitempty"`
	SignatureEnvelopes []SignatureEnvelope `json:"signature_envelopes,omitempty" yaml:"signature_envelopes,omitempty" atlasc14n:"-"`
	SchemaVersion      int                 `json:"schema_version" yaml:"schema_version"`
}

func (s BackupSnapshot) Validate() error {
	if strings.TrimSpace(s.BackupID) == "" || strings.TrimSpace(s.ManifestHash) == "" {
		return fmt.Errorf("backup_id and manifest_hash are required")
	}
	if !s.CreatedBy.IsValid() {
		return fmt.Errorf("invalid created_by actor: %s", s.CreatedBy)
	}
	if s.CreatedAt.IsZero() {
		return fmt.Errorf("created_at is required")
	}
	if !s.ScopeKind.IsValid() {
		return fmt.Errorf("invalid backup scope kind: %s", s.ScopeKind)
	}
	if s.ScopeKind == BackupScopeWorkspace && strings.TrimSpace(s.ScopeID) != "" {
		return fmt.Errorf("workspace backups must omit scope_id")
	}
	if s.ScopeKind == BackupScopeProject && strings.TrimSpace(s.ScopeID) == "" {
		return fmt.Errorf("project backups require scope_id")
	}
	for _, section := range s.IncludedSections {
		if !section.IsValid() {
			return fmt.Errorf("invalid backup section: %s", section)
		}
	}
	for _, sig := range s.SignatureEnvelopes {
		if err := validateSignatureEnvelopeForArtifact(sig, ArtifactKindBackupSnapshot, s.BackupID); err != nil {
			return err
		}
	}
	return nil
}

type RestorePlan struct {
	RestorePlanID  string            `json:"restore_plan_id" yaml:"restore_plan_id"`
	BackupID       string            `json:"backup_id" yaml:"backup_id"`
	GeneratedAt    time.Time         `json:"generated_at" yaml:"generated_at"`
	GeneratedBy    Actor             `json:"generated_by" yaml:"generated_by"`
	TargetRootHash string            `json:"target_root_hash,omitempty" yaml:"target_root_hash,omitempty"`
	Items          []RestorePlanItem `json:"items,omitempty" yaml:"items,omitempty"`
	Warnings       []string          `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	SchemaVersion  int               `json:"schema_version" yaml:"schema_version"`
}

func (p RestorePlan) Validate() error {
	if strings.TrimSpace(p.RestorePlanID) == "" || strings.TrimSpace(p.BackupID) == "" {
		return fmt.Errorf("restore_plan_id and backup_id are required")
	}
	if !p.GeneratedBy.IsValid() {
		return fmt.Errorf("invalid generated_by actor: %s", p.GeneratedBy)
	}
	if p.GeneratedAt.IsZero() {
		return fmt.Errorf("generated_at is required")
	}
	for _, item := range p.Items {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type RestorePlanAction string

const (
	RestorePlanCreate RestorePlanAction = "create"
	RestorePlanUpdate RestorePlanAction = "update"
	RestorePlanSkip   RestorePlanAction = "skip"
	RestorePlanBlock  RestorePlanAction = "block"
)

func (a RestorePlanAction) IsValid() bool {
	switch a {
	case RestorePlanCreate, RestorePlanUpdate, RestorePlanSkip, RestorePlanBlock:
		return true
	default:
		return false
	}
}

type RestorePlanItem struct {
	Path        string            `json:"path" yaml:"path"`
	Action      RestorePlanAction `json:"action" yaml:"action"`
	ReasonCodes []string          `json:"reason_codes,omitempty" yaml:"reason_codes,omitempty"`
}

func (i RestorePlanItem) Validate() error {
	if strings.TrimSpace(i.Path) == "" {
		return fmt.Errorf("restore item path is required")
	}
	if !isSafeRestorePlanPath(i.Path) {
		return fmt.Errorf("restore item path must be a clean relative Atlas data path")
	}
	if !i.Action.IsValid() {
		return fmt.Errorf("invalid restore action: %s", i.Action)
	}
	if i.Action == RestorePlanBlock && len(i.ReasonCodes) == 0 {
		return fmt.Errorf("blocked restore items require reason_codes")
	}
	return nil
}

func isSafeRestorePlanPath(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if raw != trimmed || trimmed == "" || strings.Contains(trimmed, "\x00") {
		return false
	}
	if filepath.IsAbs(trimmed) || strings.HasPrefix(filepath.ToSlash(trimmed), "/") {
		return false
	}
	slashed := filepath.ToSlash(trimmed)
	clean := path.Clean(slashed)
	if clean != slashed || clean == "." || clean == ".." {
		return false
	}
	if strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return false
	}
	return isCanonicalRestorePlanPath(clean)
}

func isCanonicalRestorePlanPath(rel string) bool {
	switch {
	case strings.HasPrefix(rel, "projects/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/collaborators/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/memberships/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/mentions/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/runs/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/gates/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/handoffs/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/evidence/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/changes/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/checks/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/permission-profiles/") && strings.HasSuffix(rel, ".toml"):
		return true
	case strings.HasPrefix(rel, ".tracker/retention/") && strings.HasSuffix(rel, ".toml"):
		return true
	case strings.HasPrefix(rel, ".tracker/archives/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/security/keys/public/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/security/revocations/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/security/signatures/") && strings.HasSuffix(rel, ".json"):
		return true
	case strings.HasPrefix(rel, ".tracker/governance/policies/") && strings.HasSuffix(rel, ".toml"):
		return true
	case strings.HasPrefix(rel, ".tracker/governance/packs/") && strings.HasSuffix(rel, ".toml"):
		return true
	case strings.HasPrefix(rel, ".tracker/classification/labels/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/classification/policies/") && strings.HasSuffix(rel, ".toml"):
		return true
	case strings.HasPrefix(rel, ".tracker/redaction/rules/") && strings.HasSuffix(rel, ".toml"):
		return true
	case strings.HasPrefix(rel, ".tracker/audit/reports/") && strings.HasSuffix(rel, ".json"):
		return true
	case strings.HasPrefix(rel, ".tracker/audit/packets/") && strings.HasSuffix(rel, ".json"):
		return true
	case strings.HasPrefix(rel, ".tracker/events/") && strings.HasSuffix(rel, ".jsonl"):
		return true
	default:
		return false
	}
}
