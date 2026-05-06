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
			info, err := os.Stat(path)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, 0, nil, err
			}
			if info.IsDir() {
				continue
			}
			paths = append(paths, path)
			bytesFreed += info.Size()
		}
	}

	records, err := queries.Archives.ListArchiveRecords(ctx)
	if err != nil {
		return nil, 0, nil, err
	}
	for _, record := range records {
		if record.Target != contracts.RetentionTargetRuntime {
			continue
		}
		for _, rel := range record.SourcePaths {
			runID := filepath.Base(rel)
			for _, path := range []string{
				filepath.Join(record.PayloadDir, storage.TrackerDirName, "runtime", runID, "launch.codex.txt"),
				filepath.Join(record.PayloadDir, storage.TrackerDirName, "runtime", runID, "launch.claude.txt"),
			} {
				info, err := os.Stat(path)
				if err != nil {
					if os.IsNotExist(err) {
						continue
					}
					return nil, 0, nil, err
				}
				if info.IsDir() {
					continue
				}
				paths = append(paths, path)
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
