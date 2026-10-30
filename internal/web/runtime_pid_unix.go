//go:build !windows

package web

import (
	"errors"
	"syscall"
)

// pidAlive reports whether a process with pid exists. EPERM means it exists
// but belongs to another user.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
