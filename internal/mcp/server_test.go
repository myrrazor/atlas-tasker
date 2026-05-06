package mcp

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
)

func TestInventoryProfilesGateHighImpactTools(t *testing.T) {
	read := Inventory(Options{Profile: ProfileRead}.Normalized())
	if !toolEnabled(read, "atlas.ticket.view") {
		t.Fatalf("expected read profile to enable atlas.ticket.view")
	}
	if toolEnabled(read, "atlas.ticket.comment") {
		t.Fatalf("read profile must not enable workflow writes")
	}
	if toolEnabled(read, "atlas.change.merge") {
		t.Fatalf("read profile must not enable high-impact writes")
	}

	admin := Inventory(Options{Profile: ProfileAdmin}.Normalized())
	if toolEnabled(admin, "atlas.change.merge") {
		t.Fatalf("admin profile without danger flag must not enable high-impact writes")
	}

	danger := Inventory(Options{Profile: ProfileAdmin, AllowHighImpactTools: true}.Normalized())
	if !toolEnabled(danger, "atlas.change.merge") {
		t.Fatalf("admin profile with danger flag should expose high-impact writes")
	}
	if !toolProviderSideEffect(danger, "atlas.import.preview") {
		t.Fatalf("import preview writes an import job and must be marked as a live side effect")
	}
}

func TestOperationApprovalIsBoundExpiringAndSingleUse(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	store := NewApprovalStore(root, func() time.Time { return now })
	approval, err := store.Create(context.Background(), "change.merge", "CHG-1", "human:owner", 10*time.Minute, "merge approved")
	if err != nil {
		t.Fatalf("create approval: %v", err)
	}
	if approval.Operation != "atlas.change.merge" {
		t.Fatalf("operation should be normalized, got %s", approval.Operation)
	}
	if _, err := store.Consume(context.Background(), approval.ID, "atlas.change.merge", "CHG-other", "human:owner", "atlas.change.merge"); err == nil {
		t.Fatalf("expected wrong target to be rejected")
	}
	if _, err := store.Consume(context.Background(), approval.ID, "atlas.change.merge", "CHG-1", "human:owner", "atlas.change.merge"); err != nil {
		t.Fatalf("consume approval: %v", err)
	}
	if _, err := store.Consume(context.Background(), approval.ID, "atlas.change.merge", "CHG-1", "human:owner", "atlas.change.merge"); err == nil {
		t.Fatalf("expected reused approval to be rejected")
	}
}

func TestOperationApprovalIDUses128BitRandomHex(t *testing.T) {
	store := NewApprovalStore(t.TempDir(), nil)
	approval, err := store.Create(context.Background(), "change.merge", "CHG-1", "human:owner", time.Minute, "approve merge")
	if err != nil {
		t.Fatalf("create approval: %v", err)
	}
	suffix := strings.TrimPrefix(approval.ID, "mcp_approval_")
	if suffix == approval.ID {
		t.Fatalf("approval id missing prefix: %s", approval.ID)
	}
	if len(suffix) != 32 {
		t.Fatalf("expected 128-bit approval id hex, got %q", suffix)
	}
	if _, err := hex.DecodeString(suffix); err != nil {
		t.Fatalf("approval id should be hex: %v", err)
	}
}

func TestOperationApprovalIDFailsClosedWhenRandomFails(t *testing.T) {
	original := approvalRandom
	approvalRandom = func([]byte) (int, error) {
		return 0, errors.New("entropy unavailable")
	}
	defer func() { approvalRandom = original }()

	store := NewApprovalStore(t.TempDir(), nil)
	if approval, err := store.Create(context.Background(), "change.merge", "CHG-1", "human:owner", time.Minute, "approve merge"); err == nil {
		t.Fatalf("expected entropy failure, got approval %#v", approval)
	}
	approvals, err := store.List()
	if err != nil {
		t.Fatalf("list approvals: %v", err)
	}
	if len(approvals) != 0 {
		t.Fatalf("entropy failure should not persist approvals, got %#v", approvals)
	}
}

func TestOperationApprovalExpiry(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	store := NewApprovalStore(root, func() time.Time { return now })
	approval, err := store.Create(context.Background(), "change.merge", "CHG-1", "human:owner", time.Minute, "merge approved")
	if err != nil {
		t.Fatalf("create approval: %v", err)
	}
	store.Now = func() time.Time { return now.Add(2 * time.Minute) }
	if _, err := store.Consume(context.Background(), approval.ID, "atlas.change.merge", "CHG-1", "human:owner", "atlas.change.merge"); err == nil {
		t.Fatalf("expected expired approval to be rejected")
	}
}

func TestHighImpactApprovalTargetBindsSideEffectingInputs(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	store := NewApprovalStore(root, func() time.Time { return now })
	server := &Server{
		Workspace: &Workspace{Root: root},
		Options:   Options{Profile: ProfileAdmin, AllowHighImpactTools: true, Now: func() time.Time { return now }}.Normalized(),
		Approvals: store,
	}
	spec, ok := ToolSpecByName("atlas.sync.pull")
	if !ok {
		t.Fatalf("missing sync pull tool")
	}
	args := map[string]any{
		"remote_id":             "origin",
		"source_workspace_id":   "other-workspace",
		"actor":                 "human:owner",
		"reason":                "pull approved state",
		"operation_approval_id": "placeholder",
		"confirm_text":          `execute atlas.sync.pull {"remote_id":"origin","source_workspace_id":"other-workspace"}`,
	}
	approval, err := store.Create(context.Background(), "atlas.sync.pull", "origin", "human:owner", 10*time.Minute, "too broad")
	if err != nil {
		t.Fatalf("create broad approval: %v", err)
	}
	args["operation_approval_id"] = approval.ID
	if _, err := server.authorizeHighImpact(context.Background(), spec, args, "human:owner", specTarget(spec, args)); err == nil {
		t.Fatalf("expected approval without source workspace binding to be rejected")
	}
	bound, err := store.Create(context.Background(), "atlas.sync.pull", `{"remote_id":"origin","source_workspace_id":"other-workspace"}`, "human:owner", 10*time.Minute, "exact pull")
	if err != nil {
		t.Fatalf("create exact approval: %v", err)
	}
	args["operation_approval_id"] = bound.ID
	if _, err := server.authorizeHighImpact(context.Background(), spec, args, "human:owner", specTarget(spec, args)); err != nil {
		t.Fatalf("expected exact approval to pass: %v", err)
	}
}

func TestHighImpactDeniedAttemptWritesSecurityAudit(t *testing.T) {
	root := t.TempDir()
	queries := service.NewQueryService(root, nil, nil, nil, nil, func() time.Time { return time.Now().UTC() })
	server := &Server{
		Workspace: &Workspace{Root: root, Queries: queries},
		Options:   Options{Profile: ProfileAdmin}.Normalized(),
		Approvals: NewApprovalStore(root, nil),
	}
	_, err := server.CallTool(context.Background(), "atlas.change.merge", map[string]any{
		"change_id": "CHG-1",
		"actor":     "human:owner",
		"reason":    "try merge",
	})
	if err == nil {
		t.Fatalf("expected high-impact tool to be disabled")
	}
	raw, readErr := os.ReadFile(filepath.Join(root, ".tracker", "runtime", "mcp", "security-audit.jsonl"))
	if readErr != nil {
		t.Fatalf("expected audit log: %v", readErr)
	}
	if !strings.Contains(string(raw), "atlas.change.merge") || !strings.Contains(string(raw), "high_impact_tools_disabled") {
		t.Fatalf("audit log missing useful denial data:\n%s", raw)
	}
}

func TestHighImpactDeniedAuditDoesNotLeakApprovalToken(t *testing.T) {
	root := t.TempDir()
	queries := service.NewQueryService(root, nil, nil, nil, nil, func() time.Time { return time.Now().UTC() })
	server := &Server{
		Workspace: &Workspace{Root: root, Queries: queries},
		Options:   Options{Profile: ProfileAdmin, AllowHighImpactTools: true}.Normalized(),
		Approvals: NewApprovalStore(root, nil),
	}
	secretApproval := "mcp_approval_super_secret"
	_, err := server.CallTool(context.Background(), "atlas.change.merge", map[string]any{
		"change_id":             "CHG-1",
		"actor":                 "human:owner",
		"reason":                "try merge",
		"operation_approval_id": secretApproval,
		"confirm_text":          "execute atlas.change.merge CHG-1",
	})
	if err == nil {
		t.Fatalf("expected missing approval to be rejected")
	}
	raw, readErr := os.ReadFile(filepath.Join(root, ".tracker", "runtime", "mcp", "security-audit.jsonl"))
	if readErr != nil {
		t.Fatalf("expected audit log: %v", readErr)
	}
	got := string(raw)
	if strings.Contains(got, secretApproval) {
		t.Fatalf("denied audit leaked approval token:\n%s", got)
	}
	if !strings.Contains(got, `"approval_id_provided":true`) {
		t.Fatalf("expected audit to record that an approval id was supplied:\n%s", got)
	}
}

func TestHighImpactHandlerFailureAfterApprovalWritesSecurityAudit(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	workspace, err := OpenWorkspace(root, nil, func() time.Time { return now })
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	defer workspace.Close()
	server := NewServer(workspace, Options{Profile: ProfileAdmin, AllowHighImpactTools: true, Now: func() time.Time { return now }}.Normalized())
	approval, err := server.Approvals.Create(context.Background(), "atlas.change.merge", "CHG-1", "human:owner", 10*time.Minute, "approve merge")
	if err != nil {
		t.Fatalf("create approval: %v", err)
	}

	_, err = server.CallTool(context.Background(), "atlas.change.merge", map[string]any{
		"change_id":             "CHG-1",
		"actor":                 "human:owner",
		"reason":                "merge approved change",
		"operation_approval_id": approval.ID,
		"confirm_text":          "execute atlas.change.merge CHG-1",
	})
	if err == nil {
		t.Fatalf("expected missing change to fail after approval consumption")
	}
	raw, readErr := os.ReadFile(filepath.Join(root, ".tracker", "runtime", "mcp", "security-audit.jsonl"))
	if readErr != nil {
		t.Fatalf("expected audit log: %v", readErr)
	}
	got := string(raw)
	if !strings.Contains(got, `"reason_code":"execution_failed"`) || !strings.Contains(got, approval.ID) {
		t.Fatalf("expected failed execution audit with approval id:\n%s", got)
	}
	used, err := server.Approvals.load(approval.ID)
	if err != nil {
		t.Fatalf("reload approval: %v", err)
	}
	if used.UsedAt.IsZero() {
		t.Fatalf("approval should stay consumed after handler failure")
	}
}

func TestWorkflowWritesRequireActorAndReason(t *testing.T) {
	root := t.TempDir()
	queries := service.NewQueryService(root, nil, nil, nil, nil, func() time.Time { return time.Now().UTC() })
	server := &Server{
		Workspace: &Workspace{Root: root, Queries: queries},
		Options:   Options{Profile: ProfileWorkflow}.Normalized(),
		Approvals: NewApprovalStore(root, nil),
	}
	_, err := server.CallTool(context.Background(), "atlas.ticket.comment", map[string]any{"ticket_id": "APP-1", "body": "hi"})
	if err == nil || !strings.Contains(err.Error(), "actor is required") {
		t.Fatalf("expected missing actor error, got %v", err)
	}
	_, err = server.CallTool(context.Background(), "atlas.ticket.comment", map[string]any{"ticket_id": "APP-1", "body": "hi", "actor": "human:owner"})
	if err == nil || !strings.Contains(err.Error(), "reason is required") {
		t.Fatalf("expected missing reason error, got %v", err)
	}
}

func TestToolArgumentsRejectUnknownAndWrongTypes(t *testing.T) {
	root := t.TempDir()
	server := &Server{
		Workspace: &Workspace{Root: root},
		Options:   Options{Profile: ProfileRead}.Normalized(),
		Approvals: NewApprovalStore(root, nil),
	}
	_, err := server.CallTool(context.Background(), "atlas.ticket.view", map[string]any{"ticket_id": "APP-1", "extra": "nope"})
	if err == nil || !strings.Contains(err.Error(), "unknown argument") {
		t.Fatalf("expected unknown argument error, got %v", err)
	}
	_, err = server.CallTool(context.Background(), "atlas.ticket.view", map[string]any{"ticket_id": []any{"APP-1"}})
	if err == nil || !strings.Contains(err.Error(), "ticket_id must be a string") {
		t.Fatalf("expected type error, got %v", err)
	}
}

func TestMutationRecordsMCPSurfaceMetadata(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	if err := os.MkdirAll(storage.EventsDir(root), 0o755); err != nil {
		t.Fatalf("mkdir events: %v", err)
	}
	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	if err := projectStore.CreateProject(context.Background(), contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := ticketStore.CreateTicket(context.Background(), contracts.TicketSnapshot{ID: "APP-1", Project: "APP", Title: "MCP ticket", Summary: "MCP ticket", Type: contracts.TicketTypeTask, Status: contracts.StatusReady, Priority: contracts.PriorityMedium, CreatedAt: now, UpdatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	workspace, err := OpenWorkspace(root, nil, func() time.Time { return now })
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	defer workspace.Close()
	server := NewServer(workspace, Options{Profile: ProfileWorkflow, Now: func() time.Time { return now }}.Normalized())
	if _, err := server.CallTool(context.Background(), "atlas.ticket.comment", map[string]any{"ticket_id": "APP-1", "body": "from MCP", "actor": "human:owner", "reason": "agent update"}); err != nil {
		t.Fatalf("comment via MCP: %v", err)
	}
	history, err := workspace.Queries.History(context.Background(), "APP-1")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history.Events) == 0 {
		t.Fatalf("expected comment event")
	}
	last := history.Events[len(history.Events)-1]
	if last.Metadata.Surface != contracts.EventSurfaceMCP {
		t.Fatalf("expected MCP event surface, got %#v", last.Metadata)
	}
	if last.Actor != contracts.Actor("human:owner") || last.Reason != "agent update" {
		t.Fatalf("expected actor/reason on event, got actor=%s reason=%q", last.Actor, last.Reason)
	}
}

func TestResultLimitsAndPagination(t *testing.T) {
	items := []string{"a", "b", "c", "d"}
	page := paginateSlice(items, map[string]any{"limit": 2}, 50, 50)
	if page.Total != 4 || page.NextCursor != "2" {
		t.Fatalf("unexpected page: %#v", page)
	}
	raw, _ := json.Marshal(map[string]any{"big": strings.Repeat("x", 200)})
	limited, truncated, err := applyResultLimits("atlas.test", time.Now(), json.RawMessage(raw), Options{MaxResultBytes: 80}.Normalized())
	if err != nil {
		t.Fatalf("apply limits: %v", err)
	}
	if !truncated {
		t.Fatalf("expected result to be truncated")
	}
	payload := limited["payload"].(map[string]any)
	if payload["truncated"] != true {
		t.Fatalf("expected truncated payload, got %#v", payload)
	}
	text := textFallback("atlas.test", map[string]any{"big": strings.Repeat("x", 1000)}, false, 50)
	if len(text) > 240 {
		t.Fatalf("expected text fallback to honor max token estimate, got length %d", len(text))
	}
	unicodeText := textFallback("atlas.test", map[string]any{"big": strings.Repeat("界", 300)}, false, 50)
	if !utf8.ValidString(unicodeText) {
		t.Fatalf("text fallback split a UTF-8 rune: %q", unicodeText)
	}
	truncatedPayload, _, err := applyResultLimits("atlas.test", time.Now(), map[string]any{"big": strings.Repeat("x", 200)}, Options{MaxResultBytes: 80}.Normalized())
	if err != nil {
		t.Fatalf("apply nested limits: %v", err)
	}
	if !resultPayloadTruncated(truncatedPayload) {
		t.Fatalf("expected nested payload truncation to be detected: %#v", truncatedPayload)
	}
	truncatedText := textFallback("atlas.test", truncatedPayload, resultPayloadTruncated(truncatedPayload), 50)
	if !strings.Contains(truncatedText, "truncated result") {
		t.Fatalf("expected truncated fallback prefix, got %q", truncatedText)
	}
}

func TestGroupedPaginationUsesIndependentCursors(t *testing.T) {
	ticket := func(id string) contracts.TicketSnapshot {
		return contracts.TicketSnapshot{ID: id, Project: "APP", Title: id, Type: contracts.TicketTypeTask, Status: contracts.StatusReady, Priority: contracts.PriorityMedium}
	}
	board := service.BoardViewModel{Board: contracts.BoardView{Columns: map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusReady:   {ticket("APP-1"), ticket("APP-2"), ticket("APP-3")},
		contracts.StatusBlocked: {ticket("APP-4")},
	}}}
	page := paginateBoard(board, map[string]any{"limit": 1, "cursor_by_status": map[string]any{"ready": "1"}}, 10)
	pagedBoard := page["board"].(service.BoardViewModel).Board
	if got := pagedBoard.Columns[contracts.StatusReady][0].ID; got != "APP-2" {
		t.Fatalf("expected ready cursor to advance ready column, got %s", got)
	}
	if got := pagedBoard.Columns[contracts.StatusBlocked][0].ID; got != "APP-4" {
		t.Fatalf("ready cursor should not empty blocked column, got %s", got)
	}

	dashboard := service.DashboardSummaryView{
		StaleWorktrees:          []string{"w1", "w2", "w3"},
		ProviderMappingWarnings: []string{"warn"},
	}
	dashboardPage := paginateDashboard(dashboard, map[string]any{"limit": 1, "cursor_by_section": map[string]any{"stale_worktrees": "1"}}, 10)
	pagedDashboard := dashboardPage["dashboard"].(service.DashboardSummaryView)
	if got := pagedDashboard.StaleWorktrees[0]; got != "w2" {
		t.Fatalf("expected stale worktree cursor to advance that section, got %s", got)
	}
	if got := pagedDashboard.ProviderMappingWarnings[0]; got != "warn" {
		t.Fatalf("section cursor should not empty provider warnings, got %s", got)
	}
}

func TestBundleImportPlanPropagatesDetailErrors(t *testing.T) {
	root := t.TempDir()
	queries := service.NewQueryService(root, nil, nil, nil, nil, func() time.Time { return time.Now().UTC() })
	server := &Server{
		Workspace: &Workspace{Root: root, Queries: queries},
		Options:   Options{Profile: ProfileRead}.Normalized(),
		Approvals: NewApprovalStore(root, nil),
	}
	_, err := server.CallTool(context.Background(), "atlas.bundle.import_plan", map[string]any{"bundle_ref": "missing-bundle"})
	if err == nil {
		t.Fatalf("expected bundle detail failure to propagate as a tool error")
	}
}

func TestMCPOutputRedactsObviousSecretFields(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	rawLocation := "https://user:secret@example.com/acme/repo.git?token=secret-token"
	if err := (service.SyncRemoteStore{Root: root}).SaveSyncRemote(ctx, contracts.SyncRemote{
		RemoteID:      "origin",
		Kind:          contracts.SyncRemoteKindGit,
		Location:      rawLocation,
		DefaultAction: contracts.SyncDefaultActionFetch,
		Enabled:       true,
	}); err != nil {
		t.Fatalf("seed sync remote: %v", err)
	}
	queries := service.NewQueryService(root, nil, nil, nil, nil, func() time.Time { return time.Now().UTC() })
	server := &Server{
		Workspace: &Workspace{Root: root, Queries: queries},
		Options:   Options{Profile: ProfileRead}.Normalized(),
		Approvals: NewApprovalStore(root, nil),
	}
	result, err := server.CallTool(ctx, "atlas.sync.status", map[string]any{"remote_id": "origin"})
	if err != nil {
		t.Fatalf("sync status via MCP: %v", err)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal MCP result: %v", err)
	}
	got := string(raw)
	for _, leaked := range []string{"secret", "secret-token", rawLocation} {
		if strings.Contains(got, leaked) {
			t.Fatalf("MCP output leaked sync remote secret %q:\n%s", leaked, got)
		}
	}
	if !strings.Contains(got, "***") && !strings.Contains(got, "%2A%2A%2A") {
		t.Fatalf("expected MCP output to include redaction marker:\n%s", got)
	}
}

func TestSDKServerListsOnlyEnabledTools(t *testing.T) {
	root := t.TempDir()
	queries := service.NewQueryService(root, nil, nil, nil, nil, func() time.Time { return time.Now().UTC() })
	server := &Server{
		Workspace: &Workspace{Root: root, Queries: queries},
		Options:   Options{Profile: ProfileRead}.Normalized(),
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
	seen := map[string]bool{}
	for tool, err := range session.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("list tool: %v", err)
		}
		seen[tool.Name] = true
	}
	if !seen["atlas.ticket.view"] {
		t.Fatalf("expected read tool to be listed")
	}
	if seen["atlas.ticket.comment"] || seen["atlas.change.merge"] {
		t.Fatalf("read profile leaked write/high-impact tools: %#v", seen)
	}
}

func toolEnabled(items []ToolInfo, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return item.Enabled
		}
	}
	return false
}

func toolProviderSideEffect(items []ToolInfo, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return item.ProviderSideEffect
		}
	}
	return false
}
