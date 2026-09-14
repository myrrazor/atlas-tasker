package adapter

import (
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
)

const (
	testRoot      = "/srv/workspace"
	testHome      = "/home/someone"
	testStateRoot = "/home/someone/.local/state/atlas-tasker"
	testTracker   = "/usr/local/bin/tracker"
)

func codexDetection() Detection {
	return Detection{
		Target:         integrations.TargetCodex,
		Installed:      true,
		ExecutablePath: "/usr/local/bin/codex",
		Version:        ClientVersion{Raw: "codex-cli 0.120.0", Major: 0, Minor: 120, Patch: 0, Known: true},
		VersionSupport: VersionSupported,
		ConfigLocations: []ConfigLocation{
			{Scope: ScopeProjectShared, Path: testRoot + "/.codex/config.toml", Exists: false, Format: ConfigFormatTOML},
			{Scope: ScopeUser, Path: testHome + "/.codex/config.toml", Exists: true, Format: ConfigFormatTOML},
		},
		Reasons: []string{"binary on PATH (/usr/local/bin/codex)"},
	}
}

func snapshotRollback(path string) *RollbackAction {
	return &RollbackAction{Kind: RollbackRestoreSnapshot, Path: path}
}

func codexPlan(t *testing.T) IntegrationPlan {
	t.Helper()
	reg, err := NewRegistration(testTracker, testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingVerifiedCwd}, "agent:codex-1", false)
	if err != nil {
		t.Fatal(err)
	}
	inspect := Command{Purpose: CommandPurposeInspect, Executable: "/usr/local/bin/codex", Args: []string{"mcp", "list", "--json"}, Dir: testRoot, Timeout: 20 * time.Second}
	return IntegrationPlan{
		PlanID:          "plan-1",
		ContractVersion: ContractVersion,
		Operation:       PlanOperationSetup,
		Target:          integrations.TargetCodex,
		WorkspaceID:     testWorkspaceID,
		WorkspaceRoot:   testRoot,
		Home:            testHome,
		LocalStateRoot:  testStateRoot,
		Scope:           ScopeProjectShared,
		Detection:       codexDetection(),
		Registration:    &reg,
		Steps: []PlanStep{
			{StepID: "block", Kind: StepUpdateManagedBlock, Description: "refresh codex managed block", Path: testRoot + "/AGENTS.md", Reversibility: Reversible, Rollback: snapshotRollback(testRoot + "/AGENTS.md"), Mode: 0o644},
			{StepID: "skill", Kind: StepWriteManagedFile, Description: "write atlas-worker skill", Path: testRoot + "/.codex/skills/atlas-worker/SKILL.md", Reversibility: Reversible, Rollback: &RollbackAction{Kind: RollbackDeleteCreated, Path: testRoot + "/.codex/skills/atlas-worker/SKILL.md"}, Mode: 0o644},
			{StepID: "config", Kind: StepWriteConfigEntry, Description: "add [mcp_servers." + reg.ServerName + "]", Path: testRoot + "/.codex/config.toml", Scope: ScopeProjectShared, Reversibility: Reversible, Rollback: snapshotRollback(testRoot + "/.codex/config.toml"), Mode: 0o644},
			{StepID: "inspect", Kind: StepRunClientCommand, Description: "list servers", Command: &inspect},
			{StepID: "state", Kind: StepRecordLocalState, Description: "record integration state", Path: testStateRoot + "/workspaces/" + testWorkspaceID + "/codex.json", Reversibility: Reversible, Rollback: snapshotRollback(testStateRoot + "/workspaces/" + testWorkspaceID + "/codex.json"), Mode: 0o600},
		},
		ApprovalSteps:  []ApprovalStep{{Requirement: ApprovalWorkspaceTrust, Instruction: "Trust this project in Codex so .codex/config.toml is loaded."}},
		ResultingState: StatePendingWorkspaceTrust,
		GeneratedAt:    time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC),
	}
}

func TestPlanFixtureValidatesAndFingerprintIsStable(t *testing.T) {
	plan := codexPlan(t)
	if err := plan.Validate(); err != nil {
		t.Fatalf("fixture plan rejected: %v", err)
	}
	fp1, err := plan.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	again := codexPlan(t)
	again.PlanID = "plan-2"
	again.GeneratedAt = again.GeneratedAt.Add(time.Hour)
	fp2, _ := again.Fingerprint()
	if fp1 != fp2 {
		t.Fatal("fingerprint must ignore plan id and generation time")
	}
	again.Steps[0].Description = "different"
	fp3, _ := again.Fingerprint()
	if fp1 == fp3 {
		t.Fatal("fingerprint must change with plan content")
	}
}

func TestPlanValidateRejects(t *testing.T) {
	cases := map[string]func(*IntegrationPlan){
		"contract version": func(p *IntegrationPlan) { p.ContractVersion = 99 },
		"operation":        func(p *IntegrationPlan) { p.Operation = "install" },
		"remove op with state": func(p *IntegrationPlan) {
			p.Operation = PlanOperationRemove
		},
		"unknown target": func(p *IntegrationPlan) { p.Target = "emacs" },
		"detection target mismatch": func(p *IntegrationPlan) {
			p.Detection.Target = integrations.TargetClaude
		},
		"relative root": func(p *IntegrationPlan) { p.WorkspaceRoot = "workspace" },
		"bad scope":     func(p *IntegrationPlan) { p.Scope = "everywhere" },
		"bad state":     func(p *IntegrationPlan) { p.ResultingState = "ready" },
		"registration other workspace": func(p *IntegrationPlan) {
			reg, _ := NewRegistration(testTracker, "other-workspace", WorkspaceBinding{Kind: WorkspaceBindingVerifiedCwd}, "", false)
			p.Registration = &reg
		},
		"absolute binding in repo scope": func(p *IntegrationPlan) {
			reg, _ := NewRegistration(testTracker, testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingAbsolutePath, WorkspaceRoot: testRoot}, "", false)
			p.Registration = &reg
		},
		"home executable in repo scope": func(p *IntegrationPlan) {
			reg, _ := NewRegistration(testHome+"/bin/tracker", testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingVerifiedCwd}, "", false)
			p.Registration = &reg
		},
		"approval but promises connected": func(p *IntegrationPlan) { p.ResultingState = StateConnected },
		"approval without instruction": func(p *IntegrationPlan) {
			p.ApprovalSteps[0].Instruction = " "
		},
		"approval step that is not a human gate": func(p *IntegrationPlan) {
			p.ApprovalSteps[0].Requirement = ApprovalNone
		},
		"connected without registration": func(p *IntegrationPlan) {
			p.ApprovalSteps = nil
			p.Registration = nil
			p.ResultingState = StateConnected
		},
		"unknown version touches config": func(p *IntegrationPlan) {
			p.Detection.VersionSupport = VersionUnknown
			p.Detection.Version.Known = false
			p.ResultingState = StateUnsupportedClientVersion
			p.ApprovalSteps = nil
		},
		"unknown version promises connected": func(p *IntegrationPlan) {
			p.Detection.VersionSupport = VersionUnknown
			p.Detection.Version.Known = false
			p.ApprovalSteps = nil
			p.ResultingState = StateConnected
			p.Steps = p.Steps[:2]
		},
		"not installed but connected": func(p *IntegrationPlan) {
			p.Detection.Installed = false
			p.Detection.ExecutablePath = ""
			p.Detection.VersionSupport = VersionUnknown
			p.Detection.Version.Known = false
			p.ApprovalSteps = nil
			p.ResultingState = StateConnected
			p.Steps = p.Steps[:2]
		},
		"write without rollback": func(p *IntegrationPlan) { p.Steps[0].Rollback = nil },
		"reversible with irreversible reason": func(p *IntegrationPlan) {
			p.Steps[0].IrreversibleReason = "why"
		},
		"irreversible without reason": func(p *IntegrationPlan) {
			p.Steps[0].Reversibility = Irreversible
			p.Steps[0].Rollback = nil
		},
		"irreversible with rollback": func(p *IntegrationPlan) {
			p.Steps[0].Reversibility = Irreversible
			p.Steps[0].IrreversibleReason = "client cli has no undo"
		},
		"path outside workspace":  func(p *IntegrationPlan) { p.Steps[0].Path = "/etc/AGENTS.md" },
		"path is workspace root":  func(p *IntegrationPlan) { p.Steps[0].Path = testRoot },
		"relative path":           func(p *IntegrationPlan) { p.Steps[0].Path = "AGENTS.md" },
		"unclean path":            func(p *IntegrationPlan) { p.Steps[0].Path = testRoot + "/./AGENTS.md" },
		"traversal path":          func(p *IntegrationPlan) { p.Steps[0].Path = testRoot + "/../AGENTS.md" },
		"file step with command":  func(p *IntegrationPlan) { p.Steps[0].Command = p.Steps[3].Command },
		"command step with path":  func(p *IntegrationPlan) { p.Steps[3].Path = testRoot + "/x" },
		"command step no command": func(p *IntegrationPlan) { p.Steps[3].Command = nil },
		"shell command": func(p *IntegrationPlan) {
			shell := *p.Steps[3].Command
			shell.Executable = "/bin/sh"
			shell.Args = []string{"-c", "codex mcp list"}
			p.Steps[3].Command = &shell
		},
		"mutating command without rollback": func(p *IntegrationPlan) {
			add := *p.Steps[3].Command
			add.Purpose = CommandPurposeRegister
			p.Steps[3].Command = &add
		},
		"local state outside root": func(p *IntegrationPlan) {
			p.Steps[4].Path = testRoot + "/.tracker/state.json"
			p.Steps[4].Rollback.Path = p.Steps[4].Path
		},
		"local state wrong mode":   func(p *IntegrationPlan) { p.Steps[4].Mode = 0o644 },
		"local state root missing": func(p *IntegrationPlan) { p.LocalStateRoot = "" },
		"local state root inside workspace": func(p *IntegrationPlan) {
			p.LocalStateRoot = testRoot + "/.tracker/local"
			p.Steps[4].Path = testRoot + "/.tracker/local/codex.json"
			p.Steps[4].Rollback.Path = p.Steps[4].Path
		},
		"duplicate step id": func(p *IntegrationPlan) { p.Steps[1].StepID = p.Steps[0].StepID },
		"empty step id":     func(p *IntegrationPlan) { p.Steps[1].StepID = " " },
		"no description":    func(p *IntegrationPlan) { p.Steps[1].Description = "" },
		"credential in description": func(p *IntegrationPlan) {
			p.Steps[1].Description = "push to https://u:p@example.com/x"
		},
		"credential in warning": func(p *IntegrationPlan) { p.Warnings = []string{"https://u:p@example.com"} },
		"noop with writes":      func(p *IntegrationPlan) { p.NoOp = true },
		"writes nothing but not noop": func(p *IntegrationPlan) {
			p.Steps = nil
			p.ApprovalSteps = nil
			p.ResultingState = StateConfiguredUnverified
		},
		"consented root overlaps workspace": func(p *IntegrationPlan) { p.ConsentedRoots = []string{testRoot + "/sub"} },
		"relative consented root":           func(p *IntegrationPlan) { p.ConsentedRoots = []string{"relative"} },
		"relative home":                     func(p *IntegrationPlan) { p.Home = "home" },
		"portable render with rollback": func(p *IntegrationPlan) {
			p.Steps = append(p.Steps, PlanStep{StepID: "render", Kind: StepRenderPortableEntry, Description: "show descriptor", Rollback: snapshotRollback("/x")})
		},
	}
	for name, mutate := range cases {
		plan := codexPlan(t)
		mutate(&plan)
		if err := plan.Validate(); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
}

func TestPlanUnsupportedVersionMayOnlyTouchAtlasFiles(t *testing.T) {
	plan := codexPlan(t)
	plan.Detection.VersionSupport = VersionUnknown
	plan.Detection.Version.Known = false
	plan.Registration = nil
	plan.ApprovalSteps = nil
	plan.ResultingState = StateUnsupportedClientVersion
	plan.Steps = plan.Steps[:2]
	plan.Warnings = []string{"codex version could not be parsed; client configuration left untouched"}
	if err := plan.Validate(); err != nil {
		t.Fatalf("unsupported-version plan limited to Atlas files must validate: %v", err)
	}
	plan.Steps = nil
	if err := plan.Validate(); err != nil {
		t.Fatalf("an unsupported-version plan may write nothing without being no-op: %v", err)
	}
}

func TestPlanNoOpAndConsentedRoots(t *testing.T) {
	plan := codexPlan(t)
	plan.Steps = []PlanStep{{StepID: "inspect", Kind: StepRunClientCommand, Description: "list", Command: plan.Steps[3].Command}}
	plan.ApprovalSteps = nil
	plan.ResultingState = StateConfiguredUnverified
	plan.NoOp = true
	if err := plan.Validate(); err != nil {
		t.Fatalf("no-op plan with a read-only inspect must validate: %v", err)
	}
	plan = codexPlan(t)
	plan.ConsentedRoots = []string{"/etc/atlas-consented"}
	plan.Steps[1].Path = "/etc/atlas-consented/atlas-mcp.json"
	plan.Steps[1].Rollback.Path = plan.Steps[1].Path
	if err := plan.Validate(); err != nil {
		t.Fatalf("consented root must allow writes: %v", err)
	}
	plan.Steps[1].Path = "/etc/atlas-consented"
	plan.Steps[1].Rollback.Path = plan.Steps[1].Path
	if err := plan.Validate(); err == nil {
		t.Fatal("the consented root itself is not a writable file path")
	}
}

func TestGenericPlanIsCappedAtPortableReady(t *testing.T) {
	reg, err := NewRegistration(PortableExecutableName, testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingVerifiedCwd}, "", true)
	if err != nil {
		t.Fatal(err)
	}
	plan := IntegrationPlan{
		PlanID: "generic-1", ContractVersion: ContractVersion, Operation: PlanOperationSetup, Target: integrations.TargetGeneric, WorkspaceID: testWorkspaceID, WorkspaceRoot: testRoot,
		Scope:        ScopeProjectShared,
		Detection:    Detection{Target: integrations.TargetGeneric, VersionSupport: VersionUnknown},
		Registration: &reg,
		Steps: []PlanStep{
			{StepID: "descriptor", Kind: StepWriteManagedFile, Description: "portable descriptor", Path: testRoot + "/.tracker/integrations/atlas-mcp.json", Reversibility: Reversible, Rollback: &RollbackAction{Kind: RollbackDeleteCreated, Path: testRoot + "/.tracker/integrations/atlas-mcp.json"}, Mode: 0o644},
			{StepID: "render", Kind: StepRenderPortableEntry, Description: "print descriptor"},
		},
		ResultingState: StatePortableReady,
		GeneratedAt:    time.Now(),
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("generic portable plan rejected: %v", err)
	}
	plan.ResultingState = StateConnected
	if err := plan.Validate(); err == nil || !strings.Contains(err.Error(), "capability cap") {
		t.Fatalf("generic plan must not promise connected, got %v", err)
	}
	plan.ResultingState = StateConfiguredUnverified
	if err := plan.Validate(); err != nil {
		t.Fatalf("generic may plan configured_unverified: %v", err)
	}
}

func TestApplyResultValidation(t *testing.T) {
	reg, _ := NewRegistration(testTracker, testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingVerifiedCwd}, "agent:codex-1", false)
	now := time.Now()
	record := IntegrationState{
		Target: integrations.TargetCodex, ContractVersion: ContractVersion, State: StateConfiguredUnverified, Scope: ScopeProjectShared,
		WorkspaceID: testWorkspaceID, WorkspaceRoot: testRoot, ConfigPath: testRoot + "/.codex/config.toml", Registration: &reg, EntryFingerprint: reg.Fingerprint(),
		ClientVersion: ClientVersion{Known: true, Major: 0, Minor: 120}, ActorHint: "agent:codex-1", UpdatedAt: now,
	}
	ok := ApplyResult{PlanID: "plan-1", Target: integrations.TargetCodex, State: StateConfiguredUnverified, Outcomes: []StepOutcome{{StepID: "block", Status: StepApplied}}, Record: &record}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid apply result rejected: %v", err)
	}
	connected := ok
	connected.State = StateConnected
	if err := connected.Validate(); err == nil {
		t.Fatal("apply must never report connected")
	}
	failed := ApplyResult{PlanID: "plan-1", Target: integrations.TargetCodex, State: StateFailed, Outcomes: []StepOutcome{{StepID: "block", Status: StepFailed, Error: "disk full"}, {StepID: "skill", Status: StepRolledBack}}}
	if err := failed.Validate(); err != nil {
		t.Fatalf("failed apply rejected: %v", err)
	}
	failed.Record = &record
	if err := failed.Validate(); err == nil {
		t.Fatal("failed apply must not persist a record")
	}
	noRecord := ok
	noRecord.Record = nil
	if err := noRecord.Validate(); err == nil {
		t.Fatal("successful apply must produce a record")
	}
	mismatch := ok
	other := record
	other.State = StatePortableReady
	mismatch.Record = &other
	if err := mismatch.Validate(); err == nil {
		t.Fatal("record state must match apply state")
	}
	noError := ok
	noError.Outcomes = []StepOutcome{{StepID: "x", Status: StepFailed}}
	if err := noError.Validate(); err == nil {
		t.Fatal("failed step outcome needs an error")
	}
}

func TestIntegrationStateValidation(t *testing.T) {
	reg, _ := NewRegistration(testTracker, testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingVerifiedCwd}, "", false)
	now := time.Now()
	good := IntegrationState{Target: integrations.TargetCodex, ContractVersion: ContractVersion, State: StateConnected, Scope: ScopeProjectShared, WorkspaceID: testWorkspaceID, WorkspaceRoot: testRoot, Registration: &reg, EntryFingerprint: reg.Fingerprint(), LastVerifiedAt: now, UpdatedAt: now}
	if err := good.Validate(); err != nil {
		t.Fatalf("valid state rejected: %v", err)
	}
	cases := map[string]func(*IntegrationState){
		"contract":       func(s *IntegrationState) { s.ContractVersion = 2 },
		"state":          func(s *IntegrationState) { s.State = "ok" },
		"scope":          func(s *IntegrationState) { s.Scope = "x" },
		"workspace id":   func(s *IntegrationState) { s.WorkspaceID = "" },
		"relative root":  func(s *IntegrationState) { s.WorkspaceRoot = "ws" },
		"no fingerprint": func(s *IntegrationState) { s.EntryFingerprint = "" },
		"other workspace reg": func(s *IntegrationState) {
			r, _ := NewRegistration(testTracker, "other", WorkspaceBinding{Kind: WorkspaceBindingVerifiedCwd}, "", false)
			s.Registration = &r
		},
		"connected unverified": func(s *IntegrationState) {
			s.LastVerifiedAt = time.Time{}
		},
		"repair without reason": func(s *IntegrationState) { s.State = StateRepairRequired },
		"no updated_at":         func(s *IntegrationState) { s.UpdatedAt = time.Time{} },
	}
	for name, mutate := range cases {
		state := good
		mutate(&state)
		if err := state.Validate(); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
}

func TestVerificationValidation(t *testing.T) {
	now := time.Now()
	probe := VerificationCheck{Name: "self_probe.tools_list", Method: VerificationSelfProbe, Passed: true}
	manual := VerificationCheck{Name: "user confirmed in Customize", Method: VerificationManualClientCheck, Passed: true}
	good := Verification{Target: integrations.TargetCursor, State: StateConnected, ConnectionKind: ConnectionKindSelfProbe, Checks: []VerificationCheck{probe}, CheckedAt: now}
	if err := good.Validate(); err != nil {
		t.Fatalf("valid verification rejected: %v", err)
	}
	manualOnly := good
	manualOnly.Checks = []VerificationCheck{manual}
	if err := manualOnly.Validate(); err == nil {
		t.Fatal("a manual client check alone cannot produce connected")
	}
	failedProbe := good
	failedProbe.Checks = []VerificationCheck{{Name: "self_probe", Method: VerificationSelfProbe, Passed: false, Detail: "exit 1"}}
	if err := failedProbe.Validate(); err == nil {
		t.Fatal("connected requires a passed check")
	}
	failedProbe.State = StateFailed
	if err := failedProbe.Validate(); err != nil {
		t.Fatalf("failed verification with failed probe must validate: %v", err)
	}
	noKind := good
	noKind.ConnectionKind = ""
	if err := noKind.Validate(); err == nil {
		t.Fatal("connected requires a connection kind")
	}
	generic := good
	generic.Target = integrations.TargetGeneric
	generic.ConnectionKind = ConnectionKindSelfProbe
	if err := generic.Validate(); err == nil {
		t.Fatal("generic connections must be standard_config or custom_adapter")
	}
	generic.ConnectionKind = ConnectionKindStandardConfig
	generic.Checks = []VerificationCheck{{Name: "conformance host", Method: VerificationConformanceHost, Passed: true}}
	if err := generic.Validate(); err != nil {
		t.Fatalf("generic conformance connection rejected: %v", err)
	}
	noTime := good
	noTime.CheckedAt = time.Time{}
	if err := noTime.Validate(); err == nil {
		t.Fatal("checked_at is required")
	}
	mutating := good
	mutating.Probes = []ProbeRecord{{Command: Command{Purpose: CommandPurposeRegister, Executable: "/usr/local/bin/codex", Args: []string{"mcp", "add"}, Timeout: time.Second}}}
	if err := mutating.Validate(); err == nil {
		t.Fatal("verification probes must be read-only")
	}
}

func TestRepairAndRemovalPlans(t *testing.T) {
	plan := codexPlan(t)
	repair := RepairPlan{Reason: "tracker binary moved", Plan: plan}
	if err := repair.Validate(); err == nil {
		t.Fatal("repair plan must carry the repair operation")
	}
	plan.Operation = PlanOperationRepair
	repair.Plan = plan
	if err := repair.Validate(); err != nil {
		t.Fatalf("repair plan rejected: %v", err)
	}
	repair.Reason = ""
	if err := repair.Validate(); err == nil {
		t.Fatal("repair needs a reason")
	}
	removalPlan := codexPlan(t)
	removalPlan.Operation = PlanOperationRemove
	removalPlan.Registration = nil
	removalPlan.ApprovalSteps = nil
	removalPlan.ResultingState = ""
	removalPlan.Steps = []PlanStep{
		{StepID: "block", Kind: StepRemoveManagedBlock, Description: "remove codex block", Path: testRoot + "/AGENTS.md", Reversibility: Reversible, Rollback: snapshotRollback(testRoot + "/AGENTS.md")},
		{StepID: "config", Kind: StepRemoveConfigEntry, Description: "remove managed table", Path: testRoot + "/.codex/config.toml", Scope: ScopeProjectShared, Reversibility: Reversible, Rollback: snapshotRollback(testRoot + "/.codex/config.toml")},
	}
	removal := RemovalPlan{Plan: removalPlan, OwnershipVerified: true}
	if err := removal.Validate(); err != nil {
		t.Fatalf("removal plan rejected: %v", err)
	}
	removal.OwnershipVerified = false
	if err := removal.Validate(); err == nil {
		t.Fatal("unverified ownership must require confirmation")
	}
	removal.RequiresConfirmation = true
	if err := removal.Validate(); err != nil {
		t.Fatalf("removal with confirmation rejected: %v", err)
	}
	removal.Plan.Steps = append(removal.Plan.Steps, PlanStep{StepID: "write", Kind: StepWriteConfigEntry, Description: "sneaky write", Path: testRoot + "/.codex/config.toml", Scope: ScopeProjectShared, Reversibility: Reversible, Rollback: snapshotRollback(testRoot + "/.codex/config.toml"), Mode: 0o644})
	if err := removal.Validate(); err == nil {
		t.Fatal("removal plans may not write config entries")
	}
	removal.Plan.Steps = removal.Plan.Steps[:2]
	removal.Plan.ResultingState = StateFailed
	if err := removal.Validate(); err == nil {
		t.Fatal("removal plans carry no resulting state")
	}
	removal.Plan.ResultingState = ""
	reg, _ := NewRegistration(testTracker, testWorkspaceID, WorkspaceBinding{Kind: WorkspaceBindingVerifiedCwd}, "", false)
	removal.Plan.Registration = &reg
	if err := removal.Validate(); err == nil {
		t.Fatal("removal plans do not register servers")
	}
	removal.Plan.Registration = nil
	unregister := Command{Purpose: CommandPurposeRegister, Executable: "/usr/local/bin/codex", Args: []string{"mcp", "add"}, Timeout: time.Second}
	removal.Plan.Steps = append(removal.Plan.Steps, PlanStep{StepID: "cmd", Kind: StepRunClientCommand, Description: "add", Command: &unregister, Reversibility: Irreversible, IrreversibleReason: "n/a"})
	if err := removal.Validate(); err == nil {
		t.Fatal("removal plans may not run register commands")
	}
	empty := RemovalPlan{Plan: removal.Plan, OwnershipVerified: true}
	empty.Plan.Steps = nil
	empty.Plan.NoOp = true
	if err := empty.Validate(); err != nil {
		t.Fatalf("a no-op removal (nothing owned on disk) must validate: %v", err)
	}
}

func TestDetectionValidation(t *testing.T) {
	good := codexDetection()
	if err := good.Validate(); err != nil {
		t.Fatalf("valid detection rejected: %v", err)
	}
	cases := map[string]func(*Detection){
		"unknown target":            func(d *Detection) { d.Target = "emacs" },
		"bad support":               func(d *Detection) { d.VersionSupport = "maybe" },
		"installed without path":    func(d *Detection) { d.ExecutablePath = "" },
		"relative path":             func(d *Detection) { d.ExecutablePath = "codex" },
		"not installed but support": func(d *Detection) { d.Installed = false; d.ExecutablePath = "" },
		"supported unknown version": func(d *Detection) { d.Version.Known = false },
	}
	for name, mutate := range cases {
		d := codexDetection()
		mutate(&d)
		if err := d.Validate(); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
}

func TestManagedFileWriteRequiresHomeOutsideWorkspace(t *testing.T) {
	plan := codexPlan(t)
	plan.Home = ""
	plan.ConsentedRoots = []string{"/home/victim/.cursor"}
	plan.Steps[1].Kind = StepWriteManagedFile
	plan.Steps[1].Path = "/home/victim/.cursor/skills/atlas-worker/SKILL.md"
	plan.Steps[1].Rollback.Path = plan.Steps[1].Path
	if err := plan.Validate(); err == nil {
		t.Fatal("write_managed_file outside the workspace must fail when home is unknown")
	}
}

func TestPlanInputValidation(t *testing.T) {
	good := PlanInput{WorkspaceRoot: testRoot, WorkspaceID: testWorkspaceID, Home: testHome, TrackerPath: testTracker, ActorHint: "agent:codex-1", Detection: codexDetection()}
	if err := good.Validate(); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}
	cases := map[string]func(*PlanInput){
		"relative root":   func(p *PlanInput) { p.WorkspaceRoot = "ws" },
		"no workspace id": func(p *PlanInput) { p.WorkspaceID = "" },
		"relative tracker": func(p *PlanInput) {
			p.TrackerPath = "tracker"
		},
		"bad actor":      func(p *PlanInput) { p.ActorHint = "someone" },
		"bad scope":      func(p *PlanInput) { p.Scope = "x" },
		"relative root2": func(p *PlanInput) { p.ConsentedRoots = []string{"x"} },
	}
	for name, mutate := range cases {
		input := good
		mutate(&input)
		if err := input.Validate(); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
}
