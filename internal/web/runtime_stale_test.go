package web

import (
	"os"
	"os/exec"
	"testing"
)

// A server.json left by a server that exited on a second Ctrl-C names a dead
// pid; it must read as stale, and a live pid must not.
func TestRuntimeStateStaleDeadPID(t *testing.T) {
	if RuntimeStateStale(RuntimeState{PID: os.Getpid()}) {
		t.Fatal("own pid reported stale")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if !RuntimeStateStale(RuntimeState{PID: cmd.Process.Pid}) {
		t.Fatalf("exited pid %d not reported stale", cmd.Process.Pid)
	}
	if !RuntimeStateStale(RuntimeState{}) {
		t.Fatal("zero pid not reported stale")
	}
}
