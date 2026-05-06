package contracts

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestChangeStatusAllowsFrozenTransitions(t *testing.T) {
	if !ChangeStatusLocalOnly.Allows(ChangeStatusDraft) {
		t.Fatalf("local_only should allow draft")
	}
	if !ChangeStatusReviewRequested.Allows(ChangeStatusApproved) {
		t.Fatalf("review_requested should allow approved")
	}
	if ChangeStatusMerged.Allows(ChangeStatusOpen) {
		t.Fatalf("merged should be terminal")
	}
	if ChangeStatusSuperseded.Allows(ChangeStatusOpen) {
		t.Fatalf("superseded should be terminal")
	}
}

func TestChangeReadyStateValidate(t *testing.T) {
	if !ChangeReadyMergeReady.IsValid() {
		t.Fatalf("merge_ready should be valid")
	}
	if ChangeReadyState("bogus").IsValid() {
		t.Fatalf("bogus change readiness should be invalid")
	}
}

func TestPermissionProfileValidateRejectsUnknownAction(t *testing.T) {
	profile := PermissionProfile{
		ProfileID:    "pp-1",
		AllowActions: []PermissionAction{"bogus"},
	}
	if err := profile.Validate(); err == nil {
		t.Fatalf("expected invalid action to fail validation")
	}
}

func TestExportBundleCarriesRedactionPreviewBinding(t *testing.T) {
	bundle := ExportBundle{
		BundleID:           "bundle_1",
		Status:             ExportBundleCreated,
		RedactionPreviewID: "redact_1",
	}
	if err := bundle.Validate(); err != nil {
		t.Fatalf("valid redacted export bundle rejected: %v", err)
	}
	raw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("marshal export bundle: %v", err)
	}
	if !strings.Contains(string(raw), `"redaction_preview_id":"redact_1"`) {
		t.Fatalf("redaction preview binding missing from export bundle JSON: %s", raw)
	}
}
