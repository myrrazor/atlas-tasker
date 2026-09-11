package service

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

const (
	backupGitAuthorName  = "Atlas Tasker"
	backupGitAuthorEmail = "atlas-backup@localhost"
	backupCommitSubject  = "atlas checkpoint"
)

func validateGitExecutable(path string) (string, error) {
	var err error
	if strings.TrimSpace(path) == "" {
		path, err = exec.LookPath("git")
		if err != nil {
			return "", fmt.Errorf("git executable not found: %w", err)
		}
	}
	if !filepath.IsAbs(path) {
		path, err = exec.LookPath(path)
		if err != nil {
			return "", fmt.Errorf("git executable not found: %w", err)
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if strings.ContainsAny(abs, "\n\r\x00;$`|") {
		return "", fmt.Errorf("git executable path contains unsafe characters")
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat git executable: %w", err)
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("git path %s is not an executable file", abs)
	}
	cmd := exec.Command(abs, "--version")
	cmd.Env = gitSafeEnv(abs, "", "", "")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git --version failed: %w", err)
	}
	if !strings.Contains(strings.ToLower(string(out)), "git version") {
		return "", fmt.Errorf("git executable did not report a version")
	}
	return abs, nil
}

func gitSafeEnv(gitPath, gitDir, indexFile, snapshotDir string) []string {
	keep := map[string]bool{
		"PATH": true, "HOME": true, "USER": true, "LOGNAME": true,
		"XDG_CONFIG_HOME": true, "XDG_CACHE_HOME": true, "XDG_STATE_HOME": true,
		"SSH_AUTH_SOCK": true, "SSH_AGENT_PID": true,
		"LANG": true, "LC_ALL": true, "TZ": true, "TMPDIR": true,
	}
	env := []string{
		"GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_NAME=" + backupGitAuthorName,
		"GIT_AUTHOR_EMAIL=" + backupGitAuthorEmail,
		"GIT_COMMITTER_NAME=" + backupGitAuthorName,
		"GIT_COMMITTER_EMAIL=" + backupGitAuthorEmail,
		"GIT_CONFIG_NOSYSTEM=0",
	}
	if gitDir != "" {
		env = append(env, "GIT_DIR="+gitDir)
	}
	if indexFile != "" {
		env = append(env, "GIT_INDEX_FILE="+indexFile)
	}
	for _, pair := range os.Environ() {
		key, _, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		switch key {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY",
			"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_NAMESPACE",
			"GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT", "GIT_DIR_CEILING":
			continue
		}
		if keep[key] {
			env = append(env, pair)
		}
	}
	_ = gitPath
	_ = snapshotDir
	return env
}

func backupRefName(workspaceID, replicaID string) string {
	return "refs/atlas/backups/" + workspaceID + "/" + replicaID
}

type gitRunner struct {
	Git      string
	Repo     string
	Snapshot string
	Index    string
	Hooks    string
}

func (r gitRunner) pinnedArgs(args ...string) []string {
	out := []string{
		"-c", "commit.gpgsign=false",
		"-c", "tag.gpgsign=false",
		"-c", "core.hooksPath=" + r.Hooks,
		"-c", "protocol.allow=never",
		"-c", "protocol.file.allow=always",
		"-c", "protocol.ssh.allow=always",
		"-c", "protocol.https.allow=always",
		"-c", "credential.interactive=never",
	}
	return append(out, args...)
}

func (r gitRunner) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, r.Git, r.pinnedArgs(args...)...)
	cmd.Dir = r.Snapshot
	if cmd.Dir == "" {
		cmd.Dir = r.Repo
	}
	cmd.Env = gitSafeEnv(r.Git, r.Repo, r.Index, r.Snapshot)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (e *CheckpointEngine) ensureBareRepo(ctx context.Context) error {
	if err := e.paths.ensure(); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(e.paths.Repo, "HEAD")); err == nil {
		return nil
	}
	if err := os.MkdirAll(e.paths.Repo, 0o700); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, e.git, "-c", "init.defaultBranch=atlas-backup", "init", "--bare", e.paths.Repo)
	cmd.Env = gitSafeEnv(e.git, "", "", "")
	if out, err := cmd.CombinedOutput(); err != nil {
		retry := exec.CommandContext(ctx, e.git, "init", "--bare", e.paths.Repo)
		retry.Env = gitSafeEnv(e.git, "", "", "")
		if out2, err2 := retry.CombinedOutput(); err2 != nil {
			return fmt.Errorf("git init --bare: %w: %s %s", err, strings.TrimSpace(string(out)), strings.TrimSpace(string(out2)))
		}
	}
	return os.Chmod(e.paths.Repo, 0o700)
}

func (e *CheckpointEngine) gitRunner(snapshot, index string) gitRunner {
	return gitRunner{
		Git:      e.git,
		Repo:     e.paths.Repo,
		Snapshot: snapshot,
		Index:    index,
		Hooks:    e.paths.Hooks,
	}
}

func (e *CheckpointEngine) commitSnapshot(ctx context.Context, snap CanonicalSnapshot, manifest contracts.CheckpointManifest) (string, error) {
	if err := e.crash(CrashAfterManifest); err != nil {
		return "", err
	}
	if err := e.ensureBareRepo(ctx); err != nil {
		return "", err
	}
	index := filepath.Join(e.paths.Tmp, "index-"+manifest.CheckpointID)
	_ = os.Remove(index)
	runner := e.gitRunner(snap.SnapshotDir, index)
	parent, _ := runner.run(ctx, "rev-parse", backupRefName(e.workspaceID, e.replicaID))
	for _, file := range snap.Files {
		if err := e.crash(CrashDuringSnapshot); err != nil {
			return "", err
		}
		blob, err := runner.run(ctx, "hash-object", "-w", "--", file.Path)
		if err != nil {
			return "", err
		}
		if _, err := runner.run(ctx, "update-index", "--add", "--cacheinfo", "100644", blob, file.Path); err != nil {
			return "", err
		}
	}
	manifestPath := filepath.Join(snap.SnapshotDir, contracts.CheckpointManifestName)
	raw, err := contracts.EncodeCheckpointManifest(manifest)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
		return "", err
	}
	blob, err := runner.run(ctx, "hash-object", "-w", "--", contracts.CheckpointManifestName)
	if err != nil {
		return "", err
	}
	if _, err := runner.run(ctx, "update-index", "--add", "--cacheinfo", "100644", blob, contracts.CheckpointManifestName); err != nil {
		return "", err
	}
	if err := e.crash(CrashAfterGitObjects); err != nil {
		return "", err
	}
	tree, err := runner.run(ctx, "write-tree")
	if err != nil {
		return "", err
	}
	args := []string{"commit-tree", tree, "-m", backupCommitSubject}
	if parent != "" && !strings.Contains(strings.ToLower(parent), "unknown revision") && !strings.Contains(strings.ToLower(parent), "needed a single revision") {
		args = append(args, "-p", parent)
	}
	commit, err := runner.run(ctx, args...)
	if err != nil {
		return "", err
	}
	if err := e.crash(CrashBeforeLocalRef); err != nil {
		return "", err
	}
	ref := backupRefName(e.workspaceID, e.replicaID)
	if _, err := runner.run(ctx, "update-ref", ref, commit); err != nil {
		return "", err
	}
	if err := e.crash(CrashAfterLocalRef); err != nil {
		return "", err
	}
	e.maintenanceMaybe(ctx, runner)
	return commit, nil
}

func (e *CheckpointEngine) maintenanceMaybe(ctx context.Context, runner gitRunner) {
	ledger, err := loadLedger(e.paths.Ledger)
	if err != nil {
		return
	}
	if ledger.CommitsSinceMaintenance < e.maintenanceEvery {
		return
	}
	e.maintenanceRuns++
	_, _ = runner.run(ctx, "gc", "--auto")
	ledger.CommitsSinceMaintenance = 0
	_ = atomicWriteJSON(e.paths.Ledger, ledger)
}

func (e *CheckpointEngine) readCommitManifest(ctx context.Context, commit string) (contracts.CheckpointManifest, error) {
	runner := e.gitRunner(e.paths.Tmp, "")
	raw, err := runner.run(ctx, "show", commit+":"+contracts.CheckpointManifestName)
	if err != nil {
		return contracts.CheckpointManifest{}, err
	}
	return contracts.ParseCheckpointManifest([]byte(raw + "\n"))
}

func (e *CheckpointEngine) materializeCommit(ctx context.Context, commit, dest string) error {
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return err
	}
	runner := e.gitRunner(dest, filepath.Join(e.paths.Tmp, "restore-index"))
	tree, err := runner.run(ctx, "rev-parse", commit+"^{tree}")
	if err != nil {
		return err
	}
	listing, err := runner.run(ctx, "ls-tree", "-r", "--full-tree", tree)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(listing, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 100644 blob <sha>\t<path>
		mode, rest, ok := strings.Cut(line, " ")
		if !ok || mode != "100644" {
			return apperr.New(apperr.CodeConflict, "checkpoint tree contains a non-regular blob")
		}
		kind, rest, ok := strings.Cut(rest, " ")
		if !ok || kind != "blob" {
			return apperr.New(apperr.CodeConflict, "checkpoint tree entry is not a blob")
		}
		sha, path, ok := strings.Cut(rest, "\t")
		if !ok {
			return apperr.New(apperr.CodeConflict, "checkpoint tree listing is malformed")
		}
		if path != contracts.CheckpointManifestName {
			item := contracts.RestorePlanItem{Path: path, Action: contracts.RestorePlanCreate}
			if err := item.Validate(); err != nil {
				return err
			}
		}
		body, err := runner.run(ctx, "cat-file", "blob", sha)
		if err != nil {
			return err
		}
		// cat-file through run() trims trailing whitespace of the whole stdout.
		// Re-read without TrimSpace for file bodies.
		raw, err := e.catFile(ctx, sha)
		if err != nil {
			return err
		}
		_ = body
		target := filepath.Join(dest, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(target, raw, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (e *CheckpointEngine) catFile(ctx context.Context, sha string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, e.git, e.gitRunner("", "").pinnedArgs("cat-file", "blob", sha)...)
	cmd.Dir = e.paths.Repo
	cmd.Env = gitSafeEnv(e.git, e.paths.Repo, "", "")
	return cmd.Output()
}

func (e *CheckpointEngine) currentRefCommit(ctx context.Context) (string, error) {
	runner := e.gitRunner(e.paths.Tmp, "")
	out, err := runner.run(ctx, "rev-parse", backupRefName(e.workspaceID, e.replicaID))
	if err != nil {
		return "", err
	}
	return out, nil
}
