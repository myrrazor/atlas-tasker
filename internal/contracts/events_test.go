package contracts

import (
	"testing"
	"time"
)

func TestEventValidate(t *testing.T) {
	event := Event{
		EventID:       1,
		Timestamp:     time.Now().UTC(),
		Actor:         Actor("agent:builder-1"),
		Type:          EventTicketCreated,
		Project:       "APP",
		SchemaVersion: CurrentSchemaVersion,
	}
	if err := event.Validate(); err != nil {
		t.Fatalf("expected valid event, got %v", err)
	}
}

func TestEventValidateRejectsMissingActor(t *testing.T) {
	event := Event{
		EventID:       1,
		Timestamp:     time.Now().UTC(),
		Actor:         Actor(""),
		Type:          EventTicketCreated,
		Project:       "APP",
		SchemaVersion: CurrentSchemaVersion,
	}
	if err := event.Validate(); err == nil {
		t.Fatal("expected invalid actor error")
	}
}
