package app

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/setup"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

var skipDirNames = map[string]struct{}{
	"node_modules": {},
	".git":         {},
	"dist":         {},
	"build":        {},
	"vendor":       {},
	"target":       {},
	".cache":       {},
	".venv":        {},
	"venv":         {},
}

func (a *App) Discover(ctx context.Context, opts DiscoverOptions) ([]DiscoveryHit, error) {
	_ = ctx
	settings := a.snapshotSettings()
	roots := opts.Roots
	if len(roots) == 0 {
		roots = append([]string{}, settings.Discovery.Roots...)
	}
	if len(roots) == 0 {
		return nil, apperr.New(apperr.CodeInvalidInput, "discovery requires configured roots; Atlas never crawls the whole home directory")
	}
	maxDepth := opts.MaxDepth
	if maxDepth <= 0 {
		maxDepth = settings.Discovery.MaxDepth
	}
	if maxDepth <= 0 {
		maxDepth = 4
	}
	reg, err := a.loadRegistry()
	if err != nil {
		return nil, err
	}
	registered := map[string]string{}
	for id, row := range reg.Workspaces {
		registered[filepath.Clean(row.CanonicalPath)] = id
	}
	var hits []DiscoveryHit
	seen := map[string]struct{}{}
	for _, root := range roots {
		if !filepath.IsAbs(root) {
			return nil, apperr.New(apperr.CodeInvalidInput, "discovery roots must be absolute")
		}
		root = filepath.Clean(root)
		info, err := os.Lstat(root)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		rootDev, _, _ := statDev(root)
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return nil
			}
			depth := 0
			if rel != "." {
				depth = len(strings.Split(rel, string(os.PathSeparator)))
			}
			if depth > maxDepth {
				return fs.SkipDir
			}
			name := d.Name()
			if strings.HasPrefix(name, ".") && path != root {
				if name != storage.TrackerDirName {
					return fs.SkipDir
				}
			}
			if _, skip := skipDirNames[name]; skip {
				return fs.SkipDir
			}
			if info, err := d.Info(); err == nil && info.Mode()&os.ModeSymlink != 0 {
				return fs.SkipDir
			}
			if rootDev != 0 {
				if dev, _, ok := statDev(path); ok && dev != rootDev {
					return fs.SkipDir
				}
			}
			marker := filepath.Join(path, storage.TrackerDirName)
			st, err := os.Lstat(marker)
			if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
				return nil
			}
			clean := filepath.Clean(path)
			if _, dup := seen[clean]; dup {
				return fs.SkipDir
			}
			seen[clean] = struct{}{}
			id, _ := service.LoadWorkspaceIdentity(clean)
			hit := DiscoveryHit{Path: clean, WorkspaceID: id, DisplayName: filepath.Base(clean)}
			if regID, ok := registered[clean]; ok {
				hit.Registered = true
				hit.WorkspaceID = regID
			}
			hits = append(hits, hit)
			return fs.SkipDir
		})
	}
	return hits, nil
}

func statDev(path string) (uint64, uint64, bool) {
	return setup.FileDevIno(path)
}
