package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestCompactablePathsSkipsEscapingArchivePayload(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	outside := t.TempDir()
	bait := filepath.Join(outside, storage.TrackerDirName, "runtime", "run_escape", "launch.codex.txt")
	if err := os.MkdirAll(filepath.Dir(bait), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bait, []byte("do-not-delete"), 0o600); err != nil {
		t.Fatal(err)
	}
	record := contracts.ArchiveRecord{
		ArchiveID:     "archive_escape",
		Target:        contracts.RetentionTargetRuntime,
		Scope:         "workspace",
		ProjectKey:    "APP",
		SourcePaths:   []string{filepath.Join(storage.TrackerDirName, "runtime", "run_escape")},
		PayloadDir:    outside,
		ItemCount:     1,
		State:         contracts.ArchiveRecordArchived,
		CreatedAt:     time.Unix(1700000000, 0).UTC(),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := (ArchiveRecordStore{Root: root}).SaveArchiveRecord(ctx, record); err != nil {
		t.Fatal(err)
	}
	paths, _, skipped, err := compactablePaths(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if filepath.Clean(path) == filepath.Clean(bait) {
			t.Fatalf("compact listed escaped payload path %s; skipped=%v", path, skipped)
		}
	}
	if _, err := os.Stat(bait); err != nil {
		t.Fatalf("escaped payload should remain: %v", err)
	}
}
