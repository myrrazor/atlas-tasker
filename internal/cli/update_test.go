package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/buildinfo"
	"github.com/myrrazor/atlas-tasker/internal/update"
)

func TestUpdateCommandCheckJSON(t *testing.T) {
	oldVersion := buildinfo.Version
	t.Cleanup(func() { buildinfo.Version = oldVersion })
	buildinfo.Version = "v1.0.0"

	tag := "v9.9.7"
	asset := update.ArchiveName("tracker", tag, runtime.GOOS, runtime.GOARCH)
	payload := []byte("tracker-check-binary")
	archiveBytes := mustCLITarGz(t, "tracker", payload)
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

	t.Setenv("RELEASE_BASE_URL", server.URL)
	t.Setenv("ALLOW_INSECURE_RELEASE_BASE_URL", "1")
	t.Setenv("VERIFY_ATTESTATIONS", "0")

	out, err := runCLI(t, "update", "--check", "--version", tag, "--json")
	if err != nil {
		t.Fatalf("update --check failed: %v\n%s", err, out)
	}
	var payloadJSON struct {
		FormatVersion   string `json:"format_version"`
		Kind            string `json:"kind"`
		Status          string `json:"status"`
		UpdateAvailable bool   `json:"update_available"`
		Asset           string `json:"asset"`
		CurrentVersion  string `json:"current_version"`
		TargetVersion   string `json:"target_version"`
	}
	if err := json.Unmarshal([]byte(out), &payloadJSON); err != nil {
		t.Fatalf("parse json: %v\n%s", err, out)
	}
	if payloadJSON.FormatVersion != jsonFormatVersion || payloadJSON.Kind != update.KindResult {
		t.Fatalf("unexpected envelope: %+v", payloadJSON)
	}
	if payloadJSON.Status != update.StatusUpdateAvailable || !payloadJSON.UpdateAvailable {
		t.Fatalf("expected update available, got %+v", payloadJSON)
	}
	if payloadJSON.Asset != asset || payloadJSON.CurrentVersion != "v1.0.0" || payloadJSON.TargetVersion != tag {
		t.Fatalf("unexpected versions/asset: %+v", payloadJSON)
	}
}

func TestUpdateCommandRequiresYes(t *testing.T) {
	oldVersion := buildinfo.Version
	t.Cleanup(func() { buildinfo.Version = oldVersion })
	buildinfo.Version = "v1.0.0"

	tag := "v9.9.6"
	asset := update.ArchiveName("tracker", tag, runtime.GOOS, runtime.GOARCH)
	archiveBytes := mustCLITarGz(t, "tracker", []byte("x"))
	sum := sha256.Sum256(archiveBytes)
	mux := http.NewServeMux()
	mux.HandleFunc("/"+asset, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archiveBytes)
	})
	mux.HandleFunc("/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), asset)))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	t.Setenv("RELEASE_BASE_URL", server.URL)
	t.Setenv("ALLOW_INSECURE_RELEASE_BASE_URL", "1")
	t.Setenv("VERIFY_ATTESTATIONS", "0")

	_, err := runCLI(t, "update", "--version", tag)
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected --yes requirement, got %v", err)
	}
}

func mustCLITarGz(t *testing.T, name string, payload []byte) []byte {
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

// Ensure the update binary path helper stays import-referenced in CLI tests.
var _ = os.TempDir
var _ = filepath.Join
