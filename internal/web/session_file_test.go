package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHomeRejectsExpiredPersistedSession(t *testing.T) {
	session, err := OpenSession(filepath.Join(t.TempDir(), "session.json"))
	if err != nil {
		t.Fatal(err)
	}
	srv := &HomeServer{session: session, token: session.Token()}
	session.expires = time.Now().Add(-time.Minute)
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/", nil)
	req.Header.Set("X-Atlas-Live", "1")
	req.AddCookie(&http.Cookie{Name: srv.sessionCookieName(), Value: srv.token})
	res := httptest.NewRecorder()
	if srv.validSession(res, req) || res.Code != http.StatusUnauthorized {
		t.Fatalf("expired session accepted: status %d", res.Code)
	}
}

func TestHomeClaimRenewsExpiredPersistedSession(t *testing.T) {
	srv, application, _ := newHomeHarness(t)
	session, err := OpenSession(filepath.Join(t.TempDir(), "session.json"))
	if err != nil {
		t.Fatal(err)
	}
	srv.session, srv.token, srv.csrf = session, session.Token(), session.CSRF()
	oldToken := srv.token
	session.expires = time.Now().Add(-time.Minute)
	claim, err := application.IssueClaim()
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/session/claim/"+claim, nil)
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, req)
	cookies := res.Result().Cookies()
	if res.Code != http.StatusSeeOther || len(cookies) != 1 || cookies[0].Value == oldToken || !session.Matches(cookies[0].Value) {
		t.Fatal("claim did not issue a fresh usable session")
	}
	form := url.Values{"csrf_token": {session.CSRF()}}
	post := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/actions/settings", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := srv.validateMutation(post); err != nil {
		t.Fatalf("fresh session CSRF rejected: %v", err)
	}
	if session.Matches(oldToken) {
		t.Fatal("expired cookie survived a fresh sign-in")
	}
}

func TestSessionFailedExpiryRotationPreservesState(t *testing.T) {
	session, err := OpenSession(filepath.Join(t.TempDir(), "session.json"))
	if err != nil {
		t.Fatal(err)
	}
	oldToken, oldCSRF := session.Token(), session.CSRF()
	session.expires = time.Now().Add(-time.Minute)
	// A directory cannot be replaced by the session document.
	session.path = t.TempDir()
	_, changed := session.Maintain(time.Now())
	if changed || session.Token() != oldToken || session.CSRF() != oldCSRF {
		t.Fatal("failed rotation replaced the persisted session in memory")
	}
}

func TestHomeConcurrentSessionRotation(t *testing.T) {
	session, err := OpenSession(filepath.Join(t.TempDir(), "session.json"))
	if err != nil {
		t.Fatal(err)
	}
	oldToken, csrf := session.Token(), session.CSRF()
	session.issued = time.Now().Add(-sessionRotateEvery - time.Minute)
	srv := &HomeServer{session: session, token: oldToken, csrf: csrf}
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/", nil)
			req.AddCookie(&http.Cookie{Name: srv.sessionCookieName(), Value: oldToken})
			if !srv.validSession(httptest.NewRecorder(), req) {
				t.Error("cookie within rotation overlap was rejected")
			}
			if !session.Matches(srv.sessionToken()) || srv.pageBase("home").CSRFToken != csrf {
				t.Error("page used stale session state")
			}
		})
	}
	workers.Wait()
	if srv.sessionToken() == oldToken {
		t.Fatal("session did not rotate")
	}
	reopened, err := OpenSession(session.path)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.Matches(srv.sessionToken()) || !reopened.Matches(oldToken) || reopened.CSRF() != csrf {
		t.Fatal("rotation did not survive reopening the session file")
	}
}
