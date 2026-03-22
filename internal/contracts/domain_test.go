package contracts

import (
	"testing"
	"time"
)

func TestActorValidation(t *testing.T) {
	valid := []Actor{"human:owner", "agent:builder-1"}
	for _, actor := range valid {
		if !actor.IsValid() {
			t.Fatalf("expected actor %q to be valid", actor)
		}
	}

	invalid := []Actor{"", "human", "robot:alice", "agent:"}
	for _, actor := range invalid {
		if actor.IsValid() {
			t.Fatalf("expected actor %q to be invalid", actor)
		}
	}
}

func TestTicketSnapshotValidateForCreate(t *testing.T) {
	now := time.Now().UTC()
	ticket := TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Wire contracts",
		Type:          TicketTypeTask,
		Status:        StatusBacklog,
		Priority:      PriorityMedium,
		Assignee:      Actor("agent:builder-1"),
		Reviewer:      Actor("human:owner"),
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: CurrentSchemaVersion,
	}

	if err := ticket.ValidateForCreate(); err != nil {
		t.Fatalf("expected valid ticket, got error: %v", err)
	}

	ticket.Type = TicketType("wrong")
	if err := ticket.ValidateForCreate(); err == nil {
		t.Fatal("expected error for invalid type")
	}
}
