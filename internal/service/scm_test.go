package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

func TestSCMServiceContextForTicket(t *testing.T) {
	root := t.TempDir()
	initGitRepo(t, root)
	writeFile(t, filepath.Join(root, "README.md"), "# atlas\n")
	gitRun(t, root, "add", "README.md")
	gitRun(t, root, "commit", "-m", "init")
	gitRun(t, root, "checkout", "-b", "ticket/app-1-build-parser")
	writeFile(t, filepath.Join(root, "parser.txt"), "parser\n")
	gitRun(t, root, "add", "parser.txt")
	gitRun(t, root, "commit", "-m", "APP-1: wire parser")

	view, err := SCMService{Root: root}.ContextForTicket(context.Background(), contracts.TicketSnapshot{
		ID:    "APP-1",
		Title: "Build parser",
	})
	if err != nil {
		t.Fatalf("context for ticket: %v", err)
	}
	if !view.Repo.Present || !view.CurrentBranchMatches {
		t.Fatalf("unexpected repo view: %#v", view)
	}
	if len(view.Refs) != 1 || !strings.Contains(view.Refs[0].Subject, "APP-1") {
		t.Fatalf("unexpected refs: %#v", view.Refs)
	}
}

func TestSCMServiceCommitRequiresStagedChangesAndPrefixesTicketID(t *testing.T) {
	root := t.TempDir()
	initGitRepo(t, root)
	writeFile(t, filepath.Join(root, "README.md"), "# atlas\n")
	gitRun(t, root, "add", "README.md")
	gitRun(t, root, "commit", "-m", "init")

	service := SCMService{Root: root}
	ticket := contracts.TicketSnapshot{ID: "APP-1", Title: "Commit me"}
	if _, err := service.Commit(context.Background(), ticket, "ship it"); err == nil {
		t.Fatal("expected commit helper to fail when nothing is staged")
	}

	writeFile(t, filepath.Join(root, "feature.txt"), "hello\n")
	gitRun(t, root, "add", "feature.txt")
	hash, err := service.Commit(context.Background(), ticket, "ship it")
	if err != nil {
		t.Fatalf("commit helper: %v", err)
	}
	if len(hash) < 7 {
		t.Fatalf("unexpected commit hash: %s", hash)
	}
	subject := strings.TrimSpace(gitRun(t, root, "log", "-1", "--pretty=%s"))
	if subject != "APP-1: ship it" {
		t.Fatalf("expected canonical ticket prefix, got %q", subject)
	}
}

func TestSCMServiceBranchExistsTreatsQuietMissAsFalse(t *testing.T) {
	root := t.TempDir()
	initGitRepo(t, root)
	writeFile(t, filepath.Join(root, "README.md"), "# atlas\n")
	gitRun(t, root, "add", "README.md")
	gitRun(t, root, "commit", "-m", "init")

	exists, err := (SCMService{Root: root}).BranchExists(context.Background(), "ticket/missing-branch")
	if err != nil {
		t.Fatalf("branch exists: %v", err)
	}
	if exists {
		t.Fatal("expected missing branch to report false")
	}
}

func TestQueryServiceQueueAndDetailIncludeGitContext(t *testing.T) {
	root := t.TempDir()
	initGitRepo(t, root)
	writeFile(t, filepath.Join(root, "README.md"), "# atlas\n")
	gitRun(t, root, "add", "README.md")
	gitRun(t, root, "commit", "-m", "init")
	gitRun(t, root, "checkout", "-b", "ticket/app-1-queue-me")

	ctx := context.Background()
	now := time.Date(2026, 3, 23, 16, 0, 0, 0, time.UTC)
	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer projection.Close()
	installFakeGH(t, `#!/bin/sh
echo "gh should not be called for passive ticket reads" >&2
exit 1
`)
	if err := config.Save(root, contracts.TrackerConfig{
		Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen},
		Provider: contracts.ProviderConfig{
			DefaultSCMProvider: contracts.ChangeProviderGitHub,
			GitHubRepo:         "myrrazor/atlas-tasker",
		},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	ticket := contracts.TicketSnapshot{
		ID:            "APP-1",
		Project:       "APP",
		Title:         "Queue me",
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
	created := contracts.Event{EventID: 1, Timestamp: now, Actor: contracts.Actor("human:owner"), Type: contracts.EventTicketCreated, Project: "APP", TicketID: ticket.ID, Payload: ticket, SchemaVersion: contracts.CurrentSchemaVersion}
	if err := eventsLog.AppendEvent(ctx, created); err != nil {
		t.Fatalf("append event: %v", err)
	}
	if err := projection.ApplyEvent(ctx, created); err != nil {
		t.Fatalf("apply event: %v", err)
	}
	queries := NewQueryService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now })
	detail, err := queries.TicketDetail(ctx, ticket.ID)
	if err != nil {
		t.Fatalf("ticket detail: %v", err)
	}
	if !detail.Git.Repo.Present || !detail.Git.CurrentBranchMatches {
		t.Fatalf("expected git context in detail: %#v", detail.Git)
	}
	queue, err := queries.Queue(ctx, contracts.Actor("human:owner"))
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if len(queue.Categories[QueueReadyForMe]) != 1 || queue.Categories[QueueReadyForMe][0].GitHint == "" {
		t.Fatalf("expected git hint in queue: %#v", queue.Categories[QueueReadyForMe])
	}
}

func initGitRepo(t *testing.T, root string) {
	t.Helper()
	gitRun(t, root, "init", "-b", "main")
	gitRun(t, root, "config", "user.email", "atlas@example.com")
	gitRun(t, root, "config", "user.name", "Atlas")
}

func gitRun(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
	return string(output)
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestSCMChangedFilesPreservesPorcelainPaths(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()

	initGitRepo(t, root)
	writeFile(t, filepath.Join(root, "README.md"), "# atlas\n")
	gitRun(t, root, "add", "README.md")
	gitRun(t, root, "commit", "-m", "init")

	writeFile(t, filepath.Join(root, "README.md"), "# atlas\n\nupdated\n")
	writeFile(t, filepath.Join(root, "notes.txt"), "untracked\n")

	files, err := (SCMService{Root: root}).ChangedFiles(ctx)
	if err != nil {
		t.Fatalf("changed files: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected two changed files, got %#v", files)
	}
	if files[0] != "README.md" || files[1] != "notes.txt" {
		t.Fatalf("unexpected changed files: %#v", files)
	}
}
