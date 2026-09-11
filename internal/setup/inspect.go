package setup

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

// Inspection is the read-only snapshot the planner starts from. It never
// includes file contents or secret values.
type Inspection struct {
	Initialized      bool                     `json:"initialized"`
	WorkspaceID      string                   `json:"workspace_id,omitempty"`
	WorkspaceRoot    string                   `json:"workspace_root"`
	Projects         []string                 `json:"projects"`
	Agents           []string                 `json:"agents"`
	CompletionMode   string                   `json:"completion_mode,omitempty"`
	RequiredReviewer string                   `json:"required_reviewer,omitempty"`
	Detections       []integrations.Detection `json:"detections"`
	ManagedBlocks    []ManagedBlockSighting   `json:"managed_blocks"`
	MCPSightings     []MCPSighting            `json:"existing_mcp_registrations"`
	Manifest         *Manifest                `json:"setup_manifest,omitempty"`
	Backup           BackupInspection         `json:"backup"`
	Scheduler        SchedulerInspection      `json:"scheduler"`
}

// ManagedBlockSighting records that a managed instruction block exists.
type ManagedBlockSighting struct {
	Target  integrations.Target `json:"target"`
	Path    string              `json:"path"`
	Present bool                `json:"present"`
}

// MCPSighting is a read-only observation of an atlas-* server name in a
// known client config file. It does not parse secrets out of the file.
type MCPSighting struct {
	Target  integrations.Target `json:"target"`
	Path    string              `json:"path"`
	Present bool                `json:"present"`
}

// BackupInspection reports existing backup state without configuring it.
type BackupInspection struct {
	ManifestsPresent bool     `json:"manifests_present"`
	SnapshotsPresent bool     `json:"snapshots_present"`
	TargetConfigured bool     `json:"target_configured"`
	Notes            []string `json:"notes,omitempty"`
}

// SchedulerInspection reports whether a user-level backup scheduler exists.
type SchedulerInspection struct {
	UserServicePresent bool     `json:"user_service_present"`
	Notes              []string `json:"notes,omitempty"`
}

func inspectWorkspace(workspaceRoot, home, stateDir string, lookPath func(string) (string, error), getenv func(string) string) (Inspection, error) {
	inspection := Inspection{
		WorkspaceRoot: workspaceRoot,
		Projects:      []string{},
		Agents:        []string{},
		ManagedBlocks: []ManagedBlockSighting{},
		MCPSightings:  []MCPSighting{},
	}
	if info, err := os.Stat(storage.TrackerDir(workspaceRoot)); err == nil && info.IsDir() {
		inspection.Initialized = true
	}
	if id, err := service.LoadWorkspaceIdentity(workspaceRoot); err == nil {
		inspection.WorkspaceID = id
	}
	inspection.Projects = listDirNames(storage.ProjectsDir(workspaceRoot))
	inspection.Agents = listStemNames(storage.AgentsDir(workspaceRoot), ".md")
	if cfg, err := config.Load(workspaceRoot); err == nil {
		inspection.CompletionMode = string(cfg.Workflow.CompletionMode)
		inspection.RequiredReviewer = string(cfg.Workflow.RequiredReviewer)
	}
	inspection.Detections = integrations.Detect(integrations.DetectOptions{
		Workspace: workspaceRoot,
		Home:      home,
		LookPath:  lookPath,
		Getenv:    getenv,
	})
	for _, target := range integrations.DetectableTargets() {
		caps, err := adapter.CapabilitiesFor(target)
		if err != nil {
			continue
		}
		instruction := filepath.Join(workspaceRoot, caps.InstructionFile)
		begin, end, err := integrations.InstructionMarkers(target)
		present := false
		if err == nil {
			if raw, readErr := os.ReadFile(instruction); readErr == nil {
				body := string(raw)
				present = strings.Contains(body, begin) && strings.Contains(body, end)
			}
		}
		inspection.ManagedBlocks = append(inspection.ManagedBlocks, ManagedBlockSighting{
			Target: target, Path: instruction, Present: present,
		})
		for _, rel := range knownMCPConfigRels(target) {
			path := rel
			if !filepath.IsAbs(path) {
				path = filepath.Join(workspaceRoot, rel)
			} else if strings.HasPrefix(rel, "~/") && home != "" {
				path = filepath.Join(home, strings.TrimPrefix(rel, "~/"))
			}
			inspection.MCPSightings = append(inspection.MCPSightings, MCPSighting{
				Target: target, Path: path, Present: fileMentionsAtlasServer(path),
			})
		}
	}
	if inspection.WorkspaceID != "" && stateDir != "" {
		if manifest, err := loadManifest(stateDir, inspection.WorkspaceID); err == nil {
			inspection.Manifest = manifest
		}
	}
	if entries, err := os.ReadDir(storage.BackupManifestsDir(workspaceRoot)); err == nil && len(entries) > 0 {
		inspection.Backup.ManifestsPresent = true
	}
	if entries, err := os.ReadDir(storage.BackupSnapshotsDir(workspaceRoot)); err == nil && len(entries) > 0 {
		inspection.Backup.SnapshotsPresent = true
	}
	if inspection.Manifest != nil && inspection.Manifest.Backup.Enabled {
		inspection.Backup.TargetConfigured = true
	}
	if inspection.WorkspaceID != "" && stateDir != "" {
		targetPath := filepath.Join(stateDir, "backups", inspection.WorkspaceID, "targets.json")
		if _, err := os.Stat(targetPath); err == nil {
			inspection.Backup.TargetConfigured = true
		}
		if _, err := os.Stat(filepath.Join(stateDir, "backups", inspection.WorkspaceID, "auto.json")); err == nil {
			inspection.Backup.TargetConfigured = true
		}
	}
	if home != "" {
		for _, rel := range []string{
			filepath.Join(".config", "systemd", "user"),
			filepath.Join("Library", "LaunchAgents"),
		} {
			dir := filepath.Join(home, rel)
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if strings.Contains(entry.Name(), "atlas-backup") || strings.Contains(entry.Name(), "atlas-tasker.backup") {
					inspection.Scheduler.UserServicePresent = true
				}
			}
		}
	}
	if !inspection.Backup.TargetConfigured {
		inspection.Backup.Notes = []string{"no machine-local backup target is configured"}
	}
	if !inspection.Scheduler.UserServicePresent {
		inspection.Scheduler.Notes = []string{"no user-level backup scheduler is installed"}
	}
	return inspection, nil
}

func knownMCPConfigRels(target integrations.Target) []string {
	switch target {
	case integrations.TargetCodex:
		return []string{filepath.Join(".codex", "config.toml")}
	case integrations.TargetClaude:
		return []string{".mcp.json"}
	case integrations.TargetCursor:
		return []string{filepath.Join(".cursor", "mcp.json")}
	case integrations.TargetGrok:
		return []string{filepath.Join(".grok", "config.toml")}
	case integrations.TargetGeneric:
		return []string{filepath.Join(".tracker", "integrations", "atlas-mcp.json"), ".mcp.json"}
	default:
		return nil
	}
}

func fileMentionsAtlasServer(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(raw), adapter.ServerNamePrefix)
}

func listDirNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{}
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

func listStemNames(dir, ext string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{}
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ext) {
			names = append(names, strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))
		}
	}
	sort.Strings(names)
	return names
}

func managedModePath(root string) string {
	return storage.ManagedModeFile(root)
}
