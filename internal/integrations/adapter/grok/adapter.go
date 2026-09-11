package grok

import (
	"fmt"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter/host"
)

func New() *host.Adapter {
	return host.New(integrations.TargetGrok, configure)
}

func configure(bc *host.BuildContext) error {
	if bc.Reg == nil {
		return nil
	}
	for _, existing := range bc.Detection.ExistingServers {
		if strings.HasPrefix(existing.Name, adapter.ServerNamePrefix) {
			bc.Warnings = append(bc.Warnings, "Grok also loads "+existing.Name+" from a compatibility import; native project config takes precedence. Disable [compat.cursor] mcps or [compat.claude] mcps to avoid a duplicate Atlas catalog.")
		}
	}
	if bc.Detection.ExecutablePath == "" {
		bc.Warnings = append(bc.Warnings, "grok CLI is not installed; skill was refreshed and native project MCP registration is deferred")
		return nil
	}
	if host.UnmanagedSameName(bc.Detection, bc.Reg.ServerName) {
		return fmt.Errorf("existing unmanaged Grok server %s", bc.Reg.ServerName)
	}
	add := adapter.Command{
		Purpose:    adapter.CommandPurposeRegister,
		Executable: bc.Detection.ExecutablePath,
		Args:       append([]string{"mcp", "add", "--scope", "project", bc.Reg.ServerName, "--", bc.Reg.Command}, bc.Reg.Args...),
		Dir:        bc.Input.WorkspaceRoot,
		Timeout:    30 * time.Second,
	}
	remove := adapter.Command{
		Purpose:    adapter.CommandPurposeRemove,
		Executable: bc.Detection.ExecutablePath,
		Args:       []string{"mcp", "remove", bc.Reg.ServerName},
		Dir:        bc.Input.WorkspaceRoot,
		Timeout:    15 * time.Second,
	}
	if err := add.Validate(); err != nil {
		return err
	}
	host.AddCommandStep(bc, "grok-mcp-add", "register native Grok project MCP server", add, &remove)
	bc.Resulting = adapter.StateConfiguredUnverified
	return nil
}
