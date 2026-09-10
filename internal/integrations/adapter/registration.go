package adapter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

// Canonical Atlas MCP registration constants (plan AT114-201, locked decision
// 3). Setup registers exactly this shape for every named provider; anything
// wider (delivery/admin profiles, high-impact tools) stays a separate explicit
// power-user operation.
const (
	RegistrationTransportStdio  = "stdio"
	RegistrationToolProfile     = "workflow"
	RegistrationMaxItems        = 30
	RegistrationMaxResultBytes  = 65536
	ServerNamePrefix            = "atlas-"
	serverNameDigestHexLength   = 12
	PortableExecutableName      = "tracker"
	flagWorkspace               = "--workspace"
	flagWorkspaceFromCwd        = "--workspace-from-cwd"
	flagExpectedWorkspaceID     = "--expected-workspace-id"
	flagToolProfile             = "--tool-profile"
	flagMaxItems                = "--max-items"
	flagMaxResultBytes          = "--max-result-bytes"
	flagDangerousHighImpactTool = "--dangerously-allow-high-impact-tools"
	flagInitIfMissing           = "--init-if-missing"
	flagReadOnly                = "--read-only"
)

var serverNamePattern = regexp.MustCompile(`^atlas-[0-9a-f]{12}$`)

// ServerNameFor derives the provider-neutral server name from the workspace ID
// (never from a project name or path). The name is stable for the life of the
// workspace identity, safe as a TOML table key, JSON key, and CLI argument, and
// leaks neither the path nor the raw identity.
func ServerNameFor(workspaceID string) (string, error) {
	trimmed := strings.TrimSpace(workspaceID)
	if trimmed == "" || trimmed != workspaceID {
		return "", fmt.Errorf("workspace id is required and must not carry surrounding whitespace")
	}
	sum := sha256.Sum256([]byte(trimmed))
	return ServerNamePrefix + hex.EncodeToString(sum[:])[:serverNameDigestHexLength], nil
}

// WorkspaceBindingKind says how the registered server learns which workspace
// it serves. Every kind also carries --expected-workspace-id so a moved,
// copied, or replaced workspace fails closed instead of answering.
type WorkspaceBindingKind string

const (
	// WorkspaceBindingAbsolutePath passes the canonical workspace root
	// literally. Only valid in machine-local configuration.
	WorkspaceBindingAbsolutePath WorkspaceBindingKind = "absolute_path"
	// WorkspaceBindingClientVariable passes a client-expanded placeholder such
	// as ${workspaceFolder}; the expanded value must still match the expected
	// workspace ID at startup.
	WorkspaceBindingClientVariable WorkspaceBindingKind = "client_variable"
	// WorkspaceBindingVerifiedCwd resolves the workspace from the server's
	// working directory and refuses anything but the expected workspace.
	WorkspaceBindingVerifiedCwd WorkspaceBindingKind = "verified_cwd"
)

func (k WorkspaceBindingKind) IsValid() bool {
	switch k {
	case WorkspaceBindingAbsolutePath, WorkspaceBindingClientVariable, WorkspaceBindingVerifiedCwd:
		return true
	default:
		return false
	}
}

// WorkspaceBinding pairs a binding kind with the value it needs.
type WorkspaceBinding struct {
	Kind WorkspaceBindingKind `json:"kind"`
	// WorkspaceRoot is the canonical absolute root for absolute_path bindings.
	WorkspaceRoot string `json:"workspace_root,omitempty"`
	// Placeholder is the client variable for client_variable bindings, e.g.
	// "${workspaceFolder}".
	Placeholder string `json:"placeholder,omitempty"`
}

// clientVariablePattern accepts the documented client placeholders, including
// Cursor's ${workspaceFolder} and Claude Code's ${CLAUDE_PROJECT_DIR:-.}
// default form.
var clientVariablePattern = regexp.MustCompile(`^\$\{[A-Za-z][A-Za-z0-9_.:\-]*\}$`)

func (b WorkspaceBinding) Validate() error {
	if !b.Kind.IsValid() {
		return fmt.Errorf("invalid workspace binding kind: %q", b.Kind)
	}
	switch b.Kind {
	case WorkspaceBindingAbsolutePath:
		if b.WorkspaceRoot == "" || !filepath.IsAbs(b.WorkspaceRoot) || filepath.Clean(b.WorkspaceRoot) != b.WorkspaceRoot {
			return fmt.Errorf("absolute_path binding requires a clean absolute workspace_root")
		}
		if b.Placeholder != "" {
			return fmt.Errorf("absolute_path binding must not carry a placeholder")
		}
	case WorkspaceBindingClientVariable:
		if !clientVariablePattern.MatchString(b.Placeholder) {
			return fmt.Errorf("client_variable binding requires a ${name} placeholder, got %q", b.Placeholder)
		}
		if b.WorkspaceRoot != "" {
			return fmt.Errorf("client_variable binding must not embed a workspace_root")
		}
	case WorkspaceBindingVerifiedCwd:
		if b.WorkspaceRoot != "" || b.Placeholder != "" {
			return fmt.Errorf("verified_cwd binding carries neither workspace_root nor placeholder")
		}
	}
	return nil
}

// MCPRegistration is the provider-neutral registration every adapter renders
// into its client's native form. Args are derived, never hand-assembled; a
// registration whose Args differ from the derivation fails validation.
type MCPRegistration struct {
	ServerName     string           `json:"server_name"`
	Transport      string           `json:"transport"`
	Command        string           `json:"command"`
	Args           []string         `json:"args"`
	ActorHint      contracts.Actor  `json:"actor_hint,omitempty"`
	WorkspaceID    string           `json:"workspace_id"`
	Binding        WorkspaceBinding `json:"binding"`
	ToolProfile    string           `json:"tool_profile"`
	MaxItems       int              `json:"max_items"`
	MaxResultBytes int              `json:"max_result_bytes"`
	// Portable registrations name the executable without a path so the entry
	// can travel with a repository; they must use the verified_cwd binding.
	Portable bool `json:"portable,omitempty"`
}

// NewRegistration builds the canonical registration for a workspace. command
// is the absolute tracker executable for machine-local registrations or
// PortableExecutableName when portable is true.
func NewRegistration(command string, workspaceID string, binding WorkspaceBinding, actorHint contracts.Actor, portable bool) (MCPRegistration, error) {
	name, err := ServerNameFor(workspaceID)
	if err != nil {
		return MCPRegistration{}, err
	}
	reg := MCPRegistration{
		ServerName:     name,
		Transport:      RegistrationTransportStdio,
		Command:        command,
		ActorHint:      actorHint,
		WorkspaceID:    workspaceID,
		Binding:        binding,
		ToolProfile:    RegistrationToolProfile,
		MaxItems:       RegistrationMaxItems,
		MaxResultBytes: RegistrationMaxResultBytes,
		Portable:       portable,
	}
	reg.Args = RegistrationArgs(workspaceID, binding)
	if err := reg.Validate(); err != nil {
		return MCPRegistration{}, err
	}
	return reg, nil
}

// RegistrationArgs derives the exact server argv for a binding. Every binding
// pins the expected workspace ID; result bounds and the workflow profile are
// fixed.
func RegistrationArgs(workspaceID string, binding WorkspaceBinding) []string {
	args := []string{"mcp", "serve"}
	switch binding.Kind {
	case WorkspaceBindingAbsolutePath:
		args = append(args, flagWorkspace, binding.WorkspaceRoot)
	case WorkspaceBindingClientVariable:
		args = append(args, flagWorkspace, binding.Placeholder)
	case WorkspaceBindingVerifiedCwd:
		args = append(args, flagWorkspaceFromCwd)
	}
	args = append(args,
		flagExpectedWorkspaceID, workspaceID,
		flagToolProfile, RegistrationToolProfile,
		flagMaxItems, strconv.Itoa(RegistrationMaxItems),
		flagMaxResultBytes, strconv.Itoa(RegistrationMaxResultBytes),
	)
	return args
}

var forbiddenRegistrationArgs = []string{flagDangerousHighImpactTool, flagInitIfMissing, flagReadOnly}

func (r MCPRegistration) Validate() error {
	if !serverNamePattern.MatchString(r.ServerName) {
		return fmt.Errorf("server name %q must be derived with ServerNameFor", r.ServerName)
	}
	expectedName, err := ServerNameFor(r.WorkspaceID)
	if err != nil {
		return err
	}
	if expectedName != r.ServerName {
		return fmt.Errorf("server name %q does not match workspace id", r.ServerName)
	}
	if r.Transport != RegistrationTransportStdio {
		return fmt.Errorf("local integrations use stdio transport only, got %q", r.Transport)
	}
	if r.Portable {
		if r.Command != PortableExecutableName {
			return fmt.Errorf("portable registrations must name %q without a path", PortableExecutableName)
		}
		if r.Binding.Kind != WorkspaceBindingVerifiedCwd {
			return fmt.Errorf("portable registrations must use the verified_cwd binding")
		}
	} else if err := validateAbsoluteExecutable(r.Command); err != nil {
		return fmt.Errorf("registration command: %w", err)
	}
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if r.ActorHint != "" && !r.ActorHint.IsValid() {
		return fmt.Errorf("invalid actor hint: %s", r.ActorHint)
	}
	if r.ToolProfile != RegistrationToolProfile {
		return fmt.Errorf("setup registers the %s profile only, got %q", RegistrationToolProfile, r.ToolProfile)
	}
	if r.MaxItems != RegistrationMaxItems || r.MaxResultBytes != RegistrationMaxResultBytes {
		return fmt.Errorf("result bounds are fixed at --max-items %d --max-result-bytes %d", RegistrationMaxItems, RegistrationMaxResultBytes)
	}
	for _, arg := range r.Args {
		for _, forbidden := range forbiddenRegistrationArgs {
			if arg == forbidden {
				return fmt.Errorf("registration args must not contain %s", forbidden)
			}
		}
		if containsCredentialURL(arg) {
			return fmt.Errorf("registration args must not embed credentials")
		}
	}
	expected := RegistrationArgs(r.WorkspaceID, r.Binding)
	if len(expected) != len(r.Args) {
		return fmt.Errorf("registration args must equal the derived argv (%d args, got %d)", len(expected), len(r.Args))
	}
	for i := range expected {
		if expected[i] != r.Args[i] {
			return fmt.Errorf("registration arg %d must be %q, got %q", i, expected[i], r.Args[i])
		}
	}
	return nil
}

// ValidateForScope adds the placement rules: repository-carried configuration
// may not embed a machine-specific absolute workspace path or an executable
// under the user's home directory.
func (r MCPRegistration) ValidateForScope(scope ConfigScope, home string) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if !scope.IsValid() {
		return fmt.Errorf("invalid config scope: %q", scope)
	}
	if !scope.RepositoryCarried() {
		return nil
	}
	if r.Binding.Kind == WorkspaceBindingAbsolutePath {
		return fmt.Errorf("%s scope is repository-carried and cannot embed an absolute workspace path", scope)
	}
	if !r.Portable && home != "" && isWithin(home, r.Command) {
		return fmt.Errorf("%s scope is repository-carried and cannot reference an executable under the home directory", scope)
	}
	return nil
}

// ServeCommand is the exact process a verifier starts to self-probe the
// registration. executable resolves a portable registration; dir is the
// working directory for cwd-bound servers.
func (r MCPRegistration) ServeCommand(executable string, dir string, timeout time.Duration) (Command, error) {
	if executable == "" {
		executable = r.Command
	}
	cmd := Command{Purpose: CommandPurposeProbe, Executable: executable, Args: append([]string(nil), r.Args...), Dir: dir, Timeout: timeout}
	if err := cmd.Validate(); err != nil {
		return Command{}, err
	}
	return cmd, nil
}

// StandardServerEntry is the common JSON shape (Cursor, Claude .mcp.json,
// generic hosts) for one stdio server. Cursor documents type as required for
// stdio entries and Claude Code reads a typeless entry as stdio, so the
// explicit type is safe for both.
type StandardServerEntry struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// StandardConfig renders {"mcpServers": {<name>: {command, args}}}.
type StandardConfig struct {
	MCPServers map[string]StandardServerEntry `json:"mcpServers"`
}

// StandardConfigJSON renders the registration in the shared mcpServers form
// with stable indentation. It never includes env.
func (r MCPRegistration) StandardConfigJSON() ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	cfg := StandardConfig{MCPServers: map[string]StandardServerEntry{r.ServerName: {Type: RegistrationTransportStdio, Command: r.Command, Args: append([]string(nil), r.Args...)}}}
	return json.MarshalIndent(cfg, "", "  ")
}

// Fingerprint identifies the Atlas-owned entry so removal and repair can prove
// ownership before touching client configuration.
func (r MCPRegistration) Fingerprint() string {
	raw, _ := json.Marshal(struct {
		Name    string   `json:"name"`
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}{r.ServerName, r.Command, r.Args})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func isWithin(root string, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
