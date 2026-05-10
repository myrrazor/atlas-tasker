package service

import (
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type MigrationState string

const (
	MigrationStateUnstamped MigrationState = "unstamped"
	MigrationStateStamped   MigrationState = "stamped"
	MigrationStateDivergent MigrationState = "divergent"
	MigrationStateUnknown   MigrationState = "unknown"
)

type MigrationDiagnosticView struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	NextStep string `json:"next_step"`
}

type MigrationEntityStatusView struct {
	Kind      string         `json:"kind"`
	State     MigrationState `json:"state"`
	Total     int            `json:"total"`
	Stamped   int            `json:"stamped"`
	Unstamped int            `json:"unstamped,omitempty"`
	Divergent int            `json:"divergent,omitempty"`
	Unknown   int            `json:"unknown,omitempty"`
	Examples  []string       `json:"examples,omitempty"`
}

type MigrationStatusView struct {
	WorkspaceID   string                      `json:"workspace_id,omitempty"`
	State         MigrationState              `json:"state"`
	Ready         bool                        `json:"ready"`
	ReasonCodes   []string                    `json:"reason_codes,omitempty"`
	Diagnostics   []MigrationDiagnosticView   `json:"diagnostics,omitempty"`
	Entities      []MigrationEntityStatusView `json:"entities,omitempty"`
	StampedAt     time.Time                   `json:"stamped_at,omitempty"`
	SchemaVersion int                         `json:"schema_version,omitempty"`
	CheckedAt     time.Time                   `json:"checked_at"`
}

type SyncPublication struct {
	WorkspaceID        string                        `json:"workspace_id"`
	BundleID           string                        `json:"bundle_id"`
	Format             string                        `json:"format"`
	CreatedAt          time.Time                     `json:"created_at"`
	ArtifactName       string                        `json:"artifact_name"`
	ManifestName       string                        `json:"manifest_name"`
	ChecksumName       string                        `json:"checksum_name"`
	FileCount          int                           `json:"file_count"`
	ArchiveSHA256      string                        `json:"archive_sha256,omitempty"`
	ManifestSHA256     string                        `json:"manifest_sha256,omitempty"`
	RedactionPreviewID string                        `json:"redaction_preview_id,omitempty"`
	SourceRemoteID     string                        `json:"source_remote_id,omitempty"`
	SourceRef          string                        `json:"source_ref,omitempty"`
	FetchedAt          time.Time                     `json:"fetched_at,omitempty"`
	SignatureEnvelopes []contracts.SignatureEnvelope `json:"signature_envelopes,omitempty" atlasc14n:"-"`
}

type SyncRemoteDetailView struct {
	Remote       contracts.SyncRemote `json:"remote"`
	GeneratedAt  time.Time            `json:"generated_at"`
	Publications []SyncPublication    `json:"publications,omitempty"`
}

type SyncStatusRemoteView struct {
	Remote       contracts.SyncRemote `json:"remote"`
	Publications []SyncPublication    `json:"publications,omitempty"`
}

type SyncStatusView struct {
	WorkspaceID       string                 `json:"workspace_id"`
	MigrationComplete bool                   `json:"migration_complete"`
	Migration         MigrationStatusView    `json:"migration"`
	ReasonCodes       []string               `json:"reason_codes,omitempty"`
	Remotes           []SyncStatusRemoteView `json:"remotes,omitempty"`
	GeneratedAt       time.Time              `json:"generated_at"`
}

type SyncJobDetailView struct {
	Job         contracts.SyncJob    `json:"job"`
	Remote      contracts.SyncRemote `json:"remote,omitempty"`
	Publication SyncPublication      `json:"publication,omitempty"`
	GeneratedAt time.Time            `json:"generated_at"`
}

type SyncBundleVerifyView struct {
	BundleRef   string          `json:"bundle_ref"`
	Verified    bool            `json:"verified"`
	Warnings    []string        `json:"warnings,omitempty"`
	Errors      []string        `json:"errors,omitempty"`
	Publication SyncPublication `json:"publication,omitempty"`
	GeneratedAt time.Time       `json:"generated_at"`
}

type ConflictDetailView struct {
	Conflict    contracts.ConflictRecord `json:"conflict"`
	GeneratedAt time.Time                `json:"generated_at"`
}
