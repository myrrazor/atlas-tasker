package service

import (
	"context"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

func TestLiveQueryServiceSeesAnotherHandlesAutomaticRebuild(t *testing.T) {
	root, tickets, events, first := freshnessFixture(t)
	ctx := context.Background()
	first.SetRoot(root)
	if err := first.ApplyEvent(ctx, seedSourcesOnly(t, tickets, events, "APP-1", 1)); err != nil {
		t.Fatal(err)
	}
	second, err := sqlitestore.Open(first.Path, tickets, events)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	locks := FileLockManager{Root: root}
	if _, err := EnsureFreshProjection(ctx, root, locks, second, nil); err != nil {
		t.Fatal(err)
	}
	queries := NewQueryService(root, mdstore.ProjectStore{RootDir: root}, tickets, events, second, nil)
	before, err := queries.Board(ctx, contracts.BoardQueryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Board.Columns[contracts.StatusReady]) != 1 {
		t.Fatalf("unexpected initial board: %#v", before)
	}

	// Model a committed event missed by the projection while this reader stays open.
	seedSourcesOnly(t, tickets, events, "APP-2", 2)
	if rebuilt, err := EnsureFreshProjection(ctx, root, locks, first, nil); err != nil || !rebuilt {
		t.Fatalf("first handle did not rebuild: rebuilt=%v err=%v", rebuilt, err)
	}
	// The consumer uses its existing query service, with no explicit reopen or freshness call.
	after, err := queries.Board(ctx, contracts.BoardQueryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Board.Columns[contracts.StatusReady]) != 2 {
		t.Fatalf("live query service retained the old projection: %#v", after.Board.Columns)
	}
	history, err := queries.History(ctx, "APP-2")
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Events) != 1 || history.Events[0].Type != contracts.EventTicketCreated {
		t.Fatalf("live query history missed the rebuilt event: %#v", history.Events)
	}
}
