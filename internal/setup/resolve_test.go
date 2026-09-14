package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func writeWorkspace(t *testing.T, root, id string) {
	t.Helper()
	if err := os.MkdirAll(storage.TrackerDir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	meta, _ := json.Marshal(map[string]any{"workspace_id": id, "created_at": "2026-09-11T00:00:00Z"})
	if err := os.WriteFile(storage.WorkspaceMetadataFile(root), append(meta, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveWorkspaceFromCWDSecurity(t *testing.T) {
	state := t.TempDir()
	root := t.TempDir()
	resolved, err := filepath.EvalSymlinks(root)
	if err == nil {
		root = resolved
	}
	writeWorkspace(t, root, "ws-1")

	got, err := ResolveWorkspaceFromCWD(root, "ws-1", state)
	if err != nil {
		t.Fatal(err)
	}
	if got.WorkspaceID != "ws-1" || got.Root != root {
		t.Fatalf("got %#v", got)
	}

	if _, err := ResolveWorkspaceFromCWD(t.TempDir(), "ws-1", state); err == nil {
		t.Fatal("wrong cwd must fail")
	}
	if _, err := ResolveWorkspaceFromCWD(root, "ws-other", state); err == nil {
		t.Fatal("wrong workspace id must fail")
	}

	nested := filepath.Join(root, "child")
	writeWorkspace(t, nested, "ws-nested")
	if _, err := ResolveWorkspaceFromCWD(nested, "ws-nested", state); err == nil {
		t.Fatal("nested workspace must fail")
	}
}

func TestResolveDetectsMoveCopyAndReplace(t *testing.T) {
	state := t.TempDir()
	root := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	writeWorkspace(t, root, "ws-move")
	dev, ino, _ := fileDevIno(root)
	reg := workspaceRegistry{Format: registryFormat, Workspaces: map[string]RegistryEntry{}}
	reg.put(RegistryEntry{WorkspaceID: "ws-move", CanonicalPath: root, Dev: dev, Ino: ino, RegisteredAt: time.Now(), VerifiedAt: time.Now()})
	if err := saveRegistry(state, reg); err != nil {
		t.Fatal(err)
	}

	moved := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(moved); err == nil {
		moved = resolved
	}
	if err := os.Rename(root, filepath.Join(moved, "ws")); err != nil {
		// fallback copy when rename crosses devices
		writeWorkspace(t, filepath.Join(moved, "ws"), "ws-move")
		_ = os.RemoveAll(root)
	}
	newRoot := filepath.Join(moved, "ws")
	if _, err := ResolveWorkspaceFromCWD(newRoot, "ws-move", state); err == nil || apperr.CodeOf(err) != apperr.CodeRepairNeeded {
		t.Fatalf("moved workspace should require repair, got %v", err)
	}

	original := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(original); err == nil {
		original = resolved
	}
	writeWorkspace(t, original, "ws-copy")
	dev, ino, _ = fileDevIno(original)
	reg = workspaceRegistry{Format: registryFormat, Workspaces: map[string]RegistryEntry{}}
	reg.put(RegistryEntry{WorkspaceID: "ws-copy", CanonicalPath: original, Dev: dev, Ino: ino, RegisteredAt: time.Now(), VerifiedAt: time.Now()})
	if err := saveRegistry(state, reg); err != nil {
		t.Fatal(err)
	}
	copyRoot := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(copyRoot); err == nil {
		copyRoot = resolved
	}
	writeWorkspace(t, copyRoot, "ws-copy")
	_, err := ResolveWorkspaceFromCWD(copyRoot, "ws-copy", state)
	if err == nil || apperr.CodeOf(err) != apperr.CodeInvalidInput {
		t.Fatalf("copied workspace must fail until reconciled, got %v", err)
	}

	replaced := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(replaced); err == nil {
		replaced = resolved
	}
	writeWorkspace(t, replaced, "ws-replace")
	reg = workspaceRegistry{Format: registryFormat, Workspaces: map[string]RegistryEntry{}}
	reg.put(RegistryEntry{WorkspaceID: "ws-replace", CanonicalPath: replaced, Dev: 1, Ino: 1, RegisteredAt: time.Now(), VerifiedAt: time.Now()})
	if err := saveRegistry(state, reg); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveWorkspaceFromCWD(replaced, "ws-replace", state); err == nil {
		t.Fatal("replaced inode must fail")
	}
}

func TestResolveRefusesSymlinkedTracker(t *testing.T) {
	root := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	real := t.TempDir()
	if err := os.Symlink(real, filepath.Join(root, storage.TrackerDirName)); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveWorkspaceFromCWD(root, "ws-1", ""); err == nil {
		t.Fatal("symlinked .tracker must be refused")
	}
}

func TestVerifyExpectedWorkspaceID(t *testing.T) {
	root := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	writeWorkspace(t, root, "ws-1")
	if err := VerifyExpectedWorkspaceID(root, "ws-1"); err != nil {
		t.Fatal(err)
	}
	if err := VerifyExpectedWorkspaceID(root, "nope"); err == nil {
		t.Fatal("expected mismatch")
	}
	if err := VerifyExpectedWorkspaceID(root, ""); err != nil {
		t.Fatal(err)
	}
}
