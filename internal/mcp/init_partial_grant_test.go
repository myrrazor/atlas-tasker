package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/myrrazor/atlas-tasker/internal/app"
	"github.com/myrrazor/atlas-tasker/internal/apperr"
)

func TestInitPartialFailureKeepsWorkspaceOnSDKError(t *testing.T) {
	outside := t.TempDir()
	machine := testGlobalMachineWithHomeService(t, outside)
	root := filepath.Join(outside, "partial")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	grant, err := machine.GrantPath(ctx, root, app.PathGrantInit)
	if err != nil {
		t.Fatalf("grant: %v", err)
	}

	result, err := machine.Init(ctx, InitCall{
		Root:           root,
		GrantID:        grant.ID,
		Register:       true,
		Agents:         false,
		Backup:         false,
		DefaultProject: true,
		Actor:          "human:owner",
		WriteClientCfg: false,
	})
	if err == nil {
		t.Fatal("noop spawner must surface PartialError, not a silent success")
	}
	if !app.IsPartial(err) {
		t.Fatalf("expected PartialError, got %T %v", err, err)
	}
	if result.WorkspaceID == "" {
		t.Fatal("partial init must keep workspace identity")
	}
	if _, err := machine.Bind(ctx, result.WorkspaceID); err != nil {
		t.Fatalf("partial workspace must stay bindable: %v", err)
	}

	server := NewGlobalServer(machine, Options{Profile: ProfileWorkflow, Now: func() time.Time { return now }, CWD: outside, Machine: machine})
	other := filepath.Join(outside, "sdk-partial")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	sdkGrant, err := machine.GrantPath(ctx, other, app.PathGrantInit)
	if err != nil {
		t.Fatalf("sdk grant: %v", err)
	}

	payload, callErr := server.CallTool(ctx, "atlas.workspace.init", map[string]any{
		"path": other, "grant_id": sdkGrant.ID, "actor": "human:owner", "reason": "partial sdk",
		"agents": false, "backup": false, "register": true,
	})
	if callErr == nil {
		t.Fatal("CallTool must return PartialError")
	}
	if !app.IsPartial(callErr) {
		t.Fatalf("CallTool error %T %v", callErr, callErr)
	}
	id := payloadWorkspaceID(t, payload)
	if id == "" {
		t.Fatalf("CallTool dropped InitResult: %#v", payload)
	}

	clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
	if _, err := server.SDKServer().Connect(ctx, serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "atlas-test", Version: "v0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	sdkRoot := filepath.Join(outside, "sdk-session")
	if err := os.MkdirAll(sdkRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionGrant, err := machine.GrantPath(ctx, sdkRoot, app.PathGrantInit)
	if err != nil {
		t.Fatalf("session grant: %v", err)
	}
	res, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name: "atlas.workspace.init",
		Arguments: map[string]any{
			"path": sdkRoot, "grant_id": sessionGrant.ID, "actor": "human:owner", "reason": "sdk partial",
			"agents": false, "backup": false, "register": true,
		},
	})
	if err != nil {
		t.Fatalf("SDK transport: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("SDK result must set isError on partial init: %#v", res)
	}
	sessionID := structuredWorkspaceID(t, res.StructuredContent)
	if sessionID == "" {
		t.Fatalf("SDK structured result missing workspace id: %#v", res.StructuredContent)
	}
	if env := structuredError(res.StructuredContent); env == nil {
		t.Fatalf("SDK structured result missing error information: %#v", res.StructuredContent)
	}
	if _, err := machine.Bind(ctx, sessionID); err != nil {
		t.Fatalf("SDK partial workspace must stay bindable: %v", err)
	}
	if _, err := server.CallTool(ctx, "atlas.workspace.get", map[string]any{"workspace_id": sessionID}); err != nil {
		t.Fatalf("get after partial: %v", err)
	}
	if _, err := server.CallTool(ctx, "atlas.project.create", map[string]any{
		"workspace_id": sessionID, "key": "FIX", "name": "Repair",
	}); err != nil {
		t.Fatalf("workspace after partial must still accept work: %v", err)
	}
}

func TestInitGrantPurposeAndPathContracts(t *testing.T) {
	cwd := t.TempDir()
	machine := testGlobalMachine(t, cwd)
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	server := NewGlobalServer(machine, Options{Profile: ProfileWorkflow, Now: func() time.Time { return now }, CWD: cwd, Machine: machine})

	wrongDir := filepath.Join(cwd, "wrong-purpose")
	if err := os.MkdirAll(wrongDir, 0o755); err != nil {
		t.Fatal(err)
	}
	registerGrant, err := machine.GrantPath(ctx, wrongDir, app.PathGrantRegister)
	if err != nil {
		t.Fatalf("register grant: %v", err)
	}
	_, err = server.CallTool(ctx, "atlas.workspace.init", map[string]any{
		"path": wrongDir, "grant_id": registerGrant.ID, "actor": "human:owner", "reason": "wrong purpose",
		"agents": false, "backup": false,
	})
	if err == nil {
		t.Fatal("wrong-purpose grant must not init")
	}
	if apperr.CodeOf(err) != apperr.CodePermissionDenied {
		t.Fatalf("wrong-purpose code %s: %v", apperr.CodeOf(err), err)
	}
	_, err = server.CallTool(ctx, "atlas.workspace.init", map[string]any{
		"path": wrongDir, "grant_id": registerGrant.ID, "actor": "human:owner", "reason": "still unused",
		"agents": false, "backup": false,
	})
	if err == nil {
		t.Fatal("wrong-purpose grant must still be unused")
	}
	if apperr.CodeOf(err) != apperr.CodePermissionDenied {
		t.Fatalf("burned grant would be not_found, got %s: %v", apperr.CodeOf(err), err)
	}

	granted := t.TempDir()
	grant, err := machine.GrantPath(ctx, granted, app.PathGrantInit)
	if err != nil {
		t.Fatalf("init grant: %v", err)
	}
	payload, err := server.CallTool(ctx, "atlas.workspace.init", map[string]any{
		"grant_id": grant.ID, "actor": "human:owner", "reason": "grant only",
		"agents": false, "backup": false, "register": true,
	})
	if err != nil {
		t.Fatalf("grant-only init outside CWD: %v", err)
	}
	id := payloadWorkspaceID(t, payload)
	if id == "" {
		t.Fatal("grant-only init missing workspace id")
	}
	ws, err := machine.Bind(ctx, id)
	if err != nil {
		t.Fatalf("bind grant-only workspace: %v", err)
	}
	if !samePathIdentity(ws.Root, granted) {
		t.Fatalf("grant-only init used %q, want %q", ws.Root, granted)
	}
	_, err = server.CallTool(ctx, "atlas.workspace.init", map[string]any{
		"grant_id": grant.ID, "actor": "human:owner", "reason": "replay",
		"agents": false, "backup": false,
	})
	if err == nil {
		t.Fatal("replayed grant must fail")
	}
	if apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("replay code %s: %v", apperr.CodeOf(err), err)
	}

	matchDir := t.TempDir()
	otherDir := t.TempDir()
	mismatchGrant, err := machine.GrantPath(ctx, matchDir, app.PathGrantInit)
	if err != nil {
		t.Fatalf("mismatch grant: %v", err)
	}
	_, err = server.CallTool(ctx, "atlas.workspace.init", map[string]any{
		"path": otherDir, "grant_id": mismatchGrant.ID, "actor": "human:owner", "reason": "mismatch",
		"agents": false, "backup": false,
	})
	if err == nil {
		t.Fatal("explicit path must match grant identity")
	}
	if apperr.CodeOf(err) != apperr.CodePermissionDenied {
		t.Fatalf("mismatch code %s: %v", apperr.CodeOf(err), err)
	}
}

func TestHappyPathInitSucceedsWithServiceDisabled(t *testing.T) {
	outside := t.TempDir()
	machine := testGlobalMachine(t, outside)
	root := filepath.Join(outside, "ok")
	id := initRegisteredWorkspace(t, machine, root)
	if _, err := machine.Bind(context.Background(), id); err != nil {
		t.Fatalf("honest init must bind: %v", err)
	}
}

func payloadWorkspaceID(t *testing.T, payload map[string]any) string {
	t.Helper()
	if payload == nil {
		t.Fatal("missing tool payload")
	}
	switch inner := payload["payload"].(type) {
	case app.InitResult:
		return inner.WorkspaceID
	case *app.InitResult:
		if inner == nil {
			return ""
		}
		return inner.WorkspaceID
	case map[string]any:
		id, _ := inner["workspace_id"].(string)
		return id
	default:
		obj := asStringMap(payload["payload"])
		if obj == nil {
			t.Fatalf("payload is not an object: %#v", payload["payload"])
		}
		id, _ := obj["workspace_id"].(string)
		return id
	}
}

func structuredWorkspaceID(t *testing.T, structured any) string {
	t.Helper()
	obj := asStringMap(structured)
	if obj == nil {
		t.Fatalf("structured content is not an object: %#v", structured)
	}
	if inner, ok := obj["payload"].(map[string]any); ok {
		if id, _ := inner["workspace_id"].(string); id != "" {
			return id
		}
	}
	if id, _ := obj["workspace_id"].(string); id != "" {
		return id
	}
	return ""
}

func structuredError(structured any) any {
	obj := asStringMap(structured)
	if obj == nil {
		return nil
	}
	return obj["error"]
}

func asStringMap(v any) map[string]any {
	if obj, ok := v.(map[string]any); ok {
		return obj
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var obj map[string]any
	if json.Unmarshal(raw, &obj) != nil {
		return nil
	}
	return obj
}
