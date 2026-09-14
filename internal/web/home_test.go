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

	"github.com/myrrazor/atlas-tasker/internal/app"
)

func newHomeHarness(t *testing.T) (*HomeServer, *app.App, string) {
	t.Helper()
	home := t.TempDir()
	application, err := app.Open(app.Options{
		Home:            home,
		StateDir:        filepath.Join(home, "state"),
		LookPath:        func(string) (string, error) { return "", os.ErrNotExist },
		CommandRunner:   app.SilentRunner{},
		SkipHostInstall: true,
		Process:         app.NoopSpawner{},
		WriteClientCfg:  true,
		Now:             func() time.Time { return time.Date(2026, 9, 14, 15, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Close() })
	root := filepath.Join(home, "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := application.Init(context.Background(), app.InitOptions{Root: root, Register: true, DefaultProject: true}); err != nil && !app.IsPartial(err) {
		t.Fatal(err)
	}
	srv, err := NewHomeServer(application, HomeConfig{
		Host:  "127.0.0.1",
		Port:  7432,
		Actor: "human:owner",
		Token: "home-token",
		CSRF:  "home-csrf",
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv, application, root
}

func TestHomeListsRegisteredWorkspace(t *testing.T) {
	srv, application, _ := newHomeHarness(t)
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/", nil)
	req.AddCookie(&http.Cookie{Name: srv.sessionCookieName(), Value: "home-token"})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	listed, _ := application.ListWorkspaces(context.Background(), app.ListOptions{})
	if len(listed) != 1 {
		t.Fatalf("expected one workspace, got %d", len(listed))
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("home status=%d", rec.Code)
	}
	api := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/workspaces/", nil)
	api.AddCookie(&http.Cookie{Name: srv.sessionCookieName(), Value: "home-token"})
	apiRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(apiRec, api)
	if apiRec.Code != http.StatusOK || !strings.Contains(apiRec.Body.String(), listed[0].WorkspaceID) {
		t.Fatalf("workspace API missing id: %d %s", apiRec.Code, apiRec.Body.String())
	}
}

func TestHomeRejectsArbitraryInitPath(t *testing.T) {
	srv, _, _ := newHomeHarness(t)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/actions/workspaces/init", strings.NewReader("csrf_token=home-csrf&path=/tmp/nope"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://127.0.0.1")
	req.AddCookie(&http.Cookie{Name: srv.sessionCookieName(), Value: "home-token"})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected grant requirement, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestHomeWorkspaceProjectBoardIsStateful(t *testing.T) {
	srv, application, _ := newHomeHarness(t)
	listed, _ := application.ListWorkspaces(context.Background(), app.ListOptions{})
	id := listed[0].WorkspaceID
	ws, err := application.Hub().Bind(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	projects, err := ws.Queries.Projects.ListProjects(context.Background())
	if err != nil || len(projects) == 0 {
		t.Fatalf("projects: %v %#v", err, projects)
	}
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/workspaces/"+id+"/board?project="+projects[0].Key, nil)
	req.AddCookie(&http.Cookie{Name: srv.sessionCookieName(), Value: "home-token"})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), projects[0].Key) && !strings.Contains(rec.Body.String(), `"kind"`) {
		t.Fatalf("board API missing payload:\n%s", rec.Body.String())
	}
}

func TestHomeHealthIdentity(t *testing.T) {
	srv, application, _ := newHomeHarness(t)
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("X-Atlas-Service") != "atlas-home" {
		t.Fatalf("health headers: %d %#v", rec.Code, rec.Header())
	}
	if rec.Header().Get("X-Atlas-Instance") != application.Settings().InstanceID {
		t.Fatalf("instance header %q", rec.Header().Get("X-Atlas-Instance"))
	}
}

func TestHomeClaimSetsCookieWithoutQueryToken(t *testing.T) {
	srv, application, _ := newHomeHarness(t)
	token, err := application.IssueClaim()
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/session/claim/"+token, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("claim: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].Value == token {
		t.Fatalf("cookie must be HttpOnly and not the claim token: %#v", cookies)
	}
	replay := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/session/claim/"+token, nil)
	replayRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(replayRec, replay)
	if replayRec.Code != http.StatusUnauthorized {
		t.Fatalf("replay claim status=%d", replayRec.Code)
	}
	secret := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/session/claim/home-token", nil)
	secretRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(secretRec, secret)
	if secretRec.Code != http.StatusUnauthorized {
		t.Fatalf("cookie secret must not work as a claim: %d", secretRec.Code)
	}
}

func TestHomeRejectsFakeHostAndAdversarialOrigin(t *testing.T) {
	srv, _, _ := newHomeHarness(t)
	health := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/healthz", nil)
	health.Host = "evil.example"
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, health)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("fake host health status=%d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/actions/workspaces/init", strings.NewReader("csrf_token=home-csrf&grant_id=x"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://127.0.0.1.attacker.example")
	req.AddCookie(&http.Cookie{Name: srv.sessionCookieName(), Value: "home-token"})
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("suffix origin status=%d %s", rec.Code, rec.Body.String())
	}
	nullReq := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/actions/workspaces/init", strings.NewReader("csrf_token=home-csrf&grant_id=x"))
	nullReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	nullReq.Header.Set("Origin", "null")
	nullReq.AddCookie(&http.Cookie{Name: srv.sessionCookieName(), Value: "home-token"})
	nullRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(nullRec, nullReq)
	if nullRec.Code != http.StatusForbidden {
		t.Fatalf("null origin status=%d", nullRec.Code)
	}
}

func TestHomeReadOnlyBlocksMutations(t *testing.T) {
	_, application, _ := newHomeHarness(t)
	ro, err := NewHomeServer(application, HomeConfig{Host: "127.0.0.1", Port: 7432, Actor: "human:owner", Token: "home-token", CSRF: "home-csrf", ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/actions/workspaces/init", strings.NewReader("csrf_token=home-csrf&grant_id=x"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://127.0.0.1")
	req.AddCookie(&http.Cookie{Name: ro.sessionCookieName(), Value: "home-token"})
	rec := httptest.NewRecorder()
	ro.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("read-only status=%d %s", rec.Code, rec.Body.String())
	}
}

func TestHomeCrossOriginClaimRejected(t *testing.T) {
	srv, application, _ := newHomeHarness(t)
	token, err := application.IssueClaim()
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/session/claim/"+token, nil)
	req.Header.Set("Origin", "http://evil.example")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-origin claim status=%d", rec.Code)
	}
}
