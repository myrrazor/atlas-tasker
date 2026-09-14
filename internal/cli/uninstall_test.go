package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/uninstall"
)

func TestUninstallCommandPreviewJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	state, err := uninstall.StateDir(home, os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(home, "bin", "tracker")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("tracker-bin\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	receipt, err := uninstall.NewScriptReceipt(bin, "v1.15.0-test", time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if err := uninstall.WriteReceipt(state, receipt); err != nil {
		t.Fatal(err)
	}
	uninstallCommandOverride = &uninstall.FakeCommandRunner{}
	t.Cleanup(func() { uninstallCommandOverride = nil })

	cmd := newUninstallCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("preview: %v\n%s", err, out.String())
	}
	var payload struct {
		Kind     string `json:"kind"`
		Status   string `json:"status"`
		CanApply bool   `json:"can_apply"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if payload.Kind != uninstall.KindResult {
		t.Fatalf("envelope: %+v", payload)
	}
	if !payload.CanApply || payload.Status != uninstall.StatusPreview {
		t.Fatalf("expected a preview that can apply, got %+v\n%s", payload, out.String())
	}
	if !strings.Contains(out.String(), "remove-executable") && !strings.Contains(out.String(), bin) {
		t.Fatalf("preview should name the receipt binary:\n%s", out.String())
	}
}

func TestUninstallCommandApplyUsesInjectedRunner(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "xdg"))
	state, err := uninstall.StateDir(home, os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(home, "bin", "tracker")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("tracker-bin\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	receipt, err := uninstall.NewScriptReceipt(bin, "v1.15.0-test", time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if err := uninstall.WriteReceipt(state, receipt); err != nil {
		t.Fatal(err)
	}
	fake := &uninstall.FakeCommandRunner{}
	uninstallCommandOverride = fake
	t.Cleanup(func() { uninstallCommandOverride = nil })

	cmd := newUninstallCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--yes", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("apply: %v\n%s", err, out.String())
	}
	if _, err := os.Stat(bin); !os.IsNotExist(err) {
		t.Fatal("apply should remove the receipt-bound binary")
	}
	if _, err := os.Stat(filepath.Join(home, "xdg", "atlas-tasker", "registry.json")); err == nil {
		t.Fatal("should not create or wipe registry")
	}
}
