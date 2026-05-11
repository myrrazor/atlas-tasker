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

func setupAgentWakeupTest(t *testing.T) (string, context.Context, *ActionService, *QueryService, mdstore.TicketStore, time.Time) {
	t.Helper()
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 5, 9, 16, 0, 0, 0, time.UTC)
	projects := mdstore.ProjectStore{RootDir: root}
	tickets := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	events := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), tickets, events)
	if err != nil {
		t.Fatalf("open projection: %v", err)
	}
	t.Cleanup(func() { _ = projection.Close() })
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := projects.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := (AgentStore{Root: root}).SaveAgent(ctx, contracts.AgentProfile{AgentID: "builder-1", DisplayName: "Builder", Provider: contracts.AgentProviderCodex, Enabled: true}); err != nil {
		t.Fatalf("save agent: %v", err)
	}
	actions := NewActionService(root, projects, tickets, events, projection, func() time.Time { return now }, FileLockManager{Root: root}, nil, nil)
	queries := NewQueryService(root, projects, tickets, events, projection, func() time.Time { return now })
	return root, ctx, actions, queries, tickets, now
}

func TestAgentWakeupCreatedWhenBlockerReachesDone(t *testing.T) {
	_, ctx, actions, queries, tickets, now := setupAgentWakeupTest(t)
	blocker := testAgentWorkTicket("APP-1", "Blocker", contracts.StatusInReview, now)
	blocker.ReviewState = contracts.ReviewStateApproved
	dependent := testAgentWorkTicket("APP-2", "Dependent", contracts.StatusReady, now)
	dependent.Assignee = contracts.Actor("agent:builder-1")
	dependent.BlockedBy = []string{blocker.ID}
	for _, ticket := range []contracts.TicketSnapshot{blocker, dependent} {
		if err := tickets.CreateTicket(ctx, ticket); err != nil {
			t.Fatalf("create %s: %v", ticket.ID, err)
		}
	}
	if _, err := actions.MoveTicket(ctx, blocker.ID, contracts.StatusDone, contracts.Actor("human:owner"), "blocker done"); err != nil {
		t.Fatalf("complete blocker: %v", err)
	}
	wakeups, err := queries.AgentWakeups(ctx, "builder-1")
	if err != nil {
		t.Fatalf("list wakeups: %v", err)
	}
	if len(wakeups) != 1 {
		t.Fatalf("expected one wakeup, got %#v", wakeups)
	}
	if wakeups[0].TicketID != dependent.ID || wakeups[0].BlockerTicketID != blocker.ID || wakeups[0].State != AgentWakeupPending {
		t.Fatalf("unexpected wakeup: %#v", wakeups[0])
	}
	events, err := actions.Events.StreamEvents(ctx, "APP", 0)
	if err != nil {
		t.Fatalf("stream events: %v", err)
	}
	found := false
	for _, event := range events {
		if event.Type == contracts.EventAgentWorkAvailable {
			found = true
			if event.Actor != contracts.ActorAtlasSystem {
				t.Fatalf("expected system actor %s, got %s", contracts.ActorAtlasSystem, event.Actor)
			}
		}
	}
	if !found {
		t.Fatalf("expected agent.work_available event, got %#v", events)
	}
}

func TestAgentWakeupCommandModeLaunchesArgvWithoutShell(t *testing.T) {
	root, ctx, actions, queries, tickets, now := setupAgentWakeupTest(t)
	marker := filepath.Join(root, "launched.marker")
	if _, err := actions.SetAgentAuto(ctx, "builder-1", AgentAutoModeCommand, []string{"/usr/bin/touch", marker}, contracts.Actor("human:owner"), "enable command mode"); err != nil {
		t.Fatalf("set auto: %v", err)
	}
	blocker := testAgentWorkTicket("APP-1", "Blocker", contracts.StatusInReview, now)
	blocker.ReviewState = contracts.ReviewStateApproved
	dependent := testAgentWorkTicket("APP-2", "Dependent", contracts.StatusReady, now)
	dependent.Assignee = contracts.Actor("agent:builder-1")
	dependent.BlockedBy = []string{blocker.ID}
	for _, ticket := range []contracts.TicketSnapshot{blocker, dependent} {
		if err := tickets.CreateTicket(ctx, ticket); err != nil {
			t.Fatalf("create %s: %v", ticket.ID, err)
		}
	}
	if _, err := actions.MoveTicket(ctx, blocker.ID, contracts.StatusDone, contracts.Actor("human:owner"), "blocker done"); err != nil {
		t.Fatalf("complete blocker: %v", err)
	}
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected command mode marker: %v", err)
	}
	wakeups, err := queries.AgentWakeups(ctx, "builder-1")
	if err != nil {
		t.Fatalf("list wakeups: %v", err)
	}
	if len(wakeups) != 1 || wakeups[0].State != AgentWakeupLaunched || len(wakeups[0].Command) != 2 {
		t.Fatalf("expected launched wakeup with command, got %#v", wakeups)
	}
}

func TestAgentAutoRejectsShellInterpreter(t *testing.T) {
	_, ctx, actions, _, _, _ := setupAgentWakeupTest(t)
	_, err := actions.SetAgentAuto(ctx, "builder-1", AgentAutoModeCommand, []string{"sh", "-c", "echo no"}, contracts.Actor("human:owner"), "bad command")
	if err == nil {
		t.Fatal("expected shell interpreter to be rejected")
	}
}
