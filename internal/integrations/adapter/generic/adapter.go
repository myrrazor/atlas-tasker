package generic

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter/host"
)

func New() *host.Adapter {
	return host.New(integrations.TargetGeneric, configure)
}

func configure(bc *host.BuildContext) error {
	portable, err := adapter.NewRegistration(adapter.PortableExecutableName, bc.Input.WorkspaceID, adapter.WorkspaceBinding{Kind: adapter.WorkspaceBindingVerifiedCwd}, host.ActorHint(bc.Input), true)
	if err != nil {
		return err
	}
	if bc.Reg == nil {
		bc.Reg = &portable
	}
	descPath := filepath.Join(bc.Input.WorkspaceRoot, ".tracker", "integrations", "atlas-mcp.json")
	body, err := portable.StandardConfigJSON()
	if err != nil {
		return err
	}
	body = withNL(body)
	if current, _ := os.ReadFile(descPath); string(current) != string(body) {
		created := !fileOK(descPath)
		host.AddManagedFile(bc, "generic-portable", "write portable Atlas MCP descriptor", descPath, body, created)
	}
	bc.ConfigPath = descPath
	bc.NativeFP = host.NativeFingerprint(body)
	bc.Resulting = adapter.StatePortableReady

	rootMCP := filepath.Join(bc.Input.WorkspaceRoot, ".mcp.json")
	if bc.Scope.Scope == adapter.ScopeProjectShared && (fileOK(rootMCP) || wantsStandardMCP(bc)) {
		existing, err := host.ReadExisting(rootMCP)
		if err != nil {
			return err
		}
		entry := adapter.StandardServerEntry{Type: adapter.RegistrationTransportStdio, Command: bc.Reg.Command, Args: append([]string(nil), bc.Reg.Args...)}
		merged, err := host.MergeJSONServer(existing, bc.Reg.ServerName, entry)
		if err != nil {
			return err
		}
		if string(existing) != string(merged) {
			host.AddConfigFile(bc, "generic-mcp-json", "merge optional root .mcp.json Atlas entry", rootMCP, withNL(merged), len(existing) == 0)
		}
	}
	if custom := customPath(bc); custom != "" && bc.Scope.Scope == adapter.ScopeUser {
		if err := validateCustom(custom, bc); err != nil {
			return err
		}
		existing, err := host.ReadExisting(custom)
		if err != nil {
			return err
		}
		entry := adapter.StandardServerEntry{Type: adapter.RegistrationTransportStdio, Command: bc.Reg.Command, Args: append([]string(nil), bc.Reg.Args...)}
		merged, err := host.MergeJSONServer(existing, bc.Reg.ServerName, entry)
		if err != nil {
			return err
		}
		host.AddConfigFile(bc, "generic-custom", "merge Atlas entry into user-selected config", custom, withNL(merged), false)
		bc.ConfigPath = custom
		bc.Warnings = append(bc.Warnings, "generic custom destination is configured_unverified until the conformance host probes it")
	}
	return nil
}

func wantsStandardMCP(bc *host.BuildContext) bool {
	if bc.Input.Existing != nil && stringsHasSuffix(bc.Input.Existing.ConfigPath, ".mcp.json") {
		return true
	}
	return false
}

func customPath(bc *host.BuildContext) string {
	for _, root := range bc.Input.ConsentedRoots {
		if stringsHasSuffix(root, ".json") {
			return root
		}
		// consented root is a directory; look for an existing json? no — user names the file.
	}
	return ""
}

func validateCustom(path string, bc *host.BuildContext) error {
	if host.IsSymlinkPath(path, os.Lstat) {
		return fmt.Errorf("refusing symlinked custom config %s", path)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("custom config must already exist: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("custom config must be a regular file")
	}
	contained := false
	for _, root := range bc.Input.ConsentedRoots {
		rel, err := filepath.Rel(root, path)
		if err == nil && rel != ".." && !hasDotDot(rel) {
			contained = true
		}
		if path == root {
			contained = true
		}
	}
	if !contained {
		return fmt.Errorf("custom config %s is outside consented roots", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = host.MergeJSONServer(raw, "probe", adapter.StandardServerEntry{Type: "stdio", Command: "tracker", Args: []string{"mcp"}})
	if err != nil {
		return fmt.Errorf("custom config failed format validation: %w", err)
	}
	return nil
}

func fileOK(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func stringsHasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func hasDotDot(rel string) bool {
	return rel == ".." || len(rel) >= 3 && rel[:3] == ".."+string(os.PathSeparator)
}

func withNL(raw []byte) []byte {
	if len(raw) == 0 || raw[len(raw)-1] == '\n' {
		return raw
	}
	return append(append([]byte(nil), raw...), '\n')
}
