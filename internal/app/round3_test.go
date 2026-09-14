package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestInitRepeatUnchangedBackupIsSuccess(t *testing.T) {
	a := testApp(t)
	root := filepath.Join(a.Home(), "repeat")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	first, err := a.Init(context.Background(), InitOptions{Root: root, Register: true, Backup: true})
	if err != nil {
		t.Fatalf("first init: %v", err)
	}
	if !first.Backup.ReplicaReady || first.Backup.CheckpointID == "" {
		t.Fatalf("first backup: %+v", first.Backup)
	}
	second, err := a.Init(context.Background(), InitOptions{Root: root, Register: true, Backup: true})
	if err != nil {
		t.Fatalf("repeat init: %v", err)
	}
	if !second.Already {
		t.Fatal("expected already bootstrapped")
	}
	if !second.Backup.ReplicaReady {
		t.Fatalf("unchanged backup reported not ready: %+v steps=%+v", second.Backup, second.Steps)
	}
	if second.Backup.CheckpointID == "" {
		t.Fatal("repeat init dropped checkpoint id")
	}
	for _, step := range second.Steps {
		if step.Name == "backup" && step.Status == InitStepFailed {
			t.Fatalf("unchanged backup marked failed: %+v", step)
		}
	}
	if strings.Contains(second.Summary, "backup failed") {
		t.Fatalf("summary hid success: %s", second.Summary)
	}
	raw, err := os.ReadFile(filepath.Join(a.StateDir(), "init-journal", second.WorkspaceID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"name": "service"`) {
		t.Fatalf("journal missing service step: %s", raw)
	}
}

func TestInitPartialFailureReturnsErrorAndKeepsWork(t *testing.T) {
	a := testApp(t)
	a.opts.Process = NoopSpawner{}
	a.opts.Probe = LatchProber{}
	root := filepath.Join(a.Home(), "partial")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := a.Init(context.Background(), InitOptions{Root: root, Register: true, Backup: true})
	if err == nil || !IsPartial(err) {
		t.Fatalf("expected partial error, got %v", err)
	}
	if result.WorkspaceID == "" || !result.Registered {
		t.Fatalf("successful work was dropped: %+v", result)
	}
	if !strings.Contains(result.Summary, "service failed") && !strings.Contains(err.Error(), "service failed") {
		t.Fatalf("summary hid failure: %s / %v", result.Summary, err)
	}
}

func TestConsumeClaimIsAtomic(t *testing.T) {
	a := testApp(t)
	token, err := a.IssueClaim()
	if err != nil {
		t.Fatal(err)
	}
	var ok atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if a.ConsumeClaim(token) == nil {
				ok.Add(1)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 1 {
		t.Fatalf("expected one consumer, got %d", ok.Load())
	}
	if err := a.ConsumeClaim(token); err == nil {
		t.Fatal("replay must fail")
	}
}

func TestPathGrantPersistsAcrossOpenAndRefusesWrongPurpose(t *testing.T) {
	a := testApp(t)
	dir := filepath.Join(a.Home(), "granted")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	grant, err := a.GrantPath(context.Background(), dir, PathGrantInit)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Open(Options{
		Home:            a.Home(),
		StateDir:        a.StateDir(),
		LookPath:        func(string) (string, error) { return "", os.ErrNotExist },
		CommandRunner:   SilentRunner{},
		SkipHostInstall: true,
		Process:         NoopSpawner{},
		WriteClientCfg:  true,
		Now:             a.opts.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if _, err := b.ConsumeGrantFor(context.Background(), grant.ID, PathGrantRegister); err == nil {
		t.Fatal("wrong purpose must fail")
	}
	got, err := b.ConsumeGrantFor(context.Background(), grant.ID, PathGrantInit)
	if err != nil || got.Path != dir {
		t.Fatalf("cross-process consume: %+v %v", got, err)
	}
	if _, err := b.ConsumeGrantFor(context.Background(), grant.ID, PathGrantInit); err == nil {
		t.Fatal("replay must fail")
	}
}

func TestPathGrantRefusesSymlinkSwap(t *testing.T) {
	a := testApp(t)
	dir := filepath.Join(a.Home(), "realdir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	grant, err := a.GrantPath(context.Background(), dir, PathGrantInit)
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(a.Home(), "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ConsumeGrantFor(context.Background(), grant.ID, PathGrantInit); err == nil {
		t.Fatal("symlink swap must fail")
	}
}

func TestPathGrantExpiry(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	a := testApp(t)
	a.opts.Now = func() time.Time { return now }
	dir := filepath.Join(a.Home(), "expiring")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	grant, err := a.GrantPath(context.Background(), dir, PathGrantRepair)
	if err != nil {
		t.Fatal(err)
	}
	a.opts.Now = func() time.Time { return now.Add(11 * time.Minute) }
	if _, err := a.ConsumeGrantFor(context.Background(), grant.ID, PathGrantRepair); err == nil {
		t.Fatal("expired grant must fail")
	}
}

func TestOpenRejectsRelativeStateDir(t *testing.T) {
	_, err := Open(Options{
		Home:     t.TempDir(),
		StateDir: "relative-state",
	})
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("relative state dir: %v", err)
	}
}

func TestConcurrentOpenAgreesOnInstance(t *testing.T) {
	home := t.TempDir()
	state := filepath.Join(home, "state")
	var wg sync.WaitGroup
	ids := make([]string, 2)
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			opened, err := Open(Options{
				Home:            home,
				StateDir:        state,
				LookPath:        func(string) (string, error) { return "", os.ErrNotExist },
				CommandRunner:   SilentRunner{},
				SkipHostInstall: true,
				Process:         NoopSpawner{},
			})
			if err != nil {
				errs[i] = err
				return
			}
			ids[i] = opened.Settings().InstanceID
			_ = opened.Close()
		}(i)
	}
	wg.Wait()
	if errs[0] != nil || errs[1] != nil {
		t.Fatalf("open: %v %v", errs[0], errs[1])
	}
	if ids[0] == "" || ids[0] != ids[1] {
		t.Fatalf("instance mismatch %q %q", ids[0], ids[1])
	}
}

func TestConcurrentEnsureServiceReusesOne(t *testing.T) {
	a := testApp(t)
	var wg sync.WaitGroup
	var running int32
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, err := a.EnsureService(context.Background(), ServiceOptions{})
			if err != nil {
				t.Errorf("ensure: %v", err)
				return
			}
			if status.Running {
				atomic.AddInt32(&running, 1)
			}
		}()
	}
	wg.Wait()
	if running < 1 {
		t.Fatal("no running service")
	}
}

func TestSettingsSnapshotIsolatesRoots(t *testing.T) {
	a := testApp(t)
	roots := []string{filepath.Join(a.Home(), "code")}
	if err := os.MkdirAll(roots[0], 0o755); err != nil {
		t.Fatal(err)
	}
	enabled := true
	if _, err := a.UpdateSettings(context.Background(), MachineSettingsPatch{
		Discovery: &DiscoverySettings{Enabled: enabled, Roots: roots, MaxDepth: 2},
	}); err != nil {
		t.Fatal(err)
	}
	got := a.Settings()
	got.Discovery.Roots[0] = "/mutated"
	again := a.Settings()
	if again.Discovery.Roots[0] == "/mutated" {
		t.Fatal("settings slice leaked")
	}
}
