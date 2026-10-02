//go:build unix

package host

import (
	"os"
	"os/exec"
	"syscall"
)

// isolateCommand puts the child in its own process group so a timeout kills
// grandchildren that would otherwise keep the stdout pipe open.
func isolateCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
