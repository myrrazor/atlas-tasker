package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestManagedWorkflowSixActorScenarioAndCheckpoint(t *testing.T) {
	ctx, actions := newCheckpointHarness(t)
	// The snapshot fixture writes raw "alice\n". CommentTicket lists
	// collaborators and requires real frontmatter documents.
	if err := os.Remove(filepath.Join(actions.Root, ".tracker", "collaborators", "alice.md")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	remote := initBareRemote(t)
	if _, err := actions.AddBackupTarget(ctx, BackupTargetAddOptions{
		TargetID: "scenario", URL: "file://" + remote, Enabled: true,
		AcknowledgeBoundary: true, AttestPrivate: true, AllowLocalFile: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := actions.EnableAutoBackup(ctx, "scenario"); err != nil {
		t.Fatal(err)
	}
	now := actions.now()
	created, err := actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "Codex starts",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityMedium,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
		Assignee:      "agent:codex",
	}, contracts.Actor("agent:codex"), "codex starts a ticket")
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.ID == "APP-1" {
		t.Fatalf("expected a second ticket, got %s", created.ID)
	}
	if _, err := actions.MoveTicket(ctx, created.ID, contracts.StatusInProgress, contracts.Actor("agent:codex"), "codex starts"); err != nil {
		t.Fatal(err)
	}
	if err := actions.CommentTicket(ctx, created.ID, "implementation note", contracts.Actor("agent:codex"), "progress"); err != nil {
		t.Fatal(err)
	}
	if _, err := actions.RequestReviewWithReviewer(ctx, created.ID, contracts.Actor("human:owner"), contracts.Actor("agent:codex"), "ready for claude"); err != nil {
		t.Fatal(err)
	}
	if _, err := actions.ApproveTicket(ctx, created.ID, contracts.Actor("agent:codex"), "self approve"); err == nil || apperr.CodeOf(err) != apperr.CodePermissionDenied {
		t.Fatalf("review separation must refuse the requester: %v", err)
	}
	if _, err := actions.ApproveTicket(ctx, created.ID, contracts.Actor("human:owner"), "owner reviews after claude"); err != nil {
		t.Fatal(err)
	}
	blocked, err := actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "OpenClaw blocked",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityMedium,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
		Assignee:      "agent:openclaw",
	}, contracts.Actor("agent:openclaw"), "blocked work")
	if err != nil {
		t.Fatal(err)
	}
	if blocked.ID == created.ID {
		t.Fatal("duplicate ticket id")
	}
	if _, err := actions.MoveTicket(ctx, blocked.ID, contracts.StatusBlocked, contracts.Actor("agent:openclaw"), "blocked"); err != nil {
		t.Fatal(err)
	}
	if err := actions.CommentTicket(ctx, blocked.ID, "waiting on API", contracts.Actor("agent:openclaw"), "blocked note"); err != nil {
		t.Fatal(err)
	}
	if err := actions.CommentTicket(ctx, "APP-1", "grok asks for next work", contracts.Actor("agent:grok"), "next"); err != nil {
		t.Fatal(err)
	}
	if err := actions.CommentTicket(ctx, "APP-1", "generic reads dashboard", contracts.Actor("agent:generic"), "dashboard"); err != nil {
		t.Fatal(err)
	}
	events, err := actions.Events.StreamEvents(ctx, "APP", 0)
	if err != nil {
		t.Fatal(err)
	}
	wantActors := map[string]bool{
		"agent:codex":    false,
		"human:owner":    false,
		"agent:openclaw": false,
		"agent:grok":     false,
		"agent:generic":  false,
	}
	for _, event := range events {
		if _, ok := wantActors[string(event.Actor)]; ok {
			wantActors[string(event.Actor)] = true
		}
	}
	for actor, seen := range wantActors {
		if !seen {
			t.Fatalf("missing actor %s in event log", actor)
		}
	}
	if _, err := actions.BackupTick(ctx, true); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	status, err := actions.AutoBackupStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.LastLocalCheckpointID == "" {
		t.Fatal("checkpoint missing")
	}
	if strings.Contains(status.HealthWarning, "://") || strings.Contains(status.LastErrorClass, "file://") {
		t.Fatalf("status leaked a URL: %#v", status)
	}
	dest := t.TempDir()
	snap, err := actions.MaterializeLatestCheckpoint(ctx, dest)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	foundCreated, foundBlocked := false, false
	for _, file := range snap.Files {
		if strings.Contains(file.Path, created.ID) {
			foundCreated = true
		}
		if strings.Contains(file.Path, blocked.ID) {
			foundBlocked = true
		}
		if strings.Contains(file.Path, "README") || strings.Contains(file.Path, ".env") {
			t.Fatalf("checkpoint included excluded path %s", file.Path)
		}
	}
	if !foundCreated || !foundBlocked {
		t.Fatalf("checkpoint missing tickets: created=%v blocked=%v files=%#v", foundCreated, foundBlocked, snap.Files)
	}
}
