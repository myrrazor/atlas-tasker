package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

// BoardsRoot is the Atlas-owned directory Home may create boards in without a
// terminal grant. It is not a discovery crawl of the user's home directory.
func (a *App) BoardsRoot() string {
	return filepath.Join(a.stateDir, "boards")
}

func (a *App) AuthorizedRoots() []AuthorizedRoot {
	roots := []AuthorizedRoot{{
		Ref:   BoardsRootRef,
		Kind:  BoardsRootKind,
		Title: "Atlas boards on this computer",
		Path:  a.BoardsRoot(),
	}}
	settings := a.snapshotSettings()
	if !settings.Discovery.Enabled {
		return roots
	}
	for i, raw := range settings.Discovery.Roots {
		clean := filepath.Clean(strings.TrimSpace(raw))
		if !filepath.IsAbs(clean) {
			continue
		}
		info, err := os.Lstat(clean)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		roots = append(roots, AuthorizedRoot{
			Ref:   fmt.Sprintf("%s:%d", DiscoveryRootKind, i),
			Kind:  DiscoveryRootKind,
			Title: filepath.Base(clean),
			Path:  clean,
		})
	}
	return roots
}

func (a *App) authorizedRootByRef(ref string) (AuthorizedRoot, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return AuthorizedRoot{}, apperr.New(apperr.CodeInvalidInput, "authorized root is required")
	}
	for _, root := range a.AuthorizedRoots() {
		if root.Ref == ref {
			return root, nil
		}
	}
	return AuthorizedRoot{}, apperr.New(apperr.CodeInvalidInput, "unknown authorized root")
}

func cleanAuthorizedRel(rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	rel = strings.ReplaceAll(rel, "\\", "/")
	if rel == "" || rel == "." {
		return "", nil
	}
	if filepath.IsAbs(rel) || strings.Contains(rel, ":") {
		return "", apperr.New(apperr.CodeInvalidInput, "folder must be relative to an authorized root")
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." {
		return "", nil
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", apperr.New(apperr.CodeInvalidInput, "folder cannot leave the authorized root")
	}
	depth := len(strings.Split(clean, string(os.PathSeparator)))
	if depth > maxAuthorizedRelDepth {
		return "", apperr.New(apperr.CodeInvalidInput, "folder is nested too deeply under the authorized root")
	}
	return clean, nil
}

// ResolveAuthorizedDir maps a browser root ref + relative folder onto a real
// directory. create may mkdir only under that authorized root. Absolute paths
// are rejected here; callers still mint and consume a purpose-bound grant.
func (a *App) ResolveAuthorizedDir(rootRef, rel string, create bool) (string, error) {
	root, err := a.authorizedRootByRef(rootRef)
	if err != nil {
		return "", err
	}
	folder, err := cleanAuthorizedRel(rel)
	if err != nil {
		return "", err
	}
	if folder == "" {
		if root.Kind == BoardsRootKind {
			return "", apperr.New(apperr.CodeInvalidInput, "choose a folder name under Atlas boards")
		}
		if err := refuseSymlinkChain(root.Path, root.Path); err != nil {
			return "", err
		}
		return root.Path, nil
	}
	target := filepath.Join(root.Path, folder)
	if !containedIn(root.Path, target) {
		return "", apperr.New(apperr.CodeInvalidInput, "folder cannot leave the authorized root")
	}
	if err := refuseSymlinkChain(root.Path, target); err != nil {
		return "", err
	}
	info, err := os.Lstat(target)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
		if !create {
			return "", apperr.New(apperr.CodeInvalidInput, "directory does not exist under the authorized root")
		}
		if err := os.MkdirAll(target, 0o755); err != nil {
			return "", err
		}
		if err := refuseSymlinkChain(root.Path, target); err != nil {
			return "", err
		}
		return target, nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", apperr.New(apperr.CodeInvalidInput, "path grant refuses a symlink")
	}
	if !info.IsDir() {
		return "", apperr.New(apperr.CodeInvalidInput, "path grant requires a directory")
	}
	return target, nil
}

func (a *App) ListAuthorizedChildren(rootRef, rel string) ([]AuthorizedChild, error) {
	root, err := a.authorizedRootByRef(rootRef)
	if err != nil {
		return nil, err
	}
	folder, err := cleanAuthorizedRel(rel)
	if err != nil {
		return nil, err
	}
	base := root.Path
	if folder != "" {
		base = filepath.Join(root.Path, folder)
		if !containedIn(root.Path, base) {
			return nil, apperr.New(apperr.CodeInvalidInput, "folder cannot leave the authorized root")
		}
	}
	if err := refuseSymlinkChain(root.Path, base); err != nil {
		return nil, err
	}
	info, err := os.Lstat(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, apperr.New(apperr.CodeInvalidInput, "authorized folder must be a real directory")
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}
	registered := map[string]struct{}{}
	if reg, err := a.loadRegistry(); err == nil {
		for _, row := range reg.Workspaces {
			registered[filepath.Clean(row.CanonicalPath)] = struct{}{}
		}
	}
	out := make([]AuthorizedChild, 0, len(entries))
	for _, entry := range entries {
		if len(out) >= maxAuthorizedChildren {
			break
		}
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if _, skip := skipDirNames[name]; skip {
			continue
		}
		child := filepath.Join(base, name)
		st, err := os.Lstat(child)
		if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			continue
		}
		relChild := name
		if folder != "" {
			relChild = filepath.Join(folder, name)
		}
		hit := AuthorizedChild{Name: name, Rel: relChild}
		marker := filepath.Join(child, storage.TrackerDirName)
		if markerInfo, err := os.Lstat(marker); err == nil && markerInfo.IsDir() && markerInfo.Mode()&os.ModeSymlink == 0 {
			hit.Workspace = true
			if _, ok := registered[filepath.Clean(child)]; ok {
				hit.Registered = true
			}
		}
		out = append(out, hit)
	}
	return out, nil
}

func containedIn(root, candidate string) bool {
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	return true
}

// CanonicalExistingDir resolves an existing absolute directory to a real
// (non-symlink) path. Intermediate OS symlinks such as /tmp are followed so
// confirmation can show the exact directory that would be granted.
func CanonicalExistingDir(absPath string) (string, error) {
	absPath = strings.TrimSpace(absPath)
	if absPath == "" {
		return "", apperr.New(apperr.CodeInvalidInput, "directory is required")
	}
	if !filepath.IsAbs(absPath) {
		return "", apperr.New(apperr.CodeInvalidInput, "directory must be an absolute path")
	}
	clean := filepath.Clean(absPath)
	info, err := os.Lstat(clean)
	if err != nil {
		if os.IsNotExist(err) {
			return "", apperr.New(apperr.CodeInvalidInput, "directory does not exist")
		}
		return "", err
	}
	if !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
		return "", apperr.New(apperr.CodeInvalidInput, "path grant requires a directory")
	}
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return "", apperr.New(apperr.CodeInvalidInput, "path grant refuses a symlink chain")
	}
	resolved = filepath.Clean(resolved)
	info, err = os.Lstat(resolved)
	if err != nil {
		return "", apperr.New(apperr.CodeInvalidInput, "directory does not exist")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", apperr.New(apperr.CodeInvalidInput, "path grant refuses a symlink")
	}
	if !info.IsDir() {
		return "", apperr.New(apperr.CodeInvalidInput, "path grant requires a directory")
	}
	return resolved, nil
}

func (a *App) refuseBroadDirectory(path string) error {
	if path == string(os.PathSeparator) || filepath.Dir(path) == path {
		return apperr.New(apperr.CodeInvalidInput, "choose a project directory, not a filesystem root")
	}
	if path == a.Home() {
		return apperr.New(apperr.CodeInvalidInput, "choose a project directory, not the user home directory")
	}
	if path == a.StateDir() {
		return apperr.New(apperr.CodeInvalidInput, "cannot initialize the Atlas state directory as a board")
	}
	return nil
}

// PreviewDirectoryGrant inspects an existing absolute directory and mints a
// Home-sourced one-time grant. It does not scaffold, register, or consume.
func (a *App) PreviewDirectoryGrant(ctx context.Context, absPath, purpose string) (PathGrant, error) {
	_ = ctx
	purpose = strings.TrimSpace(purpose)
	if purpose != PathGrantInit && purpose != PathGrantRegister {
		return PathGrant{}, apperr.New(apperr.CodeInvalidInput, "directory selection supports creating or attaching a board")
	}
	canonical, err := CanonicalExistingDir(absPath)
	if err != nil {
		return PathGrant{}, err
	}
	if err := a.refuseBroadDirectory(canonical); err != nil {
		return PathGrant{}, err
	}
	if purpose == PathGrantInit {
		if err := RefuseNestedWorkspaceInit(canonical); err != nil {
			return PathGrant{}, err
		}
	}
	if purpose == PathGrantRegister {
		if _, err := service.InitializedWorkspaceRoot(canonical); err != nil {
			return PathGrant{}, err
		}
	}
	return a.writePathGrant(canonical, purpose, PathGrantSourceHome)
}

func refuseSymlinkChain(root, target string) error {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	info, err := os.Lstat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return apperr.New(apperr.CodeInvalidInput, "authorized root refuses a symlink")
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return apperr.New(apperr.CodeInvalidInput, "folder cannot leave the authorized root")
	}
	if rel == "." {
		return nil
	}
	current := root
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return apperr.New(apperr.CodeInvalidInput, "path grant refuses a symlink")
		}
	}
	return nil
}
