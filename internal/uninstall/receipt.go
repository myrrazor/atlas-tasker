package uninstall

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/setup"
)

func getenvFn(opts Options) func(string) string {
	if opts.Getenv != nil {
		return opts.Getenv
	}
	return os.Getenv
}

// StateDir is setup.DefaultStateDir: XDG_STATE_HOME/atlas-tasker, else
// macOS ~/Library/Application Support/Atlas Tasker, else
// Linux ~/.local/state/atlas-tasker.
func StateDir(home string, getenv func(string) string) (string, error) {
	return setup.DefaultStateDir(home, getenv)
}

func resolveStateDir(opts Options) (string, error) {
	if strings.TrimSpace(opts.StateDir) != "" {
		if !filepath.IsAbs(opts.StateDir) {
			return "", apperr.New(apperr.CodeInvalidInput, "state dir must be an absolute path")
		}
		return filepath.Clean(opts.StateDir), nil
	}
	home := strings.TrimSpace(opts.Home)
	if home == "" {
		home = strings.TrimSpace(getenvFn(opts)("HOME"))
	}
	return StateDir(home, getenvFn(opts))
}

func ReceiptPath(stateDir string) string {
	return filepath.Join(stateDir, ReceiptFile)
}

func ManifestPath(stateDir string) string {
	return filepath.Join(stateDir, ManifestFile)
}

func receiptPayload(binaryPath, binarySHA, method, version string) string {
	return binaryPath + "\n" + binarySHA + "\n" + method + "\n" + version + "\n"
}

func ReceiptDigest(binaryPath, binarySHA, method, version string) string {
	sum := sha256.Sum256([]byte(receiptPayload(binaryPath, binarySHA, method, version)))
	return hex.EncodeToString(sum[:])
}

func fileSHA256(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func NewScriptReceipt(binaryPath, version string, now time.Time) (Receipt, error) {
	binaryPath = filepath.Clean(strings.TrimSpace(binaryPath))
	if !filepath.IsAbs(binaryPath) {
		return Receipt{}, apperr.New(apperr.CodeInvalidInput, "receipt binary path must be absolute")
	}
	sum, err := fileSHA256(binaryPath)
	if err != nil {
		return Receipt{}, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	method := MethodScript
	if looksHomebrew(binaryPath) {
		method = MethodHomebrew
	}
	r := Receipt{
		Format:        ReceiptFormat,
		CreatedAt:     now.UTC(),
		Version:       strings.TrimSpace(version),
		InstallMethod: method,
		BinaryPath:    binaryPath,
		BinarySHA256:  sum,
		OwnedPaths:    []OwnedPath{{Path: binaryPath, Kind: "executable", SHA256: sum}},
	}
	r.Digest = ReceiptDigest(r.BinaryPath, r.BinarySHA256, r.InstallMethod, r.Version)
	return r, r.Validate()
}

func (r Receipt) Validate() error {
	if r.Format != ReceiptFormat {
		return apperr.New(apperr.CodeConflict, "unsupported install receipt format")
	}
	if !filepath.IsAbs(r.BinaryPath) || strings.Contains(r.BinaryPath, "\x00") {
		return apperr.New(apperr.CodeConflict, "install receipt binary path is not a clean absolute path")
	}
	if len(r.BinarySHA256) != 64 {
		return apperr.New(apperr.CodeConflict, "install receipt is missing a binary hash")
	}
	want := ReceiptDigest(r.BinaryPath, r.BinarySHA256, r.InstallMethod, r.Version)
	if r.Digest != want {
		return apperr.New(apperr.CodeConflict, "install receipt digest does not match")
	}
	for _, owned := range r.OwnedPaths {
		if !filepath.IsAbs(owned.Path) || owned.Path != filepath.Clean(owned.Path) {
			return apperr.New(apperr.CodeConflict, "install receipt owns an unsafe path")
		}
		if owned.Kind != "executable" {
			return apperr.New(apperr.CodeConflict, "install receipt may only own the tracker executable")
		}
		if owned.Path != r.BinaryPath {
			return apperr.New(apperr.CodeConflict, "install receipt owned path is not the recorded binary")
		}
	}
	return nil
}

func WriteReceipt(stateDir string, receipt Receipt) error {
	if err := receipt.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	path := ReceiptPath(stateDir)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func LoadReceipt(stateDir string) (Receipt, error) {
	raw, err := os.ReadFile(ReceiptPath(stateDir))
	if err != nil {
		if os.IsNotExist(err) {
			return Receipt{}, apperr.New(apperr.CodeNotFound, "no install receipt; uninstall will preview only")
		}
		return Receipt{}, err
	}
	var receipt Receipt
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return Receipt{}, apperr.New(apperr.CodeConflict, "install receipt is not valid JSON")
	}
	if err := receipt.Validate(); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

func looksHomebrew(path string) bool {
	slash := filepath.ToSlash(path)
	return strings.Contains(slash, "/Cellar/") || strings.Contains(slash, "/homebrew/")
}

func (r Receipt) packageManager() string {
	if r.InstallMethod == MethodHomebrew || looksHomebrew(r.BinaryPath) {
		return MethodHomebrew
	}
	return MethodScript
}

func (r Receipt) managerInstruction() string {
	if r.packageManager() != MethodHomebrew {
		return ""
	}
	return "Homebrew owns this executable (Cellar or Homebrew prefix). Uninstall it with Homebrew using the formula that owns this path. tracker uninstall will not delete the binary."
}

// ValidateReceiptBinary confirms the on-disk executable still matches the
// receipt. Core uses this before claiming ownership in a manifest.
func ValidateReceiptBinary(receipt Receipt) error {
	return verifyExecutable(receipt.BinaryPath, receipt.BinarySHA256)
}

func verifyExecutable(path, wantSHA string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return err
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return apperr.New(apperr.CodeConflict, "refusing to touch a symlinked executable")
	}
	if info.IsDir() {
		return apperr.New(apperr.CodeConflict, "recorded executable path is a directory")
	}
	got, err := fileSHA256(path)
	if err != nil {
		return err
	}
	if got != wantSHA {
		return apperr.New(apperr.CodeConflict, "executable hash does not match the install receipt")
	}
	return nil
}
