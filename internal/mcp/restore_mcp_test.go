package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestRestoreApplyUsesStoredPlanIDAndServiceDigest(t *testing.T) {
	outside := t.TempDir()
	machine := testGlobalMachine(t, outside)
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	root := filepath.Join(outside, "repo")
	id := initRegisteredWorkspace(t, machine, root)
	ws, err := machine.Bind(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.Actions.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project: "APP", Title: "restore me", Type: contracts.TicketTypeTask,
		Status: contracts.StatusReady, Priority: contracts.PriorityMedium,
		CreatedAt: now, UpdatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed"); err != nil {
		t.Fatal(err)
	}
	backup, err := ws.Actions.CreateBackup(ctx, "workspace", contracts.Actor("human:owner"), "checkpoint")
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	server := NewGlobalServer(machine, Options{
		Profile: ProfileAdmin, AllowHighImpactTools: true,
		Now: func() time.Time { return now }, CWD: outside, Machine: machine, StateDir: machine.StateDir(),
	})

	plan1, err := server.CallTool(ctx, "atlas.restore.plan", map[string]any{
		"workspace_id": id, "ref": backup.Snapshot.BackupID, "actor": "human:owner", "reason": "plan one",
	})
	if err != nil {
		t.Fatalf("plan1: %v", err)
	}
	id1, digest1 := restoreIDs(t, plan1)
	stored, err := ws.Actions.RestorePlans.LoadRestorePlan(ctx, id1)
	if err != nil {
		t.Fatal(err)
	}
	if service.RestorePlanBindingDigest(stored) != digest1 {
		t.Fatalf("MCP digest %q != service digest %q", digest1, service.RestorePlanBindingDigest(stored))
	}

	if err := os.WriteFile(storage.TicketFile(root, "APP", "APP-1"), []byte("---\nid: APP-1\n---\nchanged target\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan2, err := server.CallTool(ctx, "atlas.restore.plan", map[string]any{
		"workspace_id": id, "ref": backup.Snapshot.BackupID, "actor": "human:owner", "reason": "plan two after target change",
	})
	if err != nil {
		t.Fatalf("plan2: %v", err)
	}
	id2, digest2 := restoreIDs(t, plan2)
	if id1 == id2 {
		t.Fatal("second plan must have its own plan id")
	}
	if digest1 == digest2 {
		t.Fatal("changed target must change restore digest")
	}

	mismatch := specTarget(mustSpec(t, "atlas.restore.apply"), map[string]any{"plan_id": id1, "digest": digest2})
	match := specTarget(mustSpec(t, "atlas.restore.apply"), map[string]any{"plan_id": id1, "digest": digest1})
	if mismatch == match {
		t.Fatal("digest swap must change approval target")
	}

	store := NewApprovalStore(root, func() time.Time { return now })
	approval, err := store.Create(ctx, "atlas.restore.apply", match, "human:owner", 10*time.Minute, "apply plan one")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.CallTool(ctx, "atlas.restore.apply", map[string]any{
		"workspace_id": id, "plan_id": id2, "digest": digest2,
		"actor": "human:owner", "reason": "wrong plan",
		"operation_approval_id": approval.ID,
		"confirm_text":          "execute atlas.restore.apply " + match,
	}); err == nil {
		t.Fatal("approval for plan1 must not apply plan2")
	}

	bound, err := store.Create(ctx, "atlas.restore.apply", mismatch, "human:owner", 10*time.Minute, "mismatched digest")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.CallTool(ctx, "atlas.restore.apply", map[string]any{
		"workspace_id": id, "plan_id": id1, "digest": digest2,
		"actor": "human:owner", "reason": "digest swap",
		"operation_approval_id": bound.ID,
		"confirm_text":          "execute atlas.restore.apply " + mismatch,
	}); err == nil {
		t.Fatal("source/digest swap must be refused")
	}
}

func restoreIDs(t *testing.T, payload map[string]any) (string, string) {
	t.Helper()
	inner, _ := payload["payload"].(map[string]any)
	if inner == nil {
		raw, _ := json.Marshal(payload)
		t.Fatalf("restore payload: %s", raw)
	}
	id, _ := inner["plan_id"].(string)
	digest, _ := inner["digest"].(string)
	if id == "" || digest == "" {
		t.Fatalf("missing plan id/digest: %#v", inner)
	}
	return id, digest
}

func mustSpec(t *testing.T, name string) ToolSpec {
	t.Helper()
	spec, ok := ToolSpecByName(name)
	if !ok {
		t.Fatalf("missing spec %s", name)
	}
	return spec
}

func TestSpecTargetBindsMaterialRestoreForkAndBackupInputs(t *testing.T) {
	restore := specTarget(mustSpec(t, "atlas.restore.apply"), map[string]any{"plan_id": "plan-1", "digest": "abc"})
	if restore != jsonTarget(map[string]any{"plan_id": "plan-1", "digest": "abc"}) {
		t.Fatalf("restore target: %s", restore)
	}
	fork := specTarget(mustSpec(t, "atlas.workspace.fork_copy"), map[string]any{"workspace_id": "ws-1", "path": "/tmp/copy"})
	if fork != jsonTarget(map[string]any{"workspace_id": "ws-1", "path": "/tmp/copy"}) {
		t.Fatalf("fork target: %s", fork)
	}
	backup := specTarget(mustSpec(t, "atlas.backup.configure"), map[string]any{"action": "add", "target_id": "t1", "url": "https://example.invalid/repo.git"})
	if backup != jsonTarget(map[string]any{"action": "add", "target_id": "t1", "url": "https://example.invalid/repo.git"}) {
		t.Fatalf("backup target: %s", backup)
	}
	changed := specTarget(mustSpec(t, "atlas.backup.configure"), map[string]any{"action": "add", "target_id": "t2", "url": "https://example.invalid/repo.git"})
	if backup == changed {
		t.Fatal("changed target_id must not reuse approval target")
	}
	if strings.Contains(fork, jsonTarget(map[string]any{"workspace_id": "ws-1"})) && !strings.Contains(fork, "path") {
		t.Fatal("fork target omitted copy path")
	}
}
