package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

func TestLiveBoardReindexIncludesAllTicketFields(t *testing.T) {
	h := newWebHarness(t, false)
	ticket, err := h.actions.Tickets.GetTicket(t.Context(), h.ticketID)
	if err != nil {
		t.Fatal(err)
	}
	// This imported Markdown ticket has no event snapshot to replay.
	ticket.ID = "WEB-2"
	if err := h.actions.Tickets.CreateTicket(t.Context(), ticket); err != nil {
		t.Fatal(err)
	}
	if err := h.projection.Rebuild(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	path := "/board?project=WEB&ticket=" + ticket.ID
	first := liveBoardAt(t, h.handler, "", path)
	// A manual edit or Git restore can change fields without updating the
	// timestamp. The live fingerprint must reflect what the board renders.
	ticket.Type = contracts.TicketTypeBug
	ticket.AcceptanceCriteria = []string{"updated acceptance"}
	if err := h.actions.Tickets.UpdateTicket(t.Context(), ticket); err != nil {
		t.Fatal(err)
	}
	if err := h.projection.Rebuild(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	after := liveBoardAt(t, h.handler, first.Header().Get("ETag"), path)
	var patch liveBoardBody
	if err := json.Unmarshal(after.Body.Bytes(), &patch); err != nil {
		t.Fatal(err)
	}
	if patch.Drawer == nil || patch.Drawer.Acceptance != "updated acceptance" {
		t.Fatalf("reindexed ticket changes were lost: %s", after.Body.String())
	}
	if !patch.Resync {
		found := false
		for _, card := range patch.Cards {
			if card.ID == ticket.ID && card.HTML != "" {
				found = true
			}
		}
		if !found {
			t.Fatalf("reindexed card was not patched: %s", after.Body.String())
		}
	}
}

func TestLiveBoardSavedViewKeepsItsFilters(t *testing.T) {
	h := newWebHarness(t, false)
	if err := (service.ViewStore{Root: h.root}).SaveView(contracts.SavedView{
		Name: "bugs", Kind: contracts.SavedViewKindBoard, Type: contracts.TicketTypeBug,
	}); err != nil {
		t.Fatal(err)
	}
	poll := func(etag string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/board?view=bugs", nil)
		req.Header.Set("X-Atlas-Live", "1")
		req.Header.Set("If-None-Match", etag)
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "test-token"})
		res := httptest.NewRecorder()
		h.handler.ServeHTTP(res, req)
		return res
	}
	first := poll("")
	if _, err := h.actions.MutateTrackedTicket(t.Context(), h.ticketID, "human:owner", "update task", "edit", func(ticket *contracts.TicketSnapshot) error {
		ticket.Title = "task outside the saved view"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	after := poll(first.Header().Get("ETag"))
	var patch liveBoardBody
	if err := json.Unmarshal(after.Body.Bytes(), &patch); err != nil {
		t.Fatal(err)
	}
	for _, card := range patch.Cards {
		if card.ID == h.ticketID && !card.Remove {
			t.Fatal("live update added a task to a bugs-only saved view")
		}
	}
	if patch.Resync {
		t.Fatal("a ticket edit must patch a saved view, not resync it")
	}
}

func TestLiveBoardSavedViewPatchesResolvedScope(t *testing.T) {
	h := newWebHarness(t, false)
	ctx := t.Context()
	if err := (mdstore.ProjectStore{RootDir: h.root}).CreateProject(ctx, contracts.Project{
		Key: "LIB", Name: "Lib", CreatedAt: h.now, SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatal(err)
	}
	bug, err := h.actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project: "WEB", Title: "bug in view", Type: contracts.TicketTypeBug,
		Status: contracts.StatusReady, Priority: contracts.PriorityMedium,
		CreatedAt: h.now, UpdatedAt: h.now, SchemaVersion: contracts.CurrentSchemaVersion,
	}, "human:owner", "seed bug")
	if err != nil {
		t.Fatal(err)
	}
	backlog, err := h.actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project: "WEB", Title: "backlog outside columns", Type: contracts.TicketTypeTask,
		Status: contracts.StatusBacklog, Priority: contracts.PriorityMedium,
		CreatedAt: h.now, UpdatedAt: h.now, SchemaVersion: contracts.CurrentSchemaVersion,
	}, "human:owner", "seed backlog")
	if err != nil {
		t.Fatal(err)
	}
	lib, err := h.actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project: "LIB", Title: "lib ticket", Type: contracts.TicketTypeTask,
		Status: contracts.StatusReady, Priority: contracts.PriorityMedium,
		CreatedAt: h.now, UpdatedAt: h.now, SchemaVersion: contracts.CurrentSchemaVersion,
	}, "human:owner", "seed lib")
	if err != nil {
		t.Fatal(err)
	}
	views := service.ViewStore{Root: h.root}
	if err := views.SaveView(contracts.SavedView{
		Name: "bugs", Kind: contracts.SavedViewKindBoard, Type: contracts.TicketTypeBug,
	}); err != nil {
		t.Fatal(err)
	}
	if err := views.SaveView(contracts.SavedView{
		Name: "ready-only", Kind: contracts.SavedViewKindBoard, Project: "WEB",
		Board: contracts.SavedBoardConfig{Columns: []contracts.Status{contracts.StatusReady}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := views.SaveView(contracts.SavedView{
		Name: "everything", Kind: contracts.SavedViewKindBoard,
	}); err != nil {
		t.Fatal(err)
	}
	if err := views.SaveView(contracts.SavedView{
		Name: "find-ready", Kind: contracts.SavedViewKindSearch, Query: "status=ready",
	}); err != nil {
		t.Fatal(err)
	}

	poll := func(path, etag string) *httptest.ResponseRecorder {
		t.Helper()
		return liveBoardAt(t, h.handler, etag, path)
	}
	mustPatch := func(path, etag string) (liveBoardBody, string) {
		t.Helper()
		res := poll(path, etag)
		if res.Code != http.StatusOK || !strings.Contains(res.Header().Get("Content-Type"), "application/json") {
			t.Fatalf("live poll %s = %d %s", path, res.Code, res.Body.String())
		}
		var body liveBoardBody
		if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Resync {
			t.Fatalf("resync on %s: %s", path, res.Body.String())
		}
		return body, res.Header().Get("ETag")
	}
	card := func(body liveBoardBody, id string) (liveCardPatch, bool) {
		for _, item := range body.Cards {
			if item.ID == id {
				return item, true
			}
		}
		return liveCardPatch{}, false
	}
	editTitle := func(id, title string) {
		t.Helper()
		if _, err := h.actions.MutateTrackedTicket(ctx, id, "human:owner", "edit", "retitle", func(ticket *contracts.TicketSnapshot) error {
			ticket.Title = title
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}

	first := poll("/board?view=bugs", "")
	if !strings.Contains(first.Header().Get("ETag"), "db:view=") {
		t.Fatalf("saved view stamp missing db:view: %s", first.Header().Get("ETag"))
	}
	editTitle(bug.ID, "bug entered the view")
	entered, etag := mustPatch("/board?view=bugs", first.Header().Get("ETag"))
	got, ok := card(entered, bug.ID)
	if !ok || got.Remove || !strings.Contains(got.HTML, "bug entered the view") {
		t.Fatalf("bug edit did not enter the view: %#v", got)
	}
	editTitle(h.ticketID, "task stays outside")
	outside, etag := mustPatch("/board?view=bugs", etag)
	if got, ok = card(outside, h.ticketID); ok && !got.Remove {
		t.Fatalf("task edit entered a bugs view: %#v", got)
	}
	if _, err := h.actions.MutateTrackedTicket(ctx, bug.ID, "human:owner", "edit", "retype", func(ticket *contracts.TicketSnapshot) error {
		ticket.Type = contracts.TicketTypeTask
		ticket.Title = "bug left the view"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	left, etag := mustPatch("/board?view=bugs", etag)
	if got, ok = card(left, bug.ID); !ok || !got.Remove {
		t.Fatalf("ticket that left the view was not removed: %#v", got)
	}

	readyPage := poll("/board?view=ready-only", "")
	editTitle(h.ticketID, "ready column title")
	readyPatch, readyETag := mustPatch("/board?view=ready-only", readyPage.Header().Get("ETag"))
	if got, ok = card(readyPatch, h.ticketID); !ok || got.Remove || !strings.Contains(got.HTML, "ready column title") {
		t.Fatalf("ready column dropped an in-scope card: %#v", got)
	}
	editTitle(backlog.ID, "backlog column title")
	backlogPatch, colETag := mustPatch("/board?view=ready-only", readyETag)
	if got, ok = card(backlogPatch, backlog.ID); ok && !got.Remove {
		t.Fatalf("column filter kept a backlog card: %#v", got)
	}
	if _, err := h.actions.MoveTicket(ctx, h.ticketID, contracts.StatusInProgress, "human:owner", "leave ready"); err != nil {
		t.Fatal(err)
	}
	leftReady, _ := mustPatch("/board?view=ready-only", colETag)
	if got, ok = card(leftReady, h.ticketID); !ok || !got.Remove {
		t.Fatalf("ticket that left the ready column was not removed: %#v", got)
	}
	if _, err := h.actions.MoveTicket(ctx, backlog.ID, contracts.StatusReady, "human:owner", "enter ready"); err != nil {
		t.Fatal(err)
	}
	enteredReady, _ := mustPatch("/board?view=ready-only", colETag)
	if got, ok = card(enteredReady, backlog.ID); !ok || got.Remove || !strings.Contains(got.HTML, "backlog column title") {
		t.Fatalf("ticket that entered the ready column was not added: %#v", got)
	}

	allPage := poll("/board?view=everything", "")
	editTitle(lib.ID, "lib stays visible")
	allPatch, _ := mustPatch("/board?view=everything", allPage.Header().Get("ETag"))
	if got, ok = card(allPatch, lib.ID); !ok || got.Remove || !strings.Contains(got.HTML, "lib stays visible") {
		t.Fatalf("server default project hid another project's ticket: %#v", got)
	}
	// A different query is a different board. The first poll is the full page;
	// the following edit must still patch, and the explicit project drops LIB.
	narrow := poll("/board?view=everything&project=WEB", "")
	if narrow.Code != http.StatusOK || narrow.Header().Get("ETag") == "" {
		t.Fatalf("explicit project board = %d", narrow.Code)
	}
	editTitle(lib.ID, "lib hidden by project filter")
	hidden, _ := mustPatch("/board?view=everything&project=WEB", narrow.Header().Get("ETag"))
	if got, ok = card(hidden, lib.ID); ok && !got.Remove {
		t.Fatalf("explicit project filter kept LIB: %#v", got)
	}
	typedPage := poll("/board?view=everything&type=bug", "")
	if _, err := h.actions.MutateTrackedTicket(ctx, backlog.ID, "human:owner", "edit", "retype", func(ticket *contracts.TicketSnapshot) error {
		ticket.Type = contracts.TicketTypeBug
		ticket.Title = "bug passes url type filter"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	typed, typeETag := mustPatch("/board?view=everything&type=bug", typedPage.Header().Get("ETag"))
	if got, ok = card(typed, backlog.ID); !ok || got.Remove || !strings.Contains(got.HTML, "bug passes url type filter") {
		t.Fatalf("url type filter dropped an in-scope bug: %#v", got)
	}
	editTitle(lib.ID, "task blocked by url type")
	blocked, _ := mustPatch("/board?view=everything&type=bug", typeETag)
	if got, ok = card(blocked, lib.ID); ok && !got.Remove {
		t.Fatalf("url type filter kept a task: %#v", got)
	}

	defined := poll("/board?view=bugs", etag)
	before := defined.Header().Get("ETag")
	if err := views.SaveView(contracts.SavedView{
		Name: "bugs", Kind: contracts.SavedViewKindBoard, Type: contracts.TicketTypeTask,
	}); err != nil {
		t.Fatal(err)
	}
	changed := poll("/board?view=bugs", before)
	var changedBody liveBoardBody
	if changed.Code != http.StatusOK || !strings.Contains(changed.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("view edit poll = %d %s", changed.Code, changed.Body.String())
	}
	if err := json.Unmarshal(changed.Body.Bytes(), &changedBody); err != nil {
		t.Fatal(err)
	}
	if !changedBody.Resync {
		t.Fatalf("editing the saved view did not resync: %s", changed.Body.String())
	}
	if !strings.Contains(changed.Header().Get("ETag"), "db:view=") || changed.Header().Get("ETag") == before {
		t.Fatalf("view edit did not change the stamp: %s", changed.Header().Get("ETag"))
	}

	searchFirst := poll("/board?view=find-ready", "")
	editTitle(h.ticketID, "search view must not patch")
	searchAfter := poll("/board?view=find-ready", searchFirst.Header().Get("ETag"))
	if strings.Contains(searchAfter.Header().Get("Content-Type"), "application/json") {
		var searchBody liveBoardBody
		if err := json.Unmarshal(searchAfter.Body.Bytes(), &searchBody); err != nil {
			t.Fatal(err)
		}
		if !searchBody.Resync {
			t.Fatal("a search saved view took the id patch path")
		}
	}
}

func TestLiveBoardResyncRefreshesDrawerActivity(t *testing.T) {
	h := newWebHarness(t, false)
	if err := h.actions.CommentTicket(t.Context(), h.ticketID, "new comment after a large update", "human:owner", "review test"); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/board?project=WEB&ticket="+h.ticketID, nil)
	patch, ok := h.server.liveResync(req)
	if !ok || !strings.Contains(patch.CommentsHTML, "new comment after a large update") || patch.HistoryHTML == "" {
		t.Fatal("full board resync must also refresh the open drawer's activity")
	}
}

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
	if etag == "" || !strings.Contains(etag, "data_version=") {
		t.Fatalf("etag missing sqlite data_version: %q", etag)
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

func TestFingerprintTracksReviewerAndBlockedColumn(t *testing.T) {
	base := contracts.TicketSnapshot{ID: "A", Status: contracts.StatusReady, Title: "a", UpdatedAt: time.Unix(1, 0).UTC()}
	blocker := contracts.TicketSnapshot{ID: "B", Status: contracts.StatusReady, Title: "b", UpdatedAt: time.Unix(1, 0).UTC()}
	fp1, _ := fingerprintTickets([]contracts.TicketSnapshot{base, blocker})
	withReviewer := base
	withReviewer.Reviewer = contracts.Actor("human:owner")
	fp2, _ := fingerprintTickets([]contracts.TicketSnapshot{withReviewer, blocker})
	if fp1 == fp2 {
		t.Fatal("reviewer change did not change the live fingerprint")
	}
	blocked := base
	blocked.BlockedBy = []string{"B"}
	fp3, _ := fingerprintTickets([]contracts.TicketSnapshot{blocked, blocker})
	if fp1 == fp3 {
		t.Fatal("an open blocker did not change the live fingerprint")
	}
	done := blocker
	done.Status = contracts.StatusDone
	fp4, _ := fingerprintTickets([]contracts.TicketSnapshot{blocked, done})
	if fp3 == fp4 {
		t.Fatal("finishing the blocker did not change the live fingerprint")
	}
}

func TestLivePollKeepsSQLiteSHMLock(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	projects := mdstore.ProjectStore{RootDir: root}
	tickets := mdstore.TicketStore{RootDir: root, Clock: clock}
	events := &eventstore.Log{RootDir: root}
	dbPath := filepath.Join(storage.TrackerDir(root), "index.sqlite")
	projection, err := sqlitestore.Open(dbPath, tickets, events)
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
		ID: "WEB-1", Project: "WEB", Title: "lock probe", Type: contracts.TicketTypeTask,
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
	etag := ""
	for i := 0; i < 8; i++ {
		res := liveBoard(t, handler, etag)
		if res.Code != http.StatusOK && res.Code != http.StatusNotModified {
			t.Fatalf("poll %d = %d", i, res.Code)
		}
		if next := res.Header().Get("ETag"); next != "" {
			etag = next
		}
	}
	shm := dbPath + "-shm"
	info, err := os.Stat(shm)
	if err != nil {
		t.Fatalf("stat shm: %v", err)
	}
	if info.Size() < 32768 {
		t.Fatalf("shm shrank before the other process opened it: %d", info.Size())
	}
	// /proc/locks is Linux-only; the storm, sidecar, and freshness checks
	// below still exercise the actual SQLite behavior on every platform.
	locks := posixLocksOn(os.Getpid(), shm)
	if runtime.GOOS == "linux" && locks == 0 {
		t.Fatal("live polls dropped the process lock on index.sqlite-shm")
	}
	before, _ := projection.ProjectionGeneration(ctx)
	script := `
import sqlite3, sys
path = sys.argv[1]
for i in range(40):
    con = sqlite3.connect(path, timeout=5)
    con.execute("select count(*) from tickets")
    if i % 5 == 0:
        con.execute("update tickets set title = ?", ("stormed-title",))
        con.commit()
    con.close()
`
	cmd := exec.Command("python3", "-c", script, dbPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sqlite open/close storm: %v %s", err, out)
	}
	afterInfo, err := os.Stat(shm)
	if err != nil {
		t.Fatalf("stat shm after storm: %v", err)
	}
	if afterInfo.Size() < 32768 {
		t.Fatalf("another process truncated index.sqlite-shm to %d bytes", afterInfo.Size())
	}
	if locksAfter := posixLocksOn(os.Getpid(), shm); runtime.GOOS == "linux" && locksAfter == 0 {
		t.Fatal("sqlite lock on index.sqlite-shm was gone after other processes closed it")
	}
	got, err := projection.QueryBoard(ctx, contracts.BoardQueryOptions{Project: "WEB"})
	if err != nil {
		t.Fatalf("query after storm: %v", err)
	}
	backlog := got.Columns[contracts.StatusBacklog]
	if len(backlog) != 1 || backlog[0].Title != "stormed-title" {
		t.Fatalf("board after storm = %#v", got.Columns)
	}
	after, ok := projection.ProjectionGeneration(ctx)
	if !ok || after == "" || after == before {
		t.Fatalf("data_version did not move after another process committed: before %q after %q", before, after)
	}
}

func TestLiveBoardTwoTabsPatchWhileTailIsReadable(t *testing.T) {
	h := newWebHarness(t, false)
	firstA := liveBoard(t, h.handler, "")
	firstB := liveBoard(t, h.handler, "")
	if firstA.Code != http.StatusOK || firstB.Code != http.StatusOK {
		t.Fatalf("seed polls = %d %d", firstA.Code, firstB.Code)
	}
	etagA := firstA.Header().Get("ETag")
	etagB := firstB.Header().Get("ETag")
	if !strings.Contains(etagA, "db:fp=") || !strings.Contains(etagB, "db:fp=") {
		t.Fatalf("seed etag missing fingerprint: %s", etagA)
	}
	var latest string
	for i := 0; i < 5; i++ {
		latest = fmt.Sprintf("live wave %d", i)
		if _, err := h.actions.MutateTrackedTicket(t.Context(), h.ticketID, "human:owner", "live write", "edit", func(ticket *contracts.TicketSnapshot) error {
			ticket.Title = latest
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		res := liveBoard(t, h.handler, etagA)
		assertLivePatch(t, res, latest)
		etagA = res.Header().Get("ETag")
		if !strings.Contains(etagA, "db:fp=") {
			t.Fatalf("etag after write omitted fingerprint: %s", etagA)
		}
	}
	lagging := liveBoard(t, h.handler, etagB)
	assertLivePatch(t, lagging, latest)
	unknown := withLiveFingerprint(etagB, "deadbeef")
	missed := liveBoard(t, h.handler, unknown)
	assertLivePatch(t, missed, latest)
}

func TestLiveBoardResyncsOnceWhenServerInstanceChanges(t *testing.T) {
	h := newWebHarness(t, false)
	first := liveBoard(t, h.handler, "")
	if first.Code != http.StatusOK {
		t.Fatalf("seed = %d", first.Code)
	}
	etag := first.Header().Get("ETag")
	if !strings.Contains(etag, "db:boot=") {
		t.Fatalf("stamp missing boot id: %s", etag)
	}
	previous := liveInstanceID
	liveInstanceID = previous + "-restarted"
	t.Cleanup(func() { liveInstanceID = previous })
	restarted := liveBoard(t, h.handler, etag)
	if restarted.Code != http.StatusOK {
		t.Fatalf("restart poll = %d %s", restarted.Code, restarted.Body.String())
	}
	var body liveBoardBody
	if err := json.Unmarshal(restarted.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Resync {
		t.Fatalf("restart with a foreign boot id did not resync: %s", restarted.Body.String())
	}
	next := restarted.Header().Get("ETag")
	if !strings.Contains(next, liveInstanceID) {
		t.Fatalf("resync etag missing new boot id: %s", next)
	}
	if _, err := h.actions.MutateTrackedTicket(t.Context(), h.ticketID, "human:owner", "live write", "edit", func(ticket *contracts.TicketSnapshot) error {
		ticket.Title = "after restart"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	follow := liveBoard(t, h.handler, next)
	assertLivePatch(t, follow, "after restart")
}

func assertLivePatch(t *testing.T, res *httptest.ResponseRecorder, title string) {
	t.Helper()
	if res.Code != http.StatusOK {
		t.Fatalf("live poll = %d %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("live poll fell through to HTML: %s", res.Body.String())
	}
	var patch liveBoardBody
	if err := json.Unmarshal(res.Body.Bytes(), &patch); err != nil {
		t.Fatal(err)
	}
	if patch.Resync {
		t.Fatalf("resync while the event tail is readable: %s", res.Body.String())
	}
	if title != "" && !strings.Contains(res.Body.String(), title) {
		t.Fatalf("patch missed %q: %s", title, res.Body.String())
	}
}

func withLiveFingerprint(etag, fp string) string {
	const mark = "db:fp="
	i := strings.Index(etag, mark)
	if i < 0 {
		return etag
	}
	rest := etag[i+len(mark):]
	end := strings.IndexAny(rest, "|\"")
	if end < 0 {
		return etag[:i+len(mark)] + fp
	}
	return etag[:i+len(mark)] + fp + rest[end:]
}

func liveBoard(t *testing.T, handler http.Handler, etag string) *httptest.ResponseRecorder {
	t.Helper()
	return liveBoardAt(t, handler, etag, "/board?project=WEB")
}

func liveBoardAt(t *testing.T, handler http.Handler, etag, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+path, nil)
	req.Header.Set("X-Atlas-Live", "1")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "test-token"})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}
