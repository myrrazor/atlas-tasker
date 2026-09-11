package mcp

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestWorkflowCannotMutateBackupAndStatusHidesURLs(t *testing.T) {
	for _, spec := range ToolSpecs() {
		if spec.Name == "atlas.backup.status" {
			if spec.Class != ClassRead {
				t.Fatal("atlas.backup.status must be read-only")
			}
			continue
		}
		if strings.HasPrefix(spec.Name, "atlas.backup.") {
			t.Fatalf("unexpected backup tool %s", spec.Name)
		}
	}
	workflow := Inventory(Options{Profile: ProfileWorkflow}.Normalized())
	if toolEnabled(workflow, "atlas.backup.target") || toolEnabled(workflow, "atlas.backup.restore") {
		t.Fatal("workflow must not expose backup write tools")
	}

	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", home)
	now := time.Date(2026, 9, 11, 16, 0, 0, 0, time.UTC)
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(storage.WorkspaceMetadataFile(root), []byte(`{"workspace_id":"ws-mcp-sec"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspace, err := OpenWorkspace(root, nil, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Close()
	service.AttachUserState(workspace.Actions, workspace.Queries, home, home)
	ctx := context.Background()
	if _, err := workspace.Actions.AddBackupTarget(ctx, service.BackupTargetAddOptions{
		TargetID: "hidden", URL: "https://git.example.com/org/private.git", Enabled: true,
		AcknowledgeBoundary: true, AttestPrivate: true,
	}); err != nil {
		t.Fatal(err)
	}
	server := NewServer(workspace, Options{Profile: ProfileWorkflow, Now: func() time.Time { return now }}.Normalized())
	result, err := server.CallTool(ctx, "atlas.backup.status", map[string]any{})
	if err != nil {
		t.Fatalf("backup.status: %v", err)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if strings.Contains(body, "git.example.com") || strings.Contains(body, "https://") || strings.Contains(body, "file://") {
		t.Fatalf("backup status leaked a URL:\n%s", body)
	}
}
