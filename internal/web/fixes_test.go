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

// A rejected edit must re-render THAT ticket's edit form with the submitted
// values — echoing them into whatever ticket the board auto-selects would
// hand the user a prefilled form that saves to the wrong ticket.
func TestRejectedEditEchoesIntoCorrectTicket(t *testing.T) {
	h := newWebHarness(t, false)
	ctx := context.Background()
	second, err := h.actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "WEB",
		Title:         "Second ticket",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusBacklog,
		Priority:      contracts.PriorityLow,
		CreatedAt:     h.now,
		UpdatedAt:     h.now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed second")
	if err != nil {
		t.Fatalf("seed second ticket: %v", err)
	}
	// stale CSRF: rejected in the middleware, where no handler supplies an id
	form := url.Values{"csrf_token": {"stale"}, "title": {"EDIT MEANT FOR SECOND"}}
	res := h.doAuthed(t, http.MethodPost, "/actions/tickets/"+second.ID+"/edit", form.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Origin":       "http://atlas.local",
	})
	if res.code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", res.code)
	}
	if !strings.Contains(res.body, "/actions/tickets/"+second.ID+"/edit") {
		t.Fatalf("expected the rejected ticket's own edit form to render, got:\n%s", excerpt(res.body, "/edit"))
	}
	// the page renders one detail drawer; the auto-selectable first ticket's
	// edit form must not appear at all, let alone carry the echoed values
	if strings.Contains(res.body, "/actions/tickets/"+h.ticketID+"/edit") {
		t.Fatalf("another ticket's edit form rendered on the rejected-edit page")
	}
}

// Embedded static files have zero modtimes, so FileServer emits no
// Last-Modified/ETag — without an explicit Cache-Control browsers
// heuristically cache app.js forever and users keep stale JS after
// upgrading the tracker binary.
func TestStaticAssetsRevalidate(t *testing.T) {
	h := newWebHarness(t, false)
	res := doRaw(t, h.handler, http.MethodGet, "/static/app.js", nil)
	if got := res.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("static assets must force revalidation, got Cache-Control=%q", got)
	}
}

func TestActionTargetParsing(t *testing.T) {
	for _, tt := range []struct {
		path   string
		target string
		id     string
	}{
		{"/actions/tickets/create", "create", ""},
		{"/actions/tickets/WEB-1/edit", "edit", "WEB-1"},
		{"/actions/tickets/WEB-1/label/add", "label/add", "WEB-1"},
		{"/tickets/WEB-1", "", ""},
		{"/board", "", ""},
		{"/actions/tickets/", "", ""},
	} {
		target, id := actionTarget(tt.path)
		if target != tt.target || id != tt.id {
			t.Fatalf("actionTarget(%q) = (%q, %q), want (%q, %q)", tt.path, target, id, tt.target, tt.id)
		}
	}
}

// The header's New Ticket link must not promote the implicit --project
// default into an explicit query param.
func TestNewTicketLinkOmitsImplicitProject(t *testing.T) {
	h := newWebHarness(t, false)
	implicit := h.doAuthed(t, http.MethodGet, "/board", "", nil)
	if strings.Contains(implicit.body, `href="/board?new=1&project=`) {
		t.Fatalf("New Ticket link exposes the implicit default project:\n%s", excerpt(implicit.body, "new=1"))
	}
	explicit := h.doAuthed(t, http.MethodGet, "/board?project=WEB", "", nil)
	if !strings.Contains(explicit.body, `href="/board?new=1&project=WEB"`) {
		t.Fatalf("New Ticket link should keep an explicit project:\n%s", excerpt(explicit.body, "new=1"))
	}
}

// When a rejected comment is echoed back, the Activity tab (where the echo
// lives) must be the active one — preserving text into a hidden tab reads
// as losing it.
func TestRejectedCommentActivatesActivityTab(t *testing.T) {
	h := newWebHarness(t, false)
	form := url.Values{"csrf_token": {"test-csrf"}, "body": {""}}
	res := h.doAuthed(t, http.MethodPost, "/actions/tickets/"+h.ticketID+"/comment", form.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Origin":       "http://atlas.local",
	})
	if res.code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.code)
	}
	if !strings.Contains(res.body, `tab-panel active" id="tab-activity"`) {
		t.Fatalf("expected Activity tab to be active on rejected comment:\n%s", excerpt(res.body, "tab-activity"))
	}
}

// Echoes are scoped to the form that was submitted: a rejected comment fills
// the comment box, not the edit form's audit reason.
func TestFormEchoScopedToSubmittedForm(t *testing.T) {
	h := newWebHarness(t, false)
	form := url.Values{
		"csrf_token": {"test-csrf"},
		"body":       {""}, // empty comment is rejected by the service
		"reason":     {"web ticket comment"},
	}
	res := h.doAuthed(t, http.MethodPost, "/actions/tickets/"+h.ticketID+"/comment", form.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Origin":       "http://atlas.local",
	})
	if res.code == http.StatusOK || res.code == http.StatusSeeOther {
		t.Fatalf("expected empty comment to be rejected, got %d", res.code)
	}
	if !strings.Contains(res.body, `value="web ticket edit"`) {
		t.Fatalf("edit form reason contaminated by the rejected comment form:\n%s", excerpt(res.body, "reason"))
	}
}

// The create re-render echoes the selects too, not just the text inputs.
func TestCreateEchoIncludesTypeAndPriority(t *testing.T) {
	h := newWebHarness(t, false)
	form := url.Values{
		"csrf_token": {"test-csrf"},
		"project":    {"WEB"},
		"title":      {"typed"},
		"type":       {"bug"},
		"priority":   {"critical"},
		"assignee":   {"nope"}, // rejected server-side
	}
	res := h.doAuthed(t, http.MethodPost, "/actions/tickets/create", form.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Origin":       "http://atlas.local",
	})
	if res.code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.code)
	}
	if !strings.Contains(res.body, `value="bug" selected`) || !strings.Contains(res.body, `value="critical" selected`) {
		t.Fatalf("expected type/priority selections to survive rejection:\n%s", excerpt(res.body, "option"))
	}
}

// A rejected comment must keep the typed body — losing it was the original
// complaint this mechanism exists to fix.
func TestRejectedCommentKeepsBody(t *testing.T) {
	h := newWebHarness(t, false)
	form := url.Values{
		"csrf_token": {"stale"}, // middleware rejection, worst case
		"body":       {"hard-won comment text"},
	}
	res := h.doAuthed(t, http.MethodPost, "/actions/tickets/"+h.ticketID+"/comment", form.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Origin":       "http://atlas.local",
	})
	if res.code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", res.code)
	}
	if !strings.Contains(res.body, "hard-won comment text") {
		t.Fatalf("rejected comment lost the typed body:\n%s", excerpt(res.body, "comment"))
	}
}

// Deliberately cleared fields stay cleared on a rejected edit: the submitted
// (empty) value wins over the stored one.
func TestClearedFieldsStayClearedOnRejectedEdit(t *testing.T) {
	h := newWebHarness(t, false)
	form := url.Values{
		"csrf_token":  {"test-csrf"},
		"title":       {"   "}, // rejected: title is required
		"description": {""},    // user cleared it on purpose
	}
	res := h.doAuthed(t, http.MethodPost, "/actions/tickets/"+h.ticketID+"/edit", form.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Origin":       "http://atlas.local",
	})
	if res.code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.code)
	}
	// the seeded description must NOT be resurrected into the edit textarea
	if strings.Contains(res.body, `name="description" rows="4">bad `) {
		t.Fatalf("cleared description was resurrected on rejected edit:\n%s", excerpt(res.body, "description"))
	}
}

// The filter form must not convert the server's implicit --project default
// into an explicit filter that empties saved views on first submit.
func TestFiltersFormDoesNotExposeImplicitProject(t *testing.T) {
	h := newWebHarness(t, false)
	implicit := h.doAuthed(t, http.MethodGet, "/board", "", nil)
	if !strings.Contains(implicit.body, `name="project" value=""`) {
		t.Fatalf("filters form should render an empty project input for the implicit default:\n%s", excerpt(implicit.body, `name="project"`))
	}
	explicit := h.doAuthed(t, http.MethodGet, "/board?project=WEB", "", nil)
	if !strings.Contains(explicit.body, `name="project" value="WEB"`) {
		t.Fatalf("explicit project must render in the filters form:\n%s", excerpt(explicit.body, `name="project"`))
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
