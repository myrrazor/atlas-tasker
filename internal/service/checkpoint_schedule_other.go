//go:build !unix

package service

import "io/fs"

func fileUID(info fs.FileInfo) (int, bool) {
	return 0, false
}
