//go:build !unix

package host

import "os/exec"

func isolateCommand(cmd *exec.Cmd) {}
