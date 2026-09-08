package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestArchiveName(t *testing.T) {
	got := ArchiveName("tracker", "v1.2.3", "linux", "amd64")
	want := "tracker_1.2.3_linux_amd64.tar.gz"
	if got != want {
		t.Fatalf("ArchiveName = %q, want %q", got, want)
	}
}

func TestUpdateAvailableMatrix(t *testing.T) {
	cases := []struct {
		name    string
		current string
		target  string
		force   bool
		want    bool
		wantErr bool
	}{
		{name: "newer available", current: "v1.0.0", target: "v1.1.0", want: true},
		{name: "same version", current: "v1.1.0", target: "v1.1.0", want: false},
		{name: "dev offers update", current: "dev", target: "v1.1.0", want: true},
		{name: "downgrade blocked", current: "v2.0.0", target: "v1.0.0", wantErr: true},
		{name: "downgrade forced", current: "v2.0.0", target: "v1.0.0", force: true, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := updateAvailable(tc.current, tc.target, tc.force)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("available=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestRunCheckAndApply(t *testing.T) {
	tag := "v9.9.9"
	asset := ArchiveName("tracker", tag, runtime.GOOS, runtime.GOARCH)
	payload := []byte("tracker-binary-v9.9.9")
	archiveBytes := mustTarGz(t, "tracker", payload)
	sum := sha256.Sum256(archiveBytes)
	checksums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), asset)

	mux := http.NewServeMux()
	mux.HandleFunc("/"+asset, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archiveBytes)
	})
	mux.HandleFunc("/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(checksums))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	exePath := filepath.Join(t.TempDir(), "tracker")
	if err := os.WriteFile(exePath, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("seed executable: %v", err)
	}

	attestCalls := 0
	base := Options{
		CurrentVersion:   "v1.0.0",
		TargetVersion:    tag,
		BaseURL:          server.URL,
		AllowInsecureURL: true,
		ExecutablePath:   exePath,
		GOOS:             runtime.GOOS,
		GOARCH:           runtime.GOARCH,
		HTTPClient:       server.Client(),
		RunAttest: func(ctx context.Context, archivePath, repo string) error {
			attestCalls++
			if filepath.Base(archivePath) != asset {
				t.Fatalf("unexpected archive path %q", archivePath)
			}
			if repo != DefaultRepo {
				t.Fatalf("unexpected repo %q", repo)
			}
			return nil
		},
	}

	checkOpts := base
	checkOpts.CheckOnly = true
	result, err := Run(context.Background(), checkOpts)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if result.Status != StatusUpdateAvailable || !result.UpdateAvailable || result.Asset != asset {
		t.Fatalf("unexpected check result: %+v", result)
	}

	dryOpts := base
	dryOpts.DryRun = true
	result, err = Run(context.Background(), dryOpts)
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if result.Status != StatusWouldUpdate {
		t.Fatalf("unexpected dry-run status: %+v", result)
	}
	if got, err := os.ReadFile(exePath); err != nil || string(got) != "old-binary" {
		t.Fatalf("dry-run mutated executable: %q err=%v", got, err)
	}

	_, err = Run(context.Background(), base)
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected --yes requirement, got %v", err)
	}

	applyOpts := base
	applyOpts.Yes = true
	result, err = Run(context.Background(), applyOpts)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result.Status != StatusUpdated || !result.Replaced || !result.ChecksumVerified || !result.AttestationVerified || attestCalls != 1 {
		t.Fatalf("unexpected apply result: %+v attestCalls=%d", result, attestCalls)
	}
	got, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatalf("read updated binary: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("updated binary = %q, want %q", got, payload)
	}

	upToDate := base
	upToDate.CurrentVersion = tag
	upToDate.Yes = true
	result, err = Run(context.Background(), upToDate)
	if err != nil {
		t.Fatalf("up-to-date: %v", err)
	}
	if result.Status != StatusUpToDate || result.UpdateAvailable {
		t.Fatalf("expected up to date, got %+v", result)
	}
}

func TestChecksumMismatchRejected(t *testing.T) {
	tag := "v9.9.8"
	asset := ArchiveName("tracker", tag, runtime.GOOS, runtime.GOARCH)
	archiveBytes := mustTarGz(t, "tracker", []byte("payload"))
	mux := http.NewServeMux()
	mux.HandleFunc("/"+asset, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archiveBytes)
	})
	mux.HandleFunc("/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("0000000000000000000000000000000000000000000000000000000000000000  " + asset + "\n"))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	exePath := filepath.Join(t.TempDir(), "tracker")
	if err := os.WriteFile(exePath, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), Options{
		CurrentVersion:   "v1.0.0",
		TargetVersion:    tag,
		Yes:              true,
		BaseURL:          server.URL,
		AllowInsecureURL: true,
		SkipAttestations: true,
		ExecutablePath:   exePath,
		GOOS:             runtime.GOOS,
		GOARCH:           runtime.GOARCH,
		HTTPClient:       server.Client(),
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "checksum") {
		t.Fatalf("expected checksum error, got %v", err)
	}
}

func mustTarGz(t *testing.T, name string, payload []byte) []byte {
	t.Helper()
	var raw bytes.Buffer
	gz := gzip.NewWriter(&raw)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(payload))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return raw.Bytes()
}
