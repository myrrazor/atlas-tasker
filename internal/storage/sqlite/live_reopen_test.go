package sqlite

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	if n := deletedIndexFDs(path); n != 0 {
		t.Fatalf("replaced index left %d deleted fds", n)
	}
	if _, err := os.Stat(path + "-wal"); err != nil {
		t.Fatalf("reopen removed the replacement wal: %v", err)
	}
	if _, err := os.Stat(path + "-shm"); err != nil {
		t.Fatalf("reopen removed the replacement shm: %v", err)
	}
}

func TestHelperProcessRebuildIndex(t *testing.T) {
	if os.Getenv("ATLAS_REBUILD_HELPER") != "1" {
		t.Skip()
	}
	root := os.Getenv("ATLAS_REBUILD_ROOT")
	ctx := context.Background()
	tickets := mdstore.TicketStore{RootDir: root}
	events := &eventstore.Log{RootDir: root}
	path := filepath.Join(storage.TrackerDir(root), "index.sqlite")
	// Default Open unlinks leftover sidecars. The shared-shm regression keeps
	// them so the parent and the replacement map one inode.
	store, err := open(path, tickets, events, os.Getenv("ATLAS_REBUILD_DISCARD") == "1")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Rebuild(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPinnedConnReopensWhenOnlyIndexFileIsReplaced(t *testing.T) {
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
	// Checkpoint so the only thing left beside the deleted main file is a
	// quiescent shm. Public Open still discards those sidecars; reopen must
	// follow the new files and must not delete them when the old pool closes.
	if _, err := store.DB.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	_, dirty, err := store.LiveSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirty) != 1 || dirty[0].Title != "stale-inode" {
		t.Fatalf("open index title = %#v", dirty)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcessRebuildIndex$", "-test.v")
	cmd.Env = append(os.Environ(), "ATLAS_REBUILD_HELPER=1", "ATLAS_REBUILD_DISCARD=1", "ATLAS_REBUILD_ROOT="+root)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rebuild helper: %v\n%s", err, out)
	}
	shmInfo, err := os.Stat(path + "-shm")
	if err != nil {
		t.Fatalf("replacement shm missing before reopen: %v", err)
	}
	_, got, err := store.LiveSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "fresh-title" {
		t.Fatalf("pinned connection kept the deleted index, got %#v", got)
	}
	if n := deletedIndexFDs(path); n != 0 {
		t.Fatalf("index-only replace left %d deleted index fds", n)
	}
	shmAfter, err := os.Stat(path + "-shm")
	if err != nil {
		t.Fatalf("reopen removed the replacement shm: %v", err)
	}
	if !os.SameFile(shmInfo, shmAfter) {
		t.Fatal("reopen replaced the live -shm")
	}
	shm := path + "-shm"
	info, err := os.Stat(shm)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() < 32768 {
		t.Fatalf("shm shrank before the storm: %d", info.Size())
	}
	if locks := posixLocksOn(os.Getpid(), shm); locks == 0 {
		t.Fatal("reopen dropped the process lock on the shared -shm")
	}
	script := `
import sqlite3, sys
path = sys.argv[1]
for i in range(40):
    con = sqlite3.connect(path, timeout=5)
    con.execute("select count(*) from tickets")
    if i % 5 == 0:
        con.execute("update tickets set title = ? where id = ?", ("stormed-title", "APP-1"))
        con.commit()
    con.close()
`
	storm := exec.Command("python3", "-c", script, path)
	stormOut, err := storm.CombinedOutput()
	if err != nil {
		t.Fatalf("open/close storm: %v %s", err, stormOut)
	}
	after, err := os.Stat(shm)
	if err != nil {
		t.Fatal(err)
	}
	if after.Size() < 32768 {
		t.Fatalf("storm truncated -shm to %d", after.Size())
	}
	if locks := posixLocksOn(os.Getpid(), shm); locks == 0 {
		t.Fatal("storm cleared the process lock on the shared -shm")
	}
	_, afterRows, err := store.LiveSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterRows) != 1 || afterRows[0].Title != "stormed-title" {
		t.Fatalf("snapshot after storm = %#v", afterRows)
	}
}

func TestPinnedConnSurvivesReplacingOnlyTheIndexFileWithoutCheckpoint(t *testing.T) {
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
	if _, _, err := store.LiveSnapshot(ctx); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcessRebuildIndex$", "-test.v")
	cmd.Env = append(os.Environ(), "ATLAS_REBUILD_HELPER=1", "ATLAS_REBUILD_DISCARD=1", "ATLAS_REBUILD_ROOT="+root)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rebuild helper: %v\n%s", err, out)
	}
	shmInfo, err := os.Stat(path + "-shm")
	if err != nil {
		t.Fatalf("replacement shm missing before reopen: %v", err)
	}
	_, got, err := store.LiveSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "fresh-title" {
		t.Fatalf("pinned connection kept the deleted index, got %#v", got)
	}
	if n := deletedIndexFDs(path); n != 0 {
		t.Fatalf("index-only replace left %d deleted index fds", n)
	}
	shmAfter, err := os.Stat(path + "-shm")
	if err != nil {
		t.Fatalf("reopen removed the replacement shm: %v", err)
	}
	if !os.SameFile(shmInfo, shmAfter) {
		t.Fatal("reopen replaced the live -shm")
	}
	shm := path + "-shm"
	if locks := posixLocksOn(os.Getpid(), shm); locks == 0 {
		t.Fatal("reopen dropped the process lock on -shm")
	}
	script := `
import sqlite3, sys
path = sys.argv[1]
for i in range(40):
    con = sqlite3.connect(path, timeout=5)
    con.execute("select count(*) from tickets")
    if i % 5 == 0:
        con.execute("update tickets set title = ? where id = ?", ("stormed-title", "APP-1"))
        con.commit()
    con.close()
`
	storm := exec.Command("python3", "-c", script, path)
	stormOut, err := storm.CombinedOutput()
	if err != nil {
		t.Fatalf("open/close storm: %v %s", err, stormOut)
	}
	if locks := posixLocksOn(os.Getpid(), shm); locks == 0 {
		t.Fatal("storm cleared the process lock on -shm")
	}
	info, err := os.Stat(shm)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() < 32768 {
		t.Fatalf("storm truncated -shm to %d", info.Size())
	}
	_, afterRows, err := store.LiveSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterRows) != 1 || afterRows[0].Title != "stormed-title" {
		t.Fatalf("snapshot after storm = %#v", afterRows)
	}
}

func TestRepeatedIndexReplaceDoesNotLeakFDs(t *testing.T) {
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
	if _, _, err := store.LiveSnapshot(ctx); err != nil {
		t.Fatal(err)
	}
	before := countFDs()
	if before < 0 {
		t.Skip("fd count is only available via /proc")
	}
	for i := 0; i < 10; i++ {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcessRebuildIndex$", "-test.v")
		cmd.Env = append(os.Environ(), "ATLAS_REBUILD_HELPER=1", "ATLAS_REBUILD_DISCARD=1", "ATLAS_REBUILD_ROOT="+root)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("rebuild %d: %v\n%s", i, err, out)
		}
		if _, _, err := store.LiveSnapshot(ctx); err != nil {
			t.Fatalf("reopen %d: %v", i, err)
		}
		if n := deletedIndexFDs(path); n != 0 {
			t.Fatalf("replacement %d left %d deleted index fds", i, n)
		}
	}
	after := countFDs()
	if after-before > 8 {
		t.Fatalf("fd count grew from %d to %d across 10 index replacements", before, after)
	}
	t.Logf("fds before=%d after=%d", before, after)
}

func TestReopenDropsStaleSidecarsWhenMainFileIsSwapped(t *testing.T) {
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
		ID: "APP-1", Project: "APP", Title: "old-title", Type: contracts.TicketTypeTask,
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
	if _, _, err := store.LiveSnapshot(ctx); err != nil {
		t.Fatal(err)
	}
	oldShm, err := os.Stat(path + "-shm")
	if err != nil {
		t.Fatal(err)
	}
	otherPath := filepath.Join(t.TempDir(), "index.sqlite")
	other, err := Open(otherPath, tickets, events)
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Rebuild(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := other.DB.ExecContext(ctx, `UPDATE tickets SET title = ? WHERE id = ?`, "swapped-title", "APP-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := other.DB.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	if err := other.Close(); err != nil {
		t.Fatal(err)
	}
	// Rename replaces the inode and leaves the old wal and shm in place.
	if err := os.Rename(otherPath, path); err != nil {
		t.Fatal(err)
	}
	_, got, err := store.LiveSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "swapped-title" {
		t.Fatalf("swapped index title = %#v", got)
	}
	nowShm, err := os.Stat(path + "-shm")
	if err != nil {
		t.Fatalf("shm missing after swap: %v", err)
	}
	if os.SameFile(oldShm, nowShm) {
		t.Fatal("reopen kept the previous -shm after the main file was swapped")
	}
	if _, err := os.Stat(path + "-wal"); err != nil {
		t.Fatalf("wal missing after swap: %v", err)
	}
}

func deletedIndexFDs(path string) int {
	abs, err := filepath.Abs(path)
	if err != nil {
		return -1
	}
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return -1
	}
	n := 0
	for _, entry := range entries {
		target, err := os.Readlink("/proc/self/fd/" + entry.Name())
		if err != nil {
			continue
		}
		if strings.HasPrefix(target, abs) && strings.Contains(target, "(deleted)") {
			n++
		}
	}
	return n
}
