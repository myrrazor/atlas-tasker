package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestLiveBoardReindexSeesMarkdownAfterCommit(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
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
	ticket := contracts.NormalizeTicketSnapshot(contracts.TicketSnapshot{
		ID: "WEB-1", Project: "WEB", Title: "original title", Type: contracts.TicketTypeTask,
		Status: contracts.StatusBacklog, Priority: contracts.PriorityMedium,
		CreatedAt: now, UpdatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion,
	})
	if err := tickets.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if err := projection.Rebuild(ctx, ""); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	queries := service.NewQueryService(root, projects, tickets, events, projection, clock)
	actions := service.NewActionService(root, projects, tickets, events, projection, clock, service.FileLockManager{Root: root}, nil, nil)
	srv, err := NewServer(Services{Actions: actions, Queries: queries}, Config{
		Root: root, Workspace: "live", Host: "127.0.0.1", Project: "WEB",
		Actor: contracts.Actor("human:owner"), TokenMode: "random", Token: "test-token",
		CSRFToken: "test-csrf", Clock: clock,
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	handler := srv.Handler()
	first := liveBoard(t, handler, "")
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), "original title") {
		t.Fatalf("seed board = %d %s", first.Code, first.Body.String())
	}
	etag := first.Header().Get("ETag")
	if etag == "" || !strings.Contains(etag, "walgen=") {
		t.Fatalf("etag missing wal generation: %q", etag)
	}
	warm := liveBoard(t, handler, etag)
	if warm.Code != http.StatusNotModified {
		t.Fatalf("warm poll = %d %s", warm.Code, warm.Body.String())
	}

	path := filepath.Join(root, "projects", "WEB", "tickets", "WEB-1.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	edited := strings.Replace(string(raw), "title: original title", "title: reindex marker title", 1)
	if edited == string(raw) {
		t.Fatalf("title was not in the markdown file")
	}
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatalf("write ticket: %v", err)
	}
	// A WAL spill updates the file mtime before readers can see the commit.
	// The fingerprint must stay on the old rows across that gap.
	bumped := time.Now().Add(5 * time.Second)
	for _, name := range []string{"index.sqlite", "index.sqlite-wal"} {
		target := filepath.Join(storage.TrackerDir(root), name)
		if _, err := os.Stat(target); err != nil {
			continue
		}
		if err := os.Chtimes(target, bumped, bumped); err != nil {
			t.Fatalf("chtimes %s: %v", name, err)
		}
	}
	during := liveBoard(t, handler, etag)
	if during.Code == http.StatusNotModified {
		t.Fatal("file stamp change did not miss the fingerprint cache")
	}
	if strings.Contains(during.Body.String(), "reindex marker title") {
		t.Fatalf("board showed the markdown edit before reindex: %s", during.Body.String())
	}
	poisoned := during.Header().Get("ETag")
	if poisoned == "" {
		t.Fatal("missing etag during the pre-commit gap")
	}

	if err := projection.Rebuild(ctx, ""); err != nil {
		t.Fatalf("rebuild after edit: %v", err)
	}
	after := liveBoard(t, handler, poisoned)
	if after.Code != http.StatusOK || !strings.Contains(after.Body.String(), "reindex marker title") {
		t.Fatalf("live board after reindex = %d %s", after.Code, after.Body.String())
	}
	settled := liveBoard(t, handler, after.Header().Get("ETag"))
	if settled.Code != http.StatusNotModified {
		t.Fatalf("settled poll = %d %s", settled.Code, settled.Body.String())
	}
}

func liveBoard(t *testing.T, handler http.Handler, etag string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/board?project=WEB", nil)
	req.Header.Set("X-Atlas-Live", "1")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "test-token"})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}
