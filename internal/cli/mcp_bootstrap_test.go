package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	atlasmcp "github.com/myrrazor/atlas-tasker/internal/mcp"
	"github.com/spf13/cobra"
)

func bootstrapCommand(t *testing.T, workspace string, explicit bool) *cobra.Command {
	t.Helper()
	cmd, _, err := NewRootCommand().Find([]string{"mcp", "serve"})
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("init-if-missing", "true"); err != nil {
		t.Fatal(err)
	}
	if explicit {
		if err := cmd.Flags().Set("workspace", workspace); err != nil {
			t.Fatal(err)
		}
	}
	return cmd
}

func TestMCPBootstrapInitializesOnlyExplicitWorkspaceAndPreservesIt(t *testing.T) {
	withTempWorkspace(t)
	workspace := t.TempDir()
	cmd := bootstrapCommand(t, workspace, true)
	root, err := prepareMCPWorkspace(cmd, atlasmcp.Options{Profile: atlasmcp.ProfileWorkflow})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(".tracker"); !os.IsNotExist(err) {
		t.Fatalf("bootstrap wrote to client cwd: %v", err)
	}
	config := filepath.Join(root, ".tracker", "config.toml")
	before, err := os.Stat(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepareMCPWorkspace(cmd, atlasmcp.Options{Profile: atlasmcp.ProfileWorkflow}); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(config)
	if err != nil || before.ModTime() != after.ModTime() {
		t.Fatalf("reopen rewrote initialized config: %v", err)
	}
	for _, path := range []string{"projects", ".gitignore", ".tracker/templates/task.md"} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Fatalf("missing bootstrap artifact %s: %v", path, err)
		}
	}
}

func TestMCPBootstrapRejectsImplicitWorkspaceAndReadProfiles(t *testing.T) {
	for _, tc := range []struct {
		name, workspace string
		explicit        bool
		options         atlasmcp.Options
	}{
		{"implicit", "", false, atlasmcp.Options{Profile: atlasmcp.ProfileWorkflow}},
		{"blank", " ", true, atlasmcp.Options{Profile: atlasmcp.ProfileWorkflow}},
		{"relative", "repo", true, atlasmcp.Options{Profile: atlasmcp.ProfileWorkflow}},
		{"read", "", true, atlasmcp.Options{Profile: atlasmcp.ProfileRead}},
		{"read-only", "", true, atlasmcp.Options{Profile: atlasmcp.ProfileWorkflow, ReadOnly: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workspace := t.TempDir()
			if tc.name == "read" || tc.name == "read-only" {
				tc.workspace = workspace
			}
			if _, err := prepareMCPWorkspace(bootstrapCommand(t, tc.workspace, tc.explicit), tc.options); err == nil {
				t.Fatal("unsafe bootstrap accepted")
			}
			if entries, err := os.ReadDir(workspace); err != nil || len(entries) != 0 {
				t.Fatalf("refusal wrote artifacts: %v %v", entries, err)
			}
		})
	}
}

func TestMCPBootstrapPreflightsAllOutputsBeforeWriting(t *testing.T) {
	for _, target := range []string{".tracker", "projects", ".gitignore"} {
		for _, redirect := range []bool{true, false} {
			t.Run(target+map[bool]string{true: "/symlink", false: "/wrong-type"}[redirect], func(t *testing.T) {
				workspace, outside := t.TempDir(), t.TempDir()
				sentinel := filepath.Join(outside, "sentinel")
				if err := os.WriteFile(sentinel, []byte("preserve"), 0o644); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(workspace, target)
				if redirect {
					dest := outside
					if target == ".gitignore" {
						dest = sentinel
					}
					if err := os.Symlink(dest, path); err != nil {
						t.Fatal(err)
					}
				} else if target == ".gitignore" {
					if err := os.Mkdir(path, 0o755); err != nil {
						t.Fatal(err)
					}
				} else if err := os.WriteFile(path, []byte("preserve"), 0o644); err != nil {
					t.Fatal(err)
				}
				if _, err := prepareMCPWorkspace(bootstrapCommand(t, workspace, true), atlasmcp.Options{Profile: atlasmcp.ProfileWorkflow}); err == nil {
					t.Fatal("invalid destination accepted")
				}
				if entries, err := os.ReadDir(workspace); err != nil || len(entries) != 1 {
					t.Fatalf("partial bootstrap writes: %v %v", entries, err)
				}
				if entries, err := os.ReadDir(outside); err != nil || len(entries) != 1 {
					t.Fatalf("bootstrap escaped workspace: %v %v", entries, err)
				}
				if raw, err := os.ReadFile(sentinel); err != nil || string(raw) != "preserve" {
					t.Fatalf("outside content changed: %q %v", raw, err)
				}
			})
		}
	}
}

func TestMCPBootstrapRejectsNestedAndMissingWorkspace(t *testing.T) {
	parent := t.TempDir()
	if _, err := ensureInitArtifacts(parent); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(parent, "child")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareMCPWorkspace(bootstrapCommand(t, nested, true), atlasmcp.Options{Profile: atlasmcp.ProfileWorkflow}); err == nil || !strings.Contains(err.Error(), "inside existing Atlas workspace") {
		t.Fatalf("nested bootstrap: %v", err)
	}
	if entries, err := os.ReadDir(nested); err != nil || len(entries) != 0 {
		t.Fatalf("nested writes: %v %v", entries, err)
	}
	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := prepareMCPWorkspace(bootstrapCommand(t, missing, true), atlasmcp.Options{Profile: atlasmcp.ProfileWorkflow}); err == nil {
		t.Fatal("missing root accepted")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("created missing root: %v", err)
	}
}
