package service

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

// lockSpy wraps the real file lock so a test can tell whether the rebuild ran
// while the lock was held.
type lockSpy struct {
	inner    FileLockManager
	purposes []string
	held     bool
}

func (l *lockSpy) Acquire(ctx context.Context, purpose string) (func() error, error) {
	unlock, err := l.inner.Acquire(ctx, purpose)
	if err != nil {
		return nil, err
	}
	l.purposes = append(l.purposes, purpose)
	l.held = true
	return func() error {
		l.held = false
		return unlock()
	}, nil
}

type rebuildSpy struct {
	*sqlitestore.Store
	lock      *lockSpy
	calls     int
	underLock int
}

func (s *rebuildSpy) Rebuild(ctx context.Context, project string) error {
	s.calls++
	if s.lock != nil && s.lock.held {
		s.underLock++
	}
	return s.Store.Rebuild(ctx, project)
}

func freshnessFixture(t *testing.T) (string, mdstore.TicketStore, *eventstore.Log, *sqlitestore.Store) {
	t.Helper()
	root := t.TempDir()
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	eventLog := &eventstore.Log{RootDir: root}
	if err := (mdstore.ProjectStore{RootDir: root}).CreateProject(context.Background(), contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = projection.Close() })
	return root, ticketStore, eventLog, projection
}

func seedSourcesOnly(t *testing.T, ticketStore mdstore.TicketStore, eventLog *eventstore.Log, id string, eventID int64) contracts.Event {
	t.Helper()
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	ticket := contracts.TicketSnapshot{
		ID: id, Project: "APP", Title: "Ticket " + id, Type: contracts.TicketTypeTask,
		Status: contracts.StatusReady, Priority: contracts.PriorityMedium,
		CreatedAt: now, UpdatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := ticketStore.CreateTicket(context.Background(), ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	event := contracts.Event{
		EventID: eventID, Timestamp: now, Actor: contracts.Actor("human:owner"),
		Type: contracts.EventTicketCreated, Project: "APP", TicketID: id, Payload: ticket,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := eventLog.AppendEvent(context.Background(), event); err != nil {
		t.Fatalf("append event: %v", err)
	}
	return event
}

func TestEnsureFreshProjectionRebuildsUnderTheLockAndNotifies(t *testing.T) {
	root, ticketStore, eventLog, projection := freshnessFixture(t)
	ctx := context.Background()
	seedSourcesOnly(t, ticketStore, eventLog, "APP-1", 1)
	seedSourcesOnly(t, ticketStore, eventLog, "APP-2", 2)

	lock := &lockSpy{inner: FileLockManager{Root: root}}
	spy := &rebuildSpy{Store: projection, lock: lock}
	var notice bytes.Buffer
	rebuilt, err := EnsureFreshProjection(ctx, root, lock, spy, &notice)
	if err != nil {
		t.Fatalf("ensure fresh projection: %v", err)
	}
	if !rebuilt {
		t.Fatal("expected a stale projection to be rebuilt")
	}
	if spy.calls != 1 || spy.underLock != 1 {
		t.Fatalf("expected exactly one rebuild under the write lock, got calls=%d underLock=%d purposes=%v", spy.calls, spy.underLock, lock.purposes)
	}
	want := "[tracker] index.sqlite was missing or stale; rebuilt it from markdown and events (events=2 tickets=2)\n"
	if notice.String() != want {
		t.Fatalf("unexpected notice %q, want %q", notice.String(), want)
	}
	board, err := projection.QueryBoard(ctx, contracts.BoardQueryOptions{})
	if err != nil {
		t.Fatalf("query board: %v", err)
	}
	if len(board.Columns[contracts.StatusReady]) != 2 {
		t.Fatalf("expected both tickets on the rebuilt board, got %#v", board.Columns)
	}
}

func TestEnsureFreshProjectionIsQuietOnAnEmptyWorkspace(t *testing.T) {
	root, ticketStore, eventLog, projection := freshnessFixture(t)
	ctx := context.Background()
	// stamp the index against real sources, then take the sources away: the
	// projection is now stale against an empty workspace
	projection.Root = root
	if err := projection.ApplyEvent(ctx, seedSourcesOnly(t, ticketStore, eventLog, "APP-1", 1)); err != nil {
		t.Fatalf("apply event: %v", err)
	}
	if err := os.RemoveAll(storage.EventsDir(root)); err != nil {
		t.Fatalf("drop events: %v", err)
	}
	if err := os.Remove(storage.TicketFile(root, "APP", "APP-1")); err != nil {
		t.Fatalf("drop ticket: %v", err)
	}

	spy := &rebuildSpy{Store: projection}
	var notice bytes.Buffer
	rebuilt, err := EnsureFreshProjection(ctx, root, FileLockManager{Root: root}, spy, &notice)
	if err != nil {
		t.Fatalf("ensure fresh projection: %v", err)
	}
	if !rebuilt || spy.calls != 1 {
		t.Fatalf("expected one silent rebuild, got rebuilt=%v calls=%d", rebuilt, spy.calls)
	}
	if notice.Len() != 0 {
		t.Fatalf("an empty workspace must rebuild without a notice, got %q", notice.String())
	}
}

func TestEnsureFreshProjectionSkipsWhenFresh(t *testing.T) {
	root, ticketStore, eventLog, projection := freshnessFixture(t)
	ctx := context.Background()
	projection.Root = root
	if err := projection.ApplyEvent(ctx, seedSourcesOnly(t, ticketStore, eventLog, "APP-1", 1)); err != nil {
		t.Fatalf("apply event: %v", err)
	}

	spy := &rebuildSpy{Store: projection}
	var notice bytes.Buffer
	rebuilt, err := EnsureFreshProjection(ctx, root, FileLockManager{Root: root}, spy, &notice)
	if err != nil {
		t.Fatalf("ensure fresh projection: %v", err)
	}
	if rebuilt || spy.calls != 0 {
		t.Fatalf("fresh projection must not rebuild, got rebuilt=%v calls=%d", rebuilt, spy.calls)
	}
	if notice.Len() != 0 {
		t.Fatalf("expected no notice on a fresh projection, got %q", notice.String())
	}
}

func TestEnsureFreshProjectionAdoptsARebuildDoneByAnotherProcess(t *testing.T) {
	root, ticketStore, eventLog, first := freshnessFixture(t)
	ctx := context.Background()
	seedSourcesOnly(t, ticketStore, eventLog, "APP-1", 1)
	seedSourcesOnly(t, ticketStore, eventLog, "APP-2", 2)

	// a second handle on the same file, opened before anyone rebuilt — this
	// is the process that loses the lock race
	second, err := sqlitestore.Open(first.Path, ticketStore, eventLog)
	if err != nil {
		t.Fatalf("open second handle: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	locks := FileLockManager{Root: root}
	if rebuilt, err := EnsureFreshProjection(ctx, root, locks, &rebuildSpy{Store: first}, nil); err != nil || !rebuilt {
		t.Fatalf("first handle should have rebuilt (rebuilt=%v err=%v)", rebuilt, err)
	}

	// The second handle sees the committed rebuild without another rebuild.
	spy := &rebuildSpy{Store: second}
	var notice bytes.Buffer
	rebuilt, err := EnsureFreshProjection(ctx, root, locks, spy, &notice)
	if err != nil {
		t.Fatalf("ensure fresh projection on second handle: %v", err)
	}
	if rebuilt || spy.calls != 0 {
		t.Fatalf("second handle should adopt the rebuild, got rebuilt=%v calls=%d", rebuilt, spy.calls)
	}
	if notice.Len() != 0 {
		t.Fatalf("no second notice expected, got %q", notice.String())
	}
	board, err := second.QueryBoard(ctx, contracts.BoardQueryOptions{})
	if err != nil {
		t.Fatalf("query board through second handle: %v", err)
	}
	if len(board.Columns[contracts.StatusReady]) != 2 {
		t.Fatalf("second handle missed the committed rebuild: %#v", board.Columns)
	}
}
