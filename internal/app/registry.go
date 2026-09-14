package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/setup"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

type fileRegistry struct {
	Format     string                    `json:"format"`
	Workspaces map[string]registryRecord `json:"workspaces"`
}

type registryRecord struct {
	WorkspaceID   string    `json:"workspace_id"`
	CanonicalPath string    `json:"canonical_path"`
	DisplayName   string    `json:"display_name,omitempty"`
	Dev           uint64    `json:"dev,omitempty"`
	Ino           uint64    `json:"ino,omitempty"`
	RegisteredAt  time.Time `json:"registered_at"`
	VerifiedAt    time.Time `json:"verified_at"`
	LastSeenAt    time.Time `json:"last_seen_at,omitempty"`
	Visibility    string    `json:"visibility,omitempty"`
}

func registryFile(stateDir string) string {
	return filepath.Join(stateDir, "registry.json")
}

func (a *App) loadRegistry() (fileRegistry, error) {
	reg := fileRegistry{Format: registryFormatV2, Workspaces: map[string]registryRecord{}}
	raw, err := os.ReadFile(registryFile(a.stateDir))
	if os.IsNotExist(err) {
		return reg, nil
	}
	if err != nil {
		return fileRegistry{}, fmt.Errorf("read workspace registry: %w", err)
	}
	if err := json.Unmarshal(raw, &reg); err != nil {
		return fileRegistry{}, fmt.Errorf("decode workspace registry: %w", err)
	}
	if reg.Format != "" && reg.Format != "atlas_workspace_registry_v1" && reg.Format != registryFormatV2 {
		return fileRegistry{}, fmt.Errorf("unsupported workspace registry format %q", reg.Format)
	}
	reg.Format = registryFormatV2
	if reg.Workspaces == nil {
		reg.Workspaces = map[string]registryRecord{}
	}
	return reg, nil
}

func (a *App) saveRegistry(reg fileRegistry) error {
	reg.Format = registryFormatV2
	if reg.Workspaces == nil {
		reg.Workspaces = map[string]registryRecord{}
	}
	return atomicJSON(registryFile(a.stateDir), reg)
}

func (a *App) Register(ctx context.Context, opts RegisterOptions) (WorkspaceRecord, error) {
	return a.register(ctx, opts, false)
}

func (a *App) register(_ context.Context, opts RegisterOptions, alreadyLocked bool) (WorkspaceRecord, error) {
	root, err := service.InitializedWorkspaceRoot(opts.Root)
	if err != nil {
		return WorkspaceRecord{}, err
	}
	id, err := service.LoadWorkspaceIdentity(root)
	if err != nil {
		return WorkspaceRecord{}, err
	}
	if strings.TrimSpace(id) == "" {
		return WorkspaceRecord{}, apperr.New(apperr.CodeInvalidInput, "workspace identity is missing")
	}
	dev, ino, _ := setup.FileDevIno(root)
	now := a.now()
	display := strings.TrimSpace(opts.DisplayName)
	if display == "" {
		display = filepath.Base(root)
	}
	var rec WorkspaceRecord
	write := func() error {
		reg, err := a.loadRegistry()
		if err != nil {
			return err
		}
		info, err := os.Lstat(root)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return apperr.New(apperr.CodeInvalidInput, "refusing a symlinked workspace path")
		}
		existing, ok := reg.Workspaces[id]
		if ok {
			registered := filepath.Clean(existing.CanonicalPath)
			if registered != root {
				health, detail := classifyHealth(existing)
				if health == HealthMoved || os.IsNotExist(statErr(registered)) {
					return apperr.New(apperr.CodeRepairNeeded, "moved workspace: repair required")
				}
				if health == HealthCopiedIdentityConflict || health == HealthAvailable {
					return apperr.New(apperr.CodeInvalidInput, "copied workspace with the same stale local registration fails until reconciled")
				}
				return apperr.New(apperr.CodeRepairNeeded, detail)
			}
			if existing.Dev != 0 && existing.Ino != 0 && (dev != existing.Dev || ino != existing.Ino) {
				return apperr.New(apperr.CodeRepairNeeded, "replaced workspace: the registered path no longer refers to the same directory")
			}
			existing.DisplayName = display
			existing.VerifiedAt = now
			existing.LastSeenAt = now
			if existing.Visibility == "" {
				existing.Visibility = string(VisibilityVisible)
			}
			reg.Workspaces[id] = existing
		} else {
			existing = registryRecord{
				WorkspaceID:   id,
				CanonicalPath: root,
				DisplayName:   display,
				Dev:           dev,
				Ino:           ino,
				RegisteredAt:  now,
				VerifiedAt:    now,
				LastSeenAt:    now,
				Visibility:    string(VisibilityVisible),
			}
			reg.Workspaces[id] = existing
		}
		if err := a.saveRegistry(reg); err != nil {
			return err
		}
		rec = a.decorate(existing)
		return nil
	}
	if alreadyLocked {
		err = write()
	} else {
		err = a.withMachineLock("register workspace", write)
	}
	return rec, err
}

func (a *App) ListWorkspaces(ctx context.Context, opts ListOptions) ([]WorkspaceRecord, error) {
	_ = ctx
	reg, err := a.loadRegistry()
	if err != nil {
		return nil, err
	}
	out := make([]WorkspaceRecord, 0, len(reg.Workspaces))
	for _, row := range reg.Workspaces {
		rec := a.decorate(row)
		if rec.Visibility == VisibilityHidden && !opts.IncludeHidden && !a.snapshotSettings().Home.ShowHidden {
			continue
		}
		if opts.Health != "" && rec.Health != opts.Health {
			continue
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DisplayName == out[j].DisplayName {
			return out[i].WorkspaceID < out[j].WorkspaceID
		}
		return out[i].DisplayName < out[j].DisplayName
	})
	return out, nil
}

func (a *App) WorkspaceByID(ctx context.Context, id string) (WorkspaceRecord, error) {
	_ = ctx
	row, ok, err := a.lookup(id)
	if err != nil {
		return WorkspaceRecord{}, err
	}
	if !ok {
		return WorkspaceRecord{}, apperr.New(apperr.CodeNotFound, "workspace is not registered")
	}
	return a.decorate(row), nil
}

func (a *App) lookup(id string) (registryRecord, bool, error) {
	reg, err := a.loadRegistry()
	if err != nil {
		return registryRecord{}, false, err
	}
	row, ok := reg.Workspaces[strings.TrimSpace(id)]
	return row, ok, nil
}

func (a *App) decorate(row registryRecord) WorkspaceRecord {
	vis := Visibility(row.Visibility)
	if vis == "" {
		vis = VisibilityVisible
	}
	rec := WorkspaceRecord{
		WorkspaceID:  row.WorkspaceID,
		Path:         row.CanonicalPath,
		DisplayName:  row.DisplayName,
		Dev:          row.Dev,
		Ino:          row.Ino,
		RegisteredAt: row.RegisteredAt,
		VerifiedAt:   row.VerifiedAt,
		LastSeenAt:   row.LastSeenAt,
		Visibility:   vis,
	}
	rec.Health, rec.HealthDetail = classifyHealth(row)
	return rec
}

func classifyHealth(row registryRecord) (Health, string) {
	path := filepath.Clean(row.CanonicalPath)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return HealthMoved, "registered path is missing; repair required"
	}
	if err != nil {
		if os.IsPermission(err) {
			return HealthPermissionDenied, err.Error()
		}
		return HealthUnavailable, err.Error()
	}
	if !info.IsDir() {
		return HealthMoved, "registered path is not a directory"
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return HealthUnavailable, "refusing a symlinked workspace path"
	}
	id, err := service.LoadWorkspaceIdentity(path)
	if err != nil {
		return HealthCorruptIdentity, err.Error()
	}
	if strings.TrimSpace(id) == "" {
		if _, statErr := os.Stat(storage.WorkspaceMetadataFile(path)); os.IsNotExist(statErr) {
			return HealthSchemaUpgradeRequired, "workspace identity is missing"
		}
		return HealthCorruptIdentity, "workspace identity is empty"
	}
	if id != row.WorkspaceID {
		return HealthCopiedIdentityConflict, "path no longer holds the registered workspace id"
	}
	if row.Dev != 0 && row.Ino != 0 {
		dev, ino, ok := setup.FileDevIno(path)
		if ok && (dev != row.Dev || ino != row.Ino) {
			return HealthReplacedPath, "the registered path no longer refers to the same directory"
		}
	}
	if _, err := service.InitializedWorkspaceRoot(path); err != nil {
		return HealthUnavailable, err.Error()
	}
	return HealthAvailable, ""
}

func (a *App) Repair(ctx context.Context, opts RepairOptions) (RepairResult, error) {
	_ = ctx
	id := strings.TrimSpace(opts.WorkspaceID)
	if id == "" {
		return RepairResult{}, apperr.New(apperr.CodeInvalidInput, "workspace id is required")
	}
	result := RepairResult{Kind: "workspace_repair"}
	err := a.withMachineLock("repair workspace", func() error {
		reg, err := a.loadRegistry()
		if err != nil {
			return err
		}
		row, ok := reg.Workspaces[id]
		if !ok {
			return apperr.New(apperr.CodeNotFound, "workspace is not registered")
		}
		now := a.now()
		switch opts.Action {
		case RepairHide:
			row.Visibility = string(VisibilityHidden)
			row.LastSeenAt = now
			reg.Workspaces[id] = row
		case RepairUnhide:
			row.Visibility = string(VisibilityVisible)
			row.LastSeenAt = now
			reg.Workspaces[id] = row
		case RepairRemovePointer:
			delete(reg.Workspaces, id)
			result.Record = a.decorate(row)
			return a.saveRegistry(reg)
		case RepairUpdatePath:
			root, err := service.InitializedWorkspaceRoot(opts.NewPath)
			if err != nil {
				return err
			}
			got, err := service.LoadWorkspaceIdentity(root)
			if err != nil {
				return err
			}
			if got != id {
				return apperr.New(apperr.CodeInvalidInput, "new path does not hold this workspace id")
			}
			dev, ino, _ := setup.FileDevIno(root)
			row.CanonicalPath = root
			row.Dev, row.Ino = dev, ino
			row.VerifiedAt = now
			row.LastSeenAt = now
			reg.Workspaces[id] = row
		case RepairForkCopy:
			if strings.TrimSpace(opts.NewPath) == "" {
				return apperr.New(apperr.CodeInvalidInput, "fork_copy requires NewPath")
			}
			copyRoot, err := service.InitializedWorkspaceRoot(opts.NewPath)
			if err != nil {
				return err
			}
			if filepath.Clean(copyRoot) == filepath.Clean(row.CanonicalPath) {
				return apperr.New(apperr.CodeInvalidInput, "fork_copy NewPath must be the copy, not the original")
			}
			copyID, err := service.LoadWorkspaceIdentity(copyRoot)
			if err != nil {
				return err
			}
			if copyID != id {
				return apperr.New(apperr.CodeInvalidInput, "fork_copy NewPath does not hold the original workspace id")
			}
			newID, err := forkWorkspaceIdentity(copyRoot)
			if err != nil {
				return err
			}
			dev, ino, _ := setup.FileDevIno(copyRoot)
			forked := registryRecord{
				WorkspaceID:   newID,
				CanonicalPath: copyRoot,
				DisplayName:   filepath.Base(copyRoot),
				Dev:           dev,
				Ino:           ino,
				RegisteredAt:  now,
				VerifiedAt:    now,
				LastSeenAt:    now,
				Visibility:    string(VisibilityVisible),
			}
			reg.Workspaces[newID] = forked
			result.ForkedID = newID
			result.Record = a.decorate(forked)
			return a.saveRegistry(reg)
		default:
			return apperr.New(apperr.CodeInvalidInput, "unknown repair action")
		}
		if err := a.saveRegistry(reg); err != nil {
			return err
		}
		result.Record = a.decorate(reg.Workspaces[id])
		return nil
	})
	return result, err
}

func forkWorkspaceIdentity(root string) (string, error) {
	path := storage.WorkspaceMetadataFile(root)
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		return "", err
	}
	id := randomID()
	meta["workspace_id"] = id
	meta["forked_at"] = time.Now().UTC().Format(time.RFC3339)
	encoded, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		return "", err
	}
	return id, nil
}

func statErr(path string) error {
	_, err := os.Lstat(path)
	return err
}
