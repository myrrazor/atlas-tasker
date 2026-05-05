package service

import (
	"context"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestChangeAndCheckStoreRoundTrip(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)

	changeStore := ChangeStore{Root: root}
	checkStore := CheckStore{Root: root}

	change := contracts.ChangeRef{
		ChangeID:      "change_123",
		Provider:      contracts.ChangeProviderLocal,
		TicketID:      "APP-1",
		RunID:         "run_123",
		BranchName:    "feat/app-1",
		Status:        contracts.ChangeStatusApproved,
		ChecksStatus:  contracts.CheckAggregatePassing,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := changeStore.SaveChange(ctx, change); err != nil {
		t.Fatalf("save change: %v", err)
	}
	loadedChange, err := changeStore.LoadChange(ctx, change.ChangeID)
	if err != nil {
		t.Fatalf("load change: %v", err)
	}
	if loadedChange.ChangeID != change.ChangeID || loadedChange.BranchName != change.BranchName {
		t.Fatalf("unexpected loaded change: %#v", loadedChange)
	}
	changes, err := changeStore.ListChanges(ctx, "APP-1")
	if err != nil {
		t.Fatalf("list changes: %v", err)
	}
	if len(changes) != 1 || changes[0].ChangeID != change.ChangeID {
		t.Fatalf("unexpected change list: %#v", changes)
	}

	check := contracts.CheckResult{
		CheckID:       "check_123",
		Source:        contracts.CheckSourceManual,
		Scope:         contracts.CheckScopeChange,
		ScopeID:       change.ChangeID,
		Name:          "unit",
		Status:        contracts.CheckStatusCompleted,
		Conclusion:    contracts.CheckConclusionSuccess,
		StartedAt:     now,
		CompletedAt:   now.Add(time.Minute),
		UpdatedAt:     now.Add(time.Minute),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := checkStore.SaveCheck(ctx, check); err != nil {
		t.Fatalf("save check: %v", err)
	}
	loadedCheck, err := checkStore.LoadCheck(ctx, check.CheckID)
	if err != nil {
		t.Fatalf("load check: %v", err)
	}
	if loadedCheck.CheckID != check.CheckID || loadedCheck.ScopeID != check.ScopeID {
		t.Fatalf("unexpected loaded check: %#v", loadedCheck)
	}
	checks, err := checkStore.ListChecks(ctx, contracts.CheckScopeChange, change.ChangeID)
	if err != nil {
		t.Fatalf("list checks: %v", err)
	}
	if len(checks) != 1 || checks[0].CheckID != check.CheckID {
		t.Fatalf("unexpected check list: %#v", checks)
	}
}
