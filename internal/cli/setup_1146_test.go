package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/update"
)

func TestCohesiveSetupEmptyRepoBackupAndStatus(t *testing.T) {
	withTempWorkspace(t)
	withPrivateSetupHome(t)
	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return out
	}
	must("init", "--skip-integrations")
	must("project", "create", "APP", "App")
	must("ticket", "create", "--project", "APP", "--title", "First work", "--type", "task", "--actor", "human:owner")
	remote := t.TempDir()
	if out, err := exec.Command("git", "init", "--bare", remote).CombinedOutput(); err != nil {
		t.Fatalf("bare remote: %v\n%s", err, out)
	}
	must("backup", "target", "add", "--id", "local-drill", "--url", "file://"+remote,
		"--allow-local-file", "--acknowledge-data-boundary", "--attest-private", "--json")
	out := must("setup", "--yes", "--agents", "generic", "--mode", "managed", "--backup", "--backup-target", "local-drill", "--json")
	if strings.Contains(out, `"status": "connected"`) && strings.Contains(out, `"target": "codex"`) {
		t.Fatalf("must not claim named providers connected when they were not selected:\n%s", out)
	}
	if !strings.Contains(out, `"last_verified_checkpoint"`) && !strings.Contains(out, `"backup_worker_state": "enabled"`) {
		t.Fatalf("setup result should include backup verification:\n%s", out)
	}
	status, err := runCLI(t, "setup", "status", "--json")
	if err != nil && apperr.CodeOf(err) != apperr.CodeRepairNeeded {
		t.Fatalf("status: %v\n%s", err, status)
	}
	if strings.Contains(status, "file://") || strings.Contains(status, remote) {
		t.Fatalf("setup status leaked a backup URL:\n%s", status)
	}
	if !strings.Contains(status, `"backup_worker_state"`) {
		t.Fatalf("status missing backup worker:\n%s", status)
	}
	plan, err := runCLI(t, "backup", "schedule", "plan", "--json")
	if err != nil {
		t.Fatalf("schedule plan: %v\n%s", err, plan)
	}
	if strings.Contains(plan, `"installed": true`) && !strings.Contains(plan, "remaining") {
		t.Fatal("setup must not silently install the user scheduler")
	}
	auto := must("backup", "auto", "status", "--json")
	if !strings.Contains(auto, `"automatic_enabled": true`) {
		t.Fatalf("auto backup should be enabled:\n%s", auto)
	}
}

func TestV113UpgradePreservesCustomAndUnmanagedMCP(t *testing.T) {
	withTempWorkspace(t)
	withPrivateSetupHome(t)
	if _, err := runCLI(t, "init", "--skip-integrations"); err != nil {
		t.Fatal(err)
	}
	custom := "# Operator notes\n\nDo not drop this paragraph.\n"
	if err := os.WriteFile("AGENTS.md", []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(".cursor", 0o755); err != nil {
		t.Fatal(err)
	}
	unmanaged := []byte(`{"mcpServers":{"atlas-legacy":{"command":"/opt/legacy/tracker","args":["mcp","serve"]}}}` + "\n")
	if err := os.WriteFile(filepath.Join(".cursor", "mcp.json"), unmanaged, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "setup", "--yes", "--agents", "generic", "--json"); err != nil {
		t.Fatal(err)
	}
	agents, err := os.ReadFile("AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(agents), "Do not drop this paragraph.") {
		t.Fatalf("custom AGENTS.md lost:\n%s", agents)
	}
	got, err := os.ReadFile(filepath.Join(".cursor", "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(unmanaged) {
		t.Fatalf("unmanaged cursor MCP rewritten:\n%s", got)
	}
	repair, err := runCLI(t, "setup", "repair", "--json")
	if err != nil && apperr.CodeOf(err) != apperr.CodeRepairNeeded {
		t.Fatalf("repair plan: %v\n%s", err, repair)
	}
	if strings.Contains(repair, `"kind": "setup_result"`) && !strings.Contains(repair, "setup_repair_plan") {
		t.Fatalf("repair without --yes must stay a plan:\n%s", repair)
	}
}

func TestUpdatePackageDoesNotTouchProviderConfig(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "internal", "update", "update.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	for _, needle := range []string{".codex/config.toml", ".cursor/mcp.json", ".mcp.json", ".grok/config.toml", "mcp add"} {
		if strings.Contains(body, needle) {
			t.Fatalf("tracker update must not mention provider config %s", needle)
		}
	}
	if !strings.Contains(body, "atomically replace the running executable") {
		t.Fatal("update package should stay binary-only")
	}
	_ = update.DefaultBinary
}

func TestBackupTargetEditCLIRequiresAllowLocalFile(t *testing.T) {
	withTempWorkspace(t)
	withPrivateSetupHome(t)
	if _, err := runCLI(t, "init", "--skip-integrations"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "backup", "target", "add", "--id", "https-priv",
		"--url", "https://git.example.com/org/private.git",
		"--acknowledge-data-boundary", "--attest-private", "--json"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "backup", "target", "edit", "https-priv", "--url", "file:///tmp/sneaky.git", "--json"); err == nil {
		t.Fatal("edit to file:// without --allow-local-file must fail")
	}
}
