package service

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

var backupWorkspaceIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func validBackupWorkspaceID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" || strings.Contains(id, "..") || strings.ContainsAny(id, `/\`) {
		return false
	}
	return backupWorkspaceIDRe.MatchString(id)
}

type workspaceMetadata struct {
	WorkspaceID string    `json:"workspace_id"`
	CreatedAt   time.Time `json:"created_at"`
}

func ensureWorkspaceIdentity(root string) (string, error) {
	if err := os.MkdirAll(storage.TrackerDir(root), 0o755); err != nil {
		return "", fmt.Errorf("create tracker dir: %w", err)
	}
	for _, dir := range []string{
		storage.SyncDir(root),
		storage.SyncRemotesDir(root),
		storage.SyncJobsDir(root),
		storage.SyncConflictsDir(root),
		storage.SyncBundlesDir(root),
		storage.SyncMirrorDir(root),
		storage.SyncStagingDir(root),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("create sync scaffold: %w", err)
		}
	}
	path := storage.WorkspaceMetadataFile(root)
	raw, err := os.ReadFile(path)
	if err == nil {
		var meta workspaceMetadata
		if err := json.Unmarshal(raw, &meta); err != nil {
			return "", fmt.Errorf("decode workspace metadata: %w", err)
		}
		if strings.TrimSpace(meta.WorkspaceID) == "" {
			return "", fmt.Errorf("workspace metadata missing workspace_id")
		}
		if !validBackupWorkspaceID(meta.WorkspaceID) {
			return "", fmt.Errorf("workspace metadata workspace_id is not a portable identity")
		}
		return meta.WorkspaceID, nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("read workspace metadata: %w", err)
	}
	meta := workspaceMetadata{WorkspaceID: uuid.NewString(), CreatedAt: time.Now().UTC()}
	encoded, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode workspace metadata: %w", err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		return "", fmt.Errorf("write workspace metadata: %w", err)
	}
	return meta.WorkspaceID, nil
}

func EnsureWorkspaceIdentityForCLI(root string) (string, error) {
	return ensureWorkspaceIdentity(root)
}

// LoadWorkspaceIdentity returns the stamped workspace ID without creating one.
// An absent metadata file yields ("", nil).
func LoadWorkspaceIdentity(root string) (string, error) {
	return loadWorkspaceIdentity(root)
}

func loadWorkspaceIdentity(root string) (string, error) {
	raw, err := os.ReadFile(storage.WorkspaceMetadataFile(root))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read workspace metadata: %w", err)
	}
	var meta workspaceMetadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		return "", fmt.Errorf("decode workspace metadata: %w", err)
	}
	id := strings.TrimSpace(meta.WorkspaceID)
	if id != "" && !validBackupWorkspaceID(id) {
		return "", fmt.Errorf("workspace metadata workspace_id is not a portable identity")
	}
	return id, nil
}
