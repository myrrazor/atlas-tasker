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
	if !strings.Contains(string(body), managedBegin) || !strings.Contains(string(body), "tracker agent available <agent-id> --json") {
		t.Fatalf("unexpected AGENTS.md body: %s", string(body))
	}
	guide, err := os.ReadFile(filepath.Join(root, ".tracker", "integrations", "codex-guide.md"))
	if err != nil {
		t.Fatalf("read guide: %v", err)
	}
	if !strings.Contains(string(guide), "tracker ticket claim <ID>") || !strings.Contains(string(guide), "tracker run launch <RUN-ID>") || !strings.Contains(string(guide), "tracker goal brief <ID> --md") || !strings.Contains(string(guide), "--type test_result") {
		t.Fatalf("unexpected guide content: %s", string(guide))
	}
	skill, err := os.ReadFile(filepath.Join(root, ".codex", "skills", "atlas-worker", "SKILL.md"))
	if err != nil {
		t.Fatalf("read codex skill: %v", err)
	}
	if !strings.Contains(string(skill), "name: atlas-worker") || !strings.Contains(string(skill), "tracker agent available <agent-id> --json") {
		t.Fatalf("unexpected skill content: %s", string(skill))
	}
	if len(result.SkillFiles) == 0 || len(result.CommandFiles) == 0 {
		t.Fatalf("install result should list skill and command files: %#v", result)
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
	if !strings.Contains(content, "tracker agent available <agent-id> --json") {
		t.Fatalf("updated managed block missing guidance: %s", content)
	}
	guide, err := os.ReadFile(filepath.Join(root, ".tracker", "integrations", "claude-guide.md"))
	if err != nil {
		t.Fatalf("read guide: %v", err)
	}
	if !strings.Contains(string(guide), "tracker run attach <RUN-ID> --provider claude --session-ref <session>") || !strings.Contains(string(guide), "tracker goal brief <ID> --md") {
		t.Fatalf("expected launch flow guidance, got: %s", string(guide))
	}
	command, err := os.ReadFile(filepath.Join(root, ".claude", "commands", "atlas-next.md"))
	if err != nil {
		t.Fatalf("read claude command: %v", err)
	}
	if !strings.Contains(string(command), "tracker agent pending <agent-id> --json") {
		t.Fatalf("unexpected claude command template: %s", string(command))
	}
}

func TestInstallGenericCreatesPortableSkillPack(t *testing.T) {
	root := t.TempDir()
	result, err := Installer{Root: root}.Install(TargetGeneric, false)
	if err != nil {
		t.Fatalf("install generic: %v", err)
	}
	if !strings.HasSuffix(result.InstructionFile, filepath.Join(".tracker", "integrations", "generic-agent-instructions.md")) {
		t.Fatalf("unexpected instruction file: %#v", result)
	}
	skill, err := os.ReadFile(filepath.Join(root, ".tracker", "integrations", "atlas-agent-skill", "SKILL.md"))
	if err != nil {
		t.Fatalf("read generic skill: %v", err)
	}
	if !strings.Contains(string(skill), "Atlas Worker") || !strings.Contains(string(skill), "dependency_blocked") {
		t.Fatalf("unexpected generic skill: %s", string(skill))
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
