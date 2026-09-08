package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/config"
)

func TestConfigSetJSONDoesNotEchoWebhookSecrets(t *testing.T) {
	withTempWorkspace(t)
	if _, err := runCLI(t, "init"); err != nil {
		t.Fatal(err)
	}
	url := "https://fixture-user:fixture-password@example.invalid/hook?token=fixture-token"
	out, err := runCLI(t, "config", "set", "notifications.webhook_url", url, "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"fixture-password", "fixture-token"} {
		if strings.Contains(out, secret) {
			t.Fatal("config set JSON exposed a fixture credential")
		}
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Notifications.WebhookURL != url {
		t.Fatal("redaction changed the stored webhook")
	}
}
