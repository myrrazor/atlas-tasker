package config

import (
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestLoadDefaultsIncludeV15Config(t *testing.T) {
	root := t.TempDir()

	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Provider.DefaultSCMProvider != contracts.ChangeProviderLocal {
		t.Fatalf("expected local provider default, got %s", cfg.Provider.DefaultSCMProvider)
	}
	if cfg.ImportExport.MaxBundleSizeMB != 512 {
		t.Fatalf("expected default bundle size, got %d", cfg.ImportExport.MaxBundleSizeMB)
	}
	if !cfg.ImportExport.RequireVerification {
		t.Fatalf("expected import verification default on")
	}
	if !cfg.Release.VerifyChecksums || !cfg.Release.VerifyAttestations {
		t.Fatalf("expected release verification defaults on")
	}
}

func TestSaveAndLoadV15ConfigRoundTrip(t *testing.T) {
	root := t.TempDir()
	cfg := defaultConfig()
	cfg.Provider.DefaultSCMProvider = contracts.ChangeProviderGitHub
	cfg.Provider.DefaultBaseBranch = "develop"
	cfg.Provider.GitHubRepo = "myrrazor/atlas-tasker"
	cfg.ImportExport.MaxBundleSizeMB = 1024
	cfg.ImportExport.AllowUpdateExisting = true
	cfg.Release.BaseMarker = "v1.5-base-4f1782e"
	cfg.Release.BaseSHA = "4f1782e3ef2eaeed06ae0724bd6dc0162a18d940"

	if err := Save(root, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	loaded, err := Load(root)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if loaded.Provider.DefaultSCMProvider != contracts.ChangeProviderGitHub {
		t.Fatalf("expected github provider, got %s", loaded.Provider.DefaultSCMProvider)
	}
	if loaded.Provider.DefaultBaseBranch != "develop" {
		t.Fatalf("expected develop base branch, got %s", loaded.Provider.DefaultBaseBranch)
	}
	if loaded.ImportExport.MaxBundleSizeMB != 1024 {
		t.Fatalf("expected bundle size to round-trip, got %d", loaded.ImportExport.MaxBundleSizeMB)
	}
	if !loaded.ImportExport.AllowUpdateExisting {
		t.Fatalf("expected allow_update_existing to round-trip")
	}
	if loaded.Release.BaseMarker != "v1.5-base-4f1782e" {
		t.Fatalf("expected base marker to round-trip, got %s", loaded.Release.BaseMarker)
	}
	if loaded.Release.BaseSHA != "4f1782e3ef2eaeed06ae0724bd6dc0162a18d940" {
		t.Fatalf("expected base sha to round-trip, got %s", loaded.Release.BaseSHA)
	}
}
