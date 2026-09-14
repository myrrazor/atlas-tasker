package render

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

// Writes deterministic fixture snapshots for Codex visual inspection.
// This is fixture data, not a live workspace.
func TestWritePresentationSnapshots(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	dir := os.Getenv("ATLAS_PRESENTATION_EVIDENCE")
	if dir == "" {
		dir = t.TempDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("evidence dir unavailable: %v", err)
	}

	now := time.Date(2026, 9, 11, 15, 0, 0, 0, time.UTC)
	unicodeBoard := NewCompactBoard("APP", map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusReady: {{
			ID: "APP-1", Project: "APP", Title: "Fix 👩‍💻 login on 表表表 🇺🇸 tokens",
			Status: contracts.StatusReady, Priority: contracts.PriorityHigh,
			Assignee: "agent:builder-1", Labels: []string{"auth"}, UpdatedAt: now,
		}},
		contracts.StatusBlocked: {{
			ID: "APP-9", Project: "APP", Title: "Waiting on API decision",
			Status: contracts.StatusBlocked, Priority: contracts.PriorityCritical,
			Assignee: "agent:builder-1", BlockedBy: []string{"APP-1"}, UpdatedAt: now,
		}},
	}, -1, nil)
	AttachBackup(&unicodeBoard, BackupSignal{Configured: true, SnapshotCount: 4, AutomaticEnabled: true, LastLocalCheckpointID: "cp-1", WorkerState: "idle"})

	empty := NewCompactBoard("APP", nil, -1, nil)
	manyCols := map[contracts.Status][]contracts.TicketSnapshot{}
	for i := 1; i <= 24; i++ {
		manyCols[contracts.StatusReady] = append(manyCols[contracts.StatusReady], contracts.TicketSnapshot{
			ID: "APP-" + itoa(i), Title: "Load card " + itoa(i), Status: contracts.StatusReady, UpdatedAt: now,
		})
	}
	many := NewCompactBoard("APP", manyCols, -1, nil)

	write := func(name, body string) {
		t.Helper()
		header := "FIXTURE DATA — not a live Atlas workspace\n\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(header+body+"\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	demo := NewCompactBoard("DEMO", map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusBacklog:    {{ID: "DEMO-6", Project: "DEMO", Title: "Plan the next release", Status: contracts.StatusBacklog, Priority: contracts.PriorityLow, Labels: []string{"planning"}, UpdatedAt: now}},
		contracts.StatusReady:      {{ID: "DEMO-1", Project: "DEMO", Title: "A calmer board at every width", Status: contracts.StatusReady, Priority: contracts.PriorityHigh, Labels: []string{"interface"}, UpdatedAt: now}},
		contracts.StatusInProgress: {{ID: "DEMO-3", Project: "DEMO", Title: "Keep local checkpoints automatic", Status: contracts.StatusInProgress, Labels: []string{"backup"}, UpdatedAt: now}},
		contracts.StatusInReview:   {{ID: "DEMO-7", Project: "DEMO", Title: "Welcome to Atlas Tasker", Status: contracts.StatusInReview, UpdatedAt: now}},
		contracts.StatusBlocked:    {{ID: "DEMO-5", Project: "DEMO", Title: "Handle an unavailable workspace", Status: contracts.StatusBlocked, Labels: []string{"workspace"}, UpdatedAt: now}},
		contracts.StatusCanceled:   {{ID: "DEMO-8", Project: "DEMO", Title: "Retired sample task", Status: contracts.StatusCanceled, UpdatedAt: now}},
	}, -1, nil)
	AttachBackup(&demo, BackupSignal{WorkerState: "pending", AutomaticEnabled: false, UnbackedEventCount: 10})

	write("board-narrow-40.txt", RenderBoard(unicodeBoard, BoardRenderOptions{Width: 40, ShowSummary: true}))
	write("board-medium-80.txt", RenderBoard(unicodeBoard, BoardRenderOptions{Width: 80, ShowSummary: true}))
	write("board-wide-120.txt", RenderBoard(unicodeBoard, BoardRenderOptions{Width: 120, ShowSummary: true}))
	write("board-demo-40.txt", RenderBoard(demo, BoardRenderOptions{Width: 40, ShowSummary: true}))
	write("board-demo-80.txt", RenderBoard(demo, BoardRenderOptions{Width: 80, ShowSummary: true}))
	write("board-demo-120.txt", RenderBoard(demo, BoardRenderOptions{Width: 120, ShowSummary: true}))
	write("board-demo-180.txt", RenderBoard(demo, BoardRenderOptions{Width: 180, ShowSummary: true}))
	write("board-empty.txt", RenderBoard(empty, BoardRenderOptions{Width: 80, ShowSummary: true}))
	write("board-many-tickets.txt", RenderBoard(many, BoardRenderOptions{Width: 80, Height: 20, ShowSummary: true, SelectedID: "APP-1"}))
	write("board-monochrome.txt", RenderBoard(unicodeBoard, BoardRenderOptions{Width: 80, ShowSummary: true}))
	colorOn := true
	write("board-color-80.txt", RenderBoard(unicodeBoard, BoardRenderOptions{Width: 80, ShowSummary: true, Color: &colorOn}))
	write("board-table-80.txt", BoardTableWithWidth(contracts.BoardView{Columns: map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusReady: {{ID: "APP-1", Status: contracts.StatusReady, Priority: contracts.PriorityHigh, Title: "Fix login", Assignee: "agent:builder-1"}},
	}}, 80))
	write("board-markdown.md", CompactBoardMarkdown(unicodeBoard))
	write("board-app.html", CompactBoardAppHTML(unicodeBoard))
}
