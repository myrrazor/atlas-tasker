package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/setup"
)

func (a *App) grantsDir() string {
	return filepath.Join(a.stateDir, "path-grants")
}

func (a *App) grantFile(id string) string {
	return filepath.Join(a.grantsDir(), id+".json")
}

func validGrantPurpose(purpose string) bool {
	switch purpose {
	case PathGrantInit, PathGrantRegister, PathGrantRepair:
		return true
	default:
		return false
	}
}

func (a *App) GrantPath(ctx context.Context, absPath, purpose string) (PathGrant, error) {
	_ = ctx
	purpose = strings.TrimSpace(purpose)
	if !validGrantPurpose(purpose) {
		return PathGrant{}, apperr.New(apperr.CodeInvalidInput, "path grant purpose must be init, register, or repair")
	}
	if strings.TrimSpace(absPath) == "" {
		return PathGrant{}, apperr.New(apperr.CodeInvalidInput, "path is required")
	}
	if !filepath.IsAbs(absPath) {
		return PathGrant{}, apperr.New(apperr.CodeInvalidInput, "path grant requires an absolute path")
	}
	return a.writePathGrant(filepath.Clean(absPath), purpose, "")
}

func (a *App) writePathGrant(clean, purpose, source string) (PathGrant, error) {
	info, err := os.Lstat(clean)
	if err != nil {
		if os.IsNotExist(err) {
			return PathGrant{}, apperr.New(apperr.CodeInvalidInput, "path grant requires an existing directory")
		}
		return PathGrant{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return PathGrant{}, apperr.New(apperr.CodeInvalidInput, "path grant refuses a symlink")
	}
	if !info.IsDir() {
		return PathGrant{}, apperr.New(apperr.CodeInvalidInput, "path grant requires a directory")
	}
	dev, ino, ok := setup.FileDevIno(clean)
	if !ok {
		return PathGrant{}, apperr.New(apperr.CodeInvalidInput, "path grant could not bind directory identity")
	}
	grant := PathGrant{
		ID:        randomID(),
		Path:      clean,
		Purpose:   purpose,
		ExpiresAt: a.now().Add(10 * time.Minute),
		Dev:       dev,
		Ino:       ino,
		Source:    strings.TrimSpace(source),
	}
	if err := os.MkdirAll(a.grantsDir(), 0o700); err != nil {
		return PathGrant{}, err
	}
	if err := atomicJSON(a.grantFile(grant.ID), grant); err != nil {
		return PathGrant{}, err
	}
	return grant, nil
}

func (a *App) ListPendingGrants() []PathGrant {
	entries, err := os.ReadDir(a.grantsDir())
	if err != nil {
		return nil
	}
	now := a.now()
	out := make([]PathGrant, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(a.grantsDir(), entry.Name()))
		if err != nil {
			continue
		}
		var grant PathGrant
		if json.Unmarshal(raw, &grant) != nil || grant.ID == "" {
			continue
		}
		if !grant.ExpiresAt.IsZero() && now.After(grant.ExpiresAt) {
			continue
		}
		out = append(out, grant)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ExpiresAt.Before(out[j].ExpiresAt) })
	return out
}

func (a *App) LookupPendingGrant(id string) (PathGrant, error) {
	id = strings.TrimSpace(id)
	if id == "" || strings.Contains(id, "/") || strings.Contains(id, "..") {
		return PathGrant{}, apperr.New(apperr.CodeInvalidInput, "grant id is required")
	}
	raw, err := os.ReadFile(a.grantFile(id))
	if err != nil {
		return PathGrant{}, apperr.New(apperr.CodeNotFound, "path grant is missing or already used")
	}
	var grant PathGrant
	if json.Unmarshal(raw, &grant) != nil || grant.ID != id {
		return PathGrant{}, apperr.New(apperr.CodeInvalidInput, "path grant is unreadable")
	}
	if !grant.ExpiresAt.IsZero() && a.now().After(grant.ExpiresAt) {
		return PathGrant{}, apperr.New(apperr.CodeInvalidInput, "path grant expired")
	}
	return grant, nil
}

func (a *App) ConsumeGrant(ctx context.Context, id string) (PathGrant, error) {
	return a.consumeGrant(ctx, id, "")
}

// ConsumeGrantFor is the Home/CLI consume path. requiredPurpose must match
// the minted grant (init, register, or repair). Wrong purpose does not burn
// the grant.
func (a *App) ConsumeGrantFor(ctx context.Context, id, requiredPurpose string) (PathGrant, error) {
	requiredPurpose = strings.TrimSpace(requiredPurpose)
	if !validGrantPurpose(requiredPurpose) {
		return PathGrant{}, apperr.New(apperr.CodeInvalidInput, "path grant purpose must be init, register, or repair")
	}
	return a.consumeGrant(ctx, id, requiredPurpose)
}

func (a *App) consumeGrant(ctx context.Context, id, requiredPurpose string) (PathGrant, error) {
	_ = ctx
	id = strings.TrimSpace(id)
	if id == "" || strings.Contains(id, "/") || strings.Contains(id, "..") {
		return PathGrant{}, apperr.New(apperr.CodeInvalidInput, "grant id is required")
	}
	path := a.grantFile(id)
	taken := path + ".taken-" + randomID()
	if err := os.Rename(path, taken); err != nil {
		return PathGrant{}, apperr.New(apperr.CodeNotFound, "path grant is missing or already used")
	}
	putBack := false
	defer func() {
		if putBack {
			_ = os.Rename(taken, path)
			return
		}
		_ = os.Remove(taken)
	}()
	raw, err := os.ReadFile(taken)
	if err != nil {
		return PathGrant{}, apperr.New(apperr.CodeNotFound, "path grant is missing or already used")
	}
	var grant PathGrant
	if err := json.Unmarshal(raw, &grant); err != nil || grant.ID != id {
		return PathGrant{}, apperr.New(apperr.CodeInvalidInput, "path grant is unreadable")
	}
	if requiredPurpose != "" && grant.Purpose != requiredPurpose {
		putBack = true
		return PathGrant{}, apperr.New(apperr.CodePermissionDenied, "path grant purpose does not match")
	}
	if !grant.ExpiresAt.IsZero() && a.now().After(grant.ExpiresAt) {
		return PathGrant{}, apperr.New(apperr.CodeInvalidInput, "path grant expired")
	}
	if err := bindGrantPath(grant); err != nil {
		return PathGrant{}, err
	}
	return grant, nil
}

func bindGrantPath(grant PathGrant) error {
	if grant.Source == PathGrantSourceHome {
		canonical, err := CanonicalExistingDir(grant.Path)
		if err != nil || canonical != grant.Path {
			return apperr.New(apperr.CodeInvalidInput, "selected directory changed; review it again")
		}
	}
	info, err := os.Lstat(grant.Path)
	if err != nil {
		return apperr.New(apperr.CodeInvalidInput, "path grant directory is missing")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return apperr.New(apperr.CodeInvalidInput, "path grant refuses a symlink swap")
	}
	if !info.IsDir() {
		return apperr.New(apperr.CodeInvalidInput, "path grant requires a directory")
	}
	dev, ino, ok := setup.FileDevIno(grant.Path)
	if !ok || (grant.Dev != 0 && grant.Ino != 0 && (dev != grant.Dev || ino != grant.Ino)) {
		return apperr.New(apperr.CodeInvalidInput, "path grant directory identity changed")
	}
	return nil
}
