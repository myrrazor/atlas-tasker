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

func TestApplyEventQueryHistoryBoardAndSearch(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()

	projectStore := mdstore.ProjectStore{RootDir: root}
	if err := projectStore.CreateProject(ctx, contracts.Project{
		Key:       "APP",
		Name:      "App Project",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	now := time.Date(2026, 3, 22, 16, 0, 0, 0, time.UTC)
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	eventsLog := &eventstore.Log{RootDir: root}
	sqlitePath := filepath.Join(storage.TrackerDir(root), "index.sqlite")
	store, err := Open(sqlitePath, ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()

	ticket := contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Build parser",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		Labels:        []string{"cli"},
		Assignee:      contracts.Actor("agent:builder-1"),
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
		Summary:       "Parser summary",
		Description:   "CLI parser implementation",
	}
	if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket markdown: %v", err)
	}

	event := contracts.Event{
		EventID:       1,
		Timestamp:     now,
		Actor:         contracts.Actor("human:owner"),
		Type:          contracts.EventTicketCreated,
		Project:       "APP",
		TicketID:      "APP-1",
		Payload:       ticket,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := eventsLog.AppendEvent(ctx, event); err != nil {
		t.Fatalf("append source event: %v", err)
	}
	if err := store.ApplyEvent(ctx, event); err != nil {
		t.Fatalf("apply projection event: %v", err)
	}

	history, err := store.QueryHistory(ctx, "APP-1")
	if err != nil {
		t.Fatalf("query history: %v", err)
	}
	if len(history) != 1 || history[0].EventID != 1 {
		t.Fatalf("unexpected history: %#v", history)
	}

	board, err := store.QueryBoard(ctx, contracts.BoardQueryOptions{Project: "APP"})
	if err != nil {
		t.Fatalf("query board: %v", err)
	}
	if len(board.Columns[contracts.StatusReady]) != 1 {
		t.Fatalf("unexpected board projection: %#v", board.Columns)
	}

	search, err := store.QuerySearch(ctx, contracts.SearchQuery{Terms: []contracts.SearchTerm{{Kind: contracts.SearchTermTextLike, Value: "parser"}}})
	if err != nil {
		t.Fatalf("query search: %v", err)
	}
	if len(search) != 1 || search[0].ID != "APP-1" {
		t.Fatalf("unexpected search results: %#v", search)
	}
}

func TestRebuildFromMarkdownAndEvents(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()

	projectStore := mdstore.ProjectStore{RootDir: root}
	if err := projectStore.CreateProject(ctx, contracts.Project{
		Key:       "APP",
		Name:      "App Project",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	now := time.Date(2026, 3, 22, 17, 0, 0, 0, time.UTC)
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	eventsLog := &eventstore.Log{RootDir: root}

	ticket := contracts.TicketSnapshot{
		ID:            "APP-2",
		Project:       "APP",
		Title:         "Rebuild me",
		Type:          contracts.TicketTypeBug,
		Status:        contracts.StatusBlocked,
		Priority:      contracts.PriorityCritical,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
		Description:   "Reindex projection test",
	}
	if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket markdown: %v", err)
	}

	event := contracts.Event{
		EventID:       1,
		Timestamp:     now,
		Actor:         contracts.Actor("agent:builder-1"),
		Type:          contracts.EventTicketCreated,
		Project:       "APP",
		TicketID:      "APP-2",
		Payload:       ticket,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := eventsLog.AppendEvent(ctx, event); err != nil {
		t.Fatalf("append event: %v", err)
	}

	sqlitePath := filepath.Join(storage.TrackerDir(root), "index.sqlite")
	store, err := Open(sqlitePath, ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()

	if err := store.Rebuild(ctx, "APP"); err != nil {
		t.Fatalf("rebuild failed: %v", err)
	}

	board, err := store.QueryBoard(ctx, contracts.BoardQueryOptions{Project: "APP"})
	if err != nil {
		t.Fatalf("query board: %v", err)
	}
	if len(board.Columns[contracts.StatusBlocked]) != 1 {
		t.Fatalf("rebuild board mismatch: %#v", board.Columns)
	}

	history, err := store.QueryHistory(ctx, "APP-2")
	if err != nil {
		t.Fatalf("query history: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("rebuild history mismatch: %#v", history)
	}
}

func TestRebuildRequiresSources(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	sqlitePath := filepath.Join(storage.TrackerDir(root), "index.sqlite")
	store, err := Open(sqlitePath, nil, nil)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()

	if err := store.Rebuild(ctx, ""); err == nil {
		t.Fatal("expected rebuild to fail without sources")
	}
}
