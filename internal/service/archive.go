package service

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

type ArchivePlanItem struct {
	Path       string                    `json:"path"`
	ProjectKey string                    `json:"project_key,omitempty"`
	Target     contracts.RetentionTarget `json:"target"`
	ItemRef    string                    `json:"item_ref,omitempty"`
	UpdatedAt  time.Time                 `json:"updated_at,omitempty"`
	SizeBytes  int64                     `json:"size_bytes,omitempty"`
	Reason     string                    `json:"reason,omitempty"`
}

type ArchivePlanView struct {
	Target      contracts.RetentionTarget `json:"target"`
	ProjectKey  string                    `json:"project_key,omitempty"`
	Policy      contracts.RetentionPolicy `json:"policy"`
	Items       []ArchivePlanItem         `json:"items,omitempty"`
	Warnings    []string                  `json:"warnings,omitempty"`
	TotalBytes  int64                     `json:"total_bytes,omitempty"`
	GeneratedAt time.Time                 `json:"generated_at"`
}

type ArchiveApplyResult struct {
	Record      contracts.ArchiveRecord `json:"record"`
	Warnings    []string                `json:"warnings,omitempty"`
	GeneratedAt time.Time               `json:"generated_at"`
}

type ArchiveRestoreResult struct {
	Record      contracts.ArchiveRecord `json:"record"`
	Warnings    []string                `json:"warnings,omitempty"`
	GeneratedAt time.Time               `json:"generated_at"`
}

type archiveCandidate struct {
	Path       string
	ProjectKey string
	ItemRef    string
	UpdatedAt  time.Time
	SizeBytes  int64
	Reason     string
}

func (s *QueryService) ArchivePlan(ctx context.Context, target contracts.RetentionTarget, projectKey string) (ArchivePlanView, error) {
	if !target.IsValid() {
		return ArchivePlanView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid archive target: %s", target))
	}
	policy, err := s.resolveRetentionPolicy(ctx, target, strings.TrimSpace(projectKey))
	if err != nil {
		return ArchivePlanView{}, err
	}
	candidates, warnings, err := s.archiveCandidates(ctx, target, strings.TrimSpace(projectKey))
	if err != nil {
		return ArchivePlanView{}, err
	}
	items, totalBytes := applyRetentionPolicy(policy, target, candidates, s.now())
	return ArchivePlanView{
		Target:      target,
		ProjectKey:  strings.TrimSpace(projectKey),
		Policy:      policy,
		Items:       items,
		Warnings:    warnings,
		TotalBytes:  totalBytes,
		GeneratedAt: s.now(),
	}, nil
}

func (s *QueryService) ListArchiveRecords(ctx context.Context, target contracts.RetentionTarget, projectKey string) ([]contracts.ArchiveRecord, error) {
	records, err := s.Archives.ListArchiveRecords(ctx)
	if err != nil {
		return nil, err
	}
	projectKey = strings.TrimSpace(projectKey)
	filtered := make([]contracts.ArchiveRecord, 0, len(records))
	for _, record := range records {
		if target != "" && record.Target != target {
			continue
		}
		if projectKey != "" && record.ProjectKey != projectKey {
			continue
		}
		filtered = append(filtered, record)
	}
	return filtered, nil
}

func (s *ActionService) ApplyArchive(ctx context.Context, target contracts.RetentionTarget, projectKey string, confirmed bool, actor contracts.Actor, reason string) (ArchiveApplyResult, error) {
	return withWriteLock(ctx, s.LockManager, "apply archive", func(ctx context.Context) (ArchiveApplyResult, error) {
		if !actor.IsValid() {
			return ArchiveApplyResult{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		queries := NewQueryService(s.Root, s.Projects, s.Tickets, s.Events, s.Projection, s.Clock)
		plan, err := queries.ArchivePlan(ctx, target, projectKey)
		if err != nil {
			return ArchiveApplyResult{}, err
		}
		if plan.Policy.RequiresConfirmation && !confirmed {
			return ArchiveApplyResult{}, apperr.New(apperr.CodeConflict, "archive apply requires --yes")
		}
		if len(plan.Items) == 0 {
			record := contracts.ArchiveRecord{
				ArchiveID:     "archive_" + NewOpaqueID(),
				Target:        target,
				Scope:         "workspace",
				ProjectKey:    strings.TrimSpace(projectKey),
				PayloadDir:    "",
				State:         contracts.ArchiveRecordArchived,
				Warnings:      append([]string{"no eligible archive candidates"}, plan.Warnings...),
				CreatedAt:     s.now(),
				SchemaVersion: contracts.CurrentSchemaVersion,
			}
			return ArchiveApplyResult{Record: record, Warnings: record.Warnings, GeneratedAt: s.now()}, nil
		}
		archiveID := "archive_" + NewOpaqueID()
		record := contracts.ArchiveRecord{
			ArchiveID:     archiveID,
			Target:        target,
			Scope:         "workspace",
			ProjectKey:    strings.TrimSpace(projectKey),
			SourcePaths:   make([]string, 0, len(plan.Items)),
			PayloadDir:    storage.ArchivePayloadDir(s.Root, archiveID),
			ItemCount:     len(plan.Items),
			TotalBytes:    plan.TotalBytes,
			State:         contracts.ArchiveRecordArchived,
			Warnings:      append([]string{}, plan.Warnings...),
			CreatedAt:     s.now(),
			SchemaVersion: contracts.CurrentSchemaVersion,
		}
		moved := make([]movedPath, 0, len(plan.Items))
		for _, item := range plan.Items {
			source := filepath.Join(s.Root, item.Path)
			dest := archivePayloadPath(s.Root, archiveID, item.Path)
			if err := movePath(source, dest); err != nil {
				rollbackMovedPaths(moved)
				return ArchiveApplyResult{}, err
			}
			record.SourcePaths = append(record.SourcePaths, item.Path)
			moved = append(moved, movedPath{from: source, to: dest})
		}
		event, err := s.newEvent(ctx, workspaceProjectKey, s.now(), actor, reason, contracts.EventArchiveApplied, "", record)
		if err != nil {
			rollbackMovedPaths(moved)
			return ArchiveApplyResult{}, err
		}
		if err := s.commitMutation(ctx, "apply archive", "archive_record", event, func(ctx context.Context) error {
			if err := s.applyArchiveMetadata(ctx, target, plan.Items, true); err != nil {
				return err
			}
			return s.Archives.SaveArchiveRecord(ctx, record)
		}); err != nil {
			_ = s.applyArchiveMetadata(ctx, target, plan.Items, false)
			rollbackMovedPaths(moved)
			return ArchiveApplyResult{}, err
		}
		return ArchiveApplyResult{Record: record, Warnings: record.Warnings, GeneratedAt: s.now()}, nil
	})
}

func (s *ActionService) RestoreArchive(ctx context.Context, archiveID string, actor contracts.Actor, reason string) (ArchiveRestoreResult, error) {
	return withWriteLock(ctx, s.LockManager, "restore archive", func(ctx context.Context) (ArchiveRestoreResult, error) {
		if !actor.IsValid() {
			return ArchiveRestoreResult{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		record, err := s.Archives.LoadArchiveRecord(ctx, archiveID)
		if err != nil {
			return ArchiveRestoreResult{}, err
		}
		if record.State == contracts.ArchiveRecordRestored {
			return ArchiveRestoreResult{Record: record, GeneratedAt: s.now()}, nil
		}
		for _, rel := range record.SourcePaths {
			if strings.TrimSpace(rel) == "" {
				continue
			}
			if _, err := os.Stat(filepath.Join(s.Root, rel)); err == nil {
				return ArchiveRestoreResult{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("restore target already exists: %s", rel))
			}
		}
		created := make([]string, 0, len(record.SourcePaths))
		for _, rel := range record.SourcePaths {
			source := archivePayloadPath(s.Root, record.ArchiveID, rel)
			if _, err := os.Stat(source); err != nil {
				cleanupCreatedPaths(created)
				return ArchiveRestoreResult{}, fmt.Errorf("restore source missing: %s", rel)
			}
			dest := filepath.Join(s.Root, rel)
			if err := copyPath(source, dest); err != nil {
				cleanupCreatedPaths(created)
				return ArchiveRestoreResult{}, err
			}
			created = append(created, dest)
		}
		record.State = contracts.ArchiveRecordRestored
		record.RestoredAt = s.now()
		event, err := s.newEvent(ctx, workspaceProjectKey, s.now(), actor, reason, contracts.EventArchiveRestored, "", record)
		if err != nil {
			cleanupCreatedPaths(created)
			return ArchiveRestoreResult{}, err
		}
		if err := s.commitMutation(ctx, "restore archive", "archive_record", event, func(ctx context.Context) error {
			if err := s.applyArchiveMetadata(ctx, record.Target, archivePlanItemsFromRecord(record), false); err != nil {
				return err
			}
			return s.Archives.SaveArchiveRecord(ctx, record)
		}); err != nil {
			_ = s.applyArchiveMetadata(ctx, record.Target, archivePlanItemsFromRecord(record), true)
			cleanupCreatedPaths(created)
			return ArchiveRestoreResult{}, err
		}
		return ArchiveRestoreResult{Record: record, Warnings: record.Warnings, GeneratedAt: s.now()}, nil
	})
}

func (s *QueryService) resolveRetentionPolicy(ctx context.Context, target contracts.RetentionTarget, projectKey string) (contracts.RetentionPolicy, error) {
	if strings.TrimSpace(projectKey) != "" && s.Projects != nil {
		project, err := s.Projects.GetProject(ctx, strings.TrimSpace(projectKey))
		if err == nil {
			for _, policyID := range project.Defaults.RetentionPolicies {
				policy, loadErr := s.RetentionPolicies.LoadRetentionPolicy(ctx, policyID)
				if loadErr != nil {
					return contracts.RetentionPolicy{}, loadErr
				}
				if policy.Target == target {
					return policy, nil
				}
			}
		}
	}
	return s.RetentionPolicies.LoadRetentionPolicy(ctx, defaultRetentionPolicyID(target))
}

func (s *QueryService) archiveCandidates(ctx context.Context, target contracts.RetentionTarget, projectKey string) ([]archiveCandidate, []string, error) {
	switch target {
	case contracts.RetentionTargetRuntime:
		return s.runtimeArchiveCandidates(ctx, projectKey)
	case contracts.RetentionTargetEvidenceArtifacts:
		return s.evidenceArchiveCandidates(ctx, projectKey)
	case contracts.RetentionTargetExportBundles:
		return s.exportArchiveCandidates(ctx)
	case contracts.RetentionTargetLogs:
		return s.logArchiveCandidates()
	default:
		return []archiveCandidate{}, []string{fmt.Sprintf("target %s has no concrete archive producer yet", target)}, nil
	}
}

func (s *QueryService) runtimeArchiveCandidates(ctx context.Context, projectKey string) ([]archiveCandidate, []string, error) {
	runs, err := s.Runs.ListRuns(ctx, "")
	if err != nil {
		return nil, nil, err
	}
	items := make([]archiveCandidate, 0, len(runs))
	for _, run := range runs {
		if strings.TrimSpace(projectKey) != "" && run.Project != strings.TrimSpace(projectKey) {
			continue
		}
		if run.Status != contracts.RunStatusCompleted && run.Status != contracts.RunStatusFailed && run.Status != contracts.RunStatusAborted && run.Status != contracts.RunStatusCleanedUp {
			continue
		}
		rel := filepath.Clean(filepath.Join(storage.TrackerDirName, "runtime", run.RunID))
		abs := filepath.Join(s.Root, rel)
		size, modTime, err := pathStats(abs)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, nil, err
		}
		runTime := run.CreatedAt
		runTime = maxTime(runTime, run.StartedAt)
		runTime = maxTime(runTime, run.LastHeartbeatAt)
		runTime = maxTime(runTime, run.CompletedAt)
		items = append(items, archiveCandidate{Path: rel, ProjectKey: run.Project, ItemRef: run.RunID, UpdatedAt: maxTime(modTime, runTime), SizeBytes: size, Reason: "completed runtime"})
	}
	return items, nil, nil
}

func (s *QueryService) evidenceArchiveCandidates(ctx context.Context, projectKey string) ([]archiveCandidate, []string, error) {
	runs, err := s.Runs.ListRuns(ctx, "")
	if err != nil {
		return nil, nil, err
	}
	runProjects := map[string]string{}
	for _, run := range runs {
		runProjects[run.RunID] = run.Project
	}
	entries, err := os.ReadDir(filepath.Join(storage.TrackerDir(s.Root), "evidence"))
	if err != nil {
		if os.IsNotExist(err) {
			return []archiveCandidate{}, nil, nil
		}
		return nil, nil, err
	}
	items := []archiveCandidate{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runID := entry.Name()
		project := runProjects[runID]
		if strings.TrimSpace(projectKey) != "" && project != strings.TrimSpace(projectKey) {
			continue
		}
		evidence, err := s.Evidence.ListEvidence(ctx, runID)
		if err != nil {
			return nil, nil, err
		}
		for _, item := range evidence {
			if strings.TrimSpace(item.ArtifactPath) == "" {
				continue
			}
			rel, ok := relativeWorkspacePath(s.Root, item.ArtifactPath)
			if !ok {
				continue
			}
			size, modTime, err := pathStats(filepath.Join(s.Root, rel))
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, nil, err
			}
			items = append(items, archiveCandidate{Path: rel, ProjectKey: project, ItemRef: item.EvidenceID, UpdatedAt: maxTime(modTime, item.CreatedAt), SizeBytes: size, Reason: "copied evidence artifact"})
		}
	}
	return items, nil, nil
}

func (s *QueryService) exportArchiveCandidates(ctx context.Context) ([]archiveCandidate, []string, error) {
	bundles, err := s.ExportBundles.ListExportBundles(ctx)
	if err != nil {
		return nil, nil, err
	}
	items := []archiveCandidate{}
	for _, bundle := range bundles {
		for _, path := range []string{bundle.ArtifactPath, bundle.ManifestPath, bundle.ChecksumPath} {
			if strings.TrimSpace(path) == "" {
				continue
			}
			rel, ok := relativeWorkspacePath(s.Root, path)
			if !ok {
				continue
			}
			size, modTime, err := pathStats(filepath.Join(s.Root, rel))
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, nil, err
			}
			items = append(items, archiveCandidate{Path: rel, ItemRef: bundle.BundleID, UpdatedAt: maxTime(modTime, bundle.CreatedAt), SizeBytes: size, Reason: "export bundle artifact"})
		}
	}
	return items, nil, nil
}

func (s *QueryService) logArchiveCandidates() ([]archiveCandidate, []string, error) {
	cfg, err := config.Load(s.Root)
	if err != nil {
		return nil, nil, err
	}
	items := []archiveCandidate{}
	for _, path := range []string{cfg.Notifications.FilePath, cfg.Notifications.DeliveryLogPath, cfg.Notifications.DeadLetterPath} {
		if strings.TrimSpace(path) == "" {
			continue
		}
		rel, ok := relativeWorkspacePath(s.Root, path)
		if !ok {
			continue
		}
		size, modTime, err := pathStats(filepath.Join(s.Root, rel))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, nil, err
		}
		items = append(items, archiveCandidate{Path: rel, ItemRef: filepath.Base(rel), UpdatedAt: modTime, SizeBytes: size, Reason: "notification log"})
	}
	return items, nil, nil
}

func applyRetentionPolicy(policy contracts.RetentionPolicy, target contracts.RetentionTarget, candidates []archiveCandidate, now time.Time) ([]ArchivePlanItem, int64) {
	if len(candidates) == 0 {
		return []ArchivePlanItem{}, 0
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].UpdatedAt.Equal(candidates[j].UpdatedAt) {
			return candidates[i].Path < candidates[j].Path
		}
		return candidates[i].UpdatedAt.After(candidates[j].UpdatedAt)
	})
	selected := map[string]archiveCandidate{}
	if policy.MaxAgeDays > 0 {
		cutoff := now.AddDate(0, 0, -policy.MaxAgeDays)
		for _, candidate := range candidates {
			if candidate.UpdatedAt.Before(cutoff) {
				selected[candidate.Path] = candidate
			}
		}
	}
	start := 0
	if policy.KeepLastN > 0 && len(candidates) > policy.KeepLastN {
		start = policy.KeepLastN
	}
	for _, candidate := range candidates[start:] {
		selected[candidate.Path] = candidate
	}
	if policy.MaxTotalSizeMB > 0 {
		limit := int64(policy.MaxTotalSizeMB) << 20
		var total int64
		for _, candidate := range candidates {
			total += candidate.SizeBytes
		}
		if total > limit {
			for i := len(candidates) - 1; i >= start && total > limit; i-- {
				selected[candidates[i].Path] = candidates[i]
				total -= candidates[i].SizeBytes
			}
		}
	}
	items := make([]ArchivePlanItem, 0, len(selected))
	var totalBytes int64
	for _, candidate := range selected {
		items = append(items, ArchivePlanItem{
			Path:       candidate.Path,
			ProjectKey: candidate.ProjectKey,
			Target:     target,
			ItemRef:    candidate.ItemRef,
			UpdatedAt:  candidate.UpdatedAt,
			SizeBytes:  candidate.SizeBytes,
			Reason:     candidate.Reason,
		})
		totalBytes += candidate.SizeBytes
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].Path < items[j].Path
		}
		return items[i].UpdatedAt.Before(items[j].UpdatedAt)
	})
	return items, totalBytes
}

func defaultRetentionPolicyID(target contracts.RetentionTarget) string {
	switch target {
	case contracts.RetentionTargetRuntime:
		return "runtime-default"
	case contracts.RetentionTargetEvidenceArtifacts:
		return "evidence-artifacts-default"
	case contracts.RetentionTargetExportBundles:
		return "export-bundles-default"
	case contracts.RetentionTargetLogs:
		return "logs-default"
	default:
		return "runtime-default"
	}
}

type movedPath struct {
	from string
	to   string
}

func rollbackMovedPaths(moved []movedPath) {
	for i := len(moved) - 1; i >= 0; i-- {
		_ = movePath(moved[i].to, moved[i].from)
	}
}

func movePath(source string, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create archive path: %w", err)
	}
	if err := os.Rename(source, dest); err == nil {
		return nil
	}
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("stat move source: %w", err)
	}
	if info.IsDir() {
		if err := copyDir(source, dest); err != nil {
			return err
		}
		return os.RemoveAll(source)
	}
	if err := copyFile(source, dest); err != nil {
		return err
	}
	return os.Remove(source)
}

func copyPath(source string, dest string) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("stat copy source: %w", err)
	}
	if info.IsDir() {
		return copyDir(source, dest)
	}
	return copyFile(source, dest)
}

func cleanupCreatedPaths(paths []string) {
	for i := len(paths) - 1; i >= 0; i-- {
		_ = os.RemoveAll(paths[i])
	}
}

func copyDir(source string, dest string) error {
	return filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(source string, dest string) error {
	in, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open copy source: %w", err)
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create copy target dir: %w", err)
	}
	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create copy target: %w", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy file: %w", err)
	}
	return out.Close()
}

func pathStats(path string) (int64, time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, time.Time{}, err
	}
	if !info.IsDir() {
		return info.Size(), info.ModTime().UTC(), nil
	}
	var total int64
	modTime := info.ModTime().UTC()
	err = filepath.Walk(path, func(child string, childInfo fs.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if childInfo.IsDir() {
			if childInfo.ModTime().After(modTime) {
				modTime = childInfo.ModTime().UTC()
			}
			return nil
		}
		total += childInfo.Size()
		if childInfo.ModTime().After(modTime) {
			modTime = childInfo.ModTime().UTC()
		}
		return nil
	})
	return total, modTime, err
}

func relativeWorkspacePath(root string, candidate string) (string, bool) {
	if strings.TrimSpace(candidate) == "" {
		return "", false
	}
	root = canonicalComparablePath(root)
	abs := candidate
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, candidate)
	}
	abs = canonicalComparablePath(abs)
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", false
	}
	rel = filepath.Clean(rel)
	if rel == "." || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return rel, true
}

func maxTime(a time.Time, b time.Time) time.Time {
	if a.After(b) {
		return a.UTC()
	}
	return b.UTC()
}

func archivePlanItemsFromRecord(record contracts.ArchiveRecord) []ArchivePlanItem {
	items := make([]ArchivePlanItem, 0, len(record.SourcePaths))
	for _, path := range record.SourcePaths {
		items = append(items, ArchivePlanItem{Path: path, Target: record.Target})
	}
	return items
}

func (s *ActionService) applyArchiveMetadata(ctx context.Context, target contracts.RetentionTarget, items []ArchivePlanItem, archived bool) error {
	if target != contracts.RetentionTargetExportBundles {
		return nil
	}
	bundles, err := s.ExportBundles.ListExportBundles(ctx)
	if err != nil {
		return err
	}
	touched := map[string]struct{}{}
	for _, item := range items {
		touched[item.Path] = struct{}{}
	}
	for _, bundle := range bundles {
		changed := false
		for _, path := range []string{bundle.ArtifactPath, bundle.ManifestPath, bundle.ChecksumPath} {
			rel, ok := relativeWorkspacePath(s.Root, path)
			if ok {
				if _, exists := touched[rel]; exists {
					changed = true
					break
				}
			}
		}
		if !changed {
			continue
		}
		if archived {
			bundle.Status = contracts.ExportBundleArchived
		} else {
			bundle.Status = contracts.ExportBundleCreated
		}
		if err := s.ExportBundles.SaveExportBundle(ctx, bundle); err != nil {
			return err
		}
	}
	return nil
}
