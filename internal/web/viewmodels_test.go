package web

import (
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestFirstTicketIDForColumnPrefersActiveColumn(t *testing.T) {
	columns := []BoardColumn{
		{
			Status:  contracts.StatusBacklog,
			Tickets: []TicketCard{{Ticket: contracts.TicketSnapshot{ID: "WEB-1"}}},
		},
		{
			Status:  contracts.StatusReady,
			Tickets: []TicketCard{{Ticket: contracts.TicketSnapshot{ID: "WEB-2"}}},
		},
	}
	if got := firstTicketIDForColumn(columns, contracts.StatusReady); got != "WEB-2" {
		t.Fatalf("expected active column ticket WEB-2, got %q", got)
	}
	if got := firstTicketIDForColumn(columns, contracts.StatusInReview); got != "WEB-1" {
		t.Fatalf("expected fallback ticket WEB-1, got %q", got)
	}
}
