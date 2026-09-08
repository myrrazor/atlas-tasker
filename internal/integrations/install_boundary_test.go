package integrations

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallRejectsSymlinkDestinationsBeforeWriting(t *testing.T) {
	for _, rel := range []string{".agents/skills", ".agents/skills/atlas-worker/SKILL.md", "AGENTS.md", ".tracker/integrations"} {
		t.Run(rel, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			original := []byte("external fixture must be preserved\n")
			target := filepath.Join(outside, "existing.md")
			if err := os.WriteFile(target, original, 0o644); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			linkTarget := target
			if filepath.Ext(path) == "" {
				linkTarget = outside
			}
			if err := os.Symlink(linkTarget, path); err != nil {
				t.Fatal(err)
			}
			if _, err := (Installer{Root: root}).Install(TargetOpenClaw, false); err == nil {
				t.Fatal("symlink destination accepted")
			}
			got, err := os.ReadFile(target)
			if err != nil || !bytes.Equal(got, original) {
				t.Fatal("external fixture changed")
			}
			if _, err := os.Stat(filepath.Join(root, ".tracker", "integrations", "openclaw-guide.md")); !os.IsNotExist(err) {
				t.Fatalf("installer wrote files before rejecting destination: %v", err)
			}
		})
	}
}
