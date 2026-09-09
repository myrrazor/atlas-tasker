package cli

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestIntegrationConfirmationPreservesNextAnswer(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetIn(strings.NewReader("yes\ncursor\n"))
	cmd.SetOut(io.Discard)
	ok, err := confirmIntegrationsSetup(cmd)
	if err != nil || !ok {
		t.Fatalf("confirmation = %t, %v", ok, err)
	}
	remaining, err := io.ReadAll(cmd.InOrStdin())
	if err != nil || string(remaining) != "cursor\n" {
		t.Fatalf("confirmation consumed next answer: %q, %v", remaining, err)
	}
}

func TestIntegrationConfirmationEOFDoesNotConsent(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetIn(strings.NewReader(""))
	cmd.SetOut(io.Discard)
	ok, err := confirmIntegrationsSetup(cmd)
	if err != nil || ok {
		t.Fatalf("EOF must skip setup: %t, %v", ok, err)
	}
}

func TestIntegrationPickerCancellationDoesNotInitializeWorkspace(t *testing.T) {
	for _, selection := range []string{"q\n", "none\n", "skip\n", "\n", ""} {
		t.Run(strings.TrimSpace(selection), func(t *testing.T) {
			withTempWorkspace(t)
			// A machine without detected agents must also be safe to skip.
			t.Setenv("HOME", t.TempDir())
			t.Setenv("PATH", t.TempDir())
			t.Setenv("CODEX_HOME", "")
			t.Setenv("CLAUDE_CONFIG_DIR", "")
			cmd, _, err := NewRootCommand().Find([]string{"integrations", "install"})
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			cmd.SetIn(strings.NewReader(selection))
			cmd.SetOut(&out)
			if err := runIntegrationsInstallWizard(cmd, nil, false, false, true); err != nil {
				t.Fatalf("skip failed: %v\n%s", err, out.String())
			}
			entries, err := os.ReadDir(".")
			if err != nil || len(entries) != 0 {
				t.Fatalf("skipping setup wrote workspace files: %v, %v", entries, err)
			}
		})
	}
}

func TestIntegrationInvalidSelectionDoesNotInitializeWorkspace(t *testing.T) {
	for _, args := range [][]string{
		{"integrations", "install"},
		{"integrations", "install", "--targets", "codex", "--global"},
		{"integrations", "install", "--targets", "unknown"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			withTempWorkspace(t)
			if out, err := runCLI(t, args...); err == nil {
				t.Fatalf("expected invalid selection to fail: %s", out)
			}
			entries, err := os.ReadDir(".")
			if err != nil || len(entries) != 0 {
				t.Fatalf("invalid selection wrote workspace files: %v, %v", entries, err)
			}
		})
	}
}

func TestIntegrationMalformedInteractiveSelectionDoesNotInitializeWorkspace(t *testing.T) {
	withTempWorkspace(t)
	cmd, _, err := NewRootCommand().Find([]string{"integrations", "install"})
	if err != nil {
		t.Fatal(err)
	}
	cmd.SetIn(strings.NewReader(",\n"))
	cmd.SetOut(io.Discard)
	if err := runIntegrationsInstallWizard(cmd, nil, false, false, true); err == nil {
		t.Fatal("malformed selection must fail")
	}
	entries, err := os.ReadDir(".")
	if err != nil || len(entries) != 0 {
		t.Fatalf("malformed selection wrote workspace files: %v, %v", entries, err)
	}
}
