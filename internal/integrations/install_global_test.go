package integrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallOpenClawGlobalRejectsSymlinkDestination(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	outside := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".openclaw", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(home, ".openclaw", "skills", "atlas-worker")); err != nil {
		t.Fatal(err)
	}
	_, err := (Installer{Root: root}).InstallOpts(TargetOpenClaw, InstallOptions{Global: true})
	if err == nil {
		t.Fatal("expected global openclaw install to reject symlink destination")
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("symlink target should stay empty, got %v", entries)
	}
}

func TestInstallOpenClawGlobalWritesUnderHomeSkills(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	result, err := (Installer{Root: root}).InstallOpts(TargetOpenClaw, InstallOptions{Global: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.GlobalSkillFiles) == 0 {
		t.Fatal("expected global skill files")
	}
	if resolved, err := filepath.EvalSymlinks(home); err == nil {
		home = resolved
	}
	rootSkills := filepath.Join(home, ".openclaw", "skills", "atlas-worker")
	for _, path := range result.GlobalSkillFiles {
		rel, err := filepath.Rel(rootSkills, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			t.Fatalf("global file escaped home skill root: %s", path)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing global file %s: %v", path, err)
		}
	}
}
