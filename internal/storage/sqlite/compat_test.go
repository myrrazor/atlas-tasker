package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	"github.com/myrrazor/atlas-tasker/internal/testutil"
)

func TestRebuildFromV1FixtureWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := testutil.CopyDir(testutil.FixturePath("app_sample"), root); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	ctx := context.Background()
	ticketStore := mdstore.TicketStore{RootDir: root}
	eventsLog := &eventstore.Log{RootDir: root}
	store, err := Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer store.Close()

	if err := store.Rebuild(ctx, ""); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	ticket, err := store.QueryTicket(ctx, "APP-1")
	if err != nil {
		t.Fatalf("query ticket: %v", err)
	}
	if ticket.SchemaVersion != contracts.CurrentSchemaVersion {
		t.Fatalf("expected normalized schema version, got %d", ticket.SchemaVersion)
	}
	if ticket.ReviewState != contracts.ReviewStateNone {
		t.Fatalf("unexpected review state: %s", ticket.ReviewState)
	}
	history, err := store.QueryHistory(ctx, "APP-1")
	if err != nil {
		t.Fatalf("query history: %v", err)
	}
	if len(history) != 1 || history[0].SchemaVersion != contracts.SchemaVersionV1 {
		t.Fatalf("unexpected history after rebuild: %#v", history)
	}
}
