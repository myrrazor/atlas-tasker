package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"gopkg.in/yaml.v3"
)

func TestEffectiveWorktreeConfigHonorsExplicitFalseFlags(t *testing.T) {
	var cfg contracts.WorktreeConfig
	if err := yaml.Unmarshal([]byte("enabled: false\ndefault_mode: per_run\nrequire_clean_main: false\n"), &cfg); err != nil {
		t.Fatalf("unmarshal worktree config: %v", err)
	}

	got := effectiveWorktreeConfig(cfg)
	if got.Enabled {
		t.Fatalf("expected explicit enabled=false to stay false, got %#v", got)
	}
	if got.RequireCleanMain {
		t.Fatalf("expected explicit require_clean_main=false to stay false, got %#v", got)
	}
}

func TestEffectiveWorktreeConfigKeepsDefaultsForPartialOverrides(t *testing.T) {
	var cfg contracts.WorktreeConfig
	if err := yaml.Unmarshal([]byte("root: .atlas-worktrees\n"), &cfg); err != nil {
		t.Fatalf("unmarshal worktree config: %v", err)
	}

	got := effectiveWorktreeConfig(cfg)
	if !got.Enabled || got.DefaultMode != contracts.WorktreeModePerRun || !got.RequireCleanMain {
		t.Fatalf("expected partial override to keep defaults, got %#v", got)
	}
}

func TestDispatchRepoDirtyBlockerHonorsRequireCleanMainFalse(t *testing.T) {
	root := t.TempDir()
	initGitRepo(t, root)
	writeFile(t, filepath.Join(root, "README.md"), "# atlas\n")
	gitRun(t, root, "add", "README.md")
	gitRun(t, root, "commit", "-m", "init")
	writeFile(t, filepath.Join(root, "dirty.txt"), "drift\n")

	blocked, err := dispatchRepoDirtyBlocker(context.Background(), root, contracts.Project{Key: "APP"})
	if err != nil {
		t.Fatalf("dispatch dirty check with defaults: %v", err)
	}
	if !blocked {
		t.Fatal("expected dirty repo to block dispatch under default policy")
	}

	var cfg contracts.WorktreeConfig
	if err := yaml.Unmarshal([]byte("require_clean_main: false\n"), &cfg); err != nil {
		t.Fatalf("unmarshal worktree config: %v", err)
	}
	blocked, err = dispatchRepoDirtyBlocker(context.Background(), root, contracts.Project{
		Key:      "APP",
		Defaults: contracts.ProjectDefaults{Worktrees: cfg},
	})
	if err != nil {
		t.Fatalf("dispatch dirty check with require_clean_main=false: %v", err)
	}
	if blocked {
		t.Fatal("expected require_clean_main=false to allow dispatch on a dirty repo")
	}
}

func TestDispatchRepoDirtyBlockerIgnoresAtlasWorkspaceFiles(t *testing.T) {
	root := t.TempDir()
	initGitRepo(t, root)
	writeFile(t, filepath.Join(root, "README.md"), "# atlas\n")
	gitRun(t, root, "add", "README.md")
	gitRun(t, root, "commit", "-m", "init")
	if err := os.MkdirAll(filepath.Join(root, ".tracker", "runtime", "run_1"), 0o755); err != nil {
		t.Fatalf("mkdir tracker runtime: %v", err)
	}
	writeFile(t, filepath.Join(root, ".tracker", "runtime", "run_1", "brief.md"), "derived\n")

	blocked, err := dispatchRepoDirtyBlocker(context.Background(), root, contracts.Project{Key: "APP"})
	if err != nil {
		t.Fatalf("dispatch dirty check with atlas files: %v", err)
	}
	if blocked {
		t.Fatal("expected atlas-managed files to stay out of dirty-repo checks")
	}
}
