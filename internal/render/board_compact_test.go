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
		"## Ready",
		"## Blocked",
		"## In Review",
		"**APP-1**",
		"**APP-9**",
		"/board?project=APP",
		"agent:builder-1",
	} {
		if !strings.Contains(first, needle) {
			t.Fatalf("markdown missing %q:\n%s", needle, first)
		}
	}
	if strings.Contains(first, "## Backlog") || strings.Contains(first, "(empty)") {
		t.Fatalf("populated board should omit empty lanes:\n%s", first)
	}
	if strings.Contains(first, "[ready]") || strings.Contains(first, "assignee=") || strings.Contains(first, "tracker web serve") {
		t.Fatalf("chat markdown still uses machine/table phrasing:\n%s", first)
	}
	if strings.Contains(first, "%") {
		t.Fatalf("markdown invented a percentage:\n%s", first)
	}
	if strings.ContainsRune(first, 0x1b) {
		t.Fatal("compact markdown must not include ANSI")
	}
	if !strings.Contains(first, "Next:") || !strings.Contains(first, "Attention:") {
		t.Fatalf("markdown should carry next/attention from the structured board:\n%s", first)
	}
	if strings.Index(first, "ready work APP-1") > strings.Index(first, "do not start") && strings.Contains(first, "do not start") {
		t.Fatalf("ready work should lead next actions:\n%s", first)
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
	if !strings.Contains(md, "2 of 5 tickets") && !strings.Contains(md, "showing 2") {
		t.Fatalf("markdown should indicate truncation:\n%s", md)
	}
}

func TestCompactBoardMarkdownEscapesUntrustedText(t *testing.T) {
	board := NewCompactBoard("APP", map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusReady: {{
			ID: "APP-1", Title: "See ![x](https://evil.example) **bold** <img>",
			Status: contracts.StatusReady, Labels: []string{"[hi](http://x)"},
		}},
	}, 20, nil)
	md := CompactBoardMarkdown(board)
	for _, needle := range []string{"![x]", "](http://", "**bold**"} {
		if strings.Contains(md, needle) {
			t.Fatalf("untrusted markdown leaked %q:\n%s", needle, md)
		}
	}
	if strings.Contains(md, "<img") && !strings.Contains(md, "\\<img") {
		t.Fatalf("untrusted HTML tag leaked:\n%s", md)
	}
	if !strings.Contains(md, "\\!\\[x\\]") {
		t.Fatalf("expected escaped image markup:\n%s", md)
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
	if !strings.Contains(htmlDoc, `name="viewport"`) {
		t.Fatalf("app should include a viewport meta:\n%s", htmlDoc)
	}
	article := htmlDoc
	if i := strings.Index(htmlDoc, "<article>"); i >= 0 {
		article = htmlDoc[i:]
	}
	waitIdx := strings.Index(article, "Wait")
	idIdx := strings.Index(article, "APP-1")
	if waitIdx < 0 || idIdx < 0 || waitIdx > idIdx {
		t.Fatalf("title should lead the card, id secondary:\n%s", htmlDoc)
	}
	if strings.Contains(htmlDoc, `class="status`) {
		t.Fatalf("lane cards should not repeat status chips:\n%s", htmlDoc)
	}
}
