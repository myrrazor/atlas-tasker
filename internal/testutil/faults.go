package testutil

import (
	"os"
	"path/filepath"

	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func CorruptProjection(root string) error {
	path := filepath.Join(storage.TrackerDir(root), "index.sqlite")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte("definitely not sqlite"), 0o644)
}

func SeedPendingMutationJournal(root string, journal service.MutationJournal, entry service.MutationJournalEntry) error {
	started, err := journal.Begin(entry.Purpose, entry.CanonicalKind, entry.Event)
	if err != nil {
		return err
	}
	_, err = journal.Mark(started, entry.Stage, entry.LastError)
	return err
}
