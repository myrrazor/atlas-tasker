package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

// The Referrer-Policy header must stay same-origin. With no-referrer, browsers
// serialize Origin as "null" on same-origin form POSTs (Fetch spec), which makes
// validateMutation reject the board's own forms with "cross-origin mutation
// rejected". Chromium repro lives in docs/web-board-security.md.
func TestReferrerPolicyAllowsSameOriginForms(t *testing.T) {
	h := newWebHarness(t, false)
	res := h.doAuthed(t, http.MethodGet, "/board", "", nil)
	if got := res.header.Get("Referrer-Policy"); got != "same-origin" {
		t.Fatalf("Referrer-Policy = %q, want same-origin (no-referrer breaks same-origin form POSTs)", got)
	}
}

// Origin: null (sandboxed iframes, data: URLs) must still be rejected even with
// a valid CSRF token.
func TestOriginNullMutationRejected(t *testing.T) {
	h := newWebHarness(t, false)
	form := url.Values{
		"csrf_token": {"test-csrf"},
		"project":    {"WEB"},
		"title":      {"nope"},
	}
	res := h.doAuthed(t, http.MethodPost, "/actions/tickets/create", form.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Accept":       "application/json",
		"Origin":       "null",
	})
	if res.code != http.StatusForbidden {
		t.Fatalf("expected Origin: null to be rejected, got %d body=%s", res.code, res.body)
	}
}

func TestBoardAPIDoesNotLeakCSRFToken(t *testing.T) {
	h := newWebHarness(t, false)
	res := h.doAuthed(t, http.MethodGet, "/api/board", "", map[string]string{"Accept": "application/json"})
	if res.code != http.StatusOK {
		t.Fatalf("board api status = %d", res.code)
	}
	if strings.Contains(res.body, "test-csrf") {
		t.Fatalf("/api/board payload leaks the CSRF token:\n%s", res.body)
	}
}

func TestCreateRejectsBornDoneAndInvalidStatus(t *testing.T) {
	h := newWebHarness(t, false)
	for _, tt := range []struct {
		status  string
		wantMsg string
	}{
		{"done", "not allowed on ticket create"},
		{"canceled", "not allowed on ticket create"},
		{"garbage-status", "invalid status"},
	} {
		form := url.Values{
			"csrf_token": {"test-csrf"},
			"project":    {"WEB"},
			"title":      {"born " + tt.status},
			"status":     {tt.status},
		}
		res := h.doAuthed(t, http.MethodPost, "/actions/tickets/create", form.Encode(), map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
			"Accept":       "application/json",
			"Origin":       "http://atlas.local",
		})
		if res.code != http.StatusBadRequest {
			t.Fatalf("create status=%s: expected 400, got %d body=%s", tt.status, res.code, res.body)
		}
		if !strings.Contains(res.body, tt.wantMsg) {
			t.Fatalf("create status=%s: expected %q in body, got %s", tt.status, tt.wantMsg, res.body)
		}
	}
}

func TestMoveInvalidStatusReturns400(t *testing.T) {
	h := newWebHarness(t, false)
	res := h.postMove(t, h.ticketID, "bogus")
	if res.code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid move status, got %d body=%s", res.code, res.body)
	}
	if !strings.Contains(res.body, "invalid status") {
		t.Fatalf("expected invalid status message, got %s", res.body)
	}
}

// Dragging a card onto the column it already occupies (which happens for
// dependency-blocked tickets rendered in the Blocked column) must be a no-op
// success, not "forbidden transition: ready -> ready".
func TestMoveToCurrentStatusIsNoOp(t *testing.T) {
	h := newWebHarness(t, false)
	res := h.postMove(t, h.ticketID, "ready")
	if res.code != http.StatusOK {
		t.Fatalf("expected same-status move to succeed as no-op, got %d body=%s", res.code, res.body)
	}
	if !strings.Contains(res.body, "already") {
		t.Fatalf("expected no-op flash mentioning already, got %s", res.body)
	}
}

func TestForbiddenTransitionMapsToConflict(t *testing.T) {
	h := newWebHarness(t, false)
	// Seeded ticket is ready; ready -> in_review is forbidden by the workflow.
	res := h.postMove(t, h.ticketID, "in_review")
	if res.code != http.StatusConflict {
		t.Fatalf("expected 409 for forbidden transition, got %d body=%s", res.code, res.body)
	}
	if !strings.Contains(res.body, `"conflict"`) {
		t.Fatalf("expected conflict code in envelope, got %s", res.body)
	}
}

// Plain form posts (no JS) must not dead-end on a raw text page OR silently
// redirect: they re-render the board with a real error status, the error
// banner, and the submitted values so nothing the user typed is lost.
func TestFormMutationErrorRendersBoardWithTypedValues(t *testing.T) {
	h := newWebHarness(t, false)
	form := url.Values{
		"csrf_token":  {"test-csrf"},
		"project":     {"WEB"},
		"title":       {"Payment retry hardening"},
		"description": {"long description the user must not lose"},
		"assignee":    {"bob"}, // invalid: actors need a human:/agent: prefix
	}
	res := h.doAuthed(t, http.MethodPost, "/actions/tickets/create", form.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Origin":       "http://atlas.local",
	})
	if res.code != http.StatusBadRequest {
		t.Fatalf("expected create rejection to keep its error status for non-JS posts, got %d", res.code)
	}
	for _, want := range []string{`role="alert"`, "invalid assignee", "long description the user must not lose", "Payment retry hardening"} {
		if !strings.Contains(res.body, want) {
			t.Fatalf("expected re-rendered form to contain %q, got:\n%s", want, excerpt(res.body, "alert"))
		}
	}
}

// Rejections in the security middleware (CSRF, origin) must keep their error
// status for every client — a 303 makes curl -L report success on a rejected
// mutation.
func TestCSRFFailureKeepsErrorStatusForBrowsers(t *testing.T) {
	h := newWebHarness(t, false)
	form := url.Values{"csrf_token": {"stale"}, "status": {"ready"}}
	res := h.doAuthed(t, http.MethodPost, "/actions/tickets/"+h.ticketID+"/move", form.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Origin":       "http://atlas.local",
	})
	if res.code != http.StatusForbidden {
		t.Fatalf("expected CSRF failure to stay 403 for HTML clients, got %d", res.code)
	}
	if !strings.Contains(res.body, `role="alert"`) || !strings.Contains(res.body, "invalid csrf token") {
		t.Fatalf("expected rendered error banner, got:\n%s", excerpt(res.body, "alert"))
	}
}

func TestBoardRendersErrorFlashParam(t *testing.T) {
	h := newWebHarness(t, false)
	res := h.doAuthed(t, http.MethodGet, "/board?error_flash=boom-xyz", "", nil)
	if res.code != http.StatusOK {
		t.Fatalf("board status = %d", res.code)
	}
	if !strings.Contains(res.body, "boom-xyz") || !strings.Contains(res.body, `role="alert"`) {
		t.Fatalf("expected error_flash to render as alert, body:\n%s", res.body)
	}
}

func TestCardCommentCountReflectsComments(t *testing.T) {
	h := newWebHarness(t, false)
	ctx := context.Background()
	for _, body := range []string{"first note", "second note"} {
		if err := h.actions.CommentTicket(ctx, h.ticketID, body, contracts.Actor("human:owner"), "test comment"); err != nil {
			t.Fatalf("comment: %v", err)
		}
	}
	res := h.doAuthed(t, http.MethodGet, "/board", "", nil)
	if !strings.Contains(res.body, "▱ 2") {
		t.Fatalf("expected card comment badge ▱ 2 in board HTML, got:\n%s", excerpt(res.body, "▱"))
	}
}

func TestDetailCommentsShowAuthorAndTime(t *testing.T) {
	h := newWebHarness(t, false)
	ctx := context.Background()
	if err := h.actions.CommentTicket(ctx, h.ticketID, "signed note", contracts.Actor("agent:zebra-99"), "test comment"); err != nil {
		t.Fatalf("comment: %v", err)
	}
	res := h.doAuthed(t, http.MethodGet, "/board?ticket="+url.QueryEscape(h.ticketID), "", nil)
	if !strings.Contains(res.body, "comment-meta") || !strings.Contains(res.body, "agent:zebra-99") {
		t.Fatalf("expected comment author metadata in detail HTML, got:\n%s", excerpt(res.body, "comment"))
	}
}

// A bare "agent:" actor (persistable through edit) must not panic the renderer.
func TestBoardSurvivesBarePrefixActor(t *testing.T) {
	h := newWebHarness(t, false)
	ctx := context.Background()
	if _, err := h.actions.MutateTrackedTicket(ctx, h.ticketID, contracts.Actor("human:owner"), "test", "set bare actor", func(ticket *contracts.TicketSnapshot) error {
		ticket.Assignee = contracts.Actor("agent:")
		return nil
	}); err != nil {
		t.Fatalf("mutate: %v", err)
	}
	res := h.doAuthed(t, http.MethodGet, "/board", "", nil)
	if res.code != http.StatusOK {
		t.Fatalf("board with bare-prefix actor: status %d", res.code)
	}
	if !strings.Contains(res.body, "board-grid") {
		t.Fatalf("board render truncated for bare-prefix actor:\n%s", res.body[:min(len(res.body), 400)])
	}
}

// Edit must not persist values the create path would reject: blank titles and
// malformed actors (the "agent:" case is also the actorInitials panic vector).
func TestEditRejectsBlankTitleAndInvalidActors(t *testing.T) {
	h := newWebHarness(t, false)
	for _, tt := range []struct {
		name string
		form url.Values
		want string
	}{
		{"blank title", url.Values{"title": {"   "}}, "title is required"},
		{"invalid assignee", url.Values{"assignee": {"agent:"}}, "invalid assignee"},
		{"invalid reviewer", url.Values{"reviewer": {"robot"}}, "invalid reviewer"},
	} {
		tt.form.Set("csrf_token", "test-csrf")
		res := h.doAuthed(t, http.MethodPost, "/actions/tickets/"+h.ticketID+"/edit", tt.form.Encode(), map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
			"Accept":       "application/json",
			"Origin":       "http://atlas.local",
		})
		if res.code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d body=%s", tt.name, res.code, res.body)
		}
		if !strings.Contains(res.body, tt.want) {
			t.Fatalf("%s: expected %q in body, got %s", tt.name, tt.want, res.body)
		}
	}
}

func TestActorInitialsEdgeCases(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want string
	}{
		{"agent:builder-1", "B1"},
		{"agent:", "--"},
		{"human:", "--"},
		{"human:Ünal", "Ü"},
		{"", "--"},
	} {
		if got := actorInitials(contracts.Actor(tt.in)); got != tt.want {
			t.Fatalf("actorInitials(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// Session cookies must not collide when two workspaces are served on
// different ports of 127.0.0.1 (browsers ignore ports for cookies).
func TestSessionCookieNameIncludesPort(t *testing.T) {
	h := newWebHarness(t, false)
	cfg := h.server.cfg
	cfg.Port = 4173
	srv, err := NewServer(Services{Actions: h.actions, Queries: h.queries}, cfg)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	rec := doRaw(t, srv.Handler(), http.MethodGet, "/board?token=test-token", nil)
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "atlas_web_session_4173" {
		t.Fatalf("expected port-scoped session cookie, got %#v", cookies)
	}
}

func TestFilterBoardAppliesAssigneeProjectType(t *testing.T) {
	board := contracts.BoardView{Columns: map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusReady: {
			{ID: "A-1", Project: "A", Type: contracts.TicketTypeBug, Assignee: contracts.Actor("agent:one")},
			{ID: "B-1", Project: "B", Type: contracts.TicketTypeTask, Assignee: contracts.Actor("agent:two")},
		},
	}}
	filtered := filterBoard(board, BoardPage{Assignee: "agent:one"})
	if got := len(filtered.Columns[contracts.StatusReady]); got != 1 || filtered.Columns[contracts.StatusReady][0].ID != "A-1" {
		t.Fatalf("assignee filter not applied, got %#v", filtered.Columns[contracts.StatusReady])
	}
	// project narrowing only applies to saved views, and only when the user
	// explicitly asked for a project — the SQL path scopes the direct board
	filtered = filterBoard(board, BoardPage{View: "some-view", Project: "B", ProjectExplicit: true})
	if got := len(filtered.Columns[contracts.StatusReady]); got != 1 || filtered.Columns[contracts.StatusReady][0].ID != "B-1" {
		t.Fatalf("explicit project filter not applied to saved view, got %#v", filtered.Columns[contracts.StatusReady])
	}
	filtered = filterBoard(board, BoardPage{Type: "bug"})
	if got := len(filtered.Columns[contracts.StatusReady]); got != 1 || filtered.Columns[contracts.StatusReady][0].ID != "A-1" {
		t.Fatalf("type filter not applied, got %#v", filtered.Columns[contracts.StatusReady])
	}
}

// A server started with --project APP must not empty a saved view that is
// scoped to a different project: the implicit default is not a user filter.
func TestSavedViewNotEmptiedByDefaultProject(t *testing.T) {
	board := contracts.BoardView{Columns: map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusReady: {
			{ID: "LIB-1", Project: "LIB", Type: contracts.TicketTypeTask},
		},
	}}
	filtered := filterBoard(board, BoardPage{View: "lib-board", Project: "APP", ProjectExplicit: false})
	if got := len(filtered.Columns[contracts.StatusReady]); got != 1 {
		t.Fatalf("saved view must ignore the server default project, got %#v", filtered.Columns)
	}
}

func TestUnsupportedSavedViewIsBadRequest(t *testing.T) {
	if got := statusForError(errUnsupportedSavedView("nope")); got != http.StatusBadRequest {
		t.Fatalf("unsupported saved view should map to 400, got %d", got)
	}
}

func TestFooterAdvertisesOnlyRealShortcutsAndNoDeadButtons(t *testing.T) {
	h := newWebHarness(t, false)
	res := h.doAuthed(t, http.MethodGet, "/board", "", nil)
	for _, ghost := range []string{"Change column", ">d</span>", "Column options"} {
		if strings.Contains(res.body, ghost) {
			t.Fatalf("board HTML still advertises unimplemented UI %q", ghost)
		}
	}
}

// Stopping server A must not delete the runtime state of a newer server B
// that overwrote the file in the same workspace.
func TestRuntimeStateClearOwnedOnly(t *testing.T) {
	root := t.TempDir()
	other := RuntimeState{Host: "127.0.0.1", Port: 2, URL: "http://127.0.0.1:2/board", PID: 999999, StartedAt: time.Now().UTC()}
	if err := WriteRuntimeState(root, other); err != nil {
		t.Fatalf("write runtime state: %v", err)
	}
	if err := ClearRuntimeStateOwnedBy(root, 1234); err != nil {
		t.Fatalf("clear owned: %v", err)
	}
	if _, err := ReadRuntimeState(root); err != nil {
		t.Fatal("state owned by another pid must survive cleanup")
	}
	mine := RuntimeState{Host: "127.0.0.1", Port: 3, URL: "http://127.0.0.1:3/board", PID: 1234, StartedAt: time.Now().UTC()}
	if err := WriteRuntimeState(root, mine); err != nil {
		t.Fatalf("write runtime state: %v", err)
	}
	if err := ClearRuntimeStateOwnedBy(root, 1234); err != nil {
		t.Fatalf("clear owned: %v", err)
	}
	if _, err := ReadRuntimeState(root); err == nil {
		t.Fatal("own state should be removed on shutdown")
	}
}

func TestRuntimeStateClearedHelper(t *testing.T) {
	root := t.TempDir()
	state := RuntimeState{Host: "127.0.0.1", Port: 1, URL: "http://127.0.0.1:1/board", PID: 1, StartedAt: time.Now().UTC()}
	if err := WriteRuntimeState(root, state); err != nil {
		t.Fatalf("write runtime state: %v", err)
	}
	if err := ClearRuntimeState(root); err != nil {
		t.Fatalf("clear runtime state: %v", err)
	}
	if _, err := ReadRuntimeState(root); err == nil {
		t.Fatal("expected runtime state to be gone after clear")
	}
	if err := ClearRuntimeState(root); err != nil {
		t.Fatalf("clear must be idempotent, got %v", err)
	}
}

// --- helpers ---

func (h webHarness) postMove(t *testing.T, ticketID string, status string) httpResult {
	t.Helper()
	form := url.Values{
		"csrf_token": {"test-csrf"},
		"status":     {status},
		"reason":     {"web drag move"},
	}
	return h.doAuthed(t, http.MethodPost, "/actions/tickets/"+ticketID+"/move", form.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Accept":       "application/json",
		"Origin":       "http://atlas.local",
	})
}

func excerpt(body string, needle string) string {
	idx := strings.Index(body, needle)
	if idx < 0 {
		return "(needle absent) len=" + strings.TrimSpace(body[:min(len(body), 120)])
	}
	start := max(0, idx-120)
	end := min(len(body), idx+240)
	return body[start:end]
}
