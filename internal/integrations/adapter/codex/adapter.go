package codex

import (
	"fmt"
	"os"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter/host"
)

func New() *host.Adapter {
	return host.New(integrations.TargetCodex, configure)
}

func configure(bc *host.BuildContext) error {
	if bc.Reg == nil {
		return nil
	}
	if bc.Scope.WriteMethod != adapter.WriteMethodAtlasFileEdit {
		return fmt.Errorf("codex preferred setup edits .codex/config.toml")
	}
	path, ok := bc.Scope.ResolvePath(bc.Input.WorkspaceRoot, bc.Input.Home)
	if !ok {
		return fmt.Errorf("cannot resolve Codex config path")
	}
	if host.IsSymlinkPath(path, os.Lstat) {
		return fmt.Errorf("refusing symlinked Codex config %s", path)
	}
	if host.UnmanagedSameName(bc.Detection, bc.Reg.ServerName) {
		return fmt.Errorf("existing unmanaged Codex server %s", bc.Reg.ServerName)
	}
	existing, err := host.ReadExisting(path)
	if err != nil {
		return err
	}
	required := false
	merged, err := host.MergeTOMLServer(existing, bc.Reg.ServerName, []host.TOMLField{
		{Key: "command", Value: bc.Reg.Command},
		{Key: "args", Array: append([]string(nil), bc.Reg.Args...)},
		{Key: "required", Bool: &required},
		{Key: "default_tools_approval_mode", Value: "writes"},
	})
	if err != nil {
		return err
	}
	if string(existing) == string(merged) {
		bc.ConfigPath = path
		bc.NativeFP = host.NativeFingerprint(merged)
		bc.Resulting = adapter.StatePendingWorkspaceTrust
		bc.Approvals = []adapter.ApprovalStep{{
			Requirement: adapter.ApprovalWorkspaceTrust,
			Instruction: "Trust this project in Codex so .codex/config.toml is loaded.",
		}}
		return nil
	}
	created := len(existing) == 0
	host.AddConfigFile(bc, "codex-mcp", "write Codex [mcp_servers] entry", path, merged, created)
	bc.ConfigPath = path
	bc.NativeFP = host.NativeFingerprint(merged)
	bc.Resulting = adapter.StatePendingWorkspaceTrust
	bc.Approvals = []adapter.ApprovalStep{{
		Requirement: adapter.ApprovalWorkspaceTrust,
		Instruction: "Trust this project in Codex so .codex/config.toml is loaded. A broken Atlas server is required=false and will not block other Codex MCP use.",
	}}
	return nil
}
