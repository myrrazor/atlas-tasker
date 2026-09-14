package host

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
)

var versionPattern = regexp.MustCompile(`(\d+)\.(\d+)(?:\.(\d+))?`)

// LookPath is the DetectInput lookup used when the caller did not inject one.
func LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

// DetectClient fills the common detection fields for a named client.
func DetectClient(ctx context.Context, target integrations.Target, input adapter.PlanInput, detect adapter.DetectInput) adapter.Detection {
	caps, err := adapter.CapabilitiesFor(target)
	if err != nil {
		return adapter.Detection{Target: target, VersionSupport: adapter.VersionUnknown, Reasons: []string{err.Error()}}
	}
	out := adapter.Detection{
		Target:         target,
		VersionSupport: adapter.VersionUnknown,
	}
	look := detect.LookPath
	if look == nil {
		look = LookPath
	}
	if caps.ClientExecutable != "" {
		if exe, err := look(caps.ClientExecutable); err == nil && exe != "" {
			if abs, err := filepath.Abs(exe); err == nil {
				cleaned := filepath.Clean(abs)
				if filepath.IsAbs(cleaned) {
					out.Installed = true
					out.ExecutablePath = cleaned
				}
			}
		}
	}
	home := detect.Home
	if home == "" {
		home = input.Home
	}
	root := detect.WorkspaceRoot
	if root == "" {
		root = input.WorkspaceRoot
	}
	out.ConfigLocations = configLocations(caps, root, home, detect.Lstat)
	if out.Installed && detect.Runner != nil && len(caps.VersionArgs) > 0 {
		cmd := adapter.Command{
			Purpose:    adapter.CommandPurposeDetectVersion,
			Executable: out.ExecutablePath,
			Args:       append([]string(nil), caps.VersionArgs...),
			Timeout:    8 * time.Second,
		}
		if err := cmd.Validate(); err == nil {
			result, err := detect.Runner.Run(ctx, cmd)
			probe := adapter.ProbeRecord{Command: cmd, ExitCode: result.ExitCode, TimedOut: result.TimedOut}
			if err != nil {
				probe.Summary = err.Error()
			} else {
				probe.Summary = strings.TrimSpace(string(result.Stdout))
			}
			out.Probes = append(out.Probes, probe)
			if result.ExitCode == 0 && !result.TimedOut {
				out.Version = ParseVersion(string(result.Stdout) + "\n" + string(result.Stderr))
				if out.Version.Known {
					out.VersionSupport = adapter.VersionSupported
				}
			}
		}
	} else if out.Installed {
		out.Reasons = append(out.Reasons, "client is installed but no version probe ran")
	}
	return out
}

func configLocations(caps adapter.Capabilities, root, home string, lstat func(string) (os.FileInfo, error)) []adapter.ConfigLocation {
	if lstat == nil {
		lstat = os.Lstat
	}
	var out []adapter.ConfigLocation
	for _, scope := range caps.Scopes {
		path, ok := scope.ResolvePath(root, home)
		if !ok {
			continue
		}
		loc := adapter.ConfigLocation{Scope: scope.Scope, Path: path, Format: scope.Format}
		if info, err := lstat(path); err == nil {
			loc.Exists = true
			loc.Symlink = info.Mode()&os.ModeSymlink != 0
		} else if IsSymlinkPath(path, lstat) {
			loc.Symlink = true
		}
		out = append(out, loc)
	}
	return out
}

// ParseVersion extracts the first X.Y or X.Y.Z from client version text.
func ParseVersion(raw string) adapter.ClientVersion {
	v := adapter.ClientVersion{Raw: strings.TrimSpace(raw)}
	m := versionPattern.FindStringSubmatch(v.Raw)
	if m == nil {
		return v
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch := 0
	if m[3] != "" {
		patch, _ = strconv.Atoi(m[3])
	}
	v.Major, v.Minor, v.Patch, v.Known = major, minor, patch, true
	return v
}

// IsSymlinkPath is true when path or a parent is a symlink.
func IsSymlinkPath(path string, lstat func(string) (os.FileInfo, error)) bool {
	if lstat == nil {
		lstat = os.Lstat
	}
	current := path
	for {
		info, err := lstat(current)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
		current = parent
	}
}

// DefaultRunner executes a validated Command and captures stdout/stderr.
type DefaultRunner struct{}

func (DefaultRunner) Run(ctx context.Context, command adapter.Command) (adapter.CommandResult, error) {
	if err := command.Validate(); err != nil {
		return adapter.CommandResult{}, err
	}
	runCtx := ctx
	var cancel context.CancelFunc
	if command.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, command.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(runCtx, command.Executable, command.Args...)
	if command.Dir != "" {
		cmd.Dir = command.Dir
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := adapter.CommandResult{Stdout: []byte(stdout.String()), Stderr: []byte(stderr.String())}
	if runCtx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
	}
	if err == nil {
		return result, nil
	}
	if exit, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exit.ExitCode()
		return result, err
	}
	result.ExitCode = -1
	return result, err
}
