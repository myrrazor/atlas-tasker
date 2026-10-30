package cli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func freeLoopbackPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// Edge 4: Ctrl-C (SIGINT) left `tracker serve` running when the process had
// inherited SIGINT as ignored, and otherwise killed it without a graceful
// shutdown. This runs the real binary, sends SIGINT and expects exit 0.
func TestServeStopsCleanlyOnSIGINT(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signals")
	}
	if testing.Short() {
		t.Skip("builds the binary")
	}
	bin := filepath.Join(t.TempDir(), "tracker")
	build := exec.Command("go", "build", "-o", bin, "../../cmd/tracker")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	for _, tc := range []struct {
		name string
		trap string
	}{
		{"default", ""},
		{"inherited-ignore", `trap "" INT; `},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			port := freeLoopbackPort(t)
			script := fmt.Sprintf(`%sexec "$0" serve --host 127.0.0.1 --port %d --no-browser --state-dir "$1"`, tc.trap, port)
			cmd := exec.Command("sh", "-c", script, bin, filepath.Join(home, "state"))
			cmd.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+home+"/c", "XDG_STATE_HOME="+home+"/s", "XDG_DATA_HOME="+home+"/d")
			stdout, _ := cmd.StdoutPipe()
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			ready := make(chan bool, 1)
			go func() {
				sc := bufio.NewScanner(stdout)
				for sc.Scan() {
					if strings.Contains(sc.Text(), "Atlas Home") {
						ready <- true
						break
					}
				}
				for sc.Scan() {
				}
				select {
				case ready <- false:
				default:
				}
			}()
			select {
			case ok := <-ready:
				if !ok {
					_ = cmd.Process.Kill()
					t.Fatalf("serve exited before ready: %s", stderr.String())
				}
			case <-time.After(20 * time.Second):
				_ = cmd.Process.Kill()
				t.Fatalf("serve never became ready: %s", stderr.String())
			}
			// Wait until the server answers /healthz so SIGINT lands on a
			// fully started process, not one still in startup.
			waitHealthz(t, port)
			if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
				t.Fatal(err)
			}
			waited := make(chan error, 1)
			go func() { waited <- cmd.Wait() }()
			select {
			case err := <-waited:
				if err != nil {
					t.Fatalf("serve should exit 0 after SIGINT, got %v\n%s", err, stderr.String())
				}
			case <-time.After(8 * time.Second):
				_ = cmd.Process.Kill()
				<-waited
				t.Fatalf("serve still running 8s after SIGINT\n%s", stderr.String())
			}
		})
	}
}

func TestServeSignalContextSecondSignalExits(t *testing.T) {
	exited := make(chan int, 1)
	oldExit := serveExit
	serveExit = func(code int) { exited <- code }
	t.Cleanup(func() { serveExit = oldExit })
	var stderr bytes.Buffer
	ctx, stop := serveSignalContext(context.Background(), &stderr, nil)
	defer stop()
	_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	select {
	case <-ctx.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("first SIGINT did not cancel the serve context")
	}
	_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	select {
	case code := <-exited:
		if code != 130 {
			t.Fatalf("second SIGINT exit code = %d, want 130", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("second SIGINT did not force an exit")
	}
}

func waitHealthz(t *testing.T, port int) {
	t.Helper()
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", port))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("serve on port %d never answered /healthz", port)
}

// A forced exit skips deferred cleanup, so the caller's hook must run first
// (web serve uses it to remove server.json instead of leaving a dead pid).
func TestServeSignalContextForcedExitRunsCleanup(t *testing.T) {
	exited := make(chan int, 1)
	oldExit := serveExit
	cleaned := make(chan struct{}, 1)
	serveExit = func(code int) {
		select {
		case <-cleaned:
			cleaned <- struct{}{}
		default:
			t.Error("forced exit ran before cleanup")
		}
		exited <- code
	}
	t.Cleanup(func() { serveExit = oldExit })
	ctx, stop := serveSignalContext(context.Background(), &bytes.Buffer{}, func() { cleaned <- struct{}{} })
	defer stop()
	_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	<-ctx.Done()
	_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	select {
	case code := <-exited:
		if code != 130 {
			t.Fatalf("exit code = %d, want 130", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("second SIGINT did not force an exit")
	}
}

func TestServeSignalContextBoundsHungShutdown(t *testing.T) {
	exited := make(chan int, 1)
	oldExit, oldGrace := serveExit, serveShutdownGrace
	serveExit = func(code int) { exited <- code }
	serveShutdownGrace = 200 * time.Millisecond
	t.Cleanup(func() { serveExit, serveShutdownGrace = oldExit, oldGrace })
	ctx, stop := serveSignalContext(context.Background(), &bytes.Buffer{}, nil)
	defer stop()
	_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
	<-ctx.Done()
	select {
	case code := <-exited:
		if code != 1 {
			t.Fatalf("hung shutdown exit code = %d, want 1", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("hung shutdown was not bounded")
	}
}
