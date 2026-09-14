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
		ManagedGitignoreEnd,
		"",
	}, "\n")
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
	body := string(current)
	begin := strings.Index(body, ManagedGitignoreBegin)
	end := strings.Index(body, ManagedGitignoreEnd)
	if begin >= 0 && end > begin {
		end += len(ManagedGitignoreEnd)
		if end < len(body) && body[end] == '\n' {
			end++
		}
		updated := body[:begin] + block + body[end:]
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

func DefaultProjectKey(dirName string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(dirName) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	key := b.String()
	if key == "" {
		return "MAIN"
	}
	if key[0] < 'A' || key[0] > 'Z' {
		key = "P" + key
	}
	if len(key) < 2 {
		return "MAIN"
	}
	if len(key) > 12 {
		key = key[:12]
	}
	return key
}
