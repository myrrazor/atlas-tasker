package update

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
)

func defaultAttest(ctx context.Context, archivePath, repo string) error {
	gh, err := exec.LookPath("gh")
	if err != nil {
		return apperr.New(apperr.CodeInvalidInput, "gh is required for attestation verification; install GitHub CLI or pass --skip-attestations")
	}
	cmd := exec.CommandContext(ctx, gh, "attestation", "verify", archivePath, "--repo", repo)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return apperr.Wrap(apperr.CodeInvalidInput, err, "attestation verification failed for %s", filepath.Base(archivePath))
	}
	return nil
}

func replaceExecutable(srcBinary, destPath string) error {
	destPath, err := filepath.Abs(destPath)
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, err, "resolve executable path")
	}
	info, err := os.Stat(destPath)
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, err, "stat current executable")
	}
	if info.IsDir() {
		return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("executable path is a directory: %s", destPath))
	}

	dir := filepath.Dir(destPath)
	tmp, err := os.CreateTemp(dir, ".tracker-update-*")
	if err != nil {
		return apperr.Wrap(apperr.CodePermissionDenied, err, "create staging file next to executable (is the install directory writable?)")
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	src, err := os.Open(srcBinary)
	if err != nil {
		tmp.Close()
		return apperr.Wrap(apperr.CodeInternal, err, "open downloaded binary")
	}
	if _, err := io.Copy(tmp, src); err != nil {
		src.Close()
		tmp.Close()
		return apperr.Wrap(apperr.CodeInternal, err, "stage downloaded binary")
	}
	src.Close()
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		tmp.Close()
		return apperr.Wrap(apperr.CodeInternal, err, "chmod staged binary")
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return apperr.Wrap(apperr.CodeInternal, err, "fsync staged binary")
	}
	if err := tmp.Close(); err != nil {
		return apperr.Wrap(apperr.CodeInternal, err, "close staged binary")
	}

	backup := destPath + ".bak"
	_ = os.Remove(backup)
	if err := os.Rename(destPath, backup); err != nil {
		return apperr.Wrap(apperr.CodePermissionDenied, err, "backup current executable")
	}
	if err := os.Rename(tmpName, destPath); err != nil {
		_ = os.Rename(backup, destPath)
		return apperr.Wrap(apperr.CodePermissionDenied, err, "replace executable")
	}
	cleanup = false
	_ = os.Remove(backup)
	return nil
}
