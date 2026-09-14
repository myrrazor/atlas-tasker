package web

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/app"
)

func TestHomeServiceSharesStateDirWithChild(t *testing.T) {
	if os.Getenv("ATLAS_HOME_CHILD") == "1" {
		runHomeChild()
		return
	}
	home := t.TempDir()
	state := filepath.Join(home, "state")
	parent, err := app.Open(app.Options{
		Home:            home,
		StateDir:        state,
		LookPath:        func(string) (string, error) { return "", os.ErrNotExist },
		CommandRunner:   app.SilentRunner{},
		SkipHostInstall: true,
		Process:         app.NoopSpawner{},
		WriteClientCfg:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = parent.Close() })

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	cmd := exec.Command(os.Args[0], "-test.run=TestHomeServiceSharesStateDirWithChild", "-test.v=false")
	cmd.Env = append(os.Environ(),
		"ATLAS_HOME_CHILD=1",
		"ATLAS_TEST_HOME="+home,
		"ATLAS_TEST_STATE="+state,
		"ATLAS_TEST_PORT="+strconv.Itoa(port),
	)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })

	client := &http.Client{
		Timeout: 2 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	var lastErr error
	for i := 0; i < 40; i++ {
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/healthz", port), nil)
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.Header.Get("X-Atlas-Instance") == parent.Settings().InstanceID {
				lastErr = nil
				_ = body
				break
			}
			lastErr = fmt.Errorf("instance %s", resp.Header.Get("X-Atlas-Instance"))
		} else {
			lastErr = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("child Home did not share instance: %v", lastErr)
	}

	token, err := parent.IssueClaim()
	if err != nil {
		t.Fatal(err)
	}
	claim := fmt.Sprintf("http://127.0.0.1:%d%s", port, app.SessionClaimPath(token))
	resp, err := client.Get(claim)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("claim status %d", resp.StatusCode)
	}
	replay, err := client.Get(claim)
	if err != nil {
		t.Fatal(err)
	}
	replay.Body.Close()
	if replay.StatusCode != http.StatusUnauthorized {
		t.Fatalf("replay status %d", replay.StatusCode)
	}
}

func runHomeChild() {
	home := os.Getenv("ATLAS_TEST_HOME")
	state := os.Getenv("ATLAS_TEST_STATE")
	port, _ := strconv.Atoi(os.Getenv("ATLAS_TEST_PORT"))
	a, err := app.Open(app.Options{
		Home:            home,
		StateDir:        state,
		LookPath:        func(string) (string, error) { return "", os.ErrNotExist },
		CommandRunner:   app.SilentRunner{},
		SkipHostInstall: true,
		Process:         app.NoopSpawner{},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	srv, err := NewHomeServer(a, HomeConfig{Host: "127.0.0.1", Port: port, Actor: "human:owner"})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ln, _, err := ListenLoopback("127.0.0.1", port)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = srv.Serve(ctx, ln)
}

// The real child opens the same settings file before it can answer health.
// A fake prober cannot catch a startup lock that blocks that child.
func TestEnsureHomeServiceStartsRealChild(t *testing.T) {
	home := t.TempDir()
	state := filepath.Join(home, "state")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	spawner := &homeChildSpawner{t: t, home: home, state: state, port: port}
	parent, err := app.Open(app.Options{Home: home, StateDir: state,
		SkipHostInstall: true, Process: spawner,
		LookPath:      func(string) (string, error) { return "", os.ErrNotExist },
		CommandRunner: app.SilentRunner{},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = parent.Close() })
	service := parent.Settings().Service
	service.Port, service.Enabled, service.AutoStart = port, true, false
	if _, err := parent.UpdateSettings(context.Background(), app.MachineSettingsPatch{Service: &service}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	status, err := parent.EnsureService(ctx, app.ServiceOptions{})
	if err != nil {
		t.Fatalf("real Home startup: %v", err)
	}
	if !status.Running || !status.IdentityOK || status.PID == 0 {
		t.Fatalf("status: %+v", status)
	}
	reused, err := parent.EnsureService(ctx, app.ServiceOptions{})
	if err != nil || !reused.Reused || spawner.calls != 1 {
		t.Fatalf("reuse: %+v, calls=%d, err=%v", reused, spawner.calls, err)
	}
}

type homeChildSpawner struct {
	t           *testing.T
	home, state string
	port, calls int
}

func (s *homeChildSpawner) Start(_ context.Context, _ string, _ []string, _ []string) (int, error) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestHomeServiceSharesStateDirWithChild$", "-test.v=false")
	cmd.Env = append(os.Environ(), "ATLAS_HOME_CHILD=1", "ATLAS_TEST_HOME="+s.home,
		"ATLAS_TEST_STATE="+s.state, "ATLAS_TEST_PORT="+strconv.Itoa(s.port))
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	s.calls++
	s.t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	return cmd.Process.Pid, nil
}
