package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	stateDirName        = "atlas-tasker"
	macOSAppSupportName = "Atlas Tasker"
	dirPerm             = 0o700
	filePerm            = 0o600
)

// DefaultStateDir is the machine-local private directory for the workspace
// registry, setup journal, rollback material, and setup manifest. It is never
// inside a workspace and must not be exported, synced, or backed up with
// Atlas-owned project state.
func DefaultStateDir(home string, getenv func(string) string) (string, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	if xdg := strings.TrimSpace(getenv("XDG_STATE_HOME")); xdg != "" {
		if !filepath.IsAbs(xdg) {
			return "", fmt.Errorf("XDG_STATE_HOME must be an absolute path")
		}
		return filepath.Clean(filepath.Join(xdg, stateDirName)), nil
	}
	if strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("home is required to resolve the Atlas state directory")
	}
	if !filepath.IsAbs(home) {
		return "", fmt.Errorf("home must be an absolute path")
	}
	home = filepath.Clean(home)
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", macOSAppSupportName), nil
	}
	return filepath.Join(home, ".local", "state", stateDirName), nil
}

func ensurePrivateDir(path string) error {
	if err := os.MkdirAll(path, dirPerm); err != nil {
		return fmt.Errorf("create private state directory: %w", err)
	}
	if err := os.Chmod(path, dirPerm); err != nil {
		return fmt.Errorf("chmod private state directory: %w", err)
	}
	return nil
}

func registryPath(stateDir string) string {
	return filepath.Join(stateDir, "registry.json")
}

func setupLockPath(stateDir string) string {
	return filepath.Join(stateDir, "setup.lock")
}

func journalsDir(stateDir string) string {
	return filepath.Join(stateDir, "journals")
}

func rollbackDir(stateDir string, operationID string) string {
	return filepath.Join(stateDir, "rollback", operationID)
}

func manifestsDir(stateDir string) string {
	return filepath.Join(stateDir, "manifests")
}

func workspaceStateDir(stateDir string, workspaceID string) string {
	return filepath.Join(stateDir, "workspaces", workspaceID)
}
