package web

import (
	"bytes"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/app"
	"github.com/myrrazor/atlas-tasker/internal/integrations"
)

func TestAgentsSettingsShowsProjectScopeAndActualArgv(t *testing.T) {
	tmpl, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	page := HomePage{
		Page:      "settings-agents",
		Host:      "127.0.0.1",
		Actor:     "human:owner",
		CSRFToken: "csrf",
		Settings:  app.MachineSettings{Agents: app.AgentSettings{AutoInstall: true}},
		Agents: []app.AgentClientReport{{
			Target:      integrations.TargetGrok,
			Status:      app.AgentPendingClientRestart,
			Command:     "tracker",
			Args:        []string{"mcp", "serve", "--workspace-from-cwd", "--tool-name-style", "portable"},
			Scope:       app.AgentScopeProject,
			WorkspaceID: "ws-demo",
			Provenance:  app.AgentProvenanceNative,
			Detail:      "project-scoped Grok entry matches this workspace; client restart may be required",
		}},
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "homeLayout", page); err != nil {
		t.Fatal(err)
	}
	body := buf.String()
	for _, needle := range []string{
		"project · ws-demo",
		"configured · pending client restart",
		"--tool-name-style portable",
		"--workspace-from-cwd",
		"Project-scoped setup",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("agents page missing %q:\n%s", needle, excerpt(body, "Agents"))
		}
	}
	if strings.Contains(body, `class="mode-chip">connected`) || strings.Contains(body, `class="mode-chip">unverified`) || strings.Contains(body, "UNVERIFIED") {
		t.Fatalf("project Grok must not be labeled live or user-unverified:\n%s", excerpt(body, "Agents"))
	}
}
