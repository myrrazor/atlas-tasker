package domain

import (
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestAllowedTransitions(t *testing.T) {
	tests := []struct {
		from contracts.Status
		to   contracts.Status
		ok   bool
	}{
		{contracts.StatusBacklog, contracts.StatusReady, true},
		{contracts.StatusBacklog, contracts.StatusInProgress, false},
		{contracts.StatusInProgress, contracts.StatusInReview, true},
		{contracts.StatusInReview, contracts.StatusDone, true},
		{contracts.StatusDone, contracts.StatusReady, false},
	}

	for _, tc := range tests {
		err := ValidateTransition(tc.from, tc.to)
		if tc.ok && err != nil {
			t.Fatalf("expected transition %s -> %s to succeed, got %v", tc.from, tc.to, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("expected transition %s -> %s to fail", tc.from, tc.to)
		}
	}
}

func TestCompletionPermissions(t *testing.T) {
	reviewer := contracts.Actor("agent:reviewer-1")

	if err := ValidateMove(contracts.CompletionModeOpen, contracts.StatusInReview, contracts.StatusDone, contracts.Actor("agent:builder-1"), reviewer); err != nil {
		t.Fatalf("open mode should allow completion: %v", err)
	}

	if err := ValidateMove(contracts.CompletionModeOwnerGate, contracts.StatusInReview, contracts.StatusDone, contracts.Actor("agent:builder-1"), reviewer); err == nil {
		t.Fatal("owner_gate should reject non-owner completion")
	}

	if err := ValidateMove(contracts.CompletionModeOwnerGate, contracts.StatusInReview, contracts.StatusDone, contracts.Actor("human:owner"), reviewer); err != nil {
		t.Fatalf("owner_gate should allow owner completion: %v", err)
	}

	if err := ValidateMove(contracts.CompletionModeReviewGate, contracts.StatusInReview, contracts.StatusDone, reviewer, reviewer); err != nil {
		t.Fatalf("review_gate should allow reviewer completion: %v", err)
	}

	if err := ValidateMove(contracts.CompletionModeReviewGate, contracts.StatusInReview, contracts.StatusDone, contracts.Actor("agent:builder-1"), reviewer); err == nil {
		t.Fatal("review_gate should reject non-reviewer completion")
	}
}

func TestValidateMoveSkipsCompletionRuleOutsideReviewToDone(t *testing.T) {
	err := ValidateMove(
		contracts.CompletionModeOwnerGate,
		contracts.StatusReady,
		contracts.StatusInProgress,
		contracts.Actor("agent:builder-1"),
		contracts.Actor("agent:reviewer-1"),
	)
	if err != nil {
		t.Fatalf("expected non-completion move to pass permission checks: %v", err)
	}
}
