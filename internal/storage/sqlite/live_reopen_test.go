package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
)

func TestPinnedConnReopensWhenIndexFileIsReplaced(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	if err := (mdstore.ProjectStore{RootDir: root}).CreateProject(ctx, contracts.Project{
		Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatal(err)
	}
	tickets := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	events := &eventstore.Log{RootDir: root}
	path := filepath.Join(storage.TrackerDir(root), "index.sqlite")
	store, err := Open(path, tickets, events)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ticket := contracts.TicketSnapshot{
		ID: "APP-1", Project: "APP", Title: "fresh-title", Type: contracts.TicketTypeTask,
		Status: contracts.StatusReady, Priority: contracts.PriorityMedium,
		CreatedAt: now, UpdatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := tickets.CreateTicket(ctx, ticket); err != nil {
		t.Fatal(err)
	}
	if err := events.AppendEvent(ctx, contracts.Event{
		EventID: 1, Timestamp: now, Actor: "human:owner", Type: contracts.EventTicketCreated,
		Project: "APP", TicketID: "APP-1", Payload: ticket, SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Rebuild(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.ExecContext(ctx, `UPDATE tickets SET title = ? WHERE id = ?`, "stale-inode", "APP-1"); err != nil {
		t.Fatal(err)
	}
	_, dirty, err := store.LiveSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirty) != 1 || dirty[0].Title != "stale-inode" {
		t.Fatalf("open index title = %#v", dirty)
	}
	for _, name := range []string{"index.sqlite", "index.sqlite-wal", "index.sqlite-shm"} {
		if err := os.Remove(filepath.Join(storage.TrackerDir(root), name)); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	fresh, err := Open(path, tickets, events)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fresh.Close() })
	if err := fresh.Rebuild(ctx, ""); err != nil {
		t.Fatal(err)
	}
	_, got, err := store.LiveSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "fresh-title" {
		t.Fatalf("pinned connection kept the deleted index, got %#v", got)
	}
}
