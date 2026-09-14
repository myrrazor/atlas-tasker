package uninstall

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
)

// HostCommandRunner is the production service-control runner. It only execs
// launchctl or systemctl with already-verified argv from a bound plan.
type HostCommandRunner struct{}

func (HostCommandRunner) Run(ctx context.Context, name string, args ...string) error {
	if err := allowlistedServiceArgv(name, args); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	text := strings.ToLower(string(out) + " " + err.Error())
	if alreadyStopped(text) {
		return nil
	}
	return fmt.Errorf("%s: %s", name, strings.TrimSpace(string(out)))
}

func alreadyStopped(text string) bool {
	for _, needle := range []string{
		"could not find specified service",
		"not loaded",
		"not been started",
		"not found",
		"could not find service",
		"unit not found",
		"not installed",
	} {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func allowlistedServiceArgv(name string, args []string) error {
	base := filepath.Base(name)
	switch base {
	case "launchctl":
		if len(args) == 2 && args[0] == "unload" && filepath.IsAbs(args[1]) && strings.HasSuffix(args[1], ".plist") {
			return nil
		}
		if len(args) == 3 && args[0] == "bootout" && isGUIDomain(args[1]) && filepath.IsAbs(args[2]) && strings.HasSuffix(args[2], ".plist") {
			return nil
		}
		if len(args) == 2 && args[0] == "bootout" && strings.HasPrefix(args[1], "gui/") && strings.Count(args[1], "/") == 2 {
			return nil
		}
	case "systemctl":
		if len(args) == 3 && args[0] == "--user" && (args[1] == "stop" || args[1] == "disable") && allowedUnitName(args[2]) {
			return nil
		}
	}
	return apperr.New(apperr.CodeConflict, "refusing arbitrary service command")
}

// FakeCommandRunner records allowlisted argv. Tests must inject this; it never
// talks to the host launchd/systemd.
type FakeCommandRunner struct {
	Calls  [][]string
	Err    error
	FailIf func(name string, args []string) error
}

func (f *FakeCommandRunner) Run(_ context.Context, name string, args ...string) error {
	if err := allowlistedServiceArgv(name, args); err != nil {
		return err
	}
	copied := append([]string{name}, args...)
	f.Calls = append(f.Calls, copied)
	if f.FailIf != nil {
		if err := f.FailIf(name, args); err != nil {
			return err
		}
	}
	return f.Err
}

func runnerFor(opts Options) CommandRunner {
	if opts.Command != nil {
		return opts.Command
	}
	return HostCommandRunner{}
}
