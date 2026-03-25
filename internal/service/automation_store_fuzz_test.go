package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func FuzzAutomationStoreLoadRule(f *testing.F) {
	for _, seed := range []string{
		"name = \"night-watch\"\n\n[trigger]\nevent_types = [\"ticket.moved\"]\n\n[[actions]]\nkind = \"notify\"\nmessage = \"heads up\"\n",
		"name = \"broken\"\n[trigger]\n",
		"",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		root := t.TempDir()
		if err := os.MkdirAll(storage.AutomationsDir(root), 0o755); err != nil {
			t.Fatalf("create automations dir: %v", err)
		}
		path := filepath.Join(storage.AutomationsDir(root), "fuzz.toml")
		if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
			t.Fatalf("write rule: %v", err)
		}
		_, _ = (AutomationStore{Root: root}).LoadRule("fuzz")
	})
}
