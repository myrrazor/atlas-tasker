package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	atlasmcp "github.com/myrrazor/atlas-tasker/internal/mcp"
)

type approveOutput struct {
	Scope       string `json:"scope"`
	ConfirmText string `json:"confirm_text"`
	Approval    struct {
		ID        string `json:"id"`
		Operation string `json:"operation"`
		Target    string `json:"target"`
	} `json:"approval"`
}

func approvalFile(root, id string) string {
	return filepath.Join(root, ".tracker", "runtime", "mcp", "operation-approvals", id+".json")
}

// Regression for v1.18.0 acceptance blocker 3 (rough edge 26).
func TestMCPApproveOperationMachineScopedRoundTrip(t *testing.T) {
	withTempWorkspace(t)
	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	workspace, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	stateDir := mcpStateDir()
	if stateDir == "" {
		t.Fatal("machine state dir did not resolve")
	}
	target := filepath.Join(t.TempDir(), "projects")
	out, err := runCLI(t, "mcp", "approve-operation", "--operation", "settings.grant_discovery", "--target", target, "--actor", "human:owner", "--reason", "allow projects", "--json")
	if err != nil {
		t.Fatalf("approve machine operation: %v\n%s", err, out)
	}
	var got approveOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if got.Scope != "machine" || got.Approval.Operation != "atlas.settings.grant_discovery" {
		t.Fatalf("unexpected approval: %+v", got)
	}
	if _, err := os.Stat(approvalFile(stateDir, got.Approval.ID)); err != nil {
		t.Fatalf("machine approval not in state dir: %v", err)
	}
	if _, err := os.Stat(approvalFile(workspace, got.Approval.ID)); !os.IsNotExist(err) {
		t.Fatalf("machine approval must not be written to the workspace: %v", err)
	}

	listOut, err := runCLI(t, "mcp", "approvals", "list", "--json")
	if err != nil || !strings.Contains(listOut, got.Approval.ID) || !strings.Contains(listOut, `"scope": "machine"`) {
		t.Fatalf("list must show the machine approval: %v\n%s", err, listOut)
	}

	// The MCP server resolves the same store through ApprovalRoot.
	spec, _ := atlasmcp.ToolSpecByName("atlas.settings.grant_discovery")
	root, err := atlasmcp.ApprovalRoot(spec, "", stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := atlasmcp.NewApprovalStore(root, defaultNow).Consume(context.Background(), got.Approval.ID, spec.Name, target, "human:owner", spec.Name); err != nil {
		t.Fatalf("server-side lookup of CLI approval failed: %v", err)
	}
}

func TestMCPApproveOperationMachineScopedWorksOutsideWorkspace(t *testing.T) {
	withTempWorkspace(t)
	target := filepath.Join(t.TempDir(), "projects")
	out, err := runCLI(t, "mcp", "approve-operation", "--operation", "atlas.settings.grant_discovery", "--target", target, "--actor", "human:owner", "--reason", "allow projects", "--json")
	if err != nil {
		t.Fatalf("machine approval outside a workspace: %v\n%s", err, out)
	}
	if _, err := os.Stat(".tracker"); !os.IsNotExist(err) {
		t.Fatalf("machine approval must not bootstrap a workspace: %v", err)
	}
	var got approveOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "mcp", "approvals", "revoke", got.Approval.ID, "--json"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := os.Stat(approvalFile(mcpStateDir(), got.Approval.ID)); !os.IsNotExist(err) {
		t.Fatalf("revoke must remove the machine approval: %v", err)
	}
}

func TestMCPApproveOperationWorkspaceScopedStaysInWorkspace(t *testing.T) {
	withTempWorkspace(t)
	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	workspace, _ := os.Getwd()
	out, err := runCLI(t, "mcp", "approve-operation", "--operation", "change.merge", "--target", "CHG-1", "--actor", "human:owner", "--reason", "merge", "--json")
	if err != nil {
		t.Fatalf("approve: %v\n%s", err, out)
	}
	var got approveOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.Scope != "workspace" {
		t.Fatalf("scope = %q", got.Scope)
	}
	if _, err := os.Stat(approvalFile(workspace, got.Approval.ID)); err != nil {
		t.Fatalf("workspace approval missing from workspace: %v", err)
	}
	if _, err := os.Stat(approvalFile(mcpStateDir(), got.Approval.ID)); !os.IsNotExist(err) {
		t.Fatalf("workspace approval leaked into machine state dir: %v", err)
	}
	spec, _ := atlasmcp.ToolSpecByName("atlas.change.merge")
	root, err := atlasmcp.ApprovalRoot(spec, workspace, mcpStateDir())
	if err != nil || root != workspace {
		t.Fatalf("ApprovalRoot = %q, %v", root, err)
	}
	if _, err := atlasmcp.NewApprovalStore(root, defaultNow).Consume(context.Background(), got.Approval.ID, spec.Name, "CHG-1", "human:owner", spec.Name); err != nil {
		t.Fatalf("workspace round trip: %v", err)
	}
}

func revokeJSON(t *testing.T, id string) (scope string, found bool) {
	t.Helper()
	out, err := runCLI(t, "mcp", "approvals", "revoke", id, "--json")
	if err != nil {
		t.Fatalf("revoke %s: %v\n%s", id, err, out)
	}
	var got struct {
		Kind  string `json:"kind"`
		ID    string `json:"id"`
		Scope string `json:"scope"`
		Found bool   `json:"found"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse revoke: %v\n%s", err, out)
	}
	if got.Kind != "mcp_operation_approval_revoked" || got.ID != id {
		t.Fatalf("unexpected revoke payload: %s", out)
	}
	return got.Scope, got.Found
}

func TestMCPApprovalsRevokeJSONReportsScope(t *testing.T) {
	withTempWorkspace(t)
	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	approve := func(args ...string) string {
		out, err := runCLI(t, append([]string{"mcp", "approve-operation", "--actor", "human:owner", "--reason", "revoke scope", "--json"}, args...)...)
		if err != nil {
			t.Fatalf("approve: %v\n%s", err, out)
		}
		var got approveOutput
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatal(err)
		}
		return got.Approval.ID
	}
	machineID := approve("--operation", "atlas.settings.grant_discovery", "--target", filepath.Join(t.TempDir(), "projects"))
	workspaceID := approve("--operation", "atlas.change.merge", "--target", "CHG-1")

	if scope, found := revokeJSON(t, machineID); scope != "machine" || !found {
		t.Fatalf("machine revoke scope=%q found=%v", scope, found)
	}
	if scope, found := revokeJSON(t, workspaceID); scope != "workspace" || !found {
		t.Fatalf("workspace revoke scope=%q found=%v", scope, found)
	}
	if scope, found := revokeJSON(t, machineID); scope != "" || found {
		t.Fatalf("second revoke must report not found, got scope=%q found=%v", scope, found)
	}
}
