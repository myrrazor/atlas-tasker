package mcp

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
)

func TestApprovalRootUsesMachineStateDirForMachineScopedTools(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "ws")
	state := filepath.Join(t.TempDir(), "state")
	grant, ok := ToolSpecByName("atlas.settings.grant_discovery")
	if !ok || grant.Scope != ScopeMachine {
		t.Fatalf("grant_discovery must be machine-scoped: %#v", grant)
	}
	merge, ok := ToolSpecByName("atlas.change.merge")
	if !ok || merge.Scope == ScopeMachine {
		t.Fatalf("change.merge must be workspace-scoped: %#v", merge)
	}
	cases := []struct {
		name      string
		spec      ToolSpec
		workspace string
		state     string
		want      string
		wantErr   bool
	}{
		{"machine tool inside a workspace", grant, ws, state, state, false},
		{"machine tool outside a workspace", grant, "", state, state, false},
		{"machine tool without state dir", grant, ws, "", "", true},
		{"workspace tool in a workspace", merge, ws, state, ws, false},
		{"workspace tool, global server, no workspace bound", merge, "", state, state, false},
		{"workspace tool with nothing", merge, "", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ApprovalRoot(tc.spec, tc.workspace, tc.state)
			if tc.wantErr {
				if apperr.CodeOf(err) != apperr.CodeInvalidInput {
					t.Fatalf("want invalid_input, got %q, %v", got, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("ApprovalRoot = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
	if ApprovalScope(grant) != "machine" || ApprovalScope(merge) != "workspace" {
		t.Fatal("ApprovalScope must follow ToolSpec.Scope")
	}
}

// Regression for v1.18.0 acceptance blocker 3: an approval written where
// `tracker mcp approve-operation` writes it must be found by the global
// server's machine-scoped atlas.settings.grant_discovery.
func TestGlobalGrantDiscoveryConsumesMachineScopedApproval(t *testing.T) {
	outside := t.TempDir()
	machine := testGlobalMachine(t, outside)
	allowed := filepath.Join(outside, "projects")
	workspaceRoot := filepath.Join(outside, "repo")
	initRegisteredWorkspace(t, machine, workspaceRoot)
	server := NewGlobalServer(machine, Options{
		Profile:              ProfileAdmin,
		AllowHighImpactTools: true,
		CWD:                  workspaceRoot,
		Machine:              machine,
		Now:                  func() time.Time { return time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC) },
	})
	spec, _ := ToolSpecByName("atlas.settings.grant_discovery")
	ctx := context.Background()
	call := func(id string) error {
		_, err := server.CallTool(ctx, "atlas.settings.grant_discovery", map[string]any{
			"root":                  allowed,
			"actor":                 "human:owner",
			"reason":                "allow projects dir",
			"operation_approval_id": id,
			"confirm_text":          "execute atlas.settings.grant_discovery " + allowed,
		})
		return err
	}

	// The pre-fix location (the CWD workspace) must not satisfy a machine tool.
	wrong, err := NewApprovalStore(workspaceRoot, server.Options.Now).Create(ctx, spec.Name, allowed, "human:owner", 10*time.Minute, "wrong store")
	if err != nil {
		t.Fatal(err)
	}
	if err := call(wrong.ID); err == nil {
		t.Fatal("workspace-stored approval must not authorize a machine-scoped tool")
	}

	root, err := ApprovalRoot(spec, workspaceRoot, machine.StateDir())
	if err != nil {
		t.Fatal(err)
	}
	if root != machine.StateDir() {
		t.Fatalf("machine approval root = %q, want %q", root, machine.StateDir())
	}
	approval, err := NewApprovalStore(root, server.Options.Now).Create(ctx, spec.Name, allowed, "human:owner", 10*time.Minute, "allow projects dir")
	if err != nil {
		t.Fatal(err)
	}
	if err := call(approval.ID); err != nil {
		t.Fatalf("machine-scoped approval round trip failed: %v", err)
	}
	settings := machine.Settings()
	if !settings.Discovery.Enabled || len(settings.Discovery.Roots) == 0 {
		t.Fatalf("discovery not granted: %#v", settings.Discovery)
	}
	if err := call(approval.ID); err == nil {
		t.Fatal("approval must be single-use")
	}
}
