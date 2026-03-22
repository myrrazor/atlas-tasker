package domain

import (
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func baseTickets() map[string]contracts.TicketSnapshot {
	return map[string]contracts.TicketSnapshot{
		"APP-1": {ID: "APP-1", Project: "APP"},
		"APP-2": {ID: "APP-2", Project: "APP"},
		"APP-3": {ID: "APP-3", Project: "APP"},
	}
}

func TestApplyBlocksLinkSymmetric(t *testing.T) {
	tickets := baseTickets()
	if err := ApplyLink(tickets, "APP-1", "APP-2", LinkBlocks); err != nil {
		t.Fatalf("apply blocks link failed: %v", err)
	}
	if len(tickets["APP-1"].Blocks) != 1 || tickets["APP-1"].Blocks[0] != "APP-2" {
		t.Fatalf("expected APP-1 blocks APP-2, got %#v", tickets["APP-1"].Blocks)
	}
	if len(tickets["APP-2"].BlockedBy) != 1 || tickets["APP-2"].BlockedBy[0] != "APP-1" {
		t.Fatalf("expected APP-2 blocked by APP-1, got %#v", tickets["APP-2"].BlockedBy)
	}
}

func TestApplyParentLinkRejectsCycle(t *testing.T) {
	tickets := baseTickets()
	ticket := tickets["APP-2"]
	ticket.Parent = "APP-1"
	tickets["APP-2"] = ticket

	if err := ApplyLink(tickets, "APP-1", "APP-2", LinkParent); err == nil {
		t.Fatal("expected parent cycle detection error")
	}
}

func TestApplyLinkRejectsSelfLink(t *testing.T) {
	tickets := baseTickets()
	if err := ApplyLink(tickets, "APP-1", "APP-1", LinkBlocks); err == nil {
		t.Fatal("expected self-link rejection")
	}
}

func TestRemoveLinkClearsSymmetry(t *testing.T) {
	tickets := baseTickets()
	if err := ApplyLink(tickets, "APP-1", "APP-2", LinkBlocks); err != nil {
		t.Fatalf("apply blocks link failed: %v", err)
	}
	if err := RemoveLink(tickets, "APP-1", "APP-2"); err != nil {
		t.Fatalf("remove link failed: %v", err)
	}
	if len(tickets["APP-1"].Blocks) != 0 || len(tickets["APP-2"].BlockedBy) != 0 {
		t.Fatalf("expected symmetric links cleared, got %#v %#v", tickets["APP-1"], tickets["APP-2"])
	}
}

func TestApplyLinkTrimsIDs(t *testing.T) {
	tickets := baseTickets()
	if err := ApplyLink(tickets, " APP-1 ", " APP-2 ", LinkBlocks); err != nil {
		t.Fatalf("apply trimmed blocks link failed: %v", err)
	}
	if len(tickets["APP-1"].Blocks) != 1 || tickets["APP-1"].Blocks[0] != "APP-2" {
		t.Fatalf("expected trimmed ids to map correctly, got %#v", tickets["APP-1"].Blocks)
	}
}
