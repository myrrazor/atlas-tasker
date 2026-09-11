package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
)

const registryFormat = "atlas_workspace_registry_v1"

// RegistryEntry is one machine-local binding from a workspace ID to the
// canonical path last verified on this host. The registry is never used as a
// fallback workspace: MCP resolution always starts from the current directory.
type RegistryEntry struct {
	WorkspaceID   string    `json:"workspace_id"`
	CanonicalPath string    `json:"canonical_path"`
	Dev           uint64    `json:"dev,omitempty"`
	Ino           uint64    `json:"ino,omitempty"`
	RegisteredAt  time.Time `json:"registered_at"`
	VerifiedAt    time.Time `json:"verified_at"`
}

type workspaceRegistry struct {
	Format     string                   `json:"format"`
	Workspaces map[string]RegistryEntry `json:"workspaces"`
}

func loadRegistry(stateDir string) (workspaceRegistry, error) {
	reg := workspaceRegistry{Format: registryFormat, Workspaces: map[string]RegistryEntry{}}
	raw, err := os.ReadFile(registryPath(stateDir))
	if os.IsNotExist(err) {
		return reg, nil
	}
	if err != nil {
		return reg, fmt.Errorf("read workspace registry: %w", err)
	}
	if err := json.Unmarshal(raw, &reg); err != nil {
		return reg, fmt.Errorf("decode workspace registry: %w", err)
	}
	if reg.Format != "" && reg.Format != registryFormat {
		return workspaceRegistry{}, fmt.Errorf("unsupported workspace registry format %q", reg.Format)
	}
	reg.Format = registryFormat
	if reg.Workspaces == nil {
		reg.Workspaces = map[string]RegistryEntry{}
	}
	return reg, nil
}

func saveRegistry(stateDir string, reg workspaceRegistry) error {
	if err := ensurePrivateDir(stateDir); err != nil {
		return err
	}
	reg.Format = registryFormat
	if reg.Workspaces == nil {
		reg.Workspaces = map[string]RegistryEntry{}
	}
	raw, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode workspace registry: %w", err)
	}
	return atomicWriteFile(registryPath(stateDir), append(raw, '\n'), filePerm)
}

func (r workspaceRegistry) lookup(id string) (RegistryEntry, bool) {
	entry, ok := r.Workspaces[strings.TrimSpace(id)]
	return entry, ok
}

func (r *workspaceRegistry) put(entry RegistryEntry) {
	if r.Workspaces == nil {
		r.Workspaces = map[string]RegistryEntry{}
	}
	r.Workspaces[entry.WorkspaceID] = entry
}

func checkRegistryBinding(entry RegistryEntry, currentPath string, currentID string) error {
	registered := filepath.Clean(entry.CanonicalPath)
	current := filepath.Clean(currentPath)
	if registered == current {
		if dev, ino, ok := fileDevIno(current); ok && entry.Dev != 0 && entry.Ino != 0 {
			if dev != entry.Dev || ino != entry.Ino {
				return apperr.New(apperr.CodeInvalidInput, "replaced workspace: the registered path no longer refers to the same directory")
			}
		}
		return nil
	}
	info, err := os.Lstat(registered)
	if os.IsNotExist(err) {
		return apperr.New(apperr.CodeRepairNeeded, "moved workspace: repair required")
	}
	if err != nil {
		return fmt.Errorf("inspect registered workspace path: %w", err)
	}
	if !info.IsDir() {
		return apperr.New(apperr.CodeRepairNeeded, "moved workspace: repair required")
	}
	raw, err := os.ReadFile(filepath.Join(registered, ".tracker", "workspace.json"))
	if err != nil {
		return apperr.New(apperr.CodeRepairNeeded, "moved workspace: repair required")
	}
	var meta struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return apperr.New(apperr.CodeRepairNeeded, "moved workspace: repair required")
	}
	if strings.TrimSpace(meta.WorkspaceID) == currentID {
		return apperr.New(apperr.CodeInvalidInput, "copied workspace with the same stale local registration fails until reconciled")
	}
	return apperr.New(apperr.CodeRepairNeeded, "moved workspace: repair required")
}
