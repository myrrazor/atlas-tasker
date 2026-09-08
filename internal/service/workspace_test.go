package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestInitializedWorkspaceRootAcceptsCanonicalAndSymlinkedRoots(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(storage.TrackerDir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "workspace-link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{root, link} {
		got, err := InitializedWorkspaceRoot(path)
		if err != nil || got != want {
			t.Fatalf("resolve %s: got %q, err %v; want %q", path, got, err, want)
		}
	}
	if _, err := os.Stat(filepath.Join(storage.TrackerDir(root), "index.sqlite")); !os.IsNotExist(err) {
		t.Fatalf("root validation must not open the index: %v", err)
	}
}

func TestInitializedWorkspaceRootRejectsUninitializedAndNestedRoots(t *testing.T) {
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
				root = filepath.Join(parent, "src", "deep")
				if err := os.MkdirAll(root, 0o755); err != nil {
					t.Fatal(err)
				}
				var err error
				wantMessage, err = filepath.EvalSymlinks(parent)
				if err != nil {
					t.Fatal(err)
				}
			}
			_, err := InitializedWorkspaceRoot(root)
			if apperr.CodeOf(err) != apperr.CodeInvalidInput || !strings.Contains(err.Error(), wantMessage) {
				t.Fatalf("expected invalid_input with %q, got %v", wantMessage, err)
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("refused root gained files: %v", entries)
			}
		})
	}
}

func TestInitializedWorkspaceRootRejectsFileMarkerAndInvalidDirectory(t *testing.T) {
	root := t.TempDir()
	marker := storage.TrackerDir(root)
	if err := os.WriteFile(marker, []byte("keep this file"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{root, marker} {
		_, err := InitializedWorkspaceRoot(path)
		if apperr.CodeOf(err) != apperr.CodeInvalidInput || !strings.Contains(err.Error(), "not a directory") {
			t.Fatalf("expected directory validation for %s, got %v", path, err)
		}
	}
	missing := filepath.Join(root, "missing")
	if _, err := InitializedWorkspaceRoot(missing); apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("expected not_found for missing root, got %v", err)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("missing root must not be created: %v", err)
	}
	got, err := os.ReadFile(marker)
	if err != nil || string(got) != "keep this file" {
		t.Fatalf("file marker changed: %q, %v", got, err)
	}
}
