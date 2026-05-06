package contracts

import (
	"testing"
	"time"
)

func TestLegacyEntityUIDsAreDeterministic(t *testing.T) {
	if got := TicketUID("APP", "APP-1"); got == "" || got != TicketUID("APP", "APP-1") {
		t.Fatalf("ticket uid should be deterministic, got %q", got)
	}
	if got := RunUID("run_123"); got == "" || got != RunUID("run_123") {
		t.Fatalf("run uid should be deterministic, got %q", got)
	}
	if got := GateUID("gate_123"); got == "" || got != GateUID("gate_123") {
		t.Fatalf("gate uid should be deterministic, got %q", got)
	}
	if got := HandoffUID("handoff_123"); got == "" || got != HandoffUID("handoff_123") {
		t.Fatalf("handoff uid should be deterministic, got %q", got)
	}
	if got := EvidenceUID("run_123", "evidence_1"); got == "" || got != EvidenceUID("run_123", "evidence_1") {
		t.Fatalf("evidence uid should be deterministic, got %q", got)
	}
	if got := ChangeUID("change_123"); got == "" || got != ChangeUID("change_123") {
		t.Fatalf("change uid should be deterministic, got %q", got)
	}
	if got := CheckUID("check_123"); got == "" || got != CheckUID("check_123") {
		t.Fatalf("check uid should be deterministic, got %q", got)
	}
	if got := MembershipUID("alice", MembershipScopeProject, "APP", MembershipRoleReviewer); got == "" || got != MembershipUID("alice", MembershipScopeProject, "APP", MembershipRoleReviewer) {
		t.Fatalf("membership uid should be deterministic, got %q", got)
	}
	if got := ImportJobUID("import_123"); got == "" || got != ImportJobUID("import_123") {
		t.Fatalf("import job uid should be deterministic, got %q", got)
	}
	if got := ExportBundleUID("bundle_123"); got == "" || got != ExportBundleUID("bundle_123") {
		t.Fatalf("export bundle uid should be deterministic, got %q", got)
	}
	if got := ArchiveRecordUID("archive_123"); got == "" || got != ArchiveRecordUID("archive_123") {
		t.Fatalf("archive record uid should be deterministic, got %q", got)
	}
}

func TestLegacyEventUIDUsesCanonicalDigest(t *testing.T) {
	now := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)
	event := Event{
		EventID:       7,
		Timestamp:     now,
		Actor:         Actor("human:owner"),
		Reason:        "seed",
		Type:          EventTicketCreated,
		Project:       "APP",
		TicketID:      "APP-1",
		Payload:       map[string]any{"title": "Seed"},
		SchemaVersion: SchemaVersionV4,
	}
	uidA := LegacyEventUID(event)
	uidB := LegacyEventUID(event)
	if uidA == "" || uidA != uidB {
		t.Fatalf("expected stable legacy event uid, got %q and %q", uidA, uidB)
	}

	mutated := event
	mutated.Reason = "changed"
	if uidA == LegacyEventUID(mutated) {
		t.Fatalf("expected legacy event uid to change when canonical event changes")
	}
}

func TestCanonicalEventUIDUsesWorkspaceAwareFormulaForNativeEvents(t *testing.T) {
	now := time.Date(2026, 3, 29, 13, 0, 0, 0, time.UTC)
	event := Event{
		EventID:           11,
		Timestamp:         now,
		OriginWorkspaceID: "workspace-a",
		LogicalClock:      42,
		Actor:             Actor("human:owner"),
		Type:              EventTicketCreated,
		Project:           "APP",
		TicketID:          "APP-1",
		SchemaVersion:     CurrentSchemaVersion,
	}
	uidA := CanonicalEventUID(event)
	uidB := CanonicalEventUID(event)
	if uidA == "" || uidA != uidB {
		t.Fatalf("expected stable native event uid, got %q and %q", uidA, uidB)
	}

	mutated := event
	mutated.LogicalClock++
	if uidA == CanonicalEventUID(mutated) {
		t.Fatal("expected native event uid to change when logical clock changes")
	}
}

func TestNormalizeTicketSnapshotBackfillsTicketUID(t *testing.T) {
	ticket := NormalizeTicketSnapshot(TicketSnapshot{ID: "APP-1", Project: "APP", Title: "Seed", Type: TicketTypeTask, Status: StatusBacklog, Priority: PriorityMedium})
	if ticket.TicketUID == "" {
		t.Fatalf("expected ticket uid to be backfilled")
	}
}
