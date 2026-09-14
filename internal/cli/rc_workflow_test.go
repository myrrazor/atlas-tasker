package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/app"
	atlasmcp "github.com/myrrazor/atlas-tasker/internal/mcp"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

func TestMCPToolsDefaultProfileIsWorkflow(t *testing.T) {
	withTempWorkspace(t)
	out, err := runCLI(t, "mcp", "tools", "--json")
	if err != nil {
		t.Fatalf("mcp tools: %v\n%s", err, out)
	}
	var payload struct {
		Profile string `json:"profile"`
		Tools   []struct {
			Name    string `json:"name"`
			Class   string `json:"class"`
			Enabled bool   `json:"enabled"`
		} `json:"tools"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if payload.Profile != "workflow" {
		t.Fatalf("default profile=%s", payload.Profile)
	}
	var createEnabled, mergeEnabled bool
	for _, tool := range payload.Tools {
		if tool.Name == "atlas.ticket.create" {
			createEnabled = tool.Enabled
		}
		if tool.Name == "atlas.change.merge" {
			mergeEnabled = tool.Enabled
		}
	}
	if !createEnabled {
		t.Fatal("default workflow profile should enable atlas.ticket.create")
	}
	if mergeEnabled {
		t.Fatal("default profile must not enable high-impact tools")
	}

	readOut, err := runCLI(t, "mcp", "tools", "--json", "--tool-profile", "read")
	if err != nil {
		t.Fatalf("explicit read: %v\n%s", err, readOut)
	}
	if err := json.Unmarshal([]byte(readOut), &payload); err != nil {
		t.Fatalf("parse read: %v", err)
	}
	if payload.Profile != "read" {
		t.Fatalf("explicit read profile=%s", payload.Profile)
	}
	for _, tool := range payload.Tools {
		if tool.Name == "atlas.ticket.create" && tool.Enabled {
			t.Fatal("explicit read must not enable workflow tools")
		}
	}

	roOut, err := runCLI(t, "mcp", "tools", "--json", "--read-only")
	if err != nil {
		t.Fatalf("read-only: %v\n%s", err, roOut)
	}
	if err := json.Unmarshal([]byte(roOut), &payload); err != nil {
		t.Fatalf("parse read-only: %v", err)
	}
	if payload.Profile != "read" {
		t.Fatalf("read-only profile=%s", payload.Profile)
	}
}

func TestParentHelpWorksOutsideWorkspace(t *testing.T) {
	withTempWorkspace(t)
	for _, args := range [][]string{{"ticket", "help"}, {"mcp", "help"}, {"agent", "help"}, {"bulk", "help"}, {"ticket", "help", "create"}} {
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		if !strings.Contains(out, "Usage:") {
			t.Fatalf("%v missing usage:\n%s", args, out)
		}
	}
	if _, err := os.Stat(".tracker"); !os.IsNotExist(err) {
		t.Fatalf("help must not scaffold a workspace: %v", err)
	}
	out, err := runCLI(t, "ticket", "help", "nosuch")
	if err == nil || !strings.Contains(out+err.Error(), "unknown command") {
		t.Fatalf("unknown help target must fail, err=%v out=%s", err, out)
	}
	out, err = runCLI(t, "ticket", "nosuch")
	if err == nil || !strings.Contains(out+err.Error(), "unknown command") {
		t.Fatalf("unknown subcommand must still fail, err=%v out=%s", err, out)
	}
}

func TestBoardOutsideWorkspaceMentionsHomeGrant(t *testing.T) {
	withTempWorkspace(t)
	out, err := runCLI(t, "board")
	if err == nil {
		t.Fatalf("expected refusal, got %s", out)
	}
	msg := out + err.Error()
	if !strings.Contains(msg, "tracker init") {
		t.Fatalf("missing init pointer: %s", msg)
	}
	if !strings.Contains(msg, "workspaces grant") || !strings.Contains(msg, "Atlas Home") {
		t.Fatalf("missing Home/grant pointer: %s", msg)
	}
	if _, err := os.Stat(".tracker"); !os.IsNotExist(err) {
		t.Fatalf("must not scaffold: %v", err)
	}
}

func TestMCPBootstrapCreatesDefaultProjectAndRegisters(t *testing.T) {
	withTempWorkspace(t)
	workspace := t.TempDir()
	cmd := bootstrapCommand(t, workspace, true)
	root, err := prepareMCPWorkspace(cmd, atlasmcp.Options{Profile: atlasmcp.ProfileWorkflow})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "projects"))
	if err != nil || len(entries) == 0 {
		t.Fatalf("expected a default project, entries=%v err=%v", entries, err)
	}
	assertRegistered(t, root)
	config := filepath.Join(root, ".tracker", "config.toml")
	beforeBytes, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepareMCPWorkspace(cmd, atlasmcp.Options{Profile: atlasmcp.ProfileWorkflow}); err != nil {
		t.Fatal(err)
	}
	afterBytes, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(config)
	if err != nil || before.ModTime() != after.ModTime() || string(beforeBytes) != string(afterBytes) {
		t.Fatalf("reopen rewrote initialized config: %v", err)
	}
	again, err := os.ReadDir(filepath.Join(root, "projects"))
	if err != nil || len(again) != len(entries) {
		t.Fatalf("second start changed projects: before=%d after=%d err=%v", len(entries), len(again), err)
	}
	assertRegistered(t, root)
}

func TestMCPBootstrapReportsDefaultProjectFailure(t *testing.T) {
	withTempWorkspace(t)
	workspace := filepath.Join(t.TempDir(), "app")
	if err := os.MkdirAll(filepath.Join(workspace, "projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A non-project file blocks the first project's destination.
	if err := os.WriteFile(filepath.Join(workspace, "projects", "APP"), []byte("keep this file"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := bootstrapCommand(t, workspace, true)
	if _, err := prepareMCPWorkspace(cmd, atlasmcp.Options{Profile: atlasmcp.ProfileWorkflow}); err == nil {
		t.Fatal("MCP startup must not report a failed default-project step as success")
	}
	got, err := os.ReadFile(filepath.Join(workspace, "projects", "APP"))
	if err != nil || string(got) != "keep this file" {
		t.Fatalf("existing file changed: %q %v", got, err)
	}
}

func TestMCPBootstrapUpgradesHollowWorkspace(t *testing.T) {
	withTempWorkspace(t)
	workspace := t.TempDir()
	if _, err := ensureInitArtifacts(workspace); err != nil {
		t.Fatal(err)
	}
	id, err := service.LoadWorkspaceIdentity(workspace)
	if err != nil || id == "" {
		t.Fatalf("hollow identity: %q %v", id, err)
	}
	config := filepath.Join(workspace, ".tracker", "config.toml")
	beforeCfg, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	beforeStat, err := os.Stat(config)
	if err != nil {
		t.Fatal(err)
	}
	projects, err := os.ReadDir(filepath.Join(workspace, "projects"))
	if err != nil || len(projects) != 0 {
		t.Fatalf("hollow should have zero projects: %v %v", projects, err)
	}
	cmd := bootstrapCommand(t, workspace, true)
	root, err := prepareMCPWorkspace(cmd, atlasmcp.Options{Profile: atlasmcp.ProfileWorkflow})
	if err != nil {
		t.Fatal(err)
	}
	afterID, err := service.LoadWorkspaceIdentity(root)
	if err != nil || afterID != id {
		t.Fatalf("identity changed: before=%s after=%s err=%v", id, afterID, err)
	}
	afterCfg, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	afterStat, err := os.Stat(config)
	if err != nil || string(afterCfg) != string(beforeCfg) || !afterStat.ModTime().Equal(beforeStat.ModTime()) {
		t.Fatal("hollow upgrade rewrote config")
	}
	filled, err := os.ReadDir(filepath.Join(root, "projects"))
	if err != nil || len(filled) == 0 {
		t.Fatalf("hollow upgrade must create the first project: %v %v", filled, err)
	}
	assertRegistered(t, root)
}

func TestMCPBootstrapWrongExpectedIDWritesNothing(t *testing.T) {
	withTempWorkspace(t)
	fresh := t.TempDir()
	cmd := bootstrapCommand(t, fresh, true)
	if err := cmd.Flags().Set("expected-workspace-id", "00000000-1111-2222-3333-444444444444"); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareMCPWorkspace(cmd, atlasmcp.Options{Profile: atlasmcp.ProfileWorkflow}); err == nil || !strings.Contains(err.Error(), "wrong workspace id") {
		t.Fatalf("fresh dir with expected id: %v", err)
	}
	if entries, err := os.ReadDir(fresh); err != nil || len(entries) != 0 {
		t.Fatalf("wrong id created files: %v %v", entries, err)
	}

	existing := t.TempDir()
	ok := bootstrapCommand(t, existing, true)
	root, err := prepareMCPWorkspace(ok, atlasmcp.Options{Profile: atlasmcp.ProfileWorkflow})
	if err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, ".tracker", "config.toml")
	beforeCfg, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	beforeProjects, err := os.ReadDir(filepath.Join(root, "projects"))
	if err != nil {
		t.Fatal(err)
	}
	bad := bootstrapCommand(t, existing, true)
	if err := bad.Flags().Set("expected-workspace-id", "00000000-1111-2222-3333-444444444444"); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareMCPWorkspace(bad, atlasmcp.Options{Profile: atlasmcp.ProfileWorkflow}); err == nil || !strings.Contains(err.Error(), "wrong workspace id") {
		t.Fatalf("existing workspace wrong id: %v", err)
	}
	afterCfg, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterCfg) != string(beforeCfg) {
		t.Fatal("wrong id rewrote config")
	}
	afterProjects, err := os.ReadDir(filepath.Join(root, "projects"))
	if err != nil || len(afterProjects) != len(beforeProjects) {
		t.Fatalf("wrong id changed projects: %v", afterProjects)
	}
}

func assertRegistered(t *testing.T, root string) {
	t.Helper()
	id, err := service.LoadWorkspaceIdentity(root)
	if err != nil || id == "" {
		t.Fatalf("identity: %q %v", id, err)
	}
	a, err := openApp()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	listed, err := a.ListWorkspaces(context.Background(), app.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	canon, err := service.CanonicalWorkspaceRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range listed {
		if rec.WorkspaceID == id && rec.Path == canon {
			return
		}
	}
	t.Fatalf("workspace %s at %s not in registry: %#v", id, canon, listed)
}
