package cli

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	webui "github.com/myrrazor/atlas-tasker/internal/web"
)

func TestWebOpenFailsFastWhenServerDown(t *testing.T) {
	withTempWorkspace(t)
	if out, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// Stale state from a server that is long gone (port 1 is never listening).
	state := webui.RuntimeState{Host: "127.0.0.1", Port: 1, URL: "http://127.0.0.1:1/board", PID: 999999, StartedAt: time.Now().UTC()}
	if err := webui.WriteRuntimeState(wd, state); err != nil {
		t.Fatalf("write runtime state: %v", err)
	}
	opened := false
	old := openURLFunc
	openURLFunc = func(string) error {
		opened = true
		return nil
	}
	t.Cleanup(func() { openURLFunc = old })

	out, err := runCLI(t, "web", "open")
	if err == nil {
		t.Fatalf("expected web open to fail when the recorded server is down, got:\n%s", out)
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Fatalf("expected a 'not running' hint, got: %v", err)
	}
	if opened {
		t.Fatal("web open must not launch a browser at a dead URL")
	}
}

// Edge 23: `web open` used to open the bare /board URL, which the board
// answers with 401 because it has no session cookie yet.
func TestWebOpenOpensAuthenticatedSessionURL(t *testing.T) {
	withTempWorkspace(t)
	if out, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	session, err := webui.OpenSession(filepath.Join(wd, ".tracker", "web-session.json"))
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(health.Close)
	addr := health.Listener.Addr().(*net.TCPAddr)
	boardURL := fmt.Sprintf("http://127.0.0.1:%d/board", addr.Port)
	state := webui.RuntimeState{Host: "127.0.0.1", Port: addr.Port, URL: boardURL, PID: os.Getpid(), StartedAt: time.Now().UTC()}
	if err := webui.WriteRuntimeState(wd, state); err != nil {
		t.Fatalf("write runtime state: %v", err)
	}
	var opened string
	old := openURLFunc
	openURLFunc = func(raw string) error {
		opened = raw
		return nil
	}
	t.Cleanup(func() { openURLFunc = old })

	out, err := runCLI(t, "web", "open")
	if err != nil {
		t.Fatalf("web open failed: %v\n%s", err, out)
	}
	u, err := url.Parse(opened)
	if err != nil {
		t.Fatalf("parse opened URL %q: %v", opened, err)
	}
	if u.Path != "/board" || u.Query().Get("token") != session.Token() {
		t.Fatalf("web open must open the token session URL, opened %q", opened)
	}
	if strings.Contains(out, session.Token()) {
		t.Fatalf("web open must not print the session token:\n%s", out)
	}
}
