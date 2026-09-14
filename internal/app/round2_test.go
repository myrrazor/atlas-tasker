package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/uninstall"
)

func TestInitStartsHomeWithInjectedSpawner(t *testing.T) {
	a := testApp(t)
	var running atomic.Bool
	spawner := &RecordingSpawner{OnStart: func() { running.Store(true) }}
	a.opts.Process = spawner
	a.opts.Probe = LatchProber{Instance: a.Settings().InstanceID, Running: running.Load}
	root := filepath.Join(a.Home(), "svc")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := a.Init(context.Background(), InitOptions{Root: root, Register: true, OpenHome: false})
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if spawner.Calls != 1 {
		t.Fatalf("expected one spawn, got %d", spawner.Calls)
	}
	if result.Service == nil || !result.Service.Running || result.Service.URL == "" {
		t.Fatalf("service: %+v", result.Service)
	}
	if strings.Contains(result.Service.URL, "claim") || strings.Contains(result.Service.URL, "?") {
		t.Fatalf("stable URL must not carry a secret: %s", result.Service.URL)
	}
	if result.Service.ClaimURL != "" && !strings.Contains(result.Service.ClaimURL, "#claim=") {
		t.Fatalf("claim must use a fragment: %s", result.Service.ClaimURL)
	}
	found := false
	for _, step := range result.Steps {
		if step.Name == "service" && step.Status == InitStepDone {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing service step: %+v", result.Steps)
	}
}

func TestInitRespectsServiceEnabledOptOut(t *testing.T) {
	a := testApp(t)
	spawner := &RecordingSpawner{}
	a.opts.Process = spawner
	enabled := false
	if _, err := a.UpdateSettings(context.Background(), MachineSettingsPatch{Service: &ServiceSettings{
		Bind: DefaultHomeBind, Port: DefaultHomePort, Enabled: enabled, AutoStart: true,
	}}); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(a.Home(), "off")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := a.Init(context.Background(), InitOptions{Root: root, Register: true, OpenHome: true})
	if err != nil {
		t.Fatal(err)
	}
	if spawner.Calls != 0 {
		t.Fatalf("disabled service still spawned: %d", spawner.Calls)
	}
	if result.Service == nil || result.Service.Running {
		t.Fatalf("expected idle service: %+v", result.Service)
	}
}

func TestHideDoesNotDisableBackupJobs(t *testing.T) {
	a := testApp(t)
	root := filepath.Join(a.Home(), "hidden")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := a.Init(context.Background(), InitOptions{Root: root, Register: true, Backup: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Repair(context.Background(), RepairOptions{WorkspaceID: result.WorkspaceID, Action: RepairHide}); err != nil {
		t.Fatal(err)
	}
	visible, err := a.ListWorkspaces(context.Background(), ListOptions{})
	if err != nil || len(visible) != 0 {
		t.Fatalf("hidden workspace leaked into UI list: %+v %v", visible, err)
	}
	all, err := a.ListWorkspaces(context.Background(), ListOptions{IncludeHidden: true})
	if err != nil || len(all) != 1 {
		t.Fatalf("include hidden: %+v %v", all, err)
	}
	if all[0].Visibility != VisibilityHidden || all[0].Health != HealthAvailable {
		t.Fatalf("hide must not change health: %+v", all[0])
	}
	a.tickWorkspaces(context.Background())
	repo := filepath.Join(a.StateDir(), "backups", result.WorkspaceID, "repo.git")
	if _, err := os.Stat(repo); err != nil {
		t.Fatalf("backup replica missing after hide: %v", err)
	}
	if _, err := a.Repair(context.Background(), RepairOptions{WorkspaceID: result.WorkspaceID, Action: RepairRemovePointer}); err != nil {
		t.Fatal(err)
	}
	left, _ := a.ListWorkspaces(context.Background(), ListOptions{IncludeHidden: true})
	if len(left) != 0 {
		t.Fatalf("pointer still present: %+v", left)
	}
	if _, err := os.Stat(repo); err != nil {
		t.Fatalf("remove_pointer deleted backup data: %v", err)
	}
}

func TestRepeatedInitRespectsCheckpointOptOut(t *testing.T) {
	a := testApp(t)
	root := filepath.Join(a.Home(), "optout")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	first, err := a.Init(context.Background(), InitOptions{Root: root, Register: true, Backup: true})
	if err != nil {
		t.Fatal(err)
	}
	auto := filepath.Join(a.StateDir(), "backups", first.WorkspaceID, "auto.json")
	if err := atomicJSON(auto, map[string]any{
		"format":      "atlas_backup_auto_v1",
		"enabled":     false,
		"disabled_at": a.now(),
	}); err != nil {
		t.Fatal(err)
	}
	again, err := a.Init(context.Background(), InitOptions{Root: root, Register: true, Backup: true})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(auto)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"enabled": false`) {
		t.Fatalf("opt-out was re-enabled:\n%s\ninit=%+v", raw, again.Backup)
	}
}

func TestUninstallManifestRequiresReceipt(t *testing.T) {
	a := testApp(t)
	if err := os.MkdirAll(filepath.Join(a.Home(), ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(a.Home(), "noreceipt")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Init(context.Background(), InitOptions{Root: root, Register: true, Agents: true, WriteClientCfg: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(uninstall.ManifestPath(a.StateDir())); !os.IsNotExist(err) {
		t.Fatal("development binary must not get an uninstall manifest")
	}
}

func TestUninstallManifestFromVerifiedReceipt(t *testing.T) {
	a := testApp(t)
	bin := a.opts.Executable
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho tracker\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	receipt, err := uninstall.NewScriptReceipt(bin, "v1.15.0-test", a.now())
	if err != nil {
		t.Fatal(err)
	}
	if err := uninstall.WriteReceipt(a.StateDir(), receipt); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(a.Home(), ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(a.Home(), "owned")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Init(context.Background(), InitOptions{Root: root, Register: true, Agents: true, WriteClientCfg: true}); err != nil {
		t.Fatal(err)
	}
	manifest, err := uninstall.LoadManifest(a.StateDir())
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ReceiptDigest != receipt.Digest {
		t.Fatalf("manifest digest %s want %s", manifest.ReceiptDigest, receipt.Digest)
	}
	found := false
	for _, action := range manifest.Actions {
		if action.Path == filepath.Join(a.Home(), ".cursor", "mcp.json") && action.EntryKey == GlobalMCPServerName {
			found = true
		}
	}
	if !found {
		t.Fatalf("cursor JSON action missing: %+v", manifest.Actions)
	}
}

func TestListPendingGrantsAndAgentClients(t *testing.T) {
	a := testApp(t)
	grant, err := a.GrantPath(context.Background(), a.Home(), PathGrantInit)
	if err != nil {
		t.Fatal(err)
	}
	pending := a.ListPendingGrants()
	if len(pending) != 1 || pending[0].ID != grant.ID {
		t.Fatalf("pending grants: %+v", pending)
	}
	if err := os.MkdirAll(filepath.Join(a.Home(), ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(a.Home(), "agents")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Init(context.Background(), InitOptions{Root: root, Agents: true, WriteClientCfg: true, Register: true}); err != nil {
		t.Fatal(err)
	}
	report := a.ListAgentClients(context.Background())
	if !report.Attempted {
		t.Fatal("agents report was not attempted")
	}
	raw, _ := json.Marshal(report)
	if strings.Contains(string(raw), `"connected"`) || strings.Contains(string(raw), `"ready"`) {
		t.Fatalf("must not fake connected status: %s", raw)
	}
}
