package all

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter/openclaw"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestRegistryAcceptsAllSixAdapters(t *testing.T) {
	reg, err := New(Options{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(reg.Targets()); got != 6 {
		t.Fatalf("registered %d", got)
	}
}

func testPlanInput(t *testing.T, target integrations.Target, detection adapter.Detection) (adapter.PlanInput, string) {
	t.Helper()
	root := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	if err := os.MkdirAll(storage.TrackerDir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storage.TrackerDir(root), "workspace.json"), []byte(`{"workspace_id":"ws-adapter-test"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	tracker := filepath.Join(t.TempDir(), "tracker")
	if err := os.WriteFile(tracker, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	detection.Target = target
	if detection.VersionSupport == "" {
		detection.VersionSupport = adapter.VersionUnknown
	}
	return adapter.PlanInput{
		WorkspaceRoot: root,
		WorkspaceID:   "ws-adapter-test",
		Home:          home,
		TrackerPath:   tracker,
		ActorHint:     contracts.Actor("agent:" + string(target)),
		Detection:     detection,
	}, root
}

func TestCodexWritesProjectTOMLAndKeepsTrustPending(t *testing.T) {
	reg, err := New(Options{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	item, _ := reg.Lookup(integrations.TargetCodex)
	input, root := testPlanInput(t, integrations.TargetCodex, adapter.Detection{VersionSupport: adapter.VersionUnknown})
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := item.Plan(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ResultingState != adapter.StatePendingWorkspaceTrust {
		t.Fatalf("state %s", plan.ResultingState)
	}
	if len(plan.ApprovalSteps) == 0 {
		t.Fatal("missing workspace trust approval")
	}
	src, ok := item.(interface{ LastPayloads() map[string][]byte })
	if !ok {
		t.Fatal("missing payloads")
	}
	found := false
	for _, step := range plan.Steps {
		if step.Kind == adapter.StepWriteConfigEntry {
			found = true
			body := string(src.LastPayloads()[step.StepID])
			if !strings.Contains(body, "required = false") || !strings.Contains(body, "default_tools_approval_mode") {
				t.Fatalf("codex table:\n%s", body)
			}
		}
	}
	if !found {
		t.Fatal("expected Codex config write")
	}
}

func TestCursorRejectsDuplicateJSONAndUnmanagedName(t *testing.T) {
	reg, err := New(Options{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	item, _ := reg.Lookup(integrations.TargetCursor)
	input, root := testPlanInput(t, integrations.TargetCursor, adapter.Detection{
		VersionSupport:  adapter.VersionUnknown,
		ExistingServers: []adapter.ExistingServer{{Name: mustServer(t, "ws-adapter-test"), Scope: adapter.ScopeProjectShared}},
	})
	if err := os.MkdirAll(filepath.Join(root, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := item.Plan(context.Background(), input); err == nil {
		t.Fatal("expected unmanaged same-name refusal")
	}
	input.Detection.ExistingServers = nil
	if err := os.WriteFile(filepath.Join(root, ".cursor", "mcp.json"), []byte(`{"mcpServers":{"a":{},"a":{}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := item.Plan(context.Background(), input); err == nil {
		t.Fatal("expected duplicate JSON refusal")
	}
}

func TestGenericPortableNeverClaimsConnectedFromFileAlone(t *testing.T) {
	reg, err := New(Options{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	item, _ := reg.Lookup(integrations.TargetGeneric)
	input, _ := testPlanInput(t, integrations.TargetGeneric, adapter.Detection{VersionSupport: adapter.VersionUnknown})
	plan, err := item.Plan(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ResultingState != adapter.StatePortableReady {
		t.Fatalf("state %s", plan.ResultingState)
	}
	applied, err := item.Apply(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if applied.State.Verified() {
		t.Fatalf("apply claimed %s", applied.State)
	}
	if _, err := os.Stat(filepath.Join(input.WorkspaceRoot, ".tracker", "integrations", "atlas-mcp.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(input.WorkspaceRoot, ".tracker", "integrations", "generic-agent-skill", "references", "atlas-context.md")); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeAndOpenClawRequireNamedClientForMCP(t *testing.T) {
	reg, err := New(Options{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []integrations.Target{integrations.TargetClaude, integrations.TargetOpenClaw, integrations.TargetGrok} {
		item, _ := reg.Lookup(target)
		input, _ := testPlanInput(t, target, adapter.Detection{VersionSupport: adapter.VersionUnknown})
		plan, err := item.Plan(context.Background(), input)
		if err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		for _, step := range plan.Steps {
			if step.Kind == adapter.StepRunClientCommand {
				t.Fatalf("%s planned a CLI command without a client", target)
			}
		}
	}
}

func TestGrokProjectRegistrationUsesPortableToolNames(t *testing.T) {
	reg, err := New(Options{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	item, _ := reg.Lookup(integrations.TargetGrok)
	exe := filepath.Join(t.TempDir(), "grok")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	input, _ := testPlanInput(t, integrations.TargetGrok, adapter.Detection{
		Installed: true, ExecutablePath: exe, VersionSupport: adapter.VersionSupported,
		Version: adapter.ClientVersion{Raw: "1.0.30", Major: 1, Minor: 0, Patch: 30, Known: true},
	})
	plan, err := item.Plan(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Registration == nil {
		t.Fatal("missing grok registration")
	}
	if plan.Registration.ToolNameStyle != adapter.ToolNameStylePortable {
		t.Fatalf("grok style %q", plan.Registration.ToolNameStyle)
	}
	regArgs := strings.Join(plan.Registration.Args, " ")
	if !strings.Contains(regArgs, adapter.FlagToolNameStyle) || !strings.Contains(regArgs, string(adapter.ToolNameStylePortable)) {
		t.Fatalf("grok registration args %v", plan.Registration.Args)
	}
	found := false
	for _, step := range plan.Steps {
		if step.Kind == adapter.StepRunClientCommand && step.Command != nil {
			joined := strings.Join(step.Command.Args, " ")
			if !strings.Contains(joined, "mcp add") {
				continue
			}
			found = true
			if !strings.Contains(joined, adapter.FlagToolNameStyle) || !strings.Contains(joined, "portable") {
				t.Fatalf("grok mcp add missing portable flag: %v", step.Command.Args)
			}
			if strings.Contains(joined, "dangerously") {
				t.Fatalf("grok mcp add leaked danger flag: %v", step.Command.Args)
			}
		}
	}
	if !found {
		t.Fatal("expected grok mcp add")
	}
}

func TestGrokNativeSkipRequiresExactAtlasArgv(t *testing.T) {
	reg, err := New(Options{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	item, _ := reg.Lookup(integrations.TargetGrok)

	planGrok := func(t *testing.T, write func(root string, input adapter.PlanInput, expected adapter.MCPRegistration)) (adapter.IntegrationPlan, error) {
		t.Helper()
		exe := filepath.Join(t.TempDir(), "grok")
		if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		input, root := testPlanInput(t, integrations.TargetGrok, adapter.Detection{
			Installed: true, ExecutablePath: exe, VersionSupport: adapter.VersionSupported,
			Version: adapter.ClientVersion{Raw: "1.0.30", Major: 1, Minor: 0, Patch: 30, Known: true},
		})
		base, err := adapter.NewRegistration(input.TrackerPath, input.WorkspaceID, adapter.WorkspaceBinding{Kind: adapter.WorkspaceBindingVerifiedCwd}, input.ActorHint, false)
		if err != nil {
			t.Fatal(err)
		}
		expected, err := base.WithToolNameStyle(adapter.ToolNameStylePortable)
		if err != nil {
			t.Fatal(err)
		}
		if write != nil {
			if err := os.MkdirAll(filepath.Join(root, ".grok"), 0o755); err != nil {
				t.Fatal(err)
			}
			write(root, input, expected)
		}
		return item.Plan(context.Background(), input)
	}
	hasAdd := func(plan adapter.IntegrationPlan) bool {
		for _, step := range plan.Steps {
			if step.Kind == adapter.StepRunClientCommand && step.Command != nil && strings.Contains(strings.Join(step.Command.Args, " "), "mcp add") {
				return true
			}
		}
		return false
	}

	t.Run("mixed entry and comment do not skip canonical Atlas", func(t *testing.T) {
		plan, err := planGrok(t, func(root string, input adapter.PlanInput, expected adapter.MCPRegistration) {
			canonical := expected.Args[:len(expected.Args)-2] // drop --tool-name-style portable
			body := "# leftover: --tool-name-style portable\n" +
				tomlMCPServer(expected.ServerName, expected.Command, canonical, "") +
				tomlMCPServer("notes", "/usr/bin/echo", []string{"--tool-name-style", "portable"}, "")
			if err := os.WriteFile(filepath.Join(root, ".grok", "config.toml"), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		if !hasAdd(plan) {
			t.Fatal("canonical Atlas argv must still be re-registered when another entry mentions portable")
		}
	})

	t.Run("wrong derived argv still registers", func(t *testing.T) {
		plan, err := planGrok(t, func(root string, input adapter.PlanInput, expected adapter.MCPRegistration) {
			wrong := append([]string(nil), expected.Args...)
			for i, arg := range wrong {
				if arg == "--max-items" && i+1 < len(wrong) {
					wrong[i+1] = "99"
				}
			}
			body := tomlMCPServer(expected.ServerName, expected.Command, wrong, "")
			if err := os.WriteFile(filepath.Join(root, ".grok", "config.toml"), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		if !hasAdd(plan) {
			t.Fatal("portable style with the wrong bounds must not skip grok mcp add")
		}
	})

	t.Run("exact command and argv skip", func(t *testing.T) {
		plan, err := planGrok(t, func(root string, input adapter.PlanInput, expected adapter.MCPRegistration) {
			body := tomlMCPServer(expected.ServerName, expected.Command, expected.Args, "required = false\n")
			if err := os.WriteFile(filepath.Join(root, ".grok", "config.toml"), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		if hasAdd(plan) {
			t.Fatal("exact matching Atlas argv should skip grok mcp add")
		}
	})

	t.Run("unmanaged same name still refused", func(t *testing.T) {
		_, err := planGrok(t, func(root string, input adapter.PlanInput, expected adapter.MCPRegistration) {
			body := tomlMCPServer(expected.ServerName, "/usr/bin/other", []string{"serve", "--tool-name-style", "portable"}, "")
			if err := os.WriteFile(filepath.Join(root, ".grok", "config.toml"), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		})
		if err == nil {
			t.Fatal("unmanaged same-name Grok server must still be refused")
		}
	})
}

func tomlMCPServer(name, command string, args []string, extra string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = strconv.Quote(arg)
	}
	return fmt.Sprintf("[mcp_servers.%s]\ncommand = %s\nargs = [%s]\n%s", name, strconv.Quote(command), strings.Join(quoted, ", "), extra)
}

func TestCodexProjectRegistrationKeepsCanonicalToolNames(t *testing.T) {
	reg, err := New(Options{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	item, _ := reg.Lookup(integrations.TargetCodex)
	input, _ := testPlanInput(t, integrations.TargetCodex, adapter.Detection{VersionSupport: adapter.VersionUnknown})
	plan, err := item.Plan(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Registration == nil {
		t.Fatal("missing codex registration")
	}
	if plan.Registration.ToolNameStyle != "" && plan.Registration.ToolNameStyle != adapter.ToolNameStyleCanonical {
		t.Fatalf("codex style %q", plan.Registration.ToolNameStyle)
	}
	for _, arg := range plan.Registration.Args {
		if arg == adapter.FlagToolNameStyle || arg == string(adapter.ToolNameStylePortable) {
			t.Fatalf("codex argv leaked portable flag: %v", plan.Registration.Args)
		}
	}
}

func TestClaudeCLIRegistrationUsesDetectedBinary(t *testing.T) {
	reg, err := New(Options{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	item, _ := reg.Lookup(integrations.TargetClaude)
	exe := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	input, _ := testPlanInput(t, integrations.TargetClaude, adapter.Detection{
		Installed: true, ExecutablePath: exe, VersionSupport: adapter.VersionSupported,
		Version: adapter.ClientVersion{Raw: "2.1.219", Major: 2, Minor: 1, Patch: 219, Known: true},
	})
	plan, err := item.Plan(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, step := range plan.Steps {
		if step.Kind == adapter.StepRunClientCommand && step.Command != nil {
			found = true
			if step.Command.Executable != exe {
				t.Fatalf("command exe %s", step.Command.Executable)
			}
			joined := strings.Join(step.Command.Args, " ")
			if !strings.Contains(joined, "mcp add") || strings.Contains(joined, "dangerously") {
				t.Fatalf("args %v", step.Command.Args)
			}
		}
	}
	if !found {
		t.Fatal("expected claude mcp add")
	}
}

func TestGrokWarnsOnCompatibilityDuplicate(t *testing.T) {
	reg, err := New(Options{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	item, _ := reg.Lookup(integrations.TargetGrok)
	other := "atlas-ffffffffffff"
	input, root := testPlanInput(t, integrations.TargetGrok, adapter.Detection{
		VersionSupport:  adapter.VersionUnknown,
		ExistingServers: []adapter.ExistingServer{{Name: other, Scope: adapter.ScopeProjectShared}},
		Reasons:         []string{"compatibility import also loads Atlas server " + other + " from .cursor/mcp.json"},
	})
	if err := os.MkdirAll(filepath.Join(root, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".cursor", "mcp.json"), []byte(`{"mcpServers":{"`+other+`":{"type":"stdio","command":"tracker","args":[]}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := item.Plan(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	warned := false
	for _, w := range plan.Warnings {
		if strings.Contains(w, "duplicate") || strings.Contains(w, "compat") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("warnings %#v", plan.Warnings)
	}
}

func TestInstructionBlocksCoexist(t *testing.T) {
	reg, err := New(Options{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	shared := []integrations.Target{integrations.TargetCodex, integrations.TargetCursor, integrations.TargetOpenClaw, integrations.TargetGrok, integrations.TargetGeneric}
	var root string
	var agents string
	for _, target := range shared {
		item, _ := reg.Lookup(target)
		input, r := testPlanInput(t, target, adapter.Detection{VersionSupport: adapter.VersionUnknown})
		if root == "" {
			root = r
			_ = os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# house rules\n"), 0o644)
		} else {
			input.WorkspaceRoot = root
		}
		plan, err := item.Plan(context.Background(), input)
		if err != nil {
			t.Fatalf("%s plan: %v", target, err)
		}
		if _, err := item.Apply(context.Background(), plan); err != nil {
			t.Fatalf("%s apply: %v", target, err)
		}
	}
	body, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	agents = string(body)
	for _, marker := range []string{"atlas-tasker:begin", "atlas-tasker:cursor:begin", "atlas-tasker:openclaw:begin", "atlas-tasker:grok:begin", "atlas-tasker:generic:begin", "house rules"} {
		if !strings.Contains(agents, marker) {
			t.Fatalf("missing %s in\n%s", marker, agents)
		}
	}
	claude, _ := reg.Lookup(integrations.TargetClaude)
	input, _ := testPlanInput(t, integrations.TargetClaude, adapter.Detection{VersionSupport: adapter.VersionUnknown})
	input.WorkspaceRoot = root
	plan, err := claude.Plan(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := claude.Apply(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}
}

func TestUnsupportedInstalledClientSkipsMCPConfig(t *testing.T) {
	reg, err := New(Options{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	item, _ := reg.Lookup(integrations.TargetCodex)
	exe := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	input, _ := testPlanInput(t, integrations.TargetCodex, adapter.Detection{
		Installed: true, ExecutablePath: exe, VersionSupport: adapter.VersionUnknown,
	})
	plan, err := item.Plan(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ResultingState != adapter.StateUnsupportedClientVersion {
		t.Fatalf("state %s", plan.ResultingState)
	}
	for _, step := range plan.Steps {
		if step.Kind == adapter.StepWriteConfigEntry || step.Kind == adapter.StepRunClientCommand {
			t.Fatalf("unsupported client planned %s", step.Kind)
		}
	}
}

func TestCursorRefusesSymlinkedConfig(t *testing.T) {
	reg, err := New(Options{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	item, _ := reg.Lookup(integrations.TargetCursor)
	input, root := testPlanInput(t, integrations.TargetCursor, adapter.Detection{VersionSupport: adapter.VersionUnknown})
	real := t.TempDir()
	if err := os.Symlink(real, filepath.Join(root, ".cursor")); err != nil {
		t.Fatal(err)
	}
	if _, err := item.Plan(context.Background(), input); err == nil {
		t.Fatal("expected symlink refusal")
	}
}

func TestCodexRefusesMalformedTOML(t *testing.T) {
	reg, err := New(Options{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	item, _ := reg.Lookup(integrations.TargetCodex)
	input, root := testPlanInput(t, integrations.TargetCodex, adapter.Detection{VersionSupport: adapter.VersionUnknown})
	if err := os.MkdirAll(filepath.Join(root, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".codex", "config.toml"), []byte("[[[not toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := item.Plan(context.Background(), input); err == nil {
		t.Fatal("expected malformed TOML refusal")
	}
}

func TestApplyNeverReturnsVerifiedState(t *testing.T) {
	reg, err := New(Options{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	item, _ := reg.Lookup(integrations.TargetOpenClaw)
	exe := filepath.Join(t.TempDir(), "openclaw")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	input, _ := testPlanInput(t, integrations.TargetOpenClaw, adapter.Detection{
		Installed: true, ExecutablePath: exe, VersionSupport: adapter.VersionSupported,
		Version: adapter.ClientVersion{Raw: "1.2.3", Major: 1, Minor: 2, Patch: 3, Known: true},
	})
	plan, err := item.Plan(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ResultingState != adapter.StateConnectedRestartRequired {
		t.Fatalf("plan state %s", plan.ResultingState)
	}
	applied, err := item.Apply(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if applied.State.Verified() {
		t.Fatalf("apply claimed verified %s", applied.State)
	}
}

func TestCodexClientNativeStatusUsesDetectedBinary(t *testing.T) {
	runner := &recordingRunner{}
	reg, err := New(Options{StateDir: t.TempDir(), Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	item, _ := reg.Lookup(integrations.TargetCodex)
	exe := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	input, _ := testPlanInput(t, integrations.TargetCodex, adapter.Detection{
		Installed: true, ExecutablePath: exe, VersionSupport: adapter.VersionSupported,
		Version: adapter.ClientVersion{Raw: "0.45.0", Major: 0, Minor: 45, Patch: 0, Known: true},
	})
	plan, err := item.Plan(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	state := adapter.IntegrationState{
		Target:           integrations.TargetCodex,
		ContractVersion:  adapter.ContractVersion,
		State:            adapter.StatePendingWorkspaceTrust,
		Scope:            plan.Scope,
		WorkspaceID:      input.WorkspaceID,
		WorkspaceRoot:    input.WorkspaceRoot,
		ClientExecutable: exe,
		Registration:     plan.Registration,
		UpdatedAt:        time.Now().UTC(),
	}
	v, err := item.Verify(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if v.State != adapter.StatePendingWorkspaceTrust {
		t.Fatalf("trust pending must survive a native list, got %s", v.State)
	}
	sawList := false
	for _, probe := range v.Probes {
		if probe.Command.Executable == exe && strings.Contains(strings.Join(probe.Command.Args, " "), "mcp list") {
			sawList = true
		}
	}
	if !sawList {
		t.Fatalf("expected codex mcp list, probes %#v", v.Probes)
	}
}

func TestGenericApplyStaysPortableReady(t *testing.T) {
	reg, err := New(Options{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	item, _ := reg.Lookup(integrations.TargetGeneric)
	input, _ := testPlanInput(t, integrations.TargetGeneric, adapter.Detection{VersionSupport: adapter.VersionUnknown})
	plan, err := item.Plan(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	applied, err := item.Apply(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if applied.State != adapter.StatePortableReady {
		t.Fatalf("apply state %s", applied.State)
	}
	v, err := item.Verify(context.Background(), *applied.Record)
	if err != nil {
		t.Fatal(err)
	}
	if v.State != adapter.StatePortableReady {
		t.Fatalf("verify claimed %s without a live conformance probe", v.State)
	}
}

func TestRepairAndRemovePreserveUnrelatedConfig(t *testing.T) {
	reg, err := New(Options{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	item, _ := reg.Lookup(integrations.TargetCursor)
	input, root := testPlanInput(t, integrations.TargetCursor, adapter.Detection{VersionSupport: adapter.VersionUnknown})
	if err := os.MkdirAll(filepath.Join(root, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".cursor", "mcp.json"), []byte(`{"mcpServers":{"other":{"type":"stdio","command":"/usr/bin/other","args":[]}}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := item.Plan(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	applied, err := item.Apply(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, ".cursor", "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"other"`) {
		t.Fatalf("lost unrelated server:\n%s", body)
	}
	if applied.Record == nil || applied.Record.Registration == nil {
		t.Fatal("missing record")
	}
	applied.Record.ConfigPath = filepath.Join(root, ".cursor", "mcp.json")
	applied.Record.WorkspaceID = input.WorkspaceID
	applied.Record.WorkspaceRoot = root
	removal, err := item.Remove(context.Background(), *applied.Record)
	if err != nil {
		t.Fatal(err)
	}
	removed, err := item.Apply(context.Background(), removal.Plan)
	if err != nil {
		t.Fatalf("remove apply: %v steps=%v payloads=%v", err, removal.Plan.Steps, item.(interface{ LastPayloads() map[string][]byte }).LastPayloads())
	}
	if removed.Error != "" {
		t.Fatalf("remove apply error %s outcomes %#v steps %#v", removed.Error, removed.Outcomes, removal.Plan.Steps)
	}
	after, err := os.ReadFile(filepath.Join(root, ".cursor", "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), `"other"`) {
		t.Fatalf("removal deleted unrelated server:\n%s", after)
	}
	if strings.Contains(string(after), applied.Record.Registration.ServerName) {
		t.Fatalf("atlas entry remained:\n%s", after)
	}
}

func TestUnrelatedClientListDoesNotConnect(t *testing.T) {
	runner := &scriptedRunner{handle: func(cmd adapter.Command) (adapter.CommandResult, error) {
		return adapter.CommandResult{Stdout: []byte("other-server\nfilesystem\n")}, nil
	}}
	reg, err := New(Options{StateDir: t.TempDir(), Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	item, _ := reg.Lookup(integrations.TargetClaude)
	exe := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	input, _ := testPlanInput(t, integrations.TargetClaude, adapter.Detection{
		Installed: true, ExecutablePath: exe, VersionSupport: adapter.VersionSupported,
		Version: adapter.ClientVersion{Raw: "2.1.219", Major: 2, Minor: 1, Patch: 219, Known: true},
	})
	plan, err := item.Plan(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	state := adapter.IntegrationState{
		Target: integrations.TargetClaude, ContractVersion: adapter.ContractVersion,
		State: adapter.StateConfiguredUnverified, Scope: plan.Scope,
		WorkspaceID: input.WorkspaceID, WorkspaceRoot: input.WorkspaceRoot,
		ClientExecutable: exe, Registration: plan.Registration, UpdatedAt: time.Now().UTC(),
	}
	v, err := item.Verify(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if v.State.Verified() {
		t.Fatalf("list without this Atlas server must not verify, got %s kind %s", v.State, v.ConnectionKind)
	}
}

func TestDetectUnrelatedListDoesNotPhantomCollision(t *testing.T) {
	runner := &scriptedRunner{handle: func(cmd adapter.Command) (adapter.CommandResult, error) {
		if strings.Contains(strings.Join(cmd.Args, " "), "--version") {
			return adapter.CommandResult{Stdout: []byte("2.1.219")}, nil
		}
		return adapter.CommandResult{Stdout: []byte("other-server\nfilesystem\n")}, nil
	}}
	reg, err := New(Options{StateDir: t.TempDir(), Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	item, _ := reg.Lookup(integrations.TargetClaude)
	exe := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	input, root := testPlanInput(t, integrations.TargetClaude, adapter.Detection{
		Installed: true, ExecutablePath: exe, VersionSupport: adapter.VersionSupported,
		Version: adapter.ClientVersion{Raw: "2.1.219", Major: 2, Minor: 1, Patch: 219, Known: true},
	})
	if _, err := os.Stat(filepath.Join(root, ".tracker", "workspace.json")); err != nil {
		t.Fatal(err)
	}
	detection := item.Detect(context.Background(), adapter.DetectInput{
		WorkspaceRoot: input.WorkspaceRoot, Home: input.Home, Runner: runner,
		LookPath: func(string) (string, error) { return exe, nil },
	})
	name := mustServer(t, input.WorkspaceID)
	for _, existing := range detection.ExistingServers {
		if existing.Name == name {
			t.Fatalf("unrelated list/get invented existing server %#v", existing)
		}
	}
	input.Detection = detection
	if input.Detection.ExecutablePath == "" {
		input.Detection.Installed = true
		input.Detection.ExecutablePath = exe
		input.Detection.VersionSupport = adapter.VersionSupported
		input.Detection.Version = adapter.ClientVersion{Raw: "2.1.219", Major: 2, Minor: 1, Patch: 219, Known: true}
	}
	plan, err := item.Plan(context.Background(), input)
	if err != nil {
		t.Fatalf("plan must not fail on an unrelated client list: %v", err)
	}
	foundAdd := false
	for _, step := range plan.Steps {
		if step.Kind == adapter.StepRunClientCommand && step.Command != nil && strings.Contains(strings.Join(step.Command.Args, " "), "mcp add") {
			foundAdd = true
		}
	}
	if !foundAdd {
		t.Fatal("expected claude mcp add when the client list has no Atlas server")
	}
}

func TestDoctorFailedProbeSentenceIsNotConnected(t *testing.T) {
	name := mustServer(t, "ws-adapter-test")
	reg, err := New(Options{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	item, _ := reg.Lookup(integrations.TargetOpenClaw)
	exe := filepath.Join(t.TempDir(), "openclaw")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	input, _ := testPlanInput(t, integrations.TargetOpenClaw, adapter.Detection{
		Installed: true, ExecutablePath: exe, VersionSupport: adapter.VersionSupported,
		Version: adapter.ClientVersion{Raw: "1.2.3", Major: 1, Minor: 2, Patch: 3, Known: true},
	})
	plan, err := item.Plan(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	runner := &scriptedRunner{handle: func(cmd adapter.Command) (adapter.CommandResult, error) {
		joined := strings.Join(cmd.Args, " ")
		if strings.Contains(joined, "doctor") {
			return adapter.CommandResult{Stdout: []byte(name + " configured but probe failed: connection refused")}, nil
		}
		return adapter.CommandResult{Stdout: []byte(`{"name":"` + name + `","command":"/usr/bin/other"}`)}, nil
	}}
	v, err := openclaw.New().WithStateDir(t.TempDir()).WithRunner(runner).Verify(context.Background(), adapter.IntegrationState{
		Target: integrations.TargetOpenClaw, ContractVersion: adapter.ContractVersion,
		State: adapter.StateConfiguredUnverified, Scope: plan.Scope,
		WorkspaceID: input.WorkspaceID, WorkspaceRoot: input.WorkspaceRoot,
		ClientExecutable: exe, Registration: plan.Registration, UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if v.State == adapter.StateConnected {
		t.Fatalf("failed doctor probe must not report connected, got %s kind %s", v.State, v.ConnectionKind)
	}
}

func TestOpenClawShowIsNotConnected(t *testing.T) {
	name := mustServer(t, "ws-adapter-test")
	reg, err := New(Options{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	item, _ := reg.Lookup(integrations.TargetOpenClaw)
	exe := filepath.Join(t.TempDir(), "openclaw")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	input, _ := testPlanInput(t, integrations.TargetOpenClaw, adapter.Detection{
		Installed: true, ExecutablePath: exe, VersionSupport: adapter.VersionSupported,
		Version: adapter.ClientVersion{Raw: "1.2.3", Major: 1, Minor: 2, Patch: 3, Known: true},
	})
	plan, err := item.Plan(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	runner := &scriptedRunner{handle: func(cmd adapter.Command) (adapter.CommandResult, error) {
		joined := strings.Join(cmd.Args, " ")
		if strings.Contains(joined, "doctor") {
			return adapter.CommandResult{ExitCode: 1, Stdout: []byte("gateway offline")}, fmt.Errorf("doctor failed")
		}
		return adapter.CommandResult{Stdout: []byte(`{"name":"` + name + `","command":"/usr/bin/other"}`)}, nil
	}}
	verifier := openclaw.New().WithStateDir(t.TempDir()).WithRunner(runner)
	state := adapter.IntegrationState{
		Target: integrations.TargetOpenClaw, ContractVersion: adapter.ContractVersion,
		State: adapter.StateConfiguredUnverified, Scope: plan.Scope,
		WorkspaceID: input.WorkspaceID, WorkspaceRoot: input.WorkspaceRoot,
		ClientExecutable: exe, Registration: plan.Registration, UpdatedAt: time.Now().UTC(),
	}
	v, err := verifier.Verify(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if v.State == adapter.StateConnected {
		t.Fatalf("openclaw show must not report connected, got %s kind %s", v.State, v.ConnectionKind)
	}
}

func TestCLIRemovePlansClientCommand(t *testing.T) {
	cases := []struct {
		target integrations.Target
		want   string
	}{
		{integrations.TargetClaude, "mcp remove"},
		{integrations.TargetOpenClaw, "mcp unset"},
		{integrations.TargetGrok, "mcp remove"},
	}
	for _, tc := range cases {
		t.Run(string(tc.target), func(t *testing.T) {
			reg, err := New(Options{StateDir: t.TempDir(), Home: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			item, _ := reg.Lookup(tc.target)
			exe := filepath.Join(t.TempDir(), string(tc.target))
			if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			input, _ := testPlanInput(t, tc.target, adapter.Detection{
				Installed: true, ExecutablePath: exe, VersionSupport: adapter.VersionSupported,
				Version: adapter.ClientVersion{Raw: "1.0.0", Major: 1, Minor: 0, Patch: 0, Known: true},
			})
			plan, err := item.Plan(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Registration == nil {
				t.Fatal("missing registration")
			}
			state := adapter.IntegrationState{
				Target: tc.target, ContractVersion: adapter.ContractVersion,
				State: adapter.StateConfiguredUnverified, Scope: plan.Scope,
				WorkspaceID: input.WorkspaceID, WorkspaceRoot: input.WorkspaceRoot,
				ClientExecutable: exe, ClientVersion: input.Detection.Version,
				Registration: plan.Registration, UpdatedAt: time.Now().UTC(),
			}
			removal, err := item.Remove(context.Background(), state)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, step := range removal.Plan.Steps {
				if step.Kind == adapter.StepRunClientCommand && step.Command != nil {
					found = true
					if step.Command.Executable != exe {
						t.Fatalf("remove exe %s", step.Command.Executable)
					}
					joined := strings.Join(step.Command.Args, " ")
					if !strings.Contains(joined, tc.want) {
						t.Fatalf("args %v", step.Command.Args)
					}
					if !strings.Contains(joined, plan.Registration.ServerName) {
						t.Fatalf("remove missing server name %s in %v", plan.Registration.ServerName, step.Command.Args)
					}
				}
			}
			if !found {
				t.Fatalf("expected %s CLI remove, steps %#v", tc.target, removal.Plan.Steps)
			}
		})
	}
}

func TestCodexTOMLUnmanagedSameNameRefusesOverwrite(t *testing.T) {
	reg, err := New(Options{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	item, _ := reg.Lookup(integrations.TargetCodex)
	input, root := testPlanInput(t, integrations.TargetCodex, adapter.Detection{VersionSupport: adapter.VersionUnknown})
	name := mustServer(t, input.WorkspaceID)
	if err := os.MkdirAll(filepath.Join(root, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".codex", "config.toml"), []byte("[mcp_servers."+name+"]\ncommand = \"/usr/bin/other\"\nargs = [\"serve\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".tracker", "workspace.json"), []byte(`{"workspace_id":"ws-adapter-test"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := item.Plan(context.Background(), input); err == nil {
		t.Fatal("expected unmanaged Codex TOML same-name refusal")
	}
}

func TestOpenClawLocalStateDoesNotRecordVerifiedState(t *testing.T) {
	reg, err := New(Options{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	item, _ := reg.Lookup(integrations.TargetOpenClaw)
	exe := filepath.Join(t.TempDir(), "openclaw")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	input, _ := testPlanInput(t, integrations.TargetOpenClaw, adapter.Detection{
		Installed: true, ExecutablePath: exe, VersionSupport: adapter.VersionSupported,
		Version: adapter.ClientVersion{Raw: "1.2.3", Major: 1, Minor: 2, Patch: 3, Known: true},
	})
	plan, err := item.Plan(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ResultingState != adapter.StateConnectedRestartRequired {
		t.Fatalf("plan state %s", plan.ResultingState)
	}
	src, ok := item.(interface{ LastPayloads() map[string][]byte })
	if !ok {
		t.Fatal("missing payloads")
	}
	var recorded adapter.IntegrationState
	found := false
	for id, body := range src.LastPayloads() {
		if !strings.HasPrefix(id, "state-") {
			continue
		}
		found = true
		if err := json.Unmarshal(body, &recorded); err != nil {
			t.Fatal(err)
		}
	}
	if !found {
		t.Fatal("missing local state payload")
	}
	if recorded.State.Verified() {
		t.Fatalf("local state recorded verified %s", recorded.State)
	}
}

func TestRemoveDeletesGeneratedContext(t *testing.T) {
	reg, err := New(Options{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	item, _ := reg.Lookup(integrations.TargetGeneric)
	input, _ := testPlanInput(t, integrations.TargetGeneric, adapter.Detection{VersionSupport: adapter.VersionUnknown})
	plan, err := item.Plan(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := item.Apply(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	contextPath := filepath.Join(input.WorkspaceRoot, ".tracker", "integrations", "generic-agent-skill", "references", "atlas-context.md")
	if _, err := os.Stat(contextPath); err != nil {
		t.Fatal(err)
	}
	state := adapter.IntegrationState{
		Target: integrations.TargetGeneric, ContractVersion: adapter.ContractVersion,
		State: adapter.StatePortableReady, Scope: plan.Scope,
		WorkspaceID: input.WorkspaceID, WorkspaceRoot: input.WorkspaceRoot,
		Registration: plan.Registration, UpdatedAt: time.Now().UTC(),
	}
	removal, err := item.Remove(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, step := range removal.Plan.Steps {
		if step.Path == contextPath && step.Kind == adapter.StepRemoveManagedFile {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected context file removal, steps %#v", removal.Plan.Steps)
	}
}

type recordingRunner struct {
	calls []adapter.Command
}

func (r *recordingRunner) Run(_ context.Context, cmd adapter.Command) (adapter.CommandResult, error) {
	r.calls = append(r.calls, cmd)
	return adapter.CommandResult{Stdout: []byte("ok")}, nil
}

type scriptedRunner struct {
	calls  []adapter.Command
	handle func(cmd adapter.Command) (adapter.CommandResult, error)
}

func (r *scriptedRunner) Run(_ context.Context, cmd adapter.Command) (adapter.CommandResult, error) {
	r.calls = append(r.calls, cmd)
	if r.handle != nil {
		return r.handle(cmd)
	}
	return adapter.CommandResult{Stdout: []byte("ok")}, nil
}

func mustServer(t *testing.T, id string) string {
	t.Helper()
	name, err := adapter.ServerNameFor(id)
	if err != nil {
		t.Fatal(err)
	}
	return name
}
