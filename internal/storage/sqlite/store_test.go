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

func TestQueryBoardDerivesBlockedAndDoneColumns(t *testing.T) {
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

	now := time.Date(2026, 3, 22, 16, 30, 0, 0, time.UTC)
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	eventsLog := &eventstore.Log{RootDir: root}
	sqlitePath := filepath.Join(storage.TrackerDir(root), "index.sqlite")
	store, err := Open(sqlitePath, ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()

	tickets := []contracts.TicketSnapshot{
		{
			ID:            "APP-1",
			Project:       "APP",
			Title:         "Blocked by link",
			Type:          contracts.TicketTypeBug,
			Status:        contracts.StatusBacklog,
			Priority:      contracts.PriorityHigh,
			BlockedBy:     []string{"APP-9"},
			CreatedAt:     now,
			UpdatedAt:     now,
			SchemaVersion: contracts.CurrentSchemaVersion,
		},
		{
			ID:            "APP-2",
			Project:       "APP",
			Title:         "Canceled work",
			Type:          contracts.TicketTypeTask,
			Status:        contracts.StatusCanceled,
			Priority:      contracts.PriorityLow,
			CreatedAt:     now,
			UpdatedAt:     now.Add(time.Minute),
			SchemaVersion: contracts.CurrentSchemaVersion,
		},
	}

	for index, ticket := range tickets {
		if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
			t.Fatalf("create ticket markdown: %v", err)
		}
		event := contracts.Event{
			EventID:       int64(index + 1),
			Timestamp:     ticket.UpdatedAt,
			Actor:         contracts.Actor("human:owner"),
			Type:          contracts.EventTicketCreated,
			Project:       ticket.Project,
			TicketID:      ticket.ID,
			Payload:       ticket,
			SchemaVersion: contracts.CurrentSchemaVersion,
		}
		if err := eventsLog.AppendEvent(ctx, event); err != nil {
			t.Fatalf("append event: %v", err)
		}
		if err := store.ApplyEvent(ctx, event); err != nil {
			t.Fatalf("apply event: %v", err)
		}
	}

	board, err := store.QueryBoard(ctx, contracts.BoardQueryOptions{Project: "APP"})
	if err != nil {
		t.Fatalf("query board: %v", err)
	}
	if len(board.Columns[contracts.StatusBlocked]) != 1 || board.Columns[contracts.StatusBlocked][0].ID != "APP-1" {
		t.Fatalf("expected APP-1 in blocked column, got %#v", board.Columns[contracts.StatusBlocked])
	}
	if len(board.Columns[contracts.StatusDone]) != 1 || board.Columns[contracts.StatusDone][0].ID != "APP-2" {
		t.Fatalf("expected canceled ticket in done column, got %#v", board.Columns[contracts.StatusDone])
	}
	if len(board.Columns[contracts.StatusCanceled]) != 0 {
		t.Fatalf("expected canceled column to stay empty in board view, got %#v", board.Columns[contracts.StatusCanceled])
	}
}

func TestApplyLinkEventKeepsWrappedSnapshotsMaterialized(t *testing.T) {
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

	now := time.Date(2026, 3, 22, 18, 0, 0, 0, time.UTC)
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	eventsLog := &eventstore.Log{RootDir: root}
	sqlitePath := filepath.Join(storage.TrackerDir(root), "index.sqlite")
	store, err := Open(sqlitePath, ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()

	blocker := contracts.TicketSnapshot{
		ID:            "APP-2",
		Project:       "APP",
		Title:         "Task One",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		Blocks:        []string{"APP-4"},
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	blocked := contracts.TicketSnapshot{
		ID:            "APP-4",
		Project:       "APP",
		Title:         "Bug",
		Type:          contracts.TicketTypeBug,
		Status:        contracts.StatusBacklog,
		Priority:      contracts.PriorityMedium,
		BlockedBy:     []string{"APP-2"},
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	for _, ticket := range []contracts.TicketSnapshot{blocker, blocked} {
		if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
			t.Fatalf("create ticket markdown: %v", err)
		}
	}

	event := contracts.Event{
		EventID:       1,
		Timestamp:     now.Add(time.Minute),
		Actor:         contracts.Actor("human:owner"),
		Type:          contracts.EventTicketLinked,
		Project:       "APP",
		TicketID:      blocked.ID,
		Payload: map[string]any{
			"id":           blocked.ID,
			"other_id":     blocker.ID,
			"kind":         "blocked_by",
			"ticket":       blocked,
			"other_ticket": blocker,
		},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := store.ApplyEvent(ctx, event); err != nil {
		t.Fatalf("apply link event: %v", err)
	}

	board, err := store.QueryBoard(ctx, contracts.BoardQueryOptions{Project: "APP"})
	if err != nil {
		t.Fatalf("query board: %v", err)
	}
	if len(board.Columns[contracts.StatusBlocked]) != 1 || board.Columns[contracts.StatusBlocked][0].ID != "APP-4" {
		t.Fatalf("expected APP-4 in blocked column, got %#v", board.Columns[contracts.StatusBlocked])
	}
	if len(board.Columns[contracts.StatusReady]) != 1 || board.Columns[contracts.StatusReady][0].ID != "APP-2" {
		t.Fatalf("expected blocker to stay materialized, got %#v", board.Columns[contracts.StatusReady])
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
