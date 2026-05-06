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
	if cfg.Notifications.DeliveryLogPath == "" || cfg.Notifications.DeadLetterPath == "" {
		t.Fatalf("expected delivery log defaults, got %#v", cfg.Notifications)
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
	if err := Set(root, "notifications.webhook_timeout_seconds", "9"); err != nil {
		t.Fatalf("set webhook timeout failed: %v", err)
	}
	if err := Set(root, "notifications.webhook_retries", "4"); err != nil {
		t.Fatalf("set webhook retries failed: %v", err)
	}
	if err := Set(root, "notifications.delivery_log_path", ".tracker/delivery.log"); err != nil {
		t.Fatalf("set delivery log path failed: %v", err)
	}
	if err := Set(root, "notifications.dead_letter_path", ".tracker/dead.log"); err != nil {
		t.Fatalf("set dead letter path failed: %v", err)
	}
}
