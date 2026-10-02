package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultProjectKeyKeepsShortBasenamesAndFirstWordForLongNames(t *testing.T) {
	cases := map[string]string{
		"widgets":                 "WIDGETS",
		"app":                     "APP",
		"12":                      "P12",
		"***":                     "MAIN",
		"atlas-tasker-monorepo":   "ATLAS",
		"atlas_tasker_monorepo":   "ATLAS",
		"a-very-long-hyphen-name": "VERY",
		"supercalifragilistic":    "MAIN",
	}
	for in, want := range cases {
		if got := DefaultProjectKey(in); got != want {
			t.Fatalf("DefaultProjectKey(%q)=%q want %q", in, got, want)
		}
	}
}

func TestRefreshManagedIgnoresKeepsUserLinesInsideBlock(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".gitignore")
	body := ManagedGitignoreBegin + "\n/.tracker/runtime/\nsecrets.env\n" + ManagedGitignoreEnd + "\n\n# mine\nlocal.out\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RefreshManagedIgnores(root); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if !strings.Contains(text, "local.out") {
		t.Fatalf("line outside the block was dropped:\n%s", text)
	}
	begin := strings.Index(text, ManagedGitignoreBegin)
	end := strings.Index(text, ManagedGitignoreEnd)
	if begin < 0 || end < begin {
		t.Fatalf("managed block missing:\n%s", text)
	}
	if strings.Contains(text[begin:end], "secrets.env") {
		t.Fatalf("user line stayed inside the managed block:\n%s", text)
	}
	if !strings.Contains(text[end:], "secrets.env") {
		t.Fatalf("user line was deleted:\n%s", text)
	}
	if err := RefreshManagedIgnores(root); err != nil {
		t.Fatal(err)
	}
	again, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != text {
		t.Fatalf("second refresh changed preserved lines:\n%s", again)
	}
}

func TestRefreshManagedIgnoresCRLFRewritesSessionPattern(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".gitignore")
	body := ManagedGitignoreBegin + "\n/.tracker/runtime/\nsecrets.env\n" + ManagedGitignoreEnd + "\n\n# mine\nlocal.out\n"
	body = strings.ReplaceAll(body, "\n", "\r\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RefreshManagedIgnores(root); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if strings.Contains(text, "\r") {
		t.Fatalf("carriage return left on ignore patterns:\n%s", text)
	}
	if !strings.Contains(text, "/.tracker/web-session.json\n") {
		t.Fatalf("session file is not ignored:\n%s", text)
	}
	end := strings.Index(text, ManagedGitignoreEnd)
	if end < 0 || strings.Contains(text[:end], "secrets.env") {
		t.Fatalf("user line stayed inside the managed block:\n%s", text)
	}
	if !strings.Contains(text[end:], "secrets.env\n") || !strings.Contains(text[end:], "local.out\n") {
		t.Fatalf("user lines were dropped:\n%s", text)
	}
	if err := RefreshManagedIgnores(root); err != nil {
		t.Fatal(err)
	}
	again, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != text {
		t.Fatalf("second refresh was not idempotent:\n%s", again)
	}
}

func TestRefreshManagedIgnoresProtectsWebStateWithoutManagedBlock(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required to verify ignore behavior")
	}
	for _, existing := range []string{"", "# User rules\nnotes.txt\n"} {
		name := "missing"
		if existing != "" {
			name = "unmanaged"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if out, err := exec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
				t.Fatalf("git init: %v\n%s", err, out)
			}
			path := filepath.Join(root, ".gitignore")
			if existing != "" {
				if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := RefreshManagedIgnores(root); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"web-session.json", "web-session.json.tmp", "web-create-submits.json", "web-create-submits.json.tmp", "web-create-submits.lock"} {
				cmd := exec.Command("git", "-C", root, "-c", "core.excludesFile="+os.DevNull, "check-ignore", "--no-index", "-q", ".tracker/"+name)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("local web state %s is not ignored: %v\n%s", name, err, out)
				}
			}
			raw, err := os.ReadFile(path)
			if existing == "" {
				if !os.IsNotExist(err) {
					t.Fatalf("created a root ignore file: %v", err)
				}
			} else if err != nil || string(raw) != existing {
				t.Fatalf("rewrote unrelated ignore rules: %q, %v", raw, err)
			}
			localPath := filepath.Join(root, ".tracker", ".gitignore")
			before, err := os.ReadFile(localPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := RefreshManagedIgnores(root); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(localPath)
			if err != nil || string(before) != string(after) {
				t.Fatalf("second refresh changed local ignore rules: %q, %v", after, err)
			}
		})
	}
}
