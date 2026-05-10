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

	ticket.Type = TicketTypeTask
	ticket.SchemaVersion = 99
	if err := ticket.ValidateForCreate(); err == nil {
		t.Fatal("expected error for invalid schema version")
	}
}

func TestPathDerivedIdentifiersRejectTraversal(t *testing.T) {
	for _, key := range []string{"APP", "API_2", "OPS-TOOLS"} {
		if !IsValidProjectKey(key) {
			t.Fatalf("expected project key %q to be valid", key)
		}
	}
	for _, key := range []string{"", "../../etc", "../EVIL", "app", "APP SPACE", "APP/WEB", "~APP"} {
		if IsValidProjectKey(key) {
			t.Fatalf("expected project key %q to be invalid", key)
		}
	}
	for _, id := range []string{"APP-1", "API_2-42", "OPS-TOOLS-7", "APP_secret"} {
		if !IsValidTicketID(id) {
			t.Fatalf("expected ticket id %q to be valid", id)
		}
	}
	for _, id := range []string{"", "../APP-1", "APP/1", "APP.1", "1-APP", "APP-1\nBAD"} {
		if IsValidTicketID(id) {
			t.Fatalf("expected ticket id %q to be invalid", id)
		}
	}
}

func TestTrackerConfigValidate(t *testing.T) {
	cfg := TrackerConfig{
		Workflow: WorkflowConfig{
			CompletionMode: CompletionModeOwnerGate,
		},
		Notifications: NotificationsConfig{
			DeliveryLogPath: ".tracker/delivery.log",
			DeadLetterPath:  ".tracker/dead.log",
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}

	cfg.Workflow.CompletionMode = CompletionMode("invalid")
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid completion mode error")
	}
}

func TestNormalizeProjectPreservesLegacyCompletionFallback(t *testing.T) {
	legacy := NormalizeProject(Project{
		Key:           "APP",
		Name:          "App",
		SchemaVersion: SchemaVersionV1,
	})
	if legacy.Defaults.CompletionMode != "" {
		t.Fatalf("expected legacy project completion mode to stay empty, got %s", legacy.Defaults.CompletionMode)
	}
	if legacy.SchemaVersion != SchemaVersionV1 {
		t.Fatalf("expected legacy schema to remain v1 in-memory until write, got %d", legacy.SchemaVersion)
	}

	fresh := NormalizeProject(Project{Key: "NEW", Name: "New"})
	if fresh.Defaults.CompletionMode != CompletionModeOpen {
		t.Fatalf("expected fresh project completion mode to default to open, got %s", fresh.Defaults.CompletionMode)
	}
	if fresh.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("expected fresh project schema to default to current, got %d", fresh.SchemaVersion)
	}
}
