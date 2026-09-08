package service

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

// InitializedWorkspaceRoot resolves and validates a workspace without creating
// files. Every workspace reader must call this before opening the derived index,
// whose SQLite opener creates its parent directory when it is missing.
func InitializedWorkspaceRoot(root string) (string, error) {
	root, err := CanonicalWorkspaceRoot(root)
	if err != nil {
		return "", apperr.Wrap(apperr.CodeInvalidInput, err, "resolve workspace root: %v", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return "", apperr.Wrap(apperr.CodeNotFound, err, "workspace %s does not exist", root)
		}
		return "", fmt.Errorf("inspect workspace root %s: %w", root, err)
	}
	if !info.IsDir() {
		return "", apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("workspace %s is not a directory", root))
	}
	info, err = os.Stat(storage.TrackerDir(root))
	if err == nil {
		if !info.IsDir() {
			return "", apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("%s is not an Atlas workspace: .tracker is not a directory", root))
		}
		return root, nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect Atlas workspace %s: %w", root, err)
	}
	for dir := filepath.Dir(root); ; dir = filepath.Dir(dir) {
		info, err := os.Stat(storage.TrackerDir(dir))
		if err == nil && info.IsDir() {
			return "", apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("%s is not an Atlas workspace root — the workspace is %s; run tracker from there", root, dir))
		}
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect ancestor Atlas workspace %s: %w", dir, err)
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	return "", apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("%s is not an Atlas workspace; run 'tracker init' first", root))
}
