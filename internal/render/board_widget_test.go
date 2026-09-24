package render

import (
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestChatBoardFragmentShowsSevenTicketsInStatusColumns(t *testing.T) {
	columns := map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusBacklog: {
			{ID: "APP-1", Title: "Plan the release", Type: contracts.TicketTypeEpic, Status: contracts.StatusBacklog, Priority: contracts.PriorityLow},
			{ID: "APP-2", Title: "Draft migration notes", Type: contracts.TicketTypeTask, Status: contracts.StatusBacklog, Priority: contracts.PriorityMedium},
		},
		contracts.StatusReady:      {{ID: "APP-3", Title: "Build the API", Type: contracts.TicketTypeTask, Status: contracts.StatusReady, Priority: contracts.PriorityHigh}},
		contracts.StatusInProgress: {{ID: "APP-4", Title: "Wire the client", Type: contracts.TicketTypeTask, Status: contracts.StatusInProgress, Priority: contracts.PriorityMedium}},
		contracts.StatusInReview:   {{ID: "APP-5", Title: "Review the board", Type: contracts.TicketTypeTask, Status: contracts.StatusInReview, Priority: contracts.PriorityHigh, ReviewState: contracts.ReviewStatePending}},
		contracts.StatusBlocked: {{ID: "APP-6", Title: "Fix sync", Type: contracts.TicketTypeBug, Status: contracts.StatusBlocked, Priority: contracts.PriorityHigh,
			BlockedBy: []string{"APP-3"}, Description: "Wait for the API", AcceptanceCriteria: []string{"Retry succeeds"}, Labels: []string{"sync"}}},
		contracts.StatusDone: {{ID: "APP-7", Title: "Update help", Type: contracts.TicketTypeTask, Status: contracts.StatusDone, Priority: contracts.PriorityLow}},
	}
	board := NewCompactBoard("APP", columns, -1, nil)
	fragment := CompactBoardWidgetFragment(board)
	if board.TotalCards != 7 || strings.Count(fragment, "<article>") != 7 || strings.Count(fragment, `<details class="card">`) != 7 {
		t.Fatalf("fragment lost cards: total=%d articles=%d details=%d", board.TotalCards, strings.Count(fragment, "<article>"), strings.Count(fragment, `<details class="card">`))
	}
	for _, status := range []string{"backlog", "ready", "in_progress", "in_review", "blocked", "done"} {
		if !strings.Contains(fragment, `class="lane lane-`+status+`"`) {
			t.Fatalf("missing %s lane", status)
		}
	}
	for _, id := range []string{"APP-1", "APP-2", "APP-3", "APP-4", "APP-5", "APP-6", "APP-7"} {
		if strings.Count(fragment, `class="card-id">`+id+`</span>`) != 1 {
			t.Fatalf("ticket %s missing or duplicated", id)
		}
	}
	for _, detail := range []string{"Blocked by APP-3", "Wait for the API", "Retry succeeds", "Review state", "pending"} {
		if !strings.Contains(fragment, detail) {
			t.Fatalf("missing card detail %q", detail)
		}
	}
	if strings.Contains(fragment, "<!DOCTYPE") || strings.Contains(fragment, "<script") || !strings.Contains(fragment, "--hatch-widget-surface-muted") {
		t.Fatal("fragment is not self-contained, script-free, theme-aware HTML")
	}
	if !strings.Contains(fragment, `<label class="view-switch"><input class="view-toggle" type="checkbox" checked><span>Side-scroll lanes</span></label>`) ||
		!strings.Contains(fragment, `<div class="lanes" role="region" aria-label="Board lanes" tabindex="0">`) ||
		!strings.Contains(fragment, `.view-switch:has(.view-toggle:checked)~.lanes`) {
		t.Fatal("fragment lacks an accessible horizontal-lanes switch")
	}
	if strings.Contains(fragment, `id="bw-view"`) || strings.Contains(fragment, `aria-hidden="true" tabindex="-1"`) {
		t.Fatal("view switch must not use a shared ID or hide its keyboard control")
	}
}

func TestChatBoardFragmentEscapesUntrustedTicketFields(t *testing.T) {
	board := NewCompactBoard("APP", map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusReady: {{ID: "APP-1", Title: `<img src=x onerror=alert(1)>`, Status: contracts.StatusReady,
			Description: `<script>alert(1)</script>`, Labels: []string{`<b>urgent</b>`},
			AcceptanceCriteria: []string{`A & B < C`}}},
	}, -1, nil)
	fragment := CompactBoardWidgetFragment(board)
	for _, unsafe := range []string{"<img", "<script", "<b>urgent</b>"} {
		if strings.Contains(fragment, unsafe) {
			t.Fatalf("untrusted markup leaked: %s", unsafe)
		}
	}
	for _, escaped := range []string{"&lt;img", "&lt;script", "&lt;b&gt;urgent", "A &amp; B &lt; C"} {
		if !strings.Contains(fragment, escaped) {
			t.Fatalf("missing escaped field: %s", escaped)
		}
	}
}
