package mcp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestWorkflowLoopToolsCreateAssignLinkApproveComplete(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	workspace, err := OpenWorkspace(root, nil, func() time.Time { return now })
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	defer workspace.Close()
	ctx := context.Background()
	if err := workspace.Actions.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	server := NewServer(workspace, Options{Profile: ProfileWorkflow, Now: func() time.Time { return now }}.Normalized())

	created, err := server.CallTool(ctx, "atlas.ticket.create", map[string]any{
		"project": "APP", "title": "Build it", "type": "task", "status": "in_progress",
		"actor": "human:owner", "reason": "seed",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	ticketID := payloadTicketID(t, created)

	if _, err := server.CallTool(ctx, "atlas.ticket.assign", map[string]any{
		"ticket_id": ticketID, "assignee": "agent:builder-1", "actor": "human:owner", "reason": "assign",
	}); err != nil {
		t.Fatalf("assign: %v", err)
	}
	blocker, err := server.CallTool(ctx, "atlas.ticket.create", map[string]any{
		"project": "APP", "title": "Blocker", "type": "task", "status": "done",
		"actor": "human:owner", "reason": "blocker",
	})
	if err != nil {
		t.Fatalf("create blocker: %v", err)
	}
	blockerID := payloadTicketID(t, blocker)
	if _, err := server.CallTool(ctx, "atlas.ticket.link", map[string]any{
		"ticket_id": ticketID, "other_id": blockerID, "kind": "blocked_by", "actor": "human:owner", "reason": "link",
	}); err != nil {
		t.Fatalf("link: %v", err)
	}
	if _, err := server.CallTool(ctx, "atlas.ticket.complete", map[string]any{
		"ticket_id": ticketID, "actor": "agent:builder-1", "reason": "solo close",
	}); err != nil {
		t.Fatalf("open-mode complete from in_progress: %v", err)
	}
	view, err := server.CallTool(ctx, "atlas.ticket.view", map[string]any{"ticket_id": ticketID})
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	raw := fmt.Sprintf("%v", view)
	if !strings.Contains(raw, "done") {
		t.Fatalf("expected completed ticket, got %#v", view)
	}
}

func TestGoalBriefAndTeamToolsOnReadAndWorkflow(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	workspace, err := OpenWorkspace(root, nil, func() time.Time { return now })
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	defer workspace.Close()
	ctx := context.Background()
	if err := workspace.Actions.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket, err := workspace.Actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project: "APP", Title: "Brief me", Type: contracts.TicketTypeTask,
		Status: contracts.StatusReady, Priority: contracts.PriorityMedium,
		CreatedAt: now, UpdatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	read := NewServer(workspace, Options{Profile: ProfileRead, Now: func() time.Time { return now }}.Normalized())
	if _, err := read.CallTool(ctx, "atlas.goal.brief", map[string]any{"target": ticket.ID}); err != nil {
		t.Fatalf("goal brief: %v", err)
	}
	if _, err := read.CallTool(ctx, "atlas.team.list", map[string]any{}); err != nil {
		t.Fatalf("team list: %v", err)
	}
	workflow := NewServer(workspace, Options{Profile: ProfileWorkflow, Now: func() time.Time { return now }}.Normalized())
	if _, err := workflow.CallTool(ctx, "atlas.team.apply", map[string]any{
		"preset": "solo", "dry_run": true, "actor": "human:owner", "reason": "preview",
	}); err != nil {
		t.Fatalf("team apply: %v", err)
	}
	if _, err := workflow.CallTool(ctx, "atlas.agent.create", map[string]any{
		"agent_id": "builder-1", "name": "Builder", "provider": "codex", "actor": "human:owner", "reason": "create",
	}); err != nil {
		t.Fatalf("agent create: %v", err)
	}
}

func TestContentLengthFramingRoundTrip(t *testing.T) {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	framedIn, framedOut := NewStdioFraming(inR, outW)
	go func() {
		defer framedOut.Close()
		buf := make([]byte, 4096)
		n, err := framedIn.Read(buf)
		if err != nil {
			t.Errorf("read framed body: %v", err)
			return
		}
		_, _ = framedOut.Write(buf[:n])
	}()
	body := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	msg := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	if _, err := io.WriteString(inW, msg); err != nil {
		t.Fatalf("write request: %v", err)
	}
	_ = inW.Close()
	raw, err := io.ReadAll(outR)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if !bytes.Contains(raw, []byte("Content-Length:")) || !bytes.Contains(raw, []byte(body)) {
		t.Fatalf("expected header-framed reply, got %q", raw)
	}
}

func payloadTicketID(t *testing.T, payload map[string]any) string {
	t.Helper()
	inner := payload["payload"]
	switch value := inner.(type) {
	case contracts.TicketSnapshot:
		if value.ID != "" {
			return value.ID
		}
	case map[string]any:
		if id, ok := value["id"].(string); ok && id != "" {
			return id
		}
	}
	raw := fmt.Sprintf("%#v", payload)
	if idx := strings.Index(raw, "APP-"); idx >= 0 {
		end := idx + 4
		for end < len(raw) && raw[end] >= '0' && raw[end] <= '9' {
			end++
		}
		return raw[idx:end]
	}
	t.Fatalf("ticket id not found in %#v", payload)
	return ""
}
