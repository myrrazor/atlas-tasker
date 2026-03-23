package config

import (
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestLoadDefaultWhenMissing(t *testing.T) {
	root := t.TempDir()
	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("load default config failed: %v", err)
	}
	if cfg.Workflow.CompletionMode != contracts.CompletionModeOpen {
		t.Fatalf("unexpected default completion mode: %s", cfg.Workflow.CompletionMode)
	}
}

func TestSetAndGetCompletionMode(t *testing.T) {
	root := t.TempDir()
	if err := Set(root, "workflow.completion_mode", "owner_gate"); err != nil {
		t.Fatalf("set completion mode failed: %v", err)
	}
	value, err := Get(root, "workflow.completion_mode")
	if err != nil {
		t.Fatalf("get completion mode failed: %v", err)
	}
	if value != "owner_gate" {
		t.Fatalf("unexpected completion mode: %s", value)
	}
}

func TestSetRejectsInvalidMode(t *testing.T) {
	root := t.TempDir()
	if err := Set(root, "workflow.completion_mode", "not-real"); err == nil {
		t.Fatal("expected invalid completion mode error")
	}
}

func TestSetAndGetDefaultActor(t *testing.T) {
	root := t.TempDir()
	if err := Set(root, "actor.default", "agent:builder-1"); err != nil {
		t.Fatalf("set actor.default failed: %v", err)
	}
	value, err := Get(root, "actor.default")
	if err != nil {
		t.Fatalf("get actor.default failed: %v", err)
	}
	if value != "agent:builder-1" {
		t.Fatalf("unexpected actor.default: %s", value)
	}
}

func TestNotificationsDefaultsAndSetters(t *testing.T) {
	root := t.TempDir()
	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("load default config failed: %v", err)
	}
	if !cfg.Notifications.Terminal {
		t.Fatal("expected terminal notifications to default on")
	}
	if err := Set(root, "notifications.file_enabled", "true"); err != nil {
		t.Fatalf("enable file notifications failed: %v", err)
	}
	if err := Set(root, "notifications.file_path", ".tracker/custom-notify.log"); err != nil {
		t.Fatalf("set notification file path failed: %v", err)
	}
	value, err := Get(root, "notifications.file_path")
	if err != nil {
		t.Fatalf("get notifications.file_path failed: %v", err)
	}
	if value != ".tracker/custom-notify.log" {
		t.Fatalf("unexpected notifications.file_path: %s", value)
	}
}
