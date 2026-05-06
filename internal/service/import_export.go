package service

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

const (
	exportBundleFormatV1  = "atlas_bundle_v1"
	importMaxSourceBytes  = 64 << 20
	bundleArchiveMaxFiles = 2048
	bundleArchiveMaxPath  = 1024
	bundleArchiveMaxBytes = 128 << 20
	bundleArchiveMaxRatio = 200
)

type ExportBundleDetailView struct {
	Bundle      contracts.ExportBundle `json:"bundle"`
	FileCount   int                    `json:"file_count"`
	GeneratedAt time.Time              `json:"generated_at"`
}

type ExportVerifyView struct {
	BundleID       string    `json:"bundle_id,omitempty"`
	Path           string    `json:"path"`
	Verified       bool      `json:"verified"`
	ArchiveSHA256  string    `json:"archive_sha256,omitempty"`
	ManifestSHA256 string    `json:"manifest_sha256,omitempty"`
	Warnings       []string  `json:"warnings,omitempty"`
	Errors         []string  `json:"errors,omitempty"`
	GeneratedAt    time.Time `json:"generated_at"`
}

type ImportPlanItem struct {
	ProjectKey  string   `json:"project_key,omitempty"`
	ProjectName string   `json:"project_name,omitempty"`
	TicketID    string   `json:"ticket_id,omitempty"`
	Title       string   `json:"title,omitempty"`
	Type        string   `json:"type,omitempty"`
	Status      string   `json:"status,omitempty"`
	Priority    string   `json:"priority,omitempty"`
	SourceRef   string   `json:"source_ref,omitempty"`
	ParentRef   string   `json:"parent_ref,omitempty"`
	BlockedBy   []string `json:"blocked_by,omitempty"`
}

type ImportPlan struct {
	SourcePath  string                     `json:"source_path"`
	SourceType  contracts.ImportSourceType `json:"source_type"`
	Fingerprint string                     `json:"fingerprint"`
	Warnings    []string                   `json:"warnings,omitempty"`
	Errors      []string                   `json:"errors,omitempty"`
	Conflicts   []string                   `json:"conflicts,omitempty"`
	Items       []ImportPlanItem           `json:"items,omitempty"`
	FileCount   int                        `json:"file_count,omitempty"`
}

type ImportJobDetailView struct {
	Job         contracts.ImportJob `json:"job"`
	Plan        ImportPlan          `json:"plan"`
	GeneratedAt time.Time           `json:"generated_at"`
}

type bundleManifest struct {
	FormatVersion      string             `json:"format_version"`
	BundleID           string             `json:"bundle_id"`
	Scope              string             `json:"scope"`
	RedactionPreviewID string             `json:"redaction_preview_id,omitempty"`
	CreatedAt          time.Time          `json:"created_at"`
	Files              []bundleFileRecord `json:"files"`
}

type bundleFileRecord struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type githubImportRow struct {
	ProjectKey  string `json:"project_key"`
	ProjectName string `json:"project_name"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	Priority    string `json:"priority"`
	URL         string `json:"url"`
	Number      int    `json:"number"`
	Kind        string `json:"kind"`
}

func (s *ActionService) CreateExportBundle(ctx context.Context, scope string, actor contracts.Actor, reason string) (ExportBundleDetailView, error) {
	return withWriteLock(ctx, s.LockManager, "create export bundle", func(ctx context.Context) (ExportBundleDetailView, error) {
		if !actor.IsValid() {
			return ExportBundleDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		scope = strings.TrimSpace(scope)
		if scope == "" {
			scope = "workspace"
		}
		if scope != "workspace" {
			return ExportBundleDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("unsupported export scope: %s", scope))
		}
		governanceInput := GovernanceEvaluationInput{
			Action: contracts.ProtectedActionExportCreate,
			Target: "workspace",
			Actor:  actor,
			Reason: reason,
		}
		governanceExplanation, err := s.requireGovernance(ctx, governanceInput)
		if err != nil {
			return ExportBundleDetailView{}, err
		}
		bundleID := "bundle_" + NewOpaqueID()
		manifestPath := filepath.Join(storage.ExportsDir(s.Root), bundleID+".manifest.json")
		artifactPath := filepath.Join(storage.ExportsDir(s.Root), bundleID+".tar.gz")
		checksumPath := filepath.Join(storage.ExportsDir(s.Root), bundleID+".sha256")
		files, err := collectExportFiles(s.Root)
		if err != nil {
			return ExportBundleDetailView{}, err
		}
		manifest, err := buildBundleManifest(s.Root, bundleID, scope, s.now(), files)
		if err != nil {
			return ExportBundleDetailView{}, err
		}
		if err := os.MkdirAll(storage.ExportsDir(s.Root), 0o755); err != nil {
			return ExportBundleDetailView{}, fmt.Errorf("create exports dir: %w", err)
		}
		manifestRaw, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return ExportBundleDetailView{}, fmt.Errorf("marshal manifest: %w", err)
		}
		if err := os.WriteFile(manifestPath, append(manifestRaw, '\n'), 0o644); err != nil {
			return ExportBundleDetailView{}, fmt.Errorf("write manifest: %w", err)
		}
		if err := writeBundleArchive(s.Root, artifactPath, manifestRaw, files); err != nil {
			return ExportBundleDetailView{}, err
		}
		archiveSHA, err := fileSHA256(artifactPath)
		if err != nil {
			return ExportBundleDetailView{}, err
		}
		if err := os.WriteFile(checksumPath, []byte(archiveSHA+"  "+filepath.Base(artifactPath)+"\n"), 0o644); err != nil {
			return ExportBundleDetailView{}, fmt.Errorf("write checksum: %w", err)
		}
		bundle := contracts.ExportBundle{
			BundleID:      bundleID,
			Scope:         scope,
			Format:        exportBundleFormatV1,
			ArtifactPath:  artifactPath,
			ManifestPath:  manifestPath,
			ChecksumPath:  checksumPath,
			Status:        contracts.ExportBundleCreated,
			CreatedAt:     s.now(),
			SchemaVersion: contracts.CurrentSchemaVersion,
		}
		event, err := s.newEvent(ctx, workspaceProjectKey, s.now(), actor, reason, contracts.EventExportCreated, "", bundle)
		if err != nil {
			cleanupExportSidecars(manifestPath, artifactPath, checksumPath)
			return ExportBundleDetailView{}, err
		}
		if err := s.commitMutation(ctx, "create export bundle", "export_bundle", event, func(ctx context.Context) error {
			return s.ExportBundles.SaveExportBundle(ctx, bundle)
		}); err != nil {
			cleanupExportSidecars(manifestPath, artifactPath, checksumPath)
			return ExportBundleDetailView{}, err
		}
		if err := s.recordGovernanceOverrideIfApplied(ctx, governanceInput, governanceExplanation); err != nil {
			return ExportBundleDetailView{}, err
		}
		return ExportBundleDetailView{Bundle: bundle, FileCount: len(manifest.Files), GeneratedAt: s.now()}, nil
	})
}

func (s *ActionService) VerifyExportBundle(ctx context.Context, bundleRef string, actor contracts.Actor, reason string) (ExportVerifyView, error) {
	return withWriteLock(ctx, s.LockManager, "verify export bundle", func(ctx context.Context) (ExportVerifyView, error) {
		if !actor.IsValid() {
			return ExportVerifyView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		bundle, err := s.resolveExportBundle(ctx, bundleRef)
		if err != nil {
			return ExportVerifyView{}, err
		}
		view, err := verifyBundle(bundle)
		if err != nil {
			return ExportVerifyView{}, err
		}
		view.GeneratedAt = s.now()
		event, err := s.newEvent(ctx, workspaceProjectKey, s.now(), actor, reason, contracts.EventExportVerified, "", view)
		if err != nil {
			return ExportVerifyView{}, err
		}
		if err := s.commitMutation(ctx, "verify export bundle", "event_only", event, nil); err != nil {
			return ExportVerifyView{}, err
		}
		return view, nil
	})
}

func (s *ActionService) PreviewImport(ctx context.Context, sourcePath string, actor contracts.Actor, reason string) (ImportJobDetailView, error) {
	return withWriteLock(ctx, s.LockManager, "preview import", func(ctx context.Context) (ImportJobDetailView, error) {
		if !actor.IsValid() {
			return ImportJobDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		plan, err := buildImportPlan(s.Root, sourcePath)
		if err != nil {
			return ImportJobDetailView{}, err
		}
		jobID := "import_" + NewOpaqueID()
		logPath := filepath.Join(storage.ImportsDir(s.Root), jobID+".plan.json")
		if err := os.MkdirAll(storage.ImportsDir(s.Root), 0o755); err != nil {
			return ImportJobDetailView{}, fmt.Errorf("create imports dir: %w", err)
		}
		if err := writeImportPlan(logPath, plan); err != nil {
			return ImportJobDetailView{}, err
		}
		job := contracts.ImportJob{
			JobID:             jobID,
			SourceType:        plan.SourceType,
			Status:            contracts.ImportJobPreviewed,
			SourceFingerprint: plan.Fingerprint,
			Summary:           fmt.Sprintf("previewed %d items from %s", len(plan.Items), filepath.Base(plan.SourcePath)),
			Warnings:          append([]string{}, plan.Warnings...),
			Errors:            append(append([]string{}, plan.Errors...), plan.Conflicts...),
			ConflictLogPath:   logPath,
			CreatedAt:         s.now(),
			SchemaVersion:     contracts.CurrentSchemaVersion,
		}
		if err := s.saveImportJobStage(ctx, "preview import", actor, reason, contracts.EventImportPreviewed, job, false); err != nil {
			_ = os.Remove(logPath)
			return ImportJobDetailView{}, err
		}
		return ImportJobDetailView{Job: job, Plan: plan, GeneratedAt: s.now()}, nil
	})
}

func (s *ActionService) ApplyImport(ctx context.Context, jobID string, actor contracts.Actor, reason string) (ImportJobDetailView, error) {
	return withWriteLock(ctx, s.LockManager, "apply import", func(ctx context.Context) (ImportJobDetailView, error) {
		if !actor.IsValid() {
			return ImportJobDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		job, err := s.ImportJobs.LoadImportJob(ctx, jobID)
		if err != nil {
			return ImportJobDetailView{}, err
		}
		plan, err := readImportPlan(job.ConflictLogPath)
		if err != nil {
			return ImportJobDetailView{}, err
		}
		if len(plan.Errors) > 0 || len(plan.Conflicts) > 0 {
			job.Status = contracts.ImportJobFailed
			job.PartialApplied = false
			job.CompletedAt = s.now()
			job.Errors = append(append([]string{}, plan.Errors...), plan.Conflicts...)
			if err := s.saveImportJobStage(ctx, "reject import with preview conflicts", actor, reason, contracts.EventImportFailed, job, false); err != nil {
				return ImportJobDetailView{}, err
			}
			return ImportJobDetailView{Job: job, Plan: plan, GeneratedAt: s.now()}, apperr.New(apperr.CodeConflict, "import preview has conflicts or errors")
		}
		trustedSignatures := 0
		protectedAction := contracts.ProtectedActionImportApply
		if plan.SourceType == contracts.ImportSourceAtlasBundle {
			protectedAction = contracts.ProtectedActionBundleImportApply
			count, signatureErr := s.trustedExportBundleSignatureCount(ctx, plan.SourcePath)
			if signatureErr == nil {
				trustedSignatures = count
			}
		}
		governanceInput := GovernanceEvaluationInput{
			Action:                protectedAction,
			Target:                "workspace",
			Actor:                 actor,
			Reason:                reason,
			TrustedSignatureCount: trustedSignatures,
		}
		governanceExplanation, err := s.requireGovernance(ctx, governanceInput)
		if err != nil {
			return ImportJobDetailView{}, err
		}
		job.Status = contracts.ImportJobValidated
		if err := s.saveImportJobStage(ctx, "validate import", actor, reason, contracts.EventImportValidated, job, false); err != nil {
			return ImportJobDetailView{}, err
		}
		job.Status = contracts.ImportJobApplying
		if err := s.saveImportJobStage(ctx, "start import", actor, reason, contracts.EventImportStarted, job, false); err != nil {
			return ImportJobDetailView{}, err
		}
		applyErr := func() error {
			switch plan.SourceType {
			case contracts.ImportSourceAtlasBundle:
				return applyAtlasBundleImport(ctx, s.Root, plan)
			case contracts.ImportSourceJiraCSV:
				return applyStructuredImport(ctx, s, plan, actor, reason)
			case contracts.ImportSourceGitHubExport:
				return applyStructuredImport(ctx, s, plan, actor, reason)
			default:
				return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("unsupported import source: %s", plan.SourceType))
			}
		}()
		if applyErr != nil {
			job.Status = contracts.ImportJobFailed
			job.PartialApplied = true
			job.CompletedAt = s.now()
			job.Errors = append(job.Errors, applyErr.Error())
			if err := s.saveImportJobStage(ctx, "fail import", actor, reason, contracts.EventImportFailed, job, false); err != nil {
				return ImportJobDetailView{}, err
			}
		} else {
			job.Status = contracts.ImportJobApplied
			job.PartialApplied = false
			job.CompletedAt = s.now()
			if err := s.saveImportJobStage(ctx, "apply import", actor, reason, contracts.EventImportApplied, job, true); err != nil {
				return ImportJobDetailView{}, err
			}
		}
		if applyErr != nil {
			return ImportJobDetailView{Job: job, Plan: plan, GeneratedAt: s.now()}, applyErr
		}
		if err := s.recordGovernanceOverrideIfApplied(ctx, governanceInput, governanceExplanation); err != nil {
			return ImportJobDetailView{}, err
		}
		return ImportJobDetailView{Job: job, Plan: plan, GeneratedAt: s.now()}, nil
	})
}

func (s *QueryService) ListImportJobs(ctx context.Context) ([]contracts.ImportJob, error) {
	return s.ImportJobs.ListImportJobs(ctx)
}

func (s *QueryService) ImportJobDetail(ctx context.Context, jobID string) (ImportJobDetailView, error) {
	job, err := s.ImportJobs.LoadImportJob(ctx, jobID)
	if err != nil {
		return ImportJobDetailView{}, err
	}
	plan, err := readImportPlan(job.ConflictLogPath)
	if err != nil {
		return ImportJobDetailView{}, err
	}
	return ImportJobDetailView{Job: job, Plan: plan, GeneratedAt: s.now()}, nil
}

func (s *QueryService) ListExportBundles(ctx context.Context) ([]contracts.ExportBundle, error) {
	return s.ExportBundles.ListExportBundles(ctx)
}

func (s *QueryService) ExportBundleDetail(ctx context.Context, bundleID string) (ExportBundleDetailView, error) {
	bundle, err := s.ExportBundles.LoadExportBundle(ctx, bundleID)
	if err != nil {
		return ExportBundleDetailView{}, err
	}
	manifest, err := loadBundleManifest(bundle.ManifestPath)
	if err != nil {
		return ExportBundleDetailView{}, err
	}
	return ExportBundleDetailView{Bundle: bundle, FileCount: len(manifest.Files), GeneratedAt: s.now()}, nil
}

func (s *ActionService) resolveExportBundle(ctx context.Context, bundleRef string) (contracts.ExportBundle, error) {
	ref := strings.TrimSpace(bundleRef)
	if ref == "" {
		return contracts.ExportBundle{}, apperr.New(apperr.CodeInvalidInput, "bundle reference is required")
	}
	if exportBundleRefIsPath(ref) {
		sidecarBase := bundleSidecarBase(ref)
		base := filepath.Base(sidecarBase)
		manifestPath := sidecarBase + ".manifest.json"
		checksumPath := sidecarBase + ".sha256"
		bundleID := base
		scope := ""
		redactionPreviewID := ""
		if manifest, err := loadBundleManifest(manifestPath); err == nil {
			if strings.TrimSpace(manifest.BundleID) != "" {
				bundleID = manifest.BundleID
			}
			scope = manifest.Scope
			redactionPreviewID = manifest.RedactionPreviewID
		}
		if stored, err := s.ExportBundles.LoadExportBundle(ctx, base); err == nil {
			stored.ArtifactPath = ref
			stored.ManifestPath = manifestPath
			stored.ChecksumPath = checksumPath
			if stored.RedactionPreviewID == "" {
				stored.RedactionPreviewID = redactionPreviewID
			}
			if signatures, err := readExportSignatureSidecar(ref, stored.BundleID); err != nil {
				return contracts.ExportBundle{}, err
			} else {
				stored.SignatureEnvelopes = mergeSignatureEnvelopes(stored.SignatureEnvelopes, signatures)
			}
			return stored, nil
		}
		bundle := contracts.ExportBundle{
			BundleID:           bundleID,
			Format:             exportBundleFormatV1,
			Scope:              scope,
			ArtifactPath:       ref,
			ManifestPath:       manifestPath,
			ChecksumPath:       checksumPath,
			RedactionPreviewID: redactionPreviewID,
			Status:             contracts.ExportBundleCreated,
		}
		if signatures, err := readExportSignatureSidecar(ref, bundle.BundleID); err != nil {
			return contracts.ExportBundle{}, err
		} else {
			bundle.SignatureEnvelopes = signatures
		}
		return bundle, nil
	}
	bundle, err := s.ExportBundles.LoadExportBundle(ctx, ref)
	if err != nil {
		return contracts.ExportBundle{}, err
	}
	if signatures, err := readExportSignatureSidecar(bundle.ArtifactPath, bundle.BundleID); err != nil {
		return contracts.ExportBundle{}, err
	} else {
		bundle.SignatureEnvelopes = mergeSignatureEnvelopes(bundle.SignatureEnvelopes, signatures)
	}
	return bundle, nil
}

func collectExportFiles(root string) ([]string, error) {
	candidates := []string{
		"projects",
		filepath.ToSlash(filepath.Join(".tracker", "config.toml")),
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
	}
	files := make([]string, 0)
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		full := filepath.Join(root, filepath.FromSlash(candidate))
		info, err := os.Stat(full)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if !info.IsDir() {
			rel := filepath.ToSlash(candidate)
			files = append(files, rel)
			continue
		}
		walkErr := filepath.WalkDir(full, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if strings.Contains(filepath.ToSlash(path), "/runtime") || strings.Contains(filepath.ToSlash(path), "/archives") || strings.Contains(filepath.ToSlash(path), "/exports") || strings.Contains(filepath.ToSlash(path), "/mutations") {
					return filepath.SkipDir
				}
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if strings.HasSuffix(rel, ".log") || strings.HasSuffix(rel, "index.sqlite") {
				return nil
			}
			if strings.HasPrefix(rel, ".tracker/evidence/") && !strings.HasSuffix(rel, ".md") {
				return nil
			}
			if _, ok := seen[rel]; ok {
				return nil
			}
			seen[rel] = struct{}{}
			files = append(files, rel)
			return nil
		})
		if walkErr != nil {
			return nil, walkErr
		}
	}
	sort.Strings(files)
	return files, nil
}

func buildBundleManifest(root string, bundleID string, scope string, createdAt time.Time, files []string) (bundleManifest, error) {
	manifest := bundleManifest{
		FormatVersion: "v1",
		BundleID:      bundleID,
		Scope:         scope,
		CreatedAt:     createdAt,
		Files:         make([]bundleFileRecord, 0, len(files)),
	}
	for _, rel := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Stat(full)
		if err != nil {
			return bundleManifest{}, err
		}
		sum, err := fileSHA256(full)
		if err != nil {
			return bundleManifest{}, err
		}
		manifest.Files = append(manifest.Files, bundleFileRecord{Path: rel, SHA256: sum, Size: info.Size()})
	}
	return manifest, nil
}

func writeBundleArchive(root string, archivePath string, manifestRaw []byte, files []string) error {
	file, err := os.Create(archivePath)
	if err != nil {
		return fmt.Errorf("create bundle archive: %w", err)
	}
	defer file.Close()

	gz := gzip.NewWriter(file)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	if err := writeTarEntry(tw, "manifest.json", manifestRaw, timeNowUTC()); err != nil {
		return err
	}
	for _, rel := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		raw, err := os.ReadFile(full)
		if err != nil {
			return fmt.Errorf("read export file %s: %w", rel, err)
		}
		info, err := os.Stat(full)
		if err != nil {
			return fmt.Errorf("stat export file %s: %w", rel, err)
		}
		if err := writeTarEntry(tw, rel, raw, info.ModTime()); err != nil {
			return err
		}
	}
	return nil
}

func writeTarEntry(tw *tar.Writer, rel string, raw []byte, modTime time.Time) error {
	if err := tw.WriteHeader(&tar.Header{
		Name:    filepath.ToSlash(rel),
		Mode:    0o644,
		Size:    int64(len(raw)),
		ModTime: modTime,
	}); err != nil {
		return fmt.Errorf("write tar header %s: %w", rel, err)
	}
	if _, err := tw.Write(raw); err != nil {
		return fmt.Errorf("write tar entry %s: %w", rel, err)
	}
	return nil
}

func verifyBundle(bundle contracts.ExportBundle) (ExportVerifyView, error) {
	view := ExportVerifyView{BundleID: bundle.BundleID, Path: bundle.ArtifactPath, GeneratedAt: timeNowUTC()}
	manifest, manifestRaw, err := loadBundleManifestRaw(bundle.ManifestPath)
	if err != nil {
		return view, err
	}
	manifestHash := sha256.Sum256(manifestRaw)
	view.ManifestSHA256 = hex.EncodeToString(manifestHash[:])
	archiveSHA, err := fileSHA256(bundle.ArtifactPath)
	if err != nil {
		return view, err
	}
	view.ArchiveSHA256 = archiveSHA
	expectedArchiveSHA, err := readChecksumFile(bundle.ChecksumPath)
	if err != nil {
		return view, err
	}
	if expectedArchiveSHA != "" && expectedArchiveSHA != archiveSHA {
		view.Errors = append(view.Errors, "archive_checksum_mismatch")
	}
	files, err := readBundleArchive(bundle.ArtifactPath)
	if err != nil {
		return view, err
	}
	entries := map[string]bundleFileRecord{}
	for _, item := range manifest.Files {
		entries[item.Path] = item
	}
	for path, raw := range files {
		if path == "manifest.json" {
			continue
		}
		expected, ok := entries[path]
		if !ok {
			view.Errors = append(view.Errors, "unexpected_bundle_entry:"+path)
			continue
		}
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != expected.SHA256 {
			view.Errors = append(view.Errors, "manifest_checksum_mismatch:"+path)
		}
		delete(entries, path)
	}
	for path := range entries {
		view.Errors = append(view.Errors, "missing_bundle_entry:"+path)
	}
	view.Verified = len(view.Errors) == 0
	return view, nil
}

func buildImportPlan(root string, sourcePath string) (ImportPlan, error) {
	resolved, err := filepath.Abs(strings.TrimSpace(sourcePath))
	if err != nil {
		return ImportPlan{}, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return ImportPlan{}, err
	}
	if info.Size() > importMaxSourceBytes {
		return ImportPlan{}, apperr.New(apperr.CodeInvalidInput, "import source exceeds size limit")
	}
	sourceType, err := detectImportSource(resolved)
	if err != nil {
		return ImportPlan{}, err
	}
	switch sourceType {
	case contracts.ImportSourceAtlasBundle:
		return previewAtlasBundle(root, resolved)
	case contracts.ImportSourceJiraCSV:
		return previewJiraCSV(root, resolved)
	case contracts.ImportSourceGitHubExport:
		return previewGitHubExport(root, resolved)
	default:
		return ImportPlan{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("unsupported import source: %s", sourceType))
	}
}

func detectImportSource(path string) (contracts.ImportSourceType, error) {
	switch {
	case strings.HasSuffix(path, ".tar.gz"), strings.HasSuffix(path, ".tgz"):
		return contracts.ImportSourceAtlasBundle, nil
	case strings.HasSuffix(path, ".csv"):
		return contracts.ImportSourceJiraCSV, nil
	case strings.HasSuffix(path, ".json"):
		return contracts.ImportSourceGitHubExport, nil
	default:
		return "", apperr.New(apperr.CodeInvalidInput, "unsupported import source file")
	}
}

func previewAtlasBundle(root string, sourcePath string) (ImportPlan, error) {
	manifest, _, err := loadManifestFromArchive(sourcePath)
	if err != nil {
		return ImportPlan{}, err
	}
	fingerprint, err := fileSHA256(sourcePath)
	if err != nil {
		return ImportPlan{}, err
	}
	plan := ImportPlan{SourcePath: sourcePath, SourceType: contracts.ImportSourceAtlasBundle, Fingerprint: fingerprint, FileCount: len(manifest.Files)}
	for _, file := range manifest.Files {
		if skipAtlasBundleImportPath(file.Path) {
			continue
		}
		if invalidImportPath(file.Path) {
			plan.Errors = append(plan.Errors, "invalid_bundle_path:"+file.Path)
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(file.Path))); err == nil {
			plan.Conflicts = append(plan.Conflicts, "path_conflict:"+file.Path)
		}
		if projectKey, ticketID, ok := parseTicketPath(file.Path); ok {
			plan.Items = append(plan.Items, ImportPlanItem{ProjectKey: projectKey, TicketID: ticketID, SourceRef: file.Path})
		}
	}
	if hasAtlasBundleEventHistory(manifest.Files) {
		plan.Warnings = append(plan.Warnings, "atlas_bundle_event_history_not_imported")
	}
	sort.Slice(plan.Items, func(i, j int) bool {
		if plan.Items[i].ProjectKey == plan.Items[j].ProjectKey {
			return plan.Items[i].TicketID < plan.Items[j].TicketID
		}
		return plan.Items[i].ProjectKey < plan.Items[j].ProjectKey
	})
	plan.Conflicts = dedupeStrings(plan.Conflicts)
	plan.Errors = dedupeStrings(plan.Errors)
	return plan, nil
}

func previewJiraCSV(root string, sourcePath string) (ImportPlan, error) {
	rows, err := parseJiraCSV(sourcePath)
	if err != nil {
		return ImportPlan{}, err
	}
	fingerprint, err := fileSHA256(sourcePath)
	if err != nil {
		return ImportPlan{}, err
	}
	plan := ImportPlan{SourcePath: sourcePath, SourceType: contracts.ImportSourceJiraCSV, Fingerprint: fingerprint}
	for _, row := range rows {
		plan.Items = append(plan.Items, row)
		if row.ProjectKey == "" || row.TicketID == "" || row.Title == "" {
			plan.Errors = append(plan.Errors, "jira_row_missing_required_fields:"+row.SourceRef)
			continue
		}
		if _, err := os.Stat(storage.TicketFile(root, row.ProjectKey, row.TicketID)); err == nil {
			plan.Conflicts = append(plan.Conflicts, "ticket_conflict:"+row.TicketID)
		}
	}
	plan.Conflicts = dedupeStrings(plan.Conflicts)
	plan.Errors = dedupeStrings(plan.Errors)
	return plan, nil
}

func previewGitHubExport(root string, sourcePath string) (ImportPlan, error) {
	rows, err := parseGitHubExport(sourcePath)
	if err != nil {
		return ImportPlan{}, err
	}
	fingerprint, err := fileSHA256(sourcePath)
	if err != nil {
		return ImportPlan{}, err
	}
	plan := ImportPlan{SourcePath: sourcePath, SourceType: contracts.ImportSourceGitHubExport, Fingerprint: fingerprint, Items: rows}
	for _, row := range rows {
		if row.ProjectKey == "" || row.TicketID == "" || row.Title == "" {
			plan.Errors = append(plan.Errors, "github_row_missing_required_fields:"+row.SourceRef)
			continue
		}
		if _, err := os.Stat(storage.TicketFile(root, row.ProjectKey, row.TicketID)); err == nil {
			plan.Conflicts = append(plan.Conflicts, "ticket_conflict:"+row.TicketID)
		}
	}
	plan.Conflicts = dedupeStrings(plan.Conflicts)
	plan.Errors = dedupeStrings(plan.Errors)
	return plan, nil
}

func applyAtlasBundleImport(ctx context.Context, root string, plan ImportPlan) error {
	files, err := readBundleArchive(plan.SourcePath)
	if err != nil {
		return err
	}
	staging, err := os.MkdirTemp("", "atlas-import-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	for path, raw := range files {
		if path == "manifest.json" {
			continue
		}
		if skipAtlasBundleImportPath(path) {
			continue
		}
		if invalidImportPath(path) {
			return apperr.New(apperr.CodeInvalidInput, "path traversal detected in bundle")
		}
		target := filepath.Join(staging, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, raw, 0o644); err != nil {
			return err
		}
	}
	for path := range files {
		if path == "manifest.json" {
			continue
		}
		if skipAtlasBundleImportPath(path) {
			continue
		}
		target := filepath.Join(root, filepath.FromSlash(path))
		if _, err := os.Stat(target); err == nil {
			return apperr.New(apperr.CodeConflict, "import would overwrite existing path: "+path)
		}
	}
	for path := range files {
		if path == "manifest.json" {
			continue
		}
		if skipAtlasBundleImportPath(path) {
			continue
		}
		source := filepath.Join(staging, filepath.FromSlash(path))
		target := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		raw, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, raw, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func applyStructuredImport(ctx context.Context, s *ActionService, plan ImportPlan, actor contracts.Actor, reason string) error {
	projectNames := map[string]string{}
	createdTicketIDs := map[string]struct{}{}
	for _, item := range plan.Items {
		if strings.TrimSpace(item.ProjectKey) == "" {
			continue
		}
		projectNames[item.ProjectKey] = firstNonEmpty(strings.TrimSpace(item.ProjectName), item.ProjectKey)
	}
	for key, name := range projectNames {
		if _, err := s.Projects.GetProject(ctx, key); err == nil {
			continue
		}
		if err := s.CreateProject(ctx, contracts.Project{Key: key, Name: name, CreatedAt: s.now(), SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
			return err
		}
	}
	for _, item := range plan.Items {
		if item.ProjectKey == "" || item.TicketID == "" || item.Title == "" {
			continue
		}
		ticket := contracts.TicketSnapshot{
			ID:            item.TicketID,
			Project:       item.ProjectKey,
			Title:         item.Title,
			Type:          importTicketType(item.Type),
			Status:        importTicketStatus(item.Status),
			Priority:      importTicketPriority(item.Priority),
			Labels:        nil,
			CreatedAt:     s.now(),
			UpdatedAt:     s.now(),
			SchemaVersion: contracts.CurrentSchemaVersion,
			Notes:         strings.TrimSpace(item.SourceRef),
		}
		if err := ensureTicketDoesNotExist(ctx, s.Tickets, ticket.ID); err != nil {
			return err
		}
		if _, err := s.CreateTrackedTicket(ctx, ticket, actor, reason); err != nil {
			return err
		}
		createdTicketIDs[ticket.ID] = struct{}{}
	}
	for _, item := range plan.Items {
		if item.ParentRef == "" || item.TicketID == "" {
			continue
		}
		if _, ok := createdTicketIDs[item.TicketID]; !ok {
			continue
		}
		child, err := s.Tickets.GetTicket(ctx, item.TicketID)
		if err != nil {
			return err
		}
		child.Parent = item.ParentRef
		if _, err := s.SaveTrackedTicket(ctx, child, actor, "link imported parent"); err != nil {
			return err
		}
	}
	for _, item := range plan.Items {
		for _, blocker := range item.BlockedBy {
			if blocker == "" || item.TicketID == "" {
				continue
			}
			ticket, err := s.Tickets.GetTicket(ctx, item.TicketID)
			if err != nil {
				return err
			}
			if !slicesContains(ticket.BlockedBy, blocker) {
				ticket.BlockedBy = append(ticket.BlockedBy, blocker)
			}
			if _, err := s.SaveTrackedTicket(ctx, ticket, actor, "link imported blocker"); err != nil {
				return err
			}
		}
	}
	return nil
}

func ensureTicketDoesNotExist(ctx context.Context, store contracts.TicketStore, ticketID string) error {
	if _, err := store.GetTicket(ctx, ticketID); err == nil {
		return apperr.New(apperr.CodeConflict, "ticket already exists: "+ticketID)
	}
	return nil
}

func parseJiraCSV(sourcePath string) ([]ImportPlanItem, error) {
	file, err := os.Open(sourcePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, apperr.New(apperr.CodeInvalidInput, "malformed csv input")
	}
	if len(rows) == 0 {
		return []ImportPlanItem{}, nil
	}
	headers := mapHeaders(rows[0])
	items := make([]ImportPlanItem, 0, len(rows)-1)
	for _, row := range rows[1:] {
		value := func(names ...string) string {
			for _, name := range names {
				if idx, ok := headers[strings.ToLower(name)]; ok && idx < len(row) {
					return strings.TrimSpace(row[idx])
				}
			}
			return ""
		}
		ticketID := firstNonEmpty(value("issue key", "key"), value("issue id"))
		projectKey := value("project key")
		if projectKey == "" && strings.Contains(ticketID, "-") {
			projectKey = strings.TrimSpace(strings.SplitN(ticketID, "-", 2)[0])
		}
		item := ImportPlanItem{
			ProjectKey:  projectKey,
			ProjectName: value("project name", "project"),
			TicketID:    ticketID,
			Title:       firstNonEmpty(value("summary", "title"), "(untitled import)"),
			Type:        value("issue type", "type"),
			Status:      value("status"),
			Priority:    value("priority"),
			SourceRef:   ticketID,
			ParentRef:   firstNonEmpty(value("parent", "epic link"), value("parent issue key")),
			BlockedBy:   splitDelimited(firstNonEmpty(value("blocks"), value("blocked by"))),
		}
		items = append(items, item)
	}
	return items, nil
}

func parseGitHubExport(sourcePath string) ([]ImportPlanItem, error) {
	raw, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, err
	}
	var rows []githubImportRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, apperr.New(apperr.CodeInvalidInput, "malformed github export json")
	}
	items := make([]ImportPlanItem, 0, len(rows))
	for _, row := range rows {
		projectKey := strings.TrimSpace(row.ProjectKey)
		if projectKey == "" {
			projectKey = "GHI"
		}
		kind := firstNonEmpty(strings.TrimSpace(row.Kind), "issue")
		ticketID := fmt.Sprintf("%s-%d", projectKey, row.Number)
		items = append(items, ImportPlanItem{
			ProjectKey:  projectKey,
			ProjectName: firstNonEmpty(strings.TrimSpace(row.ProjectName), projectKey),
			TicketID:    ticketID,
			Title:       firstNonEmpty(strings.TrimSpace(row.Title), "(untitled import)"),
			Type:        kind,
			Status:      row.Status,
			Priority:    row.Priority,
			SourceRef:   row.URL,
		})
	}
	return items, nil
}

func writeImportPlan(path string, plan ImportPlan) error {
	raw, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal import plan: %w", err)
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

func readImportPlan(path string) (ImportPlan, error) {
	if strings.TrimSpace(path) == "" {
		return ImportPlan{}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return ImportPlan{}, err
	}
	var plan ImportPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return ImportPlan{}, err
	}
	return plan, nil
}

func loadBundleManifest(path string) (bundleManifest, error) {
	manifest, _, err := loadBundleManifestRaw(path)
	return manifest, err
}

func loadBundleManifestRaw(path string) (bundleManifest, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return bundleManifest{}, nil, err
	}
	var manifest bundleManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return bundleManifest{}, nil, err
	}
	if manifest.FormatVersion != "v1" {
		return bundleManifest{}, nil, apperr.New(apperr.CodeInvalidInput, "bundle manifest version is unsupported")
	}
	return manifest, raw, nil
}

func loadManifestFromArchive(path string) (bundleManifest, []byte, error) {
	files, err := readBundleArchive(path)
	if err != nil {
		return bundleManifest{}, nil, err
	}
	raw, ok := files["manifest.json"]
	if !ok {
		return bundleManifest{}, nil, apperr.New(apperr.CodeInvalidInput, "bundle manifest is missing")
	}
	var manifest bundleManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return bundleManifest{}, nil, apperr.New(apperr.CodeInvalidInput, "bundle manifest is invalid")
	}
	if manifest.FormatVersion != "v1" {
		return bundleManifest{}, nil, apperr.New(apperr.CodeInvalidInput, "bundle manifest version is unsupported")
	}
	return manifest, raw, nil
}

func readBundleArchive(path string) (map[string][]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	gz, err := gzip.NewReader(file)
	if err != nil {
		return nil, apperr.New(apperr.CodeInvalidInput, "bundle archive is not a valid gzip stream")
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	files := map[string][]byte{}
	totalExpanded := int64(0)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		name := filepath.ToSlash(strings.TrimSpace(header.Name))
		if invalidImportPath(name) {
			return nil, apperr.New(apperr.CodeInvalidInput, "path traversal detected in bundle")
		}
		if len(name) > bundleArchiveMaxPath {
			return nil, apperr.New(apperr.CodeInvalidInput, "bundle path exceeds length limit")
		}
		switch header.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
		case tar.TypeDir:
			continue
		case tar.TypeSymlink:
			return nil, apperr.New(apperr.CodeInvalidInput, "bundle_symlink_rejected: symlink entries are not allowed")
		case tar.TypeLink:
			return nil, apperr.New(apperr.CodeInvalidInput, "bundle_hardlink_rejected: hardlink entries are not allowed")
		case tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
			return nil, apperr.New(apperr.CodeInvalidInput, "bundle_device_rejected: special device entries are not allowed")
		default:
			return nil, apperr.New(apperr.CodeInvalidInput, "bundle entry type is not supported")
		}
		if header.Size > importMaxSourceBytes {
			return nil, apperr.New(apperr.CodeInvalidInput, "bundle entry exceeds size limit")
		}
		totalExpanded += header.Size
		if totalExpanded > bundleArchiveMaxBytes {
			return nil, apperr.New(apperr.CodeInvalidInput, "bundle archive exceeds expanded size limit")
		}
		if info.Size() > 0 && totalExpanded > info.Size()*bundleArchiveMaxRatio {
			return nil, apperr.New(apperr.CodeInvalidInput, "bundle archive exceeds compression ratio limit")
		}
		raw, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		if len(files) >= bundleArchiveMaxFiles {
			return nil, apperr.New(apperr.CodeInvalidInput, "bundle archive exceeds file count limit")
		}
		files[name] = raw
	}
	return files, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func readChecksumFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	parts := strings.Fields(string(raw))
	if len(parts) == 0 {
		return "", nil
	}
	return strings.TrimSpace(parts[0]), nil
}

func cleanupExportSidecars(paths ...string) {
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		_ = os.Remove(path)
	}
}

func bundleSidecarBase(path string) string {
	switch {
	case strings.HasSuffix(path, ".tar.gz"):
		return strings.TrimSuffix(path, ".tar.gz")
	case strings.HasSuffix(path, ".tgz"):
		return strings.TrimSuffix(path, ".tgz")
	default:
		return strings.TrimSuffix(path, filepath.Ext(path))
	}
}

func (s *ActionService) saveImportJobStage(ctx context.Context, purpose string, actor contracts.Actor, reason string, eventType contracts.EventType, job contracts.ImportJob, rebuildProjection bool) error {
	event, err := s.newEvent(ctx, workspaceProjectKey, s.now(), actor, reason, eventType, "", job)
	if err != nil {
		return err
	}
	return s.commitMutation(ctx, purpose, "import_job", event, func(ctx context.Context) error {
		if err := s.ImportJobs.SaveImportJob(ctx, job); err != nil {
			return err
		}
		if rebuildProjection && s.Projection != nil {
			return s.Projection.Rebuild(ctx, "")
		}
		return nil
	})
}

func invalidImportPath(path string) bool {
	path = filepath.ToSlash(strings.TrimSpace(path))
	return path == "" || strings.HasPrefix(path, "/") || strings.Contains(path, "..")
}

func skipAtlasBundleImportPath(path string) bool {
	path = filepath.ToSlash(strings.TrimSpace(path))
	return strings.HasPrefix(path, ".tracker/events/") || strings.HasPrefix(path, ".tracker/imports/")
}

func hasAtlasBundleEventHistory(files []bundleFileRecord) bool {
	for _, file := range files {
		if strings.HasPrefix(filepath.ToSlash(file.Path), ".tracker/events/") {
			return true
		}
	}
	return false
}

func parseTicketPath(path string) (string, string, bool) {
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) != 4 {
		return "", "", false
	}
	if parts[0] != "projects" || parts[2] != "tickets" || !strings.HasSuffix(parts[3], ".md") {
		return "", "", false
	}
	return parts[1], strings.TrimSuffix(parts[3], ".md"), true
}

func mapHeaders(row []string) map[string]int {
	headers := make(map[string]int, len(row))
	for idx, value := range row {
		headers[strings.ToLower(strings.TrimSpace(value))] = idx
	}
	return headers
}

func splitDelimited(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ';' })
	items := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			items = append(items, field)
		}
	}
	return items
}

func importTicketType(raw string) contracts.TicketType {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "epic":
		return contracts.TicketTypeEpic
	case "bug":
		return contracts.TicketTypeBug
	case "sub-task", "subtask":
		return contracts.TicketTypeSubtask
	default:
		return contracts.TicketTypeTask
	}
}

func importTicketStatus(raw string) contracts.Status {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "ready", "selected for development":
		return contracts.StatusReady
	case "in progress", "doing":
		return contracts.StatusInProgress
	case "in review", "review":
		return contracts.StatusInReview
	case "blocked":
		return contracts.StatusBlocked
	case "done", "closed", "resolved":
		return contracts.StatusDone
	case "canceled", "cancelled":
		return contracts.StatusCanceled
	default:
		return contracts.StatusBacklog
	}
}

func importTicketPriority(raw string) contracts.Priority {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "critical", "highest":
		return contracts.PriorityCritical
	case "high":
		return contracts.PriorityHigh
	case "low", "lowest":
		return contracts.PriorityLow
	default:
		return contracts.PriorityMedium
	}
}

func slicesContains(values []string, target string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == strings.TrimSpace(target) {
			return true
		}
	}
	return false
}
