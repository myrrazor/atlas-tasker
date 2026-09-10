package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	atlasmcp "github.com/myrrazor/atlas-tasker/internal/mcp"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/spf13/cobra"
)

func prepareMCPWorkspace(cmd *cobra.Command, options atlasmcp.Options) (string, error) {
	initialize, _ := cmd.Flags().GetBool("init-if-missing")
	if !initialize {
		return requestedWorkspaceRoot(cmd)
	}
	raw, _ := cmd.Flags().GetString("workspace")
	if !cmd.Flags().Changed("workspace") || strings.TrimSpace(raw) == "" || !filepath.IsAbs(raw) {
		return "", apperr.New(apperr.CodeInvalidInput, "--init-if-missing requires an explicit absolute --workspace directory")
	}
	if options.ReadOnly || options.Profile == atlasmcp.ProfileRead {
		return "", apperr.New(apperr.CodeInvalidInput, "--init-if-missing requires a write-capable tool profile and cannot be used with --read-only")
	}
	root, err := service.CanonicalWorkspaceRoot(raw)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", apperr.Wrap(apperr.CodeInvalidInput, err, "inspect bootstrap workspace: %v", err)
	}
	if !info.IsDir() {
		return "", apperr.New(apperr.CodeInvalidInput, "bootstrap workspace must be an existing directory")
	}
	marker := filepath.Join(root, ".tracker")
	info, err = os.Lstat(marker)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", apperr.New(apperr.CodeInvalidInput, ".tracker must be a real directory for MCP bootstrap")
		}
		// Do not re-run init: it would rewrite an existing workspace's config.
		return service.InitializedWorkspaceRoot(root)
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	if err := refuseNestedWorkspaceInit(root); err != nil {
		return "", err
	}
	// Check every existing top-level init destination before the first write.
	// .tracker is absent here, so it cannot contain redirected child outputs.
	for _, output := range []struct {
		name      string
		directory bool
	}{{"projects", true}, {".gitignore", false}} {
		path := filepath.Join(root, output.name)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		validType := info.IsDir()
		if !output.directory {
			validType = info.Mode().IsRegular()
		}
		if info.Mode()&os.ModeSymlink != 0 || !validType {
			return "", apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("MCP bootstrap refuses redirected or invalid output %s", output.name))
		}
	}
	if _, err := ensureInitArtifacts(root); err != nil {
		return "", err
	}
	return service.InitializedWorkspaceRoot(root)
}
