package mcp

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

func TestServerToolsAndCalls(t *testing.T) {
	root, server := seededServer(t)
	_ = root

	toolsResult, err := server.handle(context.Background(), request{Method: "tools/list"})
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	tools := toolsResult.(map[string]any)["tools"].([]Tool)
	if len(tools) == 0 {
		t.Fatal("expected MCP tools to be exported")
	}

	viewResult, err := server.handle(context.Background(), request{
		Method: "tools/call",
		Params: map[string]any{
			"name": "atlas_ticket_view",
			"arguments": map[string]any{
				"ticket_id": "APP-1",
			},
		},
	})
	if err != nil {
		t.Fatalf("ticket view call: %v", err)
	}
	if viewResult.(map[string]any)["structuredContent"] == nil {
		t.Fatalf("expected structured content from ticket view: %#v", viewResult)
	}

	_, err = server.handle(context.Background(), request{
		Method: "tools/call",
		Params: map[string]any{
			"name": "atlas_ticket_comment",
			"arguments": map[string]any{
				"ticket_id": "APP-1",
				"body":      "from mcp",
				"actor":     "human:owner",
			},
		},
	})
	if err != nil {
		t.Fatalf("ticket comment call: %v", err)
	}
	history, err := server.Queries.History(context.Background(), "APP-1")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history.Events) < 2 || history.Events[len(history.Events)-1].Type != contracts.EventTicketCommented {
		t.Fatalf("expected comment event after MCP call, got %#v", history.Events)
	}
}

func seededServer(t *testing.T) (string, Server) {
	t.Helper()
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 24, 16, 0, 0, 0, time.UTC)
	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = projection.Close() })
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}, Actor: contracts.ActorConfig{Default: contracts.Actor("human:owner")}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	actions := service.NewActionService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now }, service.FileLockManager{Root: root}, nil, nil)
	ticket, err := actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "MCP seed",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed")
	if err != nil {
		t.Fatalf("create tracked ticket: %v", err)
	}
	if ticket.ID != "APP-1" {
		t.Fatalf("unexpected seeded ticket: %#v", ticket)
	}
	queries := service.NewQueryService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now })
	return root, Server{Actions: actions, Queries: queries}
}
