package integrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallCodexCreatesManagedFiles(t *testing.T) {
	root := t.TempDir()
	result, err := Installer{Root: root}.Install(TargetCodex, false)
	if err != nil {
		t.Fatalf("install codex: %v", err)
	}
	if !strings.HasSuffix(result.InstructionFile, "AGENTS.md") {
		t.Fatalf("unexpected instruction file: %#v", result)
	}
	body, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if !strings.Contains(string(body), managedBegin) || !strings.Contains(string(body), "tracker queue --actor <actor> --json") {
		t.Fatalf("unexpected AGENTS.md body: %s", string(body))
	}
	guide, err := os.ReadFile(filepath.Join(root, ".tracker", "integrations", "codex-guide.md"))
	if err != nil {
		t.Fatalf("read guide: %v", err)
	}
	if !strings.Contains(string(guide), "tracker ticket claim <ID>") {
		t.Fatalf("unexpected guide content: %s", string(guide))
	}
}

func TestInstallClaudeReplacesOnlyManagedBlock(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "CLAUDE.md")
	original := "# Local Notes\n\nKeep this part.\n\n" + managedBegin + "\nold\n" + managedEnd + "\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("seed CLAUDE.md: %v", err)
	}
	if _, err := (Installer{Root: root}).Install(TargetClaude, false); err != nil {
		t.Fatalf("install claude: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	content := string(body)
	if !strings.Contains(content, "# Local Notes") || !strings.Contains(content, "Keep this part.") {
		t.Fatalf("non-managed content should survive: %s", content)
	}
	if strings.Contains(content, "\nold\n") {
		t.Fatalf("managed block should have been replaced: %s", content)
	}
	if !strings.Contains(content, "tracker review-queue --actor <actor> --json") {
		t.Fatalf("updated managed block missing guidance: %s", content)
	}
}

func TestInstallForceOverwritesInstructionFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(path, []byte("# keep me?\n"), 0o644); err != nil {
		t.Fatalf("seed AGENTS.md: %v", err)
	}
	if _, err := (Installer{Root: root}).Install(TargetCodex, true); err != nil {
		t.Fatalf("install codex with force: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	content := string(body)
	if strings.Contains(content, "keep me?") {
		t.Fatalf("force install should replace the whole file: %s", content)
	}
	if !strings.HasPrefix(content, managedBegin) {
		t.Fatalf("expected managed-only file after force install: %s", content)
	}
}
