package service

import (
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

// Rough edge 19: remote restore-plan prints a plan ID, and restore-apply's
// usage is <PLAN-ID|CHECKPOINT>, but applying by that plan ID failed with
// "remote checkpoint not found" (exit 3).
func TestRemoteRestoreApplyAcceptsPlanIDFromRestorePlan(t *testing.T) {
	ctx, source := newCheckpointHarness(t)
	remote := initBareRemote(t)
	url := "file://" + remote
	if _, err := addDisposableTarget(t, source, url, "drill"); err != nil {
		t.Fatal(err)
	}
	if _, err := source.EnableAutoBackup(ctx, "drill"); err != nil {
		t.Fatal(err)
	}
	published, err := source.BackupTick(ctx, true)
	if err != nil || published.State != contracts.BackupOutboxVerified {
		t.Fatalf("checkpoint+publish: %#v %v", published, err)
	}
	dest := newEmptyDest(t)
	if _, err := addDisposableTarget(t, dest, url, "drill"); err != nil {
		t.Fatal(err)
	}
	plan, err := dest.RemoteRestorePlan(ctx, RemoteRestoreOptions{
		TargetID: "drill", Checkpoint: published.CheckpointID, AllowWorkspaceMismatch: true,
		Actor: contracts.Actor("human:owner"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.PlanID == "" {
		t.Fatal("restore-plan should return a plan id")
	}
	result, err := dest.RemoteRestoreApply(ctx, RemoteRestoreOptions{
		TargetID: "drill", Checkpoint: plan.PlanID, AllowWorkspaceMismatch: true,
		Yes: true, Actor: contracts.Actor("human:owner"), Reason: "edge 19 regression",
	})
	if err != nil {
		t.Fatalf("restore-apply by the plan id restore-plan printed: %v", err)
	}
	if result.Plan.RestorePlanID != plan.PlanID {
		t.Fatalf("apply should use the named plan %s, got %s", plan.PlanID, result.Plan.RestorePlanID)
	}
	if _, err := dest.Tickets.GetTicket(ctx, "APP-1"); err != nil {
		t.Fatalf("restored ticket should exist: %v", err)
	}
}

func TestRemoteRestoreApplyRejectsLocalPlanIDClearly(t *testing.T) {
	ctx, source := newCheckpointHarness(t)
	remote := initBareRemote(t)
	if _, err := addDisposableTarget(t, source, "file://"+remote, "drill"); err != nil {
		t.Fatal(err)
	}
	local := contracts.RestorePlan{RestorePlanID: "restore-localonly", BackupID: "backup-x", GeneratedBy: contracts.Actor("human:owner"), GeneratedAt: source.now()}
	if err := source.RestorePlans.SaveRestorePlan(ctx, local); err != nil {
		t.Fatal(err)
	}
	_, err := source.RemoteRestoreApply(ctx, RemoteRestoreOptions{
		TargetID: "drill", Checkpoint: "restore-localonly", Yes: true,
		Actor: contracts.Actor("human:owner"), Reason: "edge 19",
	})
	if err == nil || !strings.Contains(err.Error(), "backup restore-apply restore-localonly") {
		t.Fatalf("a local plan id should get a pointer to local restore-apply, got %v", err)
	}
}
