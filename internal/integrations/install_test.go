package integrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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
	if !strings.Contains(string(skill), "name: atlas-worker") || !strings.Contains(string(skill), "tracker agent available <agent-id> --json") || !strings.Contains(string(skill), "tracker run dispatch <ID> --agent agent:<agent-id>") {
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
	// modern Claude Code skill location with the reference alongside
	skill, err := os.ReadFile(filepath.Join(root, ".claude", "skills", "atlas-worker", "SKILL.md"))
	if err != nil {
		t.Fatalf("read claude skill: %v", err)
	}
	if !strings.Contains(string(skill), "name: atlas-worker") {
		t.Fatalf("unexpected claude skill frontmatter: %s", string(skill))
	}
	if _, err := os.ReadFile(filepath.Join(root, ".claude", "skills", "atlas-worker", "references", "workflow.md")); err != nil {
		t.Fatalf("claude skill should ship its workflow reference: %v", err)
	}
}

func TestSkillContentTeachesBootstrapAndWakeups(t *testing.T) {
	skill := atlasWorkerSkill("claude")
	for _, needle := range []string{"tracker team list", "tracker agent wakeups"} {
		if !strings.Contains(skill, needle) {
			t.Fatalf("skill should mention %q:\n%s", needle, skill)
		}
	}
	reference := atlasWorkerReference()
	for _, needle := range []string{"tracker team apply", "wakeups ack", "agent.work_available"} {
		if !strings.Contains(reference, needle) {
			t.Fatalf("reference should mention %q:\n%s", needle, reference)
		}
	}
}

func TestInstallGenericCreatesPortableSkillPack(t *testing.T) {
	root := t.TempDir()
	result, err := Installer{Root: root}.Install(TargetGeneric, false)
	if err != nil {
		t.Fatalf("install generic: %v", err)
	}
	if !strings.HasSuffix(result.InstructionFile, "AGENTS.md") {
		t.Fatalf("unexpected instruction file: %#v", result)
	}
	body, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if !strings.Contains(string(body), genericMarkers.begin) || !strings.Contains(string(body), "tracker agent available <agent-id> --json") {
		t.Fatalf("unexpected AGENTS.md body: %s", string(body))
	}
	skill, err := os.ReadFile(filepath.Join(root, ".tracker", "integrations", "atlas-agent-skill", "SKILL.md"))
	if err != nil {
		t.Fatalf("read generic skill: %v", err)
	}
	if !strings.Contains(string(skill), "Atlas Worker") || !strings.Contains(string(skill), "dependency_blocked") || !strings.Contains(string(skill), "tracker run dispatch <ID> --agent agent:<agent-id>") {
		t.Fatalf("unexpected generic skill: %s", string(skill))
	}
}

func TestInstallCursorAndGrokWriteAgentsBlocks(t *testing.T) {
	root := t.TempDir()
	for _, target := range []Target{TargetCursor, TargetGrok} {
		if _, err := (Installer{Root: root}).Install(target, false); err != nil {
			t.Fatalf("install %s: %v", target, err)
		}
	}
	body, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	content := string(body)
	for _, needle := range []string{cursorMarkers.begin, grokMarkers.begin, "Atlas Tasker (Cursor)", "Atlas Tasker (Grok)"} {
		if !strings.Contains(content, needle) {
			t.Fatalf("AGENTS.md missing %q:\n%s", needle, content)
		}
	}
	if _, err := os.ReadFile(filepath.Join(root, ".cursor", "skills", "atlas-worker", "SKILL.md")); err != nil {
		t.Fatalf("cursor skill missing: %v", err)
	}
}

// The frontmatter is the whole activation contract: agents match on it before they
// ever read the body, and a description with a bare ": " in it is not a YAML scalar.
func TestSkillFrontmatterParses(t *testing.T) {
	for _, provider := range []string{"codex", "claude", "openclaw", "generic", "cursor", "grok"} {
		body := atlasWorkerSkill(provider)
		_, rest, found := strings.Cut(body, "---\n")
		if !found {
			t.Fatalf("%s skill has no frontmatter:\n%s", provider, body)
		}
		frontmatter, _, found := strings.Cut(rest, "\n---")
		if !found {
			t.Fatalf("%s skill frontmatter is unterminated:\n%s", provider, body)
		}
		var parsed struct {
			Name        string         `yaml:"name"`
			Description string         `yaml:"description"`
			Metadata    map[string]any `yaml:"metadata"`
		}
		if err := yaml.Unmarshal([]byte(frontmatter), &parsed); err != nil {
			t.Fatalf("%s skill frontmatter is not valid YAML: %v\n%s", provider, err, frontmatter)
		}
		if parsed.Name != "atlas-worker" {
			t.Fatalf("%s skill name is %q", provider, parsed.Name)
		}
		// the description is what gets matched, so it has to carry trigger phrases
		for _, trigger := range []string{"what should I work on", "ready for review", "Atlas Tasker"} {
			if !strings.Contains(parsed.Description, trigger) {
				t.Fatalf("%s skill description is missing %q:\n%s", provider, trigger, parsed.Description)
			}
		}
		if provider == "openclaw" && parsed.Metadata["openclaw"] == nil {
			t.Fatalf("openclaw skill should carry its gating metadata:\n%s", frontmatter)
		}
	}
}

func TestInstallOpenClawUsesRepoLocalSkillRoot(t *testing.T) {
	root := t.TempDir()
	result, err := Installer{Root: root}.Install(TargetOpenClaw, false)
	if err != nil {
		t.Fatalf("install openclaw: %v", err)
	}
	if !strings.HasSuffix(result.InstructionFile, "AGENTS.md") {
		t.Fatalf("unexpected instruction file: %#v", result)
	}
	skillPath := filepath.Join(root, ".agents", "skills", "atlas-worker", "SKILL.md")
	skill, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("read openclaw skill: %v", err)
	}
	content := string(skill)
	if !strings.Contains(content, "name: atlas-worker") {
		t.Fatalf("unexpected skill frontmatter: %s", content)
	}
	if !strings.Contains(content, `"requires": { "bins": ["tracker"] }`) {
		t.Fatalf("openclaw skill should gate on the tracker binary: %s", content)
	}
	if !strings.Contains(content, "tracker agent available <agent-id> --json") {
		t.Fatalf("openclaw skill should reuse the shared worker body: %s", content)
	}
	if _, err := os.ReadFile(filepath.Join(root, ".agents", "skills", "atlas-worker", "references", "workflow.md")); err != nil {
		t.Fatalf("openclaw skill should ship its workflow reference: %v", err)
	}
	if _, err := os.ReadFile(filepath.Join(root, ".agents", "skills", "atlas-worker", "commands", "atlas-take.md")); err != nil {
		t.Fatalf("openclaw skill should ship its command templates: %v", err)
	}
	// Compare resolved roots because macOS temp paths include system symlinks.
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	// ~/.openclaw/skills is the user's to manage; a repo command must not write there
	for _, path := range append(append([]string{}, result.Created...), result.Updated...) {
		rel, err := filepath.Rel(canonicalRoot, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			t.Fatalf("install wrote outside the workspace: %s", path)
		}
	}
	guide, err := os.ReadFile(filepath.Join(root, ".tracker", "integrations", "openclaw-guide.md"))
	if err != nil {
		t.Fatalf("read openclaw guide: %v", err)
	}
	for _, needle := range []string{"openclaw skills list", "--global", filepath.Join(".agents", "skills", "atlas-worker")} {
		if !strings.Contains(string(guide), needle) {
			t.Fatalf("openclaw guide should mention %q:\n%s", needle, string(guide))
		}
	}
}

func TestCodexAndOpenClawKeepSeparateAgentsBlocks(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(path, []byte("# House rules\n\nRun the tests.\n"), 0o644); err != nil {
		t.Fatalf("seed AGENTS.md: %v", err)
	}
	for _, target := range []Target{TargetCodex, TargetOpenClaw} {
		if _, err := (Installer{Root: root}).Install(target, false); err != nil {
			t.Fatalf("install %s: %v", target, err)
		}
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	content := string(body)
	for _, needle := range []string{"# House rules", "Atlas Tasker (Codex)", "Atlas Tasker (OpenClaw)", openclawMarkers.begin, openclawMarkers.end} {
		if !strings.Contains(content, needle) {
			t.Fatalf("AGENTS.md lost %q after both installs:\n%s", needle, content)
		}
	}

	// re-running either target must rewrite only its own block
	if _, err := (Installer{Root: root}).Install(TargetCodex, false); err != nil {
		t.Fatalf("reinstall codex: %v", err)
	}
	body, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("re-read AGENTS.md: %v", err)
	}
	content = string(body)
	if strings.Count(content, managedBegin) != 1 || strings.Count(content, openclawMarkers.begin) != 1 {
		t.Fatalf("expected exactly one block per target:\n%s", content)
	}
	if !strings.Contains(content, "Atlas Tasker (OpenClaw)") {
		t.Fatalf("codex reinstall clobbered the openclaw block:\n%s", content)
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
