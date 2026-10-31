package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/app"
)

// TestDoctorRepairIgnoresQuarantineForExistingWorkspaces covers shared, private,
// and unmanaged git modes for a workspace whose ignore rules match v1.18.1
// (no quarantine line). doctor --repair must ignore the backup it writes.
func TestDoctorRepairIgnoresQuarantineForExistingWorkspaces(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required to verify ignore behavior")
	}
	for _, mode := range []app.GitMode{app.GitModeShared, app.GitModePrivate, app.GitModeUnmanaged} {
		t.Run(string(mode), func(t *testing.T) {
			root := t.TempDir()
			gitCmd(t, root, "init", "-q", "-b", "main")
			gitCmd(t, root, "config", "user.email", "atlas@example.com")
			gitCmd(t, root, "config", "user.name", "Atlas")
			gitCmd(t, root, "config", "core.excludesFile", os.DevNull)

			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_STATE_HOME", filepath.Join(home, "xdg"))
			oldWD, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chdir(root); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chdir(oldWD) })

			if _, err := app.ScaffoldWorkspace(root, app.ScaffoldOptions{GitMode: mode}); err != nil {
				t.Fatal(err)
			}
			// A released v1.18.1 workspace has the managed block without a
			// quarantine line. Unmanaged init writes no block at all.
			stripQuarantineIgnore(t, filepath.Join(root, ".gitignore"))
			stripQuarantineIgnore(t, filepath.Join(root, ".git", "info", "exclude"))
			stripQuarantineIgnore(t, filepath.Join(root, ".tracker", ".gitignore"))
			if out, err := exec.Command("git", "-C", root, "check-ignore", "-q", ".tracker/quarantine/x").CombinedOutput(); err == nil {
				t.Fatalf("v1.18.1 workspace already ignores quarantine before repair:\n%s", out)
			}

			mustCLI(t, "project", "create", "APP", "App Project")
			mustCLI(t, "ticket", "create", "--project", "APP", "--title", "First", "--type", "task", "--actor", "human:owner")
			ticketRel := "projects/APP/tickets/APP-1.md"
			gitCmd(t, root, "add", "--", ticketRel)
			gitCmd(t, root, "commit", "-q", "-m", "add ticket")

			session := filepath.Join(root, ".tracker", "web-session.json")
			if err := os.WriteFile(session, []byte("{}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if mode != app.GitModeUnmanaged {
				gitCheckIgnore(t, root, ".tracker/web-session.json")
			}

			truncateTicket(t, "APP", "APP-1")
			exit, payload, out := execJSON(t, "doctor", "--repair", "--json")
			if exit != 0 {
				t.Fatalf("doctor --repair exited %d\n%s", exit, out)
			}
			backup := quarantineBackup(t, payload)
			if _, err := os.Stat(filepath.Join(root, backup)); err != nil {
				t.Fatalf("backup %s: %v", backup, err)
			}
			gitCheckIgnore(t, root, backup)
			gitCheckIgnore(t, root, ".tracker/web-session.json")
			status := gitOutputIn(t, root, "status", "--untracked-files=all")
			if strings.Contains(status, "quarantine") || strings.Contains(status, "web-session.json") {
				t.Fatalf("ignored files showed up in git status:\n%s", status)
			}
			gitCmd(t, root, "ls-files", "--error-unmatch", "--", ticketRel)
			if out, err := exec.Command("git", "-C", root, "check-ignore", "-q", "--", ticketRel).CombinedOutput(); err == nil {
				t.Fatalf("ticket file is ignored:\n%s", out)
			}

			exit, _, out = execJSON(t, "doctor", "--repair", "--json")
			if exit != 0 {
				t.Fatalf("second doctor --repair exited %d\n%s", exit, out)
			}
			assertQuarantineIgnoreOnce(t, root, mode)
		})
	}
}

func quarantineBackup(t *testing.T, payload map[string]any) string {
	t.Helper()
	actions, _ := payload["repair_actions"].([]any)
	for _, action := range actions {
		text, _ := action.(string)
		if !strings.Contains(text, "backup:") {
			continue
		}
		backup := strings.TrimSpace(text[strings.Index(text, "backup:")+len("backup:"):])
		backup = strings.TrimSuffix(strings.TrimSpace(backup), ")")
		if strings.Contains(backup, "quarantine") {
			return backup
		}
	}
	t.Fatalf("repair actions missing quarantine backup: %v", actions)
	return ""
}

func assertQuarantineIgnoreOnce(t *testing.T, root string, mode app.GitMode) {
	t.Helper()
	var paths []string
	switch mode {
	case app.GitModePrivate:
		paths = []string{filepath.Join(root, ".git", "info", "exclude")}
	case app.GitModeUnmanaged:
		paths = []string{filepath.Join(root, ".tracker", ".gitignore")}
	default:
		paths = []string{filepath.Join(root, ".gitignore")}
	}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(raw)
		count := strings.Count(text, "quarantine/")
		if count != 1 {
			t.Fatalf("%s has %d quarantine ignore lines, want 1:\n%s", path, count, text)
		}
		if mode != app.GitModeUnmanaged && !strings.Contains(text, "/.tracker/web-session.json\n") {
			t.Fatalf("%s dropped the web session ignore:\n%s", path, text)
		}
		if mode == app.GitModeUnmanaged && !strings.Contains(text, "/web-session.json\n") {
			t.Fatalf("%s dropped the web session ignore:\n%s", path, text)
		}
	}
}

func stripQuarantineIgnore(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.Contains(line, "quarantine") {
			continue
		}
		kept = append(kept, line)
	}
	if err := os.WriteFile(path, []byte(strings.Join(kept, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustCLI(t *testing.T, args ...string) {
	t.Helper()
	out, err := runCLI(t, args...)
	if err != nil {
		t.Fatalf("%v failed: %v\n%s", args, err, out)
	}
}

func gitCmd(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func gitOutputIn(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func gitCheckIgnore(t *testing.T, root, rel string) {
	t.Helper()
	cmd := exec.Command("git", "-C", root, "check-ignore", "-q", "--", rel)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git check-ignore %s: %v\n%s", rel, err, out)
	}
}
