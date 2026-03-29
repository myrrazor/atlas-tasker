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
	if got := MembershipUID("alice", MembershipScopeProject, "APP", MembershipRoleReviewer); got == "" || got != MembershipUID("alice", MembershipScopeProject, "APP", MembershipRoleReviewer) {
		t.Fatalf("membership uid should be deterministic, got %q", got)
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

func TestNormalizeTicketSnapshotBackfillsTicketUID(t *testing.T) {
	ticket := NormalizeTicketSnapshot(TicketSnapshot{ID: "APP-1", Project: "APP", Title: "Seed", Type: TicketTypeTask, Status: StatusBacklog, Priority: PriorityMedium})
	if ticket.TicketUID == "" {
		t.Fatalf("expected ticket uid to be backfilled")
	}
}
