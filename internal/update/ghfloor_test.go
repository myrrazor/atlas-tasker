package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
)

// fakeGH puts a gh script that reports version first on PATH.
func fakeGH(t *testing.T, version string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fake gh")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\necho 'gh version " + version + " (2026-09-30)'\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("VERIFY_ATTESTATIONS", "")
}

func ghFloorOptions(t *testing.T) (Options, *int) {
	t.Helper()
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	exe := filepath.Join(t.TempDir(), "tracker")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	return Options{
		CurrentVersion:   "v1.0.0",
		TargetVersion:    "v9.9.9",
		BaseURL:          server.URL,
		AllowInsecureURL: true,
		ExecutablePath:   exe,
		HTTPClient:       server.Client(),
	}, &hits
}

func TestGHFloorBlocksAllModes(t *testing.T) {
	fakeGH(t, "2.101.0")
	for _, mode := range []string{"check", "dry-run", "yes"} {
		t.Run(mode, func(t *testing.T) {
			opts, hits := ghFloorOptions(t)
			switch mode {
			case "check":
				opts.CheckOnly = true
			case "dry-run":
				opts.DryRun = true
			case "yes":
				opts.Yes = true
			}
			_, err := Run(context.Background(), opts)
			if err == nil || !strings.Contains(err.Error(), GHTooOldMessage) || !strings.Contains(err.Error(), "Found gh 2.101.0") {
				t.Fatalf("expected gh floor error, got %v", err)
			}
			if code := apperr.ExitCode(err); code != 2 {
				t.Fatalf("exit code = %d, want 2", code)
			}
			if *hits != 0 {
				t.Fatalf("gh floor should fail before any network request, got %d", *hits)
			}
			if got, _ := os.ReadFile(opts.ExecutablePath); string(got) != "old" {
				t.Fatalf("executable changed: %q", got)
			}
		})
	}
}

func TestGHFloorPassesAt2102(t *testing.T) {
	fakeGH(t, "2.102.0")
	for _, mode := range []string{"check", "dry-run"} {
		t.Run(mode, func(t *testing.T) {
			opts, _ := ghFloorOptions(t)
			opts.CheckOnly = mode == "check"
			opts.DryRun = mode == "dry-run"
			result, err := Run(context.Background(), opts)
			if err != nil {
				t.Fatalf("%s with gh 2.102.0: %v", mode, err)
			}
			want := StatusUpdateAvailable
			if mode == "dry-run" {
				want = StatusWouldUpdate
			}
			if result.Status != want {
				t.Fatalf("status = %q, want %q", result.Status, want)
			}
		})
	}
	t.Run("yes", func(t *testing.T) {
		opts, _ := ghFloorOptions(t)
		opts.Yes = true
		// Floor passes; the run proceeds to download (404 here), not the gh error.
		_, err := Run(context.Background(), opts)
		if err != nil && strings.Contains(err.Error(), GHTooOldMessage) {
			t.Fatalf("gh 2.102.0 should pass the floor, got %v", err)
		}
	})
}

func TestGHFloorSkippedWithSkipAttestations(t *testing.T) {
	fakeGH(t, "2.101.0")
	opts, _ := ghFloorOptions(t)
	opts.CheckOnly = true
	opts.SkipAttestations = true
	if _, err := Run(context.Background(), opts); err != nil {
		t.Fatalf("--skip-attestations should not need gh: %v", err)
	}
}
