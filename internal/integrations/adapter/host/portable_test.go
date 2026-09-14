package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
)

func TestPortableTrackerCommandUsesBareNameOnlyWhenPATHMatches(t *testing.T) {
	dir := t.TempDir()
	tracker := filepath.Join(dir, "tracker")
	if err := os.WriteFile(tracker, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	command, portable, warning := portableTrackerCommand(tracker, func(name string) (string, error) {
		if name == adapter.PortableExecutableName {
			return tracker, nil
		}
		return "", os.ErrNotExist
	})
	if command != adapter.PortableExecutableName || !portable || warning != "" {
		t.Fatalf("same PATH tracker: command=%q portable=%v warning=%q", command, portable, warning)
	}

	other := filepath.Join(t.TempDir(), "tracker")
	if err := os.WriteFile(other, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	command, portable, warning = portableTrackerCommand(tracker, func(name string) (string, error) {
		if name == adapter.PortableExecutableName {
			return other, nil
		}
		return "", os.ErrNotExist
	})
	if command != tracker || portable || !strings.Contains(warning, "PATH found") {
		t.Fatalf("different PATH tracker: command=%q portable=%v warning=%q", command, portable, warning)
	}

	command, portable, warning = portableTrackerCommand(tracker, func(string) (string, error) {
		return "", os.ErrNotExist
	})
	if command != tracker || portable || warning == "" {
		t.Fatalf("missing PATH tracker: command=%q portable=%v warning=%q", command, portable, warning)
	}
}

func TestSameExecutableFollowsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "tracker")
	if err := os.WriteFile(target, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link-tracker")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if !SameExecutable(link, target) {
		t.Fatal("symlink should resolve to the same executable")
	}
	other := filepath.Join(dir, "other")
	if err := os.WriteFile(other, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if SameExecutable(target, other) {
		t.Fatal("distinct files must not compare equal")
	}
}
