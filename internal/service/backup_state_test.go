package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

const v114LegacyReplica = "aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa"

func getenvNone(string) string { return "" }

func darwinAppSupport(home string) string {
	return filepath.Join(home, "Library", "Application Support", "Atlas Tasker")
}

func darwinLegacyState(home string) string {
	return filepath.Join(home, ".local", "state", "atlas-tasker")
}

func writeV114LegacyBackup(t *testing.T, stateDir, workspaceID, replicaID, targetID string, autoEnabled bool) {
	t.Helper()
	paths := backupStatePaths(stateDir, workspaceID)
	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	if err := os.MkdirAll(paths.Root, 0o700); err != nil {
		t.Fatal(err)
	}
	ident := ReplicaIdentity{
		Format:      replicaIdentityFormat,
		WorkspaceID: workspaceID,
		ReplicaID:   replicaID,
		CreatedAt:   now,
	}
	if err := atomicWriteJSON(paths.Replica, ident); err != nil {
		t.Fatal(err)
	}
	ledger := BackupLedger{
		Format:           checkpointLedgerFormat,
		WorkspaceID:      workspaceID,
		ReplicaID:        replicaID,
		LastCheckpointID: "cp-v114-legacy",
		CreatedAt:        now,
	}
	if err := atomicWriteJSON(paths.Ledger, ledger); err != nil {
		t.Fatal(err)
	}
	store := BackupTargetStore{
		Format: backupTargetsFormat,
		Targets: []contracts.BackupTarget{{
			Format:                contracts.BackupTargetFormat,
			TargetID:              targetID,
			Type:                  contracts.BackupTargetTypeGit,
			URL:                   "ssh://git.example.com/atlas/backups.git",
			Enabled:               true,
			VisibilityAttestation: contracts.BackupVisibilityPriv,
			DataBoundaryAck:       true,
			CreatedAt:             now,
		}},
		UpdatedAt: now,
	}
	if err := atomicWriteJSON(paths.Targets, store); err != nil {
		t.Fatal(err)
	}
	cfg := AutoBackupConfig{
		Format:          backupAutoFormat,
		Enabled:         autoEnabled,
		DefaultTargetID: targetID,
		EnabledAt:       now,
	}
	if !autoEnabled {
		cfg.DisabledAt = now
	}
	if err := atomicWriteJSON(paths.Auto, cfg); err != nil {
		t.Fatal(err)
	}
}

func TestResolveBackupStateDirFreshMacOSUsesAppSupport(t *testing.T) {
	home := t.TempDir()
	ws := "fresh-ws-id"
	res, err := ResolveBackupState(BackupStateDirOptions{
		Home: home, WorkspaceID: ws, Getenv: getenvNone, GOOS: "darwin",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := darwinAppSupport(home)
	if res.StateDir != want || res.Source != backupStateSourceCanonical {
		t.Fatalf("fresh default: %#v", res)
	}
	if res.LegacyDir != darwinLegacyState(home) {
		t.Fatalf("legacy candidate: %s", res.LegacyDir)
	}
	cliStyle, err := ResolveBackupStateDir(BackupStateDirOptions{
		Home: home, StateDir: "", WorkspaceID: ws, Getenv: getenvNone, GOOS: "darwin",
	})
	if err != nil {
		t.Fatal(err)
	}
	appStyle, err := ResolveBackupStateDir(BackupStateDirOptions{
		Home: home, StateDir: want, WorkspaceID: ws, Getenv: getenvNone, GOOS: "darwin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cliStyle != appStyle || cliStyle != want {
		t.Fatalf("fresh CLI %s app %s want %s", cliStyle, appStyle, want)
	}
}

func TestResolveBackupStateDirSelectsV114LegacyMacOSFixture(t *testing.T) {
	home := t.TempDir()
	ws := "ws-v114-upgrade"
	writeV114LegacyBackup(t, darwinLegacyState(home), ws, v114LegacyReplica, "offsite", true)

	for _, stateDir := range []string{"", darwinAppSupport(home)} {
		res, err := ResolveBackupState(BackupStateDirOptions{
			Home: home, StateDir: stateDir, WorkspaceID: ws, Getenv: getenvNone, GOOS: "darwin",
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.Source != backupStateSourceLegacy || res.StateDir != darwinLegacyState(home) {
			t.Fatalf("stateDir=%q got %#v", stateDir, res)
		}
		if res.ReplicaID != v114LegacyReplica {
			t.Fatalf("replica %s", res.ReplicaID)
		}
		if _, err := os.Lstat(filepath.Join(darwinAppSupport(home), "backups", ws, "replica.json")); !os.IsNotExist(err) {
			t.Fatalf("upgrade must not mint App Support lineage: %v", err)
		}
	}
}

func TestResolveBackupStateDirXDGIsAuthoritative(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	ws := "ws-xdg"
	writeV114LegacyBackup(t, darwinLegacyState(home), ws, v114LegacyReplica, "offsite", true)
	getenv := func(key string) string {
		if key == "XDG_STATE_HOME" {
			return xdg
		}
		return ""
	}
	res, err := ResolveBackupState(BackupStateDirOptions{
		Home: home, WorkspaceID: ws, Getenv: getenv, GOOS: "darwin",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(xdg, "atlas-tasker")
	if res.StateDir != want || res.Source != backupStateSourceXDG {
		t.Fatalf("xdg should win: %#v", res)
	}
	if res.LegacyDir != "" {
		t.Fatalf("XDG must not scan legacy: %s", res.LegacyDir)
	}
}

func TestResolveBackupStateDirExplicitStateDirIsAuthoritative(t *testing.T) {
	home := t.TempDir()
	custom := t.TempDir()
	ws := "ws-explicit"
	writeV114LegacyBackup(t, darwinLegacyState(home), ws, v114LegacyReplica, "offsite", true)
	res, err := ResolveBackupState(BackupStateDirOptions{
		Home: home, StateDir: custom, WorkspaceID: ws, Getenv: getenvNone, GOOS: "darwin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.StateDir != filepath.Clean(custom) || res.Source != backupStateSourceExplicit {
		t.Fatalf("explicit custom: %#v", res)
	}
}

func TestResolveBackupStateDirRelativeXDGRejected(t *testing.T) {
	_, err := CanonicalUserStateDir("/tmp/home", func(key string) string {
		if key == "XDG_STATE_HOME" {
			return "relative-state"
		}
		return ""
	})
	if err == nil {
		t.Fatal("relative XDG_STATE_HOME must fail")
	}
}

func TestResolveBackupStateDirRefusesDivergentDuplicate(t *testing.T) {
	home := t.TempDir()
	ws := "ws-dup"
	writeV114LegacyBackup(t, darwinLegacyState(home), ws, v114LegacyReplica, "legacy-target", true)
	writeV114LegacyBackup(t, darwinAppSupport(home), ws, "bbbbbbbb-2222-4222-8222-bbbbbbbbbbbb", "new-target", true)

	_, err := ResolveBackupStateDir(BackupStateDirOptions{
		Home: home, StateDir: darwinAppSupport(home), WorkspaceID: ws, Getenv: getenvNone, GOOS: "darwin",
	})
	if err == nil || !IsBackupStateConflict(err) {
		t.Fatalf("duplicate must refuse: %v", err)
	}
	if apperr.CodeOf(err) != apperr.CodeRepairNeeded {
		t.Fatalf("code %s", apperr.CodeOf(err))
	}
	var conflict *BackupStateConflictError
	if !errors.As(err, &conflict) {
		t.Fatal("wrapped conflict type")
	}
	if conflict.CanonicalReplica == conflict.LegacyReplica {
		t.Fatal("replicas should differ")
	}
}

func TestResolveBackupStateDirOptOutStaysOnLegacy(t *testing.T) {
	home := t.TempDir()
	ws := "ws-opt-out"
	paths := backupStatePaths(darwinLegacyState(home), ws)
	if err := os.MkdirAll(paths.Root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := atomicWriteJSON(paths.Auto, AutoBackupConfig{
		Format:     backupAutoFormat,
		Enabled:    false,
		DisabledAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	res, err := ResolveBackupState(BackupStateDirOptions{
		Home: home, WorkspaceID: ws, Getenv: getenvNone, GOOS: "darwin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != backupStateSourceLegacy {
		t.Fatalf("opt-out is lineage: %#v", res)
	}
	cfg, err := loadAutoConfig(backupStatePaths(res.StateDir, ws).Auto)
	if err != nil || cfg.Enabled {
		t.Fatalf("opt-out lost: %#v %v", cfg, err)
	}
}

func TestResolveBackupStateDirLinuxHasNoMacSplit(t *testing.T) {
	home := t.TempDir()
	ws := "ws-linux"
	writeV114LegacyBackup(t, darwinAppSupport(home), ws, v114LegacyReplica, "offsite", true)
	res, err := ResolveBackupState(BackupStateDirOptions{
		Home: home, WorkspaceID: ws, Getenv: getenvNone, GOOS: "linux",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".local", "state", "atlas-tasker")
	if res.StateDir != want || res.LegacyDir != "" || res.Source != backupStateSourceCanonical {
		t.Fatalf("linux: %#v", res)
	}
}

func TestBackupStatusInitTickShareLegacyReplica(t *testing.T) {
	ctx, actions := newCheckpointHarness(t)
	ws, err := LoadWorkspaceIdentity(actions.Root)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	legacy := darwinLegacyState(home)
	canonical := darwinAppSupport(home)
	writeV114LegacyBackup(t, legacy, ws, v114LegacyReplica, "offsite", true)

	attach := func(stateDir string) {
		actions.Home = home
		actions.StateDir = stateDir
		actions.GOOS = "darwin"
		actions.Getenv = getenvNone
	}

	attach("") // CLI: Home + empty StateDir
	cliReplica, err := actions.ReplicaStatus(ctx)
	if err != nil {
		t.Fatalf("cli replica: %v", err)
	}
	if cliReplica.Identity.ReplicaID != v114LegacyReplica {
		t.Fatalf("cli replica %s", cliReplica.Identity.ReplicaID)
	}

	attach(canonical) // App.OpenWorkspace attaches machine App Support
	appReplica, err := actions.ReplicaStatus(ctx)
	if err != nil {
		t.Fatalf("app replica: %v", err)
	}
	if appReplica.Identity.ReplicaID != v114LegacyReplica {
		t.Fatalf("app replica %s", appReplica.Identity.ReplicaID)
	}

	resolved, err := ResolveBackupStateDir(actions.backupStateOpts(ws))
	if err != nil {
		t.Fatal(err)
	}
	if resolved != legacy {
		t.Fatalf("init auto.json root %s want %s", resolved, legacy)
	}
	if err := atomicWriteJSON(filepath.Join(resolved, "backups", ws, "auto.json"), AutoBackupConfig{
		Format:    backupAutoFormat,
		Enabled:   true,
		EnabledAt: time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}

	attach("")
	tick, err := actions.BackupTick(ctx, true)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if !tick.Created || tick.CheckpointID == "" {
		t.Fatalf("tick %#v", tick)
	}

	attach(canonical)
	status, err := actions.AutoBackupStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.ReplicaID != v114LegacyReplica {
		t.Fatalf("status replica %s", status.ReplicaID)
	}
	if status.LastLocalCheckpointID != tick.CheckpointID {
		t.Fatalf("status checkpoint %s tick %s", status.LastLocalCheckpointID, tick.CheckpointID)
	}
	if status.TargetCount != 1 {
		t.Fatalf("legacy target missing: %#v", status)
	}
	if !status.AutomaticEnabled {
		t.Fatal("init auto.json did not follow the legacy lineage")
	}
	if _, err := os.Lstat(filepath.Join(canonical, "backups", ws, "replica.json")); !os.IsNotExist(err) {
		t.Fatal("tick must not create a second replica under App Support")
	}
	engine, err := actions.checkpointEngine()
	if err != nil {
		t.Fatal(err)
	}
	if engine.stateDir != legacy || engine.replicaID != v114LegacyReplica {
		t.Fatalf("engine %#v %s", engine.stateDir, engine.replicaID)
	}
}

func TestBackupTickRefusesDuplicateStateWithoutMinting(t *testing.T) {
	ctx, actions := newCheckpointHarness(t)
	ws, err := LoadWorkspaceIdentity(actions.Root)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	writeV114LegacyBackup(t, darwinLegacyState(home), ws, v114LegacyReplica, "legacy-target", true)
	writeV114LegacyBackup(t, darwinAppSupport(home), ws, "bbbbbbbb-2222-4222-8222-bbbbbbbbbbbb", "new-target", true)
	actions.Home = home
	actions.StateDir = ""
	actions.GOOS = "darwin"
	actions.Getenv = getenvNone

	_, err = actions.BackupTick(ctx, true)
	if err == nil || !IsBackupStateConflict(err) {
		t.Fatalf("tick must refuse duplicate state: %v", err)
	}
	_, err = actions.AutoBackupStatus(ctx)
	if err == nil || !IsBackupStateConflict(err) {
		t.Fatalf("status must refuse duplicate state: %v", err)
	}
}

func TestAttachUserStateEnvCopiesGetenv(t *testing.T) {
	var actions ActionService
	var queries QueryService
	getenv := func(string) string { return "/var/state" }
	AttachUserState(&actions, &queries, "/tmp/home", "")
	AttachUserStateEnv(&actions, &queries, getenv, "darwin")
	if actions.GOOS != "darwin" || queries.GOOS != "darwin" {
		t.Fatal("GOOS")
	}
	if actions.Getenv("XDG_STATE_HOME") != "/var/state" {
		t.Fatal("getenv")
	}
}

func TestCanonicalUserStateDirMacLayout(t *testing.T) {
	dir, err := canonicalUserStateDir("/Users/atlas", getenvNone, "darwin")
	if err != nil {
		t.Fatal(err)
	}
	if dir != "/Users/atlas/Library/Application Support/Atlas Tasker" {
		t.Fatalf("mac default %s", dir)
	}
	dir, err = canonicalUserStateDir("/home/atlas", getenvNone, "linux")
	if err != nil {
		t.Fatal(err)
	}
	if dir != "/home/atlas/.local/state/atlas-tasker" {
		t.Fatalf("linux default %s", dir)
	}
}

func TestLineageIgnoresEmptyBackupDir(t *testing.T) {
	home := t.TempDir()
	ws := "ws-empty-dir"
	if err := os.MkdirAll(backupStatePaths(darwinLegacyState(home), ws).Root, 0o700); err != nil {
		t.Fatal(err)
	}
	res, err := ResolveBackupState(BackupStateDirOptions{
		Home: home, WorkspaceID: ws, Getenv: getenvNone, GOOS: "darwin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != backupStateSourceCanonical {
		t.Fatalf("empty dir is not lineage: %#v", res)
	}
}
