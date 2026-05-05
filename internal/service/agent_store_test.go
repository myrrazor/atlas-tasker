package service

import (
	"context"
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

func TestAgentStoreRoundTrip(t *testing.T) {
	root := t.TempDir()
	store := AgentStore{Root: root}
	profile := contracts.AgentProfile{
		AgentID:       "builder-1",
		DisplayName:   "Builder One",
		Provider:      contracts.AgentProviderCodex,
		Enabled:       true,
		Capabilities:  []string{"go", "tests"},
		MaxActiveRuns: 2,
	}
	if err := store.SaveAgent(context.Background(), profile); err != nil {
		t.Fatalf("save agent: %v", err)
	}
	loaded, err := store.LoadAgent(context.Background(), "builder-1")
	if err != nil {
		t.Fatalf("load agent: %v", err)
	}
	if loaded.AgentID != "builder-1" || loaded.DisplayName != "Builder One" {
		t.Fatalf("unexpected agent: %#v", loaded)
	}
	items, err := store.ListAgents(context.Background())
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one agent, got %#v", items)
	}
}

func TestQueryServiceAgentEligibility(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	now := time.Date(2026, 3, 25, 2, 0, 0, 0, time.UTC)
	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer projection.Close()
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	actions := NewActionService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now }, FileLockManager{Root: root}, nil, nil)
	queries := NewQueryService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now })
	project := contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}
	if err := projectStore.CreateProject(ctx, project); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket := contracts.TicketSnapshot{
		ID:                   "APP-1",
		Project:              "APP",
		Title:                "Needs Go",
		Type:                 contracts.TicketTypeTask,
		Status:               contracts.StatusReady,
		Priority:             contracts.PriorityHigh,
		CreatedAt:            now,
		UpdatedAt:            now,
		SchemaVersion:        contracts.CurrentSchemaVersion,
		RequiredCapabilities: []string{"go", "tests"},
	}
	if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if err := actions.AppendAndProject(ctx, contracts.Event{EventID: 1, Timestamp: now, Actor: contracts.Actor("human:owner"), Type: contracts.EventTicketCreated, Project: "APP", TicketID: ticket.ID, Payload: ticket, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("append create event: %v", err)
	}
	if _, err := actions.SaveAgentProfile(ctx, contracts.AgentProfile{
		AgentID:      "builder-ok",
		DisplayName:  "Builder OK",
		Provider:     contracts.AgentProviderCodex,
		Enabled:      true,
		Capabilities: []string{"go", "tests", "sqlite"},
	}, contracts.Actor("human:owner"), "seed"); err != nil {
		t.Fatalf("save eligible agent: %v", err)
	}
	if _, err := actions.SaveAgentProfile(ctx, contracts.AgentProfile{
		AgentID:      "builder-nope",
		DisplayName:  "Builder Nope",
		Provider:     contracts.AgentProviderCodex,
		Enabled:      false,
		Capabilities: []string{"go"},
	}, contracts.Actor("human:owner"), "seed"); err != nil {
		t.Fatalf("save ineligible agent: %v", err)
	}
	report, err := queries.AgentEligibility(ctx, "APP-1")
	if err != nil {
		t.Fatalf("eligibility: %v", err)
	}
	if len(report.Entries) != 2 {
		t.Fatalf("unexpected report: %#v", report)
	}
	if !report.Entries[0].Eligible || report.Entries[0].Agent.AgentID != "builder-ok" {
		t.Fatalf("expected eligible agent first, got %#v", report.Entries)
	}
	if report.Entries[1].Eligible {
		t.Fatalf("expected ineligible agent second, got %#v", report.Entries[1])
	}
}
