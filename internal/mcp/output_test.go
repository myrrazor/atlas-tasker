package mcp

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/render"
)

func TestTextFallbackTruncatesLargeChatBoardWithValidFence(t *testing.T) {
	chat := largeChatBoard(80)
	if utf8.RuneCountInString(chat) <= fallbackMaxChars(50) {
		t.Fatalf("large board fixture must exceed the 50-token fallback budget, got %d runes", utf8.RuneCountInString(chat))
	}
	got := textFallback("atlas.board", map[string]any{"chat": chat}, false, 50)
	assertPasteReadyANSI(t, got)
	if !strings.Contains(got, "ATLAS") {
		t.Fatalf("truncated chat should keep the board head:\n%s", got)
	}
	if strings.Contains(got, "APP-80") {
		t.Fatalf("truncated chat should drop the tail of a large board:\n%s", got)
	}
}

func TestTextFallbackDoesNotCutSGRWhenChatExceedsBudget(t *testing.T) {
	// 196 visible prefix runes after ```ansi\n (8) put the next SGR across
	// the naive 200-rune cut: `\x1b[1;37m` starts at rune 188 of the full
	// fence and would be sliced to `\x1b[1;` by truncateRunes.
	body := strings.Repeat("x", 180) + "\x1b[1;37mTITLE\x1b[0m"
	chat := "```ansi\n" + body + "\n```\n"
	naive := truncateRunes(chat, 200)
	if !strings.Contains(naive, "\x1b[1;") || strings.HasSuffix(strings.TrimSpace(naive), "```") {
		t.Fatalf("precondition: naive truncation should split SGR and drop the fence, got %q", naive)
	}
	got := textFallback("atlas.status", map[string]any{
		"payload": map[string]any{"chat": chat},
	}, false, 50)
	assertPasteReadyANSI(t, got)
	if strings.Contains(got, "\x1b[1;37m") && !strings.Contains(got, "TITLE") {
		t.Fatalf("kept a color opener without its text:\n%q", got)
	}
}

func TestTextFallbackChatKeepsFenceWhenPayloadAlreadyTruncated(t *testing.T) {
	got := textFallback("atlas.board", map[string]any{"chat": largeChatBoard(40), "truncated": true}, true, 50)
	if !strings.HasPrefix(got, "atlas.board returned a truncated result:\n") {
		t.Fatalf("expected truncated-result prefix, got:\n%s", got)
	}
	assertPasteReadyANSI(t, strings.TrimPrefix(got, "atlas.board returned a truncated result:\n"))
}

func TestTextFallbackMarkdownStillRuneTruncates(t *testing.T) {
	md := "# Board\n" + strings.Repeat("ticket APP-1 does a thing\n", 40)
	got := textFallback("atlas.board", map[string]any{"markdown": md}, false, 50)
	if isANSIChatFence(got) {
		t.Fatalf("markdown fallback must not become an ansi fence:\n%s", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("markdown fallback should keep rune truncation, got %q", got)
	}
	if utf8.RuneCountInString(got) != fallbackMaxChars(50)+3 {
		t.Fatalf("markdown fallback length %d", utf8.RuneCountInString(got))
	}
}

func largeChatBoard(n int) string {
	tickets := make([]contracts.TicketSnapshot, n)
	now := contracts.TicketSnapshot{}.UpdatedAt
	for i := 0; i < n; i++ {
		tickets[i] = contracts.TicketSnapshot{
			ID:        fmt.Sprintf("APP-%d", i+1),
			Project:   "APP",
			Title:     "Ship a long feature name for Discord paste",
			Status:    contracts.StatusReady,
			Priority:  contracts.PriorityHigh,
			UpdatedAt: now,
		}
	}
	board := render.NewCompactBoard("APP", map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusReady: tickets,
	}, -1, nil)
	return render.CompactBoardChat(board)
}

func assertPasteReadyANSI(t *testing.T, text string) {
	t.Helper()
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "```ansi") {
		t.Fatalf("missing ```ansi opener:\n%s", text)
	}
	if !strings.HasSuffix(trimmed, "```") {
		t.Fatalf("missing closing fence:\n%s", text)
	}
	if strings.Count(trimmed, "```") < 2 {
		t.Fatalf("unterminated fence:\n%s", text)
	}
	if !strings.Contains(text, "\x1b[0m") {
		t.Fatalf("truncated chat must reset SGR:\n%q", text)
	}
	if incompleteSGR(text) {
		t.Fatalf("cut inside an SGR sequence:\n%q", text)
	}
}

func incompleteSGR(text string) bool {
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		if runes[i] != 0x1b {
			continue
		}
		end, ok := sgrSequenceEnd(runes, i)
		if !ok {
			return true
		}
		i = end
	}
	return false
}
