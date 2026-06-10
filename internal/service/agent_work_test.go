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

func setupAgentWorkTest(t *testing.T) (context.Context, *QueryService, mdstore.TicketStore, AgentStore, RunStore, time.Time, func()) {
	t.Helper()
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	projects := mdstore.ProjectStore{RootDir: root}
	tickets := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	events := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), tickets, events)
	if err != nil {
		t.Fatalf("open projection: %v", err)
	}
	if err := projects.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	queries := NewQueryService(root, projects, tickets, events, projection, func() time.Time { return now })
	return ctx, queries, tickets, AgentStore{Root: root}, RunStore{Root: root}, now, func() { _ = projection.Close() }
}

func TestAgentWorkClassifiesAvailableAndPending(t *testing.T) {
	ctx, queries, tickets, agents, _, now, cleanup := setupAgentWorkTest(t)
	defer cleanup()

	if err := agents.SaveAgent(ctx, contracts.AgentProfile{AgentID: "builder-1", DisplayName: "Builder", Provider: contracts.AgentProviderCodex, Enabled: true}); err != nil {
		t.Fatalf("save agent: %v", err)
	}
	blocker := testAgentWorkTicket("APP-1", "Blocker", contracts.StatusInProgress, now)
	blocked := testAgentWorkTicket("APP-2", "Blocked work", contracts.StatusReady, now)
	blocked.Assignee = contracts.Actor("agent:builder-1")
	blocked.BlockedBy = []string{blocker.ID}
	ready := testAgentWorkTicket("APP-3", "Ready work", contracts.StatusReady, now)
	ready.Assignee = contracts.Actor("agent:builder-1")
	for _, ticket := range []contracts.TicketSnapshot{blocker, blocked, ready} {
		if err := tickets.CreateTicket(ctx, ticket); err != nil {
			t.Fatalf("create ticket %s: %v", ticket.ID, err)
		}
	}

	view, err := queries.AgentWork(ctx, contracts.Actor("agent:builder-1"))
	if err != nil {
		t.Fatalf("agent work: %v", err)
	}
	if len(view.Available) != 1 || view.Available[0].Ticket.ID != "APP-3" {
		t.Fatalf("expected APP-3 available, got %#v", view.Available)
	}
	if len(view.Pending) != 1 || view.Pending[0].Ticket.ID != "APP-2" {
		t.Fatalf("expected APP-2 pending, got %#v", view.Pending)
	}
	if got := view.Pending[0].ReasonCodes; len(got) != 1 || got[0] != AgentWorkReasonDependencyBlocked {
		t.Fatalf("expected dependency_blocked, got %#v", got)
	}

	blocker.Status = contracts.StatusDone
	if err := tickets.UpdateTicket(ctx, blocker); err != nil {
		t.Fatalf("mark blocker done: %v", err)
	}
	view, err = queries.AgentWork(ctx, contracts.Actor("agent:builder-1"))
	if err != nil {
		t.Fatalf("agent work after unblock: %v", err)
	}
	if len(view.Available) != 2 {
		t.Fatalf("expected both tickets available after blocker done, got %#v", view.Available)
	}
}

func TestAgentPendingIncludesReviewWorkForNonReviewer(t *testing.T) {
	ctx, queries, tickets, agents, _, now, cleanup := setupAgentWorkTest(t)
	defer cleanup()

	if err := agents.SaveAgent(ctx, contracts.AgentProfile{AgentID: "builder-1", DisplayName: "Builder", Provider: contracts.AgentProviderCodex, Enabled: true}); err != nil {
		t.Fatalf("save builder: %v", err)
	}
	if err := agents.SaveAgent(ctx, contracts.AgentProfile{AgentID: "reviewer-1", DisplayName: "Reviewer", Provider: contracts.AgentProviderClaude, Enabled: true}); err != nil {
		t.Fatalf("save reviewer: %v", err)
	}
	ticket := testAgentWorkTicket("APP-1", "Review me", contracts.StatusInReview, now)
	ticket.Assignee = contracts.Actor("agent:builder-1")
	ticket.Reviewer = contracts.Actor("agent:reviewer-1")
	if err := tickets.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	builder, err := queries.AgentPending(ctx, contracts.Actor("agent:builder-1"))
	if err != nil {
		t.Fatalf("builder pending: %v", err)
	}
	if len(builder.Pending) != 1 || builder.Pending[0].ReasonCodes[0] != AgentWorkReasonWaitingReview {
		t.Fatalf("expected builder waiting on review, got %#v", builder.Pending)
	}
	reviewer, err := queries.AgentAvailable(ctx, contracts.Actor("agent:reviewer-1"))
	if err != nil {
		t.Fatalf("reviewer available: %v", err)
	}
	if len(reviewer.Available) != 1 || reviewer.Available[0].Action != "review" {
		t.Fatalf("expected reviewer available review action, got %#v", reviewer.Available)
	}
}

func testAgentWorkTicket(id string, title string, status contracts.Status, now time.Time) contracts.TicketSnapshot {
	return contracts.TicketSnapshot{
		ID:            id,
		Project:       "APP",
		Title:         title,
		Type:          contracts.TicketTypeTask,
		Status:        status,
		Priority:      contracts.PriorityMedium,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
}
