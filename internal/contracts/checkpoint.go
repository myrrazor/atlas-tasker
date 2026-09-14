package contracts

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// CheckpointManifestFormat is the committed checkpoint document at
// .atlas-checkpoint.json (ADR AT114-005 / DEC-074).
const CheckpointManifestFormat = "atlas_git_checkpoint_v1"

// CheckpointManifestName is the tree-root path of the committed manifest.
const CheckpointManifestName = ".atlas-checkpoint.json"

// BackupOutboxState is the durable automatic-backup worker state.
type BackupOutboxState string

const (
	BackupOutboxPending           BackupOutboxState = "pending"
	BackupOutboxSnapshotting      BackupOutboxState = "snapshotting"
	BackupOutboxCheckpointCreated BackupOutboxState = "checkpoint_created"
	BackupOutboxPushPending       BackupOutboxState = "push_pending"
	BackupOutboxPushedUnverified  BackupOutboxState = "pushed_unverified"
	BackupOutboxVerified          BackupOutboxState = "verified"
	BackupOutboxRetryableFailure  BackupOutboxState = "retryable_failure"
	BackupOutboxBlocked           BackupOutboxState = "blocked"
)

var validBackupOutboxStates = map[BackupOutboxState]struct{}{
	BackupOutboxPending: {}, BackupOutboxSnapshotting: {}, BackupOutboxCheckpointCreated: {},
	BackupOutboxPushPending: {}, BackupOutboxPushedUnverified: {}, BackupOutboxVerified: {},
	BackupOutboxRetryableFailure: {}, BackupOutboxBlocked: {},
}

func (s BackupOutboxState) IsValid() bool {
	_, ok := validBackupOutboxStates[s]
	return ok
}

// CheckpointFile is one allowlisted blob in the canonical tree.
type CheckpointFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// CheckpointManifest is atlas_git_checkpoint_v1. manifest_sha256 is the SHA-256
// of the canonical encoding with that field blanked (ADR §4.1 / finding B-D).
type CheckpointManifest struct {
	Format                   string           `json:"format"`
	CheckpointID             string           `json:"checkpoint_id"`
	WorkspaceID              string           `json:"workspace_id"`
	ReplicaID                string           `json:"replica_id"`
	AtlasVersion             string           `json:"atlas_version"`
	CreatedAt                time.Time        `json:"created_at"`
	CanonicalTreeSHA256      string           `json:"canonical_tree_sha256"`
	ManifestSHA256           string           `json:"manifest_sha256"`
	PreviousCheckpointCommit string           `json:"previous_checkpoint_commit,omitempty"`
	EventWatermarks          map[string]int64 `json:"event_watermarks"`
	FileCount                int              `json:"file_count"`
	SchemaVersion            int              `json:"schema_version"`
	Files                    []CheckpointFile `json:"files"`
}

// CanonicalTreeHash is SHA-256 over sorted `path\0sha256\0size\n` lines,
// excluding the checkpoint manifest itself.
func CanonicalTreeHash(files []CheckpointFile) string {
	sorted := append([]CheckpointFile(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	var b strings.Builder
	for _, file := range sorted {
		if file.Path == CheckpointManifestName {
			continue
		}
		fmt.Fprintf(&b, "%s\x00%s\x00%d\n", file.Path, file.SHA256, file.Size)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// LogicalCheckpointID derives the idempotency key from workspace, replica,
// canonical tree, watermarks, and target (AT114-405).
func LogicalCheckpointID(workspaceID, replicaID, treeHash, target string, watermarks map[string]int64) string {
	keys := make([]string, 0, len(watermarks))
	for key := range watermarks {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(strings.TrimSpace(workspaceID))
	b.WriteByte('\n')
	b.WriteString(strings.TrimSpace(replicaID))
	b.WriteByte('\n')
	b.WriteString(strings.TrimSpace(treeHash))
	b.WriteByte('\n')
	b.WriteString(strings.TrimSpace(target))
	b.WriteByte('\n')
	for _, key := range keys {
		fmt.Fprintf(&b, "%s=%d\n", key, watermarks[key])
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// Canonicalize prepares a manifest for hashing or persist: sorted files,
// sorted watermark keys, UTC timestamp, computed tree hash, blank then filled
// manifest hash.
func (m CheckpointManifest) Canonicalize() (CheckpointManifest, error) {
	out := m
	out.Format = CheckpointManifestFormat
	out.WorkspaceID = strings.TrimSpace(out.WorkspaceID)
	out.ReplicaID = strings.TrimSpace(out.ReplicaID)
	out.AtlasVersion = strings.TrimSpace(out.AtlasVersion)
	out.CreatedAt = out.CreatedAt.UTC()
	if out.SchemaVersion <= 0 {
		out.SchemaVersion = CurrentSchemaVersion
	}
	files := append([]CheckpointFile(nil), out.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	out.Files = files
	out.FileCount = len(files)
	out.EventWatermarks = cloneWatermarks(out.EventWatermarks)
	out.CanonicalTreeSHA256 = CanonicalTreeHash(files)
	out.ManifestSHA256 = ""
	if out.CheckpointID == "" {
		out.CheckpointID = LogicalCheckpointID(out.WorkspaceID, out.ReplicaID, out.CanonicalTreeSHA256, "local", out.EventWatermarks)
	}
	if err := out.Validate(); err != nil {
		return CheckpointManifest{}, err
	}
	hash, err := ManifestHash(out)
	if err != nil {
		return CheckpointManifest{}, err
	}
	out.ManifestSHA256 = hash
	return out, nil
}

// ManifestHash is SHA-256 of the canonical JSON encoding with manifest_sha256
// set to the empty string, keys sorted, no insignificant whitespace, UTF-8,
// trailing newline.
func ManifestHash(m CheckpointManifest) (string, error) {
	clone := m
	clone.ManifestSHA256 = ""
	raw, err := EncodeCheckpointManifest(clone)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// EncodeCheckpointManifest renders the committed document.
func EncodeCheckpointManifest(m CheckpointManifest) ([]byte, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("encode checkpoint manifest: %w", err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return nil, fmt.Errorf("compact checkpoint manifest: %w", err)
	}
	return append(compact.Bytes(), '\n'), nil
}

// ParseCheckpointManifest decodes and verifies manifest_sha256.
func ParseCheckpointManifest(raw []byte) (CheckpointManifest, error) {
	var m CheckpointManifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&m); err != nil {
		return CheckpointManifest{}, fmt.Errorf("parse checkpoint manifest: %w", err)
	}
	if decoder.More() {
		return CheckpointManifest{}, fmt.Errorf("parse checkpoint manifest: trailing content")
	}
	got := strings.TrimSpace(m.ManifestSHA256)
	want, err := ManifestHash(m)
	if err != nil {
		return CheckpointManifest{}, err
	}
	if got == "" || got != want {
		return CheckpointManifest{}, fmt.Errorf("checkpoint manifest hash mismatch")
	}
	if err := m.Validate(); err != nil {
		return CheckpointManifest{}, err
	}
	if tree := CanonicalTreeHash(m.Files); tree != m.CanonicalTreeSHA256 {
		return CheckpointManifest{}, fmt.Errorf("checkpoint canonical tree hash mismatch")
	}
	return m, nil
}

// Validate checks required checkpoint fields. It does not recompute hashes.
func (m CheckpointManifest) Validate() error {
	if m.Format != CheckpointManifestFormat {
		return fmt.Errorf("checkpoint format must be %q, got %q", CheckpointManifestFormat, m.Format)
	}
	if strings.TrimSpace(m.CheckpointID) == "" {
		return fmt.Errorf("checkpoint_id is required")
	}
	if strings.TrimSpace(m.WorkspaceID) == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if strings.TrimSpace(m.ReplicaID) == "" {
		return fmt.Errorf("replica_id is required")
	}
	if m.CreatedAt.IsZero() {
		return fmt.Errorf("created_at is required")
	}
	if len(m.CanonicalTreeSHA256) != 64 {
		return fmt.Errorf("canonical_tree_sha256 is required")
	}
	if m.FileCount != len(m.Files) {
		return fmt.Errorf("file_count %d does not match files %d", m.FileCount, len(m.Files))
	}
	if m.SchemaVersion <= 0 {
		return fmt.Errorf("schema_version is required")
	}
	seen := map[string]struct{}{}
	for _, file := range m.Files {
		if strings.TrimSpace(file.Path) == "" || !isSafeRestorePlanPath(file.Path) {
			return fmt.Errorf("checkpoint file path %q is not restore-safe", file.Path)
		}
		if len(file.SHA256) != 64 {
			return fmt.Errorf("checkpoint file %s missing sha256", file.Path)
		}
		if file.Size < 0 {
			return fmt.Errorf("checkpoint file %s has negative size", file.Path)
		}
		if _, ok := seen[file.Path]; ok {
			return fmt.Errorf("duplicate checkpoint file %s", file.Path)
		}
		seen[file.Path] = struct{}{}
	}
	return nil
}

func cloneWatermarks(in map[string]int64) map[string]int64 {
	if len(in) == 0 {
		return map[string]int64{}
	}
	out := make(map[string]int64, len(in))
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out[key] = in[key]
	}
	return out
}

// TriggersAutomaticCheckpoint reports whether a successful canonical mutation
// should mark the backup outbox. Backup bookkeeping, restore-plan events,
// sync status that does not change recoverable state, and agent wake-ups do
// not. Automatic checkpoints themselves never append an event.
func TriggersAutomaticCheckpoint(t EventType) bool {
	switch t {
	case EventBackupCreated, EventBackupVerified, EventBackupRestorePlanned:
		return false
	case EventSyncStarted, EventSyncCompleted, EventSyncFailed, EventBundleVerified:
		return false
	case EventRemoteAdded, EventRemoteEdited, EventRemoteRemoved:
		return false
	case EventAgentWorkAvailable:
		return false
	default:
		return t.IsValid()
	}
}
