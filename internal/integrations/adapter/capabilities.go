package adapter

import (
	"fmt"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
)

// ConfigScope says where a client keeps one MCP server definition.
type ConfigScope string

const (
	// ScopeProjectLocal is machine-local configuration bound to one project
	// path (Claude Code's "local" scope inside ~/.claude.json).
	ScopeProjectLocal ConfigScope = "project_local"
	// ScopeProjectShared is a file inside the workspace that travels with the
	// repository (.codex/config.toml, .cursor/mcp.json, .grok/config.toml,
	// .mcp.json).
	ScopeProjectShared ConfigScope = "project_shared"
	// ScopeUser is the user-global client configuration.
	ScopeUser ConfigScope = "user"
	// ScopeGateway is a client-managed registry outside the workspace
	// (OpenClaw's mcp.servers entries).
	ScopeGateway ConfigScope = "gateway"
)

func (s ConfigScope) IsValid() bool {
	switch s {
	case ScopeProjectLocal, ScopeProjectShared, ScopeUser, ScopeGateway:
		return true
	default:
		return false
	}
}

// RepositoryCarried reports whether files in this scope may be committed and
// therefore must not embed machine-specific absolute paths or credentials.
func (s ConfigScope) RepositoryCarried() bool { return s == ScopeProjectShared }

// ConfigFormat is the on-disk format Atlas must parse and preserve.
type ConfigFormat string

const (
	ConfigFormatTOML          ConfigFormat = "toml"
	ConfigFormatJSON          ConfigFormat = "json"
	ConfigFormatClientManaged ConfigFormat = "client_managed"
	ConfigFormatNone          ConfigFormat = "none"
)

// WriteMethod says how Atlas is allowed to change configuration in a scope.
type WriteMethod string

const (
	// WriteMethodAtlasFileEdit means Atlas edits the file itself: parse,
	// merge one managed entry, validate, replace atomically.
	WriteMethodAtlasFileEdit WriteMethod = "atlas_file_edit"
	// WriteMethodClientCLI means Atlas only writes through the client's own
	// CLI (structured Command), never by editing the client's file.
	WriteMethodClientCLI WriteMethod = "client_cli"
	// WriteMethodPortableOnly means Atlas renders a descriptor for the user to
	// place; it never writes into the client's configuration.
	WriteMethodPortableOnly WriteMethod = "portable_only"
)

// MCPSupport is how the client consumes MCP server definitions.
type MCPSupport string

const (
	MCPSupportNativeConfigFile   MCPSupport = "native_config_file"
	MCPSupportClientCLI          MCPSupport = "client_cli"
	MCPSupportPortableDescriptor MCPSupport = "portable_descriptor"
)

// VerificationMethod is one way to prove a registration works.
type VerificationMethod string

const (
	// VerificationSelfProbe starts the exact registered command and performs
	// MCP initialize, tools/list, and a read tool call.
	VerificationSelfProbe VerificationMethod = "self_probe"
	// VerificationClientCLIList uses the client's list command (codex mcp
	// list, claude mcp list, grok mcp list --json, openclaw mcp list --json).
	VerificationClientCLIList VerificationMethod = "client_cli_list"
	// VerificationClientCLIGet uses a per-server detail command (claude mcp
	// get, codex mcp get, openclaw mcp show --json).
	VerificationClientCLIGet VerificationMethod = "client_cli_get"
	// VerificationClientCLIDoctor uses a client diagnostic that opens a live
	// connection (openclaw mcp doctor --probe, grok mcp doctor --json).
	VerificationClientCLIDoctor VerificationMethod = "client_cli_doctor"
	// VerificationConformanceHost uses Atlas's own test MCP host to prove a
	// portable descriptor is loadable.
	VerificationConformanceHost VerificationMethod = "conformance_host"
	// VerificationManualClientCheck means the user confirms in the client UI;
	// it never upgrades a state on its own.
	VerificationManualClientCheck VerificationMethod = "manual_client_check"
)

// ApprovalRequirement names the human gate a scope may impose before the
// server is usable. Atlas reports these; it never performs them.
type ApprovalRequirement string

const (
	ApprovalNone                        ApprovalRequirement = "none"
	ApprovalWorkspaceTrust              ApprovalRequirement = "workspace_trust"
	ApprovalMCPApproval                 ApprovalRequirement = "mcp_approval"
	ApprovalWorkspaceTrustThenMCPApprov ApprovalRequirement = "workspace_trust_then_mcp_approval"
	ApprovalAdminPolicy                 ApprovalRequirement = "admin_policy"
)

// PendingState maps an approval requirement to the state setup reports while
// the human step is outstanding.
func (a ApprovalRequirement) PendingState() (State, bool) {
	switch a {
	case ApprovalWorkspaceTrust, ApprovalWorkspaceTrustThenMCPApprov:
		return StatePendingWorkspaceTrust, true
	case ApprovalMCPApproval, ApprovalAdminPolicy:
		return StatePendingMCPApproval, true
	default:
		return "", false
	}
}

// RestartRequirement says what the client needs before it sees a new server.
type RestartRequirement string

const (
	RestartNone          RestartRequirement = "none"
	RestartNewSession    RestartRequirement = "new_session"
	RestartClientRestart RestartRequirement = "client_restart"
	RestartGatewayReload RestartRequirement = "gateway_reload"
	RestartUnverified    RestartRequirement = "unverified"
)

// Support is a tri-state fact: claimed only when verified against official
// documentation or a real client.
type Support string

const (
	SupportYes        Support = "supported"
	SupportNo         Support = "unsupported"
	SupportUnverified Support = "unverified"
)

// ScopeCapability describes one configuration scope of one client.
type ScopeCapability struct {
	Scope       ConfigScope          `json:"scope"`
	Path        string               `json:"path"`
	Format      ConfigFormat         `json:"format"`
	WriteMethod WriteMethod          `json:"write_method"`
	Binding     WorkspaceBindingKind `json:"binding"`
	Approval    ApprovalRequirement  `json:"approval"`
	Restart     RestartRequirement   `json:"restart"`
	Notes       string               `json:"notes,omitempty"`
}

// Capabilities is one row of the six-target capability matrix. Every field is
// either verified against the cited official documentation or listed under
// Unverified for Sprint 114.2 real-client confirmation.
type Capabilities struct {
	Target           integrations.Target  `json:"target"`
	DisplayName      string               `json:"display_name"`
	ClientExecutable string               `json:"client_executable,omitempty"`
	VersionArgs      []string             `json:"version_args,omitempty"`
	InstructionFile  string               `json:"instruction_file"`
	SkillDir         string               `json:"skill_dir"`
	MCPSupport       MCPSupport           `json:"mcp_support"`
	Scopes           []ScopeCapability    `json:"scopes"`
	PreferredScope   ConfigScope          `json:"preferred_scope"`
	Verification     []VerificationMethod `json:"verification"`
	MCPApps          Support              `json:"mcp_apps"`
	SafeRemoval      Support              `json:"safe_removal"`
	AlsoLoads        []string             `json:"also_loads,omitempty"`
	MaxPlannedState  State                `json:"max_planned_state"`
	VersionPolicy    string               `json:"version_policy"`
	Sources          []string             `json:"sources"`
	Unverified       []string             `json:"unverified,omitempty"`
}

// Preferred returns the preferred scope's capability row.
func (c Capabilities) Preferred() (ScopeCapability, bool) {
	for _, scope := range c.Scopes {
		if scope.Scope == c.PreferredScope {
			return scope, true
		}
	}
	return ScopeCapability{}, false
}

// Validate enforces the matrix invariants every row must satisfy.
func (c Capabilities) Validate() error {
	if !isKnownTarget(c.Target) {
		return fmt.Errorf("unknown target %q", c.Target)
	}
	if c.DisplayName == "" || c.InstructionFile == "" || c.SkillDir == "" {
		return fmt.Errorf("%s: display name, instruction file, and skill dir are required", c.Target)
	}
	if len(c.Sources) == 0 {
		return fmt.Errorf("%s: at least one official source is required", c.Target)
	}
	if len(c.Scopes) == 0 {
		return fmt.Errorf("%s: at least one scope is required", c.Target)
	}
	if _, ok := c.Preferred(); !ok {
		return fmt.Errorf("%s: preferred scope %q is not in scopes", c.Target, c.PreferredScope)
	}
	for _, scope := range c.Scopes {
		if !scope.Scope.IsValid() || !scope.Binding.IsValid() {
			return fmt.Errorf("%s: scope %q has invalid scope or binding", c.Target, scope.Scope)
		}
		if scope.Scope.RepositoryCarried() && scope.Binding == WorkspaceBindingAbsolutePath {
			return fmt.Errorf("%s: repository-carried scope %q cannot use an absolute path binding", c.Target, scope.Scope)
		}
		if scope.WriteMethod == WriteMethodAtlasFileEdit && scope.Format == ConfigFormatClientManaged {
			return fmt.Errorf("%s: scope %q is client-managed and cannot be edited by Atlas", c.Target, scope.Scope)
		}
	}
	if !c.MaxPlannedState.IsValid() {
		return fmt.Errorf("%s: invalid max planned state %q", c.Target, c.MaxPlannedState)
	}
	if c.Target == integrations.TargetGeneric && c.MaxPlannedState != StatePortableReady {
		return fmt.Errorf("generic target must cap planned state at portable_ready")
	}
	if c.Target != integrations.TargetGeneric && (c.ClientExecutable == "" || len(c.VersionArgs) == 0) {
		return fmt.Errorf("%s: named clients need an executable and a version probe", c.Target)
	}
	if len(c.Verification) == 0 {
		return fmt.Errorf("%s: at least one verification method is required", c.Target)
	}
	return nil
}

func isKnownTarget(target integrations.Target) bool {
	for _, candidate := range integrations.DetectableTargets() {
		if candidate == target {
			return true
		}
	}
	return false
}

// Matrix returns the verified capability matrix for all six targets in
// integrations.DetectableTargets order.
func Matrix() []Capabilities {
	return []Capabilities{claudeCapabilities(), codexCapabilities(), cursorCapabilities(), openclawCapabilities(), grokCapabilities(), genericCapabilities()}
}

// CapabilitiesFor returns one row.
func CapabilitiesFor(target integrations.Target) (Capabilities, error) {
	for _, row := range Matrix() {
		if row.Target == target {
			return row, nil
		}
	}
	return Capabilities{}, fmt.Errorf("unsupported integration target: %s", target)
}

func codexCapabilities() Capabilities {
	return Capabilities{
		Target:           integrations.TargetCodex,
		DisplayName:      "Codex",
		ClientExecutable: "codex",
		VersionArgs:      []string{"--version"},
		InstructionFile:  "AGENTS.md",
		SkillDir:         ".codex/skills/atlas-worker",
		MCPSupport:       MCPSupportNativeConfigFile,
		Scopes: []ScopeCapability{
			{
				Scope: ScopeProjectShared, Path: ".codex/config.toml", Format: ConfigFormatTOML, WriteMethod: WriteMethodAtlasFileEdit,
				Binding: WorkspaceBindingVerifiedCwd, Approval: ApprovalWorkspaceTrust, Restart: RestartNewSession,
				Notes: "Loaded only for trusted projects (projects.<path>.trust_level). Managed [mcp_servers.<name>] table with required = false so a broken Atlas server never blocks Codex. cwd is a documented key but is not embedded because the file is repository-carried.",
			},
			{
				Scope: ScopeUser, Path: "~/.codex/config.toml", Format: ConfigFormatTOML, WriteMethod: WriteMethodClientCLI,
				Binding: WorkspaceBindingAbsolutePath, Approval: ApprovalNone, Restart: RestartNewSession,
				Notes: "codex mcp add/list/get/remove operate on the user-level file; shared by Codex CLI, IDE extension, and the ChatGPT desktop app on the same host. Not selected by default because it would expose one workspace to every project.",
			},
		},
		PreferredScope:  ScopeProjectShared,
		Verification:    []VerificationMethod{VerificationSelfProbe, VerificationClientCLIList, VerificationClientCLIGet},
		MCPApps:         SupportUnverified,
		SafeRemoval:     SupportYes,
		MaxPlannedState: StateConnected,
		VersionPolicy:   "codex --version must parse; the adapter pins the verified range from Sprint 114.2 real-client runs and reports unsupported_client_version otherwise.",
		Sources: []string{
			"https://learn.chatgpt.com/docs/extend/mcp",
			"https://learn.chatgpt.com/docs/config-reference",
		},
		Unverified: []string{
			"Effective precedence when the same server name exists in user and project files (official reference calls project files overrides; third-party sources disagree).",
			"Whether a running Codex session reloads .codex/config.toml without a new session.",
			"MCP Apps rendering.",
		},
	}
}

func claudeCapabilities() Capabilities {
	return Capabilities{
		Target:           integrations.TargetClaude,
		DisplayName:      "Claude Code",
		ClientExecutable: "claude",
		VersionArgs:      []string{"--version"},
		InstructionFile:  "CLAUDE.md",
		SkillDir:         ".claude/skills/atlas-worker",
		MCPSupport:       MCPSupportClientCLI,
		Scopes: []ScopeCapability{
			{
				Scope: ScopeProjectLocal, Path: "~/.claude.json", Format: ConfigFormatClientManaged, WriteMethod: WriteMethodClientCLI,
				Binding: WorkspaceBindingAbsolutePath, Approval: ApprovalNone, Restart: RestartNewSession,
				Notes: "claude mcp add --scope local (default). Stored per project path in the user's own file, so an absolute workspace path is appropriate. Highest precedence scope.",
			},
			{
				Scope: ScopeProjectShared, Path: ".mcp.json", Format: ConfigFormatJSON, WriteMethod: WriteMethodAtlasFileEdit,
				Binding: WorkspaceBindingClientVariable, Approval: ApprovalWorkspaceTrustThenMCPApprov, Restart: RestartNewSession,
				Notes: "Explicit shared option only. Requires the workspace trust dialog and then per-server approval (⏸ Pending approval); disabledMcpjsonServers rejects. Use ${CLAUDE_PROJECT_DIR:-.} rather than a machine path.",
			},
			{
				Scope: ScopeUser, Path: "~/.claude.json", Format: ConfigFormatClientManaged, WriteMethod: WriteMethodClientCLI,
				Binding: WorkspaceBindingAbsolutePath, Approval: ApprovalNone, Restart: RestartNewSession,
				Notes: "claude mcp add --scope user. Not selected by default; lowest precedence.",
			},
		},
		PreferredScope:  ScopeProjectLocal,
		Verification:    []VerificationMethod{VerificationSelfProbe, VerificationClientCLIGet, VerificationClientCLIList},
		MCPApps:         SupportUnverified,
		SafeRemoval:     SupportYes,
		MaxPlannedState: StateConnected,
		VersionPolicy:   "claude --version must parse; documented behavior changes at 2.1.196 (trust-gated approvals), 2.1.202, 2.1.207, 2.1.219 bound the verified range set in Sprint 114.2.",
		Sources: []string{
			"https://code.claude.com/docs/en/mcp",
		},
		Unverified: []string{
			"Exact health-status text parsing of claude mcp list across versions.",
			"MCP Apps rendering.",
		},
	}
}

func cursorCapabilities() Capabilities {
	return Capabilities{
		Target:           integrations.TargetCursor,
		DisplayName:      "Cursor",
		ClientExecutable: "cursor",
		VersionArgs:      []string{"--version"},
		InstructionFile:  "AGENTS.md",
		SkillDir:         ".cursor/skills/atlas-worker",
		MCPSupport:       MCPSupportNativeConfigFile,
		Scopes: []ScopeCapability{
			{
				Scope: ScopeProjectShared, Path: ".cursor/mcp.json", Format: ConfigFormatJSON, WriteMethod: WriteMethodAtlasFileEdit,
				Binding: WorkspaceBindingClientVariable, Approval: ApprovalNone, Restart: RestartUnverified,
				Notes: "mcpServers.<name> with type stdio. ${workspaceFolder} is the folder containing .cursor/mcp.json and is resolved in command, args, env, url, headers. Enterprise MCP allowlists can block the server (admin_policy).",
			},
			{
				Scope: ScopeUser, Path: "~/.cursor/mcp.json", Format: ConfigFormatJSON, WriteMethod: WriteMethodAtlasFileEdit,
				Binding: WorkspaceBindingAbsolutePath, Approval: ApprovalNone, Restart: RestartUnverified,
				Notes: "Global configuration. Not selected by default.",
			},
		},
		PreferredScope:  ScopeProjectShared,
		Verification:    []VerificationMethod{VerificationSelfProbe, VerificationManualClientCheck},
		MCPApps:         SupportYes,
		SafeRemoval:     SupportYes,
		MaxPlannedState: StateConnected,
		VersionPolicy:   "cursor --version must parse; Sprint 114.2 pins the verified range.",
		Sources: []string{
			"https://cursor.com/docs/context/mcp",
		},
		Unverified: []string{
			"Whether Cursor picks up a new .cursor/mcp.json entry without restart (docs say custom servers need a restart after updates).",
			"Whether a project-scope server needs an explicit enable step in Customize.",
			"MCP App rendering for project-scope servers (known limitation noted in the plan).",
			"A Cursor CLI listing command usable for client_cli_list.",
		},
	}
}

func openclawCapabilities() Capabilities {
	return Capabilities{
		Target:           integrations.TargetOpenClaw,
		DisplayName:      "OpenClaw",
		ClientExecutable: "openclaw",
		VersionArgs:      []string{"--version"},
		InstructionFile:  "AGENTS.md",
		SkillDir:         ".agents/skills/atlas-worker",
		MCPSupport:       MCPSupportClientCLI,
		Scopes: []ScopeCapability{
			{
				Scope: ScopeGateway, Path: "mcp.servers.<name> (OpenClaw config)", Format: ConfigFormatClientManaged, WriteMethod: WriteMethodClientCLI,
				Binding: WorkspaceBindingAbsolutePath, Approval: ApprovalNone, Restart: RestartGatewayReload,
				Notes: "openclaw mcp add <name> --command <abs> --arg ... --cwd <workspace>; openclaw mcp unset <name> removes. Saved definition is distinct from a live probe (doctor --probe / probe). reload disposes runtimes for the current CLI process only; gateway/agent processes need their own reload or restart.",
			},
		},
		PreferredScope:  ScopeGateway,
		Verification:    []VerificationMethod{VerificationSelfProbe, VerificationClientCLIDoctor, VerificationClientCLIGet},
		MCPApps:         SupportUnverified,
		SafeRemoval:     SupportYes,
		MaxPlannedState: StateConnectedRestartRequired,
		VersionPolicy:   "openclaw --version must parse; Sprint 114.2 pins the verified range.",
		Sources: []string{
			"https://docs.openclaw.ai/cli/mcp",
		},
		Unverified: []string{
			"Exact --version output format.",
			"Whether the Gateway needs a restart or only a reload after add/unset.",
			"MCP Apps rendering.",
		},
	}
}

func grokCapabilities() Capabilities {
	return Capabilities{
		Target:           integrations.TargetGrok,
		DisplayName:      "Grok Build",
		ClientExecutable: "grok",
		VersionArgs:      []string{"version"},
		InstructionFile:  "AGENTS.md",
		SkillDir:         ".tracker/integrations/grok-agent-skill",
		MCPSupport:       MCPSupportClientCLI,
		Scopes: []ScopeCapability{
			{
				Scope: ScopeProjectShared, Path: ".grok/config.toml", Format: ConfigFormatTOML, WriteMethod: WriteMethodClientCLI,
				Binding: WorkspaceBindingVerifiedCwd, Approval: ApprovalNone, Restart: RestartNewSession,
				Notes: "grok mcp add --scope project <name> -- <command...> writes .grok/config.toml in the current directory; Grok walks from cwd up to the git root and a project server replaces a same-name user server entirely. ${VAR} expands in command/args/env at load time.",
			},
			{
				Scope: ScopeUser, Path: "~/.grok/config.toml", Format: ConfigFormatTOML, WriteMethod: WriteMethodClientCLI,
				Binding: WorkspaceBindingAbsolutePath, Approval: ApprovalNone, Restart: RestartNewSession,
				Notes: "Default scope of grok mcp add. Not selected by default.",
			},
		},
		PreferredScope:  ScopeProjectShared,
		Verification:    []VerificationMethod{VerificationSelfProbe, VerificationClientCLIDoctor, VerificationClientCLIList},
		MCPApps:         SupportUnverified,
		SafeRemoval:     SupportYes,
		AlsoLoads:       []string{"~/.claude.json", ".cursor/mcp.json", ".mcp.json"},
		MaxPlannedState: StateConnected,
		VersionPolicy:   "grok version must parse; Sprint 114.2 pins the verified range.",
		Sources: []string{
			"https://docs.x.ai/build/features/mcp-servers",
			"https://github.com/xai-org/grok-build/blob/main/crates/codegen/xai-grok-pager/docs/user-guide/05-configuration.md",
		},
		Unverified: []string{
			"Whether grok mcp remove accepts --scope project or removes from whichever file holds the entry.",
			"Working directory Grok gives a project-scoped stdio server (drives the verified_cwd binding).",
			"Whether a running session reloads after config edits without the TUI refresh.",
			"MCP Apps rendering.",
		},
	}
}

func genericCapabilities() Capabilities {
	return Capabilities{
		Target:          integrations.TargetGeneric,
		DisplayName:     "Generic agent",
		InstructionFile: "AGENTS.md",
		SkillDir:        ".tracker/integrations/generic-agent-skill",
		MCPSupport:      MCPSupportPortableDescriptor,
		Scopes: []ScopeCapability{
			{
				Scope: ScopeProjectShared, Path: ".tracker/integrations/atlas-mcp.json", Format: ConfigFormatJSON, WriteMethod: WriteMethodPortableOnly,
				Binding: WorkspaceBindingVerifiedCwd, Approval: ApprovalNone, Restart: RestartUnverified,
				Notes: "Portable mcpServers descriptor naming the tracker executable without a path. The user copies it into their client.",
			},
			{
				Scope: ScopeProjectShared, Path: ".mcp.json", Format: ConfigFormatJSON, WriteMethod: WriteMethodAtlasFileEdit,
				Binding: WorkspaceBindingVerifiedCwd, Approval: ApprovalNone, Restart: RestartUnverified,
				Notes: "Optional managed entry in the standard root file; only written when explicitly selected. Also read by Claude Code and Grok Build compatibility loading.",
			},
		},
		PreferredScope:  ScopeProjectShared,
		Verification:    []VerificationMethod{VerificationSelfProbe, VerificationConformanceHost},
		MCPApps:         SupportNo,
		SafeRemoval:     SupportYes,
		MaxPlannedState: StatePortableReady,
		VersionPolicy:   "No client to version. Never reported connected without a conformance-host or real-client probe.",
		Sources: []string{
			"https://modelcontextprotocol.io/specification/2025-11-25/server/tools",
		},
	}
}
