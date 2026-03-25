package service

import (
	"context"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestRunStoreRoundTrip(t *testing.T) {
	root := t.TempDir()
	store := RunStore{Root: root}
	now := time.Date(2026, 3, 25, 18, 0, 0, 0, time.UTC)
	run := contracts.RunSnapshot{
		RunID:        "run_abc123",
		TicketID:     "APP-1",
		Project:      "APP",
		AgentID:      "builder-1",
		Provider:     contracts.AgentProviderCodex,
		Status:       contracts.RunStatusDispatched,
		Kind:         contracts.RunKindWork,
		WorktreePath: "/tmp/worktree",
		BranchName:   "run/app-1-run_abc123",
		CreatedAt:    now,
		Summary:      "Initial dispatch",
	}
	if err := store.SaveRun(context.Background(), run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	loaded, err := store.LoadRun(context.Background(), run.RunID)
	if err != nil {
		t.Fatalf("load run: %v", err)
	}
	if loaded.RunID != run.RunID || loaded.TicketID != run.TicketID || loaded.WorktreePath != run.WorktreePath {
		t.Fatalf("unexpected loaded run: %#v", loaded)
	}
	items, err := store.ListRuns(context.Background(), "APP-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(items) != 1 || items[0].RunID != run.RunID {
		t.Fatalf("unexpected run list: %#v", items)
	}
}
