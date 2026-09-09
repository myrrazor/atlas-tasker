package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

type CompactResult struct {
	RemovedPaths []string  `json:"removed_paths,omitempty"`
	SkippedPaths []string  `json:"skipped_paths,omitempty"`
	BytesFreed   int64     `json:"bytes_freed,omitempty"`
	GeneratedAt  time.Time `json:"generated_at"`
}

func (s *QueryService) CompactPlan(ctx context.Context) (CompactResult, error) {
	removed, bytesFreed, skipped, err := compactablePaths(ctx, s.Root)
	if err != nil {
		return CompactResult{}, err
	}
	return CompactResult{RemovedPaths: removed, SkippedPaths: skipped, BytesFreed: bytesFreed, GeneratedAt: s.now()}, nil
}

func (s *ActionService) CompactWorkspace(ctx context.Context, confirmed bool, actor contracts.Actor, reason string) (CompactResult, error) {
	return withWriteLock(ctx, s.LockManager, "compact workspace", func(ctx context.Context) (CompactResult, error) {
		if !actor.IsValid() {
			return CompactResult{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if !confirmed {
			return CompactResult{}, apperr.New(apperr.CodeConflict, "compact requires --yes")
		}
		removed, bytesFreed, skipped, err := compactablePaths(ctx, s.Root)
		if err != nil {
			return CompactResult{}, err
		}
		for _, path := range removed {
			if err := rejectSymlinkedFile(path); err != nil {
				return CompactResult{}, err
			}
			if !pathWithinDir(s.Root, path) {
				return CompactResult{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("path_rejected: compact path escapes workspace: %s", path))
			}
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return CompactResult{}, fmt.Errorf("remove compacted path %s: %w", filepath.Base(path), err)
			}
		}
		result := CompactResult{RemovedPaths: removed, SkippedPaths: skipped, BytesFreed: bytesFreed, GeneratedAt: s.now()}
		event, err := s.newEvent(ctx, workspaceProjectKey, s.now(), actor, reason, contracts.EventCompactCompleted, "", result)
		if err != nil {
			return CompactResult{}, err
		}
		if err := s.commitMutation(ctx, "compact workspace", "event_only", event, nil); err != nil {
			return CompactResult{}, err
		}
		return result, nil
	})
}

func compactablePaths(ctx context.Context, root string) ([]string, int64, []string, error) {
	paths := []string{}
	skipped := []string{}
	var bytesFreed int64

	queries := NewQueryService(root, nil, nil, nil, nil, timeNowUTC)
	runs, err := queries.Runs.ListRuns(ctx, "")
	if err != nil {
		return nil, 0, nil, err
	}
	for _, run := range runs {
		if run.Status == contracts.RunStatusActive || run.Status == contracts.RunStatusAttached || run.Status == contracts.RunStatusDispatched || run.Status == contracts.RunStatusAwaitingOwner || run.Status == contracts.RunStatusAwaitingReview || run.Status == contracts.RunStatusHandoffReady {
			skipped = append(skipped, "runtime:"+run.RunID)
			continue
		}
		for _, path := range []string{storage.RuntimeLaunchFile(root, run.RunID, "codex"), storage.RuntimeLaunchFile(root, run.RunID, "claude")} {
			safePath, err := compactSafeFile(root, path)
			if err != nil {
				skipped = append(skipped, "unsafe:"+filepath.Base(path))
				continue
			}
			if safePath == "" {
				continue
			}
			info, err := os.Lstat(safePath)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, 0, nil, err
			}
			if info.Mode()&os.ModeSymlink != 0 || info.IsDir() {
				skipped = append(skipped, "symlink:"+filepath.Base(safePath))
				continue
			}
			paths = append(paths, safePath)
			bytesFreed += info.Size()
		}
	}

	records, err := queries.Archives.ListArchiveRecords(ctx)
	if err != nil {
		return nil, 0, nil, err
	}
	archivesRoot := canonicalComparablePath(storage.ArchivesDir(root))
	for _, record := range records {
		if record.Target != contracts.RetentionTargetRuntime {
			continue
		}
		payloadDir := strings.TrimSpace(record.PayloadDir)
		if payloadDir == "" {
			skipped = append(skipped, "archive_payload:"+record.ArchiveID)
			continue
		}
		payloadDir = canonicalComparablePath(payloadDir)
		if !pathWithinDir(archivesRoot, payloadDir) {
			skipped = append(skipped, "archive_payload:"+record.ArchiveID)
			continue
		}
		if err := rejectSymlinkComponents(archivesRoot, payloadDir); err != nil {
			skipped = append(skipped, "archive_symlink:"+record.ArchiveID)
			continue
		}
		for _, rel := range record.SourcePaths {
			runID := filepath.Base(rel)
			for _, path := range []string{
				filepath.Join(payloadDir, storage.TrackerDirName, "runtime", runID, "launch.codex.txt"),
				filepath.Join(payloadDir, storage.TrackerDirName, "runtime", runID, "launch.claude.txt"),
			} {
				safePath, err := compactSafeFile(payloadDir, path)
				if err != nil {
					skipped = append(skipped, "unsafe:"+filepath.Base(path))
					continue
				}
				if safePath == "" {
					continue
				}
				info, err := os.Lstat(safePath)
				if err != nil {
					if os.IsNotExist(err) {
						continue
					}
					return nil, 0, nil, err
				}
				if info.Mode()&os.ModeSymlink != 0 || info.IsDir() {
					skipped = append(skipped, "symlink:"+filepath.Base(safePath))
					continue
				}
				paths = append(paths, safePath)
				bytesFreed += info.Size()
			}
		}
	}

	sort.Strings(paths)
	sort.Strings(skipped)
	return dedupeCompactStrings(paths), bytesFreed, dedupeCompactStrings(skipped), nil
}

func dedupeCompactStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

// compactSafeFile returns path when it exists under root without symlink components.
// Missing paths return ("", nil).
func compactSafeFile(root string, path string) (string, error) {
	if !pathWithinDir(root, path) {
		return "", apperr.New(apperr.CodeInvalidInput, "path_rejected: compact path escapes containment root")
	}
	if err := rejectSymlinkComponents(root, path); err != nil {
		return "", err
	}
	if _, err := os.Lstat(path); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return path, nil
}
