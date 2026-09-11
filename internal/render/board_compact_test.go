package render

import (
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestCompactBoardMarkdownIsDeterministicAndStatusTextual(t *testing.T) {
	now := time.Date(2026, 9, 11, 15, 0, 0, 0, time.UTC)
	columns := map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusReady: {
			{ID: "APP-2", Project: "APP", Title: "Second", Status: contracts.StatusReady, Priority: contracts.PriorityLow, UpdatedAt: now},
			{ID: "APP-1", Project: "APP", Title: "First", Status: contracts.StatusReady, Priority: contracts.PriorityHigh, UpdatedAt: now.Add(time.Hour)},
		},
		contracts.StatusBlocked: {
			{ID: "APP-9", Project: "APP", Title: "API decision", Status: contracts.StatusBlocked, Priority: contracts.PriorityHigh, Assignee: "agent:builder-1", UpdatedAt: now},
		},
		contracts.StatusInReview: {
			{ID: "APP-3", Project: "APP", Title: "Review me", Status: contracts.StatusInReview, UpdatedAt: now},
		},
	}
	board := NewCompactBoard("APP", columns, 20, nil)
	first := CompactBoardMarkdown(board)
	second := CompactBoardMarkdown(board)
	if first != second {
		t.Fatal("compact markdown must be deterministic")
	}
	for _, needle := range []string{
		"# Board APP",
		"[ready]",
		"[blocked]",
		"[in_review]",
		"APP-1",
		"APP-9",
		"/board?project=APP",
		"assignee=agent:builder-1",
	} {
		if !strings.Contains(first, needle) {
			t.Fatalf("markdown missing %q:\n%s", needle, first)
		}
	}
	if strings.Contains(first, "%") {
		t.Fatalf("markdown invented a percentage:\n%s", first)
	}
	if !strings.Contains(first, "APP-1") || strings.Index(first, "APP-1") > strings.Index(first, "APP-2") {
		t.Fatalf("ready column should list newer APP-1 first:\n%s", first)
	}
}

func TestCompactBoardTruncatesAndMarksPagination(t *testing.T) {
	columns := map[contracts.Status][]contracts.TicketSnapshot{}
	for i := 1; i <= 5; i++ {
		columns[contracts.StatusReady] = append(columns[contracts.StatusReady], contracts.TicketSnapshot{
			ID:     "APP-" + string(rune('0'+i)),
			Title:  "Card",
			Status: contracts.StatusReady,
		})
	}
	board := NewCompactBoard("APP", columns, 2, map[string]string{"ready": "2"})
	if !board.Truncated || board.ShownCards != 2 || board.TotalCards != 5 {
		t.Fatalf("truncation: %#v", board)
	}
	md := CompactBoardMarkdown(board)
	if !strings.Contains(md, "Truncated") {
		t.Fatalf("markdown should indicate truncation:\n%s", md)
	}
}

func TestCompactBoardAppHTMLEnforcesCSPAndNoScripts(t *testing.T) {
	board := NewCompactBoard("APP", map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusBlocked: {{ID: "APP-1", Title: "Wait", Status: contracts.StatusBlocked}},
	}, 20, nil)
	htmlDoc := CompactBoardAppHTML(board)
	if !strings.Contains(htmlDoc, BoardAppCSP) {
		t.Fatalf("missing CSP:\n%s", htmlDoc)
	}
	banned := []string{"<script", "javascript:", "http://", "https://", "<form", "onclick=", "fetch("}
	for _, needle := range banned {
		if strings.Contains(strings.ToLower(htmlDoc), needle) {
			t.Fatalf("MCP App must not contain %q:\n%s", needle, htmlDoc)
		}
	}
	if !strings.Contains(htmlDoc, "blocked") || !strings.Contains(htmlDoc, "APP-1") {
		t.Fatalf("app should label blocked in text:\n%s", htmlDoc)
	}
	if !strings.Contains(htmlDoc, `aria-label="Blocked"`) {
		t.Fatalf("column should be accessible by name:\n%s", htmlDoc)
	}
}
