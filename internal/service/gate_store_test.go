package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestGateStoreRoundTrip(t *testing.T) {
	root := t.TempDir()
	store := GateStore{Root: root}
	now := time.Date(2026, 3, 25, 21, 0, 0, 0, time.UTC)
	gate := contracts.GateSnapshot{
		GateID:          "gate_123",
		TicketID:        "APP-1",
		RunID:           "run_123",
		Kind:            contracts.GateKindReview,
		State:           contracts.GateStateOpen,
		RequiredRole:    contracts.AgentRoleReviewer,
		RequiredAgentID: "agent:reviewer-1",
		CreatedBy:       contracts.Actor("human:owner"),
		RelatedRunIDs:   []string{"run_123"},
		CreatedAt:       now,
		SchemaVersion:   contracts.CurrentSchemaVersion,
	}
	if err := store.SaveGate(context.Background(), gate); err != nil {
		t.Fatalf("save gate: %v", err)
	}
	loaded, err := store.LoadGate(context.Background(), gate.GateID)
	if err != nil {
		t.Fatalf("load gate: %v", err)
	}
	if loaded.GateID != gate.GateID || loaded.RequiredAgentID != gate.RequiredAgentID {
		t.Fatalf("unexpected gate: %#v", loaded)
	}
	items, err := store.ListGates(context.Background(), gate.TicketID)
	if err != nil {
		t.Fatalf("list gates: %v", err)
	}
	if len(items) != 1 || items[0].GateID != gate.GateID {
		t.Fatalf("unexpected gate list: %#v", items)
	}
	markdown := RenderGateMarkdown(gate)
	if !strings.Contains(markdown, "## Related Runs") || !strings.Contains(markdown, "review") {
		t.Fatalf("unexpected gate markdown: %s", markdown)
	}
}
