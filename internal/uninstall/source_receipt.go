package uninstall

import (
	gobuildinfo "debug/buildinfo"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/buildinfo"
)

const trackerMainPackage = "github.com/myrrazor/atlas-tasker/cmd/tracker"

// BinaryProbe lets tests feed an isolated binary copy and a disk identity
// probe. Production uses the running tracker via MaybeWriteRunningReceipt.
type BinaryProbe struct {
	Executable    func() (string, error)
	BuildInfo     func() (*debug.BuildInfo, bool)
	DiskBuildInfo func(path string) (*debug.BuildInfo, bool)
}

type EnsureReceiptResult struct {
	Receipt Receipt
	Wrote   bool
	Skipped string
}

func liveBinaryProbe() BinaryProbe {
	return BinaryProbe{
		Executable:    os.Executable,
		BuildInfo:     debug.ReadBuildInfo,
		DiskBuildInfo: readDiskBuildInfo,
	}
}

func readDiskBuildInfo(path string) (*debug.BuildInfo, bool) {
	info, err := gobuildinfo.ReadFile(path)
	if err != nil || info == nil {
		return nil, false
	}
	return info, true
}

// MaybeWriteRunningReceipt records a source/go-install receipt for this
// process when none exists. Skip (nil error) is for uncertain binaries.
// A returned error is a real write/load failure callers must surface.
func MaybeWriteRunningReceipt(stateDir string, now time.Time) (EnsureReceiptResult, error) {
	return EnsureSourceReceipt(stateDir, now, liveBinaryProbe())
}

func EnsureSourceReceipt(stateDir string, now time.Time, probe BinaryProbe) (EnsureReceiptResult, error) {
	if strings.TrimSpace(stateDir) == "" || !filepath.IsAbs(stateDir) {
		return EnsureReceiptResult{Skipped: "state dir is not an absolute path"}, nil
	}
	if probe.Executable == nil || probe.BuildInfo == nil {
		return EnsureReceiptResult{Skipped: "binary probe is incomplete"}, nil
	}
	exe, err := probe.Executable()
	if err != nil || strings.TrimSpace(exe) == "" {
		return EnsureReceiptResult{Skipped: "could not resolve the running executable"}, nil
	}
	path, err := resolveReceiptBinary(exe)
	if err != nil {
		return EnsureReceiptResult{Skipped: err.Error()}, nil
	}
	if looksPackageManaged(path) {
		return EnsureReceiptResult{Skipped: "refusing to claim a package-manager-managed binary"}, nil
	}
	if looksEphemeralTracker(path) {
		return EnsureReceiptResult{Skipped: "refusing to claim a test or ephemeral binary"}, nil
	}
	info, ok := probe.BuildInfo()
	if !ok || info == nil || !isTrackerMain(info) {
		return EnsureReceiptResult{Skipped: "running binary is not the Atlas tracker main package"}, nil
	}
	disk := probe.DiskBuildInfo
	if disk == nil {
		disk = readDiskBuildInfo
	}
	diskInfo, ok := disk(path)
	if !ok || diskInfo == nil || !isTrackerMain(diskInfo) {
		return EnsureReceiptResult{Skipped: "on-disk binary is not the Atlas tracker main package"}, nil
	}
	method := MethodSource
	if isGoInstallVersion(info.Main.Version) {
		method = MethodGoInstall
	}
	version := strings.TrimSpace(buildinfo.Version)
	if version == "" || version == "dev" {
		if v := strings.TrimSpace(info.Main.Version); v != "" && v != "(devel)" {
			version = v
		} else {
			version = "dev"
		}
	}
	sum, err := fileSHA256(path)
	if err != nil {
		return EnsureReceiptResult{Skipped: "could not hash the running executable"}, nil
	}
	existing, err := LoadReceipt(stateDir)
	if err == nil {
		if existing.BinaryPath != path {
			return EnsureReceiptResult{Receipt: existing, Skipped: "existing receipt belongs to another install path"}, nil
		}
		if existing.InstallMethod != MethodSource && existing.InstallMethod != MethodGoInstall {
			return EnsureReceiptResult{Receipt: existing, Skipped: "existing receipt is installer or package-manager owned"}, nil
		}
		if existing.BinarySHA256 == sum && existing.InstallMethod == method {
			return EnsureReceiptResult{Receipt: existing}, nil
		}
	} else if apperr.CodeOf(err) != apperr.CodeNotFound {
		return EnsureReceiptResult{}, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	receipt := Receipt{
		Format:        ReceiptFormat,
		CreatedAt:     now.UTC(),
		Version:       version,
		InstallMethod: method,
		BinaryPath:    path,
		BinarySHA256:  sum,
		OwnedPaths:    []OwnedPath{{Path: path, Kind: "executable", SHA256: sum}},
	}
	receipt.Digest = ReceiptDigest(receipt.BinaryPath, receipt.BinarySHA256, receipt.InstallMethod, receipt.Version)
	if err := WriteReceipt(stateDir, receipt); err != nil {
		return EnsureReceiptResult{}, err
	}
	return EnsureReceiptResult{Receipt: receipt, Wrote: true}, nil
}

func resolveReceiptBinary(exe string) (string, error) {
	exe = filepath.Clean(strings.TrimSpace(exe))
	if !filepath.IsAbs(exe) {
		return "", apperr.New(apperr.CodeInvalidInput, "executable path is not absolute")
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", apperr.New(apperr.CodeConflict, "could not resolve executable path")
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", apperr.New(apperr.CodeConflict, "refusing to claim a symlinked executable")
	}
	if !info.Mode().IsRegular() {
		return "", apperr.New(apperr.CodeConflict, "running path is not a regular executable file")
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		return "", apperr.New(apperr.CodeConflict, "running path is not executable")
	}
	return resolved, nil
}

func isTrackerMain(info *debug.BuildInfo) bool {
	return info != nil && info.Path == trackerMainPackage
}

func isGoInstallVersion(version string) bool {
	version = strings.TrimSpace(version)
	if version == "" || version == "(devel)" {
		return false
	}
	return strings.HasPrefix(version, "v") || strings.Contains(version, "+")
}

func looksEphemeralTracker(path string) bool {
	base := filepath.Base(path)
	slash := filepath.ToSlash(path)
	if strings.HasSuffix(base, ".test") || strings.HasSuffix(base, ".test.exe") {
		return true
	}
	if strings.Contains(slash, "/go-build") || strings.Contains(slash, "/go-test") {
		return true
	}
	return false
}

func looksPackageManaged(path string) bool {
	if looksHomebrew(path) {
		return true
	}
	slash := filepath.ToSlash(filepath.Clean(path))
	if strings.Contains(slash, "/nix/store/") || strings.HasPrefix(slash, "/nix/store/") {
		return true
	}
	if strings.Contains(slash, "/opt/local/") || strings.HasPrefix(slash, "/opt/local/") {
		return true
	}
	if strings.Contains(slash, "/apt/") || strings.Contains(slash, "/pacman/") {
		return true
	}
	dir := filepath.ToSlash(filepath.Dir(slash))
	switch dir {
	case "/bin", "/usr/bin", "/sbin", "/usr/sbin":
		return true
	}
	return false
}
