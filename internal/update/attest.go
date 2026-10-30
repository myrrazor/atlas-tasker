package update

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
)

// AttestationBundleAsset is the Sigstore bundle every release publishes next
// to checksums.txt. scripts/install.sh verifies the same file.
const AttestationBundleAsset = "attestation-bundle.jsonl"

// AttestRequest is the exact policy `gh attestation verify` checks. It mirrors
// scripts/install.sh: --bundle (offline, no GitHub token), --repo,
// --signer-workflow <repo>/.github/workflows/release.yml and
// --source-ref refs/tags/<tag being installed>.
type AttestRequest struct {
	ArchivePath    string
	BundlePath     string
	Repo           string
	SignerWorkflow string
	SourceRef      string
}

func newAttestRequest(archivePath, bundlePath, repo, tag string) (AttestRequest, error) {
	tag = strings.TrimSpace(tag)
	if err := validateTag(tag); err != nil {
		return AttestRequest{}, err
	}
	return AttestRequest{
		ArchivePath:    archivePath,
		BundlePath:     bundlePath,
		Repo:           repo,
		SignerWorkflow: repo + "/.github/workflows/release.yml",
		SourceRef:      "refs/tags/" + tag,
	}, nil
}

func (r AttestRequest) policyArgs() []string {
	return []string{
		"--bundle", r.BundlePath,
		"--signer-workflow", r.SignerWorkflow,
		"--source-ref", r.SourceRef,
	}
}

func releaseBundleURL(opts Options, tag string) string {
	if opts.BaseURL != "" {
		return strings.TrimRight(opts.BaseURL, "/") + "/" + AttestationBundleAsset
	}
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", opts.Repo, tag, AttestationBundleAsset)
}

// downloadAttestationBundle fails closed: a missing or empty bundle stops the
// update. There is no fallback to token-based or unpinned verification.
func downloadAttestationBundle(ctx context.Context, opts Options, tag, dest string) error {
	if err := downloadFile(ctx, opts.HTTPClient, releaseBundleURL(opts, tag), dest); err != nil {
		return apperr.Wrap(apperr.CodeInvalidInput, err, "release %s has no %s; provenance could not be verified, refusing to update (the binary was not changed)", tag, AttestationBundleAsset)
	}
	if info, err := os.Stat(dest); err != nil || info.Size() == 0 {
		return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("release %s has an empty %s; provenance could not be verified, refusing to update (the binary was not changed)", tag, AttestationBundleAsset))
	}
	return nil
}
