package cli

import (
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

func TestFormatBoardModernVsTable(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("COLUMNS", "80")
	board := contracts.BoardView{Columns: map[contracts.Status][]contracts.TicketSnapshot{
		contracts.StatusReady: {{
			ID: "APP-1", Title: "Ready work", Status: contracts.StatusReady,
			Priority: contracts.PriorityHigh, Assignee: "agent:builder-1",
		}},
	}}
	modern, err := formatBoard(board, "APP", "modern", "comfortable", service.BackupHealthSummary{Configured: true, SnapshotCount: 2, AutomaticEnabled: true, LastLocalCheckpointID: "cp-1"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(modern, "+---") || !strings.Contains(modern, "APP-1") || !strings.Contains(modern, "Ready") {
		t.Fatalf("modern board:\n%s", modern)
	}
	if !strings.Contains(modern, "Backup") || !strings.Contains(modern, "2 snapshots") {
		t.Fatalf("modern board should use real backup data:\n%s", modern)
	}
	if !strings.Contains(modern, "Board APP") {
		t.Fatalf("filtered board should name the project:\n%s", modern)
	}
	if strings.Contains(modern, "APP ·") {
		t.Fatalf("scoped board should not repeat the project on every card:\n%s", modern)
	}
	table, err := formatBoard(board, "APP", "table", "comfortable", service.BackupHealthSummary{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(table, "+---") || strings.Contains(table, "Column") {
		t.Fatalf("table style should be the polished row board, not the legacy grid:\n%s", table)
	}
	htmlBoard, err := formatBoard(board, "APP", "html", "comfortable", service.BackupHealthSummary{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(htmlBoard, "<style>") || !strings.Contains(htmlBoard, `<details class="card">`) || !strings.Contains(htmlBoard, "APP-1") {
		t.Fatalf("html style should be a self-contained board fragment: %s", htmlBoard)
	}
	markdownBoard, err := formatBoard(board, "APP", "markdown", "comfortable", service.BackupHealthSummary{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(markdownBoard, "## Ready (1)") || !strings.Contains(markdownBoard, "**APP-1**") || strings.Contains(markdownBoard, "\x1b[") {
		t.Fatalf("markdown style should be a readable ANSI-free chat board: %s", markdownBoard)
	}
	if !strings.Contains(table, "APP-1") || !strings.Contains(table, "Ready") || !strings.Contains(table, "ID") {
		t.Fatalf("table style:\n%s", table)
	}
	legacy, err := formatBoard(board, "APP", "legacy", "comfortable", service.BackupHealthSummary{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(legacy, "+") || !strings.Contains(legacy, "Column") {
		t.Fatalf("legacy style should keep the old grid:\n%s", legacy)
	}
	if _, err := formatBoard(board, "", "neon", "comfortable", service.BackupHealthSummary{}, false); err == nil {
		t.Fatal("expected invalid style error")
	}
	if _, err := formatBoard(board, "", "modern", "chunky", service.BackupHealthSummary{}, false); err == nil {
		t.Fatal("expected invalid density error")
	}
}
