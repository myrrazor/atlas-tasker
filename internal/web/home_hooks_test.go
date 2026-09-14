package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/app"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

func TestBoardDoesNotAutoSelectTicket(t *testing.T) {
	h := newWebHarness(t, false)
	res := h.doAuthed(t, http.MethodGet, "/board", "", nil)
	if strings.Contains(res.body, `class="detail-drawer"`) || strings.Contains(res.body, "No ticket selected") {
		t.Fatal("unspecified ticket query must omit the detail drawer")
	}
	page, err := h.server.buildBoardPage(t.Context(), httptest.NewRequest(http.MethodGet, "/board", nil))
	if err != nil {
		t.Fatal(err)
	}
	if page.Detail != nil {
		t.Fatalf("detail should be nil, got %s", page.Detail.View.Ticket.ID)
	}
	if page.DisplayName == "" {
		t.Fatal("DisplayName should be set")
	}
	if len(page.Projects) == 0 {
		t.Fatal("Projects should list workspace projects")
	}
}

func TestNotesSaveFromEditForm(t *testing.T) {
	h := newWebHarness(t, false)
	form := url.Values{
		"csrf_token": {"test-csrf"},
		"notes":      {"keep this"},
		"reason":     {"web edit ticket"},
	}
	res := h.doAuthed(t, http.MethodPost, "/actions/tickets/"+h.ticketID+"/edit", form.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Origin":       "http://atlas.local",
	})
	if res.code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", res.code, res.body)
	}
	ticket, err := h.actions.Tickets.GetTicket(t.Context(), h.ticketID)
	if err != nil {
		t.Fatal(err)
	}
	if ticket.Notes != "keep this" {
		t.Fatalf("notes=%q", ticket.Notes)
	}
}

func TestBulkTicketActionUsesSharedService(t *testing.T) {
	h := newWebHarness(t, false)
	form := url.Values{
		"csrf_token": {"test-csrf"},
		"kind":       {string(service.BulkOperationMove)},
		"status":     {string(contracts.StatusInProgress)},
		"ticket_id":  {h.ticketID},
		"confirm":    {"1"},
		"reason":     {"web bulk"},
	}
	res := h.doAuthed(t, http.MethodPost, "/actions/tickets/bulk", form.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Origin":       "http://atlas.local",
		"Accept":       "application/json",
	})
	if res.code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.code, res.body)
	}
	if !strings.Contains(res.body, `"kind": "atlas_web_bulk"`) && !strings.Contains(res.body, `"kind":"atlas_web_bulk"`) {
		t.Fatalf("bulk payload:\n%s", res.body)
	}
}

func TestActionSuccessRedirectsWithPrefix(t *testing.T) {
	h := newWebHarness(t, false)
	prefixed, err := NewServer(Services{Actions: h.actions, Queries: h.queries}, Config{
		Root:        h.root,
		Workspace:   "ws-1",
		Host:        "127.0.0.1",
		Project:     "WEB",
		Actor:       contracts.Actor("human:owner"),
		TokenMode:   "random",
		Token:       "test-token",
		CSRFToken:   "test-csrf",
		Clock:       func() time.Time { return h.now },
		RoutePrefix: "/w/ws-1",
		BoardPath:   "/w/ws-1/projects/WEB",
		HomePath:    "/w/ws-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"csrf_token": {"test-csrf"}, "title": {"renamed"}, "reason": {"web edit"}}
	req := httptest.NewRequest(http.MethodPost, "http://atlas.local/actions/tickets/"+h.ticketID+"/edit", strings.NewReader(form.Encode()))
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "test-token"})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://atlas.local")
	rec := httptest.NewRecorder()
	prefixed.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/w/ws-1/projects/WEB?") {
		t.Fatalf("redirect %s", loc)
	}
}

func TestHomeBareURLOffersClaimPage(t *testing.T) {
	srv, application, _ := newHomeHarness(t)
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("bare home status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self'") || strings.Contains(csp, "unsafe-inline") {
		t.Fatalf("csp=%q", csp)
	}
	if strings.Contains(body, "location.hash") || strings.Contains(body, "<script>") && !strings.Contains(body, `src="/static/claim.js"`) {
		t.Fatalf("inline claim script under CSP:\n%s", body)
	}
	if !strings.Contains(body, `src="/static/claim.js"`) {
		t.Fatalf("missing external claim script:\n%s", body)
	}
	if !strings.Contains(body, "Sign in on this computer") {
		t.Fatalf("missing claim copy:\n%s", body)
	}
	if strings.Contains(body, srv.token) {
		t.Fatal("claim page leaked the session secret")
	}
	js := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/static/claim.js", nil)
	jsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(jsRec, js)
	if jsRec.Code != http.StatusOK || !strings.Contains(jsRec.Body.String(), "history.replaceState") {
		t.Fatalf("claim.js status=%d body=%s", jsRec.Code, jsRec.Body.String())
	}
	if !strings.Contains(jsRec.Body.String(), `fetch("/session/claim"`) {
		t.Fatal("claim.js must POST the token instead of putting it in the URL")
	}
	token, err := application.IssueClaim()
	if err != nil {
		t.Fatal(err)
	}
	claim := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/session/claim", strings.NewReader("claim="+token))
	claim.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	claim.Header.Set("Origin", "http://127.0.0.1")
	claimRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(claimRec, claim)
	if claimRec.Code != http.StatusSeeOther {
		t.Fatalf("claim consume status=%d body=%s", claimRec.Code, claimRec.Body.String())
	}
}

func TestHomeAgentsAndGrantsAPI(t *testing.T) {
	srv, application, _ := newHomeHarness(t)
	grant, err := application.GrantPath(t.Context(), application.Home(), app.PathGrantInit)
	if err != nil {
		t.Fatal(err)
	}
	agents := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/agents", nil)
	agents.AddCookie(&http.Cookie{Name: srv.sessionCookieName(), Value: "home-token"})
	agentsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(agentsRec, agents)
	if agentsRec.Code != http.StatusOK {
		t.Fatalf("agents %d %s", agentsRec.Code, agentsRec.Body.String())
	}
	if strings.Contains(agentsRec.Body.String(), `"connected"`) {
		t.Fatalf("fake connected status: %s", agentsRec.Body.String())
	}
	grants := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/grants", nil)
	grants.AddCookie(&http.Cookie{Name: srv.sessionCookieName(), Value: "home-token"})
	grantsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(grantsRec, grants)
	if grantsRec.Code != http.StatusOK || !strings.Contains(grantsRec.Body.String(), grant.ID) {
		t.Fatalf("grants %d %s", grantsRec.Code, grantsRec.Body.String())
	}
}
