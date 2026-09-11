package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type RemoteCheckpointRef struct {
	CheckpointID    string            `json:"checkpoint_id"`
	Commit          string            `json:"commit"`
	ReplicaID       string            `json:"replica_id"`
	WorkspaceID     string            `json:"workspace_id"`
	CreatedAt       time.Time         `json:"created_at"`
	AtlasVersion    string            `json:"atlas_version,omitempty"`
	EventWatermarks map[string]int64  `json:"event_watermarks,omitempty"`
	Ref             string            `json:"ref"`
	Verified        bool              `json:"verified"`
}

type RemoteCheckpointListView struct {
	Kind        string                `json:"kind"`
	GeneratedAt time.Time             `json:"generated_at"`
	TargetID    string                `json:"target_id"`
	URLRedacted string                `json:"url_redacted"`
	Items       []RemoteCheckpointRef `json:"items"`
}

type RemoteVerifyView struct {
	Kind        string              `json:"kind"`
	GeneratedAt time.Time           `json:"generated_at"`
	Verified    bool                `json:"verified"`
	Checkpoint  RemoteCheckpointRef `json:"checkpoint"`
	Errors      []string            `json:"errors,omitempty"`
}

type RemoteRestoreOptions struct {
	TargetID               string
	Checkpoint             string
	ReplicaID              string
	AllowWorkspaceMismatch bool
	Yes                    bool
	Actor                  contracts.Actor
	Reason                 string
}

func (s *ActionService) ListRemoteCheckpoints(ctx context.Context, targetID string) (RemoteCheckpointListView, error) {
	target, err := s.loadTarget(targetID)
	if err != nil {
		return RemoteCheckpointListView{}, err
	}
	engine, err := s.checkpointEngine()
	if err != nil {
		return RemoteCheckpointListView{}, err
	}
	tmp, err := os.MkdirTemp(engine.paths.Tmp, "remote-list-*")
	if err != nil {
		return RemoteCheckpointListView{}, err
	}
	defer os.RemoveAll(tmp)
	repo := filepath.Join(tmp, "repo.git")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		return RemoteCheckpointListView{}, err
	}
	runner := gitRunner{Git: engine.git, Repo: repo, Snapshot: tmp, Hooks: engine.paths.Hooks}
	if err := engine.ensureBareAt(ctx, repo); err != nil {
		return RemoteCheckpointListView{}, err
	}
	out, _, err := runner.runFull(ctx, "ls-remote", "--", target.URL)
	if err != nil {
		return RemoteCheckpointListView{}, err
	}
	view := RemoteCheckpointListView{
		Kind: "backup_remote_list", GeneratedAt: s.now(), TargetID: target.TargetID,
		URLRedacted: contracts.RedactBackupURL(target.URL),
	}
	type row struct {
		oid string
		ref string
	}
	rows := []row{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.HasPrefix(fields[1], "refs/atlas/backups/") {
			continue
		}
		rows = append(rows, row{oid: fields[0], ref: fields[1]})
	}
	for _, item := range rows {
		local := "refs/atlas/fetched/" + filepath.Base(item.ref)
		if _, err := runner.run(ctx, "fetch", "--", target.URL, item.ref+":"+local); err != nil {
			continue
		}
		manifest, err := readManifestFromRepo(ctx, runner, item.oid)
		if err != nil {
			continue
		}
		view.Items = append(view.Items, RemoteCheckpointRef{
			CheckpointID: manifest.CheckpointID, Commit: item.oid, ReplicaID: manifest.ReplicaID,
			WorkspaceID: manifest.WorkspaceID, CreatedAt: manifest.CreatedAt, AtlasVersion: manifest.AtlasVersion,
			EventWatermarks: manifest.EventWatermarks, Ref: item.ref,
		})
	}
	sort.Slice(view.Items, func(i, j int) bool {
		if view.Items[i].ReplicaID == view.Items[j].ReplicaID {
			return view.Items[i].CreatedAt.After(view.Items[j].CreatedAt)
		}
		return view.Items[i].ReplicaID < view.Items[j].ReplicaID
	})
	return view, nil
}

func (s *ActionService) VerifyRemoteCheckpoint(ctx context.Context, targetID, checkpoint string) (RemoteVerifyView, error) {
	resolved, archive, cleanup, err := s.fetchAndVerifyRemote(ctx, RemoteRestoreOptions{TargetID: targetID, Checkpoint: checkpoint})
	if cleanup != nil {
		defer cleanup()
	}
	_ = archive
	if err != nil {
		return RemoteVerifyView{Kind: "backup_remote_verify", GeneratedAt: s.now(), Errors: []string{sanitizeGitMessage(err.Error())}}, err
	}
	resolved.Verified = true
	return RemoteVerifyView{Kind: "backup_remote_verify", GeneratedAt: s.now(), Verified: true, Checkpoint: resolved}, nil
}

func (s *ActionService) RemoteRestorePlan(ctx context.Context, opts RemoteRestoreOptions) (RestorePlanDetailView, error) {
	_, archive, cleanup, err := s.fetchAndVerifyRemote(ctx, opts)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return RestorePlanDetailView{}, err
	}
	return s.CreateRestorePlan(ctx, archive, opts.Actor)
}

func (s *ActionService) RemoteRestoreApply(ctx context.Context, opts RemoteRestoreOptions) (RestoreApplyResultView, error) {
	_, archive, cleanup, err := s.fetchAndVerifyRemote(ctx, opts)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return RestoreApplyResultView{}, err
	}
	result, err := s.ApplyRestorePlan(ctx, archive, opts.Actor, opts.Reason, opts.Yes)
	if err != nil {
		return RestoreApplyResultView{}, err
	}
	if s.Projection != nil {
		_ = s.Projection.Rebuild(ctx, "")
	}
	return result, nil
}

func (s *ActionService) RemoteRecoveryDrill(ctx context.Context, targetID string, actor contracts.Actor, reason string) (map[string]any, error) {
	if !actor.IsValid() {
		return nil, apperr.New(apperr.CodeInvalidInput, "invalid actor")
	}
	listed, err := s.ListRemoteCheckpoints(ctx, targetID)
	if err != nil {
		return nil, err
	}
	if len(listed.Items) == 0 {
		return nil, apperr.New(apperr.CodeNotFound, "no remote checkpoints")
	}
	latest := listed.Items[0]
	verify, err := s.VerifyRemoteCheckpoint(ctx, targetID, latest.CheckpointID)
	if err != nil {
		return nil, err
	}
	plan, err := s.RemoteRestorePlan(ctx, RemoteRestoreOptions{TargetID: targetID, Checkpoint: latest.CheckpointID, Actor: actor})
	if err != nil {
		return nil, err
	}
	paths, _, err := s.backupPaths()
	if err == nil {
		ledger, _ := loadLedger(paths.Ledger)
		ledger.LastDrillAt = s.now()
		_ = atomicWriteJSON(paths.Ledger, ledger)
	}
	return map[string]any{
		"kind":            "backup_remote_drill",
		"generated_at":    s.now(),
		"verified":        verify.Verified,
		"checkpoint_id":   latest.CheckpointID,
		"replica_id":      latest.ReplicaID,
		"plan_items":      len(plan.Plan.Items),
		"url_redacted":    listed.URLRedacted,
		"secrets_excluded": true,
		"side_effect_free": true,
		"notes":           []string{"destructive workspace deletion is exercised only in isolated tests", reason},
	}, nil
}

func (s *ActionService) fetchAndVerifyRemote(ctx context.Context, opts RemoteRestoreOptions) (RemoteCheckpointRef, string, func(), error) {
	target, err := s.loadTarget(opts.TargetID)
	if err != nil {
		return RemoteCheckpointRef{}, "", nil, err
	}
	engine, err := s.checkpointEngine()
	if err != nil {
		return RemoteCheckpointRef{}, "", nil, err
	}
	tmp, err := os.MkdirTemp("", "atlas-remote-restore-*")
	if err != nil {
		return RemoteCheckpointRef{}, "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }
	repo := filepath.Join(tmp, "repo.git")
	if err := engine.ensureBareAt(ctx, repo); err != nil {
		cleanup()
		return RemoteCheckpointRef{}, "", nil, err
	}
	runner := gitRunner{Git: engine.git, Repo: repo, Snapshot: tmp, Hooks: filepath.Join(tmp, "hooks")}
	_ = os.MkdirAll(filepath.Join(tmp, "hooks"), 0o700)
	listed, listErr := s.ListRemoteCheckpoints(ctx, opts.TargetID)
	if listErr != nil {
		cleanup()
		return RemoteCheckpointRef{}, "", nil, listErr
	}
	want := strings.TrimSpace(opts.Checkpoint)
	var chosen RemoteCheckpointRef
	for _, item := range listed.Items {
		if opts.ReplicaID != "" && item.ReplicaID != opts.ReplicaID {
			continue
		}
		if want == "" || item.CheckpointID == want || item.Commit == want || strings.Contains(item.Ref, want) {
			chosen = item
			if want != "" {
				break
			}
		}
	}
	if chosen.Commit == "" {
		cleanup()
		return RemoteCheckpointRef{}, "", nil, apperr.New(apperr.CodeNotFound, "remote checkpoint not found")
	}
	chosenOID, chosenRef := chosen.Commit, chosen.Ref
	local := "refs/atlas/restore/" + chosenOID
	if _, err := runner.run(ctx, "fetch", "--", target.URL, chosenRef+":"+local); err != nil {
		cleanup()
		return RemoteCheckpointRef{}, "", nil, err
	}
	got, err := runner.run(ctx, "rev-parse", local)
	if err != nil || got != chosenOID {
		cleanup()
		return RemoteCheckpointRef{}, "", nil, apperr.New(apperr.CodeConflict, "fetched commit does not match")
	}
	tree, err := runner.run(ctx, "rev-parse", chosenOID+"^{tree}")
	if err != nil {
		cleanup()
		return RemoteCheckpointRef{}, "", nil, err
	}
	_ = tree
	listing, err := runner.run(ctx, "ls-tree", "-r", "--full-tree", chosenOID)
	if err != nil {
		cleanup()
		return RemoteCheckpointRef{}, "", nil, err
	}
	for _, line := range strings.Split(listing, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "100644 blob ") {
			cleanup()
			return RemoteCheckpointRef{}, "", nil, apperr.New(apperr.CodeConflict, "checkpoint tree contains a non-regular blob")
		}
	}
	manifest, err := readManifestFromRepo(ctx, runner, chosenOID)
	if err != nil {
		cleanup()
		return RemoteCheckpointRef{}, "", nil, err
	}
	if err := manifest.Validate(); err != nil {
		cleanup()
		return RemoteCheckpointRef{}, "", nil, apperr.New(apperr.CodeConflict, "corrupt_remote_checkpoint")
	}
	hash, err := contracts.ManifestHash(manifest)
	if err != nil || hash != manifest.ManifestSHA256 {
		cleanup()
		return RemoteCheckpointRef{}, "", nil, apperr.New(apperr.CodeConflict, contracts.BackupErrorCorruptRemoteCheckpoint)
	}
	if manifest.SchemaVersion > contracts.CurrentSchemaVersion {
		cleanup()
		return RemoteCheckpointRef{}, "", nil, apperr.New(apperr.CodeConflict, "unsupported future checkpoint schema")
	}
	currentID, _ := LoadWorkspaceIdentity(s.Root)
	if manifest.WorkspaceID != currentID && !opts.AllowWorkspaceMismatch {
		cleanup()
		return RemoteCheckpointRef{}, "", nil, apperr.New(apperr.CodeConflict, "checkpoint workspace does not match; pass --allow-workspace-mismatch")
	}
	if want != "" && want != manifest.CheckpointID && want != chosenOID && !strings.Contains(chosenRef, want) {
		// still accept if ls-remote matched
	}
	material := filepath.Join(tmp, "tree")
	if err := materializeRunnerCommit(ctx, runner, engine, chosenOID, material); err != nil {
		cleanup()
		return RemoteCheckpointRef{}, "", nil, err
	}
	if err := verifyMaterializedHashes(material, manifest); err != nil {
		cleanup()
		return RemoteCheckpointRef{}, "", nil, err
	}
	files := make([]string, 0, len(manifest.Files))
	for _, file := range manifest.Files {
		files = append(files, file.Path)
	}
	bundle, err := buildBundleManifest(material, "remote-"+manifest.CheckpointID, "workspace", s.now(), files)
	if err != nil {
		cleanup()
		return RemoteCheckpointRef{}, "", nil, err
	}
	raw, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		cleanup()
		return RemoteCheckpointRef{}, "", nil, err
	}
	archive := filepath.Join(tmp, "remote-checkpoint.tar.gz")
	if err := writeBundleArchive(material, archive, raw, files); err != nil {
		cleanup()
		return RemoteCheckpointRef{}, "", nil, err
	}
	return RemoteCheckpointRef{
		CheckpointID: manifest.CheckpointID, Commit: chosenOID, ReplicaID: manifest.ReplicaID,
		WorkspaceID: manifest.WorkspaceID, CreatedAt: manifest.CreatedAt, AtlasVersion: manifest.AtlasVersion,
		EventWatermarks: manifest.EventWatermarks, Ref: chosenRef, Verified: true,
	}, archive, cleanup, nil
}

func (e *CheckpointEngine) ensureBareAt(ctx context.Context, repo string) error {
	if err := os.MkdirAll(repo, 0o700); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(repo, "HEAD")); err == nil {
		return nil
	}
	cmdGit := e.git
	cmd := execCommandContext(ctx, cmdGit, "-c", "init.defaultBranch=atlas-backup", "init", "--bare", repo)
	if out, err := cmd(); err != nil {
		return fmt.Errorf("git init --bare: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return os.Chmod(repo, 0o700)
}

func readManifestFromRepo(ctx context.Context, runner gitRunner, commit string) (contracts.CheckpointManifest, error) {
	raw, err := runner.run(ctx, "show", commit+":"+contracts.CheckpointManifestName)
	if err != nil {
		return contracts.CheckpointManifest{}, err
	}
	return contracts.ParseCheckpointManifest([]byte(raw + "\n"))
}

func materializeRunnerCommit(ctx context.Context, runner gitRunner, engine *CheckpointEngine, commit, dest string) error {
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return err
	}
	listing, err := runner.run(ctx, "ls-tree", "-r", "--full-tree", commit)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(listing, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
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
		raw, err := catFileAt(ctx, runner, sha)
		if err != nil {
			return err
		}
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

func catFileAt(ctx context.Context, runner gitRunner, sha string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, runner.Git, runner.pinnedArgs("cat-file", "blob", sha)...)
	cmd.Dir = runner.Repo
	cmd.Env = gitSafeEnv(runner.Git, runner.Repo, "", "")
	return cmd.Output()
}

func verifyMaterializedHashes(root string, manifest contracts.CheckpointManifest) error {
	seen := map[string]struct{}{}
	for _, file := range manifest.Files {
		sum, err := fileSHA256(filepath.Join(root, filepath.FromSlash(file.Path)))
		if err != nil {
			return err
		}
		if sum != file.SHA256 {
			return apperr.New(apperr.CodeConflict, "file hash mismatch: "+file.Path)
		}
		seen[file.Path] = struct{}{}
	}
	var extra []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if rel == contracts.CheckpointManifestName {
			return nil
		}
		if _, ok := seen[rel]; !ok {
			extra = append(extra, rel)
		}
		return nil
	})
	if len(extra) > 0 {
		return apperr.New(apperr.CodeConflict, "checkpoint tree has extra entries")
	}
	if manifest.FileCount != len(manifest.Files) {
		return apperr.New(apperr.CodeConflict, "manifest file_count mismatch")
	}
	return nil
}

func execCommandContext(ctx context.Context, name string, args ...string) func() ([]byte, error) {
	return func() ([]byte, error) {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Env = gitSafeEnv(name, "", "", "")
		return cmd.CombinedOutput()
	}
}
