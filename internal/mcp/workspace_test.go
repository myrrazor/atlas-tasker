package mcp

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	"github.com/myrrazor/atlas-tasker/internal/testutil"
)

func seedWorkspace(t *testing.T) (string, time.Time) {
	t.Helper()
	root := t.TempDir()
	now := time.Date(2026, 9, 1, 11, 0, 0, 0, time.UTC)
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	workspace, err := OpenWorkspace(root, nil, func() time.Time { return now })
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	ctx := context.Background()
	if err := workspace.Actions.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	for _, title := range []string{"First over MCP", "Second over MCP"} {
		if _, err := workspace.Actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
			Project: "APP", Title: title, Type: contracts.TicketTypeTask,
			Status: contracts.StatusReady, Priority: contracts.PriorityMedium,
			CreatedAt: now, UpdatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion,
		}, contracts.Actor("human:owner"), "seed"); err != nil {
			t.Fatalf("create ticket: %v", err)
		}
	}
	workspace.Close()
	return root, now
}

func TestOpenWorkspaceRebuildsAStaleIndex(t *testing.T) {
	root, now := seedWorkspace(t)
	path := filepath.Join(storage.TrackerDir(root), "index.sqlite")
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Remove(candidate); err != nil && !os.IsNotExist(err) {
			t.Fatalf("remove %s: %v", candidate, err)
		}
	}

	var stderr bytes.Buffer
	workspace, err := OpenWorkspace(root, &stderr, func() time.Time { return now })
	if err != nil {
		t.Fatalf("reopen workspace: %v", err)
	}
	defer workspace.Close()
	board, err := workspace.Queries.Board(context.Background(), contracts.BoardQueryOptions{Project: "APP"})
	if err != nil {
		t.Fatalf("board: %v", err)
	}
	if len(board.Board.Columns[contracts.StatusReady]) != 2 {
		t.Fatalf("expected the rebuilt index to carry both tickets, got %#v", board.Board.Columns)
	}
	if !strings.Contains(stderr.String(), "rebuilt it from markdown and events") {
		t.Fatalf("expected the rebuild notice on the server's stderr, got %q", stderr.String())
	}
}

func TestOpenWorkspaceMapsCorruptIndexToRepairNeeded(t *testing.T) {
	root, now := seedWorkspace(t)
	if err := testutil.CorruptProjection(root); err != nil {
		t.Fatalf("corrupt projection: %v", err)
	}

	_, err := OpenWorkspace(root, nil, func() time.Time { return now })
	if err == nil {
		t.Fatal("expected a corrupt index to refuse to open")
	}
	if apperr.CodeOf(err) != apperr.CodeRepairNeeded {
		t.Fatalf("expected repair_needed, got %s: %v", apperr.CodeOf(err), err)
	}
	if !strings.Contains(err.Error(), "doctor --repair") {
		t.Fatalf("expected repair guidance in the error, got %q", err.Error())
	}
}

func TestOpenWorkspaceRejectsInvalidRootsWithoutCreatingState(t *testing.T) {
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
			workspace, err := OpenWorkspace(root, nil, nil)
			if workspace != nil {
				workspace.Close()
				t.Fatal("invalid root opened a workspace")
			}
			if apperr.CodeOf(err) != apperr.CodeInvalidInput || !strings.Contains(err.Error(), wantMessage) {
				t.Fatalf("expected invalid_input with %q, got %v", wantMessage, err)
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 0 {
				t.Fatalf("refused MCP open changed the directory: %v, %v", entries, err)
			}
		})
	}
}
