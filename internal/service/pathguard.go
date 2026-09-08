package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
)

// rejectUnsafeRelPath rejects absolute paths and any ".." escape relative to root.
func rejectUnsafeRelPath(root string, rel string) error {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return apperr.New(apperr.CodeInvalidInput, "path_rejected: empty relative path")
	}
	if filepath.IsAbs(rel) {
		return apperr.New(apperr.CodeInvalidInput, "path_rejected: absolute paths are not allowed")
	}
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return apperr.New(apperr.CodeInvalidInput, "path_rejected: path escapes the workspace")
	}
	joined := filepath.Join(root, clean)
	gotRel, err := filepath.Rel(root, joined)
	if err != nil {
		return apperr.New(apperr.CodeInvalidInput, "path_rejected: path escapes the workspace")
	}
	if gotRel == ".." || strings.HasPrefix(gotRel, ".."+string(filepath.Separator)) {
		return apperr.New(apperr.CodeInvalidInput, "path_rejected: path escapes the workspace")
	}
	return nil
}

// rejectSymlinkComponents walks each path component under root with Lstat and
// fails closed if any component is a symlink.
func rejectSymlinkComponents(root string, absPath string) error {
	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return apperr.New(apperr.CodeInvalidInput, "path_rejected: path escapes the workspace")
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("symlink_rejected: %s", current))
		}
	}
	return nil
}

// resolveContainedPath joins root+rel after containment checks and rejects symlink components.
func resolveContainedPath(root string, rel string) (string, error) {
	if err := rejectUnsafeRelPath(root, rel); err != nil {
		return "", err
	}
	abs := filepath.Join(root, filepath.Clean(rel))
	if err := rejectSymlinkComponents(root, abs); err != nil {
		return "", err
	}
	return abs, nil
}

// rejectSymlinkedFile rejects a path if Lstat says it is a symlink.
func rejectSymlinkedFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("symlink_rejected: %s", path))
	}
	return nil
}
