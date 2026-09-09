// Package update implements production-grade self-update for the tracker binary.
//
// The flow mirrors scripts/install.sh: resolve a GitHub release tag, download the
// platform archive, verify checksums.txt, optionally verify attestations with
// `gh attestation verify`, then atomically replace the running executable.
package update

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"golang.org/x/mod/semver"
)

const (
	DefaultRepo   = "myrrazor/atlas-tasker"
	DefaultBinary = "tracker"
	KindResult    = "tracker_update"
)

const (
	StatusUpToDate        = "up_to_date"
	StatusUpdateAvailable = "update_available"
	StatusWouldUpdate     = "would_update"
	StatusUpdated         = "updated"
)

// Options configures a check or apply run.
type Options struct {
	CurrentVersion string
	TargetVersion  string // empty selects GitHub "latest"
	Repo           string
	BinaryName     string

	CheckOnly bool
	DryRun    bool
	Yes       bool
	Force     bool

	SkipAttestations bool
	BaseURL          string // RELEASE_BASE_URL; empty uses GitHub downloads
	AllowInsecureURL bool

	HTTPClient *http.Client
	Stdout     io.Writer
	Stderr     io.Writer

	// ExecutablePath overrides os.Executable (tests and unusual installs).
	ExecutablePath string
	GOOS           string
	GOARCH         string

	RunAttest func(ctx context.Context, archivePath, repo string) error
	Replace   func(srcBinary, destPath string) error
}

// Result is the stable JSON shape behind `tracker update --json`.
type Result struct {
	Kind                string `json:"kind"`
	Status              string `json:"status"`
	CurrentVersion      string `json:"current_version"`
	TargetVersion       string `json:"target_version"`
	LatestVersion       string `json:"latest_version,omitempty"`
	UpdateAvailable     bool   `json:"update_available"`
	Asset               string `json:"asset,omitempty"`
	Executable          string `json:"executable,omitempty"`
	ChecksumVerified    bool   `json:"checksum_verified"`
	AttestationVerified bool   `json:"attestation_verified"`
	AttestationSkipped  bool   `json:"attestation_skipped"`
	Replaced            bool   `json:"replaced"`
	Message             string `json:"message"`
}

type applyMeta struct {
	ChecksumVerified    bool
	AttestationVerified bool
	AttestationSkipped  bool
}

// Run checks for and optionally applies a release update.
func Run(ctx context.Context, opts Options) (Result, error) {
	opts = normalizeOptions(opts)
	result := Result{
		Kind:           KindResult,
		CurrentVersion: opts.CurrentVersion,
		Executable:     opts.ExecutablePath,
	}

	tag, err := resolveTargetTag(ctx, opts)
	if err != nil {
		return result, err
	}
	result.TargetVersion = tag
	result.LatestVersion = tag
	result.Asset = ArchiveName(opts.BinaryName, tag, opts.GOOS, opts.GOARCH)

	available, err := updateAvailable(opts.CurrentVersion, tag, opts.Force)
	if err != nil {
		return result, err
	}
	result.UpdateAvailable = available
	if !available {
		result.Status = StatusUpToDate
		result.Message = fmt.Sprintf("already on %s", tag)
		return result, nil
	}

	if opts.CheckOnly {
		result.Status = StatusUpdateAvailable
		result.Message = fmt.Sprintf("update available: %s -> %s", opts.CurrentVersion, tag)
		return result, nil
	}
	if opts.DryRun {
		result.Status = StatusWouldUpdate
		result.Message = fmt.Sprintf("would update %s -> %s (%s)", opts.CurrentVersion, tag, result.Asset)
		return result, nil
	}
	if !opts.Yes {
		return result, apperr.New(apperr.CodeInvalidInput, "update requires --yes to replace the binary (or pass --check / --dry-run)")
	}

	meta, err := downloadVerifyReplace(ctx, opts, tag, result.Asset)
	if err != nil {
		return result, err
	}
	result.ChecksumVerified = meta.ChecksumVerified
	result.AttestationVerified = meta.AttestationVerified
	result.AttestationSkipped = meta.AttestationSkipped
	result.Replaced = true
	result.Status = StatusUpdated
	result.Message = fmt.Sprintf("updated %s -> %s", opts.CurrentVersion, tag)
	return result, nil
}

func normalizeOptions(opts Options) Options {
	if strings.TrimSpace(opts.Repo) == "" {
		opts.Repo = DefaultRepo
	}
	if strings.TrimSpace(opts.BinaryName) == "" {
		opts.BinaryName = DefaultBinary
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 60 * time.Second}
	}
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	if opts.GOOS == "" {
		opts.GOOS = runtime.GOOS
	}
	if opts.GOARCH == "" {
		opts.GOARCH = runtime.GOARCH
	}
	if opts.RunAttest == nil {
		opts.RunAttest = defaultAttest
	}
	if opts.Replace == nil {
		opts.Replace = replaceExecutable
	}
	if strings.TrimSpace(opts.ExecutablePath) == "" {
		if path, err := os.Executable(); err == nil {
			opts.ExecutablePath = path
		}
	}
	if opts.BaseURL == "" {
		opts.BaseURL = strings.TrimSpace(os.Getenv("RELEASE_BASE_URL"))
	}
	if os.Getenv("ALLOW_INSECURE_RELEASE_BASE_URL") == "1" {
		opts.AllowInsecureURL = true
	}
	if os.Getenv("VERIFY_ATTESTATIONS") == "0" {
		opts.SkipAttestations = true
	}
	opts.CurrentVersion = strings.TrimSpace(opts.CurrentVersion)
	opts.TargetVersion = strings.TrimSpace(opts.TargetVersion)
	opts.BaseURL = strings.TrimSpace(opts.BaseURL)
	return opts
}

// ArchiveName matches scripts/install.sh asset naming.
func ArchiveName(binary, tag, goos, goarch string) string {
	version := strings.TrimPrefix(tag, "v")
	return fmt.Sprintf("%s_%s_%s_%s.tar.gz", binary, version, goos, normalizeArch(goarch))
}

func normalizeArch(arch string) string {
	switch arch {
	case "x86_64":
		return "amd64"
	case "aarch64":
		return "arm64"
	default:
		return arch
	}
}

func updateAvailable(current, target string, force bool) (bool, error) {
	if force {
		return true, nil
	}
	currentNorm := normalizeSemver(current)
	targetNorm := normalizeSemver(target)
	if currentNorm == "" {
		// Unstamped source builds (dev) should still offer a release update.
		return true, nil
	}
	if targetNorm == "" {
		return false, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("unsafe or invalid target version: %s", target))
	}
	cmp := semver.Compare(currentNorm, targetNorm)
	if cmp == 0 {
		return false, nil
	}
	if cmp > 0 {
		return false, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("current version %s is newer than target %s; pass --force to install anyway", current, target))
	}
	return true, nil
}

func normalizeSemver(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "dev" || raw == "unknown" {
		return ""
	}
	if !strings.HasPrefix(raw, "v") {
		raw = "v" + raw
	}
	if !semver.IsValid(raw) {
		return ""
	}
	return raw
}

// FormatPretty renders a human-readable update summary.
func FormatPretty(result Result) string {
	switch result.Status {
	case StatusUpToDate:
		return fmt.Sprintf("tracker %s is up to date", result.CurrentVersion)
	case StatusUpdateAvailable:
		return fmt.Sprintf("update available: %s -> %s\nasset: %s\nrun: tracker update --yes", result.CurrentVersion, result.TargetVersion, result.Asset)
	case StatusWouldUpdate:
		return fmt.Sprintf("would update %s -> %s\nasset: %s", result.CurrentVersion, result.TargetVersion, result.Asset)
	case StatusUpdated:
		lines := []string{
			fmt.Sprintf("updated tracker %s -> %s", result.CurrentVersion, result.TargetVersion),
			fmt.Sprintf("executable: %s", result.Executable),
		}
		if result.ChecksumVerified {
			lines = append(lines, "checksum: verified")
		}
		if result.AttestationVerified {
			lines = append(lines, "attestation: verified")
		} else if result.AttestationSkipped {
			lines = append(lines, "attestation: skipped")
		}
		return strings.Join(lines, "\n")
	default:
		return result.Message
	}
}
