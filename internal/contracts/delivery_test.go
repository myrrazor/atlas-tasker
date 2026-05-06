package contracts

import "testing"

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
