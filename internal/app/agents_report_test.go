package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
)

func rcIntegrationsDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func rcApp(t *testing.T, home string, lookPath func(string) (string, error), runner adapter.CommandRunner, executable string) *App {
	t.Helper()
	if runner == nil {
		runner = SilentRunner{}
	}
	if executable == "" {
		executable = filepath.Join(home, "bin", "tracker")
	}
	var running atomic.Bool
	a, err := Open(Options{
		Home:            home,
		StateDir:        filepath.Join(home, "state"),
		LookPath:        lookPath,
		CommandRunner:   runner,
		SkipHostInstall: true,
		Process:         &RecordingSpawner{OnStart: func() { running.Store(true) }},
		WriteClientCfg:  true,
		Executable:      executable,
		Now:             func() time.Time { return time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	a.opts.Probe = LatchProber{Instance: a.Settings().InstanceID, Running: running.Load}
	t.Cleanup(func() { _ = a.Close() })
	return a
}

func TestNilLookPathUsesPATHForCLIRegistration(t *testing.T) {
	root := rcIntegrationsDir(t)
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"grok", "claude", "openclaw"} {
		path := filepath.Join(bin, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "xdg-state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin"+string(os.PathListSeparator)+"/bin")

	runner := &RecordingRunner{}
	a := rcApp(t, home, nil, runner, filepath.Join(bin, "tracker"))
	ws := filepath.Join(root, "repo")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := a.Init(context.Background(), InitOptions{Root: ws, Register: true, Agents: true, WriteClientCfg: true})
	if err != nil && !IsPartial(err) {
		t.Fatal(err)
	}
	if !result.Agents.Attempted {
		t.Fatal("agent setup was not attempted")
	}

	found := map[integrations.Target]bool{}
	for _, client := range result.Agents.Clients {
		if client.Status == AgentNotDetected && strings.Contains(client.Detail, "not installed") {
			t.Fatalf("%s detected on PATH but registration said absent: %+v", client.Target, client)
		}
		found[client.Target] = true
	}
	got := map[string]bool{}
	for _, cmd := range runner.Commands {
		base := filepath.Base(cmd.Executable)
		got[base] = true
		if !strings.HasPrefix(cmd.Executable, bin) {
			t.Fatalf("PATH lookup used %q, want synthetic under %s", cmd.Executable, bin)
		}
		joined := strings.Join(cmd.Args, " ")
		if !strings.Contains(joined, "mcp add") {
			t.Fatalf("%s missing mcp add: %v", base, cmd.Args)
		}
	}
	for _, name := range []string{"grok", "claude", "openclaw"} {
		if !got[name] {
			t.Fatalf("expected %s mcp add via PATH lookup, commands=%v", name, runner.Commands)
		}
	}
	if !found[integrations.TargetGrok] {
		t.Fatal("grok client missing from init report")
	}
}

func TestListAgentClientsDistinguishesProjectGrokFromUserScope(t *testing.T) {
	root := rcIntegrationsDir(t)
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(root, "bin", "tracker")
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := rcApp(t, home, func(name string) (string, error) {
		if name == "tracker" {
			return executable, nil
		}
		return "", os.ErrNotExist
	}, SilentRunner{}, executable)

	ws := filepath.Join(root, "board")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	initResult, err := a.Init(context.Background(), InitOptions{Root: ws, Register: true, Agents: false})
	if err != nil && !IsPartial(err) {
		t.Fatal(err)
	}
	if initResult.WorkspaceID == "" {
		t.Fatal("missing workspace id")
	}

	name, err := adapter.ServerNameFor(initResult.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	args, err := expectedGrokProjectArgs(initResult.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(ws, ".grok"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".grok", "config.toml"), []byte(tomlMCPServer(name, "tracker", args)), 0o600); err != nil {
		t.Fatal(err)
	}

	decoy := filepath.Join(home, "unregistered", "repo")
	if err := os.MkdirAll(filepath.Join(decoy, ".grok"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(decoy, ".grok", "config.toml"), []byte(tomlMCPServer(name, "/usr/bin/other", []string{"mcp", "serve"})), 0o600); err != nil {
		t.Fatal(err)
	}

	cursorPath := filepath.Join(home, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(cursorPath), 0o755); err != nil {
		t.Fatal(err)
	}
	cursorBody, _ := json.Marshal(map[string]any{
		"mcpServers": map[string]any{
			GlobalMCPServerName: map[string]any{
				"type":    "stdio",
				"command": executable,
				"args":    GlobalMCPArgs(),
			},
		},
	})
	if err := os.WriteFile(cursorPath, append(cursorBody, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	report := a.ListAgentClients(context.Background())
	raw, _ := json.Marshal(report)
	if strings.Contains(string(raw), `"connected"`) || strings.Contains(string(raw), `"ready"`) || strings.Contains(string(raw), `"live"`) {
		t.Fatalf("must not fake live status: %s", raw)
	}

	var grokProject, grokUser, cursorUser *AgentClientReport
	for i := range report.Clients {
		client := report.Clients[i]
		switch {
		case client.Target == integrations.TargetGrok && client.Scope == AgentScopeProject:
			copy := client
			grokProject = &copy
		case client.Target == integrations.TargetGrok && client.Scope == AgentScopeUser:
			copy := client
			grokUser = &copy
		case client.Target == integrations.TargetCursor && client.Scope == AgentScopeUser:
			copy := client
			cursorUser = &copy
		}
	}
	if grokUser != nil {
		t.Fatalf("project-only Grok must not appear as a user entry: %+v", grokUser)
	}
	if grokProject == nil {
		t.Fatalf("missing project Grok row: %s", raw)
	}
	if grokProject.Status != AgentPendingClientRestart {
		t.Fatalf("project Grok status %q", grokProject.Status)
	}
	if grokProject.StatusLabel() != "configured · pending client restart" {
		t.Fatalf("status label %q", grokProject.StatusLabel())
	}
	if !strings.Contains(grokProject.ArgvLine(), "--tool-name-style portable") || !strings.Contains(grokProject.ArgvLine(), "--workspace-from-cwd") {
		t.Fatalf("actual argv missing portable grok flag: %s", grokProject.ArgvLine())
	}
	if grokProject.WorkspaceID != initResult.WorkspaceID || grokProject.WorkspaceRoot != filepath.Clean(ws) {
		t.Fatalf("workspace binding %+v", grokProject)
	}
	if cursorUser == nil || cursorUser.Status != AgentPendingClientRestart {
		t.Fatalf("user-scoped cursor should stay configured: %+v", cursorUser)
	}
	if strings.Contains(cursorUser.ArgvLine(), "--tool-name-style") {
		t.Fatalf("cursor must keep canonical argv: %s", cursorUser.ArgvLine())
	}

	wrong := append([]string(nil), args...)
	for i, arg := range wrong {
		if arg == "--max-items" && i+1 < len(wrong) {
			wrong[i+1] = "99"
		}
	}
	if err := os.WriteFile(filepath.Join(ws, ".grok", "config.toml"), []byte(tomlMCPServer(name, "tracker", wrong)), 0o600); err != nil {
		t.Fatal(err)
	}
	mismatch := a.ListAgentClients(context.Background())
	var mismatched *AgentClientReport
	for i := range mismatch.Clients {
		if mismatch.Clients[i].Target == integrations.TargetGrok {
			copy := mismatch.Clients[i]
			mismatched = &copy
		}
	}
	if mismatched == nil || mismatched.Scope != AgentScopeProject || mismatched.Status != AgentUnverified {
		t.Fatalf("mismatched project Grok should stay a project unverified row: %+v", mismatched)
	}
	if !strings.Contains(mismatched.ArgvLine(), "--max-items 99") {
		t.Fatalf("mismatch row should show actual argv: %s", mismatched.ArgvLine())
	}

	if err := os.Remove(filepath.Join(ws, ".grok", "config.toml")); err != nil {
		t.Fatal(err)
	}
	absent := a.ListAgentClients(context.Background())
	for _, client := range absent.Clients {
		if client.Target == integrations.TargetGrok {
			t.Fatalf("absent Grok must not be listed: %+v", client)
		}
	}
}

func TestListAgentClientsInspectsGrantedWorkspaceNotHomeCrawl(t *testing.T) {
	root := rcIntegrationsDir(t)
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(root, "bin", "tracker")
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	a := rcApp(t, home, func(string) (string, error) { return "", os.ErrNotExist }, SilentRunner{}, executable)
	granted := filepath.Join(root, "granted")
	if err := os.MkdirAll(filepath.Join(granted, ".tracker"), 0o755); err != nil {
		t.Fatal(err)
	}
	const wsID = "ws-granted-grok"
	if err := os.WriteFile(filepath.Join(granted, ".tracker", "workspace.json"), []byte(`{"workspace_id":"`+wsID+`"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	name, err := adapter.ServerNameFor(wsID)
	if err != nil {
		t.Fatal(err)
	}
	args, err := expectedGrokProjectArgs(wsID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(granted, ".grok"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(granted, ".grok", "config.toml"), []byte(tomlMCPServer(name, executable, args)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.GrantPath(context.Background(), granted, PathGrantRegister); err != nil {
		t.Fatal(err)
	}
	report := a.ListAgentClients(context.Background())
	found := false
	for _, client := range report.Clients {
		if client.Target == integrations.TargetGrok && client.Scope == AgentScopeProject && client.WorkspaceID == wsID {
			found = true
			if client.Status != AgentPendingClientRestart {
				t.Fatalf("granted project Grok: %+v", client)
			}
		}
	}
	if !found {
		t.Fatalf("granted workspace Grok missing: %+v", report.Clients)
	}
}

func TestListAgentClientsBareTrackerRequiresPATHLaunchability(t *testing.T) {
	root := rcIntegrationsDir(t)
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(root, "bin", "tracker")
	other := filepath.Join(root, "other", "tracker")
	for _, path := range []string{executable, other} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	setup := rcApp(t, home, func(string) (string, error) { return "", os.ErrNotExist }, SilentRunner{}, executable)
	ws := filepath.Join(root, "board")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	initResult, err := setup.Init(context.Background(), InitOptions{Root: ws, Register: true, Agents: false})
	if err != nil && !IsPartial(err) {
		t.Fatal(err)
	}
	name, err := adapter.ServerNameFor(initResult.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	args, err := expectedGrokProjectArgs(initResult.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(ws, ".grok"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".grok", "config.toml"), []byte(tomlMCPServer(name, "tracker", args)), 0o600); err != nil {
		t.Fatal(err)
	}

	open := func(look func(string) (string, error)) AgentSetupReport {
		t.Helper()
		a := rcApp(t, home, look, SilentRunner{}, executable)
		return a.ListAgentClients(context.Background())
	}
	project := func(report AgentSetupReport) *AgentClientReport {
		for i := range report.Clients {
			if report.Clients[i].Target == integrations.TargetGrok && report.Clients[i].Scope == AgentScopeProject {
				copy := report.Clients[i]
				return &copy
			}
		}
		return nil
	}

	missing := project(open(func(string) (string, error) { return "", os.ErrNotExist }))
	if missing == nil || missing.Status != AgentUnverified {
		t.Fatalf("bare tracker without PATH must not look configured: %+v", missing)
	}
	if !strings.Contains(missing.ArgvLine(), "tracker mcp serve") {
		t.Fatalf("missing-PATH row should keep actual argv: %s", missing.ArgvLine())
	}

	different := project(open(func(name string) (string, error) {
		if name == "tracker" {
			return other, nil
		}
		return "", os.ErrNotExist
	}))
	if different == nil || different.Status != AgentUnverified {
		t.Fatalf("bare tracker resolving to another binary must not look configured: %+v", different)
	}

	matched := project(open(func(name string) (string, error) {
		if name == "tracker" {
			return executable, nil
		}
		return "", os.ErrNotExist
	}))
	if matched == nil || matched.Status != AgentPendingClientRestart {
		t.Fatalf("PATH-matching tracker should be configured pending restart: %+v", matched)
	}
}

func TestListAgentClientsOmitsUnrelatedUserGrokKeepsProjectRow(t *testing.T) {
	root := rcIntegrationsDir(t)
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(root, "bin", "tracker")
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := rcApp(t, home, func(string) (string, error) { return "", os.ErrNotExist }, SilentRunner{}, executable)
	ws := filepath.Join(root, "board")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	initResult, err := a.Init(context.Background(), InitOptions{Root: ws, Register: true, Agents: false})
	if err != nil && !IsPartial(err) {
		t.Fatal(err)
	}
	name, err := adapter.ServerNameFor(initResult.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	args, err := expectedGrokProjectArgs(initResult.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(ws, ".grok"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".grok", "config.toml"), []byte(tomlMCPServer(name, executable, args)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".grok"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".grok", "config.toml"), []byte("[mcp_servers.notes]\ncommand = \"echo\"\nargs = [\"ok\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	report := a.ListAgentClients(context.Background())
	var grokProject, grokUser *AgentClientReport
	for i := range report.Clients {
		client := report.Clients[i]
		switch {
		case client.Target == integrations.TargetGrok && client.Scope == AgentScopeProject:
			copy := client
			grokProject = &copy
		case client.Target == integrations.TargetGrok && client.Scope == AgentScopeUser:
			copy := client
			grokUser = &copy
		}
	}
	if grokUser != nil {
		t.Fatalf("unrelated user Grok config must not appear as a broken Atlas row: %+v", grokUser)
	}
	if grokProject == nil || grokProject.Status != AgentPendingClientRestart {
		t.Fatalf("working project Grok row missing: %+v", grokProject)
	}

	if err := os.WriteFile(filepath.Join(home, ".grok", "config.toml"), []byte("this is not toml [["), 0o600); err != nil {
		t.Fatal(err)
	}
	malformed := a.ListAgentClients(context.Background())
	grokUser = nil
	grokProject = nil
	for i := range malformed.Clients {
		client := malformed.Clients[i]
		switch {
		case client.Target == integrations.TargetGrok && client.Scope == AgentScopeProject:
			copy := client
			grokProject = &copy
		case client.Target == integrations.TargetGrok && client.Scope == AgentScopeUser:
			copy := client
			grokUser = &copy
		}
	}
	if grokUser == nil || grokUser.Status != AgentUnverified {
		t.Fatalf("malformed user Grok config should stay unverified: %+v", grokUser)
	}
	if grokProject == nil || grokProject.Status != AgentPendingClientRestart {
		t.Fatalf("project Grok should survive malformed user config: %+v", grokProject)
	}

	if err := os.WriteFile(filepath.Join(home, ".grok", "config.toml"), []byte(tomlMCPServer(GlobalMCPServerName, executable, []string{"mcp", "serve"})), 0o600); err != nil {
		t.Fatal(err)
	}
	mismatch := a.ListAgentClients(context.Background())
	grokUser = nil
	for i := range mismatch.Clients {
		if mismatch.Clients[i].Target == integrations.TargetGrok && mismatch.Clients[i].Scope == AgentScopeUser {
			copy := mismatch.Clients[i]
			grokUser = &copy
		}
	}
	if grokUser == nil || grokUser.Status != AgentUnverified {
		t.Fatalf("present-but-mismatched user Atlas entry should stay unverified: %+v", grokUser)
	}
}

func tomlMCPServer(name, command string, args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = strconv.Quote(arg)
	}
	return "[mcp_servers." + name + "]\ncommand = " + strconv.Quote(command) + "\nargs = [" + strings.Join(quoted, ", ") + "]\n"
}
