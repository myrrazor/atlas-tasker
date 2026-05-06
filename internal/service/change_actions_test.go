package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

func TestChangeLinkRecordAndUnlinkRefreshesTicketReadiness(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 26, 13, 0, 0, 0, time.UTC)
	clock := now

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return clock }}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer projection.Close()

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	actions := NewActionService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return clock }, FileLockManager{Root: root}, nil, nil)
	queries := NewQueryService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return clock })

	ticket, err := actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "Ship the slice",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed ticket")
	if err != nil {
		t.Fatalf("create tracked ticket: %v", err)
	}

	clock = clock.Add(time.Minute)
	change, err := actions.LinkChange(ctx, ticket.ID, contracts.ChangeRef{
		Provider:      contracts.ChangeProviderLocal,
		Status:        contracts.ChangeStatusApproved,
		BranchName:    "feat/app-1",
		BaseBranch:    "main",
		ReviewSummary: "Looks ready once checks pass.",
	}, contracts.Actor("human:owner"), "link change")
	if err != nil {
		t.Fatalf("link change: %v", err)
	}

	detail, err := queries.TicketDetail(ctx, ticket.ID)
	if err != nil {
		t.Fatalf("ticket detail after link: %v", err)
	}
	if detail.Ticket.ChangeReadyState != contracts.ChangeReadyChecksPending {
		t.Fatalf("expected checks_pending after linking approved change without checks, got %s", detail.Ticket.ChangeReadyState)
	}
	if len(detail.Changes) != 1 || detail.Changes[0].ChangeID != change.ChangeID {
		t.Fatalf("unexpected change detail: %#v", detail.Changes)
	}

	clock = clock.Add(time.Minute)
	check, err := actions.RecordCheck(ctx, contracts.CheckResult{
		Scope:      contracts.CheckScopeChange,
		ScopeID:    change.ChangeID,
		Name:       "unit",
		Status:     contracts.CheckStatusCompleted,
		Conclusion: contracts.CheckConclusionSuccess,
	}, contracts.Actor("human:owner"), "record success")
	if err != nil {
		t.Fatalf("record check: %v", err)
	}
	if check.CheckID == "" {
		t.Fatalf("expected generated check id")
	}

	detail, err = queries.TicketDetail(ctx, ticket.ID)
	if err != nil {
		t.Fatalf("ticket detail after check: %v", err)
	}
	if detail.Ticket.ChangeReadyState != contracts.ChangeReadyMergeReady {
		t.Fatalf("expected merge_ready after passing checks, got %s", detail.Ticket.ChangeReadyState)
	}
	if len(detail.Checks) != 1 || detail.Checks[0].CheckID != check.CheckID {
		t.Fatalf("unexpected check detail: %#v", detail.Checks)
	}

	clock = clock.Add(time.Minute)
	updatedTicket, _, err := actions.UnlinkChange(ctx, ticket.ID, change.ChangeID, contracts.Actor("human:owner"), "unlink change")
	if err != nil {
		t.Fatalf("unlink change: %v", err)
	}
	if updatedTicket.ChangeReadyState != contracts.ChangeReadyNoLinkedChange {
		t.Fatalf("expected no_linked_change after unlink, got %s", updatedTicket.ChangeReadyState)
	}

	detail, err = queries.TicketDetail(ctx, ticket.ID)
	if err != nil {
		t.Fatalf("ticket detail after unlink: %v", err)
	}
	if detail.Ticket.ChangeReadyState != contracts.ChangeReadyNoLinkedChange || len(detail.Changes) != 0 {
		t.Fatalf("unexpected post-unlink detail: %#v", detail)
	}
}

func TestAggregateChecksKeepsPendingAheadOfFailing(t *testing.T) {
	checks := []contracts.CheckResult{
		{
			CheckID:    "check_done",
			Scope:      contracts.CheckScopeChange,
			ScopeID:    "change_123",
			Name:       "unit",
			Status:     contracts.CheckStatusCompleted,
			Conclusion: contracts.CheckConclusionFailure,
		},
		{
			CheckID:    "check_waiting",
			Scope:      contracts.CheckScopeChange,
			ScopeID:    "change_123",
			Name:       "integration",
			Status:     contracts.CheckStatusRunning,
			Conclusion: contracts.CheckConclusionUnknown,
		},
	}

	if got := aggregateChecks(checks); got != contracts.CheckAggregatePending {
		t.Fatalf("expected pending when checks are mixed pending+failing, got %s", got)
	}
}
