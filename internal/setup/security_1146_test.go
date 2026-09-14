package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
)

func TestSetupInspectRefusesConfigSymlink(t *testing.T) {
	engine := testEngine(t)
	outside := filepath.Join(t.TempDir(), "secret.toml")
	if err := os.WriteFile(outside, []byte("token=sk-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(engine.WorkspaceRoot, ".tracker", "config.toml")
	_ = os.Remove(link)
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	_, err := InspectFile(link, engine.currentUID)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "symlink") {
		t.Fatalf("symlink config must be refused: %v", err)
	}
}

func TestSetupPlanJSONOmitsSecretsAndRawFileURLs(t *testing.T) {
	engine := testEngine(t)
	prepared, err := engine.Plan(PlanOptions{Agents: []integrations.Target{integrations.TargetGeneric}, Backup: true, BackupTarget: "local-drill"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(prepared.Plan)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if strings.Contains(body, "sk-") || strings.Contains(body, "BEGIN PRIVATE") || strings.Contains(body, "file:///") {
		t.Fatalf("plan leaked a secret or raw file URL:\n%s", body)
	}
}
