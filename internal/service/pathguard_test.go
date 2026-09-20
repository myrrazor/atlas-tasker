package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRejectUnsafeRelPath(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		rel     string
		wantErr bool
	}{
		{rel: "projects/APP/tickets/1.md", wantErr: false},
		{rel: ".tracker/runtime/x", wantErr: false},
		{rel: "../../etc/passwd", wantErr: true},
		{rel: "/etc/passwd", wantErr: true},
		{rel: "", wantErr: true},
		{rel: "..", wantErr: true},
	}
	for _, tc := range cases {
		err := rejectUnsafeRelPath(root, tc.rel)
		if tc.wantErr && err == nil {
			t.Fatalf("%q: expected error", tc.rel)
		}
		if !tc.wantErr && err != nil {
			t.Fatalf("%q: unexpected error: %v", tc.rel, err)
		}
	}
}

func TestRejectSymlinkComponents(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if err := rejectSymlinkComponents(root, filepath.Join(link, "secret")); err == nil {
		t.Fatal("expected symlink component rejection")
	}
	safe := filepath.Join(root, "projects", "APP", "ticket.md")
	if err := os.MkdirAll(filepath.Dir(safe), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(safe, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := rejectSymlinkComponents(root, safe); err != nil {
		t.Fatalf("safe path rejected: %v", err)
	}
	insideLink := filepath.Join(root, "inside-link")
	if err := os.Symlink(filepath.Join(root, "projects"), insideLink); err != nil {
		t.Fatal(err)
	}
	if err := rejectSymlinkComponents(root, filepath.Join(insideLink, "APP", "ticket.md")); err == nil {
		t.Fatal("expected in-workspace symlink component rejection")
	}
}

func TestResolveContainedPath(t *testing.T) {
	root := t.TempDir()
	got, err := resolveContainedPath(root, "a/b/c.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "a", "b", "c.txt")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if _, err := resolveContainedPath(root, "../outside"); err == nil {
		t.Fatal("expected escape rejection")
	}
}

func TestResolveWorkspaceInputPath(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "evidence.txt")
	if err := os.WriteFile(inside, []byte("proof"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := ResolveWorkspaceInputPath(root, inside); err != nil || got != inside {
		t.Fatalf("resolve absolute workspace input: got %q err=%v", got, err)
	}
	if _, err := ResolveWorkspaceInputPath(root, filepath.Join(t.TempDir(), "outside.txt")); err == nil {
		t.Fatal("expected outside absolute input to be rejected")
	}
}

func TestCanonicalComparablePathResolvesMissingNestedSuffix(t *testing.T) {
	root := t.TempDir()
	got := canonicalComparablePath(filepath.Join(root, "missing", "nested", "file.txt"))
	wantRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(wantRoot, "missing", "nested", "file.txt")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
