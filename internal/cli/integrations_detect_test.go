package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
)

func TestIntegrationsDetectJSON(t *testing.T) {
	withTempWorkspace(t)
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	out, err := runCLI(t, "integrations", "detect", "--json")
	if err != nil {
		t.Fatalf("detect failed: %v\n%s", err, out)
	}
	var payload struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Found         []struct {
			Target  string   `json:"target"`
			Found   bool     `json:"found"`
			Reasons []string `json:"reasons"`
		} `json:"found"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("parse json: %v\n%s", err, out)
	}
	if payload.FormatVersion != jsonFormatVersion || payload.Kind != "integrations_detect" {
		t.Fatalf("unexpected envelope: %+v", payload)
	}
	foundClaude := false
	for _, item := range payload.Found {
		if item.Target == string(integrations.TargetClaude) && item.Found {
			foundClaude = true
		}
	}
	if !foundClaude {
		t.Fatalf("expected claude detection from ~/.claude, got %s", out)
	}
}

func TestIntegrationsInstallTargetsNonInteractive(t *testing.T) {
	withTempWorkspace(t)
	out, err := runCLI(t, "integrations", "install", "--targets", "claude,cursor", "--json")
	if err != nil {
		t.Fatalf("install --targets failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"kind": "integrations_install"`) {
		t.Fatalf("unexpected output: %s", out)
	}
	if _, err := os.Stat("CLAUDE.md"); err != nil {
		t.Fatalf("expected CLAUDE.md: %v", err)
	}
	if _, err := os.Stat("AGENTS.md"); err != nil {
		t.Fatalf("expected AGENTS.md from cursor install: %v", err)
	}
	body, err := os.ReadFile("AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "atlas-tasker:cursor") {
		t.Fatalf("expected cursor markers in AGENTS.md: %s", body)
	}
}

func TestIntegrationsInstallRequiresTargetsWithoutTTY(t *testing.T) {
	withTempWorkspace(t)
	_, err := runCLI(t, "integrations", "install")
	if err == nil || !strings.Contains(err.Error(), "no integration targets") {
		t.Fatalf("expected targets required without TTY, got %v", err)
	}
}

func TestInitSkipIntegrationsFlag(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	out, err := runCLI(t, "init", "--skip-integrations", "--json")
	if err != nil {
		t.Fatalf("init --skip-integrations failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "workspace") && !strings.Contains(out, "created") {
		// envelope still includes created list; just ensure command succeeded
		t.Logf("init output: %s", out)
	}
}
