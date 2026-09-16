package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestParseBoardStyleDefaultsToTable(t *testing.T) {
	table, err := ParseBoardStyle("")
	if err != nil || table != BoardStyleTable {
		t.Fatalf("empty style should be table, got %q err=%v", table, err)
	}
	kanban, err := ParseBoardStyle("kanban")
	if err != nil || kanban != BoardStyleKanban {
		t.Fatalf("kanban: %q err=%v", kanban, err)
	}
	modern, err := ParseBoardStyle("modern")
	if err != nil || modern != BoardStyleKanban || !modern.IsKanban() {
		t.Fatalf("modern should alias kanban, got %q err=%v", modern, err)
	}
	legacy, err := ParseBoardStyle("legacy")
	if err != nil || legacy != BoardStyleLegacy {
		t.Fatalf("legacy: %q err=%v", legacy, err)
	}
	chat, err := ParseBoardStyle("chat")
	if err != nil || chat != BoardStyleChat {
		t.Fatalf("chat: %q err=%v", chat, err)
	}
	if _, err := ParseBoardStyle("neon"); err == nil {
		t.Fatal("expected invalid style error")
	}
}

func TestPolishedTableIsDefaultAndNotAGrid(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	out := RenderBoard(fixtureBoard(), BoardRenderOptions{Width: 100, ShowSummary: true})
	if strings.Contains(out, "+---") || strings.Contains(out, "│") || strings.Contains(out, "Column") {
		t.Fatalf("default table must not be a boxed grid:\n%s", out)
	}
	for _, needle := range []string{"Board APP", "APP-1", "APP-9", "Ready", "Blocked", "ID", "STATUS", "TITLE", "agent:builder-1"} {
		if !strings.Contains(out, needle) {
			t.Fatalf("polished table missing %q:\n%s", needle, out)
		}
	}
	if !strings.Contains(out, "Ready") || !strings.Contains(out, "Blocked") {
		t.Fatalf("status must stay textual:\n%s", out)
	}
}

func TestPolishedTableFitsWidthsAndKeepsIDs(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	id := "VERYLONGPROJECT-99"
	board := NewCompactBoard("VERYLONGPROJECT", map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusReady: {{
			ID: id, Project: "VERYLONGPROJECT",
			Title:  "A very long title that should wrap or yield space to the id 表表表 👩‍💻",
			Status: contracts.StatusReady, Priority: contracts.PriorityHigh,
			Assignee: "agent:builder-1",
		}},
		contracts.StatusBlocked: {{
			ID: "APP-2", Project: "VERYLONGPROJECT", Title: "Stuck",
			Status: contracts.StatusBlocked, Priority: contracts.PriorityCritical,
		}},
	}, -1, nil)
	for _, width := range []int{40, 80, 120, 180} {
		out := RenderBoard(board, BoardRenderOptions{Width: width, ShowSummary: true})
		if !outputHasID(out, id) || !outputHasID(out, "APP-2") {
			t.Fatalf("width %d dropped an id:\n%s", width, out)
		}
		if strings.Contains(out, "VERYLONGPROJ...") || strings.Contains(out, "VERYLONGPROJECT-9...") {
			t.Fatalf("width %d ellipsized the long id:\n%s", width, out)
		}
		if !strings.Contains(out, "Ready") || !strings.Contains(out, "Blocked") {
			t.Fatalf("width %d lost status labels:\n%s", width, out)
		}
		for _, line := range strings.Split(out, "\n") {
			if lipgloss.Width(line) > width {
				t.Fatalf("width %d overflow %d line=%q", width, lipgloss.Width(line), line)
			}
		}
	}
}

func TestPolishedTablePrintsAllRowsWhenUnbounded(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	columns := map[contracts.Status][]contracts.TicketSnapshot{}
	for i := 1; i <= 24; i++ {
		columns[contracts.StatusReady] = append(columns[contracts.StatusReady], contracts.TicketSnapshot{
			ID:      "APP-" + itoa(i),
			Title:   "Crowded ready work " + itoa(i) + " with a long title for scanning",
			Status:  contracts.StatusReady,
			Project: "APP",
		})
	}
	board := NewCompactBoard("APP", columns, -1, nil)
	out := RenderBoard(board, BoardRenderOptions{Width: 80, Height: 0, ShowSummary: true})
	for i := 1; i <= 24; i++ {
		if !outputHasID(out, "APP-"+itoa(i)) {
			t.Fatalf("unbounded table hid APP-%d:\n%s", i, out)
		}
	}
	if strings.Contains(out, "showing ") && strings.Contains(out, " to ") && strings.Contains(out, " of 24") {
		t.Fatalf("CLI/unbounded table should print every row, not a window:\n%s", out)
	}
}

func TestPolishedTableWindowsWithHonestCount(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	columns := map[contracts.Status][]contracts.TicketSnapshot{}
	for i := 1; i <= 20; i++ {
		columns[contracts.StatusInProgress] = append(columns[contracts.StatusInProgress], contracts.TicketSnapshot{
			ID:      "R-" + itoa(i),
			Title:   "Card " + itoa(i),
			Status:  contracts.StatusInProgress,
			Project: "APP",
		})
	}
	board := NewCompactBoard("APP", columns, -1, nil)
	out := RenderBoard(board, BoardRenderOptions{
		Width: 80, Height: 14, ShowSummary: true, SelectedID: "R-12",
	})
	if !outputHasID(out, "R-12") {
		t.Fatalf("window hid selected R-12:\n%s", out)
	}
	if !strings.Contains(out, "> R-12") {
		t.Fatalf("selected marker missing for R-12:\n%s", out)
	}
	if strings.Contains(out, "R-12...") {
		t.Fatalf("selected id ellipsized:\n%s", out)
	}
	var from, to, total int
	found := false
	for _, line := range strings.Split(out, "\n") {
		if _, err := fmt.Sscanf(strings.TrimSpace(line), "showing %d to %d of %d", &from, &to, &total); err == nil {
			found = true
			break
		}
	}
	if !found || total != 20 || from < 1 || to < from || to > 20 {
		t.Fatalf("honest window missing or nonsense: from=%d to=%d total=%d\n%s", from, to, total, out)
	}
	if from == 1 && to == 20 {
		t.Fatalf("short height should not show every row:\n%s", out)
	}
}

func TestPolishedTableDoesNotSwitchOnTicketCount(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	one := NewCompactBoard("APP", map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusReady: {{ID: "APP-1", Title: "One", Status: contracts.StatusReady, Project: "APP"}},
	}, -1, nil)
	manyCols := map[contracts.Status][]contracts.TicketSnapshot{}
	for i := 1; i <= 18; i++ {
		manyCols[contracts.StatusReady] = append(manyCols[contracts.StatusReady], contracts.TicketSnapshot{
			ID: "APP-" + itoa(i), Title: "Card", Status: contracts.StatusReady, Project: "APP",
		})
	}
	many := NewCompactBoard("APP", manyCols, -1, nil)
	a := RenderBoard(one, BoardRenderOptions{Width: 100, ShowSummary: true})
	b := RenderBoard(many, BoardRenderOptions{Width: 100, ShowSummary: true})
	if strings.Contains(a, "▏") || strings.Contains(b, "▏") {
		t.Fatalf("ticket count must not flip the board into lanes:\n%s\n---\n%s", a, b)
	}
	if !strings.Contains(a, "STATUS") || !strings.Contains(b, "STATUS") {
		t.Fatalf("both counts should stay tabular:\n%s\n---\n%s", a, b)
	}
}

func TestPolishedTableTinyWidthKeepsID(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	board := NewCompactBoard("APP", map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusReady: {{ID: "APP-1", Title: "Tiny terminal", Status: contracts.StatusReady, Priority: contracts.PriorityHigh}},
	}, -1, nil)
	out := RenderBoard(board, BoardRenderOptions{Width: 8, ShowSummary: true})
	if !outputHasID(out, "APP-1") {
		t.Fatalf("tiny table dropped id:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if lipgloss.Width(line) > 8 {
			t.Fatalf("tiny overflow width=%d line=%q", lipgloss.Width(line), line)
		}
	}
}

func TestKanbanStyleStillRendersLanes(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	out := RenderBoard(fixtureBoard(), BoardRenderOptions{Width: 100, Style: BoardStyleKanban, ShowSummary: true})
	if strings.Contains(out, "+---") {
		t.Fatalf("kanban should stay card/lane:\n%s", out)
	}
	if !strings.Contains(out, "Ready") || !strings.Contains(out, "APP-1") {
		t.Fatalf("kanban lost tickets:\n%s", out)
	}
}

func TestCrowdedBacklogStaysRows(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	columns := map[contracts.Status][]contracts.TicketSnapshot{}
	for i := 1; i <= 16; i++ {
		columns[contracts.StatusBacklog] = append(columns[contracts.StatusBacklog], contracts.TicketSnapshot{
			ID: fmt.Sprintf("SHOP-%d", i), Title: "Backlog item " + itoa(i), Status: contracts.StatusBacklog, Project: "SHOP",
		})
	}
	board := NewCompactBoard("SHOP", columns, -1, nil)
	out := RenderBoard(board, BoardRenderOptions{Width: 120, ShowSummary: true})
	if strings.Contains(out, "▏") {
		t.Fatalf("crowded backlog must stay a table:\n%s", out)
	}
	for i := 1; i <= 16; i++ {
		if !outputHasID(out, fmt.Sprintf("SHOP-%d", i)) {
			t.Fatalf("crowded backlog hid SHOP-%d:\n%s", i, out)
		}
	}
}
