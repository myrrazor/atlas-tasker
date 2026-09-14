package uninstall

import (
	"context"
	"time"
)

const (
	ReceiptFormat  = "atlas_install_receipt_v1"
	ManifestFormat = "atlas_uninstall_manifest_v1"
	ReceiptFile    = "install-receipt.json"
	ManifestFile   = "uninstall-manifest.json"
	KindResult     = "tracker_uninstall"

	KindStopService        = "stop_service"
	KindRemoveFile         = "remove_file"
	KindRemoveManagedBlock = "remove_managed_block"
	KindRemoveConfigEntry  = "remove_config_entry"
	KindPackageManager     = "package_manager"

	MethodScript    = "script"
	MethodHomebrew  = "homebrew"
	MethodApt       = "apt"
	MethodPacman    = "pacman"
	MethodSource    = "source"
	MethodGoInstall = "go-install"
	MethodUnknown   = "unknown"

	StatusPreview                = "preview"
	StatusApplied                = "applied"
	StatusRefused                = "refused"
	StatusAlreadyUninstalled     = "already_uninstalled"
	StatusSoftwareStillInstalled = "software_still_installed"
)

// CommandRunner runs allowlisted service-control binaries. Tests inject a fake;
// production uses HostCommandRunner.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) error
}

// Options is the planner/apply input. Home and Getenv must be fixture-scoped
// in tests. Command must be a fake in tests; nil selects HostCommandRunner.
type Options struct {
	Home     string
	StateDir string
	Getenv   func(string) string
	GOOS     string
	Now      func() time.Time
	Command  CommandRunner
}

// Receipt is the installer-created ownership record. Digest is SHA-256 of
// binary_path, binary_sha256, install_method, and version, each followed by \n.
type Receipt struct {
	Format        string      `json:"format"`
	CreatedAt     time.Time   `json:"created_at"`
	Version       string      `json:"version"`
	InstallMethod string      `json:"install_method"`
	BinaryPath    string      `json:"binary_path"`
	BinarySHA256  string      `json:"binary_sha256"`
	OwnedPaths    []OwnedPath `json:"owned_paths"`
	Digest        string      `json:"digest"`
}

type OwnedPath struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	SHA256 string `json:"sha256,omitempty"`
}

// Manifest is the Core-produced extra uninstall action list. Recovery also
// discovers Atlas-owned units and managed entries; Core calls NewManifest /
// WriteManifest to publish the same facts.
type Manifest struct {
	Format        string           `json:"format"`
	GeneratedAt   time.Time        `json:"generated_at"`
	ReceiptDigest string           `json:"receipt_digest"`
	Actions       []ManifestAction `json:"actions"`
	Digest        string           `json:"digest"`
}

type ManifestAction struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Label        string   `json:"label,omitempty"`
	UnitName     string   `json:"unit_name,omitempty"`
	Path         string   `json:"path,omitempty"`
	Marker       string   `json:"marker,omitempty"`
	EntryKey     string   `json:"entry_key,omitempty"`
	ConfigFormat string   `json:"config_format,omitempty"` // json | toml | markdown
	BinaryPath   string   `json:"binary_path,omitempty"`
	BinarySHA256 string   `json:"binary_sha256,omitempty"`
	Args         []string `json:"args,omitempty"`
	PairPath     string   `json:"pair_path,omitempty"`
}

type Plan struct {
	Kind      string       `json:"kind"`
	Status    string       `json:"status"`
	PlanID    string       `json:"plan_id"`
	Digest    string       `json:"digest"`
	CanApply  bool         `json:"can_apply"`
	Refusal   string       `json:"refusal,omitempty"`
	Actions   []PlanAction `json:"actions"`
	Preserved []string     `json:"preserved"`
	Notes     []string     `json:"notes,omitempty"`
}

type PlanAction struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Path         string   `json:"path,omitempty"`
	PairPath     string   `json:"pair_path,omitempty"`
	Detail       string   `json:"detail,omitempty"`
	Verified     bool     `json:"verified"`
	UnitName     string   `json:"unit_name,omitempty"`
	Marker       string   `json:"marker,omitempty"`
	EntryKey     string   `json:"entry_key,omitempty"`
	ConfigFormat string   `json:"config_format,omitempty"`
	BinaryPath   string   `json:"binary_path,omitempty"`
	Args         []string `json:"args,omitempty"`
}

type Result struct {
	Kind      string   `json:"kind"`
	Status    string   `json:"status"`
	PlanID    string   `json:"plan_id"`
	Digest    string   `json:"digest"`
	Applied   []string `json:"applied"`
	Skipped   []string `json:"skipped"`
	Failed    []string `json:"failed,omitempty"`
	Preserved []string `json:"preserved"`
	Notes     []string `json:"notes,omitempty"`
}
