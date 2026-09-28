package app

import (
	"os"
	"path/filepath"
	"strings"
)

func workspaceGitignoreBlock() string {
	return strings.Join([]string{
		ManagedGitignoreBegin,
		"# Local-only Atlas paths (not ticket markdown under projects/).",
		"/.tracker/mutations/",
		"/.tracker/runtime/",
		"/.tracker/*.log",
		"/.tracker/sync/mirror/",
		"/.tracker/sync/staging/",
		"/.tracker/sync/bundles/",
		"/.tracker/archives/*",
		"!/.tracker/archives/*.md",
		"/.tracker/exports/*",
		"!/.tracker/exports/*.md",
		"/.tracker/security/keys/private/",
		"/.tracker/security/trust/",
		"/.tracker/redaction/previews/",
		"/.tracker/backups/snapshots/",
		"/.tracker/goal/",
		"/.tracker/evidence/**",
		"!/.tracker/evidence/",
		"!/.tracker/evidence/*/",
		"!/.tracker/evidence/**/*.md",
		"/.tracker/index.sqlite",
		"/.tracker/index.sqlite-*",
		"/.tracker/write.lock",
		"/.tracker/web-session.json",
		"/.tracker/web-session.json.tmp",
		"/.tracker/web-create-submits.json",
		"/.tracker/web-create-submits.json.tmp",
		"/.tracker/web-create-submits.lock",
		ManagedGitignoreEnd,
		"",
	}, "\n")
}

// RefreshManagedIgnores rewrites an existing Atlas ignore block so session
// secrets created after init stay untracked. It does not invent a block in a
// repository that never asked for one.
func RefreshManagedIgnores(root string) error {
	block := workspaceGitignoreBlock()
	for _, path := range []string{
		filepath.Join(root, ".gitignore"),
		filepath.Join(root, ".git", "info", "exclude"),
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if !strings.Contains(string(raw), ManagedGitignoreBegin) {
			continue
		}
		_, err = upsertManagedBlock(path, block)
		return err
	}
	return nil
}

func applyGitIgnore(root string, mode GitMode) (bool, error) {
	block := workspaceGitignoreBlock()
	switch mode.Normalized() {
	case GitModePrivate:
		gitDir := filepath.Join(root, ".git")
		if info, err := os.Stat(gitDir); err != nil || !info.IsDir() {
			return false, nil
		}
		path := filepath.Join(gitDir, "info", "exclude")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return false, err
		}
		return upsertManagedBlock(path, block)
	default:
		return upsertManagedBlock(filepath.Join(root, ".gitignore"), block)
	}
}

func upsertManagedBlock(path, block string) (bool, error) {
	current, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	// A CRLF checkout leaves \r on each pattern. Git then does not match
	// .tracker/web-session.json, and a refresh that only skips a bare \n
	// after the end marker never rewrites the block. Canonical lines are LF.
	body := strings.ReplaceAll(string(current), "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	begin := strings.Index(body, ManagedGitignoreBegin)
	end := strings.Index(body, ManagedGitignoreEnd)
	if begin >= 0 && end > begin {
		preserved := extraIgnoreLines(body[begin:end], block)
		end += len(ManagedGitignoreEnd)
		if end < len(body) && body[end] == '\n' {
			end++
		}
		suffix := body[end:]
		updated := body[:begin] + block + keptIgnoreLines(preserved, suffix) + suffix
		if updated == body {
			return false, nil
		}
		if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
			return false, err
		}
		return true, nil
	}
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	if body != "" {
		body += "\n"
	}
	body += block
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// extraIgnoreLines returns ignore lines a person put inside the managed
// block. Refresh rewrites that block from the canonical list; dropping
// those lines would un-ignore a secrets file. They are moved just after
// the end marker instead.
func extraIgnoreLines(oldInner, canonicalBlock string) []string {
	canon := map[string]struct{}{}
	for _, line := range strings.Split(canonicalBlock, "\n") {
		canon[strings.TrimSpace(line)] = struct{}{}
	}
	var kept []string
	seen := map[string]struct{}{}
	for _, line := range strings.Split(oldInner, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == ManagedGitignoreBegin || trimmed == ManagedGitignoreEnd {
			continue
		}
		if _, ok := canon[trimmed]; ok {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		kept = append(kept, trimmed)
	}
	return kept
}

func keptIgnoreLines(lines []string, suffix string) string {
	if len(lines) == 0 {
		return ""
	}
	existing := map[string]struct{}{}
	for _, line := range strings.Split(suffix, "\n") {
		existing[strings.TrimSpace(line)] = struct{}{}
	}
	var b strings.Builder
	for _, line := range lines {
		if _, ok := existing[line]; ok {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func DefaultProjectKey(dirName string) string {
	if key, ok := validDefaultProjectKey(compactProjectKey(dirName)); ok {
		return key
	}
	for _, word := range splitProjectNameWords(dirName) {
		if key, ok := validDefaultProjectKey(compactProjectKey(word)); ok {
			return key
		}
	}
	return "MAIN"
}

func compactProjectKey(raw string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(raw) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func splitProjectNameWords(raw string) []string {
	return strings.FieldsFunc(raw, func(r rune) bool {
		return !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
}

func validDefaultProjectKey(key string) (string, bool) {
	if key == "" {
		return "", false
	}
	if key[0] < 'A' || key[0] > 'Z' {
		key = "P" + key
	}
	if len(key) < 2 || len(key) > 12 {
		return "", false
	}
	return key, true
}
