package contracts

import (
	"testing"
	"time"
)

func TestAutomationRuleValidate(t *testing.T) {
	rule := AutomationRule{
		Name:    "auto-review",
		Enabled: true,
		Trigger: AutomationTrigger{EventTypes: []EventType{EventTicketMoved}},
		Conditions: AutomationCondition{
			Status:      StatusInReview,
			Type:        TicketTypeTask,
			Assignee:    Actor("agent:builder-1"),
			Reviewer:    Actor("agent:reviewer-1"),
			ReviewState: ReviewStatePending,
			Labels:      []string{"backend"},
		},
		Actions: []AutomationAction{
			{Kind: AutomationActionRequestReview},
			{Kind: AutomationActionNotify, Message: "review requested"},
		},
	}
	if err := rule.Validate(); err != nil {
		t.Fatalf("expected valid automation rule, got %v", err)
	}
}

func TestAutomationRuleValidateRejectsInvalidAction(t *testing.T) {
	rule := AutomationRule{
		Name:    "broken",
		Enabled: true,
		Trigger: AutomationTrigger{EventTypes: []EventType{EventTicketMoved}},
		Actions: []AutomationAction{{Kind: AutomationActionMove, Status: "wat"}},
	}
	if err := rule.Validate(); err == nil {
		t.Fatal("expected invalid move action to fail validation")
	}
}

func TestEventValidateRejectsInvalidSurface(t *testing.T) {
	event := Event{
		EventID:       1,
		Timestamp:     mustTime(t, "2026-03-23T12:00:00Z"),
		Actor:         Actor("human:owner"),
		Type:          EventTicketCreated,
		Project:       "APP",
		SchemaVersion: CurrentSchemaVersion,
		Metadata: EventMetadata{
			Surface: EventSurface("sideways"),
		},
	}
	if err := event.Validate(); err == nil {
		t.Fatal("expected invalid metadata surface to fail validation")
	}
}

func mustTime(t *testing.T, raw string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("parse time %s: %v", raw, err)
	}
	return parsed
}
