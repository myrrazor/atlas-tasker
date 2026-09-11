package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	atlasmcp "github.com/myrrazor/atlas-tasker/internal/mcp"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/setup"
	"github.com/spf13/cobra"
)

func prepareMCPWorkspace(cmd *cobra.Command, options atlasmcp.Options) (string, error) {
	fromCWD, _ := cmd.Flags().GetBool("workspace-from-cwd")
	expectedID, _ := cmd.Flags().GetString("expected-workspace-id")
	initialize, _ := cmd.Flags().GetBool("init-if-missing")
	workspaceFlag, _ := cmd.Flags().GetString("workspace")
	if fromCWD {
		if initialize {
			return "", apperr.New(apperr.CodeInvalidInput, "--workspace-from-cwd cannot be combined with --init-if-missing")
		}
		if cmd.Flags().Changed("workspace") && strings.TrimSpace(workspaceFlag) != "" {
			return "", apperr.New(apperr.CodeInvalidInput, "--workspace-from-cwd cannot be combined with --workspace")
		}
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		stateDir := mcpStateDir()
		resolved, err := setup.ResolveWorkspaceFromCWD(cwd, expectedID, stateDir)
		if err != nil {
			return "", err
		}
		return resolved.Root, nil
	}
	if !initialize {
		root, err := requestedWorkspaceRoot(cmd)
		if err != nil {
			return "", err
		}
		if err := setup.VerifyExpectedWorkspaceID(root, expectedID); err != nil {
			return "", err
		}
		return root, nil
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
		root, err = service.InitializedWorkspaceRoot(root)
		if err != nil {
			return "", err
		}
		if err := setup.VerifyExpectedWorkspaceID(root, expectedID); err != nil {
			return "", err
		}
		return root, nil
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
	root, err = service.InitializedWorkspaceRoot(root)
	if err != nil {
		return "", err
	}
	if err := setup.VerifyExpectedWorkspaceID(root, expectedID); err != nil {
		return "", err
	}
	return root, nil
}

func mcpStateDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir, err := setup.DefaultStateDir(home, os.Getenv)
	if err != nil {
		return ""
	}
	return dir
}
