package integrations

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	managedBegin = "<!-- atlas-tasker:begin -->"
	managedEnd   = "<!-- atlas-tasker:end -->"
)

// Codex and OpenClaw both read AGENTS.md, so they get separate marker pairs and
// each install rewrites only its own block. The unprefixed pair predates the
// openclaw target and stays as-is so existing AGENTS.md files keep working.
type blockMarkers struct {
	begin string
	end   string
}

var (
	defaultMarkers  = blockMarkers{begin: managedBegin, end: managedEnd}
	openclawMarkers = blockMarkers{begin: "<!-- atlas-tasker:openclaw:begin -->", end: "<!-- atlas-tasker:openclaw:end -->"}
	genericMarkers  = blockMarkers{begin: "<!-- atlas-tasker:generic:begin -->", end: "<!-- atlas-tasker:generic:end -->"}
	cursorMarkers   = blockMarkers{begin: "<!-- atlas-tasker:cursor:begin -->", end: "<!-- atlas-tasker:cursor:end -->"}
	grokMarkers     = blockMarkers{begin: "<!-- atlas-tasker:grok:begin -->", end: "<!-- atlas-tasker:grok:end -->"}
)

type Target string

const (
	TargetCodex    Target = "codex"
	TargetClaude   Target = "claude"
	TargetOpenClaw Target = "openclaw"
	TargetGeneric  Target = "generic"
	TargetCursor   Target = "cursor"
	TargetGrok     Target = "grok"
)

type InstallResult struct {
	Target           Target   `json:"target"`
	InstructionFile  string   `json:"instruction_file"`
	GuideFile        string   `json:"guide_file"`
	SkillFiles       []string `json:"skill_files,omitempty"`
	CommandFiles     []string `json:"command_files,omitempty"`
	GlobalSkillFiles []string `json:"global_skill_files,omitempty"`
	Created          []string `json:"created"`
	Updated          []string `json:"updated"`
}

type InstallOptions struct {
	Force  bool
	Global bool
}

type Installer struct {
	Root string
}

func (i Installer) Install(target Target, force bool) (InstallResult, error) {
	return i.InstallOpts(target, InstallOptions{Force: force})
}

func (i Installer) InstallOpts(target Target, opts InstallOptions) (InstallResult, error) {
	root, err := filepath.Abs(i.Root)
	if err != nil {
		return InstallResult{}, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return InstallResult{}, err
	}
	i.Root = root
	if opts.Global && target != TargetOpenClaw {
		return InstallResult{}, fmt.Errorf("--global is only supported for the openclaw target")
	}
	spec, err := i.spec(target)
	if err != nil {
		return InstallResult{}, err
	}
	paths := []string{spec.instructionPath, spec.guidePath}
	for _, file := range spec.extraFiles {
		paths = append(paths, file.path)
	}
	// Validate the entire plan before writing even the first managed file.
	for _, path := range paths {
		if err := validateInstallPath(root, path); err != nil {
			return InstallResult{}, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(spec.guidePath), 0o755); err != nil {
		return InstallResult{}, err
	}
	// empty slices, not nil -- these land in --json and a null array is a papercut
	result := InstallResult{Target: target, InstructionFile: spec.instructionPath, GuideFile: spec.guidePath, Created: []string{}, Updated: []string{}}
	if changed, err := writeManagedFile(spec.guidePath, spec.guideBody); err != nil {
		return InstallResult{}, err
	} else if changed == createdState {
		result.Created = append(result.Created, spec.guidePath)
	} else if changed == updatedState {
		result.Updated = append(result.Updated, spec.guidePath)
	}
	if changed, err := writeInstructionFile(spec.instructionPath, spec.blockBody, spec.markers, opts.Force); err != nil {
		return InstallResult{}, err
	} else if changed == createdState {
		result.Created = append(result.Created, spec.instructionPath)
	} else if changed == updatedState {
		result.Updated = append(result.Updated, spec.instructionPath)
	}
	for _, file := range spec.extraFiles {
		if err := os.MkdirAll(filepath.Dir(file.path), 0o755); err != nil {
			return InstallResult{}, err
		}
		if changed, err := writeManagedFile(file.path, file.body); err != nil {
			return InstallResult{}, err
		} else if changed == createdState {
			result.Created = append(result.Created, file.path)
		} else if changed == updatedState {
			result.Updated = append(result.Updated, file.path)
		}
		switch file.kind {
		case "skill":
			result.SkillFiles = append(result.SkillFiles, file.path)
		case "command":
			result.CommandFiles = append(result.CommandFiles, file.path)
		}
	}
	if opts.Global {
		globalFiles, err := installOpenClawGlobalSkills(spec.extraFiles)
		if err != nil {
			return InstallResult{}, err
		}
		result.GlobalSkillFiles = globalFiles
	}
	return result, nil
}

func validateInstallPath(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("integration destination is outside workspace: %s", path)
	}
	current := root
	parts := strings.Split(rel, string(filepath.Separator))
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("integration destination contains a symlink: %s", current)
		}
		if index < len(parts)-1 && !info.IsDir() || index == len(parts)-1 && !info.Mode().IsRegular() {
			return fmt.Errorf("invalid integration destination: %s", current)
		}
	}
	return nil
}

type fileChange int

const (
	unchangedState fileChange = iota
	createdState
	updatedState
)

type installSpec struct {
	instructionPath string
	guidePath       string
	blockBody       string
	guideBody       string
	markers         blockMarkers
	extraFiles      []managedInstallFile
}

type managedInstallFile struct {
	path string
	body string
	kind string
}

func (i Installer) spec(target Target) (installSpec, error) {
	switch target {
	case TargetCodex:
		guidePath := filepath.Join(i.Root, ".tracker", "integrations", "codex-guide.md")
		guideRef := filepath.ToSlash(filepath.Join(".tracker", "integrations", "codex-guide.md"))
		skillDir := filepath.Join(i.Root, ".codex", "skills", "atlas-worker")
		return installSpec{
			instructionPath: filepath.Join(i.Root, "AGENTS.md"),
			guidePath:       guidePath,
			blockBody:       codexBlock(guideRef),
			guideBody:       codexGuide(),
			markers:         defaultMarkers,
			extraFiles: []managedInstallFile{
				{path: filepath.Join(skillDir, "SKILL.md"), body: atlasWorkerSkill("codex"), kind: "skill"},
				{path: filepath.Join(skillDir, "references", "workflow.md"), body: atlasWorkerReference(), kind: "skill"},
				{path: filepath.Join(skillDir, "agents", "openai.yaml"), body: atlasWorkerOpenAIYAML(), kind: "skill"},
				{path: filepath.Join(i.Root, ".tracker", "integrations", "commands", "atlas-next.md"), body: atlasNextCommandTemplate(), kind: "command"},
				{path: filepath.Join(i.Root, ".tracker", "integrations", "commands", "atlas-take.md"), body: atlasTakeCommandTemplate(), kind: "command"},
				{path: filepath.Join(i.Root, ".tracker", "integrations", "commands", "atlas-review.md"), body: atlasReviewCommandTemplate(), kind: "command"},
			},
		}, nil
	case TargetClaude:
		guidePath := filepath.Join(i.Root, ".tracker", "integrations", "claude-guide.md")
		guideRef := filepath.ToSlash(filepath.Join(".tracker", "integrations", "claude-guide.md"))
		commandDir := filepath.Join(i.Root, ".claude", "commands")
		skillDir := filepath.Join(i.Root, ".claude", "skills", "atlas-worker")
		return installSpec{
			instructionPath: filepath.Join(i.Root, "CLAUDE.md"),
			guidePath:       guidePath,
			blockBody:       claudeBlock(guideRef),
			guideBody:       claudeGuide(),
			markers:         defaultMarkers,
			extraFiles: []managedInstallFile{
				// modern Claude Code skill layout; the old
				// .tracker/integrations copy retired with v1.9
				{path: filepath.Join(skillDir, "SKILL.md"), body: atlasWorkerSkill("claude"), kind: "skill"},
				{path: filepath.Join(skillDir, "references", "workflow.md"), body: atlasWorkerReference(), kind: "skill"},
				{path: filepath.Join(commandDir, "atlas-next.md"), body: atlasNextCommandTemplate(), kind: "command"},
				{path: filepath.Join(commandDir, "atlas-take.md"), body: atlasTakeCommandTemplate(), kind: "command"},
				{path: filepath.Join(commandDir, "atlas-review.md"), body: atlasReviewCommandTemplate(), kind: "command"},
			},
		}, nil
	case TargetOpenClaw:
		guidePath := filepath.Join(i.Root, ".tracker", "integrations", "openclaw-guide.md")
		guideRef := filepath.ToSlash(filepath.Join(".tracker", "integrations", "openclaw-guide.md"))
		// .agents/skills is OpenClaw's repo-local skill root; ~/.openclaw/skills is
		// the shared one, and that copy is the user's to install, not ours to write
		skillDir := filepath.Join(i.Root, ".agents", "skills", "atlas-worker")
		return installSpec{
			instructionPath: filepath.Join(i.Root, "AGENTS.md"),
			guidePath:       guidePath,
			blockBody:       openclawBlock(guideRef),
			guideBody:       openclawGuide(filepath.ToSlash(filepath.Join(".agents", "skills", "atlas-worker"))),
			markers:         openclawMarkers,
			extraFiles: []managedInstallFile{
				{path: filepath.Join(skillDir, "SKILL.md"), body: atlasWorkerSkill("openclaw"), kind: "skill"},
				{path: filepath.Join(skillDir, "references", "workflow.md"), body: atlasWorkerReference(), kind: "skill"},
				{path: filepath.Join(skillDir, "commands", "atlas-next.md"), body: atlasNextCommandTemplate(), kind: "command"},
				{path: filepath.Join(skillDir, "commands", "atlas-take.md"), body: atlasTakeCommandTemplate(), kind: "command"},
				{path: filepath.Join(skillDir, "commands", "atlas-review.md"), body: atlasReviewCommandTemplate(), kind: "command"},
			},
		}, nil
	case TargetGeneric:
		guidePath := filepath.Join(i.Root, ".tracker", "integrations", "generic-agent-guide.md")
		guideRef := filepath.ToSlash(filepath.Join(".tracker", "integrations", "generic-agent-guide.md"))
		skillDir := filepath.Join(i.Root, ".tracker", "integrations", "generic-agent-skill")
		return installSpec{
			instructionPath: filepath.Join(i.Root, "AGENTS.md"),
			guidePath:       guidePath,
			blockBody:       genericBlock(guideRef),
			guideBody:       genericGuide(),
			markers:         genericMarkers,
			extraFiles: []managedInstallFile{
				{path: filepath.Join(i.Root, ".tracker", "integrations", "generic-agent-instructions.md"), body: genericBlock(guideRef) + "\n", kind: "command"},
				{path: filepath.Join(skillDir, "SKILL.md"), body: atlasWorkerSkill("generic"), kind: "skill"},
				{path: filepath.Join(skillDir, "references", "workflow.md"), body: atlasWorkerReference(), kind: "skill"},
				{path: filepath.Join(skillDir, "commands", "atlas-next.md"), body: atlasNextCommandTemplate(), kind: "command"},
				{path: filepath.Join(skillDir, "commands", "atlas-take.md"), body: atlasTakeCommandTemplate(), kind: "command"},
				{path: filepath.Join(skillDir, "commands", "atlas-review.md"), body: atlasReviewCommandTemplate(), kind: "command"},
			},
		}, nil
	case TargetCursor:
		guidePath := filepath.Join(i.Root, ".tracker", "integrations", "cursor-guide.md")
		guideRef := filepath.ToSlash(filepath.Join(".tracker", "integrations", "cursor-guide.md"))
		skillDir := filepath.Join(i.Root, ".cursor", "skills", "atlas-worker")
		return installSpec{
			instructionPath: filepath.Join(i.Root, "AGENTS.md"),
			guidePath:       guidePath,
			blockBody:       cursorBlock(guideRef),
			guideBody:       cursorGuide(),
			markers:         cursorMarkers,
			extraFiles: []managedInstallFile{
				{path: filepath.Join(skillDir, "SKILL.md"), body: atlasWorkerSkill("cursor"), kind: "skill"},
				{path: filepath.Join(skillDir, "references", "workflow.md"), body: atlasWorkerReference(), kind: "skill"},
				{path: filepath.Join(skillDir, "commands", "atlas-next.md"), body: atlasNextCommandTemplate(), kind: "command"},
				{path: filepath.Join(skillDir, "commands", "atlas-take.md"), body: atlasTakeCommandTemplate(), kind: "command"},
				{path: filepath.Join(skillDir, "commands", "atlas-review.md"), body: atlasReviewCommandTemplate(), kind: "command"},
			},
		}, nil
	case TargetGrok:
		guidePath := filepath.Join(i.Root, ".tracker", "integrations", "grok-guide.md")
		guideRef := filepath.ToSlash(filepath.Join(".tracker", "integrations", "grok-guide.md"))
		skillDir := filepath.Join(i.Root, ".tracker", "integrations", "grok-agent-skill")
		return installSpec{
			instructionPath: filepath.Join(i.Root, "AGENTS.md"),
			guidePath:       guidePath,
			blockBody:       grokBlock(guideRef),
			guideBody:       grokGuide(),
			markers:         grokMarkers,
			extraFiles: []managedInstallFile{
				{path: filepath.Join(skillDir, "SKILL.md"), body: atlasWorkerSkill("grok"), kind: "skill"},
				{path: filepath.Join(skillDir, "references", "workflow.md"), body: atlasWorkerReference(), kind: "skill"},
			},
		}, nil
	default:
		return installSpec{}, fmt.Errorf("unsupported integration target: %s", target)
	}
}

func installOpenClawGlobalSkills(files []managedInstallFile) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	if resolved, err := filepath.EvalSymlinks(home); err == nil {
		home = resolved
	}
	destRoot := filepath.Join(home, ".openclaw", "skills", "atlas-worker")
	type plannedFile struct {
		dest string
		body string
	}
	planned := make([]plannedFile, 0, len(files))
	for _, file := range files {
		if file.kind != "skill" && file.kind != "command" {
			continue
		}
		rel := ""
		switch {
		case strings.HasSuffix(file.path, filepath.Join("atlas-worker", "SKILL.md")):
			rel = "SKILL.md"
		case strings.Contains(file.path, filepath.Join("atlas-worker", "references")):
			rel = filepath.Join("references", filepath.Base(file.path))
		case strings.Contains(file.path, filepath.Join("atlas-worker", "commands")):
			rel = filepath.Join("commands", filepath.Base(file.path))
		default:
			continue
		}
		dest := filepath.Join(destRoot, rel)
		planned = append(planned, plannedFile{dest: dest, body: file.body})
	}
	if len(planned) == 0 {
		return nil, fmt.Errorf("openclaw --global found no skill files to copy")
	}
	for _, item := range planned {
		if err := validateInstallPath(home, item.dest); err != nil {
			return nil, err
		}
	}
	written := make([]string, 0, len(planned))
	for _, item := range planned {
		if err := os.MkdirAll(filepath.Dir(item.dest), 0o755); err != nil {
			return nil, err
		}
		if _, err := writeManagedFile(item.dest, item.body); err != nil {
			return nil, err
		}
		written = append(written, item.dest)
	}
	return written, nil
}

func writeManagedFile(path string, body string) (fileChange, error) {
	if current, err := os.ReadFile(path); err == nil {
		if string(current) == body {
			return unchangedState, nil
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return unchangedState, err
		}
		return updatedState, nil
	} else if !os.IsNotExist(err) {
		return unchangedState, err
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return unchangedState, err
	}
	return createdState, nil
}

func writeInstructionFile(path string, block string, markers blockMarkers, force bool) (fileChange, error) {
	if markers.begin == "" {
		markers = defaultMarkers
	}
	managed := markers.begin + "\n" + block + "\n" + markers.end + "\n"
	current, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte(managed), 0o644); err != nil {
			return unchangedState, err
		}
		return createdState, nil
	}
	if err != nil {
		return unchangedState, err
	}
	if force {
		if string(current) == managed {
			return unchangedState, nil
		}
		if err := os.WriteFile(path, []byte(managed), 0o644); err != nil {
			return unchangedState, err
		}
		return updatedState, nil
	}
	body := string(current)
	if strings.Contains(body, markers.begin) && strings.Contains(body, markers.end) {
		updated, changed := replaceManagedBlock(body, managed, markers)
		if !changed {
			return unchangedState, nil
		}
		if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
			return unchangedState, err
		}
		return updatedState, nil
	}
	updated := strings.TrimRight(body, "\n") + "\n\n" + managed
	if body == "" {
		updated = managed
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return unchangedState, err
	}
	return updatedState, nil
}

func replaceManagedBlock(body string, managed string, markers blockMarkers) (string, bool) {
	start := strings.Index(body, markers.begin)
	end := strings.Index(body, markers.end)
	if start == -1 || end == -1 || end < start {
		return body, false
	}
	end += len(markers.end)
	if end < len(body) && body[end] == '\n' {
		end++
	}
	replacement := body[:start] + managed + body[end:]
	return replacement, replacement != body
}

func codexBlock(guidePath string) string {
	return strings.TrimSpace(fmt.Sprintf(`## Atlas Tasker (Codex)

- Set `+"`TRACKER_ACTOR`"+` to your real Atlas identity before using commands below, for example `+"`export TRACKER_ACTOR='agent:builder-1'`"+`.
- Pull actionable work with `+"`tracker agent available <agent-id> --json`"+`.
- Explain blockers with `+"`tracker agent pending <agent-id> --json`"+`.
- Generate pasteable goals with `+"`tracker goal brief <TICKET-ID|RUN-ID> --md`"+`.
- Claim before coding: `+"`tracker ticket claim <ID> --actor \"$TRACKER_ACTOR\" --reason \"start work\"`"+`.
- Update status and review explicitly: `+"`move`"+`, `+"`request-review`"+`, `+"`approve`"+`, `+"`complete`"+`.
- Moving a ticket to its current status is a successful no-op; inspect the ticket before retrying a different transition.
- Use `+"`tracker inspect <ID> --actor \"$TRACKER_ACTOR\" --json`"+` when the queue and the ticket detail disagree.
- TUI is available with `+"`tracker tui --actor \"$TRACKER_ACTOR\"`"+`, but the CLI stays canonical.
- Detailed Atlas Tasker guidance lives in `+"`%s`"+`.
`, guidePath))
}

func claudeBlock(guidePath string) string {
	return strings.TrimSpace(fmt.Sprintf(`## Atlas Tasker (Claude Code)

- Set `+"`TRACKER_ACTOR`"+` to your real Atlas identity before using commands below, for example `+"`export TRACKER_ACTOR='agent:builder-1'`"+`.
- Start with `+"`tracker agent available <agent-id> --json`"+` or `+"`tracker agent pending <agent-id> --json`"+`.
- Use `+"`tracker goal brief <TICKET-ID|RUN-ID> --md`"+` when a session needs a compact handoff prompt.
- Claim work before editing and release it when you stop.
- Use explicit review commands instead of assuming `+"`move done`"+` is enough.
- Moving a ticket to its current status is a successful no-op; inspect the ticket before retrying a different transition.
- Use `+"`tracker inspect <ID> --actor \"$TRACKER_ACTOR\" --json`"+` to debug policy, lease, and queue state.
- TUI is available with `+"`tracker tui --actor \"$TRACKER_ACTOR\"`"+`, but generated guidance should stay CLI/JSON-first.
- Detailed Atlas Tasker guidance lives in `+"`%s`"+`.
`, guidePath))
}

func codexGuide() string {
	return strings.TrimSpace(`# Atlas Tasker Codex Guide

Use Atlas Tasker as the local source of truth for work state.

Set your real Atlas identity once for this shell, for example `+"`export TRACKER_ACTOR='agent:builder-1'`"+`.

## Recommended loop

1. `+"`tracker agent available builder-1 --json`"+` to find the next actionable ticket.
2. `+"`tracker ticket claim <ID> --actor \"$TRACKER_ACTOR\" --reason \"start work\"`"+` before you start.
3. `+"`tracker ticket move <ID> in_progress --actor \"$TRACKER_ACTOR\" --reason \"start work\"`"+` when implementation starts.
4. `+"`tracker goal brief <ID> --md`"+` when Codex goal mode needs a clean objective.
5. `+"`tracker ticket comment <ID> --body \"what changed\" --actor \"$TRACKER_ACTOR\" --reason \"progress note\"`"+` for durable notes.
6. `+"`tracker ticket request-review <ID> --actor \"$TRACKER_ACTOR\" --reason \"ready for review\"`"+` when the diff is ready.
7. `+"`tracker run evidence add <RUN-ID> --type test_result --title \"verification\" --body \"test output\" --actor \"$TRACKER_ACTOR\" --reason \"record verification\"`"+` when you have run-scoped proof.
8. `+"`tracker ticket approve|reject|complete ...`"+` based on the active completion policy.

## Run-scoped launch flow

- `+"`tracker run launch <RUN-ID> --actor \"$TRACKER_ACTOR\" --reason \"prepare launch files\"`"+` writes the current run brief plus provider launch files under `+"`.tracker/runtime/<run-id>/`"+`.
- `+"`tracker run open <RUN-ID> --json`"+` shows the canonical runtime, evidence, and worktree paths without changing files.
- When you attach to an external session, record it with `+"`tracker run attach <RUN-ID> --provider codex --session-ref <session> --actor \"$TRACKER_ACTOR\" --reason \"attach session\"`"+`.

## JSON-first reads

- `+"`tracker queue --actor \"$TRACKER_ACTOR\" --json`"+`
- `+"`tracker agent available <agent-id> --json`"+`
- `+"`tracker agent pending <agent-id> --json`"+`
- `+"`tracker inspect <ID> --actor \"$TRACKER_ACTOR\" --json`"+`
- `+"`tracker ticket history <ID> --json`"+`
- `+"`tracker goal brief <ID> --json`"+`

## Notes

- `+"`tracker shell`"+` and `+"`tracker tui`"+` are convenience layers. The CLI remains canonical.
- Moving a ticket to its current status is a successful no-op across CLI, MCP, bulk, and web paths.
- The generated block in `+"`AGENTS.md`"+` is managed by Atlas Tasker. Edit around it, not inside it, unless you intend to own the divergence.
`) + "\n"
}

func claudeGuide() string {
	return strings.TrimSpace(`# Atlas Tasker Claude Guide

Use Atlas Tasker as the durable workflow layer for Claude Code sessions.

Set your real Atlas identity once for this shell, for example `+"`export TRACKER_ACTOR='agent:builder-1'`"+`.

## Recommended loop

1. `+"`tracker agent available builder-1 --json`"+` for implementation work.
2. `+"`tracker agent available reviewer-1 --json`"+` for review work.
3. `+"`tracker ticket claim <ID> --actor \"$TRACKER_ACTOR\" --reason \"start work\"`"+` before you touch the task.
4. `+"`tracker goal brief <ID> --md`"+` when a compact Claude Code session prompt is useful.
5. `+"`tracker ticket comment <ID> --body \"decision or risk\" --actor \"$TRACKER_ACTOR\" --reason \"record context\"`"+` when context should survive the session.
6. `+"`tracker ticket request-review|approve|reject|complete ...`"+` instead of relying on status changes alone.

## Run-scoped launch flow

- `+"`tracker run launch <RUN-ID> --actor \"$TRACKER_ACTOR\" --reason \"prepare launch files\"`"+` writes the current brief and Claude launch text under `+"`.tracker/runtime/<run-id>/`"+`.
- `+"`tracker run open <RUN-ID> --json`"+` shows the canonical runtime, evidence, and worktree paths without changing files.
- Record the active Claude session with `+"`tracker run attach <RUN-ID> --provider claude --session-ref <session> --actor \"$TRACKER_ACTOR\" --reason \"attach session\"`"+`.

## Debugging state

- `+"`tracker inspect <ID> --actor \"$TRACKER_ACTOR\" --json`"+` shows effective policy, lease state, queue placement, and history in one call.
- `+"`tracker who --json`"+` shows active and stale lease holders.

## Notes

- The generated block in `+"`CLAUDE.md`"+` is managed by Atlas Tasker. Keep custom notes outside the managed markers.
- Moving a ticket to its current status is a successful no-op across CLI, MCP, bulk, and web paths.
- Atlas Tasker guidance is intentionally thin and editable. Extend the guide file if your local workflow needs more detail.
`) + "\n"
}
