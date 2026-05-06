package service

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

func TestCollectMentionCandidatesIgnoresNonCanonicalContexts(t *testing.T) {
	text := stringsJoinForTest(
		"real @alana mention",
		"mail alana@example.com should not count",
		"url https://example.com/@alana should not count",
		"path /tmp/@alana should not count",
		"provider handle @gh/alana should not count",
		"escaped \\@alana should not count",
		"inline `@alana` should not count",
		"```\n@alana\n```",
	)
	got := collectMentionCandidates(text)
	want := []string{"alana"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected mention candidates: got %#v want %#v", got, want)
	}
}

func stringsJoinForTest(parts ...string) string {
	joined := ""
	for i, part := range parts {
		if i > 0 {
			joined += "\n"
		}
		joined += part
	}
	return joined
}

func TestCommentMentionPersistsMentionRecordAndAuditEvent(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	now := time.Date(2026, 3, 29, 1, 0, 0, 0, time.UTC)
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
	ticket := contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Mention test",
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

	actions := NewActionService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return clock }, FileLockManager{Root: root}, nil, nil)
	createdEvent := contracts.Event{EventID: 1, Timestamp: now, Actor: contracts.Actor("human:owner"), Type: contracts.EventTicketCreated, Project: "APP", TicketID: ticket.ID, Payload: ticket, SchemaVersion: contracts.CurrentSchemaVersion}
	if err := actions.AppendAndProject(ctx, createdEvent); err != nil {
		t.Fatalf("append create event: %v", err)
	}
	if _, err := actions.AddCollaborator(ctx, contracts.CollaboratorProfile{CollaboratorID: "alana", DisplayName: "Alana"}, contracts.Actor("human:owner"), "seed collaborator"); err != nil {
		t.Fatalf("add collaborator: %v", err)
	}

	clock = clock.Add(time.Minute)
	if err := actions.CommentTicket(ctx, ticket.ID, "please review @alana", contracts.Actor("human:owner"), "mention collaborator"); err != nil {
		t.Fatalf("comment ticket: %v", err)
	}

	mentions, err := MentionStore{Root: root}.ListMentions(ctx, "alana")
	if err != nil {
		t.Fatalf("list mentions: %v", err)
	}
	if len(mentions) != 1 {
		t.Fatalf("expected one mention record, got %#v", mentions)
	}

	history, err := eventsLog.StreamEvents(ctx, "APP", 0)
	if err != nil {
		t.Fatalf("stream events: %v", err)
	}
	found := false
	for _, event := range history {
		if event.Type == contracts.EventMentionRecorded {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected mention.recorded event in history, got %#v", history)
	}
}
