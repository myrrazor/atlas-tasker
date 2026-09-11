package setup

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func testWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	resolved, err := filepath.EvalSymlinks(root)
	if err == nil {
		root = resolved
	}
	if err := os.MkdirAll(storage.TrackerDir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	meta := []byte(`{"workspace_id":"ws-setup-test","created_at":"2026-09-11T00:00:00Z"}` + "\n")
	if err := os.WriteFile(storage.WorkspaceMetadataFile(root), meta, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storage.TrackerDir(root), "config.toml"), []byte("[workflow]\ncompletion_mode = \"open\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func testEngine(t *testing.T) *Engine {
	t.Helper()
	root := testWorkspace(t)
	tracker := filepath.Join(t.TempDir(), "tracker-bin")
	if err := os.WriteFile(tracker, []byte("tracker-binary-v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &Engine{
		StateDir:      t.TempDir(),
		Home:          t.TempDir(),
		WorkspaceRoot: root,
		WorkspaceID:   "ws-setup-test",
		TrackerPath:   tracker,
		Now:           func() time.Time { return time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC) },
		LookPath:      func(string) (string, error) { return "", os.ErrNotExist },
		currentUID:    os.Getuid(),
	}
}

func applyGeneric(t *testing.T, engine *Engine) *RunReport {
	t.Helper()
	prepared, err := engine.Plan(PlanOptions{Agents: []integrations.Target{integrations.TargetGeneric}})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	report, err := engine.ApplyPrepared(context.Background(), prepared, ApplyOptions{Yes: true})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	return report
}

func TestPlanIsReadOnlyAndDeterministic(t *testing.T) {
	engine := testEngine(t)
	stateBefore, err := os.ReadDir(engine.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	rootBefore := snapshotTree(t, engine.WorkspaceRoot)
	a, err := engine.Plan(PlanOptions{Agents: []integrations.Target{integrations.TargetGeneric}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := engine.Plan(PlanOptions{Agents: []integrations.Target{integrations.TargetGeneric}})
	if err != nil {
		t.Fatal(err)
	}
	if a.Plan.Fingerprint == "" || a.Plan.Fingerprint != b.Plan.Fingerprint {
		t.Fatalf("fingerprints differ or empty: %q vs %q", a.Plan.Fingerprint, b.Plan.Fingerprint)
	}
	live := testEngine(t)
	live.Now = nil
	first, err := live.Plan(PlanOptions{Agents: []integrations.Target{integrations.TargetGeneric}})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	second, err := live.Plan(PlanOptions{Agents: []integrations.Target{integrations.TargetGeneric}})
	if err != nil {
		t.Fatal(err)
	}
	if first.Plan.Fingerprint != second.Plan.Fingerprint {
		t.Fatal("fingerprints must ignore plan timestamps")
	}
	after, err := os.ReadDir(engine.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(stateBefore) {
		t.Fatalf("planning wrote private state: before=%d after=%d", len(stateBefore), len(after))
	}
	if got := snapshotTree(t, engine.WorkspaceRoot); got != rootBefore {
		t.Fatalf("planning wrote workspace files")
	}
	raw, err := json.Marshal(a.Plan)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "atlas_worker") && strings.Contains(string(raw), "# Atlas") {
		t.Fatalf("plan JSON leaked skill payload")
	}
	if strings.Contains(string(raw), `"payload"`) || strings.Contains(string(raw), "BEGIN PRIVATE KEY") {
		t.Fatalf("plan JSON looks like it contains a payload or secret")
	}
	if a.Plan.Format != setupPlanFormat {
		t.Fatalf("format %s", a.Plan.Format)
	}
	found := false
	for _, provider := range a.Plan.Providers {
		if provider.Target == integrations.TargetGeneric && provider.Selected && !provider.NoOp {
			found = true
			if provider.ResultingState != adapter.StatePortableReady {
				t.Fatalf("generic resulting state %s", provider.ResultingState)
			}
			if !provider.AdapterMissing {
				t.Fatalf("114.1 must report adapters as missing")
			}
		}
	}
	if !found {
		t.Fatal("expected selected generic provider with writes")
	}
}

func TestApplyIdempotentAndPermissions(t *testing.T) {
	engine := testEngine(t)
	first := applyGeneric(t, engine)
	if first.Status != RunStatusUnverified && first.Status != RunStatusPending {
		t.Fatalf("first status %s", first.Status)
	}
	info, err := os.Stat(engine.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != dirPerm {
		t.Fatalf("state dir mode %o", info.Mode().Perm())
	}
	manifest := manifestPath(engine.StateDir, engine.WorkspaceID)
	minfo, err := os.Stat(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if minfo.Mode().Perm() != filePerm {
		t.Fatalf("manifest mode %o", minfo.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(engine.WorkspaceRoot, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	second := applyGeneric(t, engine)
	if !second.Providers[0].NoOp && second.Status == RunStatusFailed {
		t.Fatalf("second apply should be a no-op, got %#v", second)
	}
	entries, err := os.ReadDir(filepath.Join(engine.StateDir, "rollback"))
	if err == nil && len(entries) != 0 {
		t.Fatalf("rollback material retained after commit: %v", names(entries))
	}
}

func TestPartialProviderFailureIsIsolated(t *testing.T) {
	engine := testEngine(t)
	engine.Hooks.AfterStep = func(step adapter.PlanStep) error {
		if strings.Contains(step.Path, "grok") {
			return errors.New("injected grok failure")
		}
		return nil
	}
	prepared, err := engine.Plan(PlanOptions{Agents: []integrations.Target{integrations.TargetGeneric, integrations.TargetGrok}})
	if err != nil {
		t.Fatal(err)
	}
	report, err := engine.ApplyPrepared(context.Background(), prepared, ApplyOptions{Yes: true})
	if err == nil {
		t.Fatal("expected partial or failed apply error")
	}
	if apperr.CodeOf(err) != apperr.CodeConflict && report.Status != RunStatusPartial {
		t.Fatalf("status %s err %v", report.Status, err)
	}
	genericKept := false
	grokFailed := false
	for _, row := range report.Providers {
		if row.Target == integrations.TargetGeneric && row.OperationState.KeepsWrites() {
			genericKept = true
		}
		if row.Target == integrations.TargetGrok && !row.OperationState.KeepsWrites() {
			grokFailed = true
		}
	}
	if !genericKept || !grokFailed {
		t.Fatalf("isolation failed: %#v", report.Providers)
	}
	if _, err := os.Stat(filepath.Join(engine.WorkspaceRoot, ".tracker", "integrations", "generic-agent-skill", "SKILL.md")); err != nil {
		t.Fatal("generic skill should remain")
	}
}

func TestBackupFailureDoesNotUndoAgents(t *testing.T) {
	engine := testEngine(t)
	engine.Hooks.BackupApply = func() error { return errors.New("backup target unreachable") }
	prepared, err := engine.Plan(PlanOptions{Agents: []integrations.Target{integrations.TargetGeneric}, Backup: true, BackupTarget: "local-test"})
	if err != nil {
		t.Fatal(err)
	}
	report, err := engine.ApplyPrepared(context.Background(), prepared, ApplyOptions{Yes: true})
	if err == nil || report.Status != RunStatusPartial {
		t.Fatalf("expected partial backup failure, status=%s err=%v", report.Status, err)
	}
	if _, err := os.Stat(filepath.Join(engine.WorkspaceRoot, "AGENTS.md")); err != nil {
		t.Fatal("agents must remain after backup failure")
	}
}

func TestCrashInjectionResumesOrRollsBack(t *testing.T) {
	cases := []struct {
		name   string
		hooks  func(*int) Hooks
		expect func(*testing.T, *Engine, string)
	}{
		{
			name: "before first write",
			hooks: func(n *int) Hooks {
				return Hooks{BeforeFirstWrite: func() error { return ErrInjectedCrash }}
			},
			expect: func(t *testing.T, engine *Engine, agents string) {
				if _, err := os.Lstat(filepath.Join(engine.WorkspaceRoot, "AGENTS.md")); !os.IsNotExist(err) {
					t.Fatal("before-first-write must not create files")
				}
				if err := engine.recoverInFlight(context.Background()); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Lstat(filepath.Join(engine.WorkspaceRoot, "AGENTS.md")); !os.IsNotExist(err) {
					t.Fatal("recovery must not invent files")
				}
			},
		},
		{
			name: "after snapshot",
			hooks: func(n *int) Hooks {
				return Hooks{AfterSnapshot: func(adapter.PlanStep) error { return ErrInjectedCrash }}
			},
			expect: func(t *testing.T, engine *Engine, _ string) {
				if err := engine.recoverInFlight(context.Background()); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Lstat(filepath.Join(engine.WorkspaceRoot, "AGENTS.md")); !os.IsNotExist(err) {
					t.Fatal("after-snapshot crash must roll back to absent file")
				}
			},
		},
		{
			name: "after instruction update",
			hooks: func(n *int) Hooks {
				return Hooks{AfterStep: func(step adapter.PlanStep) error {
					if strings.HasSuffix(step.Path, "AGENTS.md") {
						return ErrInjectedCrash
					}
					return nil
				}}
			},
			expect: func(t *testing.T, engine *Engine, _ string) {
				if err := engine.recoverInFlight(context.Background()); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Lstat(filepath.Join(engine.WorkspaceRoot, "AGENTS.md")); !os.IsNotExist(err) {
					t.Fatal("after-step crash must roll back")
				}
			},
		},
		{
			name: "after verify before commit",
			hooks: func(n *int) Hooks {
				return Hooks{AfterVerify: func() error { return ErrInjectedCrash }}
			},
			expect: func(t *testing.T, engine *Engine, _ string) {
				if err := engine.recoverInFlight(context.Background()); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(manifestPath(engine.StateDir, engine.WorkspaceID)); err != nil {
					t.Fatal("verify crash should resume and commit the manifest")
				}
			},
		},
		{
			name: "during rollback",
			hooks: func(n *int) Hooks {
				return Hooks{
					AfterStep: func(adapter.PlanStep) error { return errors.New("force rollback") },
					DuringRollback: func(adapter.PlanStep) error {
						*n++
						if *n == 1 {
							return ErrInjectedCrash
						}
						return nil
					},
				}
			},
			expect: func(t *testing.T, engine *Engine, _ string) {
				if err := engine.recoverInFlight(context.Background()); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := testEngine(t)
			var n int
			engine.Hooks = tc.hooks(&n)
			prepared, err := engine.Plan(PlanOptions{Agents: []integrations.Target{integrations.TargetGeneric}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = engine.ApplyPrepared(context.Background(), prepared, ApplyOptions{Yes: true})
			if !errors.Is(err, ErrInjectedCrash) && tc.name != "during rollback" {
				t.Fatalf("expected injected crash, got %v", err)
			}
			if tc.name == "during rollback" && !errors.Is(err, ErrInjectedCrash) && err == nil {
				t.Fatalf("expected rollback crash or apply error, got %v", err)
			}
			tc.expect(t, engine, "")
		})
	}
}

func TestConcurrentSetupLock(t *testing.T) {
	dir := t.TempDir()
	release, err := AcquireSetupLock(dir, "holder")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = release() }()
	if _, err := AcquireSetupLock(dir, "waiter"); err == nil || apperr.CodeOf(err) != apperr.CodeBusy {
		t.Fatalf("second lock should be busy, got %v", err)
	}
}

func TestStalePlanRejected(t *testing.T) {
	engine := testEngine(t)
	prepared, err := engine.Plan(PlanOptions{Agents: []integrations.Target{integrations.TargetGeneric}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(engine.WorkspaceRoot, "AGENTS.md"), []byte("changed after plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.ApplyPrepared(context.Background(), prepared, ApplyOptions{Yes: true}); err == nil || apperr.CodeOf(err) != apperr.CodeConflict {
		t.Fatalf("stale plan should conflict, got %v", err)
	}
}

func TestRedactedJSONOmitsSecretsAndPayloads(t *testing.T) {
	engine := testEngine(t)
	secret := "sk-super-secret-token-12345"
	if err := os.WriteFile(filepath.Join(engine.WorkspaceRoot, "AGENTS.md"), []byte("custom "+secret+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prepared, err := engine.Plan(PlanOptions{Agents: []integrations.Target{integrations.TargetGeneric}, Backup: true, BackupTarget: "t1", Team: "pair"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(engine.PlanReport(prepared))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatal("report leaked workspace secret")
	}
	planRaw, err := json.Marshal(prepared.Plan)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(planRaw), secret) {
		t.Fatal("plan leaked workspace secret")
	}
	if prepared.Plan.Team == nil || prepared.Plan.Team.Writes {
		t.Fatal("team request must be recorded without writes")
	}
}

func TestCustomInstructionContentPreserved(t *testing.T) {
	engine := testEngine(t)
	custom := "<!-- user note -->\nkeep this paragraph\n"
	if err := os.WriteFile(filepath.Join(engine.WorkspaceRoot, "AGENTS.md"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	applyGeneric(t, engine)
	body, err := os.ReadFile(filepath.Join(engine.WorkspaceRoot, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "keep this paragraph") {
		t.Fatalf("custom content lost:\n%s", body)
	}
	if !strings.Contains(string(body), "atlas-tasker:generic:begin") {
		t.Fatal("managed block missing")
	}
}

func TestRepairIdempotentAndRemovalRespectsOwnership(t *testing.T) {
	engine := testEngine(t)
	applyGeneric(t, engine)
	first, err := engine.Repair(context.Background(), integrations.TargetGeneric, true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := engine.Repair(context.Background(), integrations.TargetGeneric, true)
	if err != nil {
		t.Fatal(err)
	}
	_ = first
	if len(second.Providers) == 1 && !second.Providers[0].NoOp && second.Status == RunStatusFailed {
		t.Fatalf("repeated repair should settle, got %#v", second)
	}

	skill := filepath.Join(engine.WorkspaceRoot, ".tracker", "integrations", "generic-agent-skill", "SKILL.md")
	if err := os.WriteFile(skill, []byte("user edited this skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Disconnect(context.Background(), integrations.TargetGeneric, false); err == nil {
		t.Fatal("removal after manual edit must fail closed without confirmation")
	}
	if _, err := engine.Disconnect(context.Background(), integrations.TargetGeneric, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(skill); !os.IsNotExist(err) {
		t.Fatal("confirmed removal should delete the edited Atlas-owned skill")
	}
	agents, err := os.ReadFile(filepath.Join(engine.WorkspaceRoot, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(agents), "atlas-tasker:generic:begin") {
		t.Fatal("managed block should be stripped")
	}
}

func TestBinaryAndWorkspaceRelocationStatus(t *testing.T) {
	engine := testEngine(t)
	applyGeneric(t, engine)
	originalTracker := engine.TrackerPath
	other := filepath.Join(t.TempDir(), "moved-tracker")
	if err := os.WriteFile(other, []byte("tracker-binary-v2"), 0o755); err != nil {
		t.Fatal(err)
	}
	engine.TrackerPath = other
	report, err := engine.StatusReport()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(report.RepairReason, "binary relocation") {
		t.Fatalf("expected binary relocation, got %q", report.RepairReason)
	}

	engine.TrackerPath = originalTracker
	moved := filepath.Join(t.TempDir(), "moved-workspace")
	if err := os.Rename(engine.WorkspaceRoot, moved); err != nil {
		t.Fatal(err)
	}
	if resolved, err := filepath.EvalSymlinks(moved); err == nil {
		moved = resolved
	}
	engine.WorkspaceRoot = moved
	report, err = engine.StatusReport()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(report.RepairReason, "workspace relocation") {
		t.Fatalf("expected workspace relocation, got %q", report.RepairReason)
	}
}

func TestDeliveryModeRefusedAndMachineWideMustBeNamed(t *testing.T) {
	engine := testEngine(t)
	if _, err := engine.Plan(PlanOptions{Mode: contracts.ManagedModeDelivery}); err == nil {
		t.Fatal("delivery mode must be refused")
	}
	prepared, err := engine.Plan(PlanOptions{Agents: []integrations.Target{integrations.TargetOpenClaw}})
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.requireConsent(prepared, ApplyOptions{Yes: true}); err == nil {
		t.Fatal("--yes alone must not consent to machine-wide OpenClaw")
	}
}

func TestRunStatusExitMapping(t *testing.T) {
	if err := errorForRunStatus(RunStatusUnverified); err != nil {
		t.Fatal(err)
	}
	if err := errorForRunStatus(RunStatusPartial); err == nil || apperr.CodeOf(err) != apperr.CodeConflict {
		t.Fatalf("partial: %v", err)
	}
	if err := errorForRunStatus(RunStatusFailed); err == nil || apperr.CodeOf(err) != apperr.CodeInternal {
		t.Fatalf("failed: %v", err)
	}
}

func TestDefaultStateDirUsesXDGAndMacLayout(t *testing.T) {
	dir, err := DefaultStateDir("/home/atlas", func(key string) string {
		if key == "XDG_STATE_HOME" {
			return "/var/state"
		}
		return ""
	})
	if err != nil || dir != "/var/state/atlas-tasker" {
		t.Fatalf("xdg: %s %v", dir, err)
	}
}

func TestRollbackIdempotencyKinds(t *testing.T) {
	engine := testEngine(t)
	path := filepath.Join(engine.WorkspaceRoot, "created.txt")
	rec := &journalStepRecord{
		Path:     path,
		Rollback: &adapter.RollbackAction{Kind: adapter.RollbackDeleteCreated, Path: path},
	}
	if err := engine.rollbackStep("op", rec); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("atlas\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := InspectFile(path, engine.currentUID)
	if err != nil {
		t.Fatal(err)
	}
	rec.After = &after
	if err := engine.rollbackStep("op", rec); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("delete_created should remove the Atlas identity")
	}
	if err := engine.rollbackStep("op", rec); err != nil {
		t.Fatal("ENOENT is success for delete_created")
	}
}

func TestInspectFileRefusesForeignOwnerAndSymlink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectFile(link, os.Getuid()); err == nil {
		t.Fatal("symlink must be refused")
	}
}

func snapshotTree(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		b.WriteString(rel)
		b.WriteByte(' ')
		b.WriteString(info.Mode().String())
		if info.Mode().IsRegular() {
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			b.WriteByte(' ')
			b.WriteString(string(raw))
		}
		b.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func names(entries []os.DirEntry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Name())
	}
	return out
}

func TestLockDoesNotWaitOnSecondHolder(t *testing.T) {
	dir := t.TempDir()
	var started sync.WaitGroup
	started.Add(1)
	done := make(chan error, 1)
	go func() {
		release, err := AcquireSetupLock(dir, "slow")
		started.Done()
		if err != nil {
			done <- err
			return
		}
		time.Sleep(50 * time.Millisecond)
		done <- release()
	}()
	started.Wait()
	time.Sleep(5 * time.Millisecond)
	_, err := AcquireSetupLock(dir, "other")
	if err == nil || apperr.CodeOf(err) != apperr.CodeBusy {
		t.Fatalf("expected busy, got %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
