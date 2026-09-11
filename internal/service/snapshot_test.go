package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
)

func TestExportAllowlistParity(t *testing.T) {
	for _, prefix := range RestoreSafeAllowlistPrefixes() {
		covered := false
		for _, candidate := range ExportCandidateRoots() {
			if candidateCoversPrefix(candidate, prefix) {
				covered = true
				break
			}
		}
		if !covered {
			for _, excluded := range IntentionallyUncollectedAllowlistPrefixes() {
				if excluded == prefix {
					covered = true
				}
			}
		}
		if !covered {
			t.Fatalf("allowlist prefix %s is neither a collector candidate nor intentionally excluded", prefix)
		}
	}
	for _, candidate := range ExportCandidateRoots() {
		sample := samplePathForCandidate(candidate)
		item := contracts.RestorePlanItem{Path: sample, Action: contracts.RestorePlanCreate}
		if item.Validate() == nil {
			continue
		}
		if !isExportedOnlyCandidate(candidate) {
			t.Fatalf("collector candidate %s (sample %s) is neither restore-safe nor exported-only", candidate, sample)
		}
	}
}

func samplePathForCandidate(candidate string) string {
	switch candidate {
	case ".tracker/managed-mode.json", ".tracker/config.toml":
		return candidate
	case "projects":
		return "projects/APP/APP.md"
	case ".tracker/events":
		return ".tracker/events/2026-09.jsonl"
	case ".tracker/permission-profiles", ".tracker/retention", ".tracker/governance/policies", ".tracker/governance/packs", ".tracker/classification/policies", ".tracker/redaction/rules":
		return filepath.ToSlash(filepath.Join(candidate, "item.toml"))
	case ".tracker/security/signatures", ".tracker/audit/reports", ".tracker/audit/packets":
		return filepath.ToSlash(filepath.Join(candidate, "item.json"))
	default:
		return filepath.ToSlash(filepath.Join(candidate, "item.md"))
	}
}

func TestCollectExportFilesIncludesCollaborationAndArchives(t *testing.T) {
	root := t.TempDir()
	writes := map[string]string{
		"projects/APP/APP.md":                          "# APP\n",
		".tracker/collaborators/alice.md":              "alice\n",
		".tracker/memberships/alice.md":                "member\n",
		".tracker/mentions/m1.md":                      "mention\n",
		".tracker/archives/arch_1.md":                  "archive\n",
		".tracker/archives/arch_1/projects/APP/APP.md": "payload\n",
		".tracker/runtime/skip.me":                     "nope\n",
		".tracker/security/keys/private/key.json":      "secret\n",
		".tracker/backups/snapshots/old.tar.gz":        "old\n",
		"README.md":                                    "source\n",
		".env":                                         "TOKEN=1\n",
	}
	for rel, body := range writes {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files, err := collectExportFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(files, "\n")
	for _, want := range []string{
		"projects/APP/APP.md",
		".tracker/collaborators/alice.md",
		".tracker/memberships/alice.md",
		".tracker/mentions/m1.md",
		".tracker/archives/arch_1.md",
		".tracker/archives/arch_1/projects/APP/APP.md",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("collector missed %s\n%s", want, joined)
		}
	}
	safe := backupRestoreSafeFiles(files)
	safeJoin := strings.Join(safe, "\n")
	for _, banned := range []string{"README.md", ".env", "private/key.json", "runtime/skip.me", "backups/snapshots"} {
		if strings.Contains(safeJoin, banned) {
			t.Fatalf("restore-safe set leaked %s:\n%s", banned, safeJoin)
		}
	}
}

func TestManualBackupAndCheckpointShareCanonicalRecords(t *testing.T) {
	ctx, actions := newCheckpointHarness(t)
	view, err := actions.CreateBackup(ctx, "workspace", contracts.Actor("human:owner"), "manual")
	if err != nil {
		t.Fatalf("manual backup: %v", err)
	}
	manual, _, err := loadBundleManifestRaw(backupManifestPath(actions.Root, view.Snapshot.BackupID))
	if err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	snap, err := actions.CaptureCanonicalSnapshot(ctx, dest)
	if err != nil {
		t.Fatal(err)
	}
	manualPaths := map[string]bool{}
	for _, file := range manual.Files {
		manualPaths[file.Path] = true
	}
	if len(snap.Files) != len(manual.Files) {
		t.Fatalf("checkpoint files %d vs archive files %d", len(snap.Files), len(manual.Files))
	}
	for _, file := range snap.Files {
		if !manualPaths[file.Path] {
			t.Fatalf("checkpoint path %s missing from archive", file.Path)
		}
	}
	if !snap.HashedAfterUnlock || !snap.CopiedUnderLock {
		t.Fatal("snapshot must copy under lock and hash after unlock")
	}
}

func TestSnapshotRejectsSymlinkAndSource(t *testing.T) {
	root := t.TempDir()
	ticket := filepath.Join(root, "projects", "APP", "tickets")
	if err := os.MkdirAll(ticket, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ticket, "APP-1.md"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(ticket, "APP-1.md"), filepath.Join(ticket, "APP-2.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := collectExportFiles(root); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlinked ticket must be rejected, got %v", err)
	}
}

func newCheckpointHarness(t *testing.T) (context.Context, *ActionService) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	state := t.TempDir()
	now := time.Date(2026, 9, 11, 16, 0, 0, 0, time.UTC)
	if err := config.Save(root, contracts.TrackerConfig{Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen}}); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureWorkspaceIdentity(root); err != nil {
		t.Fatal(err)
	}
	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	events := &eventstore.Log{RootDir: root}
	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatal(err)
	}
	ticket := contracts.TicketSnapshot{
		ID: "APP-1", Project: "APP", Title: "Checkpoint me", Type: contracts.TicketTypeTask,
		Status: contracts.StatusReady, Priority: contracts.PriorityMedium,
		CreatedAt: now, UpdatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := ticketStore.CreateTicket(ctx, ticket); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".tracker", "collaborators"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".tracker", "collaborators", "alice.md"), []byte("alice\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	actions := NewActionService(root, projectStore, ticketStore, events, nil, func() time.Time { return now }, FileLockManager{Root: root}, nil, nil)
	actions.StateDir = state
	actions.Home = t.TempDir()
	if err := actions.AppendAndProject(ctx, contracts.Event{
		EventID: 1, Timestamp: now, Actor: contracts.Actor("human:owner"), Type: contracts.EventTicketCreated,
		Project: "APP", TicketID: ticket.ID, Payload: ticket, SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatal(err)
	}
	_ = storage.TrackerDir(root)
	return ctx, actions
}
