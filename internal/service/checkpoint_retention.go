package service

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
)

const (
	localSnapshotRetain     = 3
	localSnapshotMaxAge     = 24 * time.Hour
	localTmpMaxAge          = time.Hour
	localArchiveRetainCount = 8
)

func retainLocalCheckpointState(paths checkpointPaths, now time.Time) error {
	_ = pruneDirByAge(paths.Tmp, now, localTmpMaxAge, 0)
	_ = pruneDirByAge(paths.Snapshots, now, localSnapshotMaxAge, localSnapshotRetain)
	return nil
}

func pruneDirByAge(dir string, now time.Time, maxAge time.Duration, keepNewest int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	type item struct {
		name    string
		modTime time.Time
	}
	items := make([]item, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		items = append(items, item{name: entry.Name(), modTime: info.ModTime()})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].modTime.After(items[j].modTime) })
	for i, entry := range items {
		if i < keepNewest {
			continue
		}
		if now.Sub(entry.modTime) < maxAge && keepNewest > 0 {
			continue
		}
		if keepNewest == 0 && now.Sub(entry.modTime) < maxAge {
			continue
		}
		_ = os.RemoveAll(filepath.Join(dir, entry.name))
	}
	return nil
}

type BackupPrunePlan struct {
	Kind        string    `json:"kind"`
	GeneratedAt time.Time `json:"generated_at"`
	LocalPaths  int       `json:"local_generated_paths"`
	RemotePrune string    `json:"remote_prune"`
	Notes       []string  `json:"notes,omitempty"`
}

func (s *ActionService) BackupPrunePlan(ctx context.Context) (BackupPrunePlan, error) {
	_ = ctx
	paths, _, err := s.backupPaths()
	if err != nil {
		return BackupPrunePlan{}, err
	}
	count := 0
	for _, dir := range []string{paths.Tmp, paths.Snapshots} {
		entries, err := os.ReadDir(dir)
		if err == nil {
			count += len(entries)
		}
	}
	return BackupPrunePlan{
		Kind:        "backup_prune_plan",
		GeneratedAt: s.now(),
		LocalPaths:  count,
		RemotePrune: "append_only",
		Notes:       []string{"remote Git history remains append-only in v1.14", "automatic prune never deletes remote refs"},
	}, nil
}

func (s *ActionService) BackupPruneApply(ctx context.Context, yes bool, remote bool) (BackupPrunePlan, error) {
	if remote {
		return BackupPrunePlan{}, apperr.New(apperr.CodeConflict, "remote prune is plan-only in v1.14; history stays append-only")
	}
	if !yes {
		return BackupPrunePlan{}, apperr.New(apperr.CodeInvalidInput, "prune apply requires --yes")
	}
	paths, _, err := s.backupPaths()
	if err != nil {
		return BackupPrunePlan{}, err
	}
	if err := retainLocalCheckpointState(paths, s.now()); err != nil {
		return BackupPrunePlan{}, err
	}
	return s.BackupPrunePlan(ctx)
}
