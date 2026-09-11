package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

const (
	backupTargetsFormat = "atlas_backup_targets_v1"
	backupAutoFormat    = "atlas_backup_auto_v1"
)

// BackupTargetStore is the machine-local target list.
type BackupTargetStore struct {
	Format    string                   `json:"format"`
	Targets   []contracts.BackupTarget `json:"targets"`
	UpdatedAt time.Time                `json:"updated_at"`
}

// AutoBackupConfig is the machine-local enablement record.
type AutoBackupConfig struct {
	Format          string    `json:"format"`
	Enabled         bool      `json:"enabled"`
	DefaultTargetID string    `json:"default_target_id,omitempty"`
	EnabledAt       time.Time `json:"enabled_at,omitempty"`
	DisabledAt      time.Time `json:"disabled_at,omitempty"`
}

type BackupTargetAddOptions struct {
	TargetID            string
	URL                 string
	Enabled             bool
	Scope               string
	TimeoutMS           int
	VerificationPolicy  string
	AttestPrivate       bool
	AttestPublic        bool
	AllowPublicGitHub   bool
	AcknowledgeBoundary bool
	AllowLocalFile      bool
}

type BackupTargetView struct {
	Kind        string                 `json:"kind"`
	GeneratedAt time.Time              `json:"generated_at"`
	Target      contracts.BackupTarget `json:"target"`
	URLRedacted string                 `json:"url_redacted"`
	Warnings    []string               `json:"warnings,omitempty"`
}

type BackupTargetListView struct {
	Kind        string             `json:"kind"`
	GeneratedAt time.Time          `json:"generated_at"`
	Items       []BackupTargetView `json:"items"`
}

func (s *ActionService) backupPaths() (checkpointPaths, string, error) {
	stateDir, err := resolveUserStateDir(s.StateDir, s.Home)
	if err != nil {
		return checkpointPaths{}, "", err
	}
	workspaceID, err := LoadWorkspaceIdentity(s.Root)
	if err != nil {
		return checkpointPaths{}, "", err
	}
	if strings.TrimSpace(workspaceID) == "" {
		return checkpointPaths{}, "", apperr.New(apperr.CodeInvalidInput, "workspace identity is required")
	}
	if !validBackupWorkspaceID(workspaceID) {
		return checkpointPaths{}, "", apperr.New(apperr.CodeInvalidInput, "workspace identity is not a portable backup id")
	}
	paths := backupStatePaths(stateDir, workspaceID)
	if err := paths.ensure(); err != nil {
		return checkpointPaths{}, "", err
	}
	return paths, workspaceID, nil
}

func loadTargetStore(path string) (BackupTargetStore, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return BackupTargetStore{Format: backupTargetsFormat}, nil
		}
		return BackupTargetStore{}, err
	}
	var store BackupTargetStore
	if err := json.Unmarshal(raw, &store); err != nil {
		return BackupTargetStore{}, fmt.Errorf("parse backup targets: %w", err)
	}
	if store.Format == "" {
		store.Format = backupTargetsFormat
	}
	return store, nil
}

func loadAutoConfig(path string) (AutoBackupConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return AutoBackupConfig{Format: backupAutoFormat}, nil
		}
		return AutoBackupConfig{}, err
	}
	var cfg AutoBackupConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return AutoBackupConfig{}, fmt.Errorf("parse auto backup config: %w", err)
	}
	if cfg.Format == "" {
		cfg.Format = backupAutoFormat
	}
	return cfg, nil
}

func (s *ActionService) AddBackupTarget(ctx context.Context, opts BackupTargetAddOptions) (BackupTargetView, error) {
	_ = ctx
	if !opts.AcknowledgeBoundary {
		return BackupTargetView{}, apperr.New(apperr.CodeInvalidInput, "backup target add requires --acknowledge-data-boundary")
	}
	opts.URL = strings.TrimSpace(opts.URL)
	if err := contracts.ValidateBackupTargetURL(opts.URL); err != nil {
		return BackupTargetView{}, apperr.New(apperr.CodeInvalidInput, err.Error())
	}
	if strings.HasPrefix(opts.URL, "file://") && !opts.AllowLocalFile {
		return BackupTargetView{}, apperr.New(apperr.CodeInvalidInput, "file:// remotes are disposable local targets; pass --allow-local-file")
	}
	visibility := ""
	override := false
	switch {
	case opts.AttestPrivate && opts.AttestPublic:
		return BackupTargetView{}, apperr.New(apperr.CodeInvalidInput, "attest private or public, not both")
	case opts.AttestPrivate:
		visibility = contracts.BackupVisibilityPriv
	case opts.AttestPublic:
		visibility = contracts.BackupVisibilityPub
		override = opts.AllowPublicGitHub
	default:
		return BackupTargetView{}, apperr.New(apperr.CodeInvalidInput, "visibility attestation is required (--attest-private or --attest-public)")
	}
	if contracts.IsPublicGitHubHost(opts.URL) && visibility != contracts.BackupVisibilityPriv && !opts.AllowPublicGitHub {
		return BackupTargetView{}, apperr.New(apperr.CodeInvalidInput, "a confirmed public GitHub repository is refused unless --allow-public-github is independently approved")
	}
	if contracts.IsPublicGitHubHost(opts.URL) && visibility == contracts.BackupVisibilityPub && !opts.AllowPublicGitHub {
		return BackupTargetView{}, apperr.New(apperr.CodeInvalidInput, "public GitHub targets require --allow-public-github")
	}
	paths, _, err := s.backupPaths()
	if err != nil {
		return BackupTargetView{}, err
	}
	store, err := loadTargetStore(paths.Targets)
	if err != nil {
		return BackupTargetView{}, err
	}
	id := strings.TrimSpace(opts.TargetID)
	if id == "" {
		id = "target-" + NewOpaqueID()
	}
	for _, existing := range store.Targets {
		if existing.TargetID == id {
			return BackupTargetView{}, apperr.New(apperr.CodeConflict, "backup target already exists: "+id)
		}
	}
	scope := strings.TrimSpace(opts.Scope)
	if scope == "" {
		scope = "workspace"
	}
	policy := strings.TrimSpace(opts.VerificationPolicy)
	if policy == "" {
		policy = contracts.VerificationPolicyExactCommit
	}
	now := s.now()
	target := contracts.BackupTarget{
		Format:                 contracts.BackupTargetFormat,
		TargetID:               id,
		Type:                   contracts.BackupTargetTypeGit,
		URL:                    strings.TrimSpace(opts.URL),
		Enabled:                opts.Enabled,
		DefaultWorkspaceScope:  scope,
		VerificationPolicy:     policy,
		TransportTimeoutMS:     opts.TimeoutMS,
		VisibilityAttestation:  visibility,
		PublicOverrideApproved: override,
		DataBoundaryAck:        true,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	overlap, remotes, err := workspaceRemoteOverlap(s.Root, target.URL, s.GitPath)
	if err != nil {
		return BackupTargetView{}, err
	}
	_ = remotes
	target.OriginOverlap = overlap
	if err := target.Validate(); err != nil {
		return BackupTargetView{}, apperr.New(apperr.CodeInvalidInput, err.Error())
	}
	store.Targets = append(store.Targets, target)
	store.UpdatedAt = now
	if err := atomicWriteJSON(paths.Targets, store); err != nil {
		return BackupTargetView{}, err
	}
	return s.targetView(target), nil
}

func (s *ActionService) ListBackupTargets(ctx context.Context) (BackupTargetListView, error) {
	_ = ctx
	paths, _, err := s.backupPaths()
	if err != nil {
		return BackupTargetListView{}, err
	}
	store, err := loadTargetStore(paths.Targets)
	if err != nil {
		return BackupTargetListView{}, err
	}
	view := BackupTargetListView{Kind: "backup_target_list", GeneratedAt: s.now(), Items: make([]BackupTargetView, 0, len(store.Targets))}
	for _, target := range store.Targets {
		view.Items = append(view.Items, s.targetView(target))
	}
	return view, nil
}

func (s *ActionService) ViewBackupTarget(ctx context.Context, id string) (BackupTargetView, error) {
	_ = ctx
	target, err := s.loadTarget(id)
	if err != nil {
		return BackupTargetView{}, err
	}
	return s.targetView(target), nil
}

type BackupTargetEditOptions struct {
	Enabled             *bool
	URL                 string
	TimeoutMS           *int
	AcknowledgeBoundary bool
	AllowLocalFile      bool
}

func (s *ActionService) EditBackupTarget(ctx context.Context, id string, opts BackupTargetEditOptions) (BackupTargetView, error) {
	_ = ctx
	paths, _, err := s.backupPaths()
	if err != nil {
		return BackupTargetView{}, err
	}
	store, err := loadTargetStore(paths.Targets)
	if err != nil {
		return BackupTargetView{}, err
	}
	found := false
	for i, target := range store.Targets {
		if target.TargetID != id {
			continue
		}
		if opts.URL != "" {
			opts.URL = strings.TrimSpace(opts.URL)
			if err := contracts.ValidateBackupTargetURL(opts.URL); err != nil {
				return BackupTargetView{}, apperr.New(apperr.CodeInvalidInput, err.Error())
			}
			if strings.HasPrefix(opts.URL, "file://") && !opts.AllowLocalFile {
				return BackupTargetView{}, apperr.New(apperr.CodeInvalidInput, "file:// remotes are disposable local targets; pass --allow-local-file")
			}
			if contracts.IsPublicGitHubHost(opts.URL) && target.VisibilityAttestation != contracts.BackupVisibilityPriv && !target.PublicOverrideApproved {
				return BackupTargetView{}, apperr.New(apperr.CodeInvalidInput, "a confirmed public GitHub repository is refused unless --allow-public-github is independently approved")
			}
			target.URL = strings.TrimSpace(opts.URL)
			overlap, _, err := workspaceRemoteOverlap(s.Root, target.URL, s.GitPath)
			if err != nil {
				return BackupTargetView{}, err
			}
			target.OriginOverlap = overlap
		}
		if opts.Enabled != nil {
			target.Enabled = *opts.Enabled
		}
		if opts.TimeoutMS != nil {
			target.TransportTimeoutMS = *opts.TimeoutMS
		}
		if opts.AcknowledgeBoundary {
			target.DataBoundaryAck = true
		}
		target.UpdatedAt = s.now()
		if err := target.Validate(); err != nil {
			return BackupTargetView{}, apperr.New(apperr.CodeInvalidInput, err.Error())
		}
		store.Targets[i] = target
		found = true
		break
	}
	if !found {
		return BackupTargetView{}, apperr.New(apperr.CodeNotFound, "backup target not found: "+id)
	}
	store.UpdatedAt = s.now()
	if err := atomicWriteJSON(paths.Targets, store); err != nil {
		return BackupTargetView{}, err
	}
	return s.ViewBackupTarget(ctx, id)
}

func (s *ActionService) RemoveBackupTarget(ctx context.Context, id string) (BackupTargetView, error) {
	_ = ctx
	paths, _, err := s.backupPaths()
	if err != nil {
		return BackupTargetView{}, err
	}
	store, err := loadTargetStore(paths.Targets)
	if err != nil {
		return BackupTargetView{}, err
	}
	kept := store.Targets[:0]
	var removed contracts.BackupTarget
	found := false
	for _, target := range store.Targets {
		if target.TargetID == id {
			removed = target
			found = true
			continue
		}
		kept = append(kept, target)
	}
	if !found {
		return BackupTargetView{}, apperr.New(apperr.CodeNotFound, "backup target not found: "+id)
	}
	store.Targets = kept
	store.UpdatedAt = s.now()
	if err := atomicWriteJSON(paths.Targets, store); err != nil {
		return BackupTargetView{}, err
	}
	cfg, err := loadAutoConfig(paths.Auto)
	if err == nil && cfg.DefaultTargetID == id {
		cfg.DefaultTargetID = ""
		cfg.Enabled = false
		cfg.DisabledAt = s.now()
		_ = atomicWriteJSON(paths.Auto, cfg)
	}
	view := s.targetView(removed)
	view.Warnings = append(view.Warnings, "local_target_removed_remote_data_retained")
	return view, nil
}

func (s *ActionService) EnableAutoBackup(ctx context.Context, targetID string) (AutoBackupStatus, error) {
	_ = ctx
	target, err := s.loadTarget(targetID)
	if err != nil {
		return AutoBackupStatus{}, err
	}
	if !target.Enabled {
		enabled := true
		if _, err := s.EditBackupTarget(ctx, targetID, BackupTargetEditOptions{Enabled: &enabled}); err != nil {
			return AutoBackupStatus{}, err
		}
	}
	paths, workspaceID, err := s.backupPaths()
	if err != nil {
		return AutoBackupStatus{}, err
	}
	if _, err := s.ensureReplicaIdentity(paths, workspaceID, false); err != nil {
		return AutoBackupStatus{}, err
	}
	cfg := AutoBackupConfig{Format: backupAutoFormat, Enabled: true, DefaultTargetID: targetID, EnabledAt: s.now()}
	if err := atomicWriteJSON(paths.Auto, cfg); err != nil {
		return AutoBackupStatus{}, err
	}
	if ledger, err := loadLedger(paths.Ledger); err == nil && ledger.LastVerifiedTargetID != "" && ledger.LastVerifiedTargetID != targetID {
		ledger.LastVerifiedCommit = ""
		ledger.LastVerifiedAt = time.Time{}
		ledger.LastVerifiedTargetID = ""
		ledger.LastVerifiedManifestSHA = ""
		ledger.LastVerifiedTreeSHA = ""
		ledger.LastRemoteCheckpointID = ""
		_ = atomicWriteJSON(paths.Ledger, ledger)
	}
	return s.AutoBackupStatus(ctx)
}

func (s *ActionService) DisableAutoBackup(ctx context.Context) (AutoBackupStatus, error) {
	_ = ctx
	paths, _, err := s.backupPaths()
	if err != nil {
		return AutoBackupStatus{}, err
	}
	cfg, err := loadAutoConfig(paths.Auto)
	if err != nil {
		return AutoBackupStatus{}, err
	}
	cfg.Format = backupAutoFormat
	cfg.Enabled = false
	cfg.DisabledAt = s.now()
	if err := atomicWriteJSON(paths.Auto, cfg); err != nil {
		return AutoBackupStatus{}, err
	}
	return s.AutoBackupStatus(ctx)
}

func (s *ActionService) loadTarget(id string) (contracts.BackupTarget, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return contracts.BackupTarget{}, apperr.New(apperr.CodeInvalidInput, "target id is required")
	}
	paths, _, err := s.backupPaths()
	if err != nil {
		return contracts.BackupTarget{}, err
	}
	store, err := loadTargetStore(paths.Targets)
	if err != nil {
		return contracts.BackupTarget{}, err
	}
	for _, target := range store.Targets {
		if target.TargetID == id {
			return target, nil
		}
	}
	return contracts.BackupTarget{}, apperr.New(apperr.CodeNotFound, "backup target not found: "+id)
}

func (s *ActionService) defaultEnabledTarget() (contracts.BackupTarget, AutoBackupConfig, error) {
	paths, _, err := s.backupPaths()
	if err != nil {
		return contracts.BackupTarget{}, AutoBackupConfig{}, err
	}
	cfg, err := loadAutoConfig(paths.Auto)
	if err != nil {
		return contracts.BackupTarget{}, cfg, err
	}
	if !cfg.Enabled || strings.TrimSpace(cfg.DefaultTargetID) == "" {
		return contracts.BackupTarget{}, cfg, nil
	}
	target, err := s.loadTarget(cfg.DefaultTargetID)
	if err != nil {
		return contracts.BackupTarget{}, cfg, err
	}
	return target, cfg, nil
}

func (s *ActionService) targetView(target contracts.BackupTarget) BackupTargetView {
	view := BackupTargetView{
		Kind:        "backup_target",
		GeneratedAt: s.now(),
		Target:      target,
		URLRedacted: contracts.RedactBackupURL(target.URL),
	}
	view.Target.URL = contracts.RedactBackupURL(target.URL)
	if target.VisibilityAttestation != contracts.BackupVisibilityPriv {
		view.Warnings = append(view.Warnings, "target_privacy_unverified")
	} else {
		view.Warnings = append(view.Warnings, "target_privacy_attested_not_probed")
	}
	if target.OriginOverlap {
		view.Warnings = append(view.Warnings, "target_matches_workspace_remote")
	}
	if strings.HasPrefix(strings.TrimSpace(target.URL), "file://") || view.URLRedacted == "file://local" {
		view.Warnings = append(view.Warnings, "local_file_remote_is_disposable_not_off_device")
	}
	return view
}

func workspaceRemoteOverlap(root, targetURL, gitPath string) (bool, []string, error) {
	git := strings.TrimSpace(gitPath)
	if git == "" {
		looked, err := exec.LookPath("git")
		if err != nil {
			return false, nil, nil
		}
		git = looked
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return false, nil, nil
	}
	cmd := exec.Command(git, "-C", root, "remote", "-v")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, nil, nil
	}
	want := contracts.NormalizeBackupURL(targetURL)
	remotes := []string{}
	overlap := false
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		remotes = append(remotes, fields[0]+" "+contracts.RedactBackupURL(fields[1]))
		if contracts.NormalizeBackupURL(fields[1]) == want && want != "" {
			overlap = true
		}
	}
	return overlap, remotes, nil
}
