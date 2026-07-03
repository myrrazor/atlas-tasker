package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

func TestBoardThousandTicketPerformanceTarget(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 6, 16, 15, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	projects := mdstore.ProjectStore{RootDir: root}
	tickets := mdstore.TicketStore{RootDir: root, Clock: clock}
	events := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), tickets, events)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = projection.Close() })
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := projects.CreateProject(ctx, contracts.Project{Key: "WEB", Name: "Web", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	statuses := []contracts.Status{
		contracts.StatusBacklog,
		contracts.StatusReady,
		contracts.StatusInProgress,
		contracts.StatusInReview,
		contracts.StatusBlocked,
		contracts.StatusDone,
	}
	for i := 1; i <= 1000; i++ {
		ticket := contracts.NormalizeTicketSnapshot(contracts.TicketSnapshot{
			ID:            fmt.Sprintf("WEB-%d", i),
			Project:       "WEB",
			Title:         fmt.Sprintf("Performance ticket %04d", i),
			Type:          contracts.TicketTypeTask,
			Status:        statuses[i%len(statuses)],
			Priority:      contracts.PriorityMedium,
			Assignee:      contracts.Actor("agent:builder-1"),
			Labels:        []string{"web", "perf"},
			CreatedAt:     now.Add(time.Duration(i) * time.Millisecond),
			UpdatedAt:     now.Add(time.Duration(i) * time.Millisecond),
			SchemaVersion: contracts.CurrentSchemaVersion,
		})
		if err := tickets.CreateTicket(ctx, ticket); err != nil {
			t.Fatalf("create ticket %d: %v", i, err)
		}
		event := contracts.Event{
			EventID:       int64(i),
			Timestamp:     ticket.CreatedAt,
			Actor:         contracts.Actor("human:owner"),
			Type:          contracts.EventTicketCreated,
			Project:       "WEB",
			TicketID:      ticket.ID,
			Payload:       ticket,
			SchemaVersion: contracts.CurrentSchemaVersion,
			Metadata:      contracts.EventMetadata{Surface: contracts.EventSurfaceCLI},
		}
		if err := events.AppendEvent(ctx, event); err != nil {
			t.Fatalf("append event %d: %v", i, err)
		}
		if err := projection.ApplyEvent(ctx, event); err != nil {
			t.Fatalf("apply event %d: %v", i, err)
		}
	}
	queries := service.NewQueryService(root, projects, tickets, events, projection, clock)
	actions := service.NewActionService(root, projects, tickets, events, projection, clock, service.FileLockManager{Root: root}, nil, nil)
	srv, err := NewServer(Services{Actions: actions, Queries: queries}, Config{
		Root:      root,
		Workspace: "perf",
		Host:      "127.0.0.1",
		Project:   "WEB",
		Actor:     contracts.Actor("human:owner"),
		TokenMode: "random",
		Token:     "test-token",
		CSRFToken: "test-csrf",
		Clock:     clock,
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	queryStarted := time.Now()
	board, err := queries.Board(ctx, contracts.BoardQueryOptions{Project: "WEB"})
	if err != nil {
		t.Fatalf("query board: %v", err)
	}
	if got := countBoardTickets(board.Board); got != 1000 {
		t.Fatalf("expected 1000 tickets, got %d", got)
	}
	if elapsed := time.Since(queryStarted); elapsed > 250*time.Millisecond {
		// wall-clock targets flake on loaded runners; hard-fail only when asked
		if os.Getenv("ATLAS_PERF_STRICT") != "" {
			t.Fatalf("1000-ticket board query exceeded target: %s", elapsed)
		}
		t.Logf("1000-ticket board query exceeded 250ms target: %s (set ATLAS_PERF_STRICT=1 to fail)", elapsed)
	}

	req := httptest.NewRequest(http.MethodGet, "http://atlas.local/board?project=WEB", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "test-token"})
	res := httptest.NewRecorder()
	renderStarted := time.Now()
	srv.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("render board status = %d", res.Code)
	}
	if elapsed := time.Since(renderStarted); elapsed > 500*time.Millisecond {
		if os.Getenv("ATLAS_PERF_STRICT") != "" {
			t.Fatalf("1000-ticket board render exceeded target: %s", elapsed)
		}
		t.Logf("1000-ticket board render exceeded 500ms target: %s (set ATLAS_PERF_STRICT=1 to fail)", elapsed)
	}
}

func countBoardTickets(board contracts.BoardView) int {
	total := 0
	for _, tickets := range board.Columns {
		total += len(tickets)
	}
	return total
}
