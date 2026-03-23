package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

func TestModelLoadsDataAndSwitchesTabs(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 23, 6, 0, 0, 0, time.UTC)

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
	ticket := contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Board item",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
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

	m, err := newModel(root, "")
	if err != nil {
		t.Fatalf("new model: %v", err)
	}
	defer m.close()
	msg := m.refresh()().(loadedMsg)
	updated, _ := m.Update(msg)
	m = updated.(model)
	view := m.View()
	if !strings.Contains(view, "Board") || !strings.Contains(view, "APP-1") {
		t.Fatalf("unexpected view: %s", view)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(model)
	if m.screen != screenQueues {
		t.Fatalf("expected queue screen, got %v", m.screen)
	}
}

func TestCursorClampsAcrossScreenSizes(t *testing.T) {
	m := model{
		screen: screenOwner,
		cursor: 7,
		owner: service.QueueView{
			Categories: map[service.QueueCategory][]service.QueueEntry{
				service.QueueAwaitingOwner: {
					{Ticket: contracts.TicketSnapshot{ID: "APP-1"}},
					{Ticket: contracts.TicketSnapshot{ID: "APP-2"}},
				},
			},
		},
	}

	m = m.moveCursor(-1)
	if m.cursor != 0 {
		t.Fatalf("expected cursor to clamp to first valid index, got %d", m.cursor)
	}

	m.cursor = 9
	m = m.moveCursor(1)
	if m.cursor != 1 {
		t.Fatalf("expected cursor to clamp to last valid index, got %d", m.cursor)
	}
}

func TestPaletteMutationMovesTicket(t *testing.T) {
	root := seededTUIWorkspace(t)
	m, err := newModel(root, contracts.Actor("agent:builder-1"))
	if err != nil {
		t.Fatalf("new model: %v", err)
	}
	defer m.close()
	msg := m.refresh()().(loadedMsg)
	updated, _ := m.Update(msg)
	m = updated.(model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = updated.(model)
	m.dialog.Input.SetValue("/ticket move APP-1 in_progress --actor agent:builder-1")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if cmd == nil {
		t.Fatal("expected mutation command from palette submit")
	}
	updated, _ = m.Update(cmd().(loadedMsg))
	m = updated.(model)
	if m.detail.Ticket.Status != contracts.StatusInProgress {
		t.Fatalf("expected in_progress, got %#v", m.detail.Ticket)
	}
}

func TestCreateFormCreatesTicket(t *testing.T) {
	root := seededTUIWorkspace(t)
	m, err := newModel(root, contracts.Actor("human:owner"))
	if err != nil {
		t.Fatalf("new model: %v", err)
	}
	defer m.close()
	updated, _ := m.Update(m.refresh()().(loadedMsg))
	m = updated.(model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = updated.(model)
	for _, field := range []string{"APP", "New ticket", "task", "Short desc"} {
		m.dialog.Fields[m.dialog.Focus].Input.SetValue("")
		for _, ch := range field {
			updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
			m = updated.(model)
		}
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = updated.(model)
		if cmd != nil {
			updated, _ = m.Update(cmd().(loadedMsg))
			m = updated.(model)
		}
	}
	if m.detail.Ticket.ID != "APP-2" {
		t.Fatalf("expected APP-2 detail after create, got %#v", m.detail.Ticket)
	}
	if m.detail.Ticket.Title != "New ticket" {
		t.Fatalf("unexpected created ticket: %#v", m.detail.Ticket)
	}
}

func seededTUIWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 23, 10, 0, 0, 0, time.UTC)
	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer projection.Close()
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}, Actor: contracts.ActorConfig{Default: contracts.Actor("agent:builder-1")}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket := contracts.TicketSnapshot{ID: "APP-1", Project: "APP", Title: "Seed", Summary: "Seed", Type: contracts.TicketTypeTask, Status: contracts.StatusReady, Priority: contracts.PriorityHigh, CreatedAt: now, UpdatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}
	if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	event := contracts.Event{EventID: 1, Timestamp: now, Actor: contracts.Actor("human:owner"), Type: contracts.EventTicketCreated, Project: "APP", TicketID: ticket.ID, Payload: ticket, SchemaVersion: contracts.CurrentSchemaVersion}
	if err := eventsLog.AppendEvent(ctx, event); err != nil {
		t.Fatalf("append event: %v", err)
	}
	if err := projection.ApplyEvent(ctx, event); err != nil {
		t.Fatalf("apply event: %v", err)
	}
	return root
}
