package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

// ResolvedWorkspace is the single Atlas root proven from the current
// directory. Resolution never initializes a workspace and never opens a
// different registered workspace.
type ResolvedWorkspace struct {
	Root        string
	WorkspaceID string
	Repair      string
}

// ResolveWorkspaceFromCWD walks from cwd to the nearest valid Atlas root,
// refuses nested or symlinked ambiguity, and verifies the expected workspace
// ID. The registry is consulted only to detect move/copy/replace; its stored
// path is never used as the workspace to open.
func ResolveWorkspaceFromCWD(cwd string, expectedID string, stateDir string) (ResolvedWorkspace, error) {
	expectedID = strings.TrimSpace(expectedID)
	if expectedID == "" {
		return ResolvedWorkspace{}, apperr.New(apperr.CodeInvalidInput, "--expected-workspace-id is required with --workspace-from-cwd")
	}
	if strings.TrimSpace(cwd) == "" {
		return ResolvedWorkspace{}, apperr.New(apperr.CodeInvalidInput, "current directory is required")
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return ResolvedWorkspace{}, apperr.Wrap(apperr.CodeInvalidInput, err, "canonicalize current directory: %v", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return ResolvedWorkspace{}, apperr.Wrap(apperr.CodeInvalidInput, err, "canonicalize current directory: %v", err)
	}
	abs = filepath.Clean(abs)
	resolved = filepath.Clean(resolved)

	unresolvedRoot, err := nearestAtlasRoot(abs)
	if err != nil {
		return ResolvedWorkspace{}, err
	}
	resolvedRoot, err := nearestAtlasRoot(resolved)
	if err != nil {
		return ResolvedWorkspace{}, err
	}
	if unresolvedRoot != resolvedRoot {
		return ResolvedWorkspace{}, apperr.New(apperr.CodeInvalidInput, "refusing symlink substitution between the current directory and the Atlas workspace")
	}
	root := resolvedRoot

	id, err := service.LoadWorkspaceIdentity(root)
	if err != nil {
		return ResolvedWorkspace{}, err
	}
	if strings.TrimSpace(id) == "" {
		return ResolvedWorkspace{}, apperr.New(apperr.CodeInvalidInput, "workspace identity is missing; the directory is not a complete Atlas workspace")
	}
	if id != expectedID {
		return ResolvedWorkspace{}, apperr.New(apperr.CodeInvalidInput, "wrong workspace id")
	}

	if strings.TrimSpace(stateDir) != "" {
		reg, err := loadRegistry(stateDir)
		if err != nil {
			return ResolvedWorkspace{}, err
		}
		if entry, ok := reg.lookup(expectedID); ok {
			if err := checkRegistryBinding(entry, root, id); err != nil {
				return ResolvedWorkspace{Root: root, WorkspaceID: id, Repair: err.Error()}, err
			}
		}
	}
	return ResolvedWorkspace{Root: root, WorkspaceID: id}, nil
}

func nearestAtlasRoot(start string) (string, error) {
	var found []string
	dir := filepath.Clean(start)
	for {
		marker := filepath.Join(dir, storage.TrackerDirName)
		info, err := os.Lstat(marker)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return "", apperr.New(apperr.CodeInvalidInput, "refusing a symlinked .tracker directory")
			}
			if !info.IsDir() {
				return "", apperr.New(apperr.CodeInvalidInput, ".tracker is not a directory")
			}
			found = append(found, dir)
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect %s: %w", marker, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if len(found) == 0 {
		return "", apperr.New(apperr.CodeInvalidInput, "wrong cwd: no Atlas workspace on the path from the current directory")
	}
	if len(found) > 1 {
		return "", apperr.New(apperr.CodeInvalidInput, "nested workspace")
	}
	return found[0], nil
}

// VerifyExpectedWorkspaceID checks an already-resolved workspace against the
// expected ID. It does not consult the registry and does not fall back.
func VerifyExpectedWorkspaceID(root string, expectedID string) error {
	expectedID = strings.TrimSpace(expectedID)
	if expectedID == "" {
		return nil
	}
	id, err := service.LoadWorkspaceIdentity(root)
	if err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" || id != expectedID {
		return apperr.New(apperr.CodeInvalidInput, "wrong workspace id")
	}
	return nil
}
