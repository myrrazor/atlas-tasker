package update

import (
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

// fakeGHScript stands in for the real gh. It answers --version, logs the argv of
// `attestation verify`, and, like the real policy check, fails unless the
// signer workflow and source ref match what the attestation was built from.
const fakeGHScript = `#!/bin/sh
if [ "$1" = "--version" ]; then echo "gh version 2.103.0 (2026-10-01)"; exit 0; fi
printf '%s\n' "$@" > "$FAKE_GH_LOG"
if [ -n "${GH_TOKEN:-}${GITHUB_TOKEN:-}" ]; then echo "token must not be needed" >&2; exit 9; fi
[ "$1 $2" = "attestation verify" ] || exit 3
workflow=""; ref=""; bundle=""
while [ $# -gt 0 ]; do
  case "$1" in
    --signer-workflow) workflow="$2"; shift ;;
    --source-ref) ref="$2"; shift ;;
    --bundle) bundle="$2"; shift ;;
  esac
  shift
done
[ -s "$bundle" ] || { echo "no bundle" >&2; exit 4; }
[ "$workflow" = "$ATTESTED_WORKFLOW" ] || { echo "signer workflow mismatch" >&2; exit 1; }
[ "$ref" = "$ATTESTED_REF" ] || { echo "source ref mismatch" >&2; exit 1; }
exit 0
`

func installFakeGH(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(fakeGHScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	log := filepath.Join(dir, "argv.log")
	t.Setenv("FAKE_GH_LOG", log)
	return log
}

func TestDefaultAttestUsesInstallerPolicyWithoutToken(t *testing.T) {
	log := installFakeGH(t)
	dir := t.TempDir()
	archive := filepath.Join(dir, "tracker_1.18.1_linux_amd64.tar.gz")
	bundle := filepath.Join(dir, AttestationBundleAsset)
	for _, p := range []string{archive, bundle} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	req, err := newAttestRequest(archive, bundle, DefaultRepo, "v1.18.1")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ATTESTED_WORKFLOW", "myrrazor/atlas-tasker/.github/workflows/release.yml")
	t.Setenv("ATTESTED_REF", "refs/tags/v1.18.1")
	if err := defaultAttest(context.Background(), req); err != nil {
		t.Fatalf("matching attestation must verify with no token: %v", err)
	}
	raw, _ := os.ReadFile(log)
	want := []string{"attestation", "verify", archive,
		"--repo", "myrrazor/atlas-tasker",
		"--bundle", bundle,
		"--signer-workflow", "myrrazor/atlas-tasker/.github/workflows/release.yml",
		"--source-ref", "refs/tags/v1.18.1"}
	if got := strings.Split(strings.TrimSpace(string(raw)), "\n"); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("gh argv\n got %q\nwant %q", got, want)
	}

	t.Run("signer workflow mismatch fails", func(t *testing.T) {
		t.Setenv("ATTESTED_WORKFLOW", "myrrazor/atlas-tasker/.github/workflows/other.yml")
		if err := defaultAttest(context.Background(), req); err == nil || !strings.Contains(err.Error(), "attestation verification failed") {
			t.Fatalf("want attestation failure, got %v", err)
		}
	})
	t.Run("source ref mismatch fails", func(t *testing.T) {
		t.Setenv("ATTESTED_REF", "refs/tags/v1.18.0")
		if err := defaultAttest(context.Background(), req); err == nil {
			t.Fatal("an attestation for another tag must not verify")
		}
	})
}

func TestNewAttestRequestRejectsUnsafeTag(t *testing.T) {
	if _, err := newAttestRequest("a", "b", DefaultRepo, "v1.0.0 --source-ref refs/heads/main"); err == nil {
		t.Fatal("unsafe tag must be rejected")
	}
}

func TestUpdateFailsClosedWithoutAttestationBundle(t *testing.T) {
	for _, tc := range []struct {
		name   string
		bundle []byte // nil means 404
		want   string
	}{
		{"missing", nil, "has no attestation-bundle.jsonl"},
		{"empty", []byte{}, "has an empty attestation-bundle.jsonl"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tag := "v9.9.9"
			asset := ArchiveName("tracker", tag, runtime.GOOS, runtime.GOARCH)
			archiveBytes := mustTarGz(t, "tracker", []byte("new"))
			sum := sha256.Sum256(archiveBytes)
			mux := http.NewServeMux()
			mux.HandleFunc("/"+asset, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(archiveBytes) })
			mux.HandleFunc("/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), asset)
			})
			if tc.bundle != nil {
				mux.HandleFunc("/"+AttestationBundleAsset, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(tc.bundle) })
			}
			server := httptest.NewServer(mux)
			t.Cleanup(server.Close)
			exe := filepath.Join(t.TempDir(), "tracker")
			if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
				t.Fatal(err)
			}
			called := false
			_, err := Run(context.Background(), Options{
				CurrentVersion: "v1.0.0", TargetVersion: tag, Yes: true,
				BaseURL: server.URL, AllowInsecureURL: true, HTTPClient: server.Client(),
				ExecutablePath: exe, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
				RunAttest: func(context.Context, AttestRequest) error { called = true; return nil },
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "refusing to update") {
				t.Fatalf("want fail-closed error %q, got %v", tc.want, err)
			}
			if called {
				t.Fatal("attestation must not run (or fall back) without a bundle")
			}
			if got, _ := os.ReadFile(exe); string(got) != "old" {
				t.Fatalf("executable changed: %q", got)
			}
		})
	}
}
