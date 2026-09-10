package adapter

import (
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
)

func TestMatrixCoversAllSixTargetsInOrder(t *testing.T) {
	rows := Matrix()
	targets := integrations.DetectableTargets()
	if len(rows) != 6 || len(targets) != 6 {
		t.Fatalf("expected six targets, got %d rows and %d detectable targets", len(rows), len(targets))
	}
	for i, row := range rows {
		if row.Target != targets[i] {
			t.Fatalf("row %d is %s, want %s (matrix must follow DetectableTargets order)", i, row.Target, targets[i])
		}
		if err := row.Validate(); err != nil {
			t.Fatalf("%s: %v", row.Target, err)
		}
	}
}

func TestMatrixInvariants(t *testing.T) {
	for _, row := range Matrix() {
		for _, source := range row.Sources {
			if !strings.HasPrefix(source, "https://") {
				t.Errorf("%s: source %q must be an https URL", row.Target, source)
			}
		}
		for _, scope := range row.Scopes {
			if scope.Scope.RepositoryCarried() && scope.Binding == WorkspaceBindingAbsolutePath {
				t.Errorf("%s: repository-carried scope %s embeds an absolute path binding", row.Target, scope.Path)
			}
			if scope.Scope.RepositoryCarried() && strings.HasPrefix(scope.Path, "~") {
				t.Errorf("%s: repository-carried scope %s points at the home directory", row.Target, scope.Path)
			}
			if !scope.Scope.RepositoryCarried() && scope.Scope != ScopeGateway && !strings.HasPrefix(scope.Path, "~") {
				t.Errorf("%s: machine-local scope %s should live under the home directory", row.Target, scope.Path)
			}
		}
		preferred, ok := row.Preferred()
		if !ok {
			t.Fatalf("%s: missing preferred scope", row.Target)
		}
		if preferred.WriteMethod == WriteMethodAtlasFileEdit && preferred.Format == ConfigFormatClientManaged {
			t.Errorf("%s: preferred scope is client-managed but marked for Atlas edits", row.Target)
		}
		if row.Target == integrations.TargetGeneric {
			if row.MaxPlannedState != StatePortableReady {
				t.Errorf("generic must cap at portable_ready, got %s", row.MaxPlannedState)
			}
			if row.ClientExecutable != "" {
				t.Errorf("generic has no client executable")
			}
			continue
		}
		if row.ClientExecutable == "" || len(row.VersionArgs) == 0 {
			t.Errorf("%s: named client needs executable and version probe", row.Target)
		}
		if row.MCPApps == SupportYes && row.Target != integrations.TargetCursor {
			t.Errorf("%s: MCP Apps support is only verified for Cursor", row.Target)
		}
	}
}

func TestCapabilitiesForUnknownTarget(t *testing.T) {
	if _, err := CapabilitiesFor("vim"); err == nil {
		t.Fatal("unknown target must error")
	}
	row, err := CapabilitiesFor(integrations.TargetOpenClaw)
	if err != nil {
		t.Fatal(err)
	}
	if row.PreferredScope != ScopeGateway || row.MaxPlannedState != StateConnectedRestartRequired {
		t.Fatalf("unexpected openclaw row: %+v", row)
	}
}

func TestCapabilitiesValidateRejectsBrokenRows(t *testing.T) {
	good := codexCapabilities()
	cases := map[string]func(*Capabilities){
		"no sources":        func(c *Capabilities) { c.Sources = nil },
		"no scopes":         func(c *Capabilities) { c.Scopes = nil },
		"preferred missing": func(c *Capabilities) { c.PreferredScope = ScopeGateway },
		"absolute in repo":  func(c *Capabilities) { c.Scopes[0].Binding = WorkspaceBindingAbsolutePath },
		"edit client-managed": func(c *Capabilities) {
			c.Scopes[0].Format = ConfigFormatClientManaged
			c.Scopes[0].WriteMethod = WriteMethodAtlasFileEdit
		},
		"bad state":       func(c *Capabilities) { c.MaxPlannedState = "ready" },
		"no version args": func(c *Capabilities) { c.VersionArgs = nil },
		"no verification": func(c *Capabilities) { c.Verification = nil },
		"unknown target":  func(c *Capabilities) { c.Target = "emacs" },
	}
	for name, mutate := range cases {
		row := good
		row.Scopes = append([]ScopeCapability(nil), good.Scopes...)
		mutate(&row)
		if err := row.Validate(); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
	generic := genericCapabilities()
	generic.MaxPlannedState = StateConnected
	if err := generic.Validate(); err == nil {
		t.Fatal("generic must not be allowed to plan connected")
	}
}

func TestApprovalPendingStates(t *testing.T) {
	cases := map[ApprovalRequirement]State{
		ApprovalWorkspaceTrust:              StatePendingWorkspaceTrust,
		ApprovalWorkspaceTrustThenMCPApprov: StatePendingWorkspaceTrust,
		ApprovalMCPApproval:                 StatePendingMCPApproval,
		ApprovalAdminPolicy:                 StatePendingMCPApproval,
	}
	for requirement, want := range cases {
		got, ok := requirement.PendingState()
		if !ok || got != want {
			t.Errorf("%s: pending state = %s (%v), want %s", requirement, got, ok, want)
		}
	}
	if _, ok := ApprovalNone.PendingState(); ok {
		t.Fatal("none must not map to a pending state")
	}
}
