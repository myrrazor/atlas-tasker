package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
)

type failingEventSource struct{ contracts.EventLog }

func (failingEventSource) StreamEvents(context.Context, string, int64) ([]contracts.Event, error) {
	return nil, errors.New("source unavailable")
}

func TestRebuildRemainsVisibleToOpenReadersAndWriters(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	tickets := mdstore.TicketStore{RootDir: root}
	events := &eventstore.Log{RootDir: root}
	path := filepath.Join(storage.TrackerDir(root), "index.sqlite")
	first, err := Open(path, tickets, events)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := Open(path, tickets, events)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	seedTicket(t, root, tickets, events, "APP-1", 1, now)
	if err := first.Rebuild(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := second.QueryTicket(ctx, "APP-1"); err != nil {
		t.Fatalf("open reader missed committed rebuild: %v", err)
	}
	event := seedTicket(t, root, tickets, events, "APP-2", 2, now)
	if err := second.ApplyEvent(ctx, event); err != nil {
		t.Fatalf("open writer cannot apply after rebuild: %v", err)
	}
	if _, err := first.QueryTicket(ctx, "APP-2"); err != nil {
		t.Fatalf("rebuilding handle missed another writer: %v", err)
	}
}

func TestFailedRebuildPreservesExistingProjection(t *testing.T) {
	for _, project := range []string{"", "APP"} {
		t.Run("project="+project, func(t *testing.T) {
			root := t.TempDir()
			ctx := context.Background()
			tickets := mdstore.TicketStore{RootDir: root}
			events := &eventstore.Log{RootDir: root}
			store, err := Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), tickets, events)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			event := seedTicket(t, root, tickets, events, "APP-1", 1, time.Now().UTC())
			if err := store.ApplyEvent(ctx, event); err != nil {
				t.Fatal(err)
			}
			store.EventSource = failingEventSource{events}
			if err := store.Rebuild(ctx, project); err == nil {
				t.Fatal("expected source failure")
			}
			if _, err := store.QueryTicket(ctx, "APP-1"); err != nil {
				t.Fatalf("failed rebuild destroyed previous projection: %v", err)
			}
		})
	}
}

func TestEveryPooledConnectionWaitsForSQLiteWriters(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "index ?# space.sqlite"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	// Holding one connection forces the pool to open another, where an
	// initialization-time PRAGMA executed only through DB.Exec would be lost.
	var held []*sql.Conn
	defer func() {
		for _, conn := range held {
			_ = conn.Close()
		}
	}()
	for i := 0; i < 2; i++ {
		conn, err := store.DB.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, conn)
		var timeout int
		if err := conn.QueryRowContext(context.Background(), "PRAGMA busy_timeout").Scan(&timeout); err != nil {
			t.Fatal(err)
		}
		if timeout == 0 {
			t.Fatalf("pooled connection %d fails immediately on writer contention", i)
		}
	}
}

func TestLaterApplyDoesNotHideSkippedSourceEvent(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Now().UTC()
	tickets := mdstore.TicketStore{RootDir: root}
	events := &eventstore.Log{RootDir: root}
	store, err := Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), tickets, events)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.SetRoot(root)
	if err := store.ApplyEvent(ctx, seedTicket(t, root, tickets, events, "APP-1", 1, now)); err != nil {
		t.Fatal(err)
	}
	seedTicket(t, root, tickets, events, "APP-2", 2, now)
	if err := store.ApplyEvent(ctx, seedTicket(t, root, tickets, events, "APP-3", 3, now)); err != nil {
		t.Fatal(err)
	}
	stale, _, _, err := store.IsStale(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !stale {
		t.Fatal("later event hid an earlier event missing from the projection")
	}
	if err := store.Rebuild(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if stale, _, _, err := store.IsStale(ctx); err != nil || stale {
		t.Fatalf("rebuild did not restore freshness: %v %v", stale, err)
	}
}

func TestApplyEventFailureRollsBackEventAndSnapshot(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	tickets := mdstore.TicketStore{RootDir: root}
	events := &eventstore.Log{RootDir: root}
	store, err := Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), tickets, events)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.SetRoot(root)
	event := seedTicket(t, root, tickets, events, "APP-1", 1, time.Now().UTC())
	if _, err := store.DB.Exec("CREATE TRIGGER fail_ticket BEFORE INSERT ON tickets BEGIN SELECT RAISE(FAIL, 'injected write failure'); END"); err != nil {
		t.Fatal(err)
	}
	if err := store.ApplyEvent(ctx, event); err == nil {
		t.Fatal("expected injected snapshot failure")
	}
	history, err := store.QueryHistory(ctx, "APP-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 0 {
		t.Fatal("failed transaction left an event without its snapshot")
	}
}

func TestConcurrentFirstOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.sqlite")
	start := make(chan struct{})
	errors := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			store, err := Open(path, nil, nil)
			if err == nil {
				err = store.Close()
			}
			errors <- err
		}()
	}
	close(start)
	for i := 0; i < 2; i++ {
		if err := <-errors; err != nil {
			t.Errorf("concurrent first open: %v", err)
		}
	}
}
