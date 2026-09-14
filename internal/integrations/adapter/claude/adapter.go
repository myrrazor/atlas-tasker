package claude

import (
	"fmt"
	"os"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter/host"
)

func New() *host.Adapter {
	return host.New(integrations.TargetClaude, configure)
}

func configure(bc *host.BuildContext) error {
	if bc.Reg == nil {
		return nil
	}
	if bc.Scope.Scope == adapter.ScopeProjectShared {
		return configureSharedJSON(bc)
	}
	if bc.Detection.ExecutablePath == "" {
		bc.Warnings = append(bc.Warnings, "claude CLI is not installed; skill was refreshed and MCP registration is deferred")
		return nil
	}
	if host.UnmanagedSameName(bc.Detection, bc.Reg.ServerName) {
		return fmt.Errorf("existing unmanaged Claude server %s at a higher-precedence scope", bc.Reg.ServerName)
	}
	scopeFlag := "local"
	if bc.Scope.Scope == adapter.ScopeUser {
		scopeFlag = "user"
	}
	add := adapter.Command{
		Purpose:    adapter.CommandPurposeRegister,
		Executable: bc.Detection.ExecutablePath,
		Args:       append([]string{"mcp", "add", bc.Reg.ServerName, "--scope", scopeFlag, "--"}, append([]string{bc.Reg.Command}, bc.Reg.Args...)...),
		Timeout:    30 * time.Second,
	}
	remove := adapter.Command{
		Purpose:    adapter.CommandPurposeRemove,
		Executable: bc.Detection.ExecutablePath,
		Args:       []string{"mcp", "remove", bc.Reg.ServerName},
		Timeout:    15 * time.Second,
	}
	if err := add.Validate(); err != nil {
		return err
	}
	if err := remove.Validate(); err != nil {
		return err
	}
	already := false
	for _, existing := range bc.Detection.ExistingServers {
		if existing.Name == bc.Reg.ServerName && existing.AtlasOwned {
			already = true
		}
	}
	if !already {
		host.AddCommandStep(bc, "claude-mcp-add", "register Atlas with claude mcp add", add, &remove)
	}
	bc.Resulting = adapter.StateConfiguredUnverified
	return nil
}

func configureSharedJSON(bc *host.BuildContext) error {
	path, ok := bc.Scope.ResolvePath(bc.Input.WorkspaceRoot, bc.Input.Home)
	if !ok {
		return fmt.Errorf("cannot resolve Claude .mcp.json")
	}
	if host.IsSymlinkPath(path, os.Lstat) {
		return fmt.Errorf("refusing symlinked Claude .mcp.json")
	}
	existing, err := host.ReadExisting(path)
	if err != nil {
		return err
	}
	entry := adapter.StandardServerEntry{Type: adapter.RegistrationTransportStdio, Command: bc.Reg.Command, Args: append([]string(nil), bc.Reg.Args...)}
	merged, err := host.MergeJSONServer(existing, bc.Reg.ServerName, entry)
	if err != nil {
		return err
	}
	if string(existing) != string(merged) {
		host.AddConfigFile(bc, "claude-mcp-json", "write explicit Claude project .mcp.json entry", path, withNL(merged), len(existing) == 0)
	}
	bc.ConfigPath = path
	bc.NativeFP = host.NativeFingerprint(merged)
	bc.Resulting = adapter.StatePendingWorkspaceTrust
	bc.Approvals = []adapter.ApprovalStep{{
		Requirement: adapter.ApprovalWorkspaceTrustThenMCPApprov,
		Instruction: "Trust the workspace and approve the Atlas server in Claude Code. Atlas does not bypass Pending approval or disabledMcpjsonServers.",
	}}
	return nil
}

func withNL(raw []byte) []byte {
	if len(raw) == 0 || raw[len(raw)-1] == '\n' {
		return raw
	}
	return append(append([]byte(nil), raw...), '\n')
}
