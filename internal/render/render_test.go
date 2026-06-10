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

func TestRenderTableBorderModes(t *testing.T) {
	unicode := RenderTable([]string{"ID", "Title"}, [][]string{{"APP-1", "Task"}}, TableOptions{
		Title:  "Tickets",
		Width:  40,
		Border: TableBorderUnicode,
	})
	if !strings.Contains(unicode, "╭") || !strings.Contains(unicode, "│") {
		t.Fatalf("expected unicode border, got:\n%s", unicode)
	}
	ascii := RenderTable([]string{"ID", "Title"}, [][]string{{"APP-1", "Task"}}, TableOptions{
		Title:  "Tickets",
		Width:  40,
		Border: TableBorderASCII,
	})
	if !strings.Contains(ascii, "+") || strings.Contains(ascii, "╭") {
		t.Fatalf("expected ascii border, got:\n%s", ascii)
	}
}

func TestRenderTableAutoUsesASCIINoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	out := RenderTable([]string{"ID", "Title"}, [][]string{{"APP-1", "Task"}}, TableOptions{
		Title: "Tickets",
		Width: 40,
	})
	if !strings.Contains(out, "+") || strings.Contains(out, "╭") {
		t.Fatalf("expected auto table to use ASCII without color, got:\n%s", out)
	}
}

func TestRenderTableSanitizesAndFitsWideRows(t *testing.T) {
	out := RenderTable([]string{"ID", "Title"}, [][]string{{"APP-1", "wide 表表\x1b[2J title"}}, TableOptions{
		Title:  "Tickets",
		Width:  34,
		Border: TableBorderASCII,
	})
	if strings.ContainsRune(out, 0x1b) {
		t.Fatalf("expected table cells to scrub escapes, got %q", out)
	}
	if !strings.Contains(out, "wide") {
		t.Fatalf("expected readable sanitized content, got:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if visibleWidth(line) > 34 {
			t.Fatalf("expected table line to fit width, got width=%d line=%q", visibleWidth(line), line)
		}
	}
}

func TestRenderTableKeepsSGRStyledCells(t *testing.T) {
	// our own badge styling must survive into cells; everything else stays banned
	styled := "\x1b[38;5;203m[critical]\x1b[0m"
	hostile := "title\x1b[2Jwith\x1b]0;evil\x07noise"
	out := RenderTable([]string{"Priority", "Title"}, [][]string{{styled, hostile}}, TableOptions{
		Title:  "Tickets",
		Width:  60,
		Border: TableBorderUnicode,
	})
	if !strings.Contains(out, "\x1b[38;5;203m") {
		t.Fatalf("expected SGR-styled cell to survive, got %q", out)
	}
	if strings.Contains(out, "\x1b[2J") || strings.Contains(out, "\x1b]0;") {
		t.Fatalf("expected non-SGR escapes stripped from cells, got %q", out)
	}
	if !strings.Contains(out, "[critical]") {
		t.Fatalf("expected readable cell text to remain, got:\n%s", out)
	}
}

func TestRenderTableHugsContentInsteadOfStretching(t *testing.T) {
	out := RenderTable([]string{"ID", "Title"}, [][]string{{"APP-1", "Tiny"}}, TableOptions{
		Title:  "Tickets",
		Width:  120,
		Border: TableBorderASCII,
	})
	widest := 0
	for _, line := range strings.Split(out, "\n") {
		if w := visibleWidth(line); w > widest {
			widest = w
		}
	}
	// content is ~16 cells wide; a hugged table stays close to that instead
	// of ballooning to the full 120-col terminal
	if widest > 40 {
		t.Fatalf("expected table to hug content, widest line = %d:\n%s", widest, out)
	}
}

func TestBoardTableGroupsOnceSkipsEmptyShowsStatus(t *testing.T) {
	board := contracts.BoardView{Columns: map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusReady: {
			{ID: "APP-1", Type: contracts.TicketTypeTask, Status: contracts.StatusReady, Priority: contracts.PriorityHigh, Title: "First"},
			{ID: "APP-2", Type: contracts.TicketTypeTask, Status: contracts.StatusReady, Priority: contracts.PriorityLow, Title: "Second"},
		},
		contracts.StatusDone: {
			{ID: "APP-3", Type: contracts.TicketTypeTask, Status: contracts.StatusDone, Priority: contracts.PriorityMedium, Title: "Shipped"},
		},
	}}
	out := BoardPrettyWithWidth(board, 100)
	if got := strings.Count(out, "Ready (2)"); got != 1 {
		t.Fatalf("expected group label exactly once, got %d:\n%s", got, out)
	}
	if strings.Contains(out, "(empty)") || strings.Contains(out, "In Review (0)") || strings.Contains(out, "Backlog (0)") {
		t.Fatalf("expected empty workflow columns to be omitted, got:\n%s", out)
	}
	if !strings.Contains(out, "Status") || !strings.Contains(out, "[ready]") || !strings.Contains(out, "[done]") {
		t.Fatalf("expected per-ticket status column, got:\n%s", out)
	}
	if !strings.Contains(out, "Priority") {
		t.Fatalf("expected full Priority header, got:\n%s", out)
	}
}

func TestTicketsTableMarksSelectedRow(t *testing.T) {
	out := TicketsTable("Board", []contracts.TicketSnapshot{{
		ID:       "APP-1",
		Type:     contracts.TicketTypeTask,
		Status:   contracts.StatusReady,
		Priority: contracts.PriorityHigh,
		Title:    "Selected",
	}}, 72, 0)
	if !strings.Contains(out, ">") || !strings.Contains(out, "Selected") {
		t.Fatalf("expected selected table row marker, got:\n%s", out)
	}
}

func TestSanitizeDisplayStripsTerminalControls(t *testing.T) {
	out := SanitizeDisplay("safe\x1b[2J\x7f\nnext\tcell\u202etxt.exe")
	if strings.ContainsAny(out, "\x1b\x7f") || strings.Contains(out, "\u202e") {
		t.Fatalf("expected control bytes to be removed from terminal display, got %q", out)
	}
	if !strings.Contains(out, "\nnext celltxt.exe") {
		t.Fatalf("expected structural newline and safe tab normalization, got %q", out)
	}
	line := SanitizeDisplayLine("Title\nFAKE\tLINE\u202eevil")
	if strings.ContainsAny(line, "\n\t") || strings.Contains(line, "\u202e") || line != "Title FAKE LINEevil" {
		t.Fatalf("expected inline display text to normalize layout controls, got %q", line)
	}
}

func TestSanitizeTerminalOutputPreservesSGRStylingOnly(t *testing.T) {
	// the output-boundary pass runs after our own lipgloss styling; stripping
	// the ESC there leaves "[90m" residue in every color-capable terminal
	styled := "\x1b[1mSHOP-1\x1b[0m \x1b[38;5;10m[ready]\x1b[0m Add Stripe payment intents"
	if got := SanitizeTerminalOutput(styled); got != styled {
		t.Fatalf("expected SGR styling to survive the output pass, got %q", got)
	}
	hostile := "a\x1b]0;evil\x07b \x1b[2Jc \x1b[10;10Hd \x9b31me"
	got := SanitizeTerminalOutput(hostile)
	if strings.ContainsRune(got, 0x1b) || strings.ContainsRune(got, 0x9b) {
		t.Fatalf("expected non-SGR escapes stripped, got %q", got)
	}
	if strings.Contains(got, "[2J") || strings.Contains(got, "[10;10H") || strings.Contains(got, "31m") {
		t.Fatalf("expected whole non-SGR control sequences stripped, got %q", got)
	}
	// the strict passes used on user content keep rejecting SGR too
	if got := SanitizeDisplay("desc\x1b[31mred"); strings.ContainsRune(got, 0x1b) || strings.Contains(got, "[31m") {
		t.Fatalf("expected strict block scrubber to drop SGR, got %q", got)
	}
	if line := SanitizeDisplayLine("title\x1b[31mred"); strings.ContainsRune(line, 0x1b) || strings.Contains(line, "[31m") {
		t.Fatalf("expected user-content scrubber to drop SGR, got %q", line)
	}
}

func TestTruncateDisplayKeepsStylingWhenLineFits(t *testing.T) {
	styled := "- SHOP-1 \x1b[90m[epic]\x1b[0m \x1b[90m[backlog]\x1b[0m Checkout revamp"
	if got := TruncateDisplay(styled, 120); !strings.Contains(got, "\x1b[90m[epic]\x1b[0m") {
		t.Fatalf("expected fitted line to keep badge styling, got %q", got)
	}
	// overflowing lines trade styling for a safe cut -- never a half escape
	long := "\x1b[90m[epic]\x1b[0m " + strings.Repeat("x", 60)
	got := TruncateDisplay(long, 20)
	if strings.ContainsRune(got, 0x1b) {
		t.Fatalf("expected truncated line to drop styling cleanly, got %q", got)
	}
	if !strings.HasSuffix(got, "...") || lipgloss.Width(got) > 20 {
		t.Fatalf("expected width-bounded ellipsis truncation, got %q (width %d)", got, lipgloss.Width(got))
	}
}

func TestTicketPrettySanitizesUserContent(t *testing.T) {
	ticket := contracts.TicketSnapshot{
		ID:                 "APP-1",
		Status:             contracts.StatusReady,
		Priority:           contracts.PriorityHigh,
		Title:              "wipe\x1b[2J",
		Description:        "desc\x1b[H",
		AcceptanceCriteria: []string{"criteria\x1b[31m"},
	}
	out := TicketPretty(ticket, []string{"comment\x1b[0m"})
	if strings.Contains(out, "\x1b") {
		t.Fatalf("expected pretty output to remove terminal escapes, got %q", out)
	}
	if strings.Contains(out, "[2J") || strings.Contains(out, "[0m") {
		t.Fatalf("expected whole terminal control sequences to be removed, got %q", out)
	}
	if !strings.Contains(out, "wipe") || !strings.Contains(out, "comment") {
		t.Fatalf("expected sanitized content to remain visible, got %q", out)
	}
}

func TestTicketSummarySanitizesTitle(t *testing.T) {
	ticket := contracts.TicketSnapshot{ID: "APP-1", Status: contracts.StatusReady, Priority: contracts.PriorityHigh, Title: "\x1b[2Junsafe\nFAKE\tLINE\u202etxt.exe"}
	out := TicketSummary(ticket, 80)
	if strings.ContainsAny(out, "\x1b\n\t") || strings.Contains(out, "\u202e") {
		t.Fatalf("expected summary to remove terminal and layout controls, got %q", out)
	}
	if !strings.Contains(out, "unsafe FAKE LINEtxt.exe") || strings.Contains(out, "[2J") {
		t.Fatalf("expected sanitized title to remain readable, got %q", out)
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
