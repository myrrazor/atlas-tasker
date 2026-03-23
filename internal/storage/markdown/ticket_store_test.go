package markdown

import (
	"context"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestTicketStoreCreateGetUpdateListSoftDelete(t *testing.T) {
	root := t.TempDir()
	projectStore := ProjectStore{RootDir: root}
	if err := projectStore.CreateProject(context.Background(), contracts.Project{
		Key: "APP", Name: "App Project", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create project failed: %v", err)
	}

	now := time.Date(2026, 3, 22, 13, 0, 0, 0, time.UTC)
	store := TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	ticket := contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "First ticket",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusBacklog,
		Priority:      contracts.PriorityMedium,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := store.CreateTicket(context.Background(), ticket); err != nil {
		t.Fatalf("create ticket failed: %v", err)
	}

	loaded, err := store.GetTicket(context.Background(), "APP-1")
	if err != nil {
		t.Fatalf("get ticket failed: %v", err)
	}
	if loaded.Title != "First ticket" {
		t.Fatalf("unexpected title: %s", loaded.Title)
	}

	loaded.Title = "Updated title"
	if err := store.UpdateTicket(context.Background(), loaded); err != nil {
		t.Fatalf("update ticket failed: %v", err)
	}

	listed, err := store.ListTickets(context.Background(), contracts.TicketListOptions{Project: "APP"})
	if err != nil {
		t.Fatalf("list tickets failed: %v", err)
	}
	if len(listed) != 1 || listed[0].Title != "Updated title" {
		t.Fatalf("unexpected list result: %#v", listed)
	}

	if err := store.SoftDeleteTicket(context.Background(), "APP-1", contracts.Actor("human:owner"), "cleanup"); err != nil {
		t.Fatalf("soft delete failed: %v", err)
	}
	deleted, err := store.GetTicket(context.Background(), "APP-1")
	if err != nil {
		t.Fatalf("get deleted ticket failed: %v", err)
	}
	if deleted.Status != contracts.StatusCanceled || !deleted.Archived {
		t.Fatalf("soft delete fields not set: %#v", deleted)
	}
	if deleted.Notes == "" {
		t.Fatal("soft delete should add audit note")
	}
}

func TestTicketStoreSoftDeleteRejectsInvalidActor(t *testing.T) {
	root := t.TempDir()
	projectStore := ProjectStore{RootDir: root}
	now := time.Now().UTC()
	if err := projectStore.CreateProject(context.Background(), contracts.Project{
		Key: "APP", Name: "App Project", CreatedAt: now,
	}); err != nil {
		t.Fatalf("create project failed: %v", err)
	}
	store := TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	ticket := contracts.TicketSnapshot{
		ID:            "APP-2",
		Project:       "APP",
		Title:         "Another",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusBacklog,
		Priority:      contracts.PriorityLow,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := store.CreateTicket(context.Background(), ticket); err != nil {
		t.Fatalf("create ticket failed: %v", err)
	}
	if err := store.SoftDeleteTicket(context.Background(), "APP-2", contracts.Actor(""), ""); err == nil {
		t.Fatal("expected invalid actor error")
	}
}
