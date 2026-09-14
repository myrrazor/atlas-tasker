package uninstall

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

func atlasProbe(path string, version string) BinaryProbe {
	info := &debug.BuildInfo{
		Path: trackerMainPackage,
		Main: debug.Module{Path: "github.com/myrrazor/atlas-tasker", Version: version},
	}
	return BinaryProbe{
		Executable: func() (string, error) { return path, nil },
		BuildInfo:  func() (*debug.BuildInfo, bool) { return info, true },
		DiskBuildInfo: func(disk string) (*debug.BuildInfo, bool) {
			resolved, _ := filepath.EvalSymlinks(path)
			if disk != path && disk != resolved && filepath.Clean(disk) != filepath.Clean(path) {
				return nil, false
			}
			return info, true
		},
	}
}

func writeCopy(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestEnsureSourceReceiptWritesIsolatedCopy(t *testing.T) {
	dir := t.TempDir()
	bin := writeCopy(t, dir, "tracker", "atlas-source-copy-1")
	state := filepath.Join(dir, "state")
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	got, err := EnsureSourceReceipt(state, now, atlasProbe(bin, "(devel)"))
	if err != nil || !got.Wrote || got.Receipt.InstallMethod != MethodSource {
		t.Fatalf("write source receipt: %#v %v", got, err)
	}
	loaded, err := LoadReceipt(state)
	resolved, _ := filepath.EvalSymlinks(bin)
	if err != nil || loaded.InstallMethod != MethodSource || loaded.BinaryPath != resolved {
		t.Fatalf("load: %#v resolved=%s err=%v", loaded, resolved, err)
	}
}

func TestEnsureSourceReceiptGoInstallMethod(t *testing.T) {
	dir := t.TempDir()
	bin := writeCopy(t, dir, "tracker", "atlas-go-install-copy")
	state := filepath.Join(dir, "state")
	got, err := EnsureSourceReceipt(state, time.Now().UTC(), atlasProbe(bin, "v1.15.0"))
	if err != nil || !got.Wrote || got.Receipt.InstallMethod != MethodGoInstall {
		t.Fatalf("go-install receipt: %#v %v", got, err)
	}
}

func TestEnsureSourceReceiptSkipsTestAndForeignBinaries(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	testBin := writeCopy(t, dir, "tracker.test", "not-tracker")
	got, err := EnsureSourceReceipt(state, time.Now().UTC(), atlasProbe(testBin, "(devel)"))
	if err != nil || got.Wrote || got.Skipped == "" {
		t.Fatalf("test binary must skip: %#v %v", got, err)
	}
	other := writeCopy(t, dir, "other", "payload")
	foreign := BinaryProbe{
		Executable: func() (string, error) { return other, nil },
		BuildInfo: func() (*debug.BuildInfo, bool) {
			return &debug.BuildInfo{Path: "example.com/not-atlas/cmd/tool"}, true
		},
		DiskBuildInfo: func(string) (*debug.BuildInfo, bool) {
			return &debug.BuildInfo{Path: "example.com/not-atlas/cmd/tool"}, true
		},
	}
	got, err = EnsureSourceReceipt(state, time.Now().UTC(), foreign)
	if err != nil || got.Wrote {
		t.Fatalf("foreign main must skip: %#v %v", got, err)
	}
}

func TestEnsureSourceReceiptSkipsForeignOnDiskReplacement(t *testing.T) {
	dir := t.TempDir()
	bin := writeCopy(t, dir, "tracker", "process-was-atlas")
	state := filepath.Join(dir, "state")
	probe := atlasProbe(bin, "(devel)")
	probe.DiskBuildInfo = func(string) (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Path: "example.com/replaced"}, true
	}
	got, err := EnsureSourceReceipt(state, time.Now().UTC(), probe)
	if err != nil || got.Wrote || !strings.Contains(got.Skipped, "on-disk") {
		t.Fatalf("replaced on-disk binary must skip: %#v %v", got, err)
	}
	if _, err := os.Stat(ReceiptPath(state)); !os.IsNotExist(err) {
		t.Fatal("must not write a receipt for a foreign replacement")
	}
}

func TestEnsureSourceReceiptSkipsNonExecutableOnUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix executable bit")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "tracker")
	if err := os.WriteFile(bin, []byte("not-exec"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := EnsureSourceReceipt(filepath.Join(dir, "state"), time.Now().UTC(), atlasProbe(bin, "(devel)"))
	if err != nil || got.Wrote || !strings.Contains(got.Skipped, "not executable") {
		t.Fatalf("non-executable must skip: %#v %v", got, err)
	}
}

func TestLooksPackageManagedPaths(t *testing.T) {
	cases := map[string]bool{
		"/usr/local/bin/tracker":               false,
		"/usr/bin/tracker":                     true,
		"/bin/tracker":                         true,
		"/usr/sbin/tracker":                    true,
		"/nix/store/abc123/bin/tracker":        true,
		"/opt/local/bin/tracker":               true,
		"/opt/homebrew/Cellar/atlas/bin/tracker": true,
		"/home/dev/go/bin/tracker":             false,
	}
	for path, want := range cases {
		if got := looksPackageManaged(path); got != want {
			t.Fatalf("looksPackageManaged(%q)=%v want %v", path, got, want)
		}
	}
}

func TestEnsureSourceReceiptPreservesInstallerAndOtherPath(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	scriptBin := writeCopy(t, dir, "installed", "script-install")
	receipt, err := NewScriptReceipt(scriptBin, "v1.15.0-test", time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteReceipt(state, receipt); err != nil {
		t.Fatal(err)
	}
	copyBin := writeCopy(t, dir, "tracker", "source-copy")
	got, err := EnsureSourceReceipt(state, time.Now().UTC(), atlasProbe(copyBin, "(devel)"))
	if err != nil || got.Wrote || got.Receipt.BinaryPath != scriptBin {
		t.Fatalf("must keep other-path installer receipt: %#v %v", got, err)
	}

	same, err := EnsureSourceReceipt(state, time.Now().UTC(), atlasProbe(scriptBin, "(devel)"))
	if err != nil || same.Wrote || same.Receipt.InstallMethod != MethodScript {
		t.Fatalf("must keep installer-owned same path: %#v %v", same, err)
	}
}

func TestEnsureSourceReceiptRefreshSamePathSourceRebuild(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	bin := writeCopy(t, dir, "tracker", "build-one")
	first, err := EnsureSourceReceipt(state, time.Now().UTC(), atlasProbe(bin, "(devel)"))
	if err != nil || !first.Wrote {
		t.Fatalf("first: %#v %v", first, err)
	}
	if err := os.WriteFile(bin, []byte("build-two"), 0o755); err != nil {
		t.Fatal(err)
	}
	second, err := EnsureSourceReceipt(state, time.Now().UTC(), atlasProbe(bin, "(devel)"))
	if err != nil || !second.Wrote || second.Receipt.BinarySHA256 == first.Receipt.BinarySHA256 {
		t.Fatalf("rebuild refresh: %#v %v", second, err)
	}
}

func TestEnsureSourceReceiptCorruptExistingIsError(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ReceiptPath(state), []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := writeCopy(t, dir, "tracker", "payload")
	got, err := EnsureSourceReceipt(state, time.Now().UTC(), atlasProbe(bin, "(devel)"))
	if err == nil || got.Wrote {
		t.Fatalf("corrupt receipt must error without replacing: %#v %v", got, err)
	}
	raw, _ := os.ReadFile(ReceiptPath(state))
	if string(raw) != "{not-json" {
		t.Fatalf("corrupt receipt was overwritten: %q", raw)
	}
}

func TestEnsureSourceReceiptUnwritableStateIsError(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(ReceiptPath(state), 0o700); err != nil {
		t.Fatal(err)
	}
	bin := writeCopy(t, dir, "tracker", "payload")
	got, err := EnsureSourceReceipt(state, time.Now().UTC(), atlasProbe(bin, "(devel)"))
	if err == nil || got.Wrote {
		t.Fatalf("unwritable receipt path must error: %#v %v", got, err)
	}
}

func TestSourceReceiptUninstallRemovesCopyAndKeepsTickets(t *testing.T) {
	dir := t.TempDir()
	bin := writeCopy(t, dir, "tracker", "uninstall-me")
	state := filepath.Join(dir, "state")
	workspace := filepath.Join(dir, "board")
	if err := os.MkdirAll(filepath.Join(workspace, "projects", "APP"), 0o755); err != nil {
		t.Fatal(err)
	}
	ticket := filepath.Join(workspace, "projects", "APP", "APP-1.md")
	if err := os.WriteFile(ticket, []byte("# APP-1\nkeep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := EnsureSourceReceipt(state, time.Now().UTC(), atlasProbe(bin, "(devel)"))
	if err != nil || !got.Wrote {
		t.Fatalf("receipt: %#v %v", got, err)
	}
	opts := Options{Home: filepath.Join(dir, "home"), StateDir: state, Command: &FakeCommandRunner{}}
	result, err := Apply(context.Background(), opts, true)
	if err != nil {
		t.Fatalf("apply: %v %#v", err, result)
	}
	if _, err := os.Stat(bin); !os.IsNotExist(err) {
		t.Fatal("source copy should be removed")
	}
	raw, err := os.ReadFile(ticket)
	if err != nil || string(raw) != "# APP-1\nkeep me\n" {
		t.Fatalf("tickets must remain: %q %v", raw, err)
	}
}

func TestModifiedSourceBinaryRefusesUninstall(t *testing.T) {
	dir := t.TempDir()
	bin := writeCopy(t, dir, "tracker", "original")
	state := filepath.Join(dir, "state")
	if _, err := EnsureSourceReceipt(state, time.Now().UTC(), atlasProbe(bin, "(devel)")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("tampered"), 0o755); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanUninstall(context.Background(), Options{Home: filepath.Join(dir, "home"), StateDir: state, Command: &FakeCommandRunner{}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.CanApply || plan.Status != StatusRefused {
		t.Fatalf("modified binary must refuse: %#v", plan)
	}
}

func TestMaybeWriteRunningReceiptSkipsThisTestBinary(t *testing.T) {
	state := t.TempDir()
	got, err := MaybeWriteRunningReceipt(state, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if got.Wrote {
		t.Fatal("test process must not write a tracker receipt")
	}
	if _, err := os.Stat(ReceiptPath(state)); !os.IsNotExist(err) {
		t.Fatal("test process must not write a tracker receipt")
	}
}
