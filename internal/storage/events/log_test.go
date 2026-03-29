package events

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestAppendAndStreamEvents(t *testing.T) {
	root := t.TempDir()
	log := &Log{RootDir: root}

	e1 := contracts.Event{
		EventID:       1,
		Timestamp:     time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC),
		Actor:         contracts.Actor("human:owner"),
		Type:          contracts.EventTicketCreated,
		Project:       "APP",
		TicketID:      "APP-1",
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	e2 := contracts.Event{
		EventID:       2,
		Timestamp:     time.Date(2026, 3, 22, 12, 1, 0, 0, time.UTC),
		Actor:         contracts.Actor("agent:builder-1"),
		Type:          contracts.EventTicketUpdated,
		Project:       "APP",
		TicketID:      "APP-1",
		SchemaVersion: contracts.CurrentSchemaVersion,
	}

	if err := log.AppendEvent(context.Background(), e1); err != nil {
		t.Fatalf("append event 1 failed: %v", err)
	}
	if err := log.AppendEvent(context.Background(), e2); err != nil {
		t.Fatalf("append event 2 failed: %v", err)
	}

	events, err := log.StreamEvents(context.Background(), "APP", 0)
	if err != nil {
		t.Fatalf("stream events failed: %v", err)
	}
	if len(events) != 2 || events[0].EventID != 1 || events[1].EventID != 2 {
		t.Fatalf("unexpected events: %#v", events)
	}

	after, err := log.StreamEvents(context.Background(), "APP", 1)
	if err != nil {
		t.Fatalf("stream after id failed: %v", err)
	}
	if len(after) != 1 || after[0].EventID != 2 {
		t.Fatalf("unexpected after results: %#v", after)
	}
}

func TestAppendRejectsNonMonotonicEventID(t *testing.T) {
	root := t.TempDir()
	log := &Log{RootDir: root}

	base := contracts.Event{
		EventID:       5,
		Timestamp:     time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC),
		Actor:         contracts.Actor("human:owner"),
		Type:          contracts.EventTicketCreated,
		Project:       "APP",
		TicketID:      "APP-1",
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := log.AppendEvent(context.Background(), base); err != nil {
		t.Fatalf("append base event failed: %v", err)
	}

	nonMonotonic := base
	nonMonotonic.EventID = 5
	nonMonotonic.Timestamp = nonMonotonic.Timestamp.Add(time.Minute)
	if err := log.AppendEvent(context.Background(), nonMonotonic); err == nil {
		t.Fatal("expected non-monotonic event_id append to fail")
	}
}

func TestAppendRejectsCrossMonthNonMonotonicEventID(t *testing.T) {
	root := t.TempDir()
	log := &Log{RootDir: root}

	march := contracts.Event{
		EventID:       50,
		Timestamp:     time.Date(2026, 3, 31, 23, 59, 0, 0, time.UTC),
		Actor:         contracts.Actor("human:owner"),
		Type:          contracts.EventTicketCreated,
		Project:       "APP",
		TicketID:      "APP-1",
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := log.AppendEvent(context.Background(), march); err != nil {
		t.Fatalf("append march event failed: %v", err)
	}

	april := march
	april.EventID = 1
	april.Timestamp = time.Date(2026, 4, 1, 0, 1, 0, 0, time.UTC)
	if err := log.AppendEvent(context.Background(), april); err == nil {
		t.Fatal("expected cross-month non-monotonic event_id append to fail")
	}
}

func TestStreamEventsDetectsCorruptedLine(t *testing.T) {
	root := t.TempDir()
	eventsDir := storage.EventsDir(root)
	if err := os.MkdirAll(eventsDir, 0o755); err != nil {
		t.Fatalf("mkdir events dir failed: %v", err)
	}
	path := filepath.Join(eventsDir, "2026-03.jsonl")
	if err := os.WriteFile(path, []byte("{bad json}\n"), 0o644); err != nil {
		t.Fatalf("write bad event file failed: %v", err)
	}
	log := &Log{RootDir: root}
	if _, err := log.StreamEvents(context.Background(), "APP", 0); err == nil {
		t.Fatal("expected corruption error")
	}
}

func TestStreamEventsUsesLamportOrdering(t *testing.T) {
	root := t.TempDir()
	log := &Log{RootDir: root}

	laterIDLowerClock := contracts.NormalizeEvent(contracts.Event{
		EventID:           10,
		EventUID:          "evt-a",
		Timestamp:         time.Date(2026, 3, 22, 12, 5, 0, 0, time.UTC),
		OriginWorkspaceID: "ws-a",
		LogicalClock:      2,
		Actor:             contracts.Actor("human:owner"),
		Type:              contracts.EventTicketUpdated,
		Project:           "APP",
		TicketID:          "APP-1",
		SchemaVersion:     contracts.CurrentSchemaVersion,
	})
	earlierIDHigherClock := contracts.NormalizeEvent(contracts.Event{
		EventID:           1,
		EventUID:          "evt-b",
		Timestamp:         time.Date(2026, 3, 22, 12, 1, 0, 0, time.UTC),
		OriginWorkspaceID: "ws-b",
		LogicalClock:      1,
		Actor:             contracts.Actor("human:owner"),
		Type:              contracts.EventTicketCreated,
		Project:           "APP",
		TicketID:          "APP-1",
		SchemaVersion:     contracts.CurrentSchemaVersion,
	})

	if err := os.MkdirAll(storage.EventsDir(root), 0o755); err != nil {
		t.Fatalf("mkdir events dir failed: %v", err)
	}
	path := filepath.Join(storage.EventsDir(root), "2026-03.jsonl")
	rawA, _ := json.Marshal(laterIDLowerClock)
	rawB, _ := json.Marshal(earlierIDHigherClock)
	if err := os.WriteFile(path, append(append(rawA, '\n'), append(rawB, '\n')...), 0o644); err != nil {
		t.Fatalf("write event file failed: %v", err)
	}

	events, err := log.StreamEvents(context.Background(), "APP", 0)
	if err != nil {
		t.Fatalf("stream events failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected two events, got %#v", events)
	}
	if events[0].EventUID != "evt-b" || events[1].EventUID != "evt-a" {
		t.Fatalf("expected logical clock ordering, got %#v", events)
	}
}
