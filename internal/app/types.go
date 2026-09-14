package app

import (
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

// Capability names used by CLI/web now and MCP later.
const (
	CapInit      = "init"
	CapRegister  = "register"
	CapList      = "list"
	CapRepair    = "repair"
	CapProject   = "project"
	CapTicket    = "ticket"
	CapBoard     = "board"
	CapAttention = "attention"
	CapSearch    = "search"
	CapSettings  = "settings"
	CapBackup    = "backup"
	CapRestore   = "restore"
)

type Scope string

const (
	ScopeMachine   Scope = "machine"
	ScopeWorkspace Scope = "workspace"
)

type Capability struct {
	Name     string `json:"name"`
	Scope    Scope  `json:"scope"`
	Mutating bool   `json:"mutating"`
}

type GitMode string

const (
	GitModeShared    GitMode = "shared"
	GitModePrivate   GitMode = "private"
	GitModeUnmanaged GitMode = "unmanaged"
)

func (m GitMode) IsValid() bool {
	switch m {
	case GitModeShared, GitModePrivate, GitModeUnmanaged, "":
		return true
	default:
		return false
	}
}

func (m GitMode) Normalized() GitMode {
	if m == "" {
		return GitModeShared
	}
	return m
}

type Health string

const (
	HealthAvailable              Health = "available"
	HealthUnavailable            Health = "unavailable"
	HealthMoved                  Health = "moved"
	HealthCopiedIdentityConflict Health = "copied_identity_conflict"
	HealthReplacedPath           Health = "replaced_path"
	HealthPermissionDenied       Health = "permission_denied"
	HealthSchemaUpgradeRequired  Health = "schema_upgrade_required"
	HealthCorruptIdentity        Health = "corrupt_identity"
	HealthDisabled               Health = "disabled"
)

type Visibility string

const (
	VisibilityVisible Visibility = "visible"
	VisibilityHidden  Visibility = "hidden"
)

type WorkspaceRecord struct {
	WorkspaceID  string     `json:"workspace_id"`
	Path         string     `json:"path"`
	DisplayName  string     `json:"display_name,omitempty"`
	Dev          uint64     `json:"dev,omitempty"`
	Ino          uint64     `json:"ino,omitempty"`
	RegisteredAt time.Time  `json:"registered_at"`
	VerifiedAt   time.Time  `json:"verified_at"`
	LastSeenAt   time.Time  `json:"last_seen_at,omitempty"`
	Visibility   Visibility `json:"visibility"`
	Health       Health     `json:"health"`
	HealthDetail string     `json:"health_detail,omitempty"`
}

type RepairAction string

const (
	RepairUpdatePath    RepairAction = "update_path"
	RepairForkCopy      RepairAction = "fork_copy"
	RepairHide          RepairAction = "hide"
	RepairUnhide        RepairAction = "unhide"
	RepairRemovePointer RepairAction = "remove_pointer"
)

type RepairOptions struct {
	WorkspaceID string
	Action      RepairAction
	NewPath     string // update_path
}

type RepairResult struct {
	Kind     string          `json:"kind"`
	Record   WorkspaceRecord `json:"record,omitempty"`
	ForkedID string          `json:"forked_workspace_id,omitempty"`
}

type ListOptions struct {
	IncludeHidden bool
	Health        Health
}

type RegisterOptions struct {
	Root        string
	DisplayName string
}

type InitOptions struct {
	Root           string
	GitMode        GitMode
	Register       bool
	Agents         bool
	Backup         bool
	DefaultProject bool
	OpenHome       bool
	WriteClientCfg bool
	Actor          contracts.Actor
}

func (o *InitOptions) applyDefaults() {
	o.GitMode = o.GitMode.Normalized()
}

type InitResult struct {
	Kind           string           `json:"kind"`
	Workspace      string           `json:"workspace"`
	WorkspaceID    string           `json:"workspace_id,omitempty"`
	Created        []string         `json:"created"`
	DefaultProject string           `json:"default_project,omitempty"`
	Registered     bool             `json:"registered"`
	GitMode        string           `json:"git_mode"`
	Agents         AgentSetupReport `json:"agents"`
	Backup         BackupInitReport `json:"backup"`
	Service        *ServiceStatus   `json:"service,omitempty"`
	Steps          []InitStep       `json:"steps,omitempty"`
	Already        bool             `json:"already_bootstrapped"`
	Summary        string           `json:"summary"`
}

type InitStepStatus string

const (
	InitStepDone       InitStepStatus = "done"
	InitStepSkipped    InitStepStatus = "skipped"
	InitStepFailed     InitStepStatus = "failed"
	InitStepConfigured InitStepStatus = "configured"
	InitStepUnverified InitStepStatus = "unverified"
)

type InitStep struct {
	Name   string         `json:"name"`
	Status InitStepStatus `json:"status"`
	Detail string         `json:"detail,omitempty"`
}

type AgentClientStatus string

const (
	AgentSkipped              AgentClientStatus = "skipped"
	AgentNotDetected          AgentClientStatus = "not_detected"
	AgentWritten              AgentClientStatus = "written"
	AgentPendingClientRestart AgentClientStatus = "pending_client_restart"
	AgentUnverified           AgentClientStatus = "unverified"
)

type AgentClientReport struct {
	Target     integrations.Target `json:"target"`
	Status     AgentClientStatus   `json:"status"`
	Command    string              `json:"command,omitempty"`
	Args       []string            `json:"args,omitempty"`
	ConfigPath string              `json:"config_path,omitempty"`
	Detail     string              `json:"detail,omitempty"`
}

type AgentSetupReport struct {
	Attempted bool                `json:"attempted"`
	Clients   []AgentClientReport `json:"clients,omitempty"`
	Notes     []string            `json:"notes,omitempty"`
}

type BackupInitReport struct {
	Attempted    bool   `json:"attempted"`
	ReplicaReady bool   `json:"replica_ready"`
	CheckpointID string `json:"checkpoint_id,omitempty"`
	Detail       string `json:"detail,omitempty"`
}

type PathGrant struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	Purpose   string    `json:"purpose"`
	ExpiresAt time.Time `json:"expires_at"`
	Dev       uint64    `json:"dev,omitempty"`
	Ino       uint64    `json:"ino,omitempty"`
}

type DiscoverOptions struct {
	Roots    []string
	MaxDepth int
}

type DiscoveryHit struct {
	Path        string `json:"path"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Registered  bool   `json:"registered"`
}

type DoctorOptions struct {
	Repair    bool
	Workspace string // empty: current CWD if it is a workspace
}

type DoctorReport struct {
	Kind             string         `json:"kind"`
	OK               bool           `json:"ok"`
	CurrentWorkspace string         `json:"current_workspace,omitempty"`
	Machine          map[string]any `json:"machine"`
	Workspace        map[string]any `json:"workspace,omitempty"`
	RepairRan        bool           `json:"repair_ran"`
	RepairActions    []string       `json:"repair_actions,omitempty"`
	IssueCodes       []string       `json:"issue_codes,omitempty"`
	Summary          string         `json:"summary"`
}

type AttentionOptions struct {
	Actor contracts.Actor
	Limit int
}

type AttentionItem struct {
	WorkspaceID string `json:"workspace_id"`
	Path        string `json:"path,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	TicketID    string `json:"ticket_id,omitempty"`
	Project     string `json:"project,omitempty"`
	Title       string `json:"title,omitempty"`
	Category    string `json:"category"`
	Reason      string `json:"reason,omitempty"`
	Health      Health `json:"health,omitempty"`
}

type AttentionReport struct {
	Kind    string            `json:"kind"`
	Items   []AttentionItem   `json:"items"`
	Missing []WorkspaceRecord `json:"unavailable,omitempty"`
}

type SearchOptions struct {
	Query string
	Limit int
}

type SearchHit struct {
	WorkspaceID string `json:"workspace_id"`
	TicketID    string `json:"ticket_id"`
	Project     string `json:"project"`
	Title       string `json:"title"`
	Status      string `json:"status"`
}

type SearchReport struct {
	Kind  string      `json:"kind"`
	Query string      `json:"query"`
	Hits  []SearchHit `json:"hits"`
}

type ServiceSettings struct {
	Bind      string `json:"bind"`
	Port      int    `json:"port"`
	Enabled   bool   `json:"enabled"`
	AutoStart bool   `json:"auto_start"`
}

type BrowserSettings struct {
	OpenHome bool `json:"open_home"`
}

type AgentSettings struct {
	AutoInstall bool `json:"auto_install"`
}

type DiscoverySettings struct {
	Enabled  bool     `json:"enabled"`
	Roots    []string `json:"roots"`
	MaxDepth int      `json:"max_depth"`
}

type HomeSettings struct {
	ShowHidden bool `json:"show_hidden"`
}

type MachineSettings struct {
	Format           string            `json:"format"`
	InstanceID       string            `json:"instance_id"`
	Service          ServiceSettings   `json:"service"`
	Browser          BrowserSettings   `json:"browser"`
	AutoRegister     bool              `json:"auto_register"`
	Agents           AgentSettings     `json:"agents"`
	DefaultProject   bool              `json:"default_project"`
	LocalCheckpoints bool              `json:"local_checkpoints"`
	Discovery        DiscoverySettings `json:"discovery"`
	Home             HomeSettings      `json:"home"`
	GitMode          GitMode           `json:"git_mode"`
}

type MachineSettingsPatch struct {
	Service          *ServiceSettings
	Browser          *BrowserSettings
	AutoRegister     *bool
	Agents           *AgentSettings
	DefaultProject   *bool
	LocalCheckpoints *bool
	Discovery        *DiscoverySettings
	Home             *HomeSettings
	GitMode          *GitMode
}

type ServiceStatus struct {
	Kind       string `json:"kind"`
	Running    bool   `json:"running"`
	Reused     bool   `json:"reused"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	URL        string `json:"url"`
	ClaimURL   string `json:"claim_url,omitempty"`
	InstanceID string `json:"instance_id"`
	PID        int    `json:"pid,omitempty"`
	IdentityOK bool   `json:"identity_ok"`
	Detail     string `json:"detail,omitempty"`
}

type ServiceOptions struct {
	OpenBrowser bool
	Foreground  bool
}

// Workspace is the canonical bound workspace. Actions and Queries remain
// the authoritative mutation/query services.
type Workspace struct {
	Root    string
	ID      string
	Actions *service.ActionService
	Queries *service.QueryService
	Locks   service.WriteLockManager
	closeFn func() error
}

func (w *Workspace) Close() error {
	if w == nil || w.closeFn == nil {
		return nil
	}
	return w.closeFn()
}

const (
	DefaultHomePort      = 7432
	DefaultHomeBind      = "127.0.0.1"
	settingsFormat       = "atlas_machine_settings_v1"
	registryFormatV2     = "atlas_workspace_registry_v2"
	GlobalMCPSubcommand  = "mcp"
	GlobalMCPServe       = "serve"
	GlobalMCPFlag        = "--global"
	GlobalMCPProfileFlag = "--tool-profile"
	GlobalMCPProfile     = "workflow"
	GlobalMCPServerName  = "atlas-tasker"
	PathGrantInit        = "init"
	PathGrantRegister    = "register"
	PathGrantRepair      = "repair"
	HomeLaunchdLabel     = "com.atlas-tasker.home"
	HomeSystemdService   = "atlas-home.service"
	HomeServiceMarker    = "Atlas-owned Home service. Do not edit by hand."
)
