package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

const (
	syncBundleFormatV1 = "atlas_sync_bundle_v1"
	syncMigrationFile  = "migration.json"
	syncRefPrefix      = "refs/atlas-tasker/sync/"
)

type syncMigrationState struct {
	Complete      bool      `json:"complete"`
	WorkspaceID   string    `json:"workspace_id"`
	State         string    `json:"state,omitempty"`
	StampedAt     time.Time `json:"stamped_at"`
	SchemaVersion int       `json:"schema_version"`
}

func (s *QueryService) ListSyncRemotes(ctx context.Context) ([]contracts.SyncRemote, error) {
	items, err := s.SyncRemotes.ListSyncRemotes(ctx)
	if err != nil {
		return nil, err
	}
	return sanitizeSyncRemotes(items), nil
}

func (s *QueryService) SyncRemoteDetail(ctx context.Context, remoteID string) (SyncRemoteDetailView, error) {
	remote, err := s.SyncRemotes.LoadSyncRemote(ctx, remoteID)
	if err != nil {
		return SyncRemoteDetailView{}, err
	}
	publications, err := cachedRemotePublications(s.Root, remote.RemoteID)
	if err != nil {
		return SyncRemoteDetailView{}, err
	}
	return SyncRemoteDetailView{Remote: sanitizeSyncRemote(remote), Publications: publications, GeneratedAt: s.now()}, nil
}

func (s *QueryService) ListSyncJobs(ctx context.Context, remoteID string) ([]contracts.SyncJob, error) {
	return s.SyncJobs.ListSyncJobs(ctx, remoteID)
}

func (s *QueryService) SyncJobDetail(ctx context.Context, jobID string) (SyncJobDetailView, error) {
	job, err := s.SyncJobs.LoadSyncJob(ctx, jobID)
	if err != nil {
		return SyncJobDetailView{}, err
	}
	view := SyncJobDetailView{Job: job, GeneratedAt: s.now()}
	if strings.TrimSpace(job.RemoteID) != "" {
		if remote, err := s.SyncRemotes.LoadSyncRemote(ctx, job.RemoteID); err == nil {
			view.Remote = remote
		}
	}
	if strings.TrimSpace(job.BundleRef) != "" {
		if publication, err := inspectSyncBundle(job.BundleRef); err == nil {
			view.Publication = publication
		}
	}
	return view, nil
}

func (s *QueryService) ListBundleJobs(ctx context.Context) ([]contracts.SyncJob, error) {
	jobs, err := s.SyncJobs.ListSyncJobs(ctx, "")
	if err != nil {
		return nil, err
	}
	items := make([]contracts.SyncJob, 0, len(jobs))
	for _, job := range jobs {
		switch job.Mode {
		case contracts.SyncJobModeBundleCreate, contracts.SyncJobModeBundleImport, contracts.SyncJobModeBundleVerify:
			items = append(items, job)
		}
	}
	return items, nil
}

func (s *QueryService) BundleDetail(ctx context.Context, bundleRef string) (SyncJobDetailView, error) {
	if job, err := s.SyncJobs.LoadSyncJob(ctx, bundleRef); err == nil {
		return s.SyncJobDetail(ctx, job.JobID)
	}
	publication, err := inspectSyncBundle(resolveSyncBundlePath(s.Root, bundleRef))
	if err != nil {
		return SyncJobDetailView{}, err
	}
	job := contracts.SyncJob{
		JobID:         publication.BundleID,
		BundleRef:     resolveSyncBundlePath(s.Root, bundleRef),
		Mode:          contracts.SyncJobModeBundleCreate,
		State:         contracts.SyncJobStateCompleted,
		StartedAt:     publication.CreatedAt,
		FinishedAt:    publication.CreatedAt,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	return SyncJobDetailView{Job: job, Publication: publication, GeneratedAt: s.now()}, nil
}

func (s *QueryService) SyncStatus(ctx context.Context, remoteID string) (SyncStatusView, error) {
	migration, err := s.MigrationStatus(ctx)
	if err != nil {
		return SyncStatusView{}, err
	}
	remotes, err := s.SyncRemotes.ListSyncRemotes(ctx)
	if err != nil {
		return SyncStatusView{}, err
	}
	filterID := strings.TrimSpace(remoteID)
	items := make([]SyncStatusRemoteView, 0, len(remotes))
	for _, remote := range remotes {
		if filterID != "" && remote.RemoteID != filterID {
			continue
		}
		publications, err := cachedRemotePublications(s.Root, remote.RemoteID)
		if err != nil {
			return SyncStatusView{}, err
		}
		items = append(items, SyncStatusRemoteView{Remote: sanitizeSyncRemote(remote), Publications: publications})
	}
	return SyncStatusView{
		WorkspaceID:       migration.WorkspaceID,
		MigrationComplete: migration.Ready,
		Migration:         migration,
		ReasonCodes:       append([]string{}, migration.ReasonCodes...),
		Remotes:           items,
		GeneratedAt:       s.now(),
	}, nil
}

func (s *ActionService) AddSyncRemote(ctx context.Context, remote contracts.SyncRemote, actor contracts.Actor, reason string) (contracts.SyncRemote, error) {
	return withWriteLock(ctx, s.LockManager, "add sync remote", func(ctx context.Context) (contracts.SyncRemote, error) {
		if !actor.IsValid() {
			return contracts.SyncRemote{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		remote = normalizeSyncRemote(remote)
		location, err := normalizeSyncRemoteLocation(s.Root, remote.Kind, remote.Location)
		if err != nil {
			return contracts.SyncRemote{}, err
		}
		remote.Location = location
		if _, err := s.SyncRemotes.LoadSyncRemote(ctx, remote.RemoteID); err == nil {
			return contracts.SyncRemote{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("sync remote %s already exists", remote.RemoteID))
		}
		now := s.now()
		remote.CreatedAt = now
		remote.UpdatedAt = now
		event, err := s.newEvent(ctx, workspaceEventProject, now, actor, reason, contracts.EventRemoteAdded, "", remote)
		if err != nil {
			return contracts.SyncRemote{}, err
		}
		if err := s.commitMutation(ctx, "add sync remote", "sync_remote", event, func(ctx context.Context) error {
			return s.SyncRemotes.SaveSyncRemote(ctx, remote)
		}); err != nil {
			return contracts.SyncRemote{}, err
		}
		return remote, nil
	})
}

func (s *ActionService) EditSyncRemote(ctx context.Context, remoteID string, kind contracts.SyncRemoteKind, location string, defaultAction contracts.SyncDefaultAction, enabled bool, actor contracts.Actor, reason string) (contracts.SyncRemote, error) {
	return withWriteLock(ctx, s.LockManager, "edit sync remote", func(ctx context.Context) (contracts.SyncRemote, error) {
		if !actor.IsValid() {
			return contracts.SyncRemote{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		remote, err := s.SyncRemotes.LoadSyncRemote(ctx, remoteID)
		if err != nil {
			return contracts.SyncRemote{}, err
		}
		if kind != "" {
			remote.Kind = kind
		}
		if strings.TrimSpace(location) != "" {
			remote.Location = location
		}
		if defaultAction != "" {
			remote.DefaultAction = defaultAction
		}
		remote.Enabled = enabled
		normalized, err := normalizeSyncRemoteLocation(s.Root, remote.Kind, remote.Location)
		if err != nil {
			return contracts.SyncRemote{}, err
		}
		remote.Location = normalized
		remote.UpdatedAt = s.now()
		event, err := s.newEvent(ctx, workspaceEventProject, remote.UpdatedAt, actor, reason, contracts.EventRemoteEdited, "", remote)
		if err != nil {
			return contracts.SyncRemote{}, err
		}
		if err := s.commitMutation(ctx, "edit sync remote", "sync_remote", event, func(ctx context.Context) error {
			return s.SyncRemotes.SaveSyncRemote(ctx, remote)
		}); err != nil {
			return contracts.SyncRemote{}, err
		}
		return remote, nil
	})
}

func (s *ActionService) RemoveSyncRemote(ctx context.Context, remoteID string, actor contracts.Actor, reason string) error {
	_, err := withWriteLock(ctx, s.LockManager, "remove sync remote", func(ctx context.Context) (struct{}, error) {
		if !actor.IsValid() {
			return struct{}{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		remote, err := s.SyncRemotes.LoadSyncRemote(ctx, remoteID)
		if err != nil {
			return struct{}{}, err
		}
		event, err := s.newEvent(ctx, workspaceEventProject, s.now(), actor, reason, contracts.EventRemoteRemoved, "", remote)
		if err != nil {
			return struct{}{}, err
		}
		if err := s.commitMutation(ctx, "remove sync remote", "sync_remote", event, func(ctx context.Context) error {
			if err := os.Remove(storage.SyncRemoteFile(s.Root, remoteID)); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("delete sync remote %s: %w", remoteID, err)
			}
			return nil
		}); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, nil
	})
	return err
}

func (s *ActionService) CreateSyncBundle(ctx context.Context, actor contracts.Actor, reason string) (SyncJobDetailView, error) {
	return withWriteLock(ctx, s.LockManager, "create sync bundle", func(ctx context.Context) (SyncJobDetailView, error) {
		if !actor.IsValid() {
			return SyncJobDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		governanceInput := GovernanceEvaluationInput{
			Action: contracts.ProtectedActionExportCreate,
			Target: "workspace",
			Actor:  actor,
			Reason: reason,
		}
		governanceExplanation, err := s.requireGovernance(ctx, governanceInput)
		if err != nil {
			return SyncJobDetailView{}, err
		}
		workspaceID, err := ensureWorkspaceIdentity(s.Root)
		if err != nil {
			return SyncJobDetailView{}, err
		}
		if err := s.ensureSyncMigrationStamp(ctx); err != nil {
			return SyncJobDetailView{}, err
		}
		if err := s.ensureSyncMigrationReady(ctx); err != nil {
			return SyncJobDetailView{}, err
		}
		bundleID := "syncbundle_" + NewOpaqueID()
		artifactPath, manifestPath, checksumPath, publicationPath := syncBundlePaths(s.Root, bundleID)
		files, err := collectSyncableFiles(s.Root)
		if err != nil {
			return SyncJobDetailView{}, err
		}
		if err := os.MkdirAll(storage.SyncBundlesDir(s.Root), 0o755); err != nil {
			return SyncJobDetailView{}, fmt.Errorf("create sync bundles dir: %w", err)
		}
		manifest, err := buildBundleManifest(s.Root, bundleID, syncBundleFormatV1, s.now(), files)
		if err != nil {
			return SyncJobDetailView{}, err
		}
		manifestRaw, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return SyncJobDetailView{}, fmt.Errorf("marshal sync manifest: %w", err)
		}
		if err := os.WriteFile(manifestPath, append(manifestRaw, '\n'), 0o644); err != nil {
			return SyncJobDetailView{}, fmt.Errorf("write sync manifest: %w", err)
		}
		if err := writeBundleArchive(s.Root, artifactPath, manifestRaw, files); err != nil {
			cleanupExportSidecars(manifestPath, artifactPath)
			return SyncJobDetailView{}, err
		}
		archiveSHA, err := fileSHA256(artifactPath)
		if err != nil {
			cleanupExportSidecars(manifestPath, artifactPath)
			return SyncJobDetailView{}, err
		}
		if err := os.WriteFile(checksumPath, []byte(archiveSHA+"  "+filepath.Base(artifactPath)+"\n"), 0o644); err != nil {
			cleanupExportSidecars(manifestPath, artifactPath)
			return SyncJobDetailView{}, fmt.Errorf("write sync checksum: %w", err)
		}
		manifestHash := sha256.Sum256(manifestRaw)
		publication := SyncPublication{
			WorkspaceID:    workspaceID,
			BundleID:       bundleID,
			Format:         syncBundleFormatV1,
			CreatedAt:      s.now(),
			ArtifactName:   filepath.Base(artifactPath),
			ManifestName:   filepath.Base(manifestPath),
			ChecksumName:   filepath.Base(checksumPath),
			FileCount:      len(manifest.Files),
			ArchiveSHA256:  archiveSHA,
			ManifestSHA256: hex.EncodeToString(manifestHash[:]),
		}
		if err := writeSyncPublication(publicationPath, publication); err != nil {
			cleanupExportSidecars(manifestPath, artifactPath, checksumPath)
			return SyncJobDetailView{}, err
		}
		job := normalizeSyncJob(contracts.SyncJob{
			JobID:         bundleID,
			BundleRef:     artifactPath,
			Mode:          contracts.SyncJobModeBundleCreate,
			State:         contracts.SyncJobStateCompleted,
			StartedAt:     publication.CreatedAt,
			FinishedAt:    publication.CreatedAt,
			Counts:        map[string]int{"files": publication.FileCount},
			SchemaVersion: contracts.CurrentSchemaVersion,
		})
		event, err := s.newEvent(ctx, workspaceEventProject, publication.CreatedAt, actor, reason, contracts.EventBundleCreated, "", publication)
		if err != nil {
			return SyncJobDetailView{}, err
		}
		if err := s.commitMutation(ctx, "create sync bundle", "sync_job", event, func(ctx context.Context) error {
			return s.SyncJobs.SaveSyncJob(ctx, job)
		}); err != nil {
			return SyncJobDetailView{}, err
		}
		if err := s.recordGovernanceOverrideIfApplied(ctx, governanceInput, governanceExplanation); err != nil {
			return SyncJobDetailView{}, err
		}
		return SyncJobDetailView{Job: job, Publication: publication, GeneratedAt: s.now()}, nil
	})
}

func (s *ActionService) VerifySyncBundle(ctx context.Context, bundleRef string, actor contracts.Actor, reason string) (SyncBundleVerifyView, error) {
	return withWriteLock(ctx, s.LockManager, "verify sync bundle", func(ctx context.Context) (SyncBundleVerifyView, error) {
		if !actor.IsValid() {
			return SyncBundleVerifyView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		artifactPath := resolveSyncBundlePath(s.Root, bundleRef)
		view, err := verifySyncBundle(artifactPath)
		if err != nil {
			return SyncBundleVerifyView{}, err
		}
		job := normalizeSyncJob(contracts.SyncJob{
			JobID:         "verify_" + NewOpaqueID(),
			BundleRef:     artifactPath,
			Mode:          contracts.SyncJobModeBundleVerify,
			State:         contracts.SyncJobStateCompleted,
			StartedAt:     s.now(),
			FinishedAt:    s.now(),
			Warnings:      append([]string{}, view.Warnings...),
			ReasonCodes:   append([]string{}, view.Errors...),
			Counts:        map[string]int{"files": view.Publication.FileCount},
			SchemaVersion: contracts.CurrentSchemaVersion,
		})
		event, err := s.newEvent(ctx, workspaceEventProject, s.now(), actor, reason, contracts.EventBundleVerified, "", view)
		if err != nil {
			return SyncBundleVerifyView{}, err
		}
		if err := s.commitMutation(ctx, "verify sync bundle", "sync_job", event, func(ctx context.Context) error {
			return s.SyncJobs.SaveSyncJob(ctx, job)
		}); err != nil {
			return SyncBundleVerifyView{}, err
		}
		return view, nil
	})
}

func (s *ActionService) ImportSyncBundle(ctx context.Context, bundleRef string, actor contracts.Actor, reason string) (SyncJobDetailView, error) {
	return withWriteLock(ctx, s.LockManager, "import sync bundle", func(ctx context.Context) (SyncJobDetailView, error) {
		return s.importSyncBundleLocked(ctx, bundleRef, actor, reason, contracts.ProtectedActionBundleImportApply)
	})
}

func (s *ActionService) importSyncBundleLocked(ctx context.Context, bundleRef string, actor contracts.Actor, reason string, governanceAction contracts.ProtectedAction) (SyncJobDetailView, error) {
	if !actor.IsValid() {
		return SyncJobDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
	}
	artifactPath := resolveSyncBundlePath(s.Root, bundleRef)
	count, signatureErr := s.trustedSyncPublicationSignatureCount(ctx, artifactPath)
	trustedSignatures := 0
	if signatureErr == nil {
		trustedSignatures = count
	}
	var governanceInput GovernanceEvaluationInput
	var governanceExplanation contracts.GovernanceExplanation
	if governanceAction != "" {
		governanceInput = GovernanceEvaluationInput{
			Action:                       governanceAction,
			Target:                       "workspace",
			Actor:                        actor,
			Reason:                       reason,
			TrustedSignatureCount:        trustedSignatures,
			SignatureEvidenceUnavailable: signatureErr != nil,
		}
		var err error
		governanceExplanation, err = s.requireGovernance(ctx, governanceInput)
		if err != nil {
			return SyncJobDetailView{}, err
		}
	}
	if err := s.ensureSyncMigrationStamp(ctx); err != nil {
		return SyncJobDetailView{}, err
	}
	if err := s.ensureSyncMigrationReady(ctx); err != nil {
		return SyncJobDetailView{}, err
	}
	jobID := "import_" + NewOpaqueID()
	verifyView, err := verifySyncBundle(artifactPath)
	if err != nil {
		return SyncJobDetailView{}, err
	}
	if !verifyView.Verified {
		return SyncJobDetailView{}, apperr.New(apperr.CodeConflict, "sync bundle verification failed")
	}
	files, err := readBundleArchive(artifactPath)
	if err != nil {
		return SyncJobDetailView{}, err
	}
	applyResult, err := s.applyImportedSyncFiles(ctx, jobID, files, actor, reason)
	if err != nil {
		return SyncJobDetailView{}, err
	}
	workspaceID, err := ensureWorkspaceIdentity(s.Root)
	if err != nil {
		return SyncJobDetailView{}, err
	}
	if err := writeSyncMigrationState(syncMigrationPath(s.Root), syncMigrationState{Complete: true, WorkspaceID: workspaceID, State: string(MigrationStateStamped), StampedAt: s.now(), SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		return SyncJobDetailView{}, err
	}
	if s.Projection != nil {
		if err := s.Projection.Rebuild(ctx, ""); err != nil {
			return SyncJobDetailView{}, err
		}
	}
	job := normalizeSyncJob(contracts.SyncJob{
		JobID:         jobID,
		BundleRef:     artifactPath,
		Mode:          contracts.SyncJobModeBundleImport,
		State:         contracts.SyncJobStateCompleted,
		StartedAt:     s.now(),
		FinishedAt:    s.now(),
		Counts:        map[string]int{"files": verifyView.Publication.FileCount, "applied_files": applyResult.AppliedFiles},
		ConflictIDs:   append([]string{}, applyResult.ConflictIDs...),
		SchemaVersion: contracts.CurrentSchemaVersion,
	})
	if len(applyResult.ConflictIDs) > 0 {
		job.State = contracts.SyncJobStateFailed
		job.ReasonCodes = []string{"sync_conflicts_detected"}
		if err := saveSyncJobOnly(ctx, s.SyncJobs, job); err != nil {
			return SyncJobDetailView{}, err
		}
		return SyncJobDetailView{}, buildSyncConflictError(applyResult.ConflictIDs)
	}
	event, err := s.newEvent(ctx, workspaceEventProject, s.now(), actor, reason, contracts.EventBundleImported, "", verifyView.Publication)
	if err != nil {
		return SyncJobDetailView{}, err
	}
	if err := s.commitMutation(ctx, "import sync bundle", "sync_job", event, func(ctx context.Context) error {
		return s.SyncJobs.SaveSyncJob(ctx, job)
	}); err != nil {
		return SyncJobDetailView{}, err
	}
	if governanceAction != "" {
		if err := s.recordGovernanceOverrideIfApplied(ctx, governanceInput, governanceExplanation); err != nil {
			return SyncJobDetailView{}, err
		}
	}
	return SyncJobDetailView{Job: job, Publication: verifyView.Publication, GeneratedAt: s.now()}, nil
}

func (s *ActionService) SyncFetch(ctx context.Context, remoteID string, actor contracts.Actor, reason string) (SyncJobDetailView, error) {
	return withWriteLock(ctx, s.LockManager, "sync fetch", func(ctx context.Context) (SyncJobDetailView, error) {
		if !actor.IsValid() {
			return SyncJobDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		remote, err := s.SyncRemotes.LoadSyncRemote(ctx, remoteID)
		if err != nil {
			return SyncJobDetailView{}, err
		}
		if !remote.Enabled {
			return SyncJobDetailView{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("sync remote %s is disabled", remoteID))
		}
		publications, err := fetchRemotePublications(s.Root, remote)
		if err != nil {
			return SyncJobDetailView{}, err
		}
		job := normalizeSyncJob(contracts.SyncJob{
			JobID:         "sync_fetch_" + NewOpaqueID(),
			RemoteID:      remote.RemoteID,
			Mode:          contracts.SyncJobModeFetch,
			State:         contracts.SyncJobStateCompleted,
			StartedAt:     s.now(),
			FinishedAt:    s.now(),
			Counts:        map[string]int{"publications": len(publications)},
			SchemaVersion: contracts.CurrentSchemaVersion,
		})
		view, err := s.persistCompletedSyncJob(ctx, actor, reason, remote, job, SyncPublication{})
		if err != nil {
			return SyncJobDetailView{}, err
		}
		return view, nil
	})
}

func (s *ActionService) SyncPush(ctx context.Context, remoteID string, actor contracts.Actor, reason string) (SyncJobDetailView, error) {
	return withWriteLock(ctx, s.LockManager, "sync push", func(ctx context.Context) (SyncJobDetailView, error) {
		if !actor.IsValid() {
			return SyncJobDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		remote, err := s.SyncRemotes.LoadSyncRemote(ctx, remoteID)
		if err != nil {
			return SyncJobDetailView{}, err
		}
		if !remote.Enabled {
			return SyncJobDetailView{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("sync remote %s is disabled", remoteID))
		}
		bundleView, err := s.CreateSyncBundle(ctx, actor, reason)
		if err != nil {
			return SyncJobDetailView{}, err
		}
		if err := publishRemoteBundle(s.Root, remote, bundleView.Publication, bundleView.Job.BundleRef); err != nil {
			return SyncJobDetailView{}, err
		}
		job := normalizeSyncJob(contracts.SyncJob{
			JobID:         "sync_push_" + NewOpaqueID(),
			RemoteID:      remote.RemoteID,
			BundleRef:     bundleView.Job.BundleRef,
			Mode:          contracts.SyncJobModePush,
			State:         contracts.SyncJobStateCompleted,
			StartedAt:     s.now(),
			FinishedAt:    s.now(),
			Counts:        map[string]int{"files": bundleView.Publication.FileCount},
			SchemaVersion: contracts.CurrentSchemaVersion,
		})
		view, err := s.persistCompletedSyncJob(ctx, actor, reason, remote, job, bundleView.Publication)
		if err != nil {
			return SyncJobDetailView{}, err
		}
		return view, nil
	})
}

func (s *ActionService) SyncPull(ctx context.Context, remoteID string, sourceWorkspaceID string, actor contracts.Actor, reason string) (SyncJobDetailView, error) {
	return withWriteLock(ctx, s.LockManager, "sync pull", func(ctx context.Context) (SyncJobDetailView, error) {
		if !actor.IsValid() {
			return SyncJobDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		remote, err := s.SyncRemotes.LoadSyncRemote(ctx, remoteID)
		if err != nil {
			return SyncJobDetailView{}, err
		}
		if !remote.Enabled {
			return SyncJobDetailView{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("sync remote %s is disabled", remoteID))
		}
		stagingMirror := filepath.Join(storage.SyncStagingDir(s.Root), "pull-"+sanitizeSecurityID(remote.RemoteID)+"-"+NewOpaqueID())
		defer func() { _ = os.RemoveAll(stagingMirror) }()
		publications, err := fetchRemotePublicationsToMirror(s.Root, remote, stagingMirror)
		if err != nil {
			return SyncJobDetailView{}, err
		}
		publication, artifactPath, err := selectFetchedPublicationFromMirror(stagingMirror, sourceWorkspaceID, publications)
		if err != nil {
			return SyncJobDetailView{}, err
		}
		count, signatureErr := s.trustedSyncPublicationSignatureCount(ctx, artifactPath)
		trustedSignatures := 0
		if signatureErr == nil {
			trustedSignatures = count
		}
		governanceInput := GovernanceEvaluationInput{
			Action:                       contracts.ProtectedActionSyncImportApply,
			Target:                       "workspace",
			Actor:                        actor,
			Reason:                       reason,
			TrustedSignatureCount:        trustedSignatures,
			SignatureEvidenceUnavailable: signatureErr != nil,
		}
		governanceExplanation, err := s.requireGovernance(ctx, governanceInput)
		if err != nil {
			return SyncJobDetailView{}, err
		}
		if err := promoteFetchedPublication(storage.SyncMirrorRemoteDir(s.Root, remote.RemoteID), stagingMirror, publication); err != nil {
			return SyncJobDetailView{}, err
		}
		artifactPath = filepath.Join(storage.SyncMirrorRemoteDir(s.Root, remote.RemoteID), publication.WorkspaceID, publication.ArtifactName)
		imported, err := s.importSyncBundleLocked(ctx, artifactPath, actor, reason, "")
		if err != nil {
			if conflictIDs, ok := conflictIDsFromError(err); ok {
				job := normalizeSyncJob(contracts.SyncJob{
					JobID:         "sync_pull_" + NewOpaqueID(),
					RemoteID:      remote.RemoteID,
					BundleRef:     artifactPath,
					Mode:          contracts.SyncJobModePull,
					State:         contracts.SyncJobStateFailed,
					StartedAt:     s.now(),
					FinishedAt:    s.now(),
					ReasonCodes:   []string{"sync_conflicts_detected"},
					ConflictIDs:   append([]string{}, conflictIDs...),
					Counts:        map[string]int{"files": publication.FileCount},
					SchemaVersion: contracts.CurrentSchemaVersion,
				})
				if saveErr := saveSyncJobOnly(ctx, s.SyncJobs, job); saveErr != nil {
					return SyncJobDetailView{}, saveErr
				}
			}
			return SyncJobDetailView{}, err
		}
		job := normalizeSyncJob(contracts.SyncJob{
			JobID:         "sync_pull_" + NewOpaqueID(),
			RemoteID:      remote.RemoteID,
			BundleRef:     artifactPath,
			Mode:          contracts.SyncJobModePull,
			State:         contracts.SyncJobStateCompleted,
			StartedAt:     s.now(),
			FinishedAt:    s.now(),
			Counts:        map[string]int{"files": publication.FileCount},
			SchemaVersion: contracts.CurrentSchemaVersion,
		})
		view, err := s.persistCompletedSyncJob(ctx, actor, reason, remote, job, publication)
		if err != nil {
			return SyncJobDetailView{}, err
		}
		imported.Job = view.Job
		imported.Remote = view.Remote
		imported.Publication = view.Publication
		imported.GeneratedAt = view.GeneratedAt
		if err := s.recordGovernanceOverrideIfApplied(ctx, governanceInput, governanceExplanation); err != nil {
			return SyncJobDetailView{}, err
		}
		return imported, nil
	})
}

func (s *ActionService) SyncRun(ctx context.Context, remoteID string, sourceWorkspaceID string, actor contracts.Actor, reason string) (SyncJobDetailView, error) {
	remote, err := s.SyncRemotes.LoadSyncRemote(ctx, remoteID)
	if err != nil {
		return SyncJobDetailView{}, err
	}
	switch remote.DefaultAction {
	case contracts.SyncDefaultActionPull:
		return s.SyncPull(ctx, remoteID, sourceWorkspaceID, actor, reason)
	case contracts.SyncDefaultActionPush:
		return s.SyncPush(ctx, remoteID, actor, reason)
	default:
		return s.SyncFetch(ctx, remoteID, actor, reason)
	}
}

func (s *ActionService) persistCompletedSyncJob(ctx context.Context, _ contracts.Actor, _ string, remote contracts.SyncRemote, job contracts.SyncJob, publication SyncPublication) (SyncJobDetailView, error) {
	job = normalizeSyncJob(job)
	finishedAt := job.FinishedAt
	if finishedAt.IsZero() {
		finishedAt = s.now()
		job.FinishedAt = finishedAt
	}
	remote.LastSuccessAt = finishedAt
	remote.LastJobID = job.JobID
	remote.UpdatedAt = finishedAt
	if err := s.SyncJobs.SaveSyncJob(ctx, job); err != nil {
		return SyncJobDetailView{}, err
	}
	if err := s.SyncRemotes.SaveSyncRemote(ctx, remote); err != nil {
		return SyncJobDetailView{}, err
	}
	return SyncJobDetailView{Job: job, Remote: sanitizeSyncRemote(remote), Publication: publication, GeneratedAt: s.now()}, nil
}

func (s *ActionService) ensureSyncMigrationStamp(ctx context.Context) error {
	path := syncMigrationPath(s.Root)
	complete, err := syncMigrationComplete(s.Root)
	if err != nil {
		return err
	}
	if complete {
		return nil
	}
	projects, err := s.Projects.ListProjects(ctx)
	if err != nil {
		return err
	}
	for _, project := range projects {
		tickets, err := s.Tickets.ListTickets(ctx, contracts.TicketListOptions{Project: project.Key, IncludeArchived: true})
		if err != nil {
			return err
		}
		for _, ticket := range tickets {
			if err := s.Tickets.UpdateTicket(ctx, contracts.NormalizeTicketSnapshot(ticket)); err != nil {
				return err
			}
		}
	}
	collaborators, err := s.Collaborators.ListCollaborators(ctx)
	if err != nil {
		return err
	}
	for _, collaborator := range collaborators {
		if err := s.Collaborators.SaveCollaborator(ctx, collaborator); err != nil {
			return err
		}
	}
	memberships, err := s.Memberships.ListMemberships(ctx, "")
	if err != nil {
		return err
	}
	for _, membership := range memberships {
		if err := s.Memberships.SaveMembership(ctx, membership); err != nil {
			return err
		}
	}
	mentions, err := s.Mentions.ListMentions(ctx, "")
	if err != nil {
		return err
	}
	for _, mention := range mentions {
		if err := s.Mentions.SaveMention(ctx, mention); err != nil {
			return err
		}
	}
	runs, err := s.Runs.ListRuns(ctx, "")
	if err != nil {
		return err
	}
	for _, run := range runs {
		if err := s.Runs.SaveRun(ctx, run); err != nil {
			return err
		}
		evidence, err := s.Evidence.ListEvidence(ctx, run.RunID)
		if err != nil {
			return err
		}
		for _, item := range evidence {
			if err := s.Evidence.SaveEvidence(ctx, item); err != nil {
				return err
			}
		}
	}
	gates, err := s.Gates.ListGates(ctx, "")
	if err != nil {
		return err
	}
	for _, gate := range gates {
		if err := s.Gates.SaveGate(ctx, gate); err != nil {
			return err
		}
	}
	handoffs, err := s.Handoffs.ListHandoffs(ctx, "")
	if err != nil {
		return err
	}
	for _, handoff := range handoffs {
		if err := s.Handoffs.SaveHandoff(ctx, handoff); err != nil {
			return err
		}
	}
	changes, err := s.Changes.ListChanges(ctx, "")
	if err != nil {
		return err
	}
	for _, change := range changes {
		if err := s.Changes.SaveChange(ctx, change); err != nil {
			return err
		}
	}
	checks, err := s.Checks.ListChecks(ctx, "", "")
	if err != nil {
		return err
	}
	for _, check := range checks {
		if err := s.Checks.SaveCheck(ctx, check); err != nil {
			return err
		}
	}
	imports, err := s.ImportJobs.ListImportJobs(ctx)
	if err != nil {
		return err
	}
	for _, job := range imports {
		if err := s.ImportJobs.SaveImportJob(ctx, job); err != nil {
			return err
		}
	}
	exports, err := s.ExportBundles.ListExportBundles(ctx)
	if err != nil {
		return err
	}
	for _, bundle := range exports {
		if err := s.ExportBundles.SaveExportBundle(ctx, bundle); err != nil {
			return err
		}
	}
	retentionPolicies, err := s.RetentionPolicies.ListRetentionPolicies(ctx)
	if err != nil {
		return err
	}
	for _, policy := range retentionPolicies {
		if err := s.RetentionPolicies.SaveRetentionPolicy(ctx, policy); err != nil {
			return err
		}
	}
	archives, err := s.Archives.ListArchiveRecords(ctx)
	if err != nil {
		return err
	}
	for _, archive := range archives {
		if err := s.Archives.SaveArchiveRecord(ctx, archive); err != nil {
			return err
		}
	}
	if err := restampLegacyEventFiles(s.Root); err != nil {
		return err
	}
	workspaceID, err := ensureWorkspaceIdentity(s.Root)
	if err != nil {
		return err
	}
	return writeSyncMigrationState(path, syncMigrationState{Complete: true, WorkspaceID: workspaceID, State: string(MigrationStateStamped), StampedAt: s.now(), SchemaVersion: contracts.CurrentSchemaVersion})
}

func syncMigrationPath(root string) string {
	return filepath.Join(storage.SyncDir(root), syncMigrationFile)
}

func syncMigrationComplete(root string) (bool, error) {
	state, _, err := loadSyncMigrationState(root)
	if err != nil {
		return false, err
	}
	return state.Complete, nil
}

func writeSyncMigrationState(path string, state syncMigrationState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create sync migration dir: %w", err)
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode sync migration state: %w", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("write sync migration state: %w", err)
	}
	return nil
}

func syncBundlePaths(root string, bundleID string) (string, string, string, string) {
	base := filepath.Join(storage.SyncBundlesDir(root), bundleID)
	return base + ".tar.gz", base + ".manifest.json", base + ".sha256", base + ".publication.json"
}

func resolveSyncBundlePath(root string, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if filepath.IsAbs(ref) || strings.Contains(ref, string(filepath.Separator)) {
		return ref
	}
	artifactPath, _, _, _ := syncBundlePaths(root, ref)
	if _, err := os.Stat(artifactPath); err == nil {
		return artifactPath
	}
	return ref
}

func inspectSyncBundle(artifactPath string) (SyncPublication, error) {
	artifactPath = strings.TrimSpace(artifactPath)
	if artifactPath == "" {
		return SyncPublication{}, apperr.New(apperr.CodeInvalidInput, "bundle ref is required")
	}
	manifestPath := strings.TrimSuffix(artifactPath, ".tar.gz") + ".manifest.json"
	publicationPath := syncPublicationPathForArtifact(artifactPath)
	publication, err := readSyncPublication(publicationPath)
	if err == nil {
		return publication, nil
	}
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		return SyncPublication{}, fmt.Errorf("read sync manifest: %w", err)
	}
	var manifest bundleManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return SyncPublication{}, fmt.Errorf("decode sync manifest: %w", err)
	}
	archiveSHA, err := fileSHA256(artifactPath)
	if err != nil {
		return SyncPublication{}, err
	}
	manifestHash := sha256.Sum256(manifestRaw)
	return SyncPublication{
		BundleID:       manifest.BundleID,
		Format:         manifest.Scope,
		CreatedAt:      manifest.CreatedAt,
		ArtifactName:   filepath.Base(artifactPath),
		ManifestName:   filepath.Base(manifestPath),
		ChecksumName:   filepath.Base(strings.TrimSuffix(artifactPath, ".tar.gz") + ".sha256"),
		FileCount:      len(manifest.Files),
		ArchiveSHA256:  archiveSHA,
		ManifestSHA256: hex.EncodeToString(manifestHash[:]),
	}, nil
}

func syncPublicationPathForArtifact(artifactPath string) string {
	defaultPath := strings.TrimSuffix(artifactPath, ".tar.gz") + ".publication.json"
	if _, err := os.Stat(defaultPath); err == nil {
		return defaultPath
	}
	remotePath := filepath.Join(filepath.Dir(artifactPath), "publication.json")
	if publication, err := readSyncPublication(remotePath); err == nil && syncPublicationMatchesArtifact(publication, artifactPath) {
		return remotePath
	}
	return defaultPath
}

func syncPublicationMatchesArtifact(publication SyncPublication, artifactPath string) bool {
	artifactName := filepath.Base(artifactPath)
	manifestName := filepath.Base(strings.TrimSuffix(artifactPath, ".tar.gz") + ".manifest.json")
	checksumName := filepath.Base(strings.TrimSuffix(artifactPath, ".tar.gz") + ".sha256")
	if filepath.Base(publication.ArtifactName) != artifactName {
		return false
	}
	if strings.TrimSpace(publication.ManifestName) != "" && filepath.Base(publication.ManifestName) != manifestName {
		return false
	}
	if strings.TrimSpace(publication.ChecksumName) != "" && filepath.Base(publication.ChecksumName) != checksumName {
		return false
	}
	return true
}

func verifySyncBundle(artifactPath string) (SyncBundleVerifyView, error) {
	artifactPath = strings.TrimSpace(artifactPath)
	if artifactPath == "" {
		return SyncBundleVerifyView{}, apperr.New(apperr.CodeInvalidInput, "bundle ref is required")
	}
	publication, err := inspectSyncBundle(artifactPath)
	if err != nil {
		return SyncBundleVerifyView{}, err
	}
	manifestPath := strings.TrimSuffix(artifactPath, ".tar.gz") + ".manifest.json"
	checksumPath := strings.TrimSuffix(artifactPath, ".tar.gz") + ".sha256"
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		return SyncBundleVerifyView{}, fmt.Errorf("read sync manifest: %w", err)
	}
	var manifest bundleManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return SyncBundleVerifyView{}, fmt.Errorf("decode sync manifest: %w", err)
	}
	view := SyncBundleVerifyView{BundleRef: artifactPath, Publication: publication, GeneratedAt: timeNowUTC()}
	archiveSHA, err := fileSHA256(artifactPath)
	if err != nil {
		return view, err
	}
	expectedArchiveSHA, err := readChecksumFile(checksumPath)
	if err == nil && expectedArchiveSHA != "" && expectedArchiveSHA != archiveSHA {
		view.Errors = append(view.Errors, "bundle_checksum_mismatch")
	}
	files, err := readBundleArchive(artifactPath)
	if err != nil {
		return view, err
	}
	entries := map[string]bundleFileRecord{}
	for _, item := range manifest.Files {
		if !isSyncableRelativePath(item.Path) {
			view.Errors = append(view.Errors, "unsafe_sync_entry:"+item.Path)
			continue
		}
		entries[item.Path] = item
	}
	for path, raw := range files {
		if path == "manifest.json" {
			continue
		}
		if !isSyncableRelativePath(path) {
			view.Errors = append(view.Errors, "unsafe_sync_entry:"+path)
			continue
		}
		expected, ok := entries[path]
		if !ok {
			view.Errors = append(view.Errors, "unexpected_bundle_entry:"+path)
			continue
		}
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != expected.SHA256 {
			view.Errors = append(view.Errors, "bundle_manifest_mismatch:"+path)
		}
		delete(entries, path)
	}
	for path := range entries {
		view.Errors = append(view.Errors, "missing_bundle_entry:"+path)
	}
	view.Verified = len(view.Errors) == 0
	return view, nil
}

func fetchRemotePublications(root string, remote contracts.SyncRemote) ([]SyncPublication, error) {
	return fetchRemotePublicationsToMirror(root, remote, storage.SyncMirrorRemoteDir(root, remote.RemoteID))
}

func fetchRemotePublicationsToMirror(root string, remote contracts.SyncRemote, mirrorBase string) ([]SyncPublication, error) {
	switch remote.Kind {
	case contracts.SyncRemoteKindPath:
		return fetchPathRemotePublicationsToMirror(root, remote, mirrorBase)
	case contracts.SyncRemoteKindGit:
		return fetchGitRemotePublicationsToMirror(root, remote, mirrorBase)
	default:
		return nil, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("sync remote kind %s is not implemented yet", remote.Kind))
	}
}

func publishRemoteBundle(root string, remote contracts.SyncRemote, publication SyncPublication, artifactPath string) error {
	switch remote.Kind {
	case contracts.SyncRemoteKindPath:
		return publishPathRemoteBundle(root, remote, publication, artifactPath)
	case contracts.SyncRemoteKindGit:
		return publishGitRemoteBundle(root, remote, publication, artifactPath)
	default:
		return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("sync remote kind %s is not implemented yet", remote.Kind))
	}
}

func fetchPathRemotePublications(root string, remote contracts.SyncRemote) ([]SyncPublication, error) {
	return fetchPathRemotePublicationsToMirror(root, remote, storage.SyncMirrorRemoteDir(root, remote.RemoteID))
}

func fetchPathRemotePublicationsToMirror(_ string, remote contracts.SyncRemote, mirrorBase string) ([]SyncPublication, error) {
	entries, err := os.ReadDir(remote.Location)
	if err != nil {
		if os.IsNotExist(err) {
			return []SyncPublication{}, nil
		}
		return nil, fmt.Errorf("read path remote %s: %w", remote.RemoteID, err)
	}
	items := make([]SyncPublication, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		workspaceID := entry.Name()
		publicationPath := filepath.Join(remote.Location, workspaceID, "publication.json")
		publication, err := readSyncPublication(publicationPath)
		if err != nil {
			continue
		}
		publication.SourceRemoteID = remote.RemoteID
		publication.FetchedAt = timeNowUTC()
		mirrorDir := filepath.Join(mirrorBase, workspaceID)
		if err := os.MkdirAll(mirrorDir, 0o755); err != nil {
			return nil, fmt.Errorf("create sync mirror dir: %w", err)
		}
		for _, name := range []string{publication.ArtifactName, publication.ManifestName, publication.ChecksumName, "publication.json"} {
			if err := copySyncFile(filepath.Join(remote.Location, workspaceID, name), filepath.Join(mirrorDir, name)); err != nil {
				return nil, err
			}
		}
		if err := writeSyncPublication(filepath.Join(mirrorDir, "publication.json"), publication); err != nil {
			return nil, err
		}
		items = append(items, publication)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].WorkspaceID < items[j].WorkspaceID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items, nil
}

func publishPathRemoteBundle(root string, remote contracts.SyncRemote, publication SyncPublication, artifactPath string) error {
	remoteDir := filepath.Join(remote.Location, publication.WorkspaceID)
	if err := os.MkdirAll(remoteDir, 0o755); err != nil {
		return fmt.Errorf("create path remote publication dir: %w", err)
	}
	manifestPath := strings.TrimSuffix(artifactPath, ".tar.gz") + ".manifest.json"
	checksumPath := strings.TrimSuffix(artifactPath, ".tar.gz") + ".sha256"
	publicationPath := strings.TrimSuffix(artifactPath, ".tar.gz") + ".publication.json"
	for _, pair := range [][2]string{{artifactPath, filepath.Join(remoteDir, publication.ArtifactName)}, {manifestPath, filepath.Join(remoteDir, publication.ManifestName)}, {checksumPath, filepath.Join(remoteDir, publication.ChecksumName)}, {publicationPath, filepath.Join(remoteDir, "publication.json")}} {
		if err := copySyncFile(pair[0], pair[1]); err != nil {
			return err
		}
	}
	mirrorDir := filepath.Join(storage.SyncMirrorRemoteDir(root, remote.RemoteID), publication.WorkspaceID)
	if err := os.MkdirAll(mirrorDir, 0o755); err != nil {
		return fmt.Errorf("create sync mirror dir: %w", err)
	}
	for _, pair := range [][2]string{{artifactPath, filepath.Join(mirrorDir, publication.ArtifactName)}, {manifestPath, filepath.Join(mirrorDir, publication.ManifestName)}, {checksumPath, filepath.Join(mirrorDir, publication.ChecksumName)}, {publicationPath, filepath.Join(mirrorDir, "publication.json")}} {
		if err := copySyncFile(pair[0], pair[1]); err != nil {
			return err
		}
	}
	return nil
}

func fetchGitRemotePublications(root string, remote contracts.SyncRemote) ([]SyncPublication, error) {
	return fetchGitRemotePublicationsToMirror(root, remote, storage.SyncMirrorRemoteDir(root, remote.RemoteID))
}

func fetchGitRemotePublicationsToMirror(root string, remote contracts.SyncRemote, mirrorBase string) ([]SyncPublication, error) {
	repoDir, err := ensureSyncGitCache(mirrorBase, remote)
	if err != nil {
		return nil, err
	}
	refsOutput, err := gitSyncOutput(repoDir, nil, "ls-remote", "--refs", "origin", syncRefPrefix+"*")
	if err != nil {
		return nil, err
	}
	refsOutput = strings.TrimSpace(refsOutput)
	if refsOutput == "" {
		return []SyncPublication{}, nil
	}
	if _, err := gitSyncOutput(repoDir, nil, "fetch", "--prune", "origin", fmt.Sprintf("+%s*:refs/remotes/atlas-sync/*", syncRefPrefix)); err != nil {
		return nil, err
	}
	items := make([]SyncPublication, 0)
	for _, line := range strings.Split(refsOutput, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 {
			continue
		}
		refName := strings.TrimSpace(fields[1])
		workspaceID := strings.TrimPrefix(refName, syncRefPrefix)
		if strings.TrimSpace(workspaceID) == "" || workspaceID == refName {
			continue
		}
		localRef := "refs/remotes/atlas-sync/" + workspaceID
		publicationRaw, err := gitSyncShowFile(repoDir, localRef, "publication.json")
		if err != nil {
			return nil, err
		}
		var publication SyncPublication
		if err := json.Unmarshal(publicationRaw, &publication); err != nil {
			return nil, fmt.Errorf("decode git sync publication for %s: %w", workspaceID, err)
		}
		publication.SourceRemoteID = remote.RemoteID
		publication.SourceRef = refName
		publication.FetchedAt = timeNowUTC()
		mirrorDir := filepath.Join(mirrorBase, workspaceID)
		if err := os.MkdirAll(mirrorDir, 0o755); err != nil {
			return nil, fmt.Errorf("create git sync mirror dir: %w", err)
		}
		files := map[string][]byte{
			"publication.json": publicationRaw,
		}
		artifactRaw, err := gitSyncShowFile(repoDir, localRef, publication.ArtifactName)
		if err != nil {
			return nil, err
		}
		manifestRaw, err := gitSyncShowFile(repoDir, localRef, publication.ManifestName)
		if err != nil {
			return nil, err
		}
		checksumRaw, err := gitSyncShowFile(repoDir, localRef, publication.ChecksumName)
		if err != nil {
			return nil, err
		}
		files[publication.ArtifactName] = artifactRaw
		files[publication.ManifestName] = manifestRaw
		files[publication.ChecksumName] = checksumRaw
		for name, raw := range files {
			if err := os.WriteFile(filepath.Join(mirrorDir, name), raw, 0o644); err != nil {
				return nil, fmt.Errorf("write git sync mirror %s: %w", name, err)
			}
		}
		if err := writeSyncPublication(filepath.Join(mirrorDir, "publication.json"), publication); err != nil {
			return nil, err
		}
		items = append(items, publication)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].WorkspaceID < items[j].WorkspaceID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items, nil
}

func publishGitRemoteBundle(root string, remote contracts.SyncRemote, publication SyncPublication, artifactPath string) error {
	stagingDir := filepath.Join(storage.SyncStagingDir(root), "git-"+remote.RemoteID+"-"+publication.WorkspaceID)
	if err := os.RemoveAll(stagingDir); err != nil {
		return fmt.Errorf("reset git sync staging dir: %w", err)
	}
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return fmt.Errorf("create git sync staging dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(stagingDir) }()

	manifestPath := strings.TrimSuffix(artifactPath, ".tar.gz") + ".manifest.json"
	checksumPath := strings.TrimSuffix(artifactPath, ".tar.gz") + ".sha256"
	publicationPath := strings.TrimSuffix(artifactPath, ".tar.gz") + ".publication.json"
	for _, pair := range [][2]string{
		{artifactPath, filepath.Join(stagingDir, publication.ArtifactName)},
		{manifestPath, filepath.Join(stagingDir, publication.ManifestName)},
		{checksumPath, filepath.Join(stagingDir, publication.ChecksumName)},
		{publicationPath, filepath.Join(stagingDir, "publication.json")},
	} {
		if err := copySyncFile(pair[0], pair[1]); err != nil {
			return err
		}
	}
	if _, err := gitSyncOutput(stagingDir, nil, "init"); err != nil {
		return err
	}
	if _, err := gitSyncOutput(stagingDir, nil, "config", "user.email", "sync@atlas-tasker.local"); err != nil {
		return err
	}
	if _, err := gitSyncOutput(stagingDir, nil, "config", "user.name", "Atlas Sync"); err != nil {
		return err
	}
	if _, err := gitSyncOutput(stagingDir, nil, "add", "."); err != nil {
		return err
	}
	if _, err := gitSyncOutput(stagingDir, nil, "commit", "-m", "atlas sync publication "+publication.BundleID); err != nil {
		return err
	}
	if _, err := gitSyncOutput(stagingDir, nil, "push", "--force", remote.Location, "HEAD:"+syncRefPrefix+publication.WorkspaceID); err != nil {
		return err
	}
	mirrorDir := filepath.Join(storage.SyncMirrorRemoteDir(root, remote.RemoteID), publication.WorkspaceID)
	if err := os.MkdirAll(mirrorDir, 0o755); err != nil {
		return fmt.Errorf("create sync mirror dir: %w", err)
	}
	for _, pair := range [][2]string{
		{artifactPath, filepath.Join(mirrorDir, publication.ArtifactName)},
		{manifestPath, filepath.Join(mirrorDir, publication.ManifestName)},
		{checksumPath, filepath.Join(mirrorDir, publication.ChecksumName)},
		{publicationPath, filepath.Join(mirrorDir, "publication.json")},
	} {
		if err := copySyncFile(pair[0], pair[1]); err != nil {
			return err
		}
	}
	return nil
}

func cachedRemotePublications(root string, remoteID string) ([]SyncPublication, error) {
	base := storage.SyncMirrorRemoteDir(root, remoteID)
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return []SyncPublication{}, nil
		}
		return nil, fmt.Errorf("read sync mirror %s: %w", remoteID, err)
	}
	items := make([]SyncPublication, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		publication, err := readSyncPublication(filepath.Join(base, entry.Name(), "publication.json"))
		if err != nil {
			continue
		}
		items = append(items, publication)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].WorkspaceID < items[j].WorkspaceID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items, nil
}

func selectFetchedPublication(root string, remoteID string, sourceWorkspaceID string, publications []SyncPublication) (SyncPublication, string, error) {
	return selectFetchedPublicationFromMirror(storage.SyncMirrorRemoteDir(root, remoteID), sourceWorkspaceID, publications)
}

func selectFetchedPublicationFromMirror(mirrorBase string, sourceWorkspaceID string, publications []SyncPublication) (SyncPublication, string, error) {
	if len(publications) == 0 {
		return SyncPublication{}, "", apperr.New(apperr.CodeNotFound, "no publications fetched for remote")
	}
	if strings.TrimSpace(sourceWorkspaceID) != "" {
		for _, publication := range publications {
			if publication.WorkspaceID == strings.TrimSpace(sourceWorkspaceID) {
				artifactPath := filepath.Join(mirrorBase, publication.WorkspaceID, publication.ArtifactName)
				return publication, artifactPath, nil
			}
		}
		return SyncPublication{}, "", apperr.New(apperr.CodeNotFound, fmt.Sprintf("no fetched publication for workspace %s", sourceWorkspaceID))
	}
	if len(publications) > 1 {
		return SyncPublication{}, "", apperr.New(apperr.CodeInvalidInput, "multiple remote publications found; specify --workspace")
	}
	publication := publications[0]
	artifactPath := filepath.Join(mirrorBase, publication.WorkspaceID, publication.ArtifactName)
	return publication, artifactPath, nil
}

func promoteFetchedPublication(durableMirrorBase string, stagingMirrorBase string, publication SyncPublication) error {
	sourceDir := filepath.Join(stagingMirrorBase, publication.WorkspaceID)
	targetDir := filepath.Join(durableMirrorBase, publication.WorkspaceID)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create sync mirror dir: %w", err)
	}
	for _, name := range []string{publication.ArtifactName, publication.ManifestName, publication.ChecksumName} {
		if err := copySyncFile(filepath.Join(sourceDir, name), filepath.Join(targetDir, name)); err != nil {
			return err
		}
	}
	return writeSyncPublication(filepath.Join(targetDir, "publication.json"), publication)
}

func writeSyncPublication(path string, publication SyncPublication) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create sync publication dir: %w", err)
	}
	raw, err := json.MarshalIndent(publication, "", "  ")
	if err != nil {
		return fmt.Errorf("encode sync publication: %w", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("write sync publication: %w", err)
	}
	return nil
}

func readSyncPublication(path string) (SyncPublication, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return SyncPublication{}, fmt.Errorf("read sync publication: %w", err)
	}
	var publication SyncPublication
	if err := json.Unmarshal(raw, &publication); err != nil {
		return SyncPublication{}, fmt.Errorf("decode sync publication: %w", err)
	}
	return publication, nil
}

func copySyncFile(src string, dst string) error {
	from, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer from.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create dir for %s: %w", dst, err)
	}
	to, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer to.Close()
	if _, err := io.Copy(to, from); err != nil {
		return fmt.Errorf("copy %s -> %s: %w", src, dst, err)
	}
	return nil
}

func normalizeSyncRemoteLocation(root string, kind contracts.SyncRemoteKind, location string) (string, error) {
	location = strings.TrimSpace(location)
	if location == "" {
		return "", apperr.New(apperr.CodeInvalidInput, "remote location is required")
	}
	if kind == contracts.SyncRemoteKindGit {
		if remoteLocationHasEmbeddedCredentials(location) {
			return "", apperr.New(apperr.CodeInvalidInput, "git remote URL cannot embed credentials")
		}
		if parsed, err := url.Parse(location); err == nil && parsed.Scheme != "" {
			return location, nil
		}
	}
	if kind != contracts.SyncRemoteKindPath && kind != contracts.SyncRemoteKindGit {
		return location, nil
	}
	target := location
	if !filepath.IsAbs(location) {
		target = filepath.Join(root, location)
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve remote path: %w", err)
	}
	abs = canonicalCandidatePath(abs)
	workspaceRoot := canonicalCandidatePath(root)
	if abs == workspaceRoot || strings.HasPrefix(abs, workspaceRoot+string(filepath.Separator)) {
		return "", apperr.New(apperr.CodeInvalidInput, "path remote cannot point inside the workspace")
	}
	for _, blocked := range []string{storage.SyncMirrorDir(root), storage.SyncStagingDir(root), storage.TrackerDir(root)} {
		blocked = canonicalPath(blocked)
		if abs == blocked || strings.HasPrefix(abs, blocked+string(filepath.Separator)) {
			return "", apperr.New(apperr.CodeInvalidInput, "path remote cannot point at tracker mirror or staging state")
		}
	}
	return abs, nil
}

func canonicalCandidatePath(path string) string {
	clean := filepath.Clean(path)
	current := clean
	suffix := make([]string, 0, 4)
	for {
		if _, err := os.Stat(current); err == nil {
			resolved := canonicalPath(current)
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return clean
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func ensureSyncGitCache(mirrorBase string, remote contracts.SyncRemote) (string, error) {
	cacheDir := filepath.Join(mirrorBase, ".git-cache")
	if _, err := os.Stat(filepath.Join(cacheDir, ".git")); err == nil {
		if _, err := gitSyncOutput(cacheDir, nil, "remote", "set-url", "origin", remote.Location); err != nil {
			return "", err
		}
		return cacheDir, nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("stat git sync cache: %w", err)
	}
	if err := os.RemoveAll(cacheDir); err != nil {
		return "", fmt.Errorf("reset git sync cache: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(cacheDir), 0o755); err != nil {
		return "", fmt.Errorf("create git sync cache parent: %w", err)
	}
	if _, err := gitSyncOutput("", nil, "clone", "--no-checkout", remote.Location, cacheDir); err != nil {
		return "", err
	}
	return cacheDir, nil
}

func gitSyncShowFile(repoDir string, ref string, name string) ([]byte, error) {
	cmd := exec.Command("git", "show", ref+":"+name)
	cmd.Dir = repoDir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := redactRemoteSecrets(strings.TrimSpace(string(output)))
		if message == "" {
			message = redactRemoteSecrets(err.Error())
		}
		return nil, fmt.Errorf("git show %s:%s: %s", ref, name, message)
	}
	return output, nil
}

func gitSyncOutput(dir string, env []string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if strings.TrimSpace(dir) != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	cmd.Env = append(cmd.Env, env...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := redactRemoteSecrets(strings.TrimSpace(string(output)))
		if message == "" {
			message = redactRemoteSecrets(err.Error())
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), message)
	}
	return string(output), nil
}

func collectSyncableFiles(root string) ([]string, error) {
	tracker := storage.TrackerDir(root)
	files := make([]string, 0)
	walks := []string{
		storage.ProjectsDir(root),
		storage.CollaboratorsDir(root),
		storage.MembershipsDir(root),
		storage.MentionsDir(root),
		storage.RunsDir(root),
		storage.GatesDir(root),
		storage.HandoffsDir(root),
		filepath.Join(tracker, "evidence"),
		storage.ChangesDir(root),
		storage.ChecksDir(root),
		storage.PermissionProfilesDir(root),
		storage.ImportsDir(root),
		storage.ExportsDir(root),
		storage.RetentionPoliciesDir(root),
		storage.ArchivesDir(root),
		storage.PublicKeysDir(root),
		storage.RevocationsDir(root),
		storage.SignaturesDir(root),
		storage.GovernancePoliciesDir(root),
		storage.GovernancePacksDir(root),
		storage.ClassificationLabelsDir(root),
		storage.ClassificationPoliciesDir(root),
		storage.RedactionRulesDir(root),
		storage.AuditReportsDir(root),
		storage.AuditPacketsDir(root),
		storage.EventsDir(root),
	}
	for _, base := range walks {
		if _, err := os.Stat(base); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				if filepath.Clean(path) == filepath.Clean(storage.SyncDir(root)) || filepath.Clean(path) == filepath.Clean(storage.RuntimeDir(root, "")) {
					return filepath.SkipDir
				}
				if strings.HasPrefix(canonicalPath(path), canonicalPath(storage.ArchivesDir(root))+string(filepath.Separator)) && path != storage.ArchivesDir(root) && !strings.HasSuffix(path, ".md") {
					// archive payload dirs are local-only
				}
				return nil
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if isSyncableRelativePath(rel) {
				files = append(files, rel)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	return files, nil
}

func isSyncableRelativePath(rel string) bool {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	switch {
	case strings.HasPrefix(rel, "projects/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/collaborators/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/memberships/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/mentions/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/runs/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/gates/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/handoffs/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/evidence/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/changes/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/checks/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/permission-profiles/") && strings.HasSuffix(rel, ".toml"):
		return true
	case strings.HasPrefix(rel, ".tracker/imports/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/exports/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/retention/") && strings.HasSuffix(rel, ".toml"):
		return true
	case strings.HasPrefix(rel, ".tracker/archives/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/security/keys/public/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/security/revocations/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/security/signatures/") && strings.HasSuffix(rel, ".json"):
		return true
	case strings.HasPrefix(rel, ".tracker/governance/policies/") && strings.HasSuffix(rel, ".toml"):
		return true
	case strings.HasPrefix(rel, ".tracker/governance/packs/") && strings.HasSuffix(rel, ".toml"):
		return true
	case strings.HasPrefix(rel, ".tracker/classification/labels/") && strings.HasSuffix(rel, ".md"):
		return true
	case strings.HasPrefix(rel, ".tracker/classification/policies/") && strings.HasSuffix(rel, ".toml"):
		return true
	case strings.HasPrefix(rel, ".tracker/redaction/rules/") && strings.HasSuffix(rel, ".toml"):
		return true
	case strings.HasPrefix(rel, ".tracker/audit/reports/") && strings.HasSuffix(rel, ".json"):
		return true
	case strings.HasPrefix(rel, ".tracker/audit/packets/") && strings.HasSuffix(rel, ".json"):
		return true
	case strings.HasPrefix(rel, ".tracker/events/") && strings.HasSuffix(rel, ".jsonl"):
		return true
	default:
		return false
	}
}

func sanitizeSyncRemotes(items []contracts.SyncRemote) []contracts.SyncRemote {
	if len(items) == 0 {
		return []contracts.SyncRemote{}
	}
	sanitized := make([]contracts.SyncRemote, 0, len(items))
	for _, item := range items {
		sanitized = append(sanitized, sanitizeSyncRemote(item))
	}
	return sanitized
}

func sanitizeSyncRemote(remote contracts.SyncRemote) contracts.SyncRemote {
	remote.Location = redactRemoteSecrets(remote.Location)
	return remote
}

func redactRemoteSecrets(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	if idx := strings.Index(value, "@"); idx > 0 {
		prefix := value[:idx]
		if strings.Contains(prefix, ":") && !strings.Contains(prefix, "://") {
			parts := strings.SplitN(prefix, ":", 2)
			user := strings.TrimSpace(parts[0])
			if user == "" {
				user = "***"
			}
			value = user + ":***" + value[idx:]
		}
	}
	return config.MaskSecretsInText(value)
}

func remoteLocationHasEmbeddedCredentials(location string) bool {
	location = strings.TrimSpace(location)
	if location == "" {
		return false
	}
	if idx := strings.Index(location, "@"); idx > 0 && !strings.Contains(location[:idx], "://") {
		prefix := location[:idx]
		if strings.Contains(prefix, ":") {
			return true
		}
	}
	if parsed, err := url.Parse(location); err == nil && parsed.Scheme != "" {
		if parsed.User != nil {
			return true
		}
		for key := range parsed.Query() {
			lower := strings.ToLower(strings.TrimSpace(key))
			if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "key") || strings.Contains(lower, "password") {
				return true
			}
		}
		return false
	}
	return false
}
