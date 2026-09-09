package integrations

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Detection is a read-only signal that a coding agent is present on the machine
// or in the workspace. It never writes files.
type Detection struct {
	Target  Target   `json:"target"`
	Found   bool     `json:"found"`
	Reasons []string `json:"reasons,omitempty"`
}

// DetectOptions controls how agent presence is probed.
type DetectOptions struct {
	Workspace string
	Home      string
	LookPath  func(string) (string, error)
	Stat      func(string) (os.FileInfo, error)
	Getenv    func(string) string
}

// DetectableTargets are install targets the wizard can offer.
func DetectableTargets() []Target {
	return []Target{TargetClaude, TargetCodex, TargetCursor, TargetOpenClaw, TargetGrok, TargetGeneric}
}

// Detect scans PATH, home-config dirs, and the workspace for coding agents.
func Detect(opts DetectOptions) []Detection {
	opts = normalizeDetectOptions(opts)
	out := make([]Detection, 0, len(DetectableTargets()))
	for _, target := range DetectableTargets() {
		out = append(out, detectOne(opts, target))
	}
	return out
}

// DetectedTargets returns only agents that look installed/present.
// TargetGeneric is never auto-detected.
func DetectedTargets(opts DetectOptions) []Detection {
	all := Detect(opts)
	found := make([]Detection, 0, len(all))
	for _, item := range all {
		if item.Found && item.Target != TargetGeneric {
			found = append(found, item)
		}
	}
	return found
}

func normalizeDetectOptions(opts DetectOptions) DetectOptions {
	if opts.LookPath == nil {
		opts.LookPath = exec.LookPath
	}
	if opts.Stat == nil {
		opts.Stat = os.Stat
	}
	if opts.Getenv == nil {
		opts.Getenv = os.Getenv
	}
	if strings.TrimSpace(opts.Home) == "" {
		if home, err := os.UserHomeDir(); err == nil {
			opts.Home = home
		}
	}
	if strings.TrimSpace(opts.Workspace) == "" {
		if cwd, err := os.Getwd(); err == nil {
			opts.Workspace = cwd
		}
	}
	return opts
}

func detectOne(opts DetectOptions, target Target) Detection {
	detection := Detection{Target: target}
	switch target {
	case TargetClaude:
		detection.Reasons = appendBinReason(detection.Reasons, opts, "claude")
		detection.Reasons = appendDirReason(detection.Reasons, opts, filepath.Join(opts.Home, ".claude"), "~/.claude")
		if env := strings.TrimSpace(opts.Getenv("CLAUDE_CONFIG_DIR")); env != "" {
			detection.Reasons = appendDirReason(detection.Reasons, opts, env, "$CLAUDE_CONFIG_DIR")
		}
		detection.Reasons = appendDirReason(detection.Reasons, opts, filepath.Join(opts.Workspace, ".claude"), ".claude/")
		detection.Reasons = appendFileReason(detection.Reasons, opts, filepath.Join(opts.Workspace, "CLAUDE.md"), "CLAUDE.md")
	case TargetCodex:
		detection.Reasons = appendBinReason(detection.Reasons, opts, "codex")
		detection.Reasons = appendDirReason(detection.Reasons, opts, filepath.Join(opts.Home, ".codex"), "~/.codex")
		if env := strings.TrimSpace(opts.Getenv("CODEX_HOME")); env != "" {
			detection.Reasons = appendDirReason(detection.Reasons, opts, env, "$CODEX_HOME")
		}
		detection.Reasons = appendDirReason(detection.Reasons, opts, filepath.Join(opts.Workspace, ".codex"), ".codex/")
	case TargetCursor:
		detection.Reasons = appendBinReason(detection.Reasons, opts, "cursor")
		detection.Reasons = appendDirReason(detection.Reasons, opts, filepath.Join(opts.Home, ".cursor"), "~/.cursor")
		detection.Reasons = appendDirReason(detection.Reasons, opts, filepath.Join(opts.Workspace, ".cursor"), ".cursor/")
	case TargetOpenClaw:
		detection.Reasons = appendBinReason(detection.Reasons, opts, "openclaw")
		detection.Reasons = appendDirReason(detection.Reasons, opts, filepath.Join(opts.Home, ".openclaw"), "~/.openclaw")
		detection.Reasons = appendDirReason(detection.Reasons, opts, filepath.Join(opts.Workspace, ".agents"), ".agents/")
		detection.Reasons = appendDirReason(detection.Reasons, opts, filepath.Join(opts.Workspace, ".openclaw"), ".openclaw/")
	case TargetGrok:
		detection.Reasons = appendBinReason(detection.Reasons, opts, "grok")
		detection.Reasons = appendDirReason(detection.Reasons, opts, filepath.Join(opts.Home, ".grok"), "~/.grok")
	case TargetGeneric:
		// Always available as an explicit choice; never auto-selected.
		return detection
	}
	detection.Found = len(detection.Reasons) > 0
	return detection
}

func appendBinReason(reasons []string, opts DetectOptions, name string) []string {
	path, err := opts.LookPath(name)
	if err != nil || strings.TrimSpace(path) == "" {
		return reasons
	}
	return append(reasons, fmt.Sprintf("binary on PATH (%s)", path))
}

func appendDirReason(reasons []string, opts DetectOptions, path, label string) []string {
	info, err := opts.Stat(path)
	if err != nil || info == nil || !info.IsDir() {
		return reasons
	}
	return append(reasons, "config dir "+label)
}

func appendFileReason(reasons []string, opts DetectOptions, path, label string) []string {
	info, err := opts.Stat(path)
	if err != nil || info == nil || info.IsDir() {
		return reasons
	}
	return append(reasons, "workspace file "+label)
}

// ParseTargetList parses a comma/space separated target list.
func ParseTargetList(raw string) ([]Target, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	out := make([]Target, 0, len(parts))
	seen := map[Target]bool{}
	for _, part := range parts {
		target := Target(strings.ToLower(strings.TrimSpace(part)))
		switch target {
		case TargetCodex, TargetClaude, TargetOpenClaw, TargetGeneric, TargetCursor, TargetGrok:
		default:
			return nil, fmt.Errorf("unknown integration target %q (valid: codex, claude, openclaw, cursor, grok, generic)", part)
		}
		if seen[target] {
			continue
		}
		seen[target] = true
		out = append(out, target)
	}
	return out, nil
}
