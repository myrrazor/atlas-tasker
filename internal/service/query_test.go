package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

func TestQueryServiceEffectivePolicyAndQueue(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 23, 1, 0, 0, 0, time.UTC)

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer projection.Close()

	if err := config.Save(root, contracts.TrackerConfig{
		Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOwnerGate},
		Actor:    contracts.ActorConfig{Default: contracts.Actor("agent:builder-1")},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	project := contracts.Project{
		Key:       "APP",
		Name:      "App",
		CreatedAt: now,
		Defaults: contracts.ProjectDefaults{
			CompletionMode:   contracts.CompletionModeReviewGate,
			LeaseTTLMinutes:  90,
			AllowedWorkers:   []contracts.Actor{"agent:builder-1"},
			RequiredReviewer: contracts.Actor("agent:reviewer-1"),
		},
	}
	if err := projectStore.CreateProject(ctx, project); err != nil {
		t.Fatalf("create project: %v", err)
	}

	epic := contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Epic",
		Type:          contracts.TicketTypeEpic,
		Status:        contracts.StatusInProgress,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
		Policy: contracts.TicketPolicy{
			CompletionMode: contracts.CompletionModeDualGate,
			AllowedWorkers: []contracts.Actor{"agent:builder-1", "agent:builder-2"},
		},
	}
	child := contracts.TicketSnapshot{
		ID:            "APP-2",
		Project:       "APP",
		Title:         "Child",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusInReview,
		Priority:      contracts.PriorityCritical,
		Parent:        epic.ID,
		Reviewer:      contracts.Actor("agent:reviewer-1"),
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
		ReviewState:   contracts.ReviewStateApproved,
		Policy: contracts.TicketPolicy{
			RequiredReviewer: contracts.Actor("human:owner"),
		},
	}
	stale := contracts.TicketSnapshot{
		ID:            "APP-3",
		Project:       "APP",
		Title:         "Stale",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityMedium,
		CreatedAt:     now,
		UpdatedAt:     now.Add(-2 * time.Hour),
		SchemaVersion: contracts.CurrentSchemaVersion,
		Lease: contracts.LeaseState{
			Actor:      contracts.Actor("agent:builder-2"),
			Kind:       contracts.LeaseKindWork,
			AcquiredAt: now.Add(-2 * time.Hour),
			ExpiresAt:  now.Add(-90 * time.Minute),
		},
	}
	for index, ticket := range []contracts.TicketSnapshot{epic, child, stale} {
		if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
			t.Fatalf("create ticket: %v", err)
		}
		event := contracts.Event{
			EventID:       int64(index + 1),
			Timestamp:     ticket.UpdatedAt,
			Actor:         contracts.Actor("human:owner"),
			Type:          contracts.EventTicketCreated,
			Project:       ticket.Project,
			TicketID:      ticket.ID,
			Payload:       ticket,
			SchemaVersion: contracts.CurrentSchemaVersion,
		}
		if err := eventsLog.AppendEvent(ctx, event); err != nil {
			t.Fatalf("append event: %v", err)
		}
		if err := projection.ApplyEvent(ctx, event); err != nil {
			t.Fatalf("apply event: %v", err)
		}
	}

	queries := NewQueryService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now })
	policy, err := queries.EffectivePolicy(ctx, child)
	if err != nil {
		t.Fatalf("effective policy: %v", err)
	}
	if policy.CompletionMode != contracts.CompletionModeDualGate {
		t.Fatalf("unexpected completion mode: %s", policy.CompletionMode)
	}
	if policy.RequiredReviewer != contracts.Actor("human:owner") {
		t.Fatalf("unexpected required reviewer: %s", policy.RequiredReviewer)
	}
	if len(policy.Sources) != 4 {
		t.Fatalf("expected full precedence chain, got %#v", policy.Sources)
	}

	reviewerQueue, err := queries.Queue(ctx, contracts.Actor("agent:reviewer-1"))
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if len(reviewerQueue.Categories[QueueNeedsReview]) != 1 || reviewerQueue.Categories[QueueNeedsReview][0].Ticket.ID != child.ID {
		t.Fatalf("needs_review mismatch: %#v", reviewerQueue.Categories[QueueNeedsReview])
	}
	ownerQueue, err := queries.Queue(ctx, contracts.Actor("human:owner"))
	if err != nil {
		t.Fatalf("owner queue: %v", err)
	}
	if len(ownerQueue.Categories[QueueAwaitingOwner]) != 1 || ownerQueue.Categories[QueueAwaitingOwner][0].Ticket.ID != child.ID {
		t.Fatalf("awaiting_owner mismatch: %#v", ownerQueue.Categories[QueueAwaitingOwner])
	}
	builderQueue, err := queries.Queue(ctx, "")
	if err != nil {
		t.Fatalf("builder queue: %v", err)
	}
	if len(builderQueue.Categories[QueueStaleClaims]) != 1 || builderQueue.Categories[QueueStaleClaims][0].Ticket.ID != stale.ID {
		t.Fatalf("stale_claims mismatch: %#v", builderQueue.Categories[QueueStaleClaims])
	}
	if len(reviewerQueue.Categories[QueuePolicyViolations]) == 0 {
		t.Fatalf("expected reviewer queue to surface policy violations")
	}
}

func TestQueryServiceTicketDetailUsesProjection(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 23, 2, 0, 0, 0, time.UTC)

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer projection.Close()

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket := contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Detail",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityMedium,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	for _, event := range []contracts.Event{
		{EventID: 1, Timestamp: now, Actor: contracts.Actor("human:owner"), Type: contracts.EventTicketCreated, Project: "APP", TicketID: ticket.ID, Payload: ticket, SchemaVersion: contracts.CurrentSchemaVersion},
		{EventID: 2, Timestamp: now.Add(time.Minute), Actor: contracts.Actor("agent:builder-1"), Type: contracts.EventTicketCommented, Project: "APP", TicketID: ticket.ID, Payload: map[string]any{"body": "first comment"}, SchemaVersion: contracts.CurrentSchemaVersion},
	} {
		if err := eventsLog.AppendEvent(ctx, event); err != nil {
			t.Fatalf("append event: %v", err)
		}
		if err := projection.ApplyEvent(ctx, event); err != nil {
			t.Fatalf("apply event: %v", err)
		}
	}

	queries := NewQueryService(root, projectStore, ticketStore, eventsLog, projection, nil)
	detail, err := queries.TicketDetail(ctx, ticket.ID)
	if err != nil {
		t.Fatalf("ticket detail: %v", err)
	}
	if detail.Ticket.ID != ticket.ID {
		t.Fatalf("unexpected ticket detail: %#v", detail.Ticket)
	}
	if len(detail.Comments) != 1 || detail.Comments[0] != "first comment" {
		t.Fatalf("unexpected comments: %#v", detail.Comments)
	}
}

func TestQueryServiceInspectIncludesQueueCategories(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 23, 3, 0, 0, 0, time.UTC)

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer projection.Close()

	if err := config.Save(root, contracts.TrackerConfig{
		Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen},
		Actor:    contracts.ActorConfig{Default: contracts.Actor("agent:reviewer-1")},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := projectStore.CreateProject(ctx, contracts.Project{
		Key:       "APP",
		Name:      "App",
		CreatedAt: now,
		Defaults:  contracts.ProjectDefaults{CompletionMode: contracts.CompletionModeDualGate},
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket := contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Inspect",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusInReview,
		Priority:      contracts.PriorityHigh,
		Reviewer:      contracts.Actor("agent:reviewer-1"),
		ReviewState:   contracts.ReviewStateApproved,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	event := contracts.Event{
		EventID:       1,
		Timestamp:     now,
		Actor:         contracts.Actor("human:owner"),
		Type:          contracts.EventTicketCreated,
		Project:       "APP",
		TicketID:      ticket.ID,
		Payload:       ticket,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := eventsLog.AppendEvent(ctx, event); err != nil {
		t.Fatalf("append event: %v", err)
	}
	if err := projection.ApplyEvent(ctx, event); err != nil {
		t.Fatalf("apply event: %v", err)
	}

	queries := NewQueryService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now })
	view, err := queries.InspectTicket(ctx, ticket.ID, contracts.Actor("human:owner"))
	if err != nil {
		t.Fatalf("inspect ticket: %v", err)
	}
	if view.BoardStatus != contracts.StatusInReview {
		t.Fatalf("unexpected board status: %s", view.BoardStatus)
	}
	if len(view.QueueCategories) == 0 || view.QueueCategories[0] != QueueAwaitingOwner {
		t.Fatalf("unexpected queue categories: %#v", view.QueueCategories)
	}
}

func TestQueryServiceTemplateNextAndProgress(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 23, 5, 0, 0, 0, time.UTC)

	if err := os.MkdirAll(filepath.Join(storage.TrackerDir(root), "templates"), 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	if err := os.WriteFile(filepath.Join(storage.TrackerDir(root), "templates", "design.md"), []byte(`---
type: task
labels: [design]
blueprint: design
skill_hint: design
---
# Summary

## Description

Map the UX.

## Acceptance Criteria
- Wireframe exists
`), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer projection.Close()

	if err := config.Save(root, contracts.TrackerConfig{
		Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen},
		Actor:    contracts.ActorConfig{Default: contracts.Actor("agent:builder-1")},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	epic := contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Epic",
		Type:          contracts.TicketTypeEpic,
		Status:        contracts.StatusInProgress,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	child := contracts.TicketSnapshot{
		ID:            "APP-2",
		Project:       "APP",
		Title:         "Child",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusDone,
		Priority:      contracts.PriorityCritical,
		Parent:        epic.ID,
		CreatedAt:     now,
		UpdatedAt:     now.Add(time.Minute),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	ready := contracts.TicketSnapshot{
		ID:            "APP-3",
		Project:       "APP",
		Title:         "Ready",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityMedium,
		CreatedAt:     now,
		UpdatedAt:     now.Add(2 * time.Minute),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	for index, ticket := range []contracts.TicketSnapshot{epic, child, ready} {
		if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
			t.Fatalf("create ticket: %v", err)
		}
		event := contracts.Event{
			EventID:       int64(index + 1),
			Timestamp:     ticket.UpdatedAt,
			Actor:         contracts.Actor("human:owner"),
			Type:          contracts.EventTicketCreated,
			Project:       ticket.Project,
			TicketID:      ticket.ID,
			Payload:       ticket,
			SchemaVersion: contracts.CurrentSchemaVersion,
		}
		if err := eventsLog.AppendEvent(ctx, event); err != nil {
			t.Fatalf("append event: %v", err)
		}
		if err := projection.ApplyEvent(ctx, event); err != nil {
			t.Fatalf("apply event: %v", err)
		}
	}

	queries := NewQueryService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now })
	template, err := queries.Template(ctx, "design")
	if err != nil {
		t.Fatalf("template: %v", err)
	}
	if template.Blueprint != "design" || template.Description != "Map the UX." {
		t.Fatalf("unexpected template: %#v", template)
	}
	detail, err := queries.TicketDetail(ctx, epic.ID)
	if err != nil {
		t.Fatalf("ticket detail: %v", err)
	}
	if detail.Ticket.Progress.Percent != 100 || detail.Ticket.Progress.TotalChildren != 1 {
		t.Fatalf("unexpected progress: %#v", detail.Ticket.Progress)
	}
	next, err := queries.Next(ctx, "")
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	if len(next.Entries) == 0 || next.Entries[0].Category != QueueReadyForMe || next.Entries[0].Entry.Ticket.ID != ready.ID {
		t.Fatalf("unexpected next entries: %#v", next.Entries)
	}
}
