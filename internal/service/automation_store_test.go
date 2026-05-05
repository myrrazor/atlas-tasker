package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestAutomationStoreRoundTrip(t *testing.T) {
	root := t.TempDir()
	store := AutomationStore{Root: root}
	rule := contracts.AutomationRule{
		Name:    "review-ready",
		Enabled: true,
		Trigger: contracts.AutomationTrigger{EventTypes: []contracts.EventType{contracts.EventTicketMoved}},
		Actions: []contracts.AutomationAction{{Kind: contracts.AutomationActionRequestReview}},
	}
	if err := store.SaveRule(rule); err != nil {
		t.Fatalf("save rule: %v", err)
	}
	loaded, err := store.LoadRule("review-ready")
	if err != nil {
		t.Fatalf("load rule: %v", err)
	}
	if loaded.Name != "review-ready" || !loaded.Enabled {
		t.Fatalf("unexpected loaded rule: %#v", loaded)
	}
}

func TestAutomationStoreDefaultsEnabledWhenOmitted(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(storage.AutomationsDir(root), 0o755); err != nil {
		t.Fatalf("mkdir automations: %v", err)
	}
	raw := []byte("name = \"night-watch\"\n\n[trigger]\nevent_types = [\"ticket.moved\"]\n\n[[actions]]\nkind = \"notify\"\nmessage = \"heads up\"\n")
	path := filepath.Join(storage.AutomationsDir(root), "night-watch.toml")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write rule: %v", err)
	}
	loaded, err := (AutomationStore{Root: root}).LoadRule("night-watch")
	if err != nil {
		t.Fatalf("load rule: %v", err)
	}
	if !loaded.Enabled {
		t.Fatalf("expected omitted enabled flag to default true: %#v", loaded)
	}
}
