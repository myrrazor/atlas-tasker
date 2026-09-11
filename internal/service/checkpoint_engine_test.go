package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
)

func TestAutomaticCheckpointIsolatesUserGit(t *testing.T) {
	ctx, actions := newCheckpointHarness(t)
	userGit := initUserRepo(t, actions.Root)
	before := captureUserGit(t, userGit)
	if _, err := actions.BackupTick(ctx, true); err != nil {
		t.Fatalf("tick: %v", err)
	}
	after := captureUserGit(t, userGit)
	if before != after {
		t.Fatalf("user git changed\n before %s\n after %s", before, after)
	}
	engine, err := actions.checkpointEngine()
	if err != nil {
		t.Fatal(err)
	}
	listing, err := engine.gitRunner(engine.paths.Tmp, "").run(ctx, "ls-tree", "-r", "--name-only", backupRefName(engine.workspaceID, engine.replicaID))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(listing, "README.md") || strings.Contains(listing, ".git/") {
		t.Fatalf("application source leaked into backup tree:\n%s", listing)
	}
	if !strings.Contains(listing, "projects/APP/") && !strings.Contains(listing, ".atlas-checkpoint.json") {
		t.Fatalf("expected atlas files in backup tree:\n%s", listing)
	}
}

func TestCheckpointCoalesceAndNoRecursion(t *testing.T) {
	ctx, actions := newCheckpointHarness(t)
	now := time.Date(2026, 9, 11, 16, 0, 0, 0, time.UTC)
	clock := now
	actions.Clock = func() time.Time { return clock }
	actions.QuietPeriod = 30 * time.Second
	actions.MaxUnbackedDelay = 5 * time.Minute
	actions.MaxEventsPerCheckpoint = 100
	first, err := actions.BackupTick(ctx, true)
	if err != nil || !first.Created {
		t.Fatalf("first checkpoint: %#v %v", first, err)
	}
	second, err := actions.BackupTick(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if second.Created {
		t.Fatal("unchanged tree must not create another commit")
	}
	for i := 0; i < 20; i++ {
		clock = clock.Add(time.Second)
		if err := mutateTicketTitle(ctx, actions, "edit"); err != nil {
			t.Fatal(err)
		}
	}
	quiet := actions
	_ = quiet
	clock = clock.Add(2 * time.Second)
	early, err := actions.BackupTick(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if early.Created {
		t.Fatal("twenty rapid edits inside the quiet period must coalesce")
	}
	clock = clock.Add(30 * time.Second)
	late, err := actions.BackupTick(ctx, false)
	if err != nil || !late.Created {
		t.Fatalf("quiet period should produce one checkpoint: %#v %v", late, err)
	}
}

func TestCheckpointCrashRecoveryIsIdempotent(t *testing.T) {
	ctx, actions := newCheckpointHarness(t)
	if _, err := actions.BackupTick(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := mutateTicketTitle(ctx, actions, "after first"); err != nil {
		t.Fatal(err)
	}
	for _, point := range []CrashPoint{
		CrashBeforeSnapshot, CrashAfterManifest, CrashAfterGitObjects, CrashBeforeLocalRef,
	} {
		actions.CheckpointCrashAt = string(point)
		if _, err := actions.BackupTick(ctx, true); err == nil {
			t.Fatalf("expected injected crash at %s", point)
		}
	}
	actions.CheckpointCrashAt = ""
	recovered, err := actions.BackupTick(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if !recovered.Created && recovered.SkipReason == "" {
		t.Fatalf("recovery should create or reuse one checkpoint: %#v", recovered)
	}
	again, err := actions.BackupTick(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if again.Created {
		t.Fatal("recovery must not create a conflicting duplicate")
	}
	status, err := actions.AutoBackupStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.State == contracts.BackupOutboxVerified {
		t.Fatal("local checkpoint must not be reported verified before remote verification")
	}
}

func TestLocalCheckpointSurvivesWorkspaceDeletion(t *testing.T) {
	ctx, actions := newCheckpointHarness(t)
	created, err := actions.BackupTick(ctx, true)
	if err != nil || !created.Created {
		t.Fatalf("checkpoint: %#v %v", created, err)
	}
	material := t.TempDir()
	manifest, err := actions.MaterializeLatestCheckpoint(ctx, material)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.WorkspaceID == "" || len(manifest.Files) == 0 {
		t.Fatalf("manifest: %#v", manifest)
	}
	files := make([]string, 0, len(manifest.Files))
	for _, file := range manifest.Files {
		files = append(files, file.Path)
	}
	destRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(destRoot, ".tracker"), 0o755); err != nil {
		t.Fatal(err)
	}
	bundle, err := buildBundleManifest(material, "from-checkpoint", "workspace", time.Date(2026, 9, 11, 16, 0, 0, 0, time.UTC), files)
	if err != nil {
		t.Fatal(err)
	}
	manifestRaw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(destRoot, "from-checkpoint.tar.gz")
	if err := writeBundleArchive(material, archive, manifestRaw, files); err != nil {
		t.Fatal(err)
	}
	planActions := NewActionService(destRoot, mdstore.ProjectStore{RootDir: destRoot}, mdstore.TicketStore{RootDir: destRoot, Clock: actions.Clock}, actions.Events, nil, actions.Clock, FileLockManager{Root: destRoot}, nil, nil)
	plan, err := planActions.CreateRestorePlan(ctx, archive, contracts.Actor("human:owner"))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Plan.Items) == 0 {
		t.Fatal("restore plan from checkpoint was empty")
	}
	found := false
	for _, item := range plan.Plan.Items {
		if strings.Contains(item.Path, "APP-1.md") && item.Action != contracts.RestorePlanBlock {
			found = true
		}
	}
	if !found {
		t.Fatalf("restore plan missing ticket: %#v", plan.Plan.Items)
	}
}

func TestPreDestructiveCheckpointFailureBlocks(t *testing.T) {
	ctx, actions := newCheckpointHarness(t)
	if _, err := actions.BackupTick(ctx, true); err != nil {
		t.Fatal(err)
	}
	actions.RequirePreDestructiveCheckpoint = true
	actions.CheckpointCrashAt = string(CrashBeforeSnapshot)
	if _, err := actions.CompactWorkspace(ctx, true, contracts.Actor("human:owner"), "compact"); err == nil || !strings.Contains(err.Error(), "pre-destructive") {
		t.Fatalf("compact should block, got %v", err)
	}
}

func TestMutationSucceedsWhenCheckpointStateDirOffline(t *testing.T) {
	ctx, actions := newCheckpointHarness(t)
	actions.StateDir = filepath.Join(actions.Root, "missing-parent", "state")
	if err := mutateTicketTitle(ctx, actions, "still writable"); err != nil {
		t.Fatalf("mutation must succeed when outbox write fails: %v", err)
	}
}

func mutateTicketTitle(ctx context.Context, actions *ActionService, title string) error {
	return WithWriteLock(ctx, actions.LockManager, "test edit", func(ctx context.Context) error {
		ticket, err := actions.Tickets.GetTicket(ctx, "APP-1")
		if err != nil {
			return err
		}
		ticket.Title = title
		return actions.commitTicketSnapshotEvent(ctx, "edit", ticket, contracts.Actor("human:owner"), "test edit", contracts.EventTicketUpdated, ticket)
	})
}

func TestScaleSnapshotReusesGitBlobs(t *testing.T) {
	if testing.Short() {
		t.Skip("scale")
	}
	ctx, actions := newCheckpointHarness(t)
	ticketDir := filepath.Join(actions.Root, "projects", "APP", "tickets")
	if err := os.MkdirAll(ticketDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := strings.Repeat("large description\n", 200)
	for i := 2; i <= 200; i++ {
		name := filepath.Join(ticketDir, "APP-"+strconv.Itoa(i)+".md")
		if err := os.WriteFile(name, []byte("# "+strconv.Itoa(i)+"\n\n"+body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	eventDir := filepath.Join(actions.Root, ".tracker", "events")
	if err := os.MkdirAll(eventDir, 0o755); err != nil {
		t.Fatal(err)
	}
	var events strings.Builder
	line := `{"event_id":1,"timestamp":"2026-09-11T16:00:00Z","actor":"human:owner","type":"ticket.commented","project":"APP","ticket_id":"APP-1","schema_version":1}` + "\n"
	for i := 0; i < 1000; i++ {
		events.WriteString(line)
	}
	if err := os.WriteFile(filepath.Join(eventDir, "2026-09.jsonl"), []byte(events.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	first, err := actions.BackupTick(ctx, true)
	if err != nil || !first.Created {
		t.Fatalf("scale checkpoint: %#v %v", first, err)
	}
	copyBound := 5 * time.Second
	if first.CopyDuration > copyBound {
		t.Fatalf("copy duration %s exceeded budget %s", first.CopyDuration, copyBound)
	}
	engine, err := actions.checkpointEngine()
	if err != nil {
		t.Fatal(err)
	}
	blob1, err := engine.gitRunner(engine.paths.Tmp, "").run(ctx, "rev-parse", backupRefName(engine.workspaceID, engine.replicaID)+":projects/APP/tickets/APP-2.md")
	if err != nil {
		t.Fatal(err)
	}
	second, err := actions.BackupTick(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if second.Created {
		t.Fatal("unchanged large tree must reuse the commit")
	}
	blob2, err := engine.gitRunner(engine.paths.Tmp, "").run(ctx, "rev-parse", backupRefName(engine.workspaceID, engine.replicaID)+":projects/APP/tickets/APP-2.md")
	if err != nil {
		t.Fatal(err)
	}
	if blob1 != blob2 {
		t.Fatalf("git blob identity changed for an unchanged file: %s %s", blob1, blob2)
	}
	status, err := actions.AutoBackupStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.DiskBytes <= 0 {
		t.Fatal("status must report disk growth")
	}
	if time.Since(start) > 30*time.Second {
		t.Fatalf("scale snapshot took too long: %s", time.Since(start))
	}
}

func TestGitMaintenanceOnlyAtThreshold(t *testing.T) {
	ctx, actions := newCheckpointHarness(t)
	actions.GitMaintenanceEvery = 1000
	if _, err := actions.BackupTick(ctx, true); err != nil {
		t.Fatal(err)
	}
	engine, err := actions.checkpointEngine()
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := loadLedger(engine.paths.Ledger)
	if err != nil {
		t.Fatal(err)
	}
	if ledger.CommitsSinceMaintenance >= 1000 {
		t.Fatalf("maintenance threshold should not already be met: %d", ledger.CommitsSinceMaintenance)
	}
}

func initUserRepo(t *testing.T, root string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("source\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "staged.txt"), []byte("staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "unstaged.txt"), []byte("unstaged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=user", "GIT_AUTHOR_EMAIL=user@example.com", "GIT_COMMITTER_NAME=user", "GIT_COMMITTER_EMAIL=user@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("add", "README.md")
	run("commit", "-m", "init")
	run("add", "staged.txt")
	if err := os.MkdirAll(filepath.Join(root, ".git", "rebase-merge"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "rebase-merge", "head-name"), []byte("refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("untracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func captureUserGit(t *testing.T, root string) string {
	t.Helper()
	cmd := func(args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		if err != nil {
			return string(out) + err.Error()
		}
		return string(out)
	}
	index, _ := os.ReadFile(filepath.Join(root, ".git", "index"))
	sum := sha256.Sum256(index)
	rebase, _ := os.ReadFile(filepath.Join(root, ".git", "rebase-merge", "head-name"))
	return strings.Join([]string{
		cmd("rev-parse", "HEAD"),
		cmd("symbolic-ref", "HEAD"),
		hex.EncodeToString(sum[:]),
		cmd("diff", "--cached"),
		cmd("diff"),
		cmd("ls-files", "--others", "--exclude-standard"),
		string(rebase),
	}, "|")
}
