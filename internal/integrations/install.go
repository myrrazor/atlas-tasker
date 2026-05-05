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

type Target string

const (
	TargetCodex  Target = "codex"
	TargetClaude Target = "claude"
)

type InstallResult struct {
	Target          Target   `json:"target"`
	InstructionFile string   `json:"instruction_file"`
	GuideFile       string   `json:"guide_file"`
	Created         []string `json:"created"`
	Updated         []string `json:"updated"`
}

type Installer struct {
	Root string
}

func (i Installer) Install(target Target, force bool) (InstallResult, error) {
	spec, err := i.spec(target)
	if err != nil {
		return InstallResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(spec.guidePath), 0o755); err != nil {
		return InstallResult{}, err
	}
	result := InstallResult{Target: target, InstructionFile: spec.instructionPath, GuideFile: spec.guidePath}
	if changed, err := writeManagedFile(spec.guidePath, spec.guideBody); err != nil {
		return InstallResult{}, err
	} else if changed == createdState {
		result.Created = append(result.Created, spec.guidePath)
	} else if changed == updatedState {
		result.Updated = append(result.Updated, spec.guidePath)
	}
	if changed, err := writeInstructionFile(spec.instructionPath, spec.blockBody, force); err != nil {
		return InstallResult{}, err
	} else if changed == createdState {
		result.Created = append(result.Created, spec.instructionPath)
	} else if changed == updatedState {
		result.Updated = append(result.Updated, spec.instructionPath)
	}
	return result, nil
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
}

func (i Installer) spec(target Target) (installSpec, error) {
	switch target {
	case TargetCodex:
		guidePath := filepath.Join(i.Root, ".tracker", "integrations", "codex-guide.md")
		return installSpec{
			instructionPath: filepath.Join(i.Root, "AGENTS.md"),
			guidePath:       guidePath,
			blockBody:       codexBlock(guidePath),
			guideBody:       codexGuide(),
		}, nil
	case TargetClaude:
		guidePath := filepath.Join(i.Root, ".tracker", "integrations", "claude-guide.md")
		return installSpec{
			instructionPath: filepath.Join(i.Root, "CLAUDE.md"),
			guidePath:       guidePath,
			blockBody:       claudeBlock(guidePath),
			guideBody:       claudeGuide(),
		}, nil
	default:
		return installSpec{}, fmt.Errorf("unsupported integration target: %s", target)
	}
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

func writeInstructionFile(path string, block string, force bool) (fileChange, error) {
	managed := managedBegin + "\n" + block + "\n" + managedEnd + "\n"
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
	if strings.Contains(body, managedBegin) && strings.Contains(body, managedEnd) {
		updated, changed := replaceManagedBlock(body, managed)
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

func replaceManagedBlock(body string, managed string) (string, bool) {
	start := strings.Index(body, managedBegin)
	end := strings.Index(body, managedEnd)
	if start == -1 || end == -1 || end < start {
		return body, false
	}
	end += len(managedEnd)
	if end < len(body) && body[end] == '\n' {
		end++
	}
	replacement := body[:start] + managed + body[end:]
	return replacement, replacement != body
}

func codexBlock(guidePath string) string {
	return strings.TrimSpace(fmt.Sprintf(`## Atlas Tasker (Codex)

- Pull actionable work with `+"`tracker queue --actor <actor> --json`"+`.
- Claim before coding: `+"`tracker ticket claim <ID> --actor <actor>`"+`.
- Update status and review explicitly: `+"`move`"+`, `+"`request-review`"+`, `+"`approve`"+`, `+"`complete`"+`.
- Use `+"`tracker inspect <ID> --actor <actor> --json`"+` when the queue and the ticket detail disagree.
- TUI is available with `+"`tracker tui --actor <actor>`"+`, but the CLI stays canonical.
- Detailed Atlas Tasker guidance lives in `+"`%s`"+`.
`, guidePath))
}

func claudeBlock(guidePath string) string {
	return strings.TrimSpace(fmt.Sprintf(`## Atlas Tasker (Claude Code)

- Start with `+"`tracker queue --actor <actor> --json`"+` or `+"`tracker review-queue --actor <actor> --json`"+`.
- Claim work before editing and release it when you stop.
- Use explicit review commands instead of assuming `+"`move done`"+` is enough.
- Use `+"`tracker inspect <ID> --actor <actor> --json`"+` to debug policy, lease, and queue state.
- TUI is available with `+"`tracker tui --actor <actor>`"+`, but generated guidance should stay CLI/JSON-first.
- Detailed Atlas Tasker guidance lives in `+"`%s`"+`.
`, guidePath))
}

func codexGuide() string {
	return strings.TrimSpace(`# Atlas Tasker Codex Guide

Use Atlas Tasker as the local source of truth for work state.

## Recommended loop

1. `+"`tracker queue --actor agent:builder-1 --json`"+` to find the next actionable ticket.
2. `+"`tracker ticket claim <ID> --actor agent:builder-1`"+` before you start.
3. `+"`tracker ticket move <ID> in_progress --actor agent:builder-1`"+` when implementation starts.
4. `+"`tracker ticket comment <ID> --body \"what changed\" --actor agent:builder-1`"+` for durable notes.
5. `+"`tracker ticket request-review <ID> --actor agent:builder-1`"+` when the diff is ready.
6. `+"`tracker ticket approve|reject|complete ...`"+` based on the active completion policy.

## Run-scoped launch flow

- `+"`tracker run launch <RUN-ID>`"+` writes the current run brief plus provider launch files under `+"`.tracker/runtime/<run-id>/`"+`.
- `+"`tracker run open <RUN-ID> --json`"+` shows the canonical runtime, evidence, and worktree paths without changing files.
- When you attach to an external session, record it with `+"`tracker run attach <RUN-ID> --provider codex --session-ref <session>`"+`.

## JSON-first reads

- `+"`tracker queue --actor <actor> --json`"+`
- `+"`tracker inspect <ID> --actor <actor> --json`"+`
- `+"`tracker ticket history <ID> --json`"+`

## Notes

- `+"`tracker shell`"+` and `+"`tracker tui`"+` are convenience layers. The CLI remains canonical.
- The generated block in `+"`AGENTS.md`"+` is managed by Atlas Tasker. Edit around it, not inside it, unless you intend to own the divergence.
`) + "\n"
}

func claudeGuide() string {
	return strings.TrimSpace(`# Atlas Tasker Claude Guide

Use Atlas Tasker as the durable workflow layer for Claude Code sessions.

## Recommended loop

1. `+"`tracker queue --actor agent:builder-1 --json`"+` for implementation work.
2. `+"`tracker review-queue --actor agent:reviewer-1 --json`"+` for review work.
3. `+"`tracker ticket claim <ID> --actor <actor>`"+` before you touch the task.
4. `+"`tracker ticket comment <ID> --body \"decision or risk\" --actor <actor>`"+` when context should survive the session.
5. `+"`tracker ticket request-review|approve|reject|complete ...`"+` instead of relying on status changes alone.

## Run-scoped launch flow

- `+"`tracker run launch <RUN-ID>`"+` writes the current brief and Claude launch text under `+"`.tracker/runtime/<run-id>/`"+`.
- `+"`tracker run open <RUN-ID> --json`"+` shows the canonical runtime, evidence, and worktree paths without changing files.
- Record the active Claude session with `+"`tracker run attach <RUN-ID> --provider claude --session-ref <session>`"+`.

## Debugging state

- `+"`tracker inspect <ID> --actor <actor> --json`"+` shows effective policy, lease state, queue placement, and history in one call.
- `+"`tracker who --json`"+` shows active and stale lease holders.

## Notes

- The generated block in `+"`CLAUDE.md`"+` is managed by Atlas Tasker. Keep custom notes outside the managed markers.
- Atlas Tasker guidance is intentionally thin and editable. Extend the guide file if your local workflow needs more detail.
`) + "\n"
}
