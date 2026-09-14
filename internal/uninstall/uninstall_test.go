package uninstall

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/setup"
)

func TestStateDirMatchesSetupDefault(t *testing.T) {
	xdg := filepath.Join(t.TempDir(), "xdg")
	dir, err := StateDir("/tmp/home", func(key string) string {
		if key == "XDG_STATE_HOME" {
			return xdg
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	want, err := setup.DefaultStateDir("/tmp/home", func(key string) string {
		if key == "XDG_STATE_HOME" {
			return xdg
		}
		return ""
	})
	if err != nil || dir != want || !strings.HasSuffix(dir, "atlas-tasker") {
		t.Fatalf("state dir %s want %s err %v", dir, want, err)
	}

	mac, err := setup.DefaultStateDir("/Users/atlas", func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "darwin" && mac != "/Users/atlas/Library/Application Support/Atlas Tasker" {
		t.Fatalf("mac state dir %s", mac)
	}
	if runtime.GOOS != "darwin" && !strings.HasSuffix(mac, "/.local/state/atlas-tasker") && mac != "/Users/atlas/Library/Application Support/Atlas Tasker" {
		// setup.DefaultStateDir uses compile-time GOOS, so on Linux this is the XDG fallback.
		if !strings.Contains(mac, "atlas-tasker") && !strings.Contains(mac, "Atlas Tasker") {
			t.Fatalf("unexpected default %s", mac)
		}
	}
}

func TestPlanRefusesWithoutReceipt(t *testing.T) {
	opts := Options{Home: t.TempDir(), StateDir: t.TempDir(), Command: &FakeCommandRunner{}}
	plan, err := PlanUninstall(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if plan.CanApply || plan.Status != StatusRefused {
		t.Fatalf("expected refusal, got %#v", plan)
	}
}

func TestTamperedManifestIsRejected(t *testing.T) {
	ctx := context.Background()
	env := newUninstallFixture(t, "darwin")
	manifest, err := NewManifest(env.receipt, []ManifestAction{env.homeAction}, now)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Digest = "deadbeef"
	if err := os.MkdirAll(env.stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(ManifestPath(env.stateDir), append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanUninstall(ctx, env.opts)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.CanApply {
		t.Fatalf("tampered manifest must be ignored so discovery can still plan: %#v", plan)
	}
}

func TestArbitraryUnitAndPathAreRefused(t *testing.T) {
	env := newUninstallFixture(t, "darwin")
	_, err := NewManifest(env.receipt, []ManifestAction{{
		ID: "evil", Kind: KindStopService, UnitName: "com.apple.mail", Path: "/etc/passwd",
		BinaryPath: env.receipt.BinaryPath,
	}}, now)
	if err == nil {
		t.Fatal("allowlist must reject arbitrary units")
	}
}

func TestSoftwareUninstallPreservesBoardsAndUnrelatedConfig(t *testing.T) {
	ctx := context.Background()
	env := newUninstallFixture(t, "darwin")
	fake := env.opts.Command.(*FakeCommandRunner)

	before := env.hashes()
	plan, err := PlanUninstall(ctx, env.opts)
	if err != nil || !plan.CanApply {
		t.Fatalf("plan: %#v %v", plan, err)
	}
	if plan.Digest == "" {
		t.Fatal("plan digest required")
	}

	result, err := Apply(ctx, env.opts, true)
	if err != nil {
		t.Fatalf("apply: %v %#v", err, result)
	}
	if result.Status != StatusApplied || len(result.Applied) == 0 {
		t.Fatalf("expected applied software removals: %#v", result)
	}
	if len(fake.Calls) == 0 {
		t.Fatal("production-shaped stop must go through the command runner")
	}
	if len(fake.Calls[0]) != 4 || fake.Calls[0][0] != "launchctl" || fake.Calls[0][1] != "bootout" || !strings.HasPrefix(fake.Calls[0][2], "gui/") || fake.Calls[0][3] != env.plist {
		t.Fatalf("unexpected service argv %#v", fake.Calls)
	}

	if _, err := os.Stat(env.binary); !os.IsNotExist(err) {
		t.Fatal("receipt-bound executable should be removed")
	}
	if _, err := os.Stat(env.plist); !os.IsNotExist(err) {
		t.Fatal("Atlas unit file should be removed")
	}

	agents, _ := os.ReadFile(env.agents)
	if strings.Contains(string(agents), "atlas-tasker:begin") {
		t.Fatalf("managed block still present: %s", agents)
	}
	if !strings.Contains(string(agents), "house rules stay") || !strings.Contains(string(agents), "custom after") {
		t.Fatalf("unrelated markdown was eaten: %s", agents)
	}
	raw, _ := os.ReadFile(env.mcp)
	if strings.Contains(string(raw), "atlas-aaaaaaaaaaaa") {
		t.Fatalf("atlas mcp entry still present: %s", raw)
	}
	if !strings.Contains(string(raw), `"other"`) {
		t.Fatalf("unrelated mcpServers entry was removed: %s", raw)
	}

	after := env.hashes()
	for name, sum := range before {
		if after[name] != sum {
			t.Fatalf("preserved %s hash changed", name)
		}
	}

	again, err := Apply(ctx, env.opts, true)
	if err != nil {
		t.Fatalf("idempotent apply: %v", err)
	}
	if again.Status != StatusAlreadyUninstalled && again.Status != StatusApplied {
		t.Fatalf("repeat apply status=%s", again.Status)
	}
	if len(again.Applied) != 0 && again.Status != StatusAlreadyUninstalled {
		// second pass may skip everything
	}
	after2 := env.hashes()
	for name, sum := range before {
		if after2[name] != sum {
			t.Fatalf("second apply mutated %s", name)
		}
	}
}

func TestLinuxSystemdStopUsesFakeRunner(t *testing.T) {
	ctx := context.Background()
	env := newUninstallFixture(t, "linux")
	if _, err := Apply(ctx, env.opts, true); err != nil {
		t.Fatal(err)
	}
	fake := env.opts.Command.(*FakeCommandRunner)
	if len(fake.Calls) < 2 {
		t.Fatalf("linux stop/disable argv %#v", fake.Calls)
	}
	sawStop, sawDisable := false, false
	for _, call := range fake.Calls {
		if len(call) >= 4 && call[0] == "systemctl" && call[1] == "--user" && call[2] == "stop" {
			sawStop = true
		}
		if len(call) >= 4 && call[0] == "systemctl" && call[1] == "--user" && call[2] == "disable" {
			sawDisable = true
		}
	}
	if !sawStop || !sawDisable {
		t.Fatalf("linux must stop and disable: %#v", fake.Calls)
	}
	if _, err := os.Stat(env.unit); !os.IsNotExist(err) {
		t.Fatal("systemd unit should be removed")
	}
	if _, err := os.Stat(env.timer); !os.IsNotExist(err) {
		t.Fatal("paired systemd timer should be removed")
	}
}

func TestHomebrewProvenanceDoesNotDeleteBinary(t *testing.T) {
	ctx := context.Background()
	env := newUninstallFixture(t, "darwin")
	env.receipt.InstallMethod = MethodHomebrew
	env.receipt.Digest = ReceiptDigest(env.receipt.BinaryPath, env.receipt.BinarySHA256, env.receipt.InstallMethod, env.receipt.Version)
	if err := WriteReceipt(env.stateDir, env.receipt); err != nil {
		t.Fatal(err)
	}
	result, err := Apply(ctx, env.opts, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(env.binary); err != nil {
		t.Fatalf("package-managed binary must remain: %v", err)
	}
	if result.Status != StatusSoftwareStillInstalled {
		t.Fatalf("package-managed install must remain installed, got %s", result.Status)
	}
	joined := strings.Join(result.Notes, " ")
	if !strings.Contains(joined, "Homebrew owns") || !strings.Contains(joined, "software still installed") {
		t.Fatalf("missing Homebrew still-installed instruction: %#v", result)
	}
}

func TestProducerManifestIsConsumed(t *testing.T) {
	ctx := context.Background()
	env := newUninstallFixture(t, "darwin")
	action, err := NewHomeServiceAction(env.home, "darwin", env.receipt.BinaryPath, env.receipt.BinarySHA256)
	if err != nil {
		t.Fatal(err)
	}
	jsonAction := NewManagedJSONAction("cursor-atlas", env.mcp, "atlas-aaaaaaaaaaaa", env.receipt.BinaryPath, []string{"mcp", "serve"})
	manifest, err := NewManifest(env.receipt, []ManifestAction{action, jsonAction}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteManifest(env.stateDir, manifest); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadManifest(env.stateDir)
	if err != nil || loaded.Digest != manifest.Digest {
		t.Fatalf("round trip: %#v %v", loaded, err)
	}
	plan, err := PlanUninstall(ctx, env.opts)
	if err != nil || !plan.CanApply {
		t.Fatalf("consume: %#v %v", plan, err)
	}
}

func TestApplyRequiresYes(t *testing.T) {
	env := newUninstallFixture(t, "darwin")
	if _, err := Apply(context.Background(), env.opts, false); err == nil {
		t.Fatal("apply without --yes must fail")
	}
	if _, err := os.Stat(env.binary); err != nil {
		t.Fatal("preview/refuse must not delete the binary")
	}
}

func TestBackupLaunchdUnitIsDiscovered(t *testing.T) {
	env := newUninstallFixture(t, "darwin")
	plist := filepath.Join(launchAgentsDir(env.home), "com.atlas-tasker.backup.ws-1.plist")
	body := `<?xml version="1.0"?><plist><dict><key>Label</key><string>com.atlas-tasker.backup.ws-1</string><key>Comment</key><string>` + scheduleMarker + `</string><key>ProgramArguments</key><array><string>` + env.binary + `</string><string>backup</string><string>tick</string></array></dict></plist>`
	if err := os.WriteFile(plist, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanUninstall(context.Background(), env.opts)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, action := range plan.Actions {
		if action.Path == plist && action.Kind == KindStopService {
			found = true
		}
	}
	if !found {
		t.Fatalf("backup launchd unit not planned: %#v", plan.Actions)
	}
}

func TestStopFailureLeavesBinaryAndUnit(t *testing.T) {
	env := newUninstallFixture(t, "darwin")
	fake := &FakeCommandRunner{FailIf: func(name string, args []string) error {
		if name == "launchctl" {
			return os.ErrPermission
		}
		return nil
	}}
	env.opts.Command = fake
	result, err := Apply(context.Background(), env.opts, true)
	if err == nil {
		t.Fatal("stop failure must fail apply")
	}
	if _, statErr := os.Stat(env.binary); statErr != nil {
		t.Fatal("binary must remain after stop failure")
	}
	if _, statErr := os.Stat(env.plist); statErr != nil {
		t.Fatal("unit must remain after stop failure")
	}
	if result.Status != StatusRefused {
		t.Fatalf("status=%s", result.Status)
	}
}

func TestReceiptUpgradeRebindsManifest(t *testing.T) {
	ctx := context.Background()
	env := newUninstallFixture(t, "darwin")
	manifest, err := NewManifest(env.receipt, []ManifestAction{env.homeAction}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteManifest(env.stateDir, manifest); err != nil {
		t.Fatal(err)
	}
	before := env.hashes()
	if err := os.WriteFile(env.binary, []byte("#!/bin/sh\necho upgraded\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	upgraded, err := NewScriptReceipt(env.binary, "v1.15.1-test", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteReceipt(env.stateDir, upgraded); err != nil {
		t.Fatal(err)
	}
	env.receipt = upgraded
	onDisk, err := LoadManifest(env.stateDir)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanUninstall(ctx, env.opts)
	if err != nil || !plan.CanApply {
		t.Fatalf("upgrade must rebind, not block: %#v %v", plan, err)
	}
	preview, err := LoadManifest(env.stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Digest != onDisk.Digest || preview.ReceiptDigest == upgraded.Digest {
		t.Fatalf("preview must not persist a rebound manifest: %#v", preview)
	}
	if _, err := Apply(ctx, env.opts, true); err != nil {
		t.Fatal(err)
	}
	after := env.hashes()
	for name, sum := range before {
		if after[name] != sum {
			t.Fatalf("upgrade uninstall mutated %s", name)
		}
	}
	if _, err := os.Stat(env.registry); err != nil {
		t.Fatal("reinstall must still find registry")
	}
}

func TestPlanDiscoversGrokTomlAndClaudeJSON(t *testing.T) {
	env := newUninstallFixture(t, "darwin")
	grok := filepath.Join(env.home, ".grok", "config.toml")
	if err := os.MkdirAll(filepath.Dir(grok), 0o755); err != nil {
		t.Fatal(err)
	}
	tomlBody := "[mcp_servers.atlas-tasker]\ncommand = \"" + env.binary + "\"\nargs = [\"mcp\", \"serve\", \"--global\", \"--tool-profile\", \"workflow\"]\n"
	if err := os.WriteFile(grok, []byte(tomlBody), 0o600); err != nil {
		t.Fatal(err)
	}
	claude := filepath.Join(env.home, ".claude.json")
	doc := map[string]any{
		"mcpServers": map[string]any{
			"atlas-tasker": map[string]any{"command": env.binary, "args": []string{"mcp", "serve", "--global", "--tool-profile", "workflow"}},
		},
	}
	raw, _ := json.MarshalIndent(doc, "", "  ")
	if err := os.WriteFile(claude, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanUninstall(context.Background(), env.opts)
	if err != nil {
		t.Fatal(err)
	}
	var grokHit, claudeHit bool
	for _, action := range plan.Actions {
		if action.Path == grok && action.ConfigFormat == "toml" {
			grokHit = true
		}
		if action.Path == claude && action.ConfigFormat == "json" {
			claudeHit = true
		}
	}
	if !grokHit || !claudeHit {
		t.Fatalf("missing grok/claude discovery: %#v", plan.Actions)
	}
}

func TestGlobalAtlasTaskerKeyIsRemoved(t *testing.T) {
	env := newUninstallFixture(t, "darwin")
	global := filepath.Join(env.home, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(global), 0o755); err != nil {
		t.Fatal(err)
	}
	doc := map[string]any{
		"mcpServers": map[string]any{
			"atlas-tasker": map[string]any{"command": env.binary, "args": []string{"mcp", "serve", "--global"}},
			"other":        map[string]any{"command": "/usr/bin/other", "args": []string{}},
		},
	}
	raw, _ := json.MarshalIndent(doc, "", "  ")
	if err := os.WriteFile(global, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(context.Background(), env.opts, true); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(global)
	if strings.Contains(string(got), "atlas-tasker") {
		t.Fatalf("global atlas-tasker entry remains: %s", got)
	}
	if !strings.Contains(string(got), `"other"`) {
		t.Fatalf("unrelated global entry removed: %s", got)
	}
}

func TestSymlinkedClientConfigIsRefused(t *testing.T) {
	env := newUninstallFixture(t, "darwin")
	real := filepath.Join(env.home, "outside.json")
	if err := os.WriteFile(real, []byte(`{"mcpServers":{"atlas-tasker":{"command":"`+env.binary+`","args":["mcp","serve","--global"]}}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(env.home, ".grok", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanUninstall(context.Background(), env.opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range plan.Actions {
		if action.Path == link {
			t.Fatalf("symlink client path must not be planned: %#v", action)
		}
	}
}

var now = time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

type uninstallFixture struct {
	home, stateDir, binary, plist, unit, timer, mcp, agents, ticket, ledger, registry string
	receipt                                                                           Receipt
	homeAction                                                                        ManifestAction
	opts                                                                              Options
}

func newUninstallFixture(t *testing.T, goos string) *uninstallFixture {
	t.Helper()
	home := t.TempDir()
	stateDir := filepath.Join(home, "state")
	binDir := filepath.Join(home, "bin")
	ws := filepath.Join(home, "board")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(binDir, "tracker")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\necho tracker\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	ticket := filepath.Join(ws, "projects", "APP", "tickets", "APP-1.md")
	if err := os.MkdirAll(filepath.Dir(ticket), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ticket, []byte("# APP-1\nkeep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ledger := filepath.Join(stateDir, "backups", "ws-1", "ledger.json")
	if err := os.MkdirAll(filepath.Dir(ledger), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ledger, []byte(`{"format":"atlas_backup_ledger_v1","workspace_id":"ws-1"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	registry := filepath.Join(stateDir, "registry.json")
	reg := map[string]any{
		"format": "atlas_workspace_registry_v1",
		"workspaces": map[string]any{
			"ws-1": map[string]any{"workspace_id": "ws-1", "canonical_path": ws},
		},
	}
	raw, _ := json.MarshalIndent(reg, "", "  ")
	if err := os.WriteFile(registry, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	mcp := filepath.Join(ws, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(mcp), 0o755); err != nil {
		t.Fatal(err)
	}
	mcpDoc := map[string]any{
		"mcpServers": map[string]any{
			"atlas-aaaaaaaaaaaa": map[string]any{"type": "stdio", "command": binary, "args": []string{"mcp", "serve"}},
			"other":              map[string]any{"type": "stdio", "command": "/usr/bin/other", "args": []string{}},
		},
	}
	mcpRaw, _ := json.MarshalIndent(mcpDoc, "", "  ")
	if err := os.WriteFile(mcp, append(mcpRaw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	agents := filepath.Join(ws, "AGENTS.md")
	if err := os.WriteFile(agents, []byte("house rules stay\n\n\n<!-- atlas-tasker:begin -->\natlas block\n<!-- atlas-tasker:end -->\ncustom after\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fx := &uninstallFixture{
		home: home, stateDir: stateDir, binary: binary, mcp: mcp, agents: agents,
		ticket: ticket, ledger: ledger, registry: registry,
	}
	receipt, err := NewScriptReceipt(binary, "v1.15.0-test", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteReceipt(stateDir, receipt); err != nil {
		t.Fatal(err)
	}
	fx.receipt = receipt

	if goos == "darwin" {
		plistDir := launchAgentsDir(home)
		if err := os.MkdirAll(plistDir, 0o755); err != nil {
			t.Fatal(err)
		}
		fx.plist = filepath.Join(plistDir, homeLaunchdLabel+".plist")
		body := `<?xml version="1.0"?><plist><dict><key>Label</key><string>` + homeLaunchdLabel + `</key><key>Comment</key><string>` + homeServiceMarker + `</string><key>ProgramArguments</key><array><string>` + binary + `</string><string>serve</string></array></dict></plist>`
		if err := os.WriteFile(fx.plist, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		action, err := NewHomeServiceAction(home, "darwin", binary, receipt.BinarySHA256)
		if err != nil {
			t.Fatal(err)
		}
		fx.homeAction = action
	} else {
		unitDir := systemdUserDir(home)
		if err := os.MkdirAll(unitDir, 0o755); err != nil {
			t.Fatal(err)
		}
		fx.unit = filepath.Join(unitDir, homeSystemdService)
		body := "[Unit]\nDescription=Atlas Home\n# " + homeServiceMarker + "\n[Service]\nExecStart=" + binary + " serve\n"
		if err := os.WriteFile(fx.unit, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		timer := filepath.Join(unitDir, "atlas-backup-ws-1.timer")
		service := filepath.Join(unitDir, "atlas-backup-ws-1.service")
		if err := os.WriteFile(service, []byte("[Unit]\n# "+scheduleMarker+"\n[Service]\nType=oneshot\nExecStart="+binary+" backup tick\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(timer, []byte("[Unit]\n# "+scheduleMarker+"\n[Timer]\nOnUnitActiveSec=30s\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		fx.timer = timer
	}

	fx.opts = Options{
		Home: home, StateDir: stateDir, GOOS: goos, Now: func() time.Time { return now },
		Command: &FakeCommandRunner{},
	}
	return fx
}

// set by darwin fixture for producer tests
func (f *uninstallFixture) hashes() map[string]string {
	out := map[string]string{}
	for name, path := range map[string]string{
		"ticket": f.ticket, "ledger": f.ledger, "registry": f.registry,
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		sum := sha256.Sum256(raw)
		out[name] = hex.EncodeToString(sum[:])
		_ = name
	}
	other, _ := os.ReadFile(f.mcp)
	sum := sha256.Sum256([]byte(otherKey(string(other))))
	out["other-mcp"] = hex.EncodeToString(sum[:])
	return out
}

func otherKey(raw string) string {
	if strings.Contains(raw, `"other"`) {
		return "present"
	}
	return "missing"
}
