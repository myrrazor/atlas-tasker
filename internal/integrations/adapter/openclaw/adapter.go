package openclaw

import (
	"fmt"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter/host"
)

func New() *host.Adapter {
	return host.New(integrations.TargetOpenClaw, configure)
}

func configure(bc *host.BuildContext) error {
	if bc.Reg == nil {
		return nil
	}
	if bc.Detection.ExecutablePath == "" {
		bc.Warnings = append(bc.Warnings, "openclaw CLI is not installed; skill was refreshed and gateway MCP registration is deferred")
		return nil
	}
	if host.UnmanagedSameName(bc.Detection, bc.Reg.ServerName) {
		return fmt.Errorf("existing OpenClaw server name collision %s", bc.Reg.ServerName)
	}
	args := []string{"mcp", "add", bc.Reg.ServerName, "--command", bc.Reg.Command}
	for _, arg := range bc.Reg.Args {
		args = append(args, "--arg", arg)
	}
	args = append(args, "--cwd", bc.Input.WorkspaceRoot)
	add := adapter.Command{
		Purpose:    adapter.CommandPurposeRegister,
		Executable: bc.Detection.ExecutablePath,
		Args:       args,
		Timeout:    30 * time.Second,
	}
	unset := adapter.Command{
		Purpose:    adapter.CommandPurposeRemove,
		Executable: bc.Detection.ExecutablePath,
		Args:       []string{"mcp", "unset", bc.Reg.ServerName},
		Timeout:    15 * time.Second,
	}
	if err := add.Validate(); err != nil {
		return err
	}
	host.AddCommandStep(bc, "openclaw-mcp-add", "register workspace-namespaced OpenClaw MCP server", add, &unset)
	bc.Resulting = adapter.StateConnectedRestartRequired
	bc.Warnings = append(bc.Warnings, "OpenClaw Gateway/agent processes need their own reload or restart after add; saved configuration is distinct from a live doctor --probe.")
	return nil
}
