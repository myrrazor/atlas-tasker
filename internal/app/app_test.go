package app

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

func testApp(t *testing.T) *App {
	t.Helper()
	home := t.TempDir()
	var running atomic.Bool
	spawner := &RecordingSpawner{OnStart: func() { running.Store(true) }}
	a, err := Open(Options{
		Home:            home,
		StateDir:        filepath.Join(home, "state"),
		LookPath:        func(string) (string, error) { return "", os.ErrNotExist },
		CommandRunner:   SilentRunner{},
		SkipHostInstall: true,
		Process:         spawner,
		WriteClientCfg:  true,
		Executable:      filepath.Join(home, "bin", "tracker"),
		Now:             func() time.Time { return time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	a.opts.Probe = LatchProber{Instance: a.Settings().InstanceID, Running: running.Load}
	t.Cleanup(func() { _ = a.Close() })
	return a
}

func TestInitCreatesDefaultProjectAndRegistry(t *testing.T) {
	a := testApp(t)
	root := filepath.Join(a.Home(), "src", "widgets")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := a.Init(context.Background(), InitOptions{
		Root:           root,
		Register:       true,
		Agents:         true,
		Backup:         true,
		DefaultProject: true,
		WriteClientCfg: true,
	})
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if result.WorkspaceID == "" || result.DefaultProject != "WIDGETS" {
		t.Fatalf("unexpected init result: %+v", result)
	}
	again, err := a.Init(context.Background(), InitOptions{
		Root: root, Register: true, DefaultProject: true, WriteClientCfg: true,
	})
	if err != nil {
		t.Fatalf("re-init: %v", err)
	}
	if !again.Already || again.DefaultProject != "" {
		t.Fatalf("second init should not recreate the default project: %+v", again)
	}
	listed, err := a.ListWorkspaces(context.Background(), ListOptions{})
	if err != nil || len(listed) != 1 || listed[0].Health != HealthAvailable {
		t.Fatalf("registry: %+v %v", listed, err)
	}
}

func TestInitDoesNotWriteHostMCPWhenLookPathEmpty(t *testing.T) {
	a := testApp(t)
	root := filepath.Join(a.Home(), "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Init(context.Background(), InitOptions{Root: root, Agents: true, WriteClientCfg: true, Register: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(a.Home(), ".cursor", "mcp.json")); !os.IsNotExist(err) {
		t.Fatal("no detected client should create cursor MCP config")
	}
}

func TestCursorUserJSONRegistrationIsIdempotentAcrossProjects(t *testing.T) {
	a := testApp(t)
	if err := os.MkdirAll(filepath.Join(a.Home(), ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	foreign := []byte("{\n  \"mcpServers\": {\n    \"other\": {\"command\": \"echo\"}\n  }\n}\n")
	path := filepath.Join(a.Home(), ".cursor", "mcp.json")
	if err := os.WriteFile(path, foreign, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one", "two"} {
		root := filepath.Join(a.Home(), name)
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := a.Init(context.Background(), InitOptions{Root: root, Register: true, Agents: true, WriteClientCfg: true}); err != nil {
			t.Fatalf("init %s: %v", name, err)
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, `"other"`) {
		t.Fatalf("unrelated MCP entry was removed:\n%s", body)
	}
	if strings.Count(body, `"atlas-tasker"`) != 1 {
		t.Fatalf("expected one global atlas-tasker entry:\n%s", body)
	}
	if !strings.Contains(body, `"--tool-profile"`) || !strings.Contains(body, `"workflow"`) {
		t.Fatalf("missing global MCP argv:\n%s", body)
	}
}

func TestPathGrantRequiredForBrowserInit(t *testing.T) {
	a := testApp(t)
	if _, err := a.ConsumeGrant(context.Background(), "nope"); err == nil {
		t.Fatal("missing grant must fail")
	}
	board := filepath.Join(a.Home(), "board")
	if err := os.MkdirAll(board, 0o755); err != nil {
		t.Fatal(err)
	}
	grant, err := a.GrantPath(context.Background(), board, PathGrantInit)
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.ConsumeGrant(context.Background(), grant.ID)
	if err != nil || got.Path != grant.Path {
		t.Fatalf("consume: %+v %v", got, err)
	}
	if _, err := a.ConsumeGrant(context.Background(), grant.ID); err == nil {
		t.Fatal("grant must be single-use")
	}
}

func TestDiscoveryStaysInsideConfiguredRoots(t *testing.T) {
	a := testApp(t)
	root := filepath.Join(a.Home(), "code", "one")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Init(context.Background(), InitOptions{Root: root, Register: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Discover(context.Background(), DiscoverOptions{}); err == nil {
		t.Fatal("discovery without roots must fail")
	}
	hits, err := a.Discover(context.Background(), DiscoverOptions{Roots: []string{filepath.Join(a.Home(), "code")}, MaxDepth: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].WorkspaceID == "" {
		t.Fatalf("hits=%+v", hits)
	}
}

func TestServiceIdentityConflict(t *testing.T) {
	a := testApp(t)
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	}))
	t.Cleanup(foreign.Close)
	_, portStr, _ := net.SplitHostPort(foreign.Listener.Addr().String())
	port, _ := strconv.Atoi(portStr)
	a.mu.Lock()
	a.settings.Service.Port = port
	a.mu.Unlock()
	a.opts.Probe = HTTPProber{Client: &http.Client{Timeout: time.Second}}
	_, err := a.EnsureService(context.Background(), ServiceOptions{})
	if err == nil || !strings.Contains(err.Error(), "occupied") {
		t.Fatalf("expected occupied conflict, got %v", err)
	}
}

func TestOpenWorkspaceAndCreateTicket(t *testing.T) {
	a := testApp(t)
	root := filepath.Join(a.Home(), "app")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Init(context.Background(), InitOptions{Root: root, Register: true, DefaultProject: true}); err != nil {
		t.Fatal(err)
	}
	ws, err := a.Hub().BindRoot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := service.WithEventMetadata(context.Background(), service.EventMetaContext{Surface: contracts.EventSurfaceCLI})
	ticket, err := ws.Actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       DefaultProjectKey("app"),
		Title:         "First card",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityMedium,
		CreatedAt:     time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, "human:owner", "seed")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if ticket.ID == "" {
		t.Fatal("missing ticket id")
	}
}

func TestNativeHostPlanIsRealUnit(t *testing.T) {
	home := t.TempDir()
	runner := &RecordingHostRunner{}
	host := NewNativeHostInstaller(home, runner)
	host.GOOS = "darwin"
	host.UnitDir = filepath.Join(home, "Library", "LaunchAgents")
	plan, err := host.Plan(ServiceUnit{
		Executable: "/usr/local/bin/tracker",
		Args:       []string{"serve"},
		Home:       home,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.Content, "<string>serve</string>") || !strings.Contains(plan.Content, HomeLaunchdLabel) || !strings.Contains(plan.Content, HomeServiceMarker) {
		t.Fatalf("plist missing serve/label/marker:\n%s", plan.Content)
	}
	linux := hostUnitContent("linux", ServiceUnit{Executable: "/usr/local/bin/tracker", Args: []string{"serve", "--state-dir", "/tmp/Atlas State"}, Home: "/tmp/Atlas Home"})
	if !strings.Contains(linux, HomeSystemdService[:0]+HomeServiceMarker) || !strings.Contains(linux, `"/tmp/Atlas State"`) {
		t.Fatalf("linux unit missing marker/quotes:\n%s", linux)
	}
	if err := host.Install(context.Background(), ServiceUnit{Executable: "/usr/local/bin/tracker", Args: []string{"serve"}, Home: home}); err != nil {
		t.Fatal(err)
	}
	if len(runner.Calls) == 0 {
		t.Fatal("install should invoke launchctl via the runner")
	}
	if _, err := os.Stat(plan.Path); err != nil {
		t.Fatal(err)
	}
}

func TestDoctorOutsideWorkspaceReportsMachine(t *testing.T) {
	a := testApp(t)
	report, err := a.Doctor(context.Background(), DoctorOptions{Workspace: a.Home()})
	if err != nil {
		t.Fatal(err)
	}
	if report.CurrentWorkspace != "" || !report.OK {
		t.Fatalf("%+v", report)
	}
}

func TestCopiedWorkspaceNeedsRepair(t *testing.T) {
	a := testApp(t)
	root := filepath.Join(a.Home(), "orig")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Init(context.Background(), InitOptions{Root: root, Register: true}); err != nil {
		t.Fatal(err)
	}
	listed, _ := a.ListWorkspaces(context.Background(), ListOptions{})
	id := listed[0].WorkspaceID
	copyRoot := filepath.Join(a.Home(), "copy")
	if err := os.MkdirAll(copyRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Init(context.Background(), InitOptions{Root: copyRoot, Register: false}); err != nil {
		t.Fatal(err)
	}
	// stamp the copy with the original id
	meta, _ := os.ReadFile(filepath.Join(root, ".tracker", "workspace.json"))
	if err := os.WriteFile(filepath.Join(copyRoot, ".tracker", "workspace.json"), meta, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := a.Register(context.Background(), RegisterOptions{Root: copyRoot})
	if err == nil {
		t.Fatal("copy with the same id must fail until fork/repair")
	}
	if _, err := a.Repair(context.Background(), RepairOptions{WorkspaceID: id, Action: RepairForkCopy}); err == nil {
		t.Fatal("fork_copy without NewPath must fail")
	}
	rep, err := a.Repair(context.Background(), RepairOptions{WorkspaceID: id, Action: RepairForkCopy, NewPath: copyRoot})
	if err != nil {
		t.Fatal(err)
	}
	orig, err := service.LoadWorkspaceIdentity(root)
	if err != nil || orig != id {
		t.Fatalf("original identity changed: %s %v", orig, err)
	}
	forked, err := service.LoadWorkspaceIdentity(copyRoot)
	if err != nil || forked == "" || forked == id {
		t.Fatalf("copy was not forked: %s %v", forked, err)
	}
	if rep.ForkedID != forked {
		t.Fatalf("repair forked id %s vs %s", rep.ForkedID, forked)
	}
}

func TestRegisterMovedAndReplacedRequireRepair(t *testing.T) {
	a := testApp(t)
	root := filepath.Join(a.Home(), "board")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Init(context.Background(), InitOptions{Root: root, Register: true}); err != nil {
		t.Fatal(err)
	}
	listed, _ := a.ListWorkspaces(context.Background(), ListOptions{})
	id := listed[0].WorkspaceID
	moved := filepath.Join(a.Home(), "moved")
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Register(context.Background(), RegisterOptions{Root: moved}); err == nil {
		t.Fatal("moved workspace must require explicit path repair")
	}
	if _, err := a.Repair(context.Background(), RepairOptions{WorkspaceID: id, Action: RepairUpdatePath, NewPath: moved}); err != nil {
		t.Fatal(err)
	}
}

func TestBindRootRejectsIdentityBoundToAnotherPath(t *testing.T) {
	a := testApp(t)
	root := filepath.Join(a.Home(), "alpha")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Init(context.Background(), InitOptions{Root: root, Register: true}); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(a.Home(), "beta")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Init(context.Background(), InitOptions{Root: other, Register: false}); err != nil {
		t.Fatal(err)
	}
	meta, _ := os.ReadFile(filepath.Join(root, ".tracker", "workspace.json"))
	if err := os.WriteFile(filepath.Join(other, ".tracker", "workspace.json"), meta, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Hub().BindRoot(context.Background(), other); err == nil {
		t.Fatal("BindRoot must not return a cached original for a copied path")
	}
}

func TestInitCreatesIsolatedCheckpoint(t *testing.T) {
	a := testApp(t)
	root := filepath.Join(a.Home(), "backed")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := a.Init(context.Background(), InitOptions{Root: root, Register: true, Backup: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Backup.Attempted {
		t.Fatal("backup was not attempted")
	}
	repo := filepath.Join(a.StateDir(), "backups", result.WorkspaceID, "repo.git")
	if _, err := os.Stat(repo); err != nil {
		t.Fatalf("isolated replica missing: %v (detail=%s)", err, result.Backup.Detail)
	}
	auto := filepath.Join(a.StateDir(), "backups", result.WorkspaceID, "auto.json")
	raw, err := os.ReadFile(auto)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"enabled": true`) {
		t.Fatalf("auto.json not enabled:\n%s", raw)
	}
	again, err := a.Init(context.Background(), InitOptions{Root: root, Register: true, Backup: true})
	if err != nil {
		t.Fatal(err)
	}
	raw2, _ := os.ReadFile(auto)
	if string(raw) != string(raw2) && !strings.Contains(string(raw2), `"enabled": true`) {
		t.Fatalf("re-init clobbered auto.json:\n%s", raw2)
	}
	_ = again
}

func TestClaudeUserScopeCLIRegistration(t *testing.T) {
	home := t.TempDir()
	runner := &RecordingRunner{}
	claude := filepath.Join(home, "bin", "claude")
	if err := os.MkdirAll(filepath.Dir(claude), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claude, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	a, err := Open(Options{
		Home:     home,
		StateDir: filepath.Join(home, "state"),
		LookPath: func(name string) (string, error) {
			if name == "claude" {
				return claude, nil
			}
			return "", os.ErrNotExist
		},
		CommandRunner:   runner,
		SkipHostInstall: true,
		WriteClientCfg:  true,
		Executable:      filepath.Join(home, "bin", "tracker"),
		Now:             func() time.Time { return time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	var running atomic.Bool
	a.opts.Process = &RecordingSpawner{OnStart: func() { running.Store(true) }}
	a.opts.Probe = LatchProber{Instance: a.Settings().InstanceID, Running: running.Load}
	t.Cleanup(func() { _ = a.Close() })
	root := filepath.Join(home, "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Init(context.Background(), InitOptions{Root: root, Agents: true, WriteClientCfg: true, Register: true}); err != nil {
		t.Fatal(err)
	}
	if len(runner.Commands) == 0 {
		t.Fatal("expected claude mcp add")
	}
	joined := strings.Join(runner.Commands[0].Args, " ")
	if !strings.Contains(joined, "mcp add") || !strings.Contains(joined, "--scope user") || strings.Contains(joined, "--scope local") {
		t.Fatalf("claude args: %v", runner.Commands[0].Args)
	}
	if strings.Contains(joined, "--workspace") {
		t.Fatalf("global registration must not pass --workspace: %v", runner.Commands[0].Args)
	}
	if strings.Contains(joined, GlobalMCPToolNameStyleFlag) {
		t.Fatalf("claude global registration must keep canonical names: %v", runner.Commands[0].Args)
	}
}

func TestGrokUserScopeCLIRegistrationUsesPortableToolNames(t *testing.T) {
	home := t.TempDir()
	runner := &RecordingRunner{}
	grok := filepath.Join(home, "bin", "grok")
	if err := os.MkdirAll(filepath.Dir(grok), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(grok, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	a, err := Open(Options{
		Home:     home,
		StateDir: filepath.Join(home, "state"),
		LookPath: func(name string) (string, error) {
			if name == "grok" {
				return grok, nil
			}
			return "", os.ErrNotExist
		},
		CommandRunner:   runner,
		SkipHostInstall: true,
		WriteClientCfg:  true,
		Executable:      filepath.Join(home, "bin", "tracker"),
		Now:             func() time.Time { return time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	var running atomic.Bool
	a.opts.Process = &RecordingSpawner{OnStart: func() { running.Store(true) }}
	a.opts.Probe = LatchProber{Instance: a.Settings().InstanceID, Running: running.Load}
	t.Cleanup(func() { _ = a.Close() })
	root := filepath.Join(home, "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Init(context.Background(), InitOptions{Root: root, Agents: true, WriteClientCfg: true, Register: true}); err != nil {
		t.Fatal(err)
	}
	if len(runner.Commands) == 0 {
		t.Fatal("expected grok mcp add")
	}
	joined := strings.Join(runner.Commands[0].Args, " ")
	if !strings.Contains(joined, "mcp add") || !strings.Contains(joined, "--scope user") {
		t.Fatalf("grok args: %v", runner.Commands[0].Args)
	}
	if !strings.Contains(joined, GlobalMCPToolNameStyleFlag) || !strings.Contains(joined, GlobalMCPToolNameStylePortable) {
		t.Fatalf("grok global registration missing portable tool names: %v", runner.Commands[0].Args)
	}
	if strings.Contains(joined, "--workspace") || strings.Contains(joined, "dangerously") {
		t.Fatalf("grok global registration leaked extra flags: %v", runner.Commands[0].Args)
	}
}

func TestGlobalMCPArgsForOnlyChangesGrok(t *testing.T) {
	canonical := strings.Join(GlobalMCPArgs(), " ")
	if strings.Contains(canonical, GlobalMCPToolNameStyleFlag) {
		t.Fatalf("default global argv leaked portable flag: %s", canonical)
	}
	grok := strings.Join(GlobalMCPArgsFor(integrations.TargetGrok), " ")
	if !strings.Contains(grok, GlobalMCPToolNameStyleFlag) || !strings.Contains(grok, GlobalMCPToolNameStylePortable) {
		t.Fatalf("grok argv: %s", grok)
	}
	for _, target := range []integrations.Target{integrations.TargetClaude, integrations.TargetCodex, integrations.TargetCursor, integrations.TargetOpenClaw, integrations.TargetGeneric} {
		got := strings.Join(GlobalMCPArgsFor(target), " ")
		if got != canonical {
			t.Fatalf("%s argv %s want %s", target, got, canonical)
		}
	}
}

func TestClaimIsSingleUseOnDisk(t *testing.T) {
	a := testApp(t)
	token, err := a.IssueClaim()
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ConsumeClaim(token); err != nil {
		t.Fatal(err)
	}
	if err := a.ConsumeClaim(token); err == nil {
		t.Fatal("replay must fail")
	}
}

func TestDefaultProjectKey(t *testing.T) {
	if got := DefaultProjectKey("widgets"); got != "WIDGETS" {
		t.Fatalf("got %s", got)
	}
	if got := DefaultProjectKey("12"); got != "P12" && got != "MAIN" {
		t.Fatalf("got %s", got)
	}
	if got := DefaultProjectKey("***"); got != "MAIN" {
		t.Fatalf("got %s", got)
	}
}
