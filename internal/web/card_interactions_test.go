package web

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestBoardCardsRenderMinimalFacePreviewDataAndMappedAgentChip(t *testing.T) {
	h := newWebHarness(t, false)
	if _, err := h.actions.MutateTrackedTicket(context.Background(), h.ticketID, contracts.Actor("human:owner"), "test", "seed card metadata", func(ticket *contracts.TicketSnapshot) error {
		ticket.Assignee = contracts.Actor("agent:claude")
		ticket.Reviewer = contracts.Actor("agent:reviewer-1")
		ticket.Labels = []string{"web", "release"}
		ticket.BlockedBy = []string{"WEB-99"}
		ticket.OpenGateIDs = []string{"gate-review"}
		return nil
	}); err != nil {
		t.Fatalf("seed card metadata: %v", err)
	}
	if err := h.actions.CommentTicket(context.Background(), h.ticketID, "preview note", contracts.Actor("human:owner"), "test comment"); err != nil {
		t.Fatalf("seed comment: %v", err)
	}

	res := h.doAuthed(t, http.MethodGet, "/board", "", nil)
	card := firstCardMarkup(t, res.body)
	for _, want := range []string{
		`data-status="Blocked"`,
		`data-assignee="agent:claude"`,
		`data-reviewer="agent:reviewer-1"`,
		`data-priority="high"`,
		`data-labels="web, release"`,
		`data-blockers="1"`,
		`data-gates="1"`,
		`data-comments="1"`,
		`class="card-agent-chip chip--orange"`,
		`title="Assigned to agent:claude"`,
	} {
		if !strings.Contains(card, want) {
			t.Fatalf("minimal card missing %q:\n%s", want, card)
		}
	}
	for _, removed := range []string{"card-people", "card-foot", `class="priority`, `class="labels`, "drag-handle"} {
		if strings.Contains(card, removed) {
			t.Fatalf("minimal card face still contains %q:\n%s", removed, card)
		}
	}
	for _, drawerCopy := range []string{"Notes &amp; activity", "Notes &amp; comments", `class="drawer-labels"`} {
		if !strings.Contains(res.body, drawerCopy) {
			t.Fatalf("drawer missing %q:\n%s", drawerCopy, excerpt(res.body, "detail-drawer"))
		}
	}
}

func TestAgentChipRequiresSupportedConfiguredColor(t *testing.T) {
	h := newWebHarness(t, false)
	if _, err := h.actions.MutateTrackedTicket(context.Background(), h.ticketID, contracts.Actor("human:owner"), "test", "assign unmapped agent", func(ticket *contracts.TicketSnapshot) error {
		ticket.Assignee = contracts.Actor("agent:merlin")
		return nil
	}); err != nil {
		t.Fatalf("assign unmapped agent: %v", err)
	}

	assertChip := func(want bool) {
		t.Helper()
		res := h.doAuthed(t, http.MethodGet, "/board", "", nil)
		got := strings.Contains(firstCardMarkup(t, res.body), "card-agent-chip")
		if got != want {
			t.Fatalf("agent chip present=%v, want %v", got, want)
		}
	}

	assertChip(false)
	if err := config.Set(h.root, "web.agent_colors.merlin", "chartreuse"); err != nil {
		t.Fatalf("set unsupported color: %v", err)
	}
	assertChip(false)
	if err := config.Set(h.root, "web.agent_colors.merlin", "orange"); err != nil {
		t.Fatalf("set supported color: %v", err)
	}
	assertChip(true)
}

func TestBoardInteractionAssetsKeepPreviewLocalAndMotionReduced(t *testing.T) {
	jsRaw, err := embeddedFiles.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	js := string(jsRaw)
	for _, want := range []string{
		"showCardPreview(card)",
		"card.getBoundingClientRect()",
		"}, 2000);",
		"document.addEventListener('scroll', dismissCardPreview, true)",
		"event.key === 'Escape'",
		"window.requestAnimationFrame(() =>",
		"animation: 150",
		"ghostClass: 'sortable-ghost'",
		"chosenClass: 'sortable-chosen'",
		"function captureBoardMotion(grid)",
		"function playBoardSwapMotion(previous, grid)",
		"dragsInFlight > 0 || prefersReducedMotion()",
		"{ duration: 180, easing }",
		"settleDroppedCard(event.item)",
		"'is-count-pulsing'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("app.js missing interaction contract %q", want)
		}
	}
	previewStart := strings.Index(js, "function showCardPreview")
	previewEnd := strings.Index(js[previewStart:], "function setupCardPreviews")
	if previewStart < 0 || previewEnd < 0 {
		t.Fatal("could not isolate card preview implementation")
	}
	if strings.Contains(js[previewStart:previewStart+previewEnd], "fetch(") {
		t.Fatal("hover preview must use card data attributes without fetching")
	}
	refreshStart := strings.Index(js, "async function refreshBoard")
	if refreshStart < 0 {
		t.Fatal("could not find board refresh implementation")
	}
	refreshEnd := strings.Index(js[refreshStart:], "function setupSortable")
	if refreshEnd < 0 {
		t.Fatal("could not isolate board refresh implementation")
	}
	refresh := js[refreshStart : refreshStart+refreshEnd]
	wait := strings.Index(refresh, "await waitForDragEnd()")
	capture := strings.Index(refresh, "captureBoardMotion(")
	swap := strings.Index(refresh, "current.replaceWith(next)")
	play := strings.Index(refresh, "playBoardSwapMotion(")
	if wait < 0 || capture < wait || swap < capture || play < swap {
		t.Fatalf("board refresh must wait for drag end, capture before swap, and animate after swap")
	}

	cssRaw, err := embeddedFiles.ReadFile("static/app.css")
	if err != nil {
		t.Fatalf("read app.css: %v", err)
	}
	css := string(cssRaw)
	for _, want := range []string{
		".detail-drawer.drawer--motion-ready",
		"transition: transform 0.2s var(--ease)",
		"@media (prefers-reduced-motion: reduce)",
		".card-preview",
		".topbar--board .meta-block",
		"animation: ticket-drop-settle 150ms var(--ease)",
		"animation: column-count-pulse 300ms var(--ease)",
		".ticket-card.is-drop-settling,",
		".col-count.is-count-pulsing::after",
	} {
		if !strings.Contains(css, want) {
			t.Fatalf("app.css missing interaction contract %q", want)
		}
	}
}

func firstCardMarkup(t *testing.T, body string) string {
	t.Helper()
	start := strings.Index(body, `<a class="ticket-card"`)
	if start < 0 {
		t.Fatalf("ticket card not found:\n%s", body)
	}
	end := strings.Index(body[start:], "</a>")
	if end < 0 {
		t.Fatalf("ticket card closing tag not found:\n%s", body[start:])
	}
	return body[start : start+end+len("</a>")]
}
