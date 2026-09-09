package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestInitHardensLocalPathsAndSeedsGitignore(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix mode bits")
	}
	withTempWorkspace(t)
	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	for _, rel := range []string{
		storage.TrackerDirName,
		filepath.Join(storage.TrackerDirName, "runtime"),
		filepath.Join(storage.TrackerDirName, "archives"),
		filepath.Join(storage.TrackerDirName, "exports"),
		filepath.Join(storage.TrackerDirName, "imports"),
		filepath.Join(storage.TrackerDirName, "evidence"),
	} {
		info, err := os.Stat(rel)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("%s mode=%o want private", rel, info.Mode().Perm())
		}
	}
	cfgInfo, err := os.Stat(config.Path("."))
	if err != nil {
		t.Fatal(err)
	}
	if cfgInfo.Mode().Perm()&0o077 != 0 {
		t.Fatalf("config mode=%o want private", cfgInfo.Mode().Perm())
	}
	raw, err := os.ReadFile(".gitignore")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	wantRuntime := "/" + storage.TrackerDirName + "/runtime/"
	if !strings.Contains(body, workspaceGitignoreBegin) || !strings.Contains(body, wantRuntime) {
		t.Fatalf("gitignore missing managed local-ignore block:\n%s", body)
	}
	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("re-init: %v", err)
	}
	raw2, err := os.ReadFile(".gitignore")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(raw2), workspaceGitignoreBegin) != 1 {
		t.Fatalf("managed gitignore block duplicated:\n%s", raw2)
	}
}
