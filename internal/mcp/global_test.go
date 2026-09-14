package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/myrrazor/atlas-tasker/internal/app"
	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func testGlobalMachine(t *testing.T, cwd string) Machine {
	t.Helper()
	machine := openTestGlobalMachine(t, cwd)
	disableHomeService(t, machine)
	return machine
}

func testGlobalMachineWithHomeService(t *testing.T, cwd string) Machine {
	t.Helper()
	return openTestGlobalMachine(t, cwd)
}

func openTestGlobalMachine(t *testing.T, cwd string) Machine {
	t.Helper()
	home := t.TempDir()
	a, err := app.Open(app.Options{
		Home:            home,
		StateDir:        filepath.Join(home, "state"),
		LookPath:        func(string) (string, error) { return "", os.ErrNotExist },
		CommandRunner:   app.SilentRunner{},
		SkipHostInstall: true,
		Process:         app.NoopSpawner{},
		WriteClientCfg:  false,
		Executable:      filepath.Join(home, "bin", "tracker"),
		Now:             func() time.Time { return time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("app.Open: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return AdaptApp(a, cwd)
}

func disableHomeService(t *testing.T, machine Machine) {
	t.Helper()
	enabled := false
	if _, err := machine.UpdateSettings(context.Background(), app.MachineSettingsPatch{
		Service: &app.ServiceSettings{
			Bind:      app.DefaultHomeBind,
			Port:      app.DefaultHomePort,
			Enabled:   enabled,
			AutoStart: false,
		},
	}); err != nil {
		t.Fatalf("disable home service: %v", err)
	}
}

func initRegisteredWorkspace(t *testing.T, machine Machine, root string) string {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
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
	if err != nil {
		t.Fatalf("init %s: %v", root, err)
	}
	if result.WorkspaceID == "" {
		t.Fatal("missing workspace id")
	}
	return result.WorkspaceID
}

func TestGlobalServeStartsOutsideInitializedCWD(t *testing.T) {
	outside := t.TempDir()
	machine := testGlobalMachine(t, outside)
	server := NewGlobalServer(machine, Options{Profile: ProfileWorkflow, Now: func() time.Time { return time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC) }})
	payload, err := server.CallTool(context.Background(), "atlas.workspace.list", map[string]any{})
	if err != nil {
		t.Fatalf("list from empty cwd: %v", err)
	}
	raw, _ := json.Marshal(payload)
	if strings.Contains(string(raw), outside) && !strings.Contains(string(raw), "workspace_id") {
		t.Fatalf("list leaked raw path without ids: %s", raw)
	}
}

func TestGlobalDuplicateTicketIDsStayIsolated(t *testing.T) {
	outside := t.TempDir()
	machine := testGlobalMachine(t, outside)
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	rootA := filepath.Join(outside, "alpha")
	rootB := filepath.Join(outside, "beta")
	idA := initRegisteredWorkspace(t, machine, rootA)
	idB := initRegisteredWorkspace(t, machine, rootB)
	server := NewGlobalServer(machine, Options{Profile: ProfileWorkflow, Now: func() time.Time { return now }, CWD: outside, Machine: machine})
	for _, id := range []string{idA, idB} {
		if _, err := server.CallTool(ctx, "atlas.project.create", map[string]any{"workspace_id": id, "key": "APP", "name": "App"}); err != nil {
			t.Fatalf("project %s: %v", id, err)
		}
	}
	create := func(workspaceID, title string) string {
		t.Helper()
		created, err := server.CallTool(ctx, "atlas.ticket.create", map[string]any{
			"workspace_id": workspaceID,
			"project":      "APP",
			"title":        title,
			"type":         "task",
			"actor":        "human:owner",
			"reason":       "seed",
		})
		if err != nil {
			t.Fatalf("create %s: %v", title, err)
		}
		return payloadTicketID(t, created)
	}
	ticketA := create(idA, "Alpha only")
	ticketB := create(idB, "Beta only")
	if ticketA != "APP-1" || ticketB != "APP-1" {
		t.Fatalf("expected the same ticket id APP-1 in both workspaces, got %q and %q", ticketA, ticketB)
	}
	viewA, err := server.CallTool(ctx, "atlas.ticket.view", map[string]any{"workspace_id": idA, "ticket_id": "APP-1"})
	if err != nil {
		t.Fatal(err)
	}
	viewB, err := server.CallTool(ctx, "atlas.ticket.view", map[string]any{"workspace_id": idB, "ticket_id": "APP-1"})
	if err != nil {
		t.Fatal(err)
	}
	rawA, _ := json.Marshal(viewA)
	rawB, _ := json.Marshal(viewB)
	if !strings.Contains(string(rawA), "Alpha only") || strings.Contains(string(rawA), "Beta only") {
		t.Fatalf("alpha view crossed: %s", rawA)
	}
	if !strings.Contains(string(rawB), "Beta only") || strings.Contains(string(rawB), "Alpha only") {
		t.Fatalf("beta view crossed: %s", rawB)
	}
}

type barrierMachine struct {
	Machine
	n       int
	mu      sync.Mutex
	waiters []chan struct{}
}

func (m *barrierMachine) Bind(ctx context.Context, id string) (*Workspace, error) {
	ready := make(chan struct{})
	m.mu.Lock()
	m.waiters = append(m.waiters, ready)
	if len(m.waiters) >= m.n {
		for _, waiter := range m.waiters {
			close(waiter)
		}
		m.waiters = nil
	}
	m.mu.Unlock()
	select {
	case <-ready:
	case <-time.After(8 * time.Second):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return m.Machine.Bind(ctx, id)
}

func TestGlobalConcurrentSameTicketIDsStayIsolated(t *testing.T) {
	outside := t.TempDir()
	inner := testGlobalMachine(t, outside)
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	rootA := filepath.Join(outside, "alpha")
	rootB := filepath.Join(outside, "beta")
	idA := initRegisteredWorkspace(t, inner, rootA)
	idB := initRegisteredWorkspace(t, inner, rootB)
	setup := NewGlobalServer(inner, Options{Profile: ProfileWorkflow, Now: func() time.Time { return now }, CWD: outside, Machine: inner})
	for _, id := range []string{idA, idB} {
		if _, err := setup.CallTool(ctx, "atlas.project.create", map[string]any{"workspace_id": id, "key": "APP", "name": "App"}); err != nil {
			t.Fatalf("project: %v", err)
		}
		created, err := setup.CallTool(ctx, "atlas.ticket.create", map[string]any{
			"workspace_id": id, "project": "APP", "title": "shared id", "type": "task",
			"actor": "human:owner", "reason": "seed same id",
		})
		if err != nil {
			t.Fatalf("seed ticket: %v", err)
		}
		if got := payloadTicketID(t, created); got != "APP-1" {
			t.Fatalf("expected APP-1, got %q", got)
		}
	}

	gated := &barrierMachine{Machine: inner, n: 2}
	server := NewGlobalServer(gated, Options{Profile: ProfileWorkflow, Now: func() time.Time { return now }, CWD: outside, Machine: gated})
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

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	comment := func(workspaceID, body string) {
		defer wg.Done()
		_, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
			Name: "atlas.ticket.comment",
			Arguments: map[string]any{
				"workspace_id": workspaceID,
				"ticket_id":    "APP-1",
				"body":         body,
				"actor":        "human:owner",
				"reason":       "concurrent isolate",
			},
		})
		errs <- err
	}
	wg.Add(2)
	go comment(idA, "alpha-only-comment")
	go comment(idB, "beta-only-comment")
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent comment: %v", err)
		}
	}

	if _, err := os.Stat(storage.TicketFile(rootA, "APP", "APP-1")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(storage.TicketFile(rootB, "APP", "APP-1")); err != nil {
		t.Fatal(err)
	}
	alphaAll := readWorkspaceBytes(t, rootA)
	betaAll := readWorkspaceBytes(t, rootB)
	if strings.Contains(alphaAll, "beta-only-comment") {
		t.Fatal("alpha canonical files/events contain beta comment")
	}
	if strings.Contains(betaAll, "alpha-only-comment") {
		t.Fatal("beta canonical files/events contain alpha comment")
	}
	viewA, err := setup.CallTool(ctx, "atlas.ticket.history", map[string]any{"workspace_id": idA, "ticket_id": "APP-1"})
	if err != nil {
		t.Fatal(err)
	}
	viewB, err := setup.CallTool(ctx, "atlas.ticket.history", map[string]any{"workspace_id": idB, "ticket_id": "APP-1"})
	if err != nil {
		t.Fatal(err)
	}
	rawA, _ := json.Marshal(viewA)
	rawB, _ := json.Marshal(viewB)
	if !strings.Contains(string(rawA), "alpha-only-comment") || strings.Contains(string(rawA), "beta-only-comment") {
		t.Fatalf("alpha events crossed: %s", rawA)
	}
	if !strings.Contains(string(rawB), "beta-only-comment") || strings.Contains(string(rawB), "alpha-only-comment") {
		t.Fatalf("beta events crossed: %s", rawB)
	}
}

func TestGlobalAmbiguousScopeRejectedAndCWDReadAllowed(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "repo")
	machine := testGlobalMachine(t, home)
	id := initRegisteredWorkspace(t, machine, root)
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	outside := NewGlobalServer(machine, Options{Profile: ProfileRead, Now: func() time.Time { return now }, CWD: home, Machine: machine})
	if _, err := outside.CallTool(context.Background(), "atlas.project.list", map[string]any{}); err == nil {
		t.Fatal("empty cwd must require workspace_id")
	}
	cwdMachine := AdaptApp(machine.(*appMachine).app, root)
	server := NewGlobalServer(cwdMachine, Options{Profile: ProfileRead, Now: func() time.Time { return now }, CWD: root, Machine: cwdMachine})
	if _, err := server.CallTool(context.Background(), "atlas.project.list", map[string]any{}); err != nil {
		t.Fatalf("unambiguous cwd read: %v", err)
	}
	writeServer := NewGlobalServer(cwdMachine, Options{Profile: ProfileWorkflow, Now: func() time.Time { return now }, CWD: root, Machine: cwdMachine})
	if _, err := writeServer.CallTool(context.Background(), "atlas.project.update", map[string]any{
		"workspace_id": id,
		"key":          app.DefaultProjectKey("repo"),
		"name":         "Renamed",
		"actor":        "human:owner",
		"reason":       "rename",
	}); err != nil {
		t.Fatalf("explicit workspace write: %v", err)
	}
}

func TestGlobalMovedWorkspaceRejected(t *testing.T) {
	outside := t.TempDir()
	machine := testGlobalMachine(t, outside)
	root := filepath.Join(outside, "moved")
	id := initRegisteredWorkspace(t, machine, root)
	if err := os.Rename(root, root+"-gone"); err != nil {
		t.Fatal(err)
	}
	server := NewGlobalServer(machine, Options{Profile: ProfileWorkflow, CWD: outside, Machine: machine})
	_, err := server.CallTool(context.Background(), "atlas.project.list", map[string]any{"workspace_id": id})
	if err == nil {
		t.Fatal("moved workspace must be refused")
	}
	if apperr.CodeOf(err) != apperr.CodeRepairNeeded && apperr.CodeOf(err) != apperr.CodeNotFound && apperr.CodeOf(err) != apperr.CodeInvalidInput {
		t.Fatalf("unexpected code %s: %v", apperr.CodeOf(err), err)
	}
}

func TestGlobalCopiedWorkspaceRejected(t *testing.T) {
	outside := t.TempDir()
	machine := testGlobalMachine(t, outside)
	src := filepath.Join(outside, "src")
	id := initRegisteredWorkspace(t, machine, src)
	dst := filepath.Join(outside, "copy")
	if err := copyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	server := NewGlobalServer(machine, Options{Profile: ProfileWorkflow, CWD: dst, Machine: machine})
	_, err := server.CallTool(context.Background(), "atlas.ticket.create", map[string]any{
		"project": "SRC",
		"title":   "from copy",
		"type":    "task",
		"actor":   "human:owner",
		"reason":  "copy write",
	})
	if err == nil {
		t.Fatal("copied cwd with original registration must not write silently")
	}
	_ = id
}

func TestGlobalSettingsCannotWidenDiscoveryThroughWorkflow(t *testing.T) {
	outside := t.TempDir()
	machine := testGlobalMachine(t, outside)
	server := NewGlobalServer(machine, Options{Profile: ProfileWorkflow, CWD: outside, Machine: machine})
	_, err := server.CallTool(context.Background(), "atlas.settings.update", map[string]any{
		"actor":  "human:owner",
		"reason": "widen",
	})
	if err != nil {
		t.Fatalf("safe settings update: %v", err)
	}
	if _, err := server.CallTool(context.Background(), "atlas.settings.grant_discovery", map[string]any{
		"root":   outside,
		"actor":  "human:owner",
		"reason": "widen",
	}); err == nil {
		t.Fatal("high-impact discovery grant must stay disabled without the danger flag")
	}
}

func TestGlobalUnknownSchemaAndActorBoundaries(t *testing.T) {
	outside := t.TempDir()
	machine := testGlobalMachine(t, outside)
	root := filepath.Join(outside, "repo")
	id := initRegisteredWorkspace(t, machine, root)
	server := NewGlobalServer(machine, Options{Profile: ProfileWorkflow, CWD: outside, Machine: machine})
	if _, err := server.CallTool(context.Background(), "atlas.ticket.create", map[string]any{
		"workspace_id": id,
		"project":      app.DefaultProjectKey("repo"),
		"title":        "x",
		"type":         "task",
		"mystery":      true,
		"actor":        "human:owner",
		"reason":       "schema",
	}); err == nil {
		t.Fatal("unknown schema field must be rejected")
	}
	if _, err := server.CallTool(context.Background(), "atlas.ticket.create", map[string]any{
		"workspace_id": id,
		"project":      app.DefaultProjectKey("repo"),
		"title":        "x",
		"type":         "task",
		"reason":       "schema",
	}); err == nil {
		t.Fatal("missing actor must be rejected")
	}
}

func TestGlobalSDKInventoryMatchesSchemaAndResources(t *testing.T) {
	outside := t.TempDir()
	machine := testGlobalMachine(t, outside)
	options := Options{Profile: ProfileWorkflow, Global: true, Machine: machine, CWD: outside}.Normalized()
	schema := EnabledSchemas(options)
	names := map[string]bool{}
	for _, item := range schema {
		name, _ := item["name"].(string)
		names[name] = true
		if name == "atlas.ticket.create" {
			props, _ := item["inputSchema"].(map[string]any)["properties"].(map[string]any)
			if _, ok := props["workspace_id"]; !ok {
				t.Fatal("global ticket.create schema missing workspace_id")
			}
		}
		if name == "atlas.board" {
			if item["ui_resource_uri"] != BoardAppResourceURI {
				t.Fatalf("board schema missing ui resource: %#v", item)
			}
		}
	}
	if !names["atlas.workspace.list"] || !names["atlas.ticket.create"] {
		t.Fatalf("schema missing tools: %#v", names)
	}
	server := NewGlobalServer(machine, options)
	clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
	if _, err := server.SDKServer().Connect(context.Background(), serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "atlas-test", Version: "v0"}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	seen := map[string]bool{}
	var boardMeta mcpsdk.Meta
	for tool, err := range session.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		seen[tool.Name] = true
		if tool.Name == "atlas.board" {
			boardMeta = tool.Meta
		}
		if !names[tool.Name] {
			t.Fatalf("SDK listed %s not in CLI schema", tool.Name)
		}
	}
	if !seen["atlas.workspace.list"] || !seen["atlas.ticket.create"] {
		t.Fatalf("SDK missing tools: %#v", seen)
	}
	ui, _ := boardMeta["ui"].(map[string]any)
	if ui["resourceUri"] != BoardAppResourceURI {
		t.Fatalf("board tool metadata: %#v", boardMeta)
	}
	foundBoardApp := false
	for resource, err := range session.Resources(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		if resource.URI == BoardAppResourceURI {
			foundBoardApp = true
			if resource.MIMEType != BoardAppMIME {
				t.Fatalf("board app mime %q", resource.MIMEType)
			}
		}
	}
	if !foundBoardApp {
		t.Fatal("resources/list omitted ui://atlas/board")
	}
	foundTemplate := false
	for tmpl, err := range session.ResourceTemplates(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(tmpl.URITemplate, "{workspace_id}") {
			foundTemplate = true
		}
	}
	if !foundTemplate {
		t.Fatal("resources/templates/list omitted workspace templates")
	}
	got, err := session.ReadResource(context.Background(), &mcpsdk.ReadResourceParams{URI: BoardAppResourceURI})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Contents) == 0 || got.Contents[0].MIMEType != BoardAppMIME {
		t.Fatalf("resource read: %#v", got)
	}
	if !boardAppUsesSafeDOM(got.Contents[0].Text) {
		t.Fatal("board app HTML failed safe-DOM / handshake checks")
	}
	if !resourceHasUICSP(got.Contents[0].Meta) {
		t.Fatalf("resources/read content missing _meta.ui.csp: %#v", got.Contents[0].Meta)
	}
}

func TestResourceListChangedWhenProjectAdded(t *testing.T) {
	outside := t.TempDir()
	machine := testGlobalMachine(t, outside)
	root := filepath.Join(outside, "repo")
	id := initRegisteredWorkspace(t, machine, root)
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	server := NewGlobalServer(machine, Options{Profile: ProfileWorkflow, Now: func() time.Time { return now }, CWD: outside, Machine: machine})
	saw := make(chan struct{}, 1)
	clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
	if _, err := server.SDKServer().Connect(context.Background(), serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "atlas-test", Version: "v0"}, &mcpsdk.ClientOptions{
		ResourceListChangedHandler: func(context.Context, *mcpsdk.ResourceListChangedRequest) {
			select {
			case saw <- struct{}{}:
			default:
			}
		},
	})
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if _, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name: "atlas.project.create",
		Arguments: map[string]any{
			"workspace_id": id, "key": "ZZZ", "name": "Zed",
		},
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	select {
	case <-saw:
	case <-time.After(3 * time.Second):
		t.Fatal("expected resources/list_changed after project create")
	}
}

func TestGlobalResourceNotificationsCoalesce(t *testing.T) {
	hub := newResourceHub()
	hub.debounce = time.Hour
	hub.note(ResourceChange{URI: "atlas://attention", Event: "a", WorkspaceID: "w1"})
	hub.note(ResourceChange{URI: "atlas://attention", Event: "b", WorkspaceID: "w1"})
	hub.mu.Lock()
	pending := hub.pending["atlas://attention"]
	hub.mu.Unlock()
	if pending.Event != "a,b" && pending.Event != "b" && pending.Event != "a" {
		t.Fatalf("expected coalesced event, got %#v", pending)
	}
}

func TestReadOnlyProfileCannotInit(t *testing.T) {
	outside := t.TempDir()
	machine := testGlobalMachine(t, outside)
	server := NewGlobalServer(machine, Options{Profile: ProfileRead, CWD: outside, Machine: machine})
	if _, err := server.CallTool(context.Background(), "atlas.workspace.init", map[string]any{
		"actor": "human:owner", "reason": "nope",
	}); err == nil {
		t.Fatal("read profile must not init")
	}
}

func readWorkspaceBytes(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		b.Write(data)
		b.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

var _ = contracts.CurrentSchemaVersion
