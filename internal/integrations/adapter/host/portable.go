package host

import (
	"os/exec"
	"path/filepath"

	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
)

// SameExecutable reports whether two paths resolve to the same file.
func SameExecutable(a, b string) bool {
	return resolveExecutable(a) == resolveExecutable(b) && resolveExecutable(a) != ""
}

func resolveExecutable(path string) string {
	path = filepath.Clean(path)
	if path == "" || path == "." {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

// PortableTrackerCommand returns the bare tracker name only when PATH resolves
// to the same file as trackerPath. Otherwise it keeps the absolute command so
// the registered server still launches.
func PortableTrackerCommand(trackerPath string) (command string, portable bool, warning string) {
	return portableTrackerCommand(trackerPath, exec.LookPath)
}

func portableTrackerCommand(trackerPath string, look func(string) (string, error)) (string, bool, string) {
	if look == nil {
		look = exec.LookPath
	}
	found, err := look(adapter.PortableExecutableName)
	if err == nil && found != "" && SameExecutable(found, trackerPath) {
		return adapter.PortableExecutableName, true, ""
	}
	note := "PATH does not resolve " + adapter.PortableExecutableName + " to this Atlas executable; keeping the absolute command so MCP still launches"
	if err == nil && found != "" {
		note += " (PATH found " + found + ")"
	}
	if trackerPath == "" {
		return adapter.PortableExecutableName, true, ""
	}
	return trackerPath, false, note
}
