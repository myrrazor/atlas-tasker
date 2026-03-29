package service

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

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
