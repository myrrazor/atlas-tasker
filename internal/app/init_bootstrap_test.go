package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/uninstall"
)

func TestEnsureDefaultProjectAndRegisterFillsHollowWorkspace(t *testing.T) {
	a := testApp(t)
	root := filepath.Join(a.Home(), "hollow")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ScaffoldWorkspace(root, ScaffoldOptions{}); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, ".tracker", "config.toml")
	beforeCfg, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	beforeStat, err := os.Stat(config)
	if err != nil {
		t.Fatal(err)
	}
	projects, err := os.ReadDir(filepath.Join(root, "projects"))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 0 {
		t.Fatalf("scaffold should not create projects, got %v", projects)
	}
	key, rec, err := a.EnsureDefaultProjectAndRegister(context.Background(), root)
	if err != nil || key == "" || rec.WorkspaceID == "" {
		t.Fatalf("repair hollow: key=%q rec=%#v err=%v", key, rec, err)
	}
	listed, err := a.ListWorkspaces(context.Background(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range listed {
		if row.WorkspaceID == rec.WorkspaceID && row.Path == rec.Path {
			found = true
		}
	}
	if !found {
		t.Fatalf("workspace not registered: %#v", listed)
	}
	afterCfg, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	afterStat, err := os.Stat(config)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterCfg) != string(beforeCfg) || !afterStat.ModTime().Equal(beforeStat.ModTime()) {
		t.Fatal("repair rewrote config")
	}
	againKey, _, err := a.EnsureDefaultProjectAndRegister(context.Background(), root)
	if err != nil || againKey != "" {
		t.Fatalf("second repair must not recreate project: key=%q err=%v", againKey, err)
	}
}

func TestInitReportsReceiptWriteFailureWithoutFailing(t *testing.T) {
	orig := writeInstallReceipt
	t.Cleanup(func() { writeInstallReceipt = orig })
	writeInstallReceipt = func(string, time.Time) (uninstall.EnsureReceiptResult, error) {
		return uninstall.EnsureReceiptResult{}, apperr.New(apperr.CodeConflict, "install receipt is not valid JSON")
	}
	a := testApp(t)
	root := filepath.Join(a.Home(), "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := a.Init(context.Background(), InitOptions{
		Root:            root,
		Register:        true,
		DefaultProject:  true,
		SkipHomeService: true,
	})
	if err != nil {
		t.Fatalf("init should succeed: %v", err)
	}
	found := false
	for _, step := range result.Steps {
		if step.Name == "install_receipt" && step.Status == InitStepUnverified {
			found = true
			if !strings.Contains(step.Detail, "install receipt not written") || !strings.Contains(step.Detail, "uninstall will refuse") {
				t.Fatalf("receipt warning detail: %q", step.Detail)
			}
		}
	}
	if !found {
		t.Fatalf("missing install_receipt unverified step: %#v", result.Steps)
	}
}

func TestMCPBootstrapExplicitlySkipsHomeStartup(t *testing.T) {
	a := testApp(t)
	spawner := &RecordingSpawner{}
	a.opts.Process = spawner
	a.opts.Probe = LatchProber{}
	result, err := a.Init(context.Background(), InitOptions{
		Root:            filepath.Join(a.Home(), "mcp-only"),
		DefaultProject:  true,
		Register:        true,
		SkipHomeService: true,
	})
	if err != nil || spawner.Calls != 0 {
		t.Fatalf("MCP bootstrap launched Home: calls=%d err=%v", spawner.Calls, err)
	}
	if !result.Registered || result.DefaultProject == "" {
		t.Fatalf("MCP bootstrap incomplete: %+v", result)
	}
}

func TestHollowBootstrapPreservesMachineOptOuts(t *testing.T) {
	a := testApp(t)
	root := filepath.Join(a.Home(), "hollow")
	if _, err := ScaffoldWorkspace(root, ScaffoldOptions{}); err != nil {
		t.Fatal(err)
	}
	disabled := false
	if _, err := a.UpdateSettings(context.Background(), MachineSettingsPatch{
		DefaultProject: &disabled,
		AutoRegister:   &disabled,
	}); err != nil {
		t.Fatal(err)
	}
	key, rec, err := a.EnsureDefaultProjectAndRegister(context.Background(), root)
	if err != nil || key != "" || rec.WorkspaceID != "" {
		t.Fatalf("bootstrap ignored opt-outs: key=%q record=%#v err=%v", key, rec, err)
	}
	projects, err := os.ReadDir(filepath.Join(root, "projects"))
	if err != nil || len(projects) != 0 {
		t.Fatalf("default project was created: %v %v", projects, err)
	}
	listed, err := a.ListWorkspaces(context.Background(), ListOptions{})
	if err != nil || len(listed) != 0 {
		t.Fatalf("workspace was registered: %v %v", listed, err)
	}
}
