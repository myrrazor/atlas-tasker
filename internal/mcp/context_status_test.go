package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/render"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestContextAndStatusManagedWorkflow(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 11, 16, 0, 0, 0, time.UTC)
	if err := config.Save(root, contracts.TrackerConfig{
		Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeReviewGate, RequiredReviewer: "agent:reviewer-1"},
		Actor:    contracts.ActorConfig{Default: "agent:builder-1"},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := os.WriteFile(storage.WorkspaceMetadataFile(root), []byte(`{"workspace_id":"ws-managed-test"}`+"\n"), 0o644); err != nil {
		t.Fatalf("workspace id: %v", err)
	}
	policy, err := contracts.ManagedModePolicyForMode(contracts.ManagedModeDelivery)
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	body, err := policy.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(storage.ManagedModeFile(root)), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(storage.ManagedModeFile(root), body, 0o644); err != nil {
		t.Fatalf("write policy: %v", err)
	}
	workspace, err := OpenWorkspace(root, nil, func() time.Time { return now })
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer workspace.Close()
	ctx := context.Background()
	if err := workspace.Actions.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create APP: %v", err)
	}
	if err := workspace.Actions.CreateProject(ctx, contracts.Project{Key: "OPS", Name: "Ops", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create OPS: %v", err)
	}
	server := NewServer(workspace, Options{Profile: ProfileRead, Now: func() time.Time { return now }}.Normalized())

	before, err := workspace.Queries.Tickets.ListTickets(ctx, contracts.TicketListOptions{})
	if err != nil {
		t.Fatalf("list before: %v", err)
	}
	contextResult, err := server.CallTool(ctx, "atlas.context", map[string]any{"project": "APP"})
	if err != nil {
		t.Fatalf("context: %v", err)
	}
	statusAPP, err := server.CallTool(ctx, "atlas.status", map[string]any{"project": "APP"})
	if err != nil {
		t.Fatalf("status APP: %v", err)
	}
	statusUnknown, err := server.CallTool(ctx, "atlas.status", map[string]any{"project": "NOPE"})
	if err != nil {
		t.Fatalf("status unknown: %v", err)
	}
	statusWorkspace, err := server.CallTool(ctx, "atlas.status", map[string]any{})
	if err != nil {
		t.Fatalf("status workspace: %v", err)
	}
	after, err := workspace.Queries.Tickets.ListTickets(ctx, contracts.TicketListOptions{})
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	if len(before) != len(after) {
		t.Fatalf("status/context must not create tickets: before=%d after=%d", len(before), len(after))
	}

	ctxPayload := mustPayload[contextPayload](t, contextResult)
	if ctxPayload.WorkspaceID != "ws-managed-test" {
		t.Fatalf("workspace id = %q", ctxPayload.WorkspaceID)
	}
	if ctxPayload.WorkspaceRoot != "" {
		t.Fatal("context leaked an absolute workspace path")
	}
	if ctxPayload.ManagedMode.DeclaredMode != contracts.ManagedModeDelivery || ctxPayload.ManagedMode.EffectiveMode != contracts.ManagedModeManaged {
		t.Fatalf("declared/effective = %s/%s", ctxPayload.ManagedMode.DeclaredMode, ctxPayload.ManagedMode.EffectiveMode)
	}
	if ctxPayload.ManagedMode.CompletionMode != contracts.CompletionModeReviewGate {
		t.Fatalf("completion = %s", ctxPayload.ManagedMode.CompletionMode)
	}
	if ctxPayload.Actor != "agent:builder-1" {
		t.Fatalf("actor = %s", ctxPayload.Actor)
	}
	if !strings.Contains(ctxPayload.Markdown, "ws-managed-test") || !strings.Contains(ctxPayload.Markdown, "agent:builder-1") {
		t.Fatalf("markdown not derived from structured context:\n%s", ctxPayload.Markdown)
	}
	if strings.Contains(ctxPayload.Markdown, root) {
		t.Fatalf("markdown leaked workspace root:\n%s", ctxPayload.Markdown)
	}

	appStatus := mustPayload[statusPayload](t, statusAPP)
	if appStatus.Project != "APP" || appStatus.UnknownProject {
		t.Fatalf("APP status: %#v", appStatus)
	}
	if strings.Contains(appStatus.Markdown, "OPS") && strings.Contains(appStatus.Markdown, "# Board OPS") {
		t.Fatal("status of APP returned the OPS board")
	}
	if appStatus.MCPApp == nil || !boardAppPassesCSP(appStatus.MCPApp) {
		t.Fatalf("project status should include a CSP-safe MCP App: %#v", appStatus.MCPApp)
	}
	if !strings.Contains(appStatus.Markdown, appStatus.Board.Title) {
		t.Fatalf("status markdown must come from structured board:\n%s", appStatus.Markdown)
	}

	unknown := mustPayload[statusPayload](t, statusUnknown)
	if !unknown.UnknownProject || !containsString(unknown.Disambiguation, "APP") || !containsString(unknown.Disambiguation, "OPS") {
		t.Fatalf("unknown project should disambiguate, got %#v", unknown)
	}
	if strings.Contains(unknown.Markdown, "# Board APP") {
		t.Fatal("unknown project must not return another project's board")
	}

	workspaceStatus := mustPayload[statusPayload](t, statusWorkspace)
	if workspaceStatus.Scope != "workspace" || workspaceStatus.Project != "" {
		t.Fatalf("multiple projects must stay a workspace overview: %#v", workspaceStatus)
	}

	workflow := NewServer(workspace, Options{Profile: ProfileWorkflow, Now: func() time.Time { return now }}.Normalized())
	created, err := workflow.CallTool(ctx, "atlas.ticket.create", map[string]any{
		"project": "APP", "title": "Build it", "type": "task", "status": "ready",
		"actor": "human:owner", "reason": "seed", "assignee": "agent:builder-1",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	ticketID := payloadTicketID(t, created)
	fresh, err := server.CallTool(ctx, "atlas.status", map[string]any{"project": "APP", "actor": "agent:builder-1"})
	if err != nil {
		t.Fatalf("fresh status: %v", err)
	}
	ready := mustPayload[statusPayload](t, fresh)
	found := false
	for _, item := range ready.Ready {
		if item.ID == ticketID {
			found = true
		}
	}
	if !found {
		t.Fatalf("fresh status missed %s: %#v", ticketID, ready.Ready)
	}
	if strings.Contains(ready.Markdown, "%") && strings.Contains(strings.ToLower(ready.Markdown), "percent") {
		t.Fatalf("invented progress percentage:\n%s", ready.Markdown)
	}

	delivery := NewServer(workspace, Options{Profile: ProfileDelivery, Now: func() time.Time { return now }}.Normalized())
	delivered, err := delivery.CallTool(ctx, "atlas.context", map[string]any{})
	if err != nil {
		t.Fatalf("delivery context: %v", err)
	}
	if mustPayload[contextPayload](t, delivered).ManagedMode.EffectiveMode != contracts.ManagedModeDelivery {
		t.Fatal("delivery-profile server should enable declared delivery")
	}
}

func TestContextMissingActorIsVisibleSetupError(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 11, 16, 30, 0, 0, time.UTC)
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	workspace, err := OpenWorkspace(root, nil, func() time.Time { return now })
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer workspace.Close()
	t.Setenv("TRACKER_ACTOR", "")
	server := NewServer(workspace, Options{Profile: ProfileRead, Now: func() time.Time { return now }}.Normalized())
	result, err := server.CallTool(context.Background(), "atlas.context", map[string]any{})
	if err != nil {
		t.Fatalf("context should return a visible setup error, not fail: %v", err)
	}
	payload := mustPayload[contextPayload](t, result)
	if payload.ActorConfigured || len(payload.SetupErrors) == 0 {
		t.Fatalf("expected actor setup error, got %#v", payload)
	}
	if !strings.Contains(strings.Join(payload.SetupErrors, "\n"), "configured Atlas actor is missing") {
		t.Fatalf("setup error = %v", payload.SetupErrors)
	}
}

func TestBoardToolIncludesSharedPresentation(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 11, 17, 0, 0, 0, time.UTC)
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	workspace, err := OpenWorkspace(root, nil, func() time.Time { return now })
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer workspace.Close()
	ctx := context.Background()
	if err := workspace.Actions.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create: %v", err)
	}
	server := NewServer(workspace, Options{Profile: ProfileRead, Now: func() time.Time { return now }}.Normalized())
	result, err := server.CallTool(ctx, "atlas.board", map[string]any{"project": "APP"})
	if err != nil {
		t.Fatalf("board: %v", err)
	}
	inner, _ := result["payload"].(map[string]any)
	if inner == nil {
		t.Fatalf("board payload: %#v", result["payload"])
	}
	md, _ := inner["markdown"].(string)
	if !strings.Contains(md, "# Board APP") {
		t.Fatalf("board markdown = %q", md)
	}
	if strings.ContainsRune(md, 0x1b) {
		t.Fatalf("atlas.board markdown must not include ANSI: %q", md)
	}
	if inner["board_url"] != "/board?project=APP" {
		t.Fatalf("board url = %#v", inner["board_url"])
	}
}

func TestBoardAndStatusChatPresentation(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 16, 18, 0, 0, 0, time.UTC)
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	workspace, err := OpenWorkspace(root, nil, func() time.Time { return now })
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer workspace.Close()
	ctx := context.Background()
	if err := workspace.Actions.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create: %v", err)
	}
	workflow := NewServer(workspace, Options{Profile: ProfileWorkflow, Now: func() time.Time { return now }}.Normalized())
	if _, err := workflow.CallTool(ctx, "atlas.ticket.create", map[string]any{
		"project": "APP", "title": "Ship first feature", "type": "task", "status": "ready",
		"actor": "human:owner", "reason": "seed",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	server := NewServer(workspace, Options{Profile: ProfileRead, Now: func() time.Time { return now }}.Normalized())
	board, err := server.CallTool(ctx, "atlas.board", map[string]any{"project": "APP", "format": "chat"})
	if err != nil {
		t.Fatalf("board chat: %v", err)
	}
	inner, _ := board["payload"].(map[string]any)
	chat, _ := inner["chat"].(string)
	if !strings.Contains(chat, "```ansi") || !strings.Contains(chat, "APP-1") || !strings.ContainsRune(chat, 0x1b) {
		t.Fatalf("board chat payload:\n%s", chat)
	}
	md, _ := inner["markdown"].(string)
	if strings.ContainsRune(md, 0x1b) {
		t.Fatalf("markdown must stay ANSI-free when format=chat: %q", md)
	}
	status, err := server.CallTool(ctx, "atlas.status", map[string]any{"project": "APP", "format": "chat"})
	if err != nil {
		t.Fatalf("status chat: %v", err)
	}
	payload := mustPayload[statusPayload](t, status)
	if payload.Presentation != "chat" || !strings.Contains(payload.Chat, "```ansi") || !strings.Contains(payload.Chat, "APP-1") {
		t.Fatalf("status chat: %#v", payload)
	}
	if strings.ContainsRune(payload.Markdown, 0x1b) {
		t.Fatal("status markdown must stay ANSI-free")
	}
	text := textFallback("atlas.status", status, false, 400)
	if strings.TrimSpace(text) != strings.TrimSpace(payload.Chat) {
		t.Fatalf("MCP text fallback should be the chat field, got:\n%s", text)
	}
	if _, err := server.CallTool(ctx, "atlas.board", map[string]any{"project": "APP", "format": "neon"}); err == nil {
		t.Fatal("invalid format should fail")
	}
}

func TestReadInventoryIncludesContextAndStatus(t *testing.T) {
	read := Inventory(Options{Profile: ProfileRead}.Normalized())
	if !toolEnabled(read, "atlas.context") || !toolEnabled(read, "atlas.status") || !toolEnabled(read, "atlas.backup.status") {
		t.Fatal("read profile must expose atlas.context, atlas.status, and atlas.backup.status")
	}
	extraRead, extraWorkflow := 0, 0
	for _, spec := range extraWorkspaceSpecs() {
		switch spec.Class {
		case ClassRead:
			extraRead++
		case ClassWorkflow:
			extraWorkflow++
		}
	}
	if countEnabled(read) != 44+extraRead {
		t.Fatalf("read profile count = %d, want %d", countEnabled(read), 44+extraRead)
	}
	workflow := Inventory(Options{Profile: ProfileWorkflow}.Normalized())
	if countEnabled(workflow) != 76+extraRead+extraWorkflow {
		t.Fatalf("workflow profile count = %d, want %d", countEnabled(workflow), 76+extraRead+extraWorkflow)
	}
	delivery := Inventory(Options{Profile: ProfileDelivery}.Normalized())
	if countEnabled(delivery) != 80+extraRead+extraWorkflow {
		t.Fatalf("delivery profile count = %d, want %d", countEnabled(delivery), 80+extraRead+extraWorkflow)
	}
	admin := Inventory(Options{Profile: ProfileAdmin}.Normalized())
	if countEnabled(admin) != 80+extraRead+extraWorkflow {
		t.Fatalf("admin profile count = %d, want %d", countEnabled(admin), 80+extraRead+extraWorkflow)
	}
	workflowEnabled := Inventory(Options{Profile: ProfileWorkflow}.Normalized())
	if toolEnabled(workflowEnabled, "atlas.backup.configure") || toolEnabled(workflowEnabled, "atlas.restore.apply") {
		t.Fatal("workflow must not expose high-impact backup/restore tools")
	}
}

func TestBoardAppCSPHelper(t *testing.T) {
	doc := newBoardApp(render.NewCompactBoard("APP", map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusReady: {{ID: "APP-1", Title: "Ready", Status: contracts.StatusReady}},
	}, 20, nil))
	if !boardAppPassesCSP(doc) {
		t.Fatalf("generated app failed CSP: %#v", doc)
	}
	bad := *doc
	bad.HTML = `<script>alert(1)</script>`
	if boardAppPassesCSP(&bad) {
		t.Fatal("scripted app must fail CSP")
	}
}

func countEnabled(items []ToolInfo) int {
	n := 0
	for _, item := range items {
		if item.Enabled {
			n++
		}
	}
	return n
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func mustPayload[T any](t *testing.T, result map[string]any) T {
	t.Helper()
	inner := result["payload"]
	typed, ok := inner.(T)
	if !ok {
		t.Fatalf("payload type %T, want %T: %#v", inner, *new(T), result)
	}
	return typed
}
