package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
)

// seedTicket writes a ticket file and its created event to the sources only —
// the projection is left alone so each test decides how (or whether) it
// catches up.
func seedTicket(t *testing.T, root string, ticketStore mdstore.TicketStore, eventsLog *eventstore.Log, id string, eventID int64, now time.Time) contracts.Event {
	t.Helper()
	return seedTicketIn(t, root, ticketStore, eventsLog, "APP", id, eventID, now)
}

func seedTicketIn(t *testing.T, root string, ticketStore mdstore.TicketStore, eventsLog *eventstore.Log, project string, id string, eventID int64, now time.Time) contracts.Event {
	t.Helper()
	ctx := context.Background()
	projectStore := mdstore.ProjectStore{RootDir: root}
	if _, err := projectStore.GetProject(ctx, project); err != nil {
		if err := projectStore.CreateProject(ctx, contracts.Project{Key: project, Name: project + " Project", CreatedAt: now}); err != nil {
			t.Fatalf("create project: %v", err)
		}
	}
	ticket := contracts.TicketSnapshot{
		ID:            id,
		Project:       project,
		Title:         "Ticket " + id,
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityMedium,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket %s: %v", id, err)
	}
	event := contracts.Event{
		EventID:       eventID,
		Timestamp:     now,
		Actor:         contracts.Actor("human:owner"),
		Type:          contracts.EventTicketCreated,
		Project:       project,
		TicketID:      id,
		Payload:       ticket,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := eventsLog.AppendEvent(ctx, event); err != nil {
		t.Fatalf("append event %d: %v", eventID, err)
	}
	return event
}

func TestApplyEventStampsSourceFingerprint(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	eventsLog := &eventstore.Log{RootDir: root}
	store, err := Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	store.Root = root

	if _, ok, err := store.StoredSourceFingerprint(ctx); err != nil || ok {
		t.Fatalf("fresh index must carry no fingerprint yet (ok=%v err=%v)", ok, err)
	}
	event := seedTicket(t, root, ticketStore, eventsLog, "APP-1", 1, now)
	if err := store.ApplyEvent(ctx, event); err != nil {
		t.Fatalf("apply event: %v", err)
	}

	want, err := storage.ComputeSourceFingerprint(root)
	if err != nil {
		t.Fatalf("compute fingerprint: %v", err)
	}
	stored, ok, err := store.StoredSourceFingerprint(ctx)
	if err != nil || !ok {
		t.Fatalf("expected a stored fingerprint after ApplyEvent (ok=%v err=%v)", ok, err)
	}
	if stored != want.String() {
		t.Fatalf("stored fingerprint %q does not match sources %q", stored, want)
	}
	stale, _, _, err := store.IsStale(ctx)
	if err != nil || stale {
		t.Fatalf("projection that just applied the event must not be stale (stale=%v err=%v)", stale, err)
	}
}

func TestRebuildCommitsSourceFingerprintWithProjection(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 9, 1, 9, 5, 0, 0, time.UTC)
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	eventsLog := &eventstore.Log{RootDir: root}
	store, err := Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	store.Root = root
	seedTicket(t, root, ticketStore, eventsLog, "APP-1", 1, now)
	seedTicket(t, root, ticketStore, eventsLog, "APP-2", 2, now)

	// The fingerprint must commit with the rebuilt rows.
	if err := store.Rebuild(ctx, ""); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	want, err := storage.ComputeSourceFingerprint(root)
	if err != nil {
		t.Fatalf("compute fingerprint: %v", err)
	}
	stored, ok, err := store.StoredSourceFingerprint(ctx)
	if err != nil || !ok {
		t.Fatalf("expected a stored fingerprint after rebuild (ok=%v err=%v)", ok, err)
	}
	if stored != want.String() {
		t.Fatalf("stored fingerprint %q does not match sources %q", stored, want)
	}
	board, err := store.QueryBoard(ctx, contracts.BoardQueryOptions{})
	if err != nil {
		t.Fatalf("query board: %v", err)
	}
	if len(board.Columns[contracts.StatusReady]) != 2 {
		t.Fatalf("rebuilt board should carry both tickets: %#v", board.Columns)
	}
}

func TestIsStaleWhenLogGrewBehindTheProjection(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 9, 1, 9, 10, 0, 0, time.UTC)
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	eventsLog := &eventstore.Log{RootDir: root}
	store, err := Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	store.Root = root
	first := seedTicket(t, root, ticketStore, eventsLog, "APP-1", 1, now)
	if err := store.ApplyEvent(ctx, first); err != nil {
		t.Fatalf("apply event: %v", err)
	}

	// a second writer appended to the log and the projection never saw it
	seedTicket(t, root, ticketStore, eventsLog, "APP-2", 2, now.Add(time.Minute))

	stale, stored, current, err := store.IsStale(ctx)
	if err != nil {
		t.Fatalf("is stale: %v", err)
	}
	if !stale {
		t.Fatalf("expected stale projection, stored=%q current=%q", stored, current)
	}
	if stored != "events=1 tickets=1" || current != "events=2 tickets=2" {
		t.Fatalf("unexpected fingerprints stored=%q current=%q", stored, current)
	}
}

func TestEmptyWorkspaceWithoutMetaIsNotStale(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	ticketStore := mdstore.TicketStore{RootDir: root}
	eventsLog := &eventstore.Log{RootDir: root}
	store, err := Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	store.Root = root

	// right after `tracker init` nothing has been written and nothing has been
	// stamped; that must read as fresh, not as "rebuild me"
	stale, stored, current, err := store.IsStale(ctx)
	if err != nil {
		t.Fatalf("is stale: %v", err)
	}
	if stale {
		t.Fatalf("empty workspace must not be stale, stored=%q current=%q", stored, current)
	}
	if stored != "events=0 tickets=0" || current != "events=0 tickets=0" {
		t.Fatalf("unexpected fingerprints stored=%q current=%q", stored, current)
	}
}

func TestProjectScopedRebuildDoesNotStamp(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 9, 1, 9, 15, 0, 0, time.UTC)
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	eventsLog := &eventstore.Log{RootDir: root}
	store, err := Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()
	store.Root = root
	if err := store.ApplyEvent(ctx, seedTicket(t, root, ticketStore, eventsLog, "APP-1", 1, now)); err != nil {
		t.Fatalf("apply event: %v", err)
	}
	// OPS grew behind the projection's back; a rebuild scoped to OPS catches
	// that project up but says nothing about the rest of the workspace
	seedTicketIn(t, root, ticketStore, eventsLog, "OPS", "OPS-1", 1, now)

	if err := store.Rebuild(ctx, "OPS"); err != nil {
		t.Fatalf("rebuild OPS: %v", err)
	}
	stored, ok, err := store.StoredSourceFingerprint(ctx)
	if err != nil || !ok {
		t.Fatalf("expected the ApplyEvent stamp to survive (ok=%v err=%v)", ok, err)
	}
	if stored != "events=1 tickets=1" {
		t.Fatalf("a project-scoped rebuild must not claim the whole workspace is caught up, stored=%q", stored)
	}
	stale, _, current, err := store.IsStale(ctx)
	if err != nil || !stale {
		t.Fatalf("workspace must still read as stale after a partial rebuild (stale=%v current=%q err=%v)", stale, current, err)
	}
}
