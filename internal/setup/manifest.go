package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
)

const manifestFormat = "atlas_setup_v1"

// TrackerIdentity is the installed binary recorded in the setup manifest.
type TrackerIdentity struct {
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Version string `json:"version"`
}

// ManifestManagedMode is the subset of managed mode stored locally.
type ManifestManagedMode struct {
	Enabled       bool   `json:"enabled"`
	CapturePolicy string `json:"capture_policy,omitempty"`
	StatusSource  string `json:"status_source,omitempty"`
	DeclaredMode  string `json:"declared_mode,omitempty"`
}

// ManifestIntegration is one provider row in the setup manifest.
type ManifestIntegration struct {
	State            adapter.State             `json:"state"`
	ServerName       string                    `json:"server_name,omitempty"`
	Actor            contracts.Actor           `json:"actor,omitempty"`
	AdapterVersion   int                       `json:"adapter_version"`
	SkillVersion     string                    `json:"skill_version,omitempty"`
	ManagedBlock     string                    `json:"managed_block_version,omitempty"`
	MCPProfile       string                    `json:"mcp_profile,omitempty"`
	WorkspaceBinding string                    `json:"workspace_binding,omitempty"`
	Fingerprint      string                    `json:"entry_fingerprint,omitempty"`
	RepairReason     string                    `json:"repair_reason,omitempty"`
	Record           *adapter.IntegrationState `json:"record,omitempty"`
}

// ManifestBackup is the backup group recorded locally.
type ManifestBackup struct {
	Enabled   bool   `json:"enabled"`
	TargetID  string `json:"target_id,omitempty"`
	ReplicaID string `json:"replica_id,omitempty"`
	Deferred  string `json:"deferred,omitempty"`
}

// Manifest is the local setup/ownership record. It never contains credentials,
// ticket text, or provider tokens.
type Manifest struct {
	Format        string                         `json:"format"`
	WorkspaceID   string                         `json:"workspace_id"`
	WorkspacePath string                         `json:"workspace_path"`
	TrackerBinary TrackerIdentity                `json:"tracker_binary"`
	ManagedMode   ManifestManagedMode            `json:"managed_mode"`
	Integrations  map[string]ManifestIntegration `json:"integrations"`
	Backup        ManifestBackup                 `json:"backup"`
	SchemaVersion int                            `json:"schema_version"`
	UpdatedAt     time.Time                      `json:"updated_at"`
}

func manifestPath(stateDir, workspaceID string) string {
	return filepath.Join(manifestsDir(stateDir), workspaceID+".json")
}

func loadManifest(stateDir, workspaceID string) (*Manifest, error) {
	raw, err := os.ReadFile(manifestPath(stateDir, workspaceID))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read setup manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("decode setup manifest: %w", err)
	}
	if manifest.Integrations == nil {
		manifest.Integrations = map[string]ManifestIntegration{}
	}
	return &manifest, nil
}

func saveManifest(stateDir string, manifest *Manifest) error {
	if err := ensurePrivateDir(manifestsDir(stateDir)); err != nil {
		return err
	}
	manifest.Format = manifestFormat
	manifest.SchemaVersion = 1
	manifest.UpdatedAt = time.Now().UTC()
	if manifest.Integrations == nil {
		manifest.Integrations = map[string]ManifestIntegration{}
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode setup manifest: %w", err)
	}
	return atomicWriteFile(manifestPath(stateDir, manifest.WorkspaceID), append(raw, '\n'), filePerm)
}

func emptyManifest(workspaceID, workspacePath string, tracker TrackerIdentity) *Manifest {
	return &Manifest{
		Format:        manifestFormat,
		WorkspaceID:   workspaceID,
		WorkspacePath: workspacePath,
		TrackerBinary: tracker,
		Integrations:  map[string]ManifestIntegration{},
		SchemaVersion: 1,
	}
}

func integrationKey(target integrations.Target) string {
	return string(target)
}

func (m *Manifest) putIntegration(target integrations.Target, row ManifestIntegration) {
	if m.Integrations == nil {
		m.Integrations = map[string]ManifestIntegration{}
	}
	m.Integrations[integrationKey(target)] = row
}

func (m *Manifest) integration(target integrations.Target) (ManifestIntegration, bool) {
	if m == nil {
		return ManifestIntegration{}, false
	}
	row, ok := m.Integrations[integrationKey(target)]
	return row, ok
}

func trackerIdentity(path string, version string) TrackerIdentity {
	identity := TrackerIdentity{Path: path, Version: version}
	if strings.TrimSpace(path) == "" {
		return identity
	}
	if sum, err := hashFile(path); err == nil {
		identity.SHA256 = sum
	}
	return identity
}
