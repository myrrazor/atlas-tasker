package integrations

import (
	"bytes"
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
	if !strings.Contains(string(body), "## Board display in chat") || !strings.Contains(string(body), "tracker board --style html") {
		t.Fatalf("workspace AGENTS.md did not teach chat board rendering: %s", string(body))
	}
	guide, err := os.ReadFile(filepath.Join(root, ".tracker", "integrations", "codex-guide.md"))
	if err != nil {
		t.Fatalf("read guide: %v", err)
	}
	if !strings.Contains(string(guide), "tracker ticket claim <ID>") || !strings.Contains(string(guide), "tracker run launch <RUN-ID>") || !strings.Contains(string(guide), "tracker goal brief <ID> --md") || !strings.Contains(string(guide), "--type test_result") {
		t.Fatalf("unexpected guide content: %s", string(guide))
	}
	skill, err := os.ReadFile(filepath.Join(root, ".agents", "skills", "atlas-worker", "SKILL.md"))
	if err != nil {
		t.Fatalf("read codex skill: %v", err)
	}
	if !strings.Contains(string(skill), "name: atlas-worker") || !strings.Contains(string(skill), "tracker agent available <agent-id> --json") || !strings.Contains(string(skill), "tracker run dispatch <ID> --agent agent:<agent-id>") {
		t.Fatalf("unexpected skill content: %s", string(skill))
	}
	if !strings.Contains(string(skill), "## Board display in chat") || !strings.Contains(string(skill), "tracker board --style html") {
		t.Fatalf("installed skill did not teach chat board rendering: %s", string(skill))
	}
	if _, err := os.Stat(filepath.Join(root, ".codex", "skills", "atlas-worker", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("fresh Codex install must not write the legacy .codex/skills root")
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

func TestInstallWritesUninstallCommand(t *testing.T) {
	root := t.TempDir()
	result, err := Installer{Root: root}.Install(TargetClaude, false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, path := range result.CommandFiles {
		if strings.HasSuffix(path, "atlas-uninstall.md") {
			found = true
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			text := string(body)
			if !strings.Contains(text, "tracker uninstall") || !strings.Contains(text, "backup") {
				t.Fatalf("uninstall command missing preserve copy:\n%s", text)
			}
		}
	}
	if !found {
		t.Fatalf("claude install missing atlas-uninstall.md: %#v", result.CommandFiles)
	}
}

func TestSkillContentTeachesBootstrapAndWakeups(t *testing.T) {
	skill := atlasWorkerSkill("claude")
	for _, needle := range []string{"tracker team list", "tracker agent wakeups", "atlas.context", "atlas.status"} {
		if !strings.Contains(skill, needle) {
			t.Fatalf("skill should mention %q:\n%s", needle, skill)
		}
	}
	reference := atlasWorkerReference()
	for _, needle := range []string{"tracker team apply", "wakeups ack", "agent.work_available", "atlas.context"} {
		if !strings.Contains(reference, needle) {
			t.Fatalf("reference should mention %q:\n%s", needle, reference)
		}
	}
}

func TestGrokSkillTeachesPortableMCPNames(t *testing.T) {
	skill := atlasWorkerSkill("grok")
	for _, needle := range []string{"atlas_status", "atlas_board", "atlas_context", "atlas.status"} {
		if !strings.Contains(skill, needle) {
			t.Fatalf("grok skill missing %q:\n%s", needle, skill)
		}
	}
	guide := grokGuide()
	if !strings.Contains(guide, "atlas_status") || !strings.Contains(guide, "--tool-name-style portable") {
		t.Fatalf("grok guide missing portable MCP names:\n%s", guide)
	}
}

func TestAllProviderSkillsShareManagedLifecycle(t *testing.T) {
	core := lifecycleSection(atlasWorkerSkill("claude"))
	if core == "" {
		t.Fatal("claude skill is missing the managed lifecycle markers")
	}
	needles := []string{
		"Prefer Atlas MCP tools",
		"atlas.context",
		"atlas.status",
		"Search first",
		"Use the ticket the user named",
		"Avoid duplicate work",
		"Claim before substantial edits",
		"legal workflow edge",
		"milestone progress",
		"durable evidence",
		"Request review or complete",
		"Query Atlas before every status report",
		"Reconcile Atlas state",
		"tracking_excluded",
		"follow_workspace",
		"dependency_blocked",
		"If MCP is unavailable",
	}
	for _, provider := range []string{"codex", "claude", "openclaw", "generic", "cursor", "grok"} {
		body := atlasWorkerSkill(provider)
		section := lifecycleSection(body)
		if section != core {
			t.Fatalf("%s lifecycle core differs from claude", provider)
		}
		if strings.Contains(body, "atlas-manager") || strings.Contains(body, "atlas-board") && !strings.Contains(body, "atlas.board") {
			t.Fatalf("%s invented a second skill name", provider)
		}
		if provider == "codex" || provider == "openclaw" {
			if !strings.Contains(body, "from coding-agent sessions that load project .agents/skills") {
				t.Fatalf("%s shared agents-root skill missing identity:\n%s", provider, body)
			}
		} else if !strings.Contains(body, "from "+skillProviderLabel(provider)) {
			t.Fatalf("%s skill should name its provider identity", provider)
		}
		for _, needle := range needles {
			if !strings.Contains(section, needle) {
				t.Fatalf("%s lifecycle missing %q", provider, needle)
			}
		}
	}
}

func lifecycleSection(body string) string {
	_, rest, ok := strings.Cut(body, "<!-- atlas-managed-lifecycle -->")
	if !ok {
		return ""
	}
	section, _, ok := strings.Cut(rest, "<!-- /atlas-managed-lifecycle -->")
	if !ok {
		return ""
	}
	return section
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
	skill, err := os.ReadFile(filepath.Join(root, ".tracker", "integrations", "generic-agent-skill", "SKILL.md"))
	if err != nil {
		t.Fatalf("read generic skill: %v", err)
	}
	if !strings.Contains(string(skill), "Atlas Worker") || !strings.Contains(string(skill), "dependency_blocked") || !strings.Contains(string(skill), "tracker run dispatch <ID> --agent agent:<agent-id>") {
		t.Fatalf("unexpected generic skill: %s", string(skill))
	}
}

func TestGenericAndGrokKeepDistinctSkillsAndPreserveLegacyPath(t *testing.T) {
	root := t.TempDir()
	legacyPath := filepath.Join(root, ".tracker", "integrations", "atlas-agent-skill", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("create legacy skill directory: %v", err)
	}
	const legacy = "custom legacy skill\n"
	if err := os.WriteFile(legacyPath, []byte(legacy), 0o644); err != nil {
		t.Fatalf("seed legacy skill: %v", err)
	}
	for _, target := range []Target{TargetGeneric, TargetGrok} {
		if _, err := (Installer{Root: root}).Install(target, false); err != nil {
			t.Fatalf("install %s: %v", target, err)
		}
	}
	legacyAfter, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("read legacy skill: %v", err)
	}
	if string(legacyAfter) != legacy {
		t.Fatalf("provider installs overwrote the legacy shared path: %q", string(legacyAfter))
	}
	genericSkill, err := os.ReadFile(filepath.Join(root, ".tracker", "integrations", "generic-agent-skill", "SKILL.md"))
	if err != nil {
		t.Fatalf("read generic skill: %v", err)
	}
	grokSkill, err := os.ReadFile(filepath.Join(root, ".grok", "skills", "atlas-worker", "SKILL.md"))
	if err != nil {
		t.Fatalf("read grok skill: %v", err)
	}
	if !strings.Contains(string(genericSkill), "from generic agent sessions") || !strings.Contains(string(grokSkill), "from Grok sessions") {
		t.Fatalf("provider-specific skills were not retained:\ngeneric=%s\ngrok=%s", genericSkill, grokSkill)
	}
}

func TestGeneratedIntegrationGuidanceUsesPortableWorkspacePaths(t *testing.T) {
	root := t.TempDir()
	installer := Installer{Root: root}
	for _, target := range []Target{TargetCodex, TargetClaude, TargetOpenClaw, TargetGeneric, TargetCursor, TargetGrok} {
		result, err := installer.Install(target, false)
		if err != nil {
			t.Fatalf("install %s: %v", target, err)
		}
		paths := append([]string{result.InstructionFile, result.GuideFile}, result.SkillFiles...)
		paths = append(paths, result.CommandFiles...)
		for _, path := range paths {
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read generated %s file %s: %v", target, path, err)
			}
			if strings.Contains(string(body), root) {
				t.Fatalf("%s guidance embeds temporary workspace root %q in %s:\n%s", target, root, path, body)
			}
		}
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
		if (provider == "openclaw" || provider == "codex") && parsed.Metadata["openclaw"] == nil {
			t.Fatalf("%s shared agents-root skill should carry OpenClaw gating metadata:\n%s", provider, frontmatter)
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
	// The default install remains repository-local; only explicit --global may write there.
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
	for _, needle := range []string{"openclaw skills list", "tracker integrations install openclaw --global", filepath.Join(".agents", "skills", "atlas-worker")} {
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

func TestInstallMigratesAtlasManagedLegacySkills(t *testing.T) {
	// Codex 0.144.5 still lists .codex/skills; removing the Atlas-managed copy
	// after writing .agents is canonicalization so the same skill is not listed
	// twice, not repair of a dead path.
	root := t.TempDir()
	legacyCodex := filepath.Join(root, ".codex", "skills", "atlas-worker", "SKILL.md")
	legacyGrok := filepath.Join(root, ".tracker", "integrations", "grok-agent-skill", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(legacyCodex), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyCodex, []byte(v115ProviderSkill("codex")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(legacyGrok), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyGrok, []byte(atlasWorkerSkill("grok")), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (Installer{Root: root}).Install(TargetCodex, false); err != nil {
		t.Fatal(err)
	}
	if _, err := (Installer{Root: root}).Install(TargetGrok, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "skills", "atlas-worker", "SKILL.md")); err != nil {
		t.Fatalf("codex native skill missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".grok", "skills", "atlas-worker", "SKILL.md")); err != nil {
		t.Fatalf("grok native skill missing: %v", err)
	}
	if _, err := os.Stat(legacyCodex); !os.IsNotExist(err) {
		t.Fatal("Atlas-managed legacy Codex skill should have been removed")
	}
	if _, err := os.Stat(legacyGrok); !os.IsNotExist(err) {
		t.Fatal("Atlas-managed legacy Grok skill should have been removed")
	}
}

func TestInstallKeepsCustomizedLegacySkillWithMarkers(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, ".codex", "skills", "atlas-worker", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	custom := v115ProviderSkill("codex") + "\n## House rule\nAlways run tracker inspect before claiming a ticket.\n"
	if err := os.WriteFile(legacy, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := (Installer{Root: root}).Install(TargetCodex, false)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != custom {
		t.Fatalf("customized leftover was rewritten:\n%s", got)
	}
	found := false
	for _, path := range result.Collisions {
		if filepath.Base(path) == "SKILL.md" && strings.Contains(path, string(filepath.Separator)+filepath.FromSlash(CodexLegacySkillDir)+string(filepath.Separator)) {
			found = true
		}
	}
	if !found {
		t.Fatalf("customized leftover must be reported as a collision: %+v", result.Collisions)
	}
	for _, path := range result.LegacyRemoved {
		if strings.Contains(path, string(filepath.Separator)+filepath.FromSlash(CodexLegacySkillDir)+string(filepath.Separator)) {
			t.Fatal("customized leftover must not be deleted")
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "skills", "atlas-worker", "SKILL.md")); err != nil {
		t.Fatal("canonical Codex skill missing")
	}
}

func TestInstallDoesNotFollowLegacySymlinks(t *testing.T) {
	t.Run("legacy-root", func(t *testing.T) {
		root, outside := t.TempDir(), t.TempDir()
		payload := []byte(v115ProviderSkill("codex"))
		outsideSkill := filepath.Join(outside, "SKILL.md")
		if err := os.WriteFile(outsideSkill, payload, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, ".codex", "skills"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(root, ".codex", "skills", "atlas-worker")); err != nil {
			t.Fatal(err)
		}
		if _, err := (Installer{Root: root}).Install(TargetCodex, false); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(outsideSkill)
		if err != nil || !bytes.Equal(got, payload) {
			t.Fatalf("outside fixture changed: %s %v", got, err)
		}
		info, err := os.Lstat(filepath.Join(root, ".codex", "skills", "atlas-worker"))
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatal("legacy-root symlink must be preserved")
		}
	})
	t.Run("ancestor", func(t *testing.T) {
		root, outside := t.TempDir(), t.TempDir()
		payload := []byte(v115ProviderSkill("codex"))
		outsideDir := filepath.Join(outside, "atlas-worker")
		if err := os.MkdirAll(outsideDir, 0o755); err != nil {
			t.Fatal(err)
		}
		outsideSkill := filepath.Join(outsideDir, "SKILL.md")
		if err := os.WriteFile(outsideSkill, payload, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, ".codex"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(root, ".codex", "skills")); err != nil {
			t.Fatal(err)
		}
		if _, err := (Installer{Root: root}).Install(TargetCodex, false); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(outsideSkill)
		if err != nil || !bytes.Equal(got, payload) {
			t.Fatalf("outside fixture changed: %s %v", got, err)
		}
		info, err := os.Lstat(filepath.Join(root, ".codex", "skills"))
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatal("ancestor symlink must be preserved")
		}
	})
}

func TestInstallPreservesUnmanagedSkillCollision(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".cursor", "skills", "atlas-worker", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	const custom = "this is my own atlas-worker, hands off\n"
	if err := os.WriteFile(path, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := (Installer{Root: root}).Install(TargetCursor, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Collisions) == 0 {
		t.Fatal("expected collision on unmanaged skill")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != custom {
		t.Fatalf("unmanaged skill was overwritten: %q", got)
	}
}

func TestCodexAndOpenClawShareAgentsRootSkill(t *testing.T) {
	root := t.TempDir()
	for _, target := range []Target{TargetCodex, TargetOpenClaw} {
		if _, err := (Installer{Root: root}).Install(target, false); err != nil {
			t.Fatalf("install %s: %v", target, err)
		}
	}
	shared := filepath.Join(root, ".agents", "skills", "atlas-worker", "SKILL.md")
	body, err := os.ReadFile(shared)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != atlasWorkerSkill("codex") || string(body) != atlasWorkerSkill("openclaw") {
		t.Fatal("shared SKILL.md must be identical for Codex and OpenClaw")
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "skills", "atlas-worker", "agents", "openai.yaml")); err != nil {
		t.Fatal("codex extras missing")
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "skills", "atlas-worker", "commands", "atlas-take.md")); err != nil {
		t.Fatal("openclaw extras missing")
	}
	agents, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(agents), "Atlas Tasker (Codex)") || !strings.Contains(string(agents), "Atlas Tasker (OpenClaw)") {
		t.Fatalf("expected both instruction blocks:\n%s", agents)
	}
}

func TestUninstallCodexKeepsSharedSkillWhileOpenClawRemains(t *testing.T) {
	root := t.TempDir()
	for _, target := range []Target{TargetCodex, TargetOpenClaw} {
		if _, err := (Installer{Root: root}).Install(target, false); err != nil {
			t.Fatal(err)
		}
	}
	shared := filepath.Join(root, ".agents", "skills", "atlas-worker", "SKILL.md")
	if !KeepSharedAgentsSkill(root, TargetCodex, shared) {
		t.Fatal("OpenClaw block is present; Codex uninstall must keep the shared SKILL.md")
	}
	if KeepSharedAgentsSkill(root, TargetCodex, filepath.Join(root, ".agents", "skills", "atlas-worker", "agents", "openai.yaml")) {
		t.Fatal("Codex-only extras must not be retained for OpenClaw")
	}
}
