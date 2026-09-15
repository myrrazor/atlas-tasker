package integrations

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

const (
	// AgentsRootSkillDir is the repo-local skill root current Codex docs
	// advertise (.agents/skills). OpenClaw already used it. Cursor also
	// discovers it, but Atlas only writes Cursor's own copy under .cursor.
	AgentsRootSkillDir    = ".agents/skills/atlas-worker"
	ClaudeNativeSkillDir  = ".claude/skills/atlas-worker"
	CursorNativeSkillDir  = ".cursor/skills/atlas-worker"
	GrokNativeSkillDir    = ".grok/skills/atlas-worker"
	GenericNativeSkillDir = ".tracker/integrations/generic-agent-skill"

	// Codex CLI 0.144.5 still lists this root. Atlas writes .agents/skills as
	// the canonical copy and removes an Atlas-managed duplicate here so the
	// same skill is not listed twice. The path is not dead.
	CodexLegacySkillDir = ".codex/skills/atlas-worker"
	// Grok used to drop a generated skill under Atlas-owned integrations.
	GrokLegacySkillDir = ".tracker/integrations/grok-agent-skill"
)

const (
	atlasSkillNameLine     = "name: atlas-worker"
	atlasLifecycleBegin    = "<!-- atlas-managed-lifecycle -->"
	atlasLifecycleEnd      = "<!-- /atlas-managed-lifecycle -->"
	atlasWorkerReferenceH1 = "# Atlas Worker Reference"
	atlasWorkerDisplayName = "display_name: Atlas Worker"
)

// NativeSkillDir is the workspace-relative directory Atlas writes for a target.
func NativeSkillDir(target Target) string {
	switch target {
	case TargetCodex, TargetOpenClaw:
		return AgentsRootSkillDir
	case TargetClaude:
		return ClaudeNativeSkillDir
	case TargetCursor:
		return CursorNativeSkillDir
	case TargetGrok:
		return GrokNativeSkillDir
	case TargetGeneric:
		return GenericNativeSkillDir
	default:
		return ""
	}
}

// LegacySkillDirs are previous Atlas write locations. Re-install may delete
// files there only when they still match a known generated Atlas skill
// byte-for-byte (Codex: avoid listing atlas-worker from both .codex and
// .agents). Customized leftovers stay.
func LegacySkillDirs(target Target) []string {
	switch target {
	case TargetCodex:
		return []string{CodexLegacySkillDir}
	case TargetGrok:
		return []string{GrokLegacySkillDir}
	default:
		return nil
	}
}

// AgentsRootTargets share .agents/skills/atlas-worker. Uninstall of one must
// not yank SKILL.md while the other still has a managed instruction block.
func AgentsRootTargets() []Target {
	return []Target{TargetCodex, TargetOpenClaw}
}

// ClientExecutableNames is PATH lookup order. Cursor CLI is cursor-agent;
// the GUI binary remains cursor when present.
func ClientExecutableNames(target Target) []string {
	switch target {
	case TargetClaude:
		return []string{"claude"}
	case TargetCodex:
		return []string{"codex"}
	case TargetCursor:
		return []string{"cursor", "cursor-agent"}
	case TargetOpenClaw:
		return []string{"openclaw"}
	case TargetGrok:
		return []string{"grok"}
	default:
		return nil
	}
}

// LookClientExecutable returns the first absolute-looking PATH hit.
func LookClientExecutable(look func(string) (string, error), target Target) string {
	if look == nil {
		return ""
	}
	for _, name := range ClientExecutableNames(target) {
		path, err := look(name)
		if err != nil || strings.TrimSpace(path) == "" {
			continue
		}
		return path
	}
	return ""
}

// AtlasManagedSkillFile is the marker check used for the ordinary managed
// refresh contract on native skill paths. It is not proof that a leftover
// legacy file is safe to delete: users can append guidance without stripping
// markers. Destructive legacy migration uses exactGeneratedLegacyFile.
func AtlasManagedSkillFile(path string, body []byte) bool {
	text := string(body)
	switch filepath.Base(path) {
	case "SKILL.md":
		return strings.Contains(text, atlasSkillNameLine) &&
			strings.Contains(text, atlasLifecycleBegin) &&
			strings.Contains(text, atlasLifecycleEnd)
	case "workflow.md":
		return strings.Contains(text, atlasWorkerReferenceH1) &&
			strings.Contains(text, "tracker agent available")
	case "openai.yaml":
		return strings.Contains(text, atlasWorkerDisplayName)
	case "atlas-context.md":
		return strings.Contains(text, "# Atlas provider context")
	default:
		return strings.Contains(text, "tracker agent available") &&
			(strings.Contains(text, "Atlas Tasker") || strings.Contains(text, "Atlas Worker"))
	}
}

// KeepSharedAgentsSkill reports whether uninstall of removing must leave path
// in place because another agents-root target still owns the workspace.
func KeepSharedAgentsSkill(root string, removing Target, path string) bool {
	if !underRelDir(root, path, AgentsRootSkillDir) {
		return false
	}
	base := filepath.Base(path)
	if base != "SKILL.md" && base != "workflow.md" && base != "atlas-context.md" {
		return false
	}
	return agentsRootPeerInstalled(root, removing)
}

func agentsRootPeerInstalled(root string, removing Target) bool {
	for _, other := range AgentsRootTargets() {
		if other == removing {
			continue
		}
		begin, end, err := InstructionMarkers(other)
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
		if err != nil {
			continue
		}
		body := string(raw)
		if strings.Contains(body, begin) && strings.Contains(body, end) {
			return true
		}
	}
	return false
}

// LegacySkillRemovals lists leftover files that still match a known generated
// Atlas skill exactly. Customized or unsafe (symlinked) leftovers are
// collisions: keep them, do not follow links.
func LegacySkillRemovals(root string, target Target) (remove []string, collisions []string) {
	plan := planLegacySkillMigration(root, target)
	return plan.remove, plan.collisions
}

type legacySkillPlan struct {
	remove     []string
	collisions []string
}

func planLegacySkillMigration(root string, target Target) legacySkillPlan {
	var plan legacySkillPlan
	for _, rel := range LegacySkillDirs(target) {
		dir := filepath.Join(root, filepath.FromSlash(rel))
		if !workspacePathUnlinked(root, dir) {
			if _, err := os.Lstat(dir); err == nil {
				plan.collisions = append(plan.collisions, dir)
			}
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		collectLegacyOwned(root, target, dir, entries, &plan)
	}
	return plan
}

func collectLegacyOwned(root string, target Target, dir string, entries []os.DirEntry, plan *legacySkillPlan) {
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if !workspacePathUnlinked(root, path) {
			plan.collisions = append(plan.collisions, path)
			continue
		}
		info, err := os.Lstat(path)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			plan.collisions = append(plan.collisions, path)
			continue
		}
		if info.IsDir() {
			nested, err := os.ReadDir(path)
			if err != nil {
				continue
			}
			collectLegacyOwned(root, target, path, nested, plan)
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if exactGeneratedLegacyFile(target, path, body) {
			plan.remove = append(plan.remove, path)
			continue
		}
		if AtlasManagedSkillFile(path, body) {
			plan.collisions = append(plan.collisions, path)
		}
	}
}

func exactGeneratedLegacyFile(target Target, path string, body []byte) bool {
	for _, known := range generatedLegacyBodies(target, filepath.Base(path)) {
		if bytes.Equal(body, known) {
			return true
		}
	}
	return false
}

func generatedLegacyBodies(target Target, name string) [][]byte {
	var out [][]byte
	add := func(body string) {
		raw := []byte(body)
		for _, existing := range out {
			if bytes.Equal(existing, raw) {
				return
			}
		}
		out = append(out, raw)
	}
	switch name {
	case "SKILL.md":
		add(atlasWorkerSkill(string(target)))
		add(v115ProviderSkill(string(target)))
	case "workflow.md":
		add(atlasWorkerReference())
	case "openai.yaml":
		add(atlasWorkerOpenAIYAML())
	case "atlas-next.md":
		add(atlasNextCommandTemplate())
	case "atlas-take.md":
		add(atlasTakeCommandTemplate())
	case "atlas-review.md":
		add(atlasReviewCommandTemplate())
	case "atlas-uninstall.md":
		add(atlasUninstallCommandTemplate())
	}
	return out
}

// RemoveLegacySkillFiles deletes leftover files that still match known
// generated Atlas content. Symlinks and customized files are left in place.
func RemoveLegacySkillFiles(root string, target Target) (removed []string, collisions []string, err error) {
	plan := planLegacySkillMigration(root, target)
	for _, path := range plan.remove {
		if !workspacePathUnlinked(root, path) {
			plan.collisions = append(plan.collisions, path)
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return plan.remove, plan.collisions, err
		}
		removed = append(removed, path)
	}
	for _, rel := range LegacySkillDirs(target) {
		pruneEmptyTree(root, filepath.Join(root, filepath.FromSlash(rel)))
	}
	return removed, plan.collisions, nil
}

func pruneEmptyTree(root, dir string) {
	if !workspacePathUnlinked(root, dir) {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			continue
		}
		pruneEmptyTree(root, path)
	}
	entries, err = os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		return
	}
	_ = os.Remove(dir)
}

// workspacePathUnlinked is true when path is inside root and no path
// component from root to path is a symlink. Missing trailing components are
// allowed; a symlink anywhere on the existing prefix is not.
func workspacePathUnlinked(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return false
	}
	if rel == "." {
		return true
	}
	current := root
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return true
		}
		if err != nil {
			return false
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	return true
}

func underRelDir(root, path, rel string) bool {
	dir := filepath.Join(root, filepath.FromSlash(rel))
	clean := filepath.Clean(path)
	if clean == dir {
		return true
	}
	prefix := dir + string(os.PathSeparator)
	return strings.HasPrefix(clean, prefix)
}

func underAnySkillDir(root, path string) bool {
	for _, target := range DetectableTargets() {
		if dir := NativeSkillDir(target); dir != "" && underRelDir(root, path, dir) {
			return true
		}
		for _, rel := range LegacySkillDirs(target) {
			if underRelDir(root, path, rel) {
				return true
			}
		}
	}
	return false
}

// ClientSkillCollision is true when path is in a client-native skill root
// (not Atlas-owned .tracker/integrations) and the existing file is not
// Atlas-managed. Repair of drifted files under .tracker/integrations still
// overwrites; a user's own .cursor/.agents/.grok/.claude skill is left alone.
func ClientSkillCollision(root, path string, existing []byte) bool {
	if !underAnySkillDir(root, path) || underRelDir(root, path, ".tracker/integrations") {
		return false
	}
	return !AtlasManagedSkillFile(path, existing)
}
