package render

import (
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestTicketsPrettyIncludesTicketID(t *testing.T) {
	out := TicketsPretty("Backlog", []contracts.TicketSnapshot{{ID: "APP-1", Status: contracts.StatusBacklog, Priority: contracts.PriorityHigh, Title: "Task"}})
	if !strings.Contains(out, "APP-1") {
		t.Fatalf("expected ticket id in pretty output, got: %s", out)
	}
}

func TestBoardPrettyIncludesColumnLabels(t *testing.T) {
	board := contracts.BoardView{Columns: map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusReady: {{ID: "APP-1", Title: "Task", UpdatedAt: time.Now()}},
	}}
	out := BoardPretty(board)
	if !strings.Contains(out, "Ready") {
		t.Fatalf("expected Ready column in board output, got: %s", out)
	}
}

func TestEmptyStateContainsAction(t *testing.T) {
	out := EmptyState("No results", "Try `tracker ticket create`")
	if !strings.Contains(out, "tracker ticket create") {
		t.Fatalf("expected actionable guidance in empty state, got: %s", out)
	}
}
