package cli

import (
	"os"
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
