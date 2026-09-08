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
	if cfg.Web.AgentColors["claude"] != "orange" || cfg.Web.AgentColors["codex"] != "blue" {
		t.Fatalf("unexpected web agent color defaults: %#v", cfg.Web.AgentColors)
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
	cfg.Web.OwnerName = "Master Hit"
	cfg.Web.Lang = "es"
	cfg.Web.AgentColors["merlin"] = "orange"

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
	if loaded.Web.OwnerName != "Master Hit" || loaded.Web.Lang != "es" || loaded.Web.AgentColors["merlin"] != "orange" {
		t.Fatalf("expected web config to round-trip, got %#v", loaded.Web)
	}
}

func TestGetAndSetWebConfig(t *testing.T) {
	root := t.TempDir()

	if err := Set(root, "web.owner_name", "  Atlas Owner  "); err != nil {
		t.Fatalf("set owner name: %v", err)
	}
	if err := Set(root, "web.agent_colors.Merlin", " Orange "); err != nil {
		t.Fatalf("set agent color: %v", err)
	}
	if err := Set(root, "web.lang", " ES "); err != nil {
		t.Fatalf("set web language: %v", err)
	}
	owner, err := Get(root, "web.owner_name")
	if err != nil {
		t.Fatalf("get owner name: %v", err)
	}
	color, err := Get(root, "web.agent_colors.merlin")
	if err != nil {
		t.Fatalf("get agent color: %v", err)
	}
	lang, err := Get(root, "web.lang")
	if err != nil {
		t.Fatalf("get web language: %v", err)
	}
	if owner != "Atlas Owner" || color != "orange" || lang != "es" {
		t.Fatalf("unexpected web config values owner=%q color=%q lang=%q", owner, color, lang)
	}
	if err := Set(root, "web.lang", "fr"); err == nil {
		t.Fatal("expected unsupported web language to be rejected")
	}
}
