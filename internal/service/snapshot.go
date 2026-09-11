package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

// CheckpointCopyBudget is how long a checkpoint may hold the exclusive
// workspace write lock while copying candidate files (ADR §3 / B-B). Hashing
// and Git run after the lock is released. The value matches FileLockManager's
// default wait so a checkpoint never out-waits an ordinary CLI writer.
const CheckpointCopyBudget = 5 * time.Second

// ExportCandidateRoots is the single collector candidate list used by manual
// backups, exports, and automatic checkpoints (AT114-401). Do not maintain a
// second "automatic backup files" list.
func ExportCandidateRoots() []string {
	return []string{
		"projects",
		filepath.ToSlash(filepath.Join(".tracker", "config.toml")),
		filepath.ToSlash(filepath.Join(".tracker", "managed-mode.json")),
		filepath.ToSlash(filepath.Join(".tracker", "events")),
		filepath.ToSlash(filepath.Join(".tracker", "automations")),
		filepath.ToSlash(filepath.Join(".tracker", "views")),
		filepath.ToSlash(filepath.Join(".tracker", "subscriptions")),
		filepath.ToSlash(filepath.Join(".tracker", "agents")),
		filepath.ToSlash(filepath.Join(".tracker", "runbooks")),
		filepath.ToSlash(filepath.Join(".tracker", "runs")),
		filepath.ToSlash(filepath.Join(".tracker", "gates")),
		filepath.ToSlash(filepath.Join(".tracker", "handoffs")),
		filepath.ToSlash(filepath.Join(".tracker", "evidence")),
		filepath.ToSlash(filepath.Join(".tracker", "changes")),
		filepath.ToSlash(filepath.Join(".tracker", "checks")),
		filepath.ToSlash(filepath.Join(".tracker", "permission-profiles")),
		filepath.ToSlash(filepath.Join(".tracker", "imports")),
		filepath.ToSlash(filepath.Join(".tracker", "retention")),
		filepath.ToSlash(filepath.Join(".tracker", "security", "keys", "public")),
		filepath.ToSlash(filepath.Join(".tracker", "security", "revocations")),
		filepath.ToSlash(filepath.Join(".tracker", "security", "signatures")),
		filepath.ToSlash(filepath.Join(".tracker", "governance", "policies")),
		filepath.ToSlash(filepath.Join(".tracker", "governance", "packs")),
		filepath.ToSlash(filepath.Join(".tracker", "classification", "labels")),
		filepath.ToSlash(filepath.Join(".tracker", "classification", "policies")),
		filepath.ToSlash(filepath.Join(".tracker", "redaction", "rules")),
		filepath.ToSlash(filepath.Join(".tracker", "audit", "reports")),
		filepath.ToSlash(filepath.Join(".tracker", "audit", "packets")),
		filepath.ToSlash(filepath.Join(".tracker", "collaborators")),
		filepath.ToSlash(filepath.Join(".tracker", "memberships")),
		filepath.ToSlash(filepath.Join(".tracker", "mentions")),
		filepath.ToSlash(filepath.Join(".tracker", "archives")),
	}
}

// ExportedOnlyCandidateRoots are collector roots that tracker backup create
// still walks for export bundles but that backupRestoreSafeFiles drops because
// they are not restore-safe. The AT114-507 drill decides whether any of these
// later join the allowlist. Keep them named so the parity test cannot drift.
func ExportedOnlyCandidateRoots() []string {
	return []string{
		filepath.ToSlash(filepath.Join(".tracker", "config.toml")),
		filepath.ToSlash(filepath.Join(".tracker", "agents")),
		filepath.ToSlash(filepath.Join(".tracker", "views")),
		filepath.ToSlash(filepath.Join(".tracker", "automations")),
		filepath.ToSlash(filepath.Join(".tracker", "subscriptions")),
		filepath.ToSlash(filepath.Join(".tracker", "runbooks")),
		filepath.ToSlash(filepath.Join(".tracker", "imports")),
	}
}

// RestoreSafeAllowlistPrefixes is every prefix isCanonicalRestorePlanPath
// accepts. Each must be a collector candidate or listed in
// IntentionallyUncollectedAllowlistPrefixes.
func RestoreSafeAllowlistPrefixes() []string {
	return []string{
		"projects/",
		".tracker/collaborators/",
		".tracker/memberships/",
		".tracker/mentions/",
		".tracker/runs/",
		".tracker/gates/",
		".tracker/handoffs/",
		".tracker/evidence/",
		".tracker/changes/",
		".tracker/checks/",
		".tracker/permission-profiles/",
		".tracker/retention/",
		".tracker/archives/",
		".tracker/security/keys/public/",
		".tracker/security/revocations/",
		".tracker/security/signatures/",
		".tracker/governance/policies/",
		".tracker/governance/packs/",
		".tracker/classification/labels/",
		".tracker/classification/policies/",
		".tracker/redaction/rules/",
		".tracker/audit/reports/",
		".tracker/audit/packets/",
		".tracker/events/",
		".tracker/managed-mode.json",
	}
}

// IntentionallyUncollectedAllowlistPrefixes is the named hole list required
// by ADR §2.1. After AT114-401 it is empty: every restore-safe prefix is a
// collector candidate.
func IntentionallyUncollectedAllowlistPrefixes() []string {
	return nil
}

func shouldSkipExportWalkDir(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	for _, skip := range []string{
		".tracker/runtime",
		".tracker/exports",
		".tracker/mutations",
		".tracker/backups",
	} {
		if rel == skip || strings.HasPrefix(rel, skip+"/") {
			return true
		}
	}
	return false
}

// CanonicalFile is one hashed restore-safe path.
type CanonicalFile struct {
	Path   string
	SHA256 string
	Size   int64
}

// CanonicalSnapshot is the shared hashed tree used by archive backups and Git
// checkpoints. Files never include the SQLite index, locks, private keys,
// remotes, approvals, or runtime state.
type CanonicalSnapshot struct {
	Files             []CanonicalFile
	TreeHash          string
	Bytes             int64
	CopiedUnderLock   bool
	HashedAfterUnlock bool
	CopyDuration      time.Duration
	Watermarks        map[string]int64
	SnapshotDir       string
}

func (s CanonicalSnapshot) RestoreSafePaths() []string {
	out := make([]string, 0, len(s.Files))
	for _, file := range s.Files {
		out = append(out, file.Path)
	}
	return out
}

func (s CanonicalSnapshot) CheckpointFiles() []contracts.CheckpointFile {
	out := make([]contracts.CheckpointFile, 0, len(s.Files))
	for _, file := range s.Files {
		out = append(out, contracts.CheckpointFile{Path: file.Path, SHA256: file.SHA256, Size: file.Size})
	}
	return out
}

func (s *ActionService) collectRestoreSafeFiles() ([]string, error) {
	files, err := collectExportFiles(s.Root)
	if err != nil {
		return nil, err
	}
	return backupRestoreSafeFiles(files), nil
}

// CaptureCanonicalSnapshot copies restore-safe files under the exclusive
// workspace write lock, then hashes the copy after releasing the lock.
func (s *ActionService) CaptureCanonicalSnapshot(ctx context.Context, destDir string) (CanonicalSnapshot, error) {
	if strings.TrimSpace(destDir) == "" {
		return CanonicalSnapshot{}, fmt.Errorf("snapshot destination is required")
	}
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return CanonicalSnapshot{}, fmt.Errorf("create snapshot dir: %w", err)
	}
	started := time.Now()
	var files []string
	copyFn := func(ctx context.Context) error {
		deadline := time.Now().Add(CheckpointCopyBudget)
		if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
			deadline = dl
		}
		var err error
		files, err = s.collectRestoreSafeFiles()
		if err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return apperr.New(apperr.CodeBusy, "checkpoint copy budget exceeded before copy")
		}
		for _, rel := range files {
			if time.Now().After(deadline) {
				return apperr.New(apperr.CodeBusy, "checkpoint copy budget exceeded")
			}
			if err := copyRegularFile(s.Root, destDir, rel); err != nil {
				return err
			}
		}
		return nil
	}
	var err error
	if lockHeld(ctx) {
		err = copyFn(ctx)
	} else {
		err = WithWriteLock(ctx, s.LockManager, "checkpoint snapshot copy", copyFn)
	}
	if err != nil {
		return CanonicalSnapshot{}, err
	}
	copyDuration := time.Since(started)
	snap := CanonicalSnapshot{CopiedUnderLock: true, SnapshotDir: destDir, CopyDuration: copyDuration}
	hashed, err := hashSnapshotDir(destDir, files)
	if err != nil {
		return CanonicalSnapshot{}, err
	}
	snap.Files = hashed
	snap.HashedAfterUnlock = true
	for _, file := range hashed {
		snap.Bytes += file.Size
	}
	snap.TreeHash = contracts.CanonicalTreeHash(snap.CheckpointFiles())
	marks, err := eventWatermarks(ctx, s.Events)
	if err != nil {
		return CanonicalSnapshot{}, err
	}
	snap.Watermarks = marks
	return snap, nil
}

func copyRegularFile(root, destRoot, rel string) error {
	src := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("stat %s: %w", rel, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return apperr.New(apperr.CodeInvalidInput, "export_symlink_rejected: symlink entries are not allowed")
	}
	if !info.Mode().IsRegular() {
		return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("snapshot rejects non-regular file %s", rel))
	}
	item := contracts.RestorePlanItem{Path: rel, Action: contracts.RestorePlanCreate}
	if err := item.Validate(); err != nil {
		return err
	}
	dst := filepath.Join(destRoot, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func hashSnapshotDir(destRoot string, files []string) ([]CanonicalFile, error) {
	out := make([]CanonicalFile, 0, len(files))
	for _, rel := range files {
		full := filepath.Join(destRoot, filepath.FromSlash(rel))
		info, err := os.Lstat(full)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("snapshot hash rejects %s", rel))
		}
		sum, err := fileSHA256(full)
		if err != nil {
			return nil, err
		}
		out = append(out, CanonicalFile{Path: rel, SHA256: sum, Size: info.Size()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func eventWatermarks(ctx context.Context, events contracts.EventLog) (map[string]int64, error) {
	marks := map[string]int64{}
	if events == nil {
		return marks, nil
	}
	all, err := events.StreamEvents(ctx, "", 0)
	if err != nil {
		return nil, err
	}
	for _, event := range all {
		key := strings.TrimSpace(event.Project)
		if key == "" || key == workspaceProjectKey {
			key = "workspace"
		}
		if event.EventID > marks[key] {
			marks[key] = event.EventID
		}
	}
	return marks, nil
}

func defaultUserStateDir(home string, getenv func(string) string) (string, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	if xdg := strings.TrimSpace(getenv("XDG_STATE_HOME")); xdg != "" {
		if !filepath.IsAbs(xdg) {
			return "", fmt.Errorf("XDG_STATE_HOME must be an absolute path")
		}
		return filepath.Clean(filepath.Join(xdg, "atlas-tasker")), nil
	}
	if strings.TrimSpace(home) == "" {
		home = strings.TrimSpace(getenv("HOME"))
	}
	if strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("home is required to resolve the Atlas state directory")
	}
	if !filepath.IsAbs(home) {
		return "", fmt.Errorf("home must be an absolute path")
	}
	return filepath.Join(filepath.Clean(home), ".local", "state", "atlas-tasker"), nil
}

func resolveUserStateDir(stateDir, home string) (string, error) {
	if strings.TrimSpace(stateDir) != "" {
		if !filepath.IsAbs(stateDir) {
			return "", fmt.Errorf("state dir must be absolute")
		}
		return filepath.Clean(stateDir), nil
	}
	return defaultUserStateDir(home, os.Getenv)
}

func candidateCoversPrefix(candidate, prefix string) bool {
	candidate = strings.TrimSuffix(filepath.ToSlash(candidate), "/")
	prefix = filepath.ToSlash(prefix)
	if candidate == strings.TrimSuffix(prefix, "/") || candidate == prefix {
		return true
	}
	if strings.HasSuffix(prefix, "/") {
		return prefix == candidate+"/" || strings.HasPrefix(prefix, candidate+"/")
	}
	return prefix == candidate || strings.HasPrefix(prefix, candidate+"/")
}

func isExportedOnlyCandidate(candidate string) bool {
	candidate = filepath.ToSlash(candidate)
	for _, root := range ExportedOnlyCandidateRoots() {
		if candidate == root || strings.HasPrefix(candidate, strings.TrimSuffix(root, "/")+"/") {
			return true
		}
	}
	return false
}
