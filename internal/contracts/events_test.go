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

func TestNormalizeEventBackfillsStableLegacyEventUIDAfterSchemaHydration(t *testing.T) {
	event := NormalizeEvent(Event{
		EventID:   7,
		Timestamp: time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC),
		Actor:     Actor("human:owner"),
		Reason:    "seed legacy event",
		Type:      EventTicketCreated,
		Project:   "APP",
		TicketID:  "APP-1",
		Payload:   map[string]any{"title": "Seed"},
	})
	if event.SchemaVersion != SchemaVersionV1 {
		t.Fatalf("expected legacy schema to hydrate to v1, got %d", event.SchemaVersion)
	}
	if event.EventUID == "" {
		t.Fatal("expected normalize event to backfill event uid")
	}
	if event.EventUID != LegacyEventUID(event) {
		t.Fatalf("expected normalized event uid to match canonical legacy digest, got %q want %q", event.EventUID, LegacyEventUID(event))
	}
}
