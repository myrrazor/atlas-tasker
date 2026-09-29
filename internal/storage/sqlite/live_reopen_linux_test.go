//go:build linux

package sqlite

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

func countFDs() int {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return -1
	}
	return len(entries)
}

func posixLocksOn(pid int, path string) int {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0
	}
	inode := strconv.FormatUint(stat.Ino, 10)
	raw, err := os.ReadFile("/proc/locks")
	if err != nil {
		return 0
	}
	wantPID := strconv.Itoa(pid)
	n := 0
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 || fields[4] != wantPID {
			continue
		}
		parts := strings.Split(fields[5], ":")
		if len(parts) == 3 && parts[2] == inode {
			n++
		}
	}
	return n
}
