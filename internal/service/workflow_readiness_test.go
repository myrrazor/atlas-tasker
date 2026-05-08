package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/domain"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

func TestActionServiceDependenciesBlockUnsafeWorkflowTransitions(t *testing.T) {
	ctx, actions := newWorkflowReadinessActions(t)

	blocker := createWorkflowTicket(t, ctx, actions, contracts.TicketSnapshot{
		Project:  "APP",
		Title:    "Blocker",
		Type:     contracts.TicketTypeTask,
		Status:   contracts.StatusInProgress,
		Priority: contracts.PriorityMedium,
	})
	requestReview := createWorkflowTicket(t, ctx, actions, contracts.TicketSnapshot{
		Project:  "APP",
		Title:    "Request review blocked",
		Type:     contracts.TicketTypeTask,
		Status:   contracts.StatusInProgress,
		Priority: contracts.PriorityMedium,
		Reviewer: contracts.Actor("agent:reviewer-1"),
	})
	approve := createWorkflowTicket(t, ctx, actions, contracts.TicketSnapshot{
		Project:     "APP",
		Title:       "Approve blocked",
		Type:        contracts.TicketTypeTask,
		Status:      contracts.StatusInReview,
		Priority:    contracts.PriorityMedium,
		Reviewer:    contracts.Actor("agent:reviewer-1"),
		ReviewState: contracts.ReviewStatePending,
	})
	complete := createWorkflowTicket(t, ctx, actions, contracts.TicketSnapshot{
		Project:     "APP",
		Title:       "Complete blocked",
		Type:        contracts.TicketTypeTask,
		Status:      contracts.StatusInReview,
		Priority:    contracts.PriorityMedium,
		Reviewer:    contracts.Actor("agent:reviewer-1"),
		ReviewState: contracts.ReviewStateApproved,
	})
	for _, ticketID := range []string{requestReview.ID, approve.ID, complete.ID} {
		if _, err := actions.LinkTickets(ctx, ticketID, blocker.ID, domain.LinkBlockedBy, contracts.Actor("human:owner"), "depends on blocker"); err != nil {
			t.Fatalf("link %s to blocker: %v", ticketID, err)
		}
	}

	if _, err := actions.RequestReviewWithReviewer(ctx, requestReview.ID, "", contracts.Actor("agent:builder-1"), "ready"); err == nil || !strings.Contains(err.Error(), "dependency_blocked") {
		t.Fatalf("expected request-review dependency block, got %v", err)
	}
	if _, err := actions.ApproveTicket(ctx, approve.ID, contracts.Actor("agent:reviewer-1"), "reviewed"); err == nil || !strings.Contains(err.Error(), "dependency_blocked") {
		t.Fatalf("expected approve dependency block, got %v", err)
	}
	if _, err := actions.CompleteTicket(ctx, complete.ID, contracts.Actor("agent:reviewer-1"), "complete"); err == nil || !strings.Contains(err.Error(), "dependency_blocked") {
		t.Fatalf("expected complete dependency block, got %v", err)
	}

	loadedBlocker, err := actions.Tickets.GetTicket(ctx, blocker.ID)
	if err != nil {
		t.Fatalf("load blocker: %v", err)
	}
	loadedBlocker.Status = contracts.StatusDone
	if _, err := actions.SaveTrackedTicket(ctx, loadedBlocker, contracts.Actor("human:owner"), "blocker done"); err != nil {
		t.Fatalf("mark blocker done: %v", err)
	}
	if _, err := actions.RequestReviewWithReviewer(ctx, requestReview.ID, "", contracts.Actor("agent:builder-1"), "ready after unblock"); err != nil {
		t.Fatalf("request-review should pass after blocker is done: %v", err)
	}
}

func TestActionServiceRejectsAssigneeReviewerSelfApproval(t *testing.T) {
	ctx, actions := newWorkflowReadinessActions(t)
	ticket := createWorkflowTicket(t, ctx, actions, contracts.TicketSnapshot{
		Project:   "APP",
		Title:     "Self approval",
		Type:      contracts.TicketTypeTask,
		Status:    contracts.StatusInProgress,
		Priority:  contracts.PriorityMedium,
		Assignee:  contracts.Actor("agent:builder-1"),
		Reviewer:  contracts.Actor("agent:builder-1"),
		CreatedAt: time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC),
	})
	if _, err := actions.RequestReviewWithReviewer(ctx, ticket.ID, "", contracts.Actor("agent:builder-1"), "ready"); err != nil {
		t.Fatalf("request review: %v", err)
	}
	if _, err := actions.ApproveTicket(ctx, ticket.ID, contracts.Actor("agent:builder-1"), "approve myself"); err == nil || !strings.Contains(err.Error(), "self_approval_denied") {
		t.Fatalf("expected self approval denial, got %v", err)
	}
}

func newWorkflowReadinessActions(t *testing.T) (context.Context, *ActionService) {
	t.Helper()
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = projection.Close() })
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	return ctx, NewActionService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now }, FileLockManager{Root: root}, nil, nil)
}

func createWorkflowTicket(t *testing.T, ctx context.Context, actions *ActionService, ticket contracts.TicketSnapshot) contracts.TicketSnapshot {
	t.Helper()
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	if ticket.CreatedAt.IsZero() {
		ticket.CreatedAt = now
	}
	if ticket.UpdatedAt.IsZero() {
		ticket.UpdatedAt = ticket.CreatedAt
	}
	if ticket.SchemaVersion == 0 {
		ticket.SchemaVersion = contracts.CurrentSchemaVersion
	}
	created, err := actions.CreateTrackedTicket(ctx, ticket, contracts.Actor("human:owner"), "create test ticket")
	if err != nil {
		t.Fatalf("create ticket %q: %v", ticket.Title, err)
	}
	return created
}
