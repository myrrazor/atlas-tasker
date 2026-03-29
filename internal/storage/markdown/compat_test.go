package markdown

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/testutil"
)

func TestStoresLoadV1FixtureWithDefaults(t *testing.T) {
	root := t.TempDir()
	if err := testutil.CopyDir(testutil.FixturePath("app_sample"), root); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}

	projectStore := ProjectStore{RootDir: root}
	ticketStore := TicketStore{RootDir: root}

	project, err := projectStore.GetProject(context.Background(), "APP")
	if err != nil {
		t.Fatalf("get project: %v", err)
	}
	if project.SchemaVersion != contracts.SchemaVersionV1 {
		t.Fatalf("expected legacy project schema to remain v1 on read, got %d", project.SchemaVersion)
	}
	if project.Defaults.CompletionMode != "" {
		t.Fatalf("expected legacy project completion mode to stay empty, got %s", project.Defaults.CompletionMode)
	}
	if project.Defaults.LeaseTTLMinutes != int(contracts.DefaultLeaseTTL.Minutes()) {
		t.Fatalf("unexpected default lease ttl: %d", project.Defaults.LeaseTTLMinutes)
	}

	ticket, err := ticketStore.GetTicket(context.Background(), "APP-1")
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if ticket.SchemaVersion != contracts.CurrentSchemaVersion {
		t.Fatalf("expected normalized ticket schema %d, got %d", contracts.CurrentSchemaVersion, ticket.SchemaVersion)
	}
	if ticket.ReviewState != contracts.ReviewStateNone {
		t.Fatalf("expected default review state none, got %s", ticket.ReviewState)
	}
	if len(ticket.ChangeIDs) != 0 || len(ticket.PermissionProfiles) != 0 || len(ticket.ChangeReadyReasons) != 0 {
		t.Fatalf("expected v1.5 arrays to default empty: %#v", ticket)
	}

	ticket.Title = "Upgraded title"
	if err := ticketStore.UpdateTicket(context.Background(), ticket); err != nil {
		t.Fatalf("update ticket: %v", err)
	}
	rawPath := filepath.Join(root, "projects", "APP", "tickets", "APP-1.md")
	raw, err := os.ReadFile(rawPath)
	if err != nil {
		t.Fatalf("read upgraded ticket: %v", err)
	}
	if !strings.Contains(string(raw), "schema_version: 5") {
		t.Fatalf("expected lazy upgrade write to persist schema_version 5:\n%s", string(raw))
	}

	project.Name = "App Sample Updated"
	if err := projectStore.UpdateProject(context.Background(), project); err != nil {
		t.Fatalf("update project: %v", err)
	}
	projectRaw, err := os.ReadFile(filepath.Join(root, "projects", "APP", "project.md"))
	if err != nil {
		t.Fatalf("read upgraded project: %v", err)
	}
	if !strings.Contains(string(projectRaw), "schema_version: 5") {
		t.Fatalf("expected project update to persist schema_version 5:\n%s", string(projectRaw))
	}
}
