package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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

type webHarness struct {
	server     *Server
	handler    http.Handler
	actions    *service.ActionService
	queries    *service.QueryService
	projection contracts.ProjectionStore
	root       string
	ticketID   string
	now        time.Time
}

func newWebHarness(t *testing.T, readOnly bool) webHarness {
	t.Helper()
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
	project := contracts.Project{
		Key:           "WEB",
		Name:          "Web",
		CreatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := projects.CreateProject(ctx, project); err != nil {
		t.Fatalf("create project: %v", err)
	}
	actions := service.NewActionService(root, projects, tickets, events, projection, clock, service.FileLockManager{Root: root}, nil, nil)
	queries := service.NewQueryService(root, projects, tickets, events, projection, clock)
	seedCtx := service.WithEventMetadata(ctx, service.EventMetaContext{Surface: contracts.EventSurfaceCLI})
	seed, err := actions.CreateTrackedTicket(seedCtx, contracts.TicketSnapshot{
		Project:            "WEB",
		Title:              `<script>alert("x")</script>`,
		Type:               contracts.TicketTypeTask,
		Status:             contracts.StatusReady,
		Priority:           contracts.PriorityHigh,
		Description:        "bad <img src=x onerror=alert(1)>",
		AcceptanceCriteria: []string{"render safely"},
		CreatedAt:          now,
		UpdatedAt:          now,
		SchemaVersion:      contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed")
	if err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	srv, err := NewServer(Services{Actions: actions, Queries: queries}, Config{
		Root:      root,
		Workspace: "test-workspace",
		Host:      "127.0.0.1",
		Project:   "WEB",
		Actor:     contracts.Actor("human:owner"),
		ReadOnly:  readOnly,
		TokenMode: "random",
		Token:     "test-token",
		CSRFToken: "test-csrf",
		Clock:     clock,
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	return webHarness{
		server:     srv,
		handler:    srv.Handler(),
		actions:    actions,
		queries:    queries,
		projection: projection,
		root:       root,
		ticketID:   seed.ID,
		now:        now,
	}
}

func TestNewServerRejectsUnsafeHostUnlessExplicit(t *testing.T) {
	h := newWebHarness(t, false)
	cfg := Config{
		Root:      h.root,
		Workspace: "test",
		Host:      "0.0.0.0",
		TokenMode: "random",
		Token:     "test-token",
		CSRFToken: "test-csrf",
		Clock:     func() time.Time { return h.now },
	}
	if _, err := NewServer(Services{Actions: h.actions, Queries: h.queries}, cfg); err == nil {
		t.Fatal("expected non-loopback host to require --unsafe-host")
	}
	cfg.UnsafeHost = true
	if _, err := NewServer(Services{Actions: h.actions, Queries: h.queries}, cfg); err != nil {
		t.Fatalf("expected unsafe host opt-in to pass: %v", err)
	}
}

func TestSecurityHeadersSessionAndNoCORS(t *testing.T) {
	h := newWebHarness(t, false)
	health := httptest.NewRecorder()
	h.handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "http://atlas.local/healthz", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health status = %d", health.Code)
	}
	if health.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("CORS header should not be set")
	}
	if health.Header().Get("Content-Security-Policy") == "" {
		t.Fatalf("CSP header missing: %#v", health.Header())
	}
	if health.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("nosniff header missing: %#v", health.Header())
	}

	unauthorized := httptest.NewRecorder()
	h.handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "http://atlas.local/board", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing session to return 401, got %d", unauthorized.Code)
	}

	preflight := httptest.NewRecorder()
	h.handler.ServeHTTP(preflight, httptest.NewRequest(http.MethodOptions, "http://atlas.local/board", nil))
	if preflight.Code != http.StatusForbidden {
		t.Fatalf("expected OPTIONS to be rejected, got %d", preflight.Code)
	}
	if preflight.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("OPTIONS should not opt into CORS")
	}

	static := httptest.NewRecorder()
	h.handler.ServeHTTP(static, httptest.NewRequest(http.MethodGet, "http://atlas.local/static/app.css", nil))
	if static.Code != http.StatusOK || !strings.Contains(static.Body.String(), "app-shell") {
		t.Fatalf("expected static CSS to be served, code=%d body=%s", static.Code, static.Body.String())
	}

	favicon := httptest.NewRecorder()
	h.handler.ServeHTTP(favicon, httptest.NewRequest(http.MethodGet, "http://atlas.local/favicon.ico", nil))
	if favicon.Code != http.StatusSeeOther || favicon.Header().Get("Location") != "/static/favicon.svg" {
		t.Fatalf("expected favicon redirect, code=%d location=%q", favicon.Code, favicon.Header().Get("Location"))
	}
}

func TestTokenQuerySetsStrictSessionCookie(t *testing.T) {
	h := newWebHarness(t, false)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://atlas.local/board?token=test-token&project=WEB", nil)
	h.handler.ServeHTTP(res, req)
	if res.Code != http.StatusSeeOther {
		t.Fatalf("expected token redirect, got %d", res.Code)
	}
	redirect, err := url.Parse(res.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse redirect: %v", err)
	}
	if redirect.Path != "/board" || redirect.Query().Get("project") != "WEB" || redirect.Query().Get("token") != "" {
		t.Fatalf("expected token to be stripped from redirect, got %q", res.Header().Get("Location"))
	}
	cookies := res.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one session cookie, got %#v", cookies)
	}
	cookie := cookies[0]
	if cookie.Name != sessionCookie || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("unexpected session cookie: %#v", cookie)
	}
}

func TestBoardEscapesTicketContent(t *testing.T) {
	h := newWebHarness(t, false)
	res := h.doAuthed(t, http.MethodGet, "/board?ticket="+url.QueryEscape(h.ticketID), "", nil)
	if res.code != http.StatusOK {
		t.Fatalf("board status = %d body=%s", res.code, res.body)
	}
	for _, unsafe := range []string{`<script>alert("x")</script>`, `<img src=x onerror=alert(1)>`} {
		if strings.Contains(res.body, unsafe) {
			t.Fatalf("board rendered unsafe content %q in:\n%s", unsafe, res.body)
		}
	}
	if !strings.Contains(res.body, "&lt;script&gt;") || !strings.Contains(res.body, "&lt;img") {
		t.Fatalf("expected escaped malicious content in board HTML:\n%s", res.body)
	}
}

func TestMutationCSRFOriginReadOnlyAndWebSurface(t *testing.T) {
	h := newWebHarness(t, false)
	form := url.Values{
		"project":  {"WEB"},
		"title":    {"Created from web"},
		"type":     {"task"},
		"status":   {"backlog"},
		"priority": {"medium"},
		"actor":    {"human:owner"},
		"reason":   {"web create test"},
	}

	missingCSRF := h.doAuthed(t, http.MethodPost, "/actions/tickets/create", form.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Accept":       "application/json",
	})
	if missingCSRF.code != http.StatusForbidden {
		t.Fatalf("expected missing csrf to be rejected, got %d body=%s", missingCSRF.code, missingCSRF.body)
	}

	crossOrigin := h.doAuthed(t, http.MethodPost, "/actions/tickets/create", withCSRF(form).Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Accept":       "application/json",
		"Origin":       "http://evil.local",
	})
	if crossOrigin.code != http.StatusForbidden {
		t.Fatalf("expected cross-origin mutation to be rejected, got %d body=%s", crossOrigin.code, crossOrigin.body)
	}

	created := h.doAuthed(t, http.MethodPost, "/actions/tickets/create", withCSRF(form).Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Accept":       "application/json",
		"Origin":       "http://atlas.local",
	})
	if created.code != http.StatusOK {
		t.Fatalf("expected create to pass, got %d body=%s", created.code, created.body)
	}
	ticketID := decodeActionTicketID(t, created.body)
	history, err := h.projection.QueryHistory(context.Background(), ticketID)
	if err != nil {
		t.Fatalf("query create history: %v", err)
	}
	if got := history[len(history)-1].Metadata.Surface; got != contracts.EventSurfaceWeb {
		t.Fatalf("expected web surface metadata, got %s", got)
	}

	comment := url.Values{"csrf_token": {"test-csrf"}, "body": {"Looks good from web"}, "actor": {"human:owner"}}
	commented := h.doAuthed(t, http.MethodPost, "/actions/tickets/"+ticketID+"/comment", comment.Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Accept":       "application/json",
		"Origin":       "http://atlas.local",
	})
	if commented.code != http.StatusOK {
		t.Fatalf("expected comment to pass, got %d body=%s", commented.code, commented.body)
	}
	history, err = h.projection.QueryHistory(context.Background(), ticketID)
	if err != nil {
		t.Fatalf("query comment history: %v", err)
	}
	if got := history[len(history)-1].Metadata.Surface; got != contracts.EventSurfaceWeb {
		t.Fatalf("expected comment web surface metadata, got %s", got)
	}

	readOnly := newWebHarness(t, true)
	blocked := readOnly.doAuthed(t, http.MethodPost, "/actions/tickets/create", withCSRF(form).Encode(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Accept":       "application/json",
		"Origin":       "http://atlas.local",
	})
	if blocked.code != http.StatusForbidden {
		t.Fatalf("expected read-only mutation rejection, got %d body=%s", blocked.code, blocked.body)
	}
}

func TestTicketDetailAndMissingTicketAPI(t *testing.T) {
	h := newWebHarness(t, false)
	ok := h.doAuthed(t, http.MethodGet, "/api/tickets/"+h.ticketID, "", map[string]string{"Accept": "application/json"})
	if ok.code != http.StatusOK {
		t.Fatalf("expected ticket API to pass, got %d body=%s", ok.code, ok.body)
	}
	missing := h.doAuthed(t, http.MethodGet, "/api/tickets/WEB-404", "", map[string]string{"Accept": "application/json"})
	if missing.code != http.StatusNotFound {
		t.Fatalf("expected missing ticket API to return 404, got %d body=%s", missing.code, missing.body)
	}
}

type httpResult struct {
	code   int
	body   string
	header http.Header
}

func (h webHarness) doAuthed(t *testing.T, method string, target string, body string, headers map[string]string) httpResult {
	t.Helper()
	req := httptest.NewRequest(method, "http://atlas.local"+target, strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "test-token"})
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	res := httptest.NewRecorder()
	h.handler.ServeHTTP(res, req)
	raw, err := io.ReadAll(res.Result().Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return httpResult{code: res.Code, body: string(raw), header: res.Header()}
}

func doRaw(t *testing.T, handler http.Handler, method string, target string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "http://atlas.local"+target, nil)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

func withCSRF(values url.Values) url.Values {
	next := url.Values{}
	for key, raw := range values {
		next[key] = append([]string(nil), raw...)
	}
	next.Set("csrf_token", "test-csrf")
	return next
}

func decodeActionTicketID(t *testing.T, raw string) string {
	t.Helper()
	var envelope struct {
		Payload struct {
			TicketID string `json:"ticket_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("decode action response: %v\n%s", err, raw)
	}
	if envelope.Payload.TicketID == "" {
		t.Fatalf("action response did not include ticket id: %s", raw)
	}
	return envelope.Payload.TicketID
}
