package render

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestTicketsPrettyIncludesTicketID(t *testing.T) {
	out := TicketsPretty("Backlog", []contracts.TicketSnapshot{{ID: "APP-1", Status: contracts.StatusBacklog, Priority: contracts.PriorityHigh, Title: "Task"}})
	if !strings.Contains(out, "APP-1") {
		t.Fatalf("expected ticket id in pretty output, got: %s", out)
	}
	if !strings.Contains(out, "[backlog]") || !strings.Contains(out, "[high]") {
		t.Fatalf("expected semantic badges in pretty output, got: %s", out)
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
	if !strings.Contains(out, "(empty)") {
		t.Fatalf("expected empty marker in empty state, got: %s", out)
	}
}

func TestTicketSummaryTruncatesByDisplayWidth(t *testing.T) {
	ticket := contracts.TicketSnapshot{ID: "APP-1", Status: contracts.StatusReady, Priority: contracts.PriorityHigh, Title: "Fix wide 表表表 title"}
	out := TicketSummary(ticket, 28)
	if got := visibleWidth(out); got > 28 {
		t.Fatalf("expected summary to fit width, got width=%d out=%q", got, out)
	}
	if !strings.HasSuffix(out, "...") {
		t.Fatalf("expected truncated summary suffix, got %q", out)
	}
}

func TestBoardPrettyWithWidthNarrowGolden(t *testing.T) {
	board := contracts.BoardView{Columns: map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusReady: {{ID: "APP-1", Status: contracts.StatusReady, Priority: contracts.PriorityHigh, Title: "A very long board title", UpdatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}},
	}}
	out := BoardPrettyWithWidth(board, 32)
	for _, line := range strings.Split(out, "\n") {
		if visibleWidth(line) > 32 {
			t.Fatalf("expected board item line to fit width, got width=%d line=%q", visibleWidth(line), line)
		}
	}
	if !strings.Contains(out, "APP-1 [ready] [high] A ve...") {
		t.Fatalf("unexpected narrow board output:\n%s", out)
	}
}

func TestBoardPrettyWithTinyWidthKeepsHeadersInsideWidth(t *testing.T) {
	board := contracts.BoardView{Columns: map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusReady: {{ID: "APP-1", Status: contracts.StatusReady, Priority: contracts.PriorityHigh, Title: "Tiny terminal", UpdatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}},
	}}
	out := BoardPrettyWithWidth(board, 8)
	for _, line := range strings.Split(out, "\n") {
		if visibleWidth(line) > 8 {
			t.Fatalf("expected every board line to fit width, got width=%d line=%q", visibleWidth(line), line)
		}
	}
}

func TestTerminalWidthUsesColumnsWhenStdoutIsNotTTY(t *testing.T) {
	t.Setenv("COLUMNS", "41")
	if got := TerminalWidth(100); got != 41 {
		t.Fatalf("expected COLUMNS fallback width, got %d", got)
	}
}

func TestSemanticBadgesAreASCIIWhenNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	badges := []string{
		StatusBadge(contracts.StatusReady),
		GateBadge(contracts.GateStateOpen),
		SignatureBadge(contracts.VerificationTrustedValid),
		SyncBadge(contracts.SyncJobStateFailed),
	}
	for _, badge := range badges {
		if strings.Contains(badge, "\x1b[") {
			t.Fatalf("badge should not contain ANSI when NO_COLOR is set: %q", badge)
		}
		if strings.ContainsAny(badge, "✓●→") {
			t.Fatalf("badge should remain ASCII-stable: %q", badge)
		}
	}
}

func TestMarkdownWithWidthAcceptsNarrowWidth(t *testing.T) {
	out := MarkdownWithWidth("# Title\n\nSome paragraph text.", 12)
	if strings.TrimSpace(out) == "" {
		t.Fatal("expected markdown renderer to return content for narrow width")
	}
}

func visibleWidth(value string) int {
	return lipgloss.Width(value)
}
