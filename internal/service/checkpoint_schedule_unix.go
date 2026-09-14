//go:build unix

package service

import (
	"io/fs"
	"syscall"
)

func fileUID(info fs.FileInfo) (int, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(stat.Uid), true
}
