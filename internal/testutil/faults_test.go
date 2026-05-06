package testutil

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestCorruptProjection(t *testing.T) {
	root := t.TempDir()
	if err := CorruptProjection(root); err != nil {
		t.Fatalf("corrupt projection: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(storage.TrackerDir(root), "index.sqlite"))
	if err != nil {
		t.Fatalf("read projection: %v", err)
	}
	if string(raw) != "definitely not sqlite" {
		t.Fatalf("unexpected projection contents: %q", string(raw))
	}
}

func TestSeedPendingMutationJournal(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC)
	journal := service.MutationJournal{Root: root, Clock: func() time.Time { return now }}
	entry := service.MutationJournalEntry{
		Purpose:       "repair me",
		CanonicalKind: "ticket_snapshot",
		Event: contracts.Event{
			EventID:       1,
			Timestamp:     now,
			Actor:         contracts.Actor("human:owner"),
			Type:          contracts.EventTicketCreated,
			Project:       "APP",
			TicketID:      "APP-1",
			Payload:       map[string]any{"id": "APP-1"},
			SchemaVersion: contracts.CurrentSchemaVersion,
		},
		Stage: service.MutationStageCanonicalWritten,
	}
	if err := SeedPendingMutationJournal(root, journal, entry); err != nil {
		t.Fatalf("seed pending journal: %v", err)
	}
	entries, err := journal.List()
	if err != nil {
		t.Fatalf("list journals: %v", err)
	}
	if len(entries) != 1 || entries[0].Stage != service.MutationStageCanonicalWritten {
		t.Fatalf("unexpected journal entries: %#v", entries)
	}
}
