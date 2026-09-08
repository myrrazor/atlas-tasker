package service

import (
	"context"
	"fmt"
	"io"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

// RebuildableProjection is the slice of the sqlite store the freshness check
// needs. Kept as an interface so service doesn't have to import the sqlite
// package for one call site.
type RebuildableProjection interface {
	SetRoot(root string)
	IsStale(ctx context.Context) (stale bool, stored string, current string, err error)
	Rebuild(ctx context.Context, project string) error
}

// EnsureFreshProjection rebuilds index.sqlite when its stamped fingerprint no
// longer matches the markdown + event log on disk. It runs on every workspace
// open, so the happy path counts source files/event lines and reads one metadata row.
//
// The workspace write lock keeps source files stable during the rebuild.
// Recheck after acquiring it because another process may have rebuilt while
// this one waited. SQLite commits the rebuild in place, so existing readers
// see the new projection without having to replace their connection pools.
func EnsureFreshProjection(ctx context.Context, root string, locks WriteLockManager, projection RebuildableProjection, notice io.Writer) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	projection.SetRoot(root)
	stale, _, _, err := projection.IsStale(ctx)
	if err != nil {
		return false, err
	}
	if !stale {
		return false, nil
	}
	rebuilt := false
	current := ""
	if err := WithWriteLock(ctx, locks, "rebuild stale index", func(ctx context.Context) error {
		var stillStale bool
		var err error
		stillStale, _, current, err = projection.IsStale(ctx)
		if err != nil {
			return err
		}
		if !stillStale {
			return nil
		}
		if err := projection.Rebuild(ctx, ""); err != nil {
			return apperr.Wrap(apperr.CodeRepairNeeded, err, "index.sqlite is stale and could not be rebuilt: %v; run 'tracker doctor'", err)
		}
		rebuilt = true
		return nil
	}); err != nil {
		return false, err
	}
	if !rebuilt {
		return false, nil
	}
	// `tracker init` then `tracker board` lands here with nothing to index;
	// that path stays silent
	if notice != nil && current != (storage.SourceFingerprint{}).String() {
		fmt.Fprintf(notice, "[tracker] index.sqlite was missing or stale; rebuilt it from markdown and events (%s)\n", current)
	}
	return true, nil
}
