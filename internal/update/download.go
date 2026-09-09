package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
)

func downloadVerifyReplace(ctx context.Context, opts Options, tag, asset string) (applyMeta, error) {
	meta := applyMeta{}
	if strings.TrimSpace(opts.ExecutablePath) == "" {
		return meta, apperr.New(apperr.CodeInternal, "cannot locate current tracker executable")
	}
	if opts.BaseURL != "" {
		if err := validateBaseURL(opts.BaseURL, opts.AllowInsecureURL); err != nil {
			return meta, err
		}
	}

	tmpDir, err := os.MkdirTemp("", "tracker-update-*")
	if err != nil {
		return meta, apperr.Wrap(apperr.CodeInternal, err, "create update temp dir")
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, asset)
	if err := downloadFile(ctx, opts.HTTPClient, releaseAssetURL(opts, tag, asset), archivePath); err != nil {
		return meta, err
	}

	checksumsPath := filepath.Join(tmpDir, "checksums.txt")
	if err := downloadFile(ctx, opts.HTTPClient, releaseChecksumsURL(opts, tag), checksumsPath); err != nil {
		return meta, err
	}
	if err := verifyChecksum(archivePath, asset, checksumsPath); err != nil {
		return meta, err
	}
	meta.ChecksumVerified = true

	if opts.SkipAttestations {
		meta.AttestationSkipped = true
	} else {
		if err := opts.RunAttest(ctx, archivePath, opts.Repo); err != nil {
			return meta, err
		}
		meta.AttestationVerified = true
	}

	binaryPath, err := extractBinary(archivePath, tmpDir, opts.BinaryName)
	if err != nil {
		return meta, err
	}
	if err := opts.Replace(binaryPath, opts.ExecutablePath); err != nil {
		return meta, err
	}
	return meta, nil
}

func releaseAssetURL(opts Options, tag, asset string) string {
	if opts.BaseURL != "" {
		return strings.TrimRight(opts.BaseURL, "/") + "/" + asset
	}
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", opts.Repo, tag, asset)
}

func releaseChecksumsURL(opts Options, tag string) string {
	if opts.BaseURL != "" {
		return strings.TrimRight(opts.BaseURL, "/") + "/checksums.txt"
	}
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/checksums.txt", opts.Repo, tag)
}

func downloadFile(ctx context.Context, client *http.Client, rawURL, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "atlas-tasker-update")
	resp, err := client.Do(req)
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, err, "download %s", rawURL)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		code := apperr.CodeInternal
		if resp.StatusCode == http.StatusNotFound {
			code = apperr.CodeNotFound
		}
		return apperr.New(code, fmt.Sprintf("download failed for %s: HTTP %d", rawURL, resp.StatusCode))
	}
	file, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, err, "create download file")
	}
	defer file.Close()
	if _, err := io.Copy(file, io.LimitReader(resp.Body, 256<<20)); err != nil {
		return apperr.Wrap(apperr.CodeInternal, err, "write download file")
	}
	return nil
}

func verifyChecksum(archivePath, archiveName, checksumsPath string) error {
	raw, err := os.ReadFile(checksumsPath)
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, err, "read checksums.txt")
	}
	expected := ""
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := filepath.Base(fields[1])
		if name == archiveName {
			expected = fields[0]
			break
		}
	}
	if expected == "" {
		return apperr.New(apperr.CodeNotFound, fmt.Sprintf("checksum entry missing for %s", archiveName))
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, err, "open archive for checksum")
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return apperr.Wrap(apperr.CodeInternal, err, "hash archive")
	}
	actual := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(expected, actual) {
		return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("checksum mismatch for %s: expected %s got %s", archiveName, expected, actual))
	}
	return nil
}

func extractBinary(archivePath, destDir, binaryName string) (string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return "", apperr.Wrap(apperr.CodeInternal, err, "open archive")
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return "", apperr.Wrap(apperr.CodeInvalidInput, err, "ungzip archive")
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", apperr.Wrap(apperr.CodeInvalidInput, err, "read archive entry")
		}
		name := filepath.Base(header.Name)
		if name != binaryName || header.Typeflag != tar.TypeReg {
			continue
		}
		outPath := filepath.Join(destDir, binaryName)
		out, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return "", apperr.Wrap(apperr.CodeInternal, err, "create extracted binary")
		}
		if _, err := io.Copy(out, io.LimitReader(tr, 256<<20)); err != nil {
			out.Close()
			return "", apperr.Wrap(apperr.CodeInternal, err, "extract binary")
		}
		if err := out.Close(); err != nil {
			return "", err
		}
		return outPath, nil
	}
	return "", apperr.New(apperr.CodeNotFound, fmt.Sprintf("archive does not contain %s", binaryName))
}
