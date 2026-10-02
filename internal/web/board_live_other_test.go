//go:build !linux

package web

func posixLocksOn(int, string) int { return 0 }
