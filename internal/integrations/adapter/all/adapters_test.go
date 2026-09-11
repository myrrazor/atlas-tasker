package all

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
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
	name := mustServer(t, "ws-adapter-test")
	input, root := testPlanInput(t, integrations.TargetGrok, adapter.Detection{
		VersionSupport:  adapter.VersionUnknown,
		ExistingServers: []adapter.ExistingServer{{Name: name, Scope: adapter.ScopeProjectShared, AtlasOwned: true}},
		Reasons:         []string{"compatibility import also loads Atlas server " + name + " from .cursor/mcp.json"},
	})
	if err := os.MkdirAll(filepath.Join(root, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".cursor", "mcp.json"), []byte(`{"mcpServers":{"`+name+`":{"type":"stdio","command":"tracker","args":[]}}}`), 0o644); err != nil {
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

type recordingRunner struct {
	calls []adapter.Command
}

func (r *recordingRunner) Run(_ context.Context, cmd adapter.Command) (adapter.CommandResult, error) {
	r.calls = append(r.calls, cmd)
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
