package contracts

import (
	"encoding/json"
	"testing"
	"time"
)

func FuzzRestorePlanItemValidate(f *testing.F) {
	for _, seed := range []string{
		"projects/APP/tickets/APP-1.md",
		".tracker/security/keys/private/key.json",
		"../outside",
		".tracker/audit/reports/report.json",
		" projects/APP/tickets/APP-1.md ",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, path string) {
		item := RestorePlanItem{Path: path, Action: RestorePlanUpdate}
		_ = item.Validate()
		item.Action = RestorePlanBlock
		item.ReasonCodes = []string{"fuzz"}
		_ = item.Validate()
	})
}

func FuzzGoalManifestDecodeValidate(f *testing.F) {
	now := time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC)
	valid := GoalManifest{
		ManifestID:    "goal_fuzz",
		TargetKind:    GoalTargetTicket,
		TargetID:      "APP-1",
		Objective:     "Fuzz the manifest parser",
		Sections:      completeGoalSections(),
		SourceHash:    "abc123",
		GeneratedAt:   now,
		GeneratedBy:   Actor("human:owner"),
		Reason:        "fuzz seed",
		SchemaVersion: CurrentSchemaVersion,
	}
	raw, err := json.Marshal(valid)
	if err != nil {
		f.Fatalf("marshal seed: %v", err)
	}
	f.Add(string(raw))
	f.Add(`{"manifest_id":"goal_bad","sections":[]}`)
	f.Add(`not-json`)
	f.Fuzz(func(t *testing.T, raw string) {
		var manifest GoalManifest
		if err := json.Unmarshal([]byte(raw), &manifest); err != nil {
			return
		}
		_ = manifest.Validate()
	})
}
