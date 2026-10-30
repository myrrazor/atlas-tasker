package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// isolatedServeEnv returns an environment whose HOME and XDG dirs live under
// t.TempDir() and whose machine settings disable the Atlas Home service, so
// nothing the test runs (including `init`) can start or reach a server on the
// default Home port.
func isolatedServeEnv(t *testing.T) (env []string, stateDir string) {
	t.Helper()
	home := t.TempDir()
	stateDir = filepath.Join(home, "s", "atlas-tasker")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	settings := `{"format":"atlas_machine_settings_v1","service":{"enabled":false,"auto_start":false},"agents":{"auto_install":false},"browser":{"open_home":false}}`
	if err := os.WriteFile(filepath.Join(stateDir, "settings.json"), []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+home+"/c", "XDG_STATE_HOME="+home+"/s", "XDG_DATA_HOME="+home+"/d")
	return env, stateDir
}

// sigCaught reports whether pid has a handler installed for SIGINT, read from
// /proc/<pid>/status. That is true once signal.Notify has run.
func sigCaught(pid int) bool {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "SigCgt:") {
			mask, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "SigCgt:")), 16, 64)
			return err == nil && mask&(1<<(uint(syscall.SIGINT)-1)) != 0
		}
	}
	return false
}

// waitForTrackerExe waits until pid has exec'd bin. The child is a shell that
// execs the binary, and until that happens the pid is still the shell:
// reading SigCgt or sending SIGINT then hits sh, which dies with SIGINT and
// the inherited-ignore case reports "signal: interrupt".
func waitForTrackerExe(t *testing.T, pid int, bin string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var last string
	var lastErr error
	for {
		exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
		if err == nil && exe == bin {
			return
		}
		if err == nil {
			last = exe
			lastErr = nil
		} else {
			lastErr = err
		}
		if !time.Now().Before(deadline) {
			if last != "" {
				t.Fatalf("pid %d did not exec %s before deadline (exe is %s)", pid, bin, last)
			}
			t.Fatalf("pid %d did not exec %s before deadline: %v", pid, bin, lastErr)
		}
		time.Sleep(100 * time.Microsecond)
	}
}

// Edge 4 follow-up: a SIGINT that arrives while serve or web serve is still
// starting (before the listener is bound) must stop the process, including
// when SIGINT was inherited as ignored. Every run uses a fresh free port and
// never the default Home port, and the child is always killed and reaped.
func TestServeStopsOnEarlySIGINT(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signals")
	}
	if testing.Short() {
		t.Skip("builds the binary")
	}
	bin := filepath.Join(t.TempDir(), "tracker")
	if out, err := exec.Command("go", "build", "-o", bin, "../../cmd/tracker").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	for _, mode := range []string{"serve", "web-serve"} {
		for _, tc := range []struct {
			name string
			trap string
		}{
			{"default", ""},
			{"inherited-ignore", `trap "" INT; `},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				if tc.trap != "" && runtime.GOOS != "linux" {
					t.Skip("waits on /proc/<pid>/status SigCgt")
				}
				env, stateDir := isolatedServeEnv(t)
				dir := t.TempDir()
				if mode == "web-serve" {
					initCmd := exec.Command(bin, "init", "--no-agents", "--no-register", "--no-backup", "--no-open")
					initCmd.Dir, initCmd.Env = dir, env
					if out, err := initCmd.CombinedOutput(); err != nil {
						t.Fatalf("init: %v\n%s", err, out)
					}
				}
				for i := 0; i < 10; i++ {
					port := freeLoopbackPort(t)
					if port == 7432 {
						t.Fatal("refusing to use the default Home port")
					}
					args := fmt.Sprintf(`serve --host 127.0.0.1 --port %d --no-browser --state-dir "$1"`, port)
					if mode == "web-serve" {
						args = fmt.Sprintf(`web serve --host 127.0.0.1 --port %d --no-browser`, port)
					}
					cmd := exec.Command("sh", "-c", tc.trap+`exec "$0" `+args, bin, stateDir)
					cmd.Dir, cmd.Env = dir, env
					var stderr bytes.Buffer
					cmd.Stderr = &stderr
					if err := cmd.Start(); err != nil {
						t.Fatal(err)
					}
					waited := make(chan error, 1)
					go func() { waited <- cmd.Wait() }()
					reaped := false
					t.Cleanup(func() {
						if !reaped {
							_ = cmd.Process.Kill()
							<-waited
						}
					})
					// The pid is the shell until exec. dash catches SIGINT, so
					// SigCgt can already name SIGINT before tracker is running.
					if runtime.GOOS == "linux" {
						waitForTrackerExe(t, cmd.Process.Pid, bin)
					}
					if tc.trap != "" {
						// Under an inherited SIG_IGN the kernel drops a SIGINT
						// sent before the Go runtime registers for it, so send
						// it the instant the handler is installed: that is
						// still before the bind.
						deadline := time.Now().Add(10 * time.Second)
						for !sigCaught(cmd.Process.Pid) && time.Now().Before(deadline) {
							time.Sleep(100 * time.Microsecond)
						}
					}
					_ = cmd.Process.Signal(syscall.SIGINT)
					select {
					case err := <-waited:
						reaped = true
						var exitErr *exec.ExitError
						if err == nil {
							break
						}
						if !errors.As(err, &exitErr) {
							t.Fatalf("wait: %v", err)
						}
						ws, _ := exitErr.Sys().(syscall.WaitStatus)
						if ws.Signaled() && ws.Signal() == syscall.SIGINT && tc.trap == "" {
							break // default disposition killed it before Notify
						}
						if ws.Exited() && ws.ExitStatus() == 130 {
							break
						}
						t.Fatalf("run %d: unexpected exit %v\n%s", i, err, stderr.String())
					case <-time.After(5 * time.Second):
						t.Fatalf("run %d: %s still running 5s after an early SIGINT\n%s", i, mode, stderr.String())
					}
				}
			})
		}
	}
}
