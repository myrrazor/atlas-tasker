//go:build !linux

package sqlite

func countFDs() int { return -1 }

func posixLocksOn(int, string) int { return 0 }
