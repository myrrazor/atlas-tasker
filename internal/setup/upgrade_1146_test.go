package setup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
)

func TestV113CustomInstructionsSurviveSetup(t *testing.T) {
	engine := testEngine(t)
	custom := "# Keep me\n\nThis is operator-owned text outside Atlas markers.\n"
	if err := os.WriteFile(filepath.Join(engine.WorkspaceRoot, "AGENTS.md"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(engine.WorkspaceRoot, "CLAUDE.md"), []byte("# Claude keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	applyGeneric(t, engine)
	agents, err := os.ReadFile(filepath.Join(engine.WorkspaceRoot, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(agents), "This is operator-owned text") {
		t.Fatalf("custom AGENTS.md text was rewritten:\n%s", agents)
	}
	claude, err := os.ReadFile(filepath.Join(engine.WorkspaceRoot, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(claude), "Claude keep") {
		t.Fatalf("custom CLAUDE.md text was rewritten:\n%s", claude)
	}
}

func TestUnmanagedAtlasMCPIsNotRewritten(t *testing.T) {
	engine := testEngine(t)
	cursorDir := filepath.Join(engine.WorkspaceRoot, ".cursor")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	unmanaged := []byte(`{"mcpServers":{"atlas-legacy":{"command":"/opt/other/tracker","args":["mcp","serve"]}}}` + "\n")
	if err := os.WriteFile(filepath.Join(cursorDir, "mcp.json"), unmanaged, 0o644); err != nil {
		t.Fatal(err)
	}
	applyGeneric(t, engine)
	got, err := os.ReadFile(filepath.Join(cursorDir, "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(unmanaged) {
		t.Fatalf("unmanaged MCP was rewritten:\n%s", got)
	}
	status, err := engine.StatusReport()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status.RepairReason, "unmanaged MCP") {
		t.Fatalf("status should surface unmanaged MCP, got %q", status.RepairReason)
	}
}

func TestWorkspaceWithoutManifestCanPlanRepair(t *testing.T) {
	engine := testEngine(t)
	report, err := engine.Repair(context.Background(), integrations.TargetGeneric, false)
	if err != nil {
		t.Fatal(err)
	}
	if report.Kind != "setup_repair_plan" {
		t.Fatalf("kind=%s", report.Kind)
	}
	if _, err := os.Stat(filepath.Join(engine.WorkspaceRoot, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatal("repair plan must not write")
	}
}
