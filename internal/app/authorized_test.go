package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveAuthorizedDirCreatesUnderBoardsRoot(t *testing.T) {
	a := testApp(t)
	got, err := a.ResolveAuthorizedDir(BoardsRootRef, "widgets", true)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(a.BoardsRoot(), "widgets")
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	info, err := os.Lstat(got)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("created dir: err=%v info=%v", err, info)
	}
}

func TestResolveAuthorizedDirRejectsEscapeAndAbsolute(t *testing.T) {
	a := testApp(t)
	if _, err := a.ResolveAuthorizedDir(BoardsRootRef, "../outside", true); err == nil {
		t.Fatal("expected parent escape to fail")
	}
	if _, err := a.ResolveAuthorizedDir(BoardsRootRef, "/tmp/nope", true); err == nil {
		t.Fatal("expected absolute folder to fail")
	}
	if _, err := a.ResolveAuthorizedDir("discovery:0", "app", true); err == nil {
		t.Fatal("expected unknown discovery root to fail")
	}
	if _, err := a.ResolveAuthorizedDir(BoardsRootRef, "", true); err == nil {
		t.Fatal("boards root itself must not be a workspace")
	}
}

func TestResolveAuthorizedDirHonorsDiscoveryRoot(t *testing.T) {
	a := testApp(t)
	root := filepath.Join(a.Home(), "projects")
	if err := os.MkdirAll(filepath.Join(root, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := a.UpdateSettings(context.Background(), MachineSettingsPatch{
		Discovery: &DiscoverySettings{Enabled: true, Roots: []string{root}, MaxDepth: 4},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := a.ResolveAuthorizedDir("discovery:0", "alpha", false)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(root, "alpha") {
		t.Fatalf("got %s", got)
	}
	if _, err := a.ResolveAuthorizedDir("discovery:0", "missing", false); err == nil {
		t.Fatal("missing dir without create must fail")
	}
}

func TestResolveAuthorizedDirRefusesSymlink(t *testing.T) {
	a := testApp(t)
	realDir := filepath.Join(a.Home(), "real")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(a.BoardsRoot(), "linked")
	if err := os.MkdirAll(a.BoardsRoot(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realDir, link); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ResolveAuthorizedDir(BoardsRootRef, "linked", false); err == nil {
		t.Fatal("symlink child must fail")
	}
}

func TestListAuthorizedChildrenSkipsHiddenAndCaps(t *testing.T) {
	a := testApp(t)
	root := a.BoardsRoot()
	if err := os.MkdirAll(filepath.Join(root, "visible"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".secret"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	children, err := a.ListAuthorizedChildren(BoardsRootRef, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 1 || children[0].Name != "visible" {
		t.Fatalf("children=%#v", children)
	}
}

func TestEnsureInitProjectUsesExplicitKey(t *testing.T) {
	a := testApp(t)
	root := filepath.Join(a.Home(), "named")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := a.Init(context.Background(), InitOptions{
		Root:           root,
		Register:       true,
		DefaultProject: true,
		ProjectKey:     "OPS",
		ProjectName:    "Operations",
	})
	if err != nil && !IsPartial(err) {
		t.Fatal(err)
	}
	if result.DefaultProject != "OPS" {
		t.Fatalf("project=%q", result.DefaultProject)
	}
}

func TestListAuthorizedChildrenRefusesIntermediateSymlink(t *testing.T) {
	a := testApp(t)
	root := a.BoardsRoot()
	real := filepath.Join(a.Home(), "elsewhere")
	child := filepath.Join(real, "real-child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ListAuthorizedChildren(BoardsRootRef, "link/real-child"); err == nil {
		t.Fatal("listing through an intermediate symlink must fail")
	}
	if _, err := a.ResolveAuthorizedDir(BoardsRootRef, "link/real-child", false); err == nil {
		t.Fatal("resolve through an intermediate symlink must fail")
	}
}

func TestPreviewDirectoryGrantDoesNotInit(t *testing.T) {
	a := testApp(t)
	dir := filepath.Join(a.Home(), "chosen")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	grant, err := a.PreviewDirectoryGrant(context.Background(), dir, PathGrantInit)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := CanonicalExistingDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if grant.Source != PathGrantSourceHome || grant.Purpose != PathGrantInit || grant.Path != canonical {
		t.Fatalf("grant=%#v canonical=%s", grant, canonical)
	}
	if _, err := os.Stat(filepath.Join(dir, ".tracker")); !os.IsNotExist(err) {
		t.Fatal("preview must not scaffold a workspace")
	}
	pending, err := a.LookupPendingGrant(grant.ID)
	if err != nil || pending.ID != grant.ID {
		t.Fatalf("pending=%#v err=%v", pending, err)
	}
}

func TestPreviewDirectoryGrantRefusesMissingAndNested(t *testing.T) {
	a := testApp(t)
	if _, err := a.PreviewDirectoryGrant(context.Background(), "relative", PathGrantInit); err == nil {
		t.Fatal("relative path must fail")
	}
	if _, err := a.PreviewDirectoryGrant(context.Background(), filepath.Join(a.Home(), "missing"), PathGrantInit); err == nil {
		t.Fatal("missing dir must fail")
	}
	root := filepath.Join(a.Home(), "outer")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Init(context.Background(), InitOptions{Root: root, Register: true}); err != nil && !IsPartial(err) {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "inner")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := a.PreviewDirectoryGrant(context.Background(), nested, PathGrantInit); err == nil {
		t.Fatal("nested workspace child must fail")
	}
}

func TestCleanAuthorizedRelRejectsColon(t *testing.T) {
	if _, err := cleanAuthorizedRel("C:/windows"); err == nil {
		t.Fatal("expected colon path to fail")
	}
	got, err := cleanAuthorizedRel("clients/acme")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "clients") {
		t.Fatalf("rel=%q", got)
	}
}

func TestHomeDirectorySelectionCannotMintRepairGrant(t *testing.T) {
	a := testApp(t)
	dir := filepath.Join(a.Home(), "selected")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := a.PreviewDirectoryGrant(context.Background(), dir, PathGrantRepair); err == nil {
		t.Fatal("Home directory selection must not authorize repair through a different confirmation route")
	}
	if grants := a.ListPendingGrants(); len(grants) != 0 {
		t.Fatalf("invalid purpose created grants: %v", grants)
	}
}

func TestHomeDirectoryGrantRejectsAncestorSymlinkSwap(t *testing.T) {
	a := testApp(t)
	parent := filepath.Join(a.Home(), "original")
	dir := filepath.Join(parent, "selected")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	grant, err := a.PreviewDirectoryGrant(context.Background(), dir, PathGrantInit)
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(a.Home(), "moved")
	if err := os.Rename(parent, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(moved, parent); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ConsumeGrantFor(context.Background(), grant.ID, PathGrantInit); err == nil {
		t.Fatal("grant accepted a changed ancestor path even though the final directory inode stayed the same")
	}
}
