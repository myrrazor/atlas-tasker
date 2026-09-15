package host

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
)

func TestDetectClientFindsCursorAgent(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "cursor-agent")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	detection := DetectClient(context.Background(), integrations.TargetCursor, adapter.PlanInput{
		WorkspaceRoot: t.TempDir(),
		Home:          t.TempDir(),
	}, adapter.DetectInput{
		LookPath: func(name string) (string, error) {
			if name == "cursor-agent" {
				return exe, nil
			}
			return "", os.ErrNotExist
		},
	})
	if !detection.Installed {
		t.Fatalf("cursor-agent should count as installed: %+v", detection)
	}
	if detection.ExecutablePath == "" {
		t.Fatal("missing executable path")
	}
	found := false
	for _, reason := range detection.Reasons {
		if strings.Contains(reason, "cursor-agent") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected cursor-agent reason, got %v", detection.Reasons)
	}
}
