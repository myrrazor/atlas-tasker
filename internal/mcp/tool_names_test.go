package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestPortableToolNamesAreUniqueAndGrokSafe(t *testing.T) {
	for _, global := range []bool{false, true} {
		opts := Options{Profile: ProfileAdmin, AllowHighImpactTools: true, Global: global, ToolNameStyle: ToolNameStylePortable}.Normalized()
		if err := ValidateAdvertisedToolNames(opts); err != nil {
			t.Fatalf("global=%v: %v", global, err)
		}
		seen := map[string]string{}
		for _, spec := range ToolSpecsFor(opts) {
			name := advertisedName(spec.Name, opts)
			if err := grokSafeAdvertisedName(name); err != nil {
				t.Fatalf("%s: %v", spec.Name, err)
			}
			if other, dup := seen[name]; dup {
				t.Fatalf("collision %q from %s and %s", name, other, spec.Name)
			}
			seen[name] = spec.Name
			if name == "atlas.status" || strings.Contains(name, ".") {
				t.Fatalf("portable catalog still advertised %q for %s", name, spec.Name)
			}
		}
		if seen[PortableToolName("atlas.status")] != "atlas.status" {
			t.Fatal("portable catalog missing atlas_status")
		}
		if seen[PortableToolName("atlas.board")] != "atlas.board" {
			t.Fatal("portable catalog missing atlas_board")
		}
		if global && seen[PortableToolName("atlas.workspace.list")] != "atlas.workspace.list" {
			t.Fatal("global portable catalog missing atlas_workspace_list")
		}
	}
}

func TestDefaultInventoryKeepsCanonicalDottedNames(t *testing.T) {
	items := Inventory(Options{Profile: ProfileRead}.Normalized())
	if !toolEnabled(items, "atlas.status") || !toolEnabled(items, "atlas.board") {
		t.Fatal("canonical inventory must advertise atlas.status and atlas.board")
	}
	if toolEnabled(items, "atlas_status") || toolEnabled(items, "atlas_board") {
		t.Fatal("canonical inventory must not advertise portable names")
	}
	for _, item := range items {
		if item.Enabled && !strings.Contains(item.Name, ".") {
			t.Fatalf("canonical enabled tool %q has no dot", item.Name)
		}
	}
}

func TestPortableInventoryHasNoDotsOrAmbiguousQualifiers(t *testing.T) {
	items := Inventory(Options{Profile: ProfileWorkflow, ToolNameStyle: ToolNameStylePortable}.Normalized())
	if !toolEnabled(items, "atlas_status") || !toolEnabled(items, "atlas_board") {
		t.Fatal("portable inventory must advertise atlas_status and atlas_board")
	}
	if toolEnabled(items, "atlas.status") || toolEnabled(items, "atlas.board") {
		t.Fatal("portable inventory must not advertise dotted names")
	}
	for _, item := range items {
		if !item.Enabled {
			continue
		}
		if err := grokSafeAdvertisedName(item.Name); err != nil {
			t.Fatalf("%s: %v", item.Name, err)
		}
	}
}

func TestSDKListsCanonicalNamesByDefault(t *testing.T) {
	seen := sdkListedTools(t, Options{Profile: ProfileRead}.Normalized())
	if seen["atlas.status"] == nil || seen["atlas.board"] == nil {
		t.Fatalf("default tools/list missing canonical names: %#v", seen)
	}
	if seen["atlas_status"] != nil || seen["atlas_board"] != nil {
		t.Fatalf("default tools/list leaked portable names: %#v", seen)
	}
	if got := seen["atlas.status"].Title; got != "atlas.status" {
		t.Fatalf("canonical atlas.status title drifted: %q", got)
	}
	if got := seen["atlas.board"].Title; got != "atlas.board" {
		t.Fatalf("canonical atlas.board title drifted: %q", got)
	}
}

func TestSDKPortableListAndDispatch(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 14, 18, 0, 0, 0, time.UTC)
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}}); err != nil {
		t.Fatal(err)
	}
	workspace, err := OpenWorkspace(root, nil, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Close()
	ctx := context.Background()
	if err := workspace.Actions.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project: "APP", Title: "Status over portable MCP", Type: contracts.TicketTypeTask,
		Status: contracts.StatusReady, Priority: contracts.PriorityMedium,
		CreatedAt: now, UpdatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed"); err != nil {
		t.Fatal(err)
	}

	opts := Options{Profile: ProfileRead, ToolNameStyle: ToolNameStylePortable, Now: func() time.Time { return now }}.Normalized()
	server := NewServer(workspace, opts)
	clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
	if _, err := server.SDKServer().Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("connect server: %v", err)
	}
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "atlas-test", Version: "v0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer session.Close()

	seen := map[string]*mcpsdk.Tool{}
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("list tool: %v", err)
		}
		seen[tool.Name] = tool
		if err := grokSafeAdvertisedName(tool.Name); err != nil {
			t.Fatalf("tools/list advertised %q: %v", tool.Name, err)
		}
	}
	if _, ok := seen["atlas.status"]; ok {
		t.Fatal("portable tools/list still advertised atlas.status")
	}
	statusTool, ok := seen["atlas_status"]
	if !ok {
		t.Fatal("portable tools/list missing atlas_status")
	}
	if statusTool.Title != "atlas_status" {
		t.Fatalf("portable atlas_status title=%q", statusTool.Title)
	}
	if !strings.Contains(statusTool.Description, "atlas.status") {
		t.Fatalf("portable description should mention canonical name: %q", statusTool.Description)
	}
	boardTool, ok := seen["atlas_board"]
	if !ok {
		t.Fatal("portable tools/list missing atlas_board")
	}
	if boardTool.Meta == nil {
		t.Fatal("portable atlas_board lost UI resource metadata")
	}

	status, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: "atlas_status", Arguments: map[string]any{"project": "APP"}})
	if err != nil {
		t.Fatalf("atlas_status: %v", err)
	}
	statusBody := structuredMap(t, status.StructuredContent)
	if statusBody["kind"] != "atlas.status" {
		t.Fatalf("portable status kind should stay canonical, got %#v", statusBody["kind"])
	}
	payload, _ := statusBody["payload"].(map[string]any)
	if payload == nil {
		t.Fatalf("atlas_status missing payload: %#v", statusBody)
	}
	if payload["project"] != "APP" && payload["scope"] != "project" {
		markdown, _ := payload["markdown"].(string)
		if !strings.Contains(markdown, "APP") {
			t.Fatalf("atlas_status did not dispatch to status for APP: %#v", payload)
		}
	}

	board, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: "atlas_board", Arguments: map[string]any{"project": "APP"}})
	if err != nil {
		t.Fatalf("atlas_board: %v", err)
	}
	boardBody := structuredMap(t, board.StructuredContent)
	if boardBody["kind"] != "atlas.board" {
		t.Fatalf("portable board kind should stay canonical, got %#v", boardBody["kind"])
	}
	boardPayload, _ := boardBody["payload"].(map[string]any)
	if boardPayload == nil {
		t.Fatalf("atlas_board missing payload: %#v", boardBody)
	}
	markdown, _ := boardPayload["markdown"].(string)
	if markdown == "" && boardPayload["board"] == nil {
		t.Fatalf("atlas_board did not dispatch to the board handler: %#v", boardPayload)
	}

	if _, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: "atlas.status", Arguments: map[string]any{"project": "APP"}}); err == nil {
		t.Fatal("SDK tools/call must refuse unlisted canonical names in portable mode")
	}
	if _, err := server.CallTool(ctx, "not_a_tool", nil); err == nil || !strings.Contains(err.Error(), "unknown MCP tool") {
		t.Fatalf("unknown tool must be refused, got %v", err)
	}
	if _, err := server.CallTool(ctx, "atlas.status.nope", nil); err == nil {
		t.Fatal("unknown dotted name must be refused")
	}
}

func TestCanonicalCallToolRejectsPortableNames(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 14, 18, 0, 0, 0, time.UTC)
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}}); err != nil {
		t.Fatal(err)
	}
	workspace, err := OpenWorkspace(root, nil, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Close()
	server := NewServer(workspace, Options{Profile: ProfileRead, Now: func() time.Time { return now }}.Normalized())
	if _, err := server.CallTool(context.Background(), "atlas_status", map[string]any{}); err == nil || !strings.Contains(err.Error(), "unknown MCP tool") {
		t.Fatalf("canonical mode must not alias atlas_status, got %v", err)
	}
}

func TestParseToolNameStyle(t *testing.T) {
	got, err := ParseToolNameStyle("")
	if err != nil || got != ToolNameStyleCanonical {
		t.Fatalf("empty style: %q %v", got, err)
	}
	got, err = ParseToolNameStyle(" portable ")
	if err != nil || got != ToolNameStylePortable {
		t.Fatalf("portable: %q %v", got, err)
	}
	if _, err := ParseToolNameStyle("dotted"); err == nil {
		t.Fatal("unknown style should fail")
	}
}

func sdkListedTools(t *testing.T, opts Options) map[string]*mcpsdk.Tool {
	t.Helper()
	root := t.TempDir()
	server := &Server{
		Workspace: &Workspace{Root: root},
		Options:   opts,
		Approvals: NewApprovalStore(root, nil),
	}
	clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
	if _, err := server.SDKServer().Connect(context.Background(), serverTransport, nil); err != nil {
		t.Fatalf("connect server: %v", err)
	}
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "atlas-test", Version: "v0"}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer session.Close()
	seen := map[string]*mcpsdk.Tool{}
	for tool, err := range session.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("list tool: %v", err)
		}
		seen[tool.Name] = tool
	}
	return seen
}

func structuredMap(t *testing.T, value any) map[string]any {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		return typed
	default:
		t.Fatalf("structured content is %T, want map", value)
		return nil
	}
}
