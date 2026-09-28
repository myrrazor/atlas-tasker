package app

import (
	"os"
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
