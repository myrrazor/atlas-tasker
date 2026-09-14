package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/app"
	"github.com/myrrazor/atlas-tasker/internal/apperr"
)

func TestApprovedPathGateRejectsSymlinkEscapeAndAllowsCWD(t *testing.T) {
	outside := t.TempDir()
	machine := testGlobalMachine(t, outside)
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

	allowed := filepath.Join(outside, "allowed")
	if err := os.MkdirAll(allowed, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(outside, "secret")
	if err := os.MkdirAll(secret, 0o755); err != nil {
		t.Fatal(err)
	}
	hop := filepath.Join(allowed, "hop")
	if err := os.Symlink(secret, hop); err != nil {
		t.Fatal(err)
	}
	if _, err := machine.GrantDiscovery(ctx, app.DiscoverySettings{Enabled: true, Roots: []string{allowed}, MaxDepth: 4}); err != nil {
		t.Fatalf("grant discovery: %v", err)
	}

	server := NewGlobalServer(machine, Options{Profile: ProfileWorkflow, Now: func() time.Time { return now }, CWD: outside, Machine: machine})
	if _, err := server.CallTool(ctx, "atlas.workspace.init", map[string]any{
		"path": hop, "actor": "human:owner", "reason": "escape", "agents": false, "register": true, "backup": false,
	}); err == nil {
		t.Fatal("init via symlink escape must be rejected")
	} else if apperr.CodeOf(err) != apperr.CodePermissionDenied && apperr.CodeOf(err) != apperr.CodeInvalidInput {
		t.Fatalf("unexpected init escape error: %v", err)
	}

	if _, err := server.CallTool(ctx, "atlas.workspace.register", map[string]any{
		"path": hop, "actor": "human:owner", "reason": "register escape",
	}); err == nil {
		t.Fatal("register via symlink escape must be rejected")
	}

	cwdRoot := filepath.Join(outside, "cwd-ws")
	if err := os.MkdirAll(cwdRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	cwdMachine := AdaptApp(machine.(*appMachine).app, cwdRoot)
	cwdServer := NewGlobalServer(cwdMachine, Options{Profile: ProfileWorkflow, Now: func() time.Time { return now }, CWD: cwdRoot, Machine: cwdMachine})
	if _, err := cwdServer.CallTool(ctx, "atlas.workspace.init", map[string]any{
		"actor": "human:owner", "reason": "cwd init", "agents": false, "register": true, "backup": false,
	}); err != nil {
		t.Fatalf("current CWD init must still work: %v", err)
	}
}

func TestRepairCopyPathRejectsSymlinkEscape(t *testing.T) {
	outside := t.TempDir()
	machine := testGlobalMachine(t, outside)
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	root := filepath.Join(outside, "src")
	id := initRegisteredWorkspace(t, machine, root)
	allowed := filepath.Join(outside, "allowed")
	if err := os.MkdirAll(allowed, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(outside, "secret-copy")
	if err := os.MkdirAll(secret, 0o755); err != nil {
		t.Fatal(err)
	}
	hop := filepath.Join(allowed, "copy")
	if err := os.Symlink(secret, hop); err != nil {
		t.Fatal(err)
	}
	if _, err := machine.GrantDiscovery(ctx, app.DiscoverySettings{Enabled: true, Roots: []string{allowed}, MaxDepth: 4}); err != nil {
		t.Fatal(err)
	}
	server := NewGlobalServer(machine, Options{
		Profile: ProfileAdmin, AllowHighImpactTools: true,
		Now: func() time.Time { return now }, CWD: outside, Machine: machine,
	})
	if _, err := server.CallTool(ctx, "atlas.workspace.repair", map[string]any{
		"workspace_id": id, "action": "update_path", "path": hop,
		"actor": "human:owner", "reason": "repair escape",
	}); err == nil {
		t.Fatal("repair new path symlink escape must be rejected")
	}
}
