package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTeamListShowsPresets(t *testing.T) {
	withTempWorkspace(t)
	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	out, err := runCLI(t, "team", "list", "--json")
	if err != nil {
		t.Fatalf("team list failed: %v", err)
	}
	var payload struct {
		Kind    string `json:"kind"`
		Presets []struct {
			Name    string `json:"name"`
			Summary string `json:"summary"`
		} `json:"presets"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("parse team list payload: %v\nraw=%s", err, out)
	}
	if payload.Kind != "team_preset_list" || len(payload.Presets) != 4 {
		t.Fatalf("expected 4 presets, got %#v", payload)
	}
}

func TestTeamApplyPairEndToEnd(t *testing.T) {
	withTempWorkspace(t)
	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	// dry-run first: previews without creating
	out, err := runCLI(t, "team", "apply", "pair", "--dry-run", "--json", "--actor", "human:owner", "--reason", "preview")
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	var dry struct {
		Result struct {
			DryRun  bool     `json:"dry_run"`
			Created []string `json:"created"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out), &dry); err != nil {
		t.Fatalf("parse dry-run payload: %v\nraw=%s", err, out)
	}
	if !dry.Result.DryRun || len(dry.Result.Created) == 0 {
		t.Fatalf("expected dry-run preview, got %s", out)
	}
	if _, err := runCLI(t, "agent", "view", "builder-1", "--json"); err == nil {
		t.Fatal("dry-run must not create agents")
	}

	// real apply creates the roster
	out, err = runCLI(t, "team", "apply", "pair", "--json", "--actor", "human:owner", "--reason", "team setup")
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if !strings.Contains(out, "builder-1") || !strings.Contains(out, "reviewer-1") {
		t.Fatalf("expected roster in apply output, got %s", out)
	}
	if _, err := runCLI(t, "agent", "view", "builder-1", "--json"); err != nil {
		t.Fatalf("builder-1 should exist after apply: %v", err)
	}

	// second apply is a no-op
	out, err = runCLI(t, "team", "apply", "pair", "--json", "--actor", "human:owner", "--reason", "again")
	if err != nil {
		t.Fatalf("re-apply failed: %v", err)
	}
	var again struct {
		Result struct {
			Created []string `json:"created"`
			Skipped []string `json:"skipped"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out), &again); err != nil {
		t.Fatalf("parse re-apply payload: %v\nraw=%s", err, out)
	}
	if len(again.Result.Created) != 0 || len(again.Result.Skipped) == 0 {
		t.Fatalf("expected idempotent re-apply, got %s", out)
	}
}
