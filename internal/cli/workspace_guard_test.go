package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The wrong-CWD footgun: running a read command in a directory that was never
// initialized used to print an empty board AND scaffold a stray .tracker dir,
// which doctor would then happily bless. These tests pin the fix.

func TestBoardOutsideWorkspaceFailsWithoutCreatingTrackerDir(t *testing.T) {
	withTempWorkspace(t)

	out, err := runCLI(t, "board")
	if err == nil {
		t.Fatalf("expected board to refuse an uninitialized directory, got output:\n%s", out)
	}
	if !strings.Contains(err.Error(), "not an Atlas workspace") {
		t.Fatalf("error should say the directory is not a workspace, got: %v", err)
	}
	if !strings.Contains(err.Error(), "tracker init") {
		t.Fatalf("error should point at 'tracker init', got: %v", err)
	}
	if _, statErr := os.Stat(".tracker"); !os.IsNotExist(statErr) {
		t.Fatalf("a refused read must not scaffold .tracker (stat err: %v)", statErr)
	}
}

func TestConfigGetOutsideWorkspaceFailsWithoutCreatingTrackerDir(t *testing.T) {
	withTempWorkspace(t)
	var stdout, stderr bytes.Buffer
	if exit := Execute([]string{"config", "get", "workflow.completion_mode", "--json"}, &stdout, &stderr); exit != 2 {
		t.Fatalf("config get outside workspace exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(".tracker"); !os.IsNotExist(err) {
		t.Fatalf("config get created state: %v", err)
	}
}

func TestDoctorOutsideWorkspaceFailsInsteadOfReportingOK(t *testing.T) {
	withTempWorkspace(t)

	out, err := runCLI(t, "doctor")
	if err == nil {
		t.Fatalf("expected doctor to refuse an uninitialized directory, got output:\n%s", out)
	}
	if !strings.Contains(err.Error(), "not an Atlas workspace") {
		t.Fatalf("error should say the directory is not a workspace, got: %v", err)
	}
}

func TestSubdirectoryOfWorkspacePointsAtTheRealRoot(t *testing.T) {
	withTempWorkspace(t)
	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	sub := filepath.Join(root, "src", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir subdir failed: %v", err)
	}
	if err := os.Chdir(sub); err != nil {
		t.Fatalf("chdir subdir failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(root) })

	out, cliErr := runCLI(t, "board")
	if cliErr == nil {
		t.Fatalf("expected board in a workspace subdirectory to fail, got output:\n%s", out)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		resolvedRoot = root
	}
	if !strings.Contains(cliErr.Error(), resolvedRoot) {
		t.Fatalf("error should name the workspace root %s so the user knows where to cd, got: %v", resolvedRoot, cliErr)
	}
	if _, statErr := os.Stat(filepath.Join(sub, ".tracker")); !os.IsNotExist(statErr) {
		t.Fatalf("a refused read must not scaffold .tracker in the subdirectory (stat err: %v)", statErr)
	}
}

func TestInitStillBootstrapsAFreshDirectory(t *testing.T) {
	withTempWorkspace(t)

	out, err := runCLI(t, "init")
	if err != nil {
		t.Fatalf("init must keep working in a fresh directory: %v\noutput=%s", err, out)
	}
	if _, err := runCLI(t, "board"); err != nil {
		t.Fatalf("board right after init should work: %v", err)
	}
}

func TestIntegrationsInstallStillBootstrapsAFreshDirectory(t *testing.T) {
	withTempWorkspace(t)
	if out, err := runCLI(t, "integrations", "install", "generic", "--json"); err != nil {
		t.Fatalf("integration install must retain explicit bootstrap: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(".tracker", "config.toml")); err != nil {
		t.Fatalf("integration install did not initialize the workspace: %v", err)
	}
	if _, err := runCLI(t, "board"); err != nil {
		t.Fatalf("bootstrapped integration workspace must support reads: %v", err)
	}
}

func TestInvalidStatusErrorsListTheLegalValues(t *testing.T) {
	withTempWorkspace(t)
	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if _, err := runCLI(t, "project", "create", "APP", "App"); err != nil {
		t.Fatalf("project create failed: %v", err)
	}
	if _, err := runCLI(t, "ticket", "create", "--project", "APP", "--title", "One", "--type", "task", "--actor", "human:owner"); err != nil {
		t.Fatalf("ticket create failed: %v", err)
	}

	wantVocabulary := "backlog, ready, in_progress, in_review, blocked, done, canceled"
	cases := [][]string{
		{"ticket", "move", "APP-1", "shipped", "--actor", "human:owner"},
		{"ticket", "create", "--project", "APP", "--title", "Two", "--type", "task", "--status", "shipped", "--actor", "human:owner"},
		{"bulk", "move", "shipped", "--ticket", "APP-1", "--dry-run", "--actor", "human:owner"},
	}
	for _, args := range cases {
		_, err := runCLI(t, args...)
		if err == nil {
			t.Fatalf("expected %v to be rejected", args)
		}
		if !strings.Contains(err.Error(), "invalid status") {
			t.Fatalf("%v: expected an invalid-status error, got: %v", args, err)
		}
		if !strings.Contains(err.Error(), wantVocabulary) {
			t.Fatalf("%v: error should enumerate the legal statuses (%s), got: %v", args, wantVocabulary, err)
		}
	}
}

func TestRootHelpTeachesTheHappyPath(t *testing.T) {
	root := NewRootCommand()
	if strings.TrimSpace(root.Long) == "" {
		t.Fatal("root command needs Long help text — Short alone leaves a new user lost in ~70 commands")
	}
	for _, expected := range []string{"tracker init", "ticket create", "tracker board"} {
		if !strings.Contains(root.Example, expected) {
			t.Fatalf("root help example should walk init → create → board; missing %q in:\n%s", expected, root.Example)
		}
	}
}
