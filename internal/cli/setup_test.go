package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	atlasmcp "github.com/myrrazor/atlas-tasker/internal/mcp"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

func withPrivateSetupHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
}

func TestSetupPlanJSONIsStdoutOnlyAndReadOnly(t *testing.T) {
	withTempWorkspace(t)
	withPrivateSetupHome(t)
	if _, err := runCLI(t, "init", "--skip-integrations"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile("AGENTS.md")
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	out, err := runCLI(t, "setup", "--plan", "--json", "--agents", "generic")
	if err != nil {
		t.Fatalf("plan: %v\n%s", err, out)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Fatalf("json must be the whole stdout:\n%s", out)
	}
	if !strings.Contains(out, `"format_version": "v1"`) || !strings.Contains(out, `"kind": "setup_plan"`) {
		t.Fatalf("missing envelope:\n%s", out)
	}
	if strings.Contains(out, "sk-") || strings.Contains(out, "BEGIN PRIVATE") {
		t.Fatal("plan leaked a secret")
	}
	after, err := os.ReadFile("AGENTS.md")
	if !os.IsNotExist(err) && string(before) != string(after) {
		t.Fatal("plan wrote AGENTS.md")
	}
}

func TestSetupYesAppliesAndIsIdempotent(t *testing.T) {
	withTempWorkspace(t)
	withPrivateSetupHome(t)
	if _, err := runCLI(t, "init", "--skip-integrations"); err != nil {
		t.Fatal(err)
	}
	out, err := runCLI(t, "setup", "--yes", "--agents", "generic", "--json")
	if err != nil {
		t.Fatalf("apply: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(".tracker", "integrations", "generic-agent-skill", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	again, err := runCLI(t, "setup", "--yes", "--agents", "generic", "--json")
	if err != nil {
		t.Fatalf("re-apply: %v\n%s", err, again)
	}
	status, err := runCLI(t, "setup", "status", "--json")
	if err != nil && apperr.CodeOf(err) != apperr.CodeRepairNeeded {
		t.Fatalf("status: %v\n%s", err, status)
	}
	if !strings.Contains(status, `"kind": "setup_status"`) {
		t.Fatalf("status kind:\n%s", status)
	}
	intStatus, err := runCLI(t, "integrations", "status", "--json")
	if err != nil && apperr.CodeOf(err) != apperr.CodeRepairNeeded {
		t.Fatalf("integrations status: %v\n%s", err, intStatus)
	}
	if !strings.Contains(intStatus, `"kind": "integrations_status"`) {
		t.Fatalf("integrations status kind:\n%s", intStatus)
	}
}

func TestSetupNoninteractiveRequiresYesAndBackupTarget(t *testing.T) {
	withTempWorkspace(t)
	withPrivateSetupHome(t)
	if _, err := runCLI(t, "init", "--skip-integrations"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "setup", "--json", "--agents", "generic"); err == nil {
		t.Fatal("json without --yes/--plan must fail")
	}
	if _, err := runCLI(t, "setup", "--yes", "--backup", "--json"); err == nil {
		t.Fatal("--yes --backup without target must fail")
	}
	out, err := runCLI(t, "setup", "--plan", "--backup", "--json")
	if err != nil {
		t.Fatalf("plan backup: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"requested": true`) && !strings.Contains(out, "backup") {
		t.Fatalf("backup plan should include the backup consent group:\n%s", out)
	}
}

func TestSetupCancelAndEOFAreNotConsent(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetIn(strings.NewReader("n\n"))
	ok, err := readYesNo(cmd)
	if err != nil || ok {
		t.Fatalf("n must refuse, got %v %v", ok, err)
	}
	cmd = NewRootCommand()
	cmd.SetIn(strings.NewReader(""))
	ok, err = readYesNo(cmd)
	if err != nil || ok {
		t.Fatalf("EOF must refuse, got %v %v", ok, err)
	}
	cmd = NewRootCommand()
	cmd.SetIn(strings.NewReader("yes\n"))
	ok, err = readYesNo(cmd)
	if err != nil || !ok {
		t.Fatalf("yes must accept, got %v %v", ok, err)
	}
}

func TestSetupDeliveryAndOpenClawNaming(t *testing.T) {
	withTempWorkspace(t)
	withPrivateSetupHome(t)
	if _, err := runCLI(t, "init", "--skip-integrations"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "setup", "--yes", "--mode", "delivery", "--json"); err == nil {
		t.Fatal("delivery mode must be refused")
	}
}

func TestIntegrationsDisconnectAndRepairLeaves(t *testing.T) {
	withTempWorkspace(t)
	withPrivateSetupHome(t)
	if _, err := runCLI(t, "init", "--skip-integrations"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "setup", "--yes", "--agents", "generic", "--json"); err != nil {
		t.Fatal(err)
	}
	plan, err := runCLI(t, "integrations", "repair", "generic", "--json")
	if err != nil {
		t.Fatalf("repair plan: %v\n%s", err, plan)
	}
	if _, err := runCLI(t, "integrations", "repair", "generic", "--yes", "--json"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "integrations", "disconnect", "generic", "--yes", "--json"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(".tracker", "integrations", "generic-agent-skill", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("disconnect should remove the skill")
	}
}

func TestMCPWorkspaceFromCWD(t *testing.T) {
	withTempWorkspace(t)
	withPrivateSetupHome(t)
	if _, err := runCLI(t, "init", "--skip-integrations"); err != nil {
		t.Fatal(err)
	}
	id, err := service.LoadWorkspaceIdentity(".")
	if err != nil || id == "" {
		t.Fatalf("identity: %q %v", id, err)
	}
	cmd, _, err := NewRootCommand().Find([]string{"mcp", "serve"})
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("workspace-from-cwd", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("expected-workspace-id", id); err != nil {
		t.Fatal(err)
	}
	root, err := prepareMCPWorkspace(cmd, atlasmcp.Options{Profile: atlasmcp.ProfileRead})
	if err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(".")
	resolved, _ := filepath.EvalSymlinks(abs)
	if root != resolved {
		t.Fatalf("resolved %s want %s", root, resolved)
	}

	if err := cmd.Flags().Set("expected-workspace-id", "not-this"); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareMCPWorkspace(cmd, atlasmcp.Options{Profile: atlasmcp.ProfileRead}); err == nil {
		t.Fatal("wrong id must fail")
	}

	if err := cmd.Flags().Set("expected-workspace-id", id); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("workspace", root); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareMCPWorkspace(cmd, atlasmcp.Options{Profile: atlasmcp.ProfileRead}); err == nil {
		t.Fatal("from-cwd + workspace must fail")
	}
}

func TestMCPExpectedIDWithExplicitWorkspace(t *testing.T) {
	withTempWorkspace(t)
	withPrivateSetupHome(t)
	if _, err := runCLI(t, "init", "--skip-integrations"); err != nil {
		t.Fatal(err)
	}
	id, err := service.LoadWorkspaceIdentity(".")
	if err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	cmd, _, err := NewRootCommand().Find([]string{"mcp", "serve"})
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("workspace", abs); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("expected-workspace-id", id); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareMCPWorkspace(cmd, atlasmcp.Options{Profile: atlasmcp.ProfileRead}); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("expected-workspace-id", "other"); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareMCPWorkspace(cmd, atlasmcp.Options{Profile: atlasmcp.ProfileRead}); err == nil {
		t.Fatal("wrong expected id must fail")
	}
}

func TestSetupStatusJSONEnvelope(t *testing.T) {
	withTempWorkspace(t)
	withPrivateSetupHome(t)
	if _, err := runCLI(t, "init", "--skip-integrations"); err != nil {
		t.Fatal(err)
	}
	out, err := runCLI(t, "setup", "status", "--json")
	if err != nil && apperr.CodeOf(err) != apperr.CodeRepairNeeded {
		t.Fatalf("status: %v\n%s", err, out)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["format_version"] != "v1" {
		t.Fatalf("%v", payload["format_version"])
	}
}
