package cursor

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter/host"
)

func New() *host.Adapter {
	return host.New(integrations.TargetCursor, configure)
}

func configure(bc *host.BuildContext) error {
	if bc.Reg == nil {
		return nil
	}
	path, ok := bc.Scope.ResolvePath(bc.Input.WorkspaceRoot, bc.Input.Home)
	if !ok {
		return fmt.Errorf("cannot resolve Cursor config path")
	}
	if host.IsSymlinkPath(path, os.Lstat) || host.IsSymlinkPath(filepath.Dir(path), os.Lstat) {
		return fmt.Errorf("refusing symlinked Cursor config %s", path)
	}
	if host.UnmanagedSameName(bc.Detection, bc.Reg.ServerName) {
		return fmt.Errorf("existing unmanaged Cursor server %s", bc.Reg.ServerName)
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
	if string(existing) == string(merged) {
		bc.ConfigPath = path
		bc.NativeFP = host.NativeFingerprint(merged)
		bc.Resulting = adapter.StateConfiguredUnverified
		bc.Warnings = append(bc.Warnings, "Cursor may need a restart before it lists a new project MCP server.")
		return nil
	}
	host.AddConfigFile(bc, "cursor-mcp", "merge Atlas mcpServers entry into .cursor/mcp.json", path, withNL(merged), len(existing) == 0)
	bc.ConfigPath = path
	bc.NativeFP = host.NativeFingerprint(merged)
	bc.Resulting = adapter.StateConfiguredUnverified
	bc.Warnings = append(bc.Warnings, "Cursor may need a restart before it lists a new project MCP server. Markdown remains the primary board presentation.")
	return nil
}

func withNL(raw []byte) []byte {
	if len(raw) == 0 || raw[len(raw)-1] == '\n' {
		return raw
	}
	return append(append([]byte(nil), raw...), '\n')
}
