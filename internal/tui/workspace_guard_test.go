package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestNewModelRejectsInvalidRootsWithoutCreatingState(t *testing.T) {
	for _, nested := range []bool{false, true} {
		name := "uninitialized"
		if nested {
			name = "nested"
		}
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			root := parent
			wantMessage := "tracker init"
			if nested {
				if err := os.Mkdir(storage.TrackerDir(parent), 0o755); err != nil {
					t.Fatal(err)
				}
				root = filepath.Join(parent, "src")
				if err := os.Mkdir(root, 0o755); err != nil {
					t.Fatal(err)
				}
				var err error
				wantMessage, err = service.CanonicalWorkspaceRoot(parent)
				if err != nil {
					t.Fatal(err)
				}
			}
			m, err := newModel(root, contracts.Actor("human:owner"))
			if nested {
				if err != nil {
					t.Fatalf("subdirectory should open the parent board: %v", err)
				}
				m.close()
				if m.root != wantMessage && !strings.HasPrefix(m.root, wantMessage) {
					t.Fatalf("TUI root = %s, want parent %s", m.root, wantMessage)
				}
				entries, readErr := os.ReadDir(root)
				if readErr != nil || len(entries) != 0 {
					t.Fatalf("opening the parent changed the subdirectory: %v, %v", entries, readErr)
				}
				return
			}
			if err == nil {
				m.close()
				t.Fatal("invalid root opened a TUI model")
			}
			if apperr.CodeOf(err) != apperr.CodeInvalidInput || !strings.Contains(err.Error(), wantMessage) {
				t.Fatalf("expected invalid_input with %q, got %v", wantMessage, err)
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 0 {
				t.Fatalf("refused TUI open changed the directory: %v, %v", entries, err)
			}
		})
	}
}
