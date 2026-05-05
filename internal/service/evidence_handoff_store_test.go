package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestEvidenceStoreRoundTrip(t *testing.T) {
	root := t.TempDir()
	store := EvidenceStore{Root: root}
	now := time.Date(2026, 3, 25, 19, 0, 0, 0, time.UTC)
	item := contracts.EvidenceItem{
		EvidenceID:           "evidence_123",
		RunID:                "run_123",
		TicketID:             "APP-1",
		Type:                 contracts.EvidenceTypeTestResult,
		Title:                "go test",
		Body:                 "ok",
		SupersedesEvidenceID: "evidence_older",
		Actor:                contracts.Actor("human:owner"),
		CreatedAt:            now,
		SchemaVersion:        contracts.CurrentSchemaVersion,
	}
	if err := store.SaveEvidence(context.Background(), item); err != nil {
		t.Fatalf("save evidence: %v", err)
	}
	loaded, err := store.LoadEvidence(context.Background(), item.EvidenceID)
	if err != nil {
		t.Fatalf("load evidence: %v", err)
	}
	if loaded.EvidenceID != item.EvidenceID || loaded.SupersedesEvidenceID != item.SupersedesEvidenceID || loaded.Body != item.Body {
		t.Fatalf("unexpected evidence: %#v", loaded)
	}
	items, err := store.ListEvidence(context.Background(), item.RunID)
	if err != nil {
		t.Fatalf("list evidence: %v", err)
	}
	if len(items) != 1 || items[0].EvidenceID != item.EvidenceID {
		t.Fatalf("unexpected evidence list: %#v", items)
	}
}

func TestHandoffStoreRoundTripAndMarkdown(t *testing.T) {
	root := t.TempDir()
	store := HandoffStore{Root: root}
	now := time.Date(2026, 3, 25, 20, 0, 0, 0, time.UTC)
	packet := contracts.HandoffPacket{
		HandoffID:                 "handoff_123",
		SourceRunID:               "run_123",
		TicketID:                  "APP-1",
		Actor:                     contracts.Actor("human:owner"),
		StatusSummary:             "ready for review",
		ChangedFiles:              []string{"internal/service/evidence_actions.go"},
		CommitRefs:                []string{"abc123"},
		Tests:                     []string{"go test ./internal/service"},
		EvidenceLinks:             []string{"evidence_123"},
		OpenQuestions:             []string{"Do we need an owner gate?"},
		Risks:                     []string{"migration drift"},
		SuggestedNextActor:        "agent:reviewer-1",
		SuggestedNextGate:         contracts.GateKindReview,
		SuggestedNextTicketStatus: contracts.StatusInReview,
		GeneratedAt:               now,
		SchemaVersion:             contracts.CurrentSchemaVersion,
	}
	if err := store.SaveHandoff(context.Background(), packet); err != nil {
		t.Fatalf("save handoff: %v", err)
	}
	loaded, err := store.LoadHandoff(context.Background(), packet.HandoffID)
	if err != nil {
		t.Fatalf("load handoff: %v", err)
	}
	if loaded.HandoffID != packet.HandoffID || loaded.SuggestedNextActor != packet.SuggestedNextActor {
		t.Fatalf("unexpected handoff: %#v", loaded)
	}
	items, err := store.ListHandoffs(context.Background(), packet.TicketID)
	if err != nil {
		t.Fatalf("list handoffs: %v", err)
	}
	if len(items) != 1 || items[0].HandoffID != packet.HandoffID {
		t.Fatalf("unexpected handoff list: %#v", items)
	}
	markdown := RenderHandoffMarkdown(packet)
	if !strings.Contains(markdown, "## Changed Files") || !strings.Contains(markdown, "## Next") || !strings.Contains(markdown, "migration drift") {
		t.Fatalf("unexpected handoff markdown: %s", markdown)
	}
}
