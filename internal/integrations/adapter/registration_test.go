package adapter

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

const testWorkspaceID = "6f1d2c3b-4a5e-4f60-8b7c-9d0e1f2a3b4c"

func TestServerNameForIsDeterministicAndSafe(t *testing.T) {
	a, err := ServerNameFor(testWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := ServerNameFor(testWorkspaceID)
	if a != b {
		t.Fatalf("server name must be deterministic: %s vs %s", a, b)
	}
	if !serverNamePattern.MatchString(a) {
		t.Fatalf("server name %q does not match %s", a, serverNamePattern)
	}
	if strings.Contains(a, testWorkspaceID[:8]) {
		t.Fatal("server name must not leak the raw workspace id")
	}
	other, _ := ServerNameFor("another-workspace")
	if other == a {
		t.Fatal("different workspaces must get different names")
	}
	for _, bad := range []string{"", " ", " id", "id "} {
		if _, err := ServerNameFor(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestRegistrationArgsPerBinding(t *testing.T) {
	tail := []string{"--expected-workspace-id", testWorkspaceID, "--tool-profile", "workflow", "--max-items", "30", "--max-result-bytes", "65536"}
	cases := []struct {
		binding WorkspaceBinding
		head    []string
	}{
		{WorkspaceBinding{Kind: WorkspaceBindingAbsolutePath, WorkspaceRoot: "/srv/workspace"}, []string{"mcp", "serve", "--workspace", "/srv/workspace"}},
		{WorkspaceBinding{Kind: WorkspaceBindingClientVariable, Placeholder: "${workspaceFolder}"}, []string{"mcp", "serve", "--workspace", "${workspaceFolder}"}},
		{WorkspaceBinding{Kind: WorkspaceBindingClientVariable, Placeholder: "${CLAUDE_PROJECT_DIR:-.}"}, []string{"mcp", "serve", "--workspace", "${CLAUDE_PROJECT_DIR:-.}"}},
		{WorkspaceBinding{Kind: WorkspaceBindingVerifiedCwd}, []string{"mcp", "serve", "--workspace-from-cwd"}},
	}
	for _, tc := range cases {
		want := append(append([]string{}, tc.head...), tail...)
		got := RegistrationArgs(testWorkspaceID, tc.binding)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: args = %v, want %v", tc.binding.Kind, got, want)
		}
		reg, err := NewRegistration("/usr/local/bin/tracker", testWorkspaceID, tc.binding, "agent:builder-1", false)
		if err != nil {
			t.Fatalf("%s: %v", tc.binding.Kind, err)
		}
		if err := reg.Validate(); err != nil {
			t.Fatalf("%s: %v", tc.binding.Kind, err)
		}
	}
}

func TestBindingValidation(t *testing.T) {
	bad := []WorkspaceBinding{
		{Kind: "magic"},
		{Kind: WorkspaceBindingAbsolutePath},
		{Kind: WorkspaceBindingAbsolutePath, WorkspaceRoot: "relative"},
		{Kind: WorkspaceBindingAbsolutePath, WorkspaceRoot: "/srv/ws/", Placeholder: ""},
		{Kind: WorkspaceBindingAbsolutePath, WorkspaceRoot: "/srv/ws", Placeholder: "${workspaceFolder}"},
		{Kind: WorkspaceBindingClientVariable},
		{Kind: WorkspaceBindingClientVariable, Placeholder: "$HOME"},
		{Kind: WorkspaceBindingClientVariable, Placeholder: "${a} ${b}"},
		{Kind: WorkspaceBindingClientVariable, Placeholder: "${workspaceFolder}", WorkspaceRoot: "/srv/ws"},
		{Kind: WorkspaceBindingVerifiedCwd, WorkspaceRoot: "/srv/ws"},
		{Kind: WorkspaceBindingVerifiedCwd, Placeholder: "${workspaceFolder}"},
	}
	for i, binding := range bad {
		if err := binding.Validate(); err == nil {
			t.Errorf("case %d (%+v) should be rejected", i, binding)
		}
	}
}

func TestRegistrationValidateRejectsTampering(t *testing.T) {
	base := func() MCPRegistration {
		reg, err := NewRegistration("/usr/local/bin/tracker", testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingAbsolutePath, WorkspaceRoot: "/srv/workspace"}, "agent:builder-1", false)
		if err != nil {
			t.Fatal(err)
		}
		return reg
	}
	cases := map[string]func(*MCPRegistration){
		"danger flag":         func(r *MCPRegistration) { r.Args = append(r.Args, "--dangerously-allow-high-impact-tools") },
		"init if missing":     func(r *MCPRegistration) { r.Args = append(r.Args, "--init-if-missing") },
		"read only":           func(r *MCPRegistration) { r.Args = append(r.Args, "--read-only") },
		"admin profile":       func(r *MCPRegistration) { r.Args[7] = "admin"; r.ToolProfile = "admin" },
		"delivery in args":    func(r *MCPRegistration) { r.Args[7] = "delivery" },
		"bigger max items":    func(r *MCPRegistration) { r.MaxItems = 500 },
		"bigger result bytes": func(r *MCPRegistration) { r.MaxResultBytes = 1 << 20 },
		"args reordered":      func(r *MCPRegistration) { r.Args[0], r.Args[1] = r.Args[1], r.Args[0] },
		"extra arg":           func(r *MCPRegistration) { r.Args = append(r.Args, "--verbose") },
		"missing arg":         func(r *MCPRegistration) { r.Args = r.Args[:len(r.Args)-2] },
		"http transport":      func(r *MCPRegistration) { r.Transport = "http" },
		"relative command":    func(r *MCPRegistration) { r.Command = "tracker" },
		"renamed server":      func(r *MCPRegistration) { r.ServerName = "atlas-000000000000" },
		"free-form name":      func(r *MCPRegistration) { r.ServerName = "atlas" },
		"other workspace":     func(r *MCPRegistration) { r.WorkspaceID = "other" },
		"bad actor hint":      func(r *MCPRegistration) { r.ActorHint = "agent:" },
		"portable with path":  func(r *MCPRegistration) { r.Portable = true },
	}
	for name, mutate := range cases {
		reg := base()
		mutate(&reg)
		if err := reg.Validate(); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
}

func TestPortableRegistration(t *testing.T) {
	reg, err := NewRegistration(PortableExecutableName, testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingVerifiedCwd}, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.ValidateForScope(ScopeProjectShared, "/home/someone"); err != nil {
		t.Fatalf("portable registration must be allowed in repository-carried scope: %v", err)
	}
	if _, err := NewRegistration(PortableExecutableName, testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingAbsolutePath, WorkspaceRoot: "/srv/ws"}, "", true); err == nil {
		t.Fatal("portable registrations must use verified_cwd")
	}
	raw, err := reg.StandardConfigJSON()
	if err != nil {
		t.Fatal(err)
	}
	var cfg StandardConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	entry, ok := cfg.MCPServers[reg.ServerName]
	if !ok {
		t.Fatalf("descriptor missing server %s: %s", reg.ServerName, raw)
	}
	if entry.Type != "stdio" || entry.Command != "tracker" || !reflect.DeepEqual(entry.Args, reg.Args) {
		t.Fatalf("unexpected descriptor entry: %+v", entry)
	}
	if strings.Contains(string(raw), "/home/") || strings.Contains(string(raw), `"env"`) {
		t.Fatalf("portable descriptor must not carry home paths or env: %s", raw)
	}
	if _, err := reg.ServeCommand("", "/srv/ws", time.Minute); err == nil {
		t.Fatal("portable registration cannot be probed without a resolved executable")
	}
	cmd, err := reg.ServeCommand("/usr/local/bin/tracker", "/srv/ws", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Purpose != CommandPurposeProbe || cmd.Dir != "/srv/ws" || !reflect.DeepEqual(cmd.Args, reg.Args) {
		t.Fatalf("unexpected serve command: %+v", cmd)
	}
}

func TestValidateForScopeRefusesMachinePathsInRepositoryCarriedConfig(t *testing.T) {
	absolute, err := NewRegistration("/usr/local/bin/tracker", testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingAbsolutePath, WorkspaceRoot: "/srv/workspace"}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := absolute.ValidateForScope(ScopeProjectLocal, "/home/someone"); err != nil {
		t.Fatalf("machine-local scope must accept absolute bindings: %v", err)
	}
	if err := absolute.ValidateForScope(ScopeGateway, "/home/someone"); err != nil {
		t.Fatalf("gateway scope must accept absolute bindings: %v", err)
	}
	if err := absolute.ValidateForScope(ScopeProjectShared, "/home/someone"); err == nil {
		t.Fatal("repository-carried scope must refuse absolute workspace paths")
	}
	homeExe, err := NewRegistration("/home/someone/.local/bin/tracker", testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingClientVariable, Placeholder: "${workspaceFolder}"}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := homeExe.ValidateForScope(ScopeProjectShared, "/home/someone"); err == nil {
		t.Fatal("repository-carried scope must refuse executables under the home directory")
	}
	if err := homeExe.ValidateForScope(ScopeProjectShared, ""); err != nil {
		t.Fatalf("without a home hint the executable check is skipped: %v", err)
	}
	if err := homeExe.ValidateForScope("nowhere", "/home/someone"); err == nil {
		t.Fatal("invalid scope must be rejected")
	}
}

func TestRegistrationFingerprintTracksOwnedFields(t *testing.T) {
	reg, _ := NewRegistration("/usr/local/bin/tracker", testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingVerifiedCwd}, "agent:a", false)
	same, _ := NewRegistration("/usr/local/bin/tracker", testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingVerifiedCwd}, "agent:b", false)
	if reg.Fingerprint() != same.Fingerprint() {
		t.Fatal("actor hint is not part of the on-disk entry and must not change the fingerprint")
	}
	moved, _ := NewRegistration("/opt/atlas/tracker", testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingVerifiedCwd}, "agent:a", false)
	if reg.Fingerprint() == moved.Fingerprint() {
		t.Fatal("a different executable must change the fingerprint")
	}
}
