package render

import (
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func chatFixtureBoard() CompactBoard {
	now := time.Date(2026, 9, 16, 15, 0, 0, 0, time.UTC)
	return NewCompactBoard("APP", map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusReady: {{
			ID: "APP-12", Project: "APP", Title: "Ship first feature",
			Status: contracts.StatusReady, Priority: contracts.PriorityHigh,
			Assignee: "agent:builder-1", UpdatedAt: now,
		}},
		contracts.StatusInProgress: {{
			ID: "APP-8", Project: "APP", Title: "Retry backoff",
			Status: contracts.StatusInProgress, UpdatedAt: now,
		}},
		contracts.StatusBlocked: {{
			ID: "APP-3", Project: "APP", Title: "Auth API",
			Status: contracts.StatusBlocked, Priority: contracts.PriorityCritical,
			BlockedBy: []string{"APP-1"}, UpdatedAt: now,
		}},
	}, -1, nil)
}

func TestCompactBoardChatIsFencedANSIAndStatusTextual(t *testing.T) {
	board := chatFixtureBoard()
	first := CompactBoardChat(board)
	second := CompactBoardChat(board)
	if first != second {
		t.Fatal("chat board must be deterministic")
	}
	if !strings.HasPrefix(first, "```ansi\n") || !strings.HasSuffix(strings.TrimSpace(first), "```") {
		t.Fatalf("chat board must be an ansi fence:\n%s", first)
	}
	for _, needle := range []string{
		"ATLAS",
		"APP",
		"READY",
		"IN PROGRESS",
		"BLOCKED",
		"APP-12",
		"APP-8",
		"APP-3",
		"Ship first feature",
		"HIGH",
		"NEXT",
	} {
		if !strings.Contains(first, needle) {
			t.Fatalf("chat board missing %q:\n%s", needle, first)
		}
	}
	if !strings.ContainsRune(first, 0x1b) {
		t.Fatal("chat board should include Discord-safe ANSI")
	}
	if strings.Contains(first, "38;5;") || strings.Contains(first, "38;2;") {
		t.Fatalf("chat board must stay on 3/4-bit Discord ANSI:\n%s", first)
	}
	if strings.Contains(first, "## Backlog") || strings.Contains(first, "(empty)") {
		t.Fatalf("populated chat board should omit empty lanes:\n%s", first)
	}
	if strings.Contains(first, "%") {
		t.Fatalf("chat board invented a percentage:\n%s", first)
	}
	if !strings.Contains(stripChatANSI(first), "HIGH") || strings.Contains(first, "HIG...") {
		t.Fatalf("priority should stay readable:\n%s", first)
	}
	plain := stripChatANSI(first)
	if DisplayWidth(strings.Split(plain, "\n")[0]) > ChatBoardWidth+8 {
		t.Fatalf("chat headline is wider than the chat column:\n%s", first)
	}
}

func TestCompactBoardChatStripsUntrustedMarkup(t *testing.T) {
	board := NewCompactBoard("APP", map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusReady: {{
			ID: "APP-1", Title: "See ![x](https://evil.example) ```rm -rf``` <img>",
			Status: contracts.StatusReady,
		}},
	}, 20, nil)
	out := CompactBoardChat(board)
	if strings.Contains(out, "```rm") || strings.Count(out, "```") != 2 {
		t.Fatalf("untrusted fence leaked:\n%s", out)
	}
	if strings.ContainsRune(out[len("```ansi\n"):strings.LastIndex(out, "```")], '`') {
		t.Fatalf("untrusted backticks leaked into the ansi body:\n%s", out)
	}
}

func TestStatusChatUnknownProject(t *testing.T) {
	out := StatusChat(StatusChatView{
		UnknownProject: true,
		Project:        "NOPE",
		Disambiguation: []string{"APP", "OPS"},
	})
	if !strings.Contains(out, "Unknown project") || !strings.Contains(out, "APP") || !strings.Contains(out, "OPS") {
		t.Fatalf("unknown project chat:\n%s", out)
	}
	if strings.Contains(out, "# Board APP") {
		t.Fatal("unknown project must not return another project's board")
	}
}

func TestStatusChatFocusesTicket(t *testing.T) {
	out := StatusChat(StatusChatView{
		Scope:           "ticket",
		TicketID:        "APP-8",
		Project:         "APP",
		Board:           chatFixtureBoard(),
		RecommendedNext: []string{"continue APP-8"},
	})
	if !strings.Contains(out, "APP-8") || !strings.Contains(out, "Retry backoff") {
		t.Fatalf("ticket chat missing focused card:\n%s", out)
	}
	if strings.Contains(out, "APP-12") {
		t.Fatalf("ticket chat should not dump the whole board:\n%s", out)
	}
	if !strings.Contains(out, "continue APP-8") {
		t.Fatalf("ticket chat should keep recommended next:\n%s", out)
	}
}

func TestRenderBoardChatStyleIgnoresNOCOLOR(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	out := RenderBoard(chatFixtureBoard(), BoardRenderOptions{Style: BoardStyleChat, ShowSummary: true})
	if !strings.Contains(out, "```ansi") || !strings.ContainsRune(out, 0x1b) {
		t.Fatalf("chat style is a paste payload, not terminal chrome:\n%s", out)
	}
}

func TestParseBoardStyleChat(t *testing.T) {
	style, err := ParseBoardStyle("chat")
	if err != nil || style != BoardStyleChat {
		t.Fatalf("chat style: %q err=%v", style, err)
	}
	if _, err := ParseBoardStyle("neon"); err == nil {
		t.Fatal("expected invalid style error")
	}
}

func stripChatANSI(value string) string {
	var b strings.Builder
	in := false
	for i := 0; i < len(value); i++ {
		if value[i] == 0x1b {
			in = true
			continue
		}
		if in {
			if value[i] == 'm' {
				in = false
			}
			continue
		}
		b.WriteByte(value[i])
	}
	return b.String()
}
