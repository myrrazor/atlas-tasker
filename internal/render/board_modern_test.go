package render

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func fixtureBoard() CompactBoard {
	now := time.Date(2026, 9, 11, 15, 0, 0, 0, time.UTC)
	return NewCompactBoard("APP", map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusReady: {
			{
				ID: "APP-1", Project: "APP", Title: "First", Status: contracts.StatusReady,
				Priority: contracts.PriorityHigh, Assignee: "agent:builder-1", UpdatedAt: now.Add(time.Hour),
			},
			{
				ID: "APP-2", Project: "APP", Title: "Second", Status: contracts.StatusReady,
				Priority: contracts.PriorityLow, UpdatedAt: now,
			},
		},
		contracts.StatusBlocked: {
			{
				ID: "APP-9", Project: "APP", Title: "API decision", Status: contracts.StatusBlocked,
				Priority: contracts.PriorityHigh, Assignee: "agent:builder-1", BlockedBy: []string{"APP-1"}, UpdatedAt: now,
			},
		},
		contracts.StatusInReview: {
			{ID: "APP-3", Project: "APP", Title: "Review me", Status: contracts.StatusInReview, UpdatedAt: now},
		},
	}, -1, nil)
}

func TestModernBoardIsNotAGridTable(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	out := RenderBoard(fixtureBoard(), BoardRenderOptions{Width: 100, Style: BoardStyleModern, Density: DensityComfortable, ShowSummary: true})
	if strings.Contains(out, "+---") || strings.Contains(out, "│") {
		t.Fatalf("modern board must not be a boxed table:\n%s", out)
	}
	for _, needle := range []string{"Board APP", "APP-1", "APP-9", "Ready", "Blocked", "agent:builder-1", "Next"} {
		if !strings.Contains(out, needle) {
			t.Fatalf("modern board missing %q:\n%s", needle, out)
		}
	}
	// status lives on the lane, not as a chip on every ready card
	if strings.Count(out, "[ready]") > 0 {
		t.Fatalf("ready lane should not repeat [ready] on each card:\n%s", out)
	}
	medium := RenderBoard(fixtureBoard(), BoardRenderOptions{Width: 80, Style: BoardStyleKanban, ShowSummary: true})
	if strings.Contains(medium, "...|") || strings.Contains(medium, "tokens|") {
		t.Fatalf("adjacent lanes should keep a gutter, got:\n%s", medium)
	}
	for _, line := range strings.Split(medium, "\n") {
		if strings.Contains(line, "APP-1") && strings.Contains(line, "APP-9") {
			t.Fatalf("ready and blocked cards collided on one line: %q\n%s", line, medium)
		}
	}
}

func TestModernBoardFitsWidthsAndKeepsIDs(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	board := fixtureBoard()
	for _, width := range []int{8, 32, 52, 80, 120} {
		out := RenderBoard(board, BoardRenderOptions{Width: width, Style: BoardStyleKanban, ShowSummary: true})
		if !outputHasID(out, "APP-1") {
			t.Fatalf("width %d dropped APP-1:\n%s", width, out)
		}
		for _, line := range strings.Split(out, "\n") {
			if lipgloss.Width(line) > width {
				t.Fatalf("width %d overflow %d line=%q", width, lipgloss.Width(line), line)
			}
		}
	}
}

func TestModernBoardEmptyAndMany(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	empty := NewCompactBoard("APP", nil, -1, nil)
	out := RenderBoard(empty, BoardRenderOptions{Width: 72, Style: BoardStyleKanban, ShowSummary: true})
	if !strings.Contains(out, "(empty)") {
		t.Fatalf("empty board should say so:\n%s", out)
	}
	columns := map[contracts.Status][]contracts.TicketSnapshot{}
	for i := 1; i <= 30; i++ {
		columns[contracts.StatusReady] = append(columns[contracts.StatusReady], contracts.TicketSnapshot{
			ID:     "APP-" + itoa(i),
			Title:  "Card",
			Status: contracts.StatusReady,
		})
	}
	many := NewCompactBoard("APP", columns, 8, nil)
	clipped := RenderBoard(many, BoardRenderOptions{Width: 72, Height: 16, Style: BoardStyleKanban, ShowSummary: true, SelectedID: "APP-1"})
	if !strings.Contains(clipped, "APP-1") {
		t.Fatalf("windowed lane dropped selected id:\n%s", clipped)
	}
	if !strings.Contains(clipped, "more") && !strings.Contains(clipped, "showing") {
		t.Fatalf("many-ticket board should admit it is clipped:\n%s", clipped)
	}
}

func TestWindowKeepsNonFirstSelectedID(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	columns := map[contracts.Status][]contracts.TicketSnapshot{}
	for i := 1; i <= 20; i++ {
		columns[contracts.StatusReady] = append(columns[contracts.StatusReady], contracts.TicketSnapshot{
			ID:     "R-" + itoa(i),
			Title:  "Card " + itoa(i),
			Status: contracts.StatusReady,
		})
	}
	board := NewCompactBoard("APP", columns, -1, nil)
	out := RenderBoard(board, BoardRenderOptions{
		Width: 72, Height: 14, Style: BoardStyleKanban, ShowSummary: true, SelectedID: "R-12",
	})
	if !outputHasID(out, "R-12") {
		t.Fatalf("window hid selected R-12:\n%s", out)
	}
	if !strings.Contains(out, "> R-12") {
		t.Fatalf("selected marker missing for R-12:\n%s", out)
	}
	if strings.Contains(out, "R-1...") || strings.Contains(out, "R-12...") {
		t.Fatalf("selected id ellipsized:\n%s", out)
	}
	shown := 0
	for _, line := range strings.Split(out, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "> ") || strings.HasPrefix(trim, "| ") {
			shown++
		}
	}
	above, more := 0, 0
	for _, line := range strings.Split(out, "\n") {
		var n int
		if _, err := fmt.Sscanf(strings.TrimSpace(line), "%d above", &n); err == nil {
			above = n
		}
		if _, err := fmt.Sscanf(strings.TrimSpace(line), "+%d more", &n); err == nil {
			more = n
		}
	}
	if above+shown+more != 20 {
		t.Fatalf("hidden rows dishonest: above=%d shown=%d more=%d (want 20)\n%s", above, shown, more, out)
	}
}

func TestGraphemeTruncateDoesNotSplitClusters(t *testing.T) {
	flag := "🇺🇸" // two regional indicators, one cluster
	got := TruncateDisplay("APP-1 "+flag+" title", 10)
	if strings.Contains(got, "\U0001F1FA") && !strings.Contains(got, flag) && !strings.HasSuffix(got, "...") {
		t.Fatalf("truncated through a flag cluster: %q", got)
	}
	combo := "e\u0301pique" // e + combining acute
	out := TruncateDisplay(combo, 3)
	if strings.HasPrefix(out, "e") && !strings.HasPrefix(out, "é") && !strings.HasPrefix(out, "e\u0301") && out != "..." {
		// either keep the cluster or drop it; never emit a combining mark first
		if strings.HasPrefix(out, "\u0301") {
			t.Fatalf("split combining mark: %q", out)
		}
	}
	wide := TruncateDisplay("表表表表", 5)
	if lipgloss.Width(wide) > 5 {
		t.Fatalf("wide glyphs overflowed: %q width=%d", wide, lipgloss.Width(wide))
	}
}

func TestWrapDisplayKeepsLongUnicode(t *testing.T) {
	lines := WrapDisplay("Fix 👩‍💻 login on 表表表 tokens", 12)
	if len(lines) < 2 {
		t.Fatalf("expected wrap, got %#v", lines)
	}
	joined := strings.Join(lines, "")
	if !strings.Contains(joined, "Fix") || !strings.Contains(joined, "login") {
		t.Fatalf("wrap dropped words: %#v", lines)
	}
	for _, line := range lines {
		if lipgloss.Width(line) > 12 {
			t.Fatalf("wrapped line too wide: %q", line)
		}
	}
}

func TestMarkdownAndAppHaveNoANSI(t *testing.T) {
	board := fixtureBoard()
	md := CompactBoardMarkdown(board)
	htmlDoc := CompactBoardAppHTML(board)
	if strings.ContainsRune(md, 0x1b) || strings.ContainsRune(htmlDoc, 0x1b) {
		t.Fatal("chat surfaces must not carry ANSI")
	}
	if !strings.Contains(md, "**APP-1**") || !strings.Contains(md, "## Blocked") {
		t.Fatalf("markdown lost status text:\n%s", md)
	}
	if strings.Contains(md, "%") {
		t.Fatalf("markdown invented a percentage:\n%s", md)
	}
}

func TestComfortableCardsPutTitleOnOwnLine(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	board := NewCompactBoard("DEMO", map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusReady: {{
			ID: "DEMO-1", Project: "DEMO", Title: "A calmer board at every width",
			Status: contracts.StatusReady, Priority: contracts.PriorityHigh,
		}},
		contracts.StatusInProgress: {{
			ID: "DEMO-3", Project: "DEMO", Title: "Keep local checkpoints automatic",
			Status: contracts.StatusInProgress, Priority: contracts.PriorityMedium,
		}},
	}, -1, nil)
	for _, width := range []int{40, 80, 120} {
		out := RenderBoard(board, BoardRenderOptions{Width: width, Style: BoardStyleKanban, Density: DensityComfortable, ShowSummary: true})
		if !outputHasID(out, "DEMO-1") || !outputHasID(out, "DEMO-3") {
			t.Fatalf("width %d dropped an id:\n%s", width, out)
		}
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, "DEMO-1") && strings.Contains(line, "calmer") {
				t.Fatalf("comfortable width %d still inlines title with id: %q\n%s", width, line, out)
			}
			if lipgloss.Width(line) > width {
				t.Fatalf("width %d overflow %d line=%q", width, lipgloss.Width(line), line)
			}
		}
		if strings.Contains(out, "DEMO ·") || strings.Contains(out, "medium") {
			t.Fatalf("scoped comfortable board should not repeat project/medium:\n%s", out)
		}
	}
}

func TestLaneCountFollowsUsefulWidth(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	board := NewCompactBoard("DEMO", map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusBacklog:    {{ID: "DEMO-6", Project: "DEMO", Title: "Plan the next release", Status: contracts.StatusBacklog}},
		contracts.StatusReady:      {{ID: "DEMO-1", Project: "DEMO", Title: "A calmer board at every width", Status: contracts.StatusReady, Priority: contracts.PriorityHigh}},
		contracts.StatusInProgress: {{ID: "DEMO-3", Project: "DEMO", Title: "Keep local checkpoints automatic", Status: contracts.StatusInProgress}},
		contracts.StatusInReview:   {{ID: "DEMO-7", Project: "DEMO", Title: "Welcome to Atlas Tasker", Status: contracts.StatusInReview}},
		contracts.StatusBlocked:    {{ID: "DEMO-5", Project: "DEMO", Title: "Handle an unavailable workspace", Status: contracts.StatusBlocked}},
		contracts.StatusCanceled:   {{ID: "DEMO-8", Project: "DEMO", Title: "Retired sample task", Status: contracts.StatusCanceled}},
	}, -1, nil)
	wide := RenderBoard(board, BoardRenderOptions{Width: 120, Style: BoardStyleKanban, Density: DensityComfortable, ShowSummary: true})
	header := firstLaneHeaderLine(wide)
	if strings.Contains(header, "Backlog") && strings.Contains(header, "In Review") {
		t.Fatalf("120-col comfortable board squeezed four lanes onto one row: %q\n%s", header, wide)
	}
	if strings.Contains(wide, "...|") || strings.Contains(wide, "...▏") {
		t.Fatalf("lane ellipsis collided with the next accent:\n%s", wide)
	}
	for _, line := range strings.Split(wide, "\n") {
		ids := 0
		for _, id := range []string{"DEMO-6", "DEMO-1", "DEMO-3", "DEMO-7"} {
			if strings.Contains(line, id) {
				ids++
			}
		}
		if ids > 3 {
			t.Fatalf("more than three ids on one 120-col line: %q\n%s", line, wide)
		}
	}
}

func TestLongIDWrapsInsteadOfEllipsis(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	id := "VERYLONGPROJECT-12345"
	board := NewCompactBoard("VERYLONGPROJECT", map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusReady: {{
			ID: id, Project: "VERYLONGPROJECT", Title: "Short",
			Status: contracts.StatusReady, Priority: contracts.PriorityHigh,
		}},
	}, -1, nil)
	out := RenderBoard(board, BoardRenderOptions{Width: 14, Style: BoardStyleKanban, Density: DensityComfortable, ShowSummary: false, SelectedID: id})
	if !outputHasID(out, id) {
		t.Fatalf("wrapped id missing characters:\n%s", out)
	}
	if strings.Contains(out, "VERYLONGPROJ...") || strings.Contains(out, "12345...") {
		t.Fatalf("id was ellipsized:\n%s", out)
	}
	if !strings.Contains(out, ">") {
		t.Fatalf("selected marker missing:\n%s", out)
	}
}

func TestBackupSummaryDistinguishesOptOutFromPending(t *testing.T) {
	off := BackupSignal{WorkerState: "pending", AutomaticEnabled: false, UnbackedEventCount: 10}
	got := off.SummaryLine()
	if strings.Contains(got, "pending") && !strings.Contains(got, "local checkpoints off") {
		t.Fatalf("opted-out backup should not read as a running pending job: %q", got)
	}
	if !strings.Contains(got, "local checkpoints off") || !strings.Contains(got, "10 unbacked events") {
		t.Fatalf("opted-out backup line = %q", got)
	}
	local := BackupSignal{AutomaticEnabled: true, LastLocalCheckpointID: "cp-1", WorkerState: "checkpoint_created"}
	if !strings.Contains(local.SummaryLine(), "local checkpoint present") {
		t.Fatalf("local checkpoint line = %q", local.SummaryLine())
	}
	remote := BackupSignal{AutomaticEnabled: true, LastLocalCheckpointID: "cp-1", VerifiedRemote: true}
	if !strings.Contains(remote.SummaryLine(), "remote verified") {
		t.Fatalf("verified remote line = %q", remote.SummaryLine())
	}
	unverified := BackupSignal{AutomaticEnabled: true, LastRemoteCheckpointID: "r1", VerifiedRemote: false}
	if strings.Contains(unverified.SummaryLine(), "remote verified") {
		t.Fatalf("invented remote verification: %q", unverified.SummaryLine())
	}
}

func TestNextActionsLeadWithReadyWork(t *testing.T) {
	board := NewCompactBoard("DEMO", map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusReady:   {{ID: "DEMO-1", Project: "DEMO", Title: "Ready", Status: contracts.StatusReady}},
		contracts.StatusBlocked: {{ID: "DEMO-5", Project: "DEMO", Title: "Blocked", Status: contracts.StatusBlocked}},
	}, -1, nil)
	if len(board.NextActions) == 0 || !strings.Contains(board.NextActions[0], "DEMO-1") {
		t.Fatalf("ready work should lead next actions: %#v", board.NextActions)
	}
	if strings.Contains(board.NextActions[0], "DEMO-5") {
		t.Fatalf("blocked caution should not lead when ready work exists: %#v", board.NextActions)
	}
	if strings.Contains(board.NextActions[0], "claim") {
		t.Fatalf("must not promise claim permission: %#v", board.NextActions)
	}
}

func TestMixedProjectKeepsTags(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	board := NewCompactBoard("", map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusReady: {
			{ID: "APP-1", Project: "APP", Title: "App work", Status: contracts.StatusReady},
			{ID: "OPS-1", Project: "OPS", Title: "Ops work", Status: contracts.StatusReady},
		},
	}, -1, nil)
	out := RenderBoard(board, BoardRenderOptions{Width: 80, Style: BoardStyleKanban, ShowSummary: true})
	if strings.Contains(out, "Board APP") || strings.Contains(out, "Board OPS") {
		t.Fatalf("mixed board should not pretend to be one project:\n%s", out)
	}
	if !strings.Contains(out, "APP") || !strings.Contains(out, "OPS") {
		t.Fatalf("mixed board should tag cards with project keys:\n%s", out)
	}
}

func TestFocusDensityShowsSelectedLane(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	out := RenderBoard(fixtureBoard(), BoardRenderOptions{
		Width: 80, Style: BoardStyleModern, Density: DensityFocus, SelectedID: "APP-9", ShowSummary: true,
	})
	if !strings.Contains(out, "APP-9") || !strings.Contains(out, "Blocked") {
		t.Fatalf("focus density should keep the selected blocked card:\n%s", out)
	}
}

func outputHasID(out, id string) bool {
	if strings.Contains(out, id) {
		return true
	}
	compact := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r == ' ' {
			return -1
		}
		return r
	}, out)
	return strings.Contains(compact, strings.ReplaceAll(id, " ", ""))
}

func firstLaneHeaderLine(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Backlog") || strings.Contains(line, "Ready") {
			return line
		}
	}
	return ""
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func TestSanitizeStillStripsOSC(t *testing.T) {
	out := TruncateDisplay("APP-1 \x1b]0;evil\x07title", 40)
	for _, r := range out {
		if r == 0x1b || r == 0x07 || unicode.IsControl(r) && r != '\n' {
			t.Fatalf("control leftover %q in %q", r, out)
		}
	}
}
