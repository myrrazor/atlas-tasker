package cli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	"github.com/myrrazor/atlas-tasker/internal/testutil"
)

func TestDualGateFlowRejectsAndCompletes(t *testing.T) {
	withTempWorkspace(t)

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("command failed %v: %v\n%s", args, err, out)
		}
		return out
	}

	must("init")
	must("project", "create", "APP", "App Project")
	must("project", "policy", "set", "APP", "--completion-mode", "dual_gate", "--lease-ttl", "45", "--required-reviewer", "agent:reviewer-1", "--allowed-workers", "agent:builder-1", "--actor", "human:owner")
	must("ticket", "create", "--project", "APP", "--title", "Flow", "--type", "task", "--reviewer", "agent:reviewer-1", "--actor", "human:owner")
	must("ticket", "claim", "APP-1", "--actor", "agent:builder-1")
	must("ticket", "move", "APP-1", "ready", "--actor", "agent:builder-1")
	must("ticket", "move", "APP-1", "in_progress", "--actor", "agent:builder-1")
	must("ticket", "request-review", "APP-1", "--actor", "agent:builder-1")
	must("ticket", "reject", "APP-1", "--actor", "agent:reviewer-1", "--reason", "missing test coverage")

	history := must("ticket", "history", "APP-1", "--json")
	if !strings.Contains(history, "ticket.rejected") || !strings.Contains(history, "missing test coverage") {
		t.Fatalf("history should keep reject event and reason: %s", history)
	}

	must("ticket", "request-review", "APP-1", "--actor", "agent:builder-1")
	must("ticket", "approve", "APP-1", "--actor", "agent:reviewer-1")
	ownerQueue := must("owner-queue", "--pretty")
	if !strings.Contains(ownerQueue, "APP-1") {
		t.Fatalf("owner queue missing approved dual_gate ticket: %s", ownerQueue)
	}
	if _, err := runCLI(t, "ticket", "complete", "APP-1", "--actor", "agent:reviewer-1"); err == nil {
		t.Fatal("expected reviewer completion to fail in dual_gate")
	}
	must("ticket", "complete", "APP-1", "--actor", "human:owner")
	inspect := must("inspect", "APP-1", "--actor", "human:owner", "--json")
	if !strings.Contains(inspect, "\"status\": \"done\"") {
		t.Fatalf("inspect should show done after owner completion: %s", inspect)
	}
}

func TestExpiredClaimCanBeSweptAndReclaimed(t *testing.T) {
	withTempWorkspace(t)

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("command failed %v: %v\n%s", args, err, out)
		}
		return out
	}

	must("init")
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Lease", "--type", "task", "--actor", "human:owner")
	must("ticket", "move", "APP-1", "ready", "--actor", "human:owner")
	must("ticket", "claim", "APP-1", "--actor", "agent:builder-1")

	store := mdstore.TicketStore{RootDir: mustGetwd(t)}
	ticket, err := store.GetTicket(context.Background(), "APP-1")
	if err != nil {
		t.Fatalf("load claimed ticket: %v", err)
	}
	ticket.Lease.ExpiresAt = time.Now().UTC().Add(-2 * time.Hour)
	ticket.Lease.LastHeartbeatAt = ticket.Lease.ExpiresAt
	ticket.UpdatedAt = time.Now().UTC().Add(-2 * time.Hour)
	if err := store.UpdateTicket(context.Background(), ticket); err != nil {
		t.Fatalf("persist expired lease: %v", err)
	}

	sweep := must("sweep", "--actor", "human:owner", "--reason", "cleanup")
	if !strings.Contains(sweep, "expired 1 lease(s)") {
		t.Fatalf("unexpected sweep output: %s", sweep)
	}
	must("ticket", "claim", "APP-1", "--actor", "agent:builder-2")
	who := must("who", "--json")
	if !strings.Contains(who, "agent:builder-2") || strings.Contains(who, "agent:builder-1") {
		t.Fatalf("who should show only the reclaimed lease holder: %s", who)
	}
}

func TestPolicyPrecedenceAcrossProjectEpicAndTicket(t *testing.T) {
	withTempWorkspace(t)

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("command failed %v: %v\n%s", args, err, out)
		}
		return out
	}

	must("init")
	must("project", "create", "APP", "App Project")
	must("project", "policy", "set", "APP", "--completion-mode", "owner_gate", "--lease-ttl", "45", "--required-reviewer", "agent:reviewer-1", "--actor", "human:owner")
	must("ticket", "create", "--project", "APP", "--title", "Epic", "--type", "epic", "--actor", "human:owner")
	must("ticket", "policy", "set", "APP-1", "--completion-mode", "dual_gate", "--required-reviewer", "agent:reviewer-2", "--actor", "human:owner")
	must("ticket", "create", "--project", "APP", "--title", "Child", "--type", "task", "--parent", "APP-1", "--actor", "human:owner")

	policyJSON := must("ticket", "policy", "get", "APP-2", "--json")
	var payload struct {
		EffectivePolicy struct {
			CompletionMode   string `json:"completion_mode"`
			RequiredReviewer string `json:"required_reviewer"`
		} `json:"effective_policy"`
	}
	if err := json.Unmarshal([]byte(policyJSON), &payload); err != nil {
		t.Fatalf("parse ticket policy output: %v\n%s", err, policyJSON)
	}
	if payload.EffectivePolicy.CompletionMode != string(contracts.CompletionModeDualGate) || payload.EffectivePolicy.RequiredReviewer != "agent:reviewer-2" {
		t.Fatalf("unexpected effective policy from project+epic inheritance: %#v", payload.EffectivePolicy)
	}

	must("ticket", "policy", "set", "APP-2", "--completion-mode", "review_gate", "--actor", "human:owner")
	policyJSON = must("ticket", "policy", "get", "APP-2", "--json")
	if err := json.Unmarshal([]byte(policyJSON), &payload); err != nil {
		t.Fatalf("parse ticket override output: %v\n%s", err, policyJSON)
	}
	if payload.EffectivePolicy.CompletionMode != string(contracts.CompletionModeReviewGate) {
		t.Fatalf("ticket override should win precedence: %#v", payload.EffectivePolicy)
	}
}

func TestFixtureUpgradeRepairReindexAndIntegrations(t *testing.T) {
	root := t.TempDir()
	if err := testutil.CopyDir(testutil.FixturePath("app_sample"), root); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir fixture: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("command failed %v: %v\n%s", args, err, out)
		}
		return out
	}

	doctor := must("doctor", "--repair", "--pretty")
	if !strings.Contains(doctor, "doctor ok:") {
		t.Fatalf("doctor should pass on repaired fixture: %s", doctor)
	}
	history := must("ticket", "history", "APP-1", "--json")
	if !strings.Contains(history, "\"schema_version\": 1") {
		t.Fatalf("fixture history should keep legacy schema event: %s", history)
	}
	must("reindex")
	board := must("board", "--project", "APP", "--pretty")
	if !strings.Contains(board, "APP-1") {
		t.Fatalf("board should survive fixture reindex: %s", board)
	}
	must("integrations", "install", "codex")
	must("integrations", "install", "claude")
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatalf("expected AGENTS.md after install: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "CLAUDE.md")); err != nil {
		t.Fatalf("expected CLAUDE.md after install: %v", err)
	}
}

func TestDoctorRepairIsIdempotentAfterJournalReplayAndProjectionCorruption(t *testing.T) {
	withTempWorkspace(t)

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("command failed %v: %v\n%s", args, err, out)
		}
		return out
	}

	must("init")
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Repair me", "--type", "task", "--actor", "human:owner")

	root := mustGetwd(t)
	store := mdstore.TicketStore{RootDir: root}
	ticket, err := store.GetTicket(context.Background(), "APP-1")
	if err != nil {
		t.Fatalf("load ticket: %v", err)
	}
	now := time.Now().UTC()
	ticket.Status = contracts.StatusReady
	ticket.UpdatedAt = now
	if err := store.UpdateTicket(context.Background(), ticket); err != nil {
		t.Fatalf("update ticket: %v", err)
	}
	journal := service.MutationJournal{Root: root, Clock: func() time.Time { return now }}
	entry := service.MutationJournalEntry{
		Purpose:       "repair replay",
		CanonicalKind: "ticket_snapshot",
		Event: contracts.Event{
			EventID:       2,
			Timestamp:     now,
			Actor:         contracts.Actor("human:owner"),
			Type:          contracts.EventTicketUpdated,
			Project:       "APP",
			TicketID:      "APP-1",
			Payload:       ticket,
			SchemaVersion: contracts.CurrentSchemaVersion,
		},
		Stage: service.MutationStageCanonicalWritten,
	}
	if err := testutil.SeedPendingMutationJournal(root, journal, entry); err != nil {
		t.Fatalf("seed pending journal: %v", err)
	}
	if err := testutil.CorruptProjection(root); err != nil {
		t.Fatalf("corrupt projection: %v", err)
	}

	first := must("doctor", "--repair", "--json")
	second := must("doctor", "--repair", "--json")
	if !strings.Contains(first, "\"repair_pending\": 1") {
		t.Fatalf("expected first repair to see one pending journal entry: %s", first)
	}
	if !strings.Contains(second, "\"repair_pending\": 0") {
		t.Fatalf("expected second repair to be idempotent: %s", second)
	}

	history := must("ticket", "history", "APP-1", "--json")
	if strings.Count(history, "\"event_id\": 2") != 1 {
		t.Fatalf("expected replayed event to exist exactly once: %s", history)
	}
	board := must("board", "--pretty")
	if !strings.Contains(board, "APP-1") {
		t.Fatalf("expected board to recover after repair: %s", board)
	}
}

func TestPackagedReleaseRehearsalInstallsAndRunsSmokeFlow(t *testing.T) {
	repoRoot := testutil.RepoRoot()
	distDir := t.TempDir()
	workDir := t.TempDir()
	binDir := t.TempDir()
	version := "v1.6.0-rc1"
	archive := filepath.Join(distDir, packagedArchiveName(version))

	build := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", filepath.Join(distDir, "tracker"), "./cmd/tracker")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build packaged tracker: %v\n%s", err, output)
	}
	tarCmd := exec.Command("tar", "-czf", archive, "-C", distDir, "tracker")
	if output, err := tarCmd.CombinedOutput(); err != nil {
		t.Fatalf("tar packaged tracker: %v\n%s", err, output)
	}
	archiveBytes, err := os.ReadFile(archive)
	if err != nil {
		t.Fatalf("read packaged archive: %v", err)
	}
	sum := sha256.Sum256(archiveBytes)
	if err := os.WriteFile(filepath.Join(distDir, "checksums.txt"), []byte(fmt.Sprintf("%x  %s\n", sum, filepath.Base(archive))), 0o644); err != nil {
		t.Fatalf("write checksums.txt: %v", err)
	}
	server := httptest.NewServer(http.FileServer(http.Dir(distDir)))
	defer server.Close()

	verify := exec.Command("sh", filepath.Join(repoRoot, "scripts", "verify-release.sh"), archive)
	verify.Dir = repoRoot
	verify.Env = append(os.Environ(),
		"VERSION="+version,
		"RELEASE_BASE_URL="+server.URL,
		"VERIFY_ATTESTATIONS=0",
	)
	if output, err := verify.CombinedOutput(); err != nil {
		t.Fatalf("verify packaged tracker: %v\n%s", err, output)
	}

	install := exec.Command("sh", filepath.Join(repoRoot, "scripts", "install.sh"))
	install.Dir = repoRoot
	install.Env = append(os.Environ(),
		"VERSION="+version,
		"BIN_DIR="+binDir,
		"RELEASE_BASE_URL="+server.URL,
	)
	if output, err := install.CombinedOutput(); err != nil {
		t.Fatalf("install packaged tracker: %v\n%s", err, output)
	}

	if err := os.WriteFile(filepath.Join(distDir, "checksums.txt"), []byte(strings.Repeat("0", 64)+"  "+filepath.Base(archive)+"\n"), 0o644); err != nil {
		t.Fatalf("write bad checksums.txt: %v", err)
	}
	badInstall := exec.Command("sh", filepath.Join(repoRoot, "scripts", "install.sh"))
	badInstall.Dir = repoRoot
	badInstall.Env = append(os.Environ(),
		"VERSION="+version,
		"BIN_DIR="+t.TempDir(),
		"RELEASE_BASE_URL="+server.URL,
	)
	if output, err := badInstall.CombinedOutput(); err == nil || !strings.Contains(string(output), "checksum mismatch") {
		t.Fatalf("expected install checksum mismatch, err=%v output=%s", err, output)
	}

	trackerBin := filepath.Join(binDir, "tracker")
	runInstalled := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(trackerBin, args...)
		cmd.Dir = workDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("installed tracker %v failed: %v\n%s", args, err, output)
		}
		return string(output)
	}

	gitRunInDir(t, workDir, "init", "-b", "main")
	gitRunInDir(t, workDir, "config", "user.email", "atlas@example.com")
	gitRunInDir(t, workDir, "config", "user.name", "Atlas")
	if err := os.WriteFile(filepath.Join(workDir, "README.md"), []byte("# atlas\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	gitRunInDir(t, workDir, "add", "README.md")
	gitRunInDir(t, workDir, "commit", "-m", "init")

	runInstalled("init")
	runInstalled("project", "create", "APP", "App Project")
	runInstalled("ticket", "create", "--project", "APP", "--title", "Smoke", "--type", "task", "--reviewer", "agent:reviewer-1", "--actor", "human:owner")
	runInstalled("ticket", "move", "APP-1", "ready", "--actor", "human:owner")
	runInstalled("agent", "create", "builder-1", "--name", "Builder One", "--provider", "codex", "--capability", "go", "--actor", "human:owner")

	dispatchJSON := runInstalled("run", "dispatch", "APP-1", "--agent", "builder-1", "--actor", "human:owner", "--json")
	var dispatch struct {
		Payload struct {
			RunID string `json:"run_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(dispatchJSON), &dispatch); err != nil {
		t.Fatalf("parse packaged dispatch: %v\n%s", err, dispatchJSON)
	}
	runID := dispatch.Payload.RunID
	runInstalled("run", "launch", runID, "--actor", "human:owner")
	runInstalled("run", "start", runID, "--actor", "human:owner")
	runInstalled("ticket", "move", "APP-1", "in_progress", "--actor", "human:owner")
	runInstalled("run", "checkpoint", runID, "--title", "Smoke checkpoint", "--body", "runtime + worktree ready", "--actor", "human:owner")
	runInstalled("run", "evidence", "add", runID, "--type", "note", "--title", "Smoke evidence", "--body", "packaged rehearsal", "--actor", "human:owner")
	changeJSON := runInstalled("change", "create", runID, "--actor", "human:owner", "--json")
	var createdChange struct {
		Payload struct {
			Change struct {
				ChangeID string `json:"change_id"`
			} `json:"change"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(changeJSON), &createdChange); err != nil {
		t.Fatalf("parse packaged change create: %v\n%s", err, changeJSON)
	}
	if createdChange.Payload.Change.ChangeID == "" {
		t.Fatalf("expected packaged change id, got %s", changeJSON)
	}
	runInstalled("change", "status", createdChange.Payload.Change.ChangeID, "--json")
	runInstalled("run", "handoff", runID, "--next-actor", "agent:reviewer-1", "--next-gate", "review", "--actor", "human:owner")

	approvalsJSON := runInstalled("approvals", "--json")
	var approvals struct {
		Items []struct {
			Gate struct {
				GateID string `json:"gate_id"`
			} `json:"gate"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(approvalsJSON), &approvals); err != nil {
		t.Fatalf("parse packaged approvals: %v\n%s", err, approvalsJSON)
	}
	if len(approvals.Items) == 0 {
		t.Fatalf("expected packaged approval gate, got %s", approvalsJSON)
	}
	gateID := approvals.Items[0].Gate.GateID
	runInstalled("gate", "approve", gateID, "--actor", "agent:reviewer-1", "--reason", "packaged rehearsal")
	runInstalled("run", "complete", runID, "--actor", "human:owner", "--summary", "smoke complete")
	runInstalled("ticket", "request-review", "APP-1", "--actor", "agent:builder-1")
	runInstalled("ticket", "approve", "APP-1", "--actor", "agent:reviewer-1")
	runInstalled("ticket", "complete", "APP-1", "--actor", "human:owner")
	runInstalled("export", "create", "--scope", "workspace", "--actor", "human:owner")
	runtimeDir := filepath.Join(workDir, ".tracker", "runtime", runID)
	old := time.Date(2020, 1, 1, 1, 1, 0, 0, time.UTC)
	if err := filepath.Walk(runtimeDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		return os.Chtimes(path, old, old)
	}); err != nil {
		t.Fatalf("backdate runtime dir: %v", err)
	}
	runInstalled("archive", "plan", "--target", "runtime", "--project", "APP")
	runInstalled("archive", "apply", "--target", "runtime", "--project", "APP", "--yes", "--actor", "human:owner")
	archiveJSON := runInstalled("archive", "list", "--target", "runtime", "--json")
	var archives struct {
		Items []struct {
			ArchiveID string `json:"archive_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(archiveJSON), &archives); err != nil {
		t.Fatalf("parse packaged archive list: %v\n%s", err, archiveJSON)
	}
	if len(archives.Items) == 0 || archives.Items[0].ArchiveID == "" {
		t.Fatalf("expected packaged runtime archive, got %s", archiveJSON)
	}
	runInstalled("archive", "restore", archives.Items[0].ArchiveID, "--actor", "human:owner")
	runInstalled("run", "cleanup", runID, "--actor", "human:owner")

	inspect := runInstalled("inspect", "APP-1", "--actor", "human:owner", "--json")
	if !strings.Contains(inspect, "\"status\": \"done\"") {
		t.Fatalf("expected packaged inspect to show done: %s", inspect)
	}
	if _, err := os.Stat(filepath.Join(workDir, ".tracker", "runtime", runID)); !os.IsNotExist(err) {
		t.Fatalf("expected packaged cleanup to remove runtime dir, err=%v", err)
	}

	runInstalledAt := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command(trackerBin, args...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("installed tracker in %s %v failed: %v\n%s", dir, args, err, output)
		}
		return string(output)
	}
	runInstalledErrAt := func(dir string, args ...string) (string, error) {
		t.Helper()
		cmd := exec.Command(trackerBin, args...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		return string(output), err
	}

	gitRemoteRoot := t.TempDir()
	gitRemoteDir := filepath.Join(gitRemoteRoot, "sync-remote.git")
	gitRunInDir(t, gitRemoteRoot, "init", "--bare", gitRemoteDir)

	workspaceB := t.TempDir()
	workspaceC := t.TempDir()
	workspaceD := t.TempDir()

	for _, dir := range []string{workspaceB, workspaceC, workspaceD} {
		runInstalledAt(dir, "init")
	}

	runInstalled("collaborator", "add", "rev-1", "--name", "Rev One", "--actor-map", "agent:reviewer-1", "--actor", "human:owner")
	runInstalled("collaborator", "trust", "rev-1", "--actor", "human:owner")
	runInstalled("membership", "bind", "rev-1", "--scope-kind", "project", "--scope-id", "APP", "--role", "reviewer", "--actor", "human:owner")
	runInstalled("ticket", "create", "--project", "APP", "--title", "Shared sync", "--type", "task", "--actor", "human:owner")
	runInstalled("ticket", "comment", "APP-2", "--body", "loop in @rev-1", "--actor", "human:owner")
	runInstalled("remote", "add", "origin", "--kind", "git", "--location", gitRemoteDir, "--default-action", "push", "--actor", "human:owner")

	sourcePushJSON := runInstalled("sync", "push", "--remote", "origin", "--actor", "human:owner", "--json")
	var sourcePush struct {
		Payload struct {
			Publication struct {
				WorkspaceID string `json:"workspace_id"`
			} `json:"publication"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(sourcePushJSON), &sourcePush); err != nil {
		t.Fatalf("parse packaged source sync push: %v\n%s", err, sourcePushJSON)
	}
	if sourcePush.Payload.Publication.WorkspaceID == "" {
		t.Fatalf("expected source workspace id, got %s", sourcePushJSON)
	}

	runInstalledAt(workspaceB, "remote", "add", "origin", "--kind", "git", "--location", gitRemoteDir, "--default-action", "pull", "--actor", "human:owner")
	runInstalledAt(workspaceB, "sync", "pull", "--remote", "origin", "--workspace", sourcePush.Payload.Publication.WorkspaceID, "--actor", "human:owner")
	mentionsB := runInstalledAt(workspaceB, "mentions", "list", "--collaborator", "rev-1", "--json")
	if !strings.Contains(mentionsB, "\"collaborator_id\": \"rev-1\"") {
		t.Fatalf("expected synced canonical mention in workspace B: %s", mentionsB)
	}
	runInstalledAt(workspaceB, "ticket", "edit", "APP-2", "--title", "Shared sync from B", "--actor", "human:owner")
	remotePushJSON := runInstalledAt(workspaceB, "sync", "push", "--remote", "origin", "--actor", "human:owner", "--json")
	var remotePush struct {
		Payload struct {
			Publication struct {
				WorkspaceID string `json:"workspace_id"`
			} `json:"publication"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(remotePushJSON), &remotePush); err != nil {
		t.Fatalf("parse packaged remote sync push: %v\n%s", err, remotePushJSON)
	}
	if remotePush.Payload.Publication.WorkspaceID == "" {
		t.Fatalf("expected workspace B id, got %s", remotePushJSON)
	}

	runInstalled("ticket", "edit", "APP-2", "--title", "Shared sync from A", "--actor", "human:owner")
	if output, err := runInstalledErrAt(workDir, "sync", "pull", "--remote", "origin", "--workspace", remotePush.Payload.Publication.WorkspaceID, "--actor", "human:owner", "--json"); err == nil {
		t.Fatalf("expected packaged sync pull conflict, got success output=%s", output)
	}
	conflictListJSON := runInstalled("conflict", "list", "--json")
	var conflictList struct {
		Items []struct {
			ConflictID string `json:"conflict_id"`
			EntityKind string `json:"entity_kind"`
			Status     string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(conflictListJSON), &conflictList); err != nil {
		t.Fatalf("parse packaged conflict list: %v\n%s", err, conflictListJSON)
	}
	conflictID := ""
	for _, item := range conflictList.Items {
		if item.EntityKind == "ticket" && item.Status == "open" {
			conflictID = item.ConflictID
			break
		}
	}
	if conflictID == "" {
		t.Fatalf("expected open packaged ticket conflict, got %s", conflictListJSON)
	}
	runInstalled("conflict", "resolve", conflictID, "--resolution", "use_remote", "--actor", "human:owner")
	ticketAfterResolve := runInstalled("ticket", "view", "APP-2", "--json")
	if !strings.Contains(ticketAfterResolve, "Shared sync from B") {
		t.Fatalf("expected remote resolution to win for APP-2, got %s", ticketAfterResolve)
	}

	runInstalledAt(workspaceB, "ticket", "create", "--project", "APP", "--title", "Shared from B", "--type", "task", "--actor", "human:owner")
	workspaceBPush := runInstalledAt(workspaceB, "sync", "push", "--remote", "origin", "--actor", "human:owner", "--json")
	if err := json.Unmarshal([]byte(workspaceBPush), &remotePush); err != nil {
		t.Fatalf("parse workspace B follow-up push: %v\n%s", err, workspaceBPush)
	}

	runInstalledAt(workspaceC, "remote", "add", "origin", "--kind", "git", "--location", gitRemoteDir, "--default-action", "pull", "--actor", "human:owner")
	runInstalledAt(workspaceC, "sync", "pull", "--remote", "origin", "--workspace", remotePush.Payload.Publication.WorkspaceID, "--actor", "human:owner")
	ticketsC := runInstalledAt(workspaceC, "ticket", "list", "--project", "APP", "--json")
	if !strings.Contains(ticketsC, "\"id\": \"APP-3\"") {
		t.Fatalf("expected workspace C to receive APP-3, got %s", ticketsC)
	}
	runInstalledAt(workspaceC, "ticket", "create", "--project", "APP", "--title", "Shared from C", "--type", "task", "--actor", "human:owner")
	workspaceCPush := runInstalledAt(workspaceC, "sync", "push", "--remote", "origin", "--actor", "human:owner", "--json")
	var cPush struct {
		Payload struct {
			Publication struct {
				WorkspaceID string `json:"workspace_id"`
			} `json:"publication"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(workspaceCPush), &cPush); err != nil {
		t.Fatalf("parse workspace C push: %v\n%s", err, workspaceCPush)
	}
	runInstalled("sync", "pull", "--remote", "origin", "--workspace", cPush.Payload.Publication.WorkspaceID, "--actor", "human:owner")
	ticketsA := runInstalled("ticket", "list", "--project", "APP", "--json")
	for _, ticketID := range []string{"APP-3", "APP-4"} {
		if !strings.Contains(ticketsA, "\"id\": \""+ticketID+"\"") {
			t.Fatalf("expected workspace A to converge with %s, got %s", ticketID, ticketsA)
		}
	}

	bundleCreateJSON := runInstalled("bundle", "create", "--actor", "human:owner", "--json")
	var bundleCreated struct {
		Payload struct {
			Job struct {
				BundleRef string `json:"bundle_ref"`
			} `json:"job"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(bundleCreateJSON), &bundleCreated); err != nil {
		t.Fatalf("parse packaged bundle create: %v\n%s", err, bundleCreateJSON)
	}
	if bundleCreated.Payload.Job.BundleRef == "" {
		t.Fatalf("expected bundle ref, got %s", bundleCreateJSON)
	}
	bundleVerifyJSON := runInstalled("bundle", "verify", bundleCreated.Payload.Job.BundleRef, "--actor", "human:owner", "--json")
	if !strings.Contains(bundleVerifyJSON, "\"verified\": true") {
		t.Fatalf("expected packaged bundle verify to pass, got %s", bundleVerifyJSON)
	}
	runInstalledAt(workspaceD, "bundle", "import", bundleCreated.Payload.Job.BundleRef, "--actor", "human:owner")
	ticketsD := runInstalledAt(workspaceD, "ticket", "list", "--project", "APP", "--json")
	if !strings.Contains(ticketsD, "\"id\": \"APP-4\"") {
		t.Fatalf("expected bundle import workspace to receive converged tickets, got %s", ticketsD)
	}
	mentionsD := runInstalledAt(workspaceD, "mentions", "list", "--collaborator", "rev-1", "--json")
	if !strings.Contains(mentionsD, "\"collaborator_id\": \"rev-1\"") {
		t.Fatalf("expected bundle import workspace to receive mentions, got %s", mentionsD)
	}

	runInstalled("ticket", "move", "APP-3", "ready", "--actor", "human:owner")
	dispatchSyncedJSON := runInstalled("run", "dispatch", "APP-3", "--agent", "builder-1", "--actor", "human:owner", "--json")
	if err := json.Unmarshal([]byte(dispatchSyncedJSON), &dispatch); err != nil {
		t.Fatalf("parse packaged synced dispatch: %v\n%s", err, dispatchSyncedJSON)
	}
	syncedRunID := dispatch.Payload.RunID
	runInstalled("run", "launch", syncedRunID, "--actor", "human:owner")
	runInstalled("run", "start", syncedRunID, "--actor", "human:owner")
	runInstalled("ticket", "move", "APP-3", "in_progress", "--actor", "human:owner")
	runInstalled("run", "checkpoint", syncedRunID, "--title", "Synced checkpoint", "--body", "runtime after sync", "--actor", "human:owner")
	runInstalled("run", "handoff", syncedRunID, "--next-actor", "agent:reviewer-1", "--next-gate", "review", "--actor", "human:owner")
	syncedApprovalsJSON := runInstalled("approvals", "--json")
	if err := json.Unmarshal([]byte(syncedApprovalsJSON), &approvals); err != nil {
		t.Fatalf("parse packaged synced approvals: %v\n%s", err, syncedApprovalsJSON)
	}
	if len(approvals.Items) == 0 {
		t.Fatalf("expected synced approval gate, got %s", syncedApprovalsJSON)
	}
	runInstalled("gate", "approve", approvals.Items[0].Gate.GateID, "--actor", "agent:reviewer-1", "--reason", "synced archive rehearsal")
	runInstalled("run", "complete", syncedRunID, "--actor", "human:owner", "--summary", "synced flow complete")

	syncedRuntimeDir := filepath.Join(workDir, ".tracker", "runtime", syncedRunID)
	if err := filepath.Walk(syncedRuntimeDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		return os.Chtimes(path, old, old)
	}); err != nil {
		t.Fatalf("backdate synced runtime dir: %v", err)
	}
	runInstalled("archive", "apply", "--target", "runtime", "--project", "APP", "--yes", "--actor", "human:owner")
	archiveAfterSyncJSON := runInstalled("archive", "list", "--target", "runtime", "--json")
	var archiveAfterSync struct {
		Items []struct {
			ArchiveID   string   `json:"archive_id"`
			SourcePaths []string `json:"source_paths"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(archiveAfterSyncJSON), &archiveAfterSync); err != nil {
		t.Fatalf("parse packaged synced archive list: %v\n%s", err, archiveAfterSyncJSON)
	}
	syncedArchiveID := ""
	targetRuntimePath := filepath.ToSlash(filepath.Join(".tracker", "runtime", syncedRunID))
	for _, item := range archiveAfterSync.Items {
		for _, path := range item.SourcePaths {
			if path == targetRuntimePath {
				syncedArchiveID = item.ArchiveID
				break
			}
		}
		if syncedArchiveID != "" {
			break
		}
	}
	if syncedArchiveID == "" {
		t.Fatalf("expected synced runtime archive, got %s", archiveAfterSyncJSON)
	}
	runInstalled("compact", "--yes", "--actor", "human:owner")
	runInstalled("archive", "restore", syncedArchiveID, "--actor", "human:owner")
	runInstalled("reindex")
	doctorJSON := runInstalled("doctor", "--repair", "--json")
	if !strings.Contains(doctorJSON, "\"repair_pending\": 0") {
		t.Fatalf("expected doctor repair to settle after synced archive flow, got %s", doctorJSON)
	}
}

func TestOrchestrationFlowSurvivesReindexAndMatchesReadSurfaces(t *testing.T) {
	withTempWorkspace(t)
	gitRunCLI(t, "init", "-b", "main")
	gitRunCLI(t, "config", "user.email", "atlas@example.com")
	gitRunCLI(t, "config", "user.name", "Atlas")
	writeGitFile(t, "README.md", "# atlas\n")
	gitRunCLI(t, "add", "README.md")
	gitRunCLI(t, "commit", "-m", "init")

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("command failed %v: %v\n%s", args, err, out)
		}
		return out
	}

	must("init")
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Orchestrate", "--type", "task", "--reviewer", "agent:reviewer-1", "--actor", "human:owner")
	must("ticket", "move", "APP-1", "ready", "--actor", "human:owner")
	must("agent", "create", "builder-1", "--name", "Builder One", "--provider", "codex", "--capability", "go", "--actor", "human:owner")

	dispatchJSON := must("run", "dispatch", "APP-1", "--agent", "builder-1", "--actor", "human:owner", "--json")
	var dispatch struct {
		Payload struct {
			RunID string `json:"run_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(dispatchJSON), &dispatch); err != nil {
		t.Fatalf("parse dispatch: %v\n%s", err, dispatchJSON)
	}
	runID := dispatch.Payload.RunID

	must("run", "launch", runID, "--actor", "human:owner")
	must("run", "start", runID, "--actor", "human:owner")
	must("ticket", "move", "APP-1", "in_progress", "--actor", "human:owner")
	must("run", "checkpoint", runID, "--title", "Checkpoint", "--body", "worktree ready", "--actor", "human:owner")
	must("run", "evidence", "add", runID, "--type", "note", "--title", "Implementation note", "--body", "orchestration proof", "--actor", "human:owner")
	must("run", "handoff", runID, "--next-actor", "agent:reviewer-1", "--next-gate", "review", "--next-status", "in_review", "--actor", "human:owner")

	approvalsJSON := must("approvals", "--json")
	inboxJSON := must("inbox", "--json")
	runViewJSON := must("run", "view", runID, "--json")
	inspectJSON := must("inspect", "APP-1", "--actor", "human:owner", "--json")
	evidenceJSON := must("evidence", "list", runID, "--json")
	if !strings.Contains(approvalsJSON, "gate_") || !strings.Contains(inboxJSON, "gate:") {
		t.Fatalf("expected gate to appear in approvals and inbox\napprovals=%s\ninbox=%s", approvalsJSON, inboxJSON)
	}
	if !strings.Contains(runViewJSON, runID) || !strings.Contains(evidenceJSON, "Implementation note") || !strings.Contains(inspectJSON, "APP-1") {
		t.Fatalf("expected orchestration surfaces to agree\nrun=%s\nevidence=%s\ninspect=%s", runViewJSON, evidenceJSON, inspectJSON)
	}

	var approvals struct {
		Items []struct {
			Gate struct {
				GateID string `json:"gate_id"`
			} `json:"gate"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(approvalsJSON), &approvals); err != nil {
		t.Fatalf("parse approvals: %v\n%s", err, approvalsJSON)
	}
	if len(approvals.Items) == 0 {
		t.Fatalf("expected approval items: %s", approvalsJSON)
	}
	gateID := approvals.Items[0].Gate.GateID

	var runView struct {
		Payload struct {
			Handoffs []struct {
				HandoffID string `json:"handoff_id"`
			} `json:"handoffs"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(runViewJSON), &runView); err != nil {
		t.Fatalf("parse run view: %v\n%s", err, runViewJSON)
	}
	if len(runView.Payload.Handoffs) == 0 {
		t.Fatalf("expected handoff in run detail: %s", runViewJSON)
	}
	handoffID := runView.Payload.Handoffs[0].HandoffID

	must("gate", "approve", gateID, "--actor", "agent:reviewer-1", "--reason", "looks good")
	must("run", "complete", runID, "--actor", "human:owner", "--summary", "implemented")
	must("ticket", "request-review", "APP-1", "--actor", "agent:builder-1")
	must("ticket", "approve", "APP-1", "--actor", "agent:reviewer-1")
	must("ticket", "complete", "APP-1", "--actor", "human:owner")
	must("run", "cleanup", runID, "--actor", "human:owner")
	must("reindex")

	postRun := must("run", "view", runID, "--json")
	postEvidence := must("evidence", "list", runID, "--json")
	postHandoff := must("handoff", "view", handoffID, "--json")
	postWorktree := must("worktree", "view", runID, "--json")
	postInspect := must("inspect", "APP-1", "--actor", "human:owner", "--json")
	if !strings.Contains(postRun, "\"status\": \"cleaned_up\"") || !strings.Contains(postInspect, "\"status\": \"done\"") {
		t.Fatalf("expected final run/ticket states to survive reindex\nrun=%s\ninspect=%s", postRun, postInspect)
	}
	if !strings.Contains(postEvidence, "Implementation note") || !strings.Contains(postHandoff, handoffID) {
		t.Fatalf("expected evidence and handoff history to survive reindex\nevidence=%s\nhandoff=%s", postEvidence, postHandoff)
	}
	if !strings.Contains(postWorktree, "\"present\": false") {
		t.Fatalf("expected cleaned worktree to stay absent after reindex: %s", postWorktree)
	}
}

func TestLongSessionSoakKeepsReadSurfacesConsistent(t *testing.T) {
	withTempWorkspace(t)

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("command failed %v: %v\n%s", args, err, out)
		}
		return out
	}

	must("init")
	must("project", "create", "APP", "App Project")
	must("project", "policy", "set", "APP", "--completion-mode", "dual_gate", "--required-reviewer", "agent:reviewer-1", "--allowed-workers", "agent:builder-1", "--actor", "human:owner")
	for i := 1; i <= 6; i++ {
		must("ticket", "create", "--project", "APP", "--title", "Soak", "--type", "task", "--reviewer", "agent:reviewer-1", "--actor", "human:owner")
		id := "APP-" + strconv.Itoa(i)
		must("ticket", "move", id, "ready", "--actor", "human:owner")
		must("ticket", "claim", id, "--actor", "agent:builder-1")
		must("ticket", "move", id, "in_progress", "--actor", "agent:builder-1")
		must("ticket", "comment", id, "--body", "loop "+strconv.Itoa(i), "--actor", "agent:builder-1")
		must("ticket", "request-review", id, "--actor", "agent:builder-1")
		if i%2 == 0 {
			must("ticket", "approve", id, "--actor", "agent:reviewer-1")
			must("ticket", "complete", id, "--actor", "human:owner")
		} else {
			must("ticket", "reject", id, "--actor", "agent:reviewer-1", "--reason", "rework")
		}
	}

	for _, id := range []string{"APP-1", "APP-2", "APP-3", "APP-4", "APP-5", "APP-6"} {
		view := must("ticket", "view", id, "--json")
		inspect := must("inspect", id, "--actor", "human:owner", "--json")
		history := must("ticket", "history", id, "--json")
		if !strings.Contains(inspect, id) || !strings.Contains(view, id) || !strings.Contains(history, id) {
			t.Fatalf("surface mismatch for %s\nview=%s\ninspect=%s\nhistory=%s", id, view, inspect, history)
		}
		if strings.Contains(view, "\"status\": \"done\"") != strings.Contains(inspect, "\"status\": \"done\"") {
			t.Fatalf("status drift between view and inspect for %s\nview=%s\ninspect=%s", id, view, inspect)
		}
	}

	queue := must("queue", "--actor", "agent:builder-1", "--json")
	next := must("next", "--actor", "agent:builder-1", "--json")
	board := must("board", "--json")
	for _, id := range []string{"APP-2", "APP-4", "APP-6"} {
		if !strings.Contains(queue, id) || !strings.Contains(next, id) {
			t.Fatalf("expected approved dual-gate tickets to remain in awaiting_owner surfaces\nqueue=%s\nnext=%s", queue, next)
		}
		if !strings.Contains(board, id) {
			t.Fatalf("expected board to retain completed ticket %s: %s", id, board)
		}
	}
	for _, id := range []string{"APP-1", "APP-3", "APP-5"} {
		if !strings.Contains(board, id) {
			t.Fatalf("expected board to retain rejected in_progress ticket %s: %s", id, board)
		}
	}
	must("reindex")
	postReindexQueue := must("queue", "--actor", "agent:builder-1", "--json")
	for _, id := range []string{"APP-2", "APP-4", "APP-6"} {
		if !strings.Contains(postReindexQueue, id) {
			t.Fatalf("expected awaiting_owner queue entries to survive reindex: %s", postReindexQueue)
		}
	}
	postReindexBoard := must("board", "--json")
	for _, id := range []string{"APP-1", "APP-3", "APP-5", "APP-2", "APP-4", "APP-6"} {
		if !strings.Contains(postReindexBoard, id) {
			t.Fatalf("expected board to survive reindex after soak: %s", postReindexBoard)
		}
	}
	if strings.Contains(postReindexQueue, "APP-1") || strings.Contains(postReindexQueue, "APP-3") || strings.Contains(postReindexQueue, "APP-5") {
		t.Fatalf("expected queue to survive reindex after soak: %s", postReindexQueue)
	}
}

func TestReindexDoesNotRerunAutomation(t *testing.T) {
	withTempWorkspace(t)

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("command failed %v: %v\n%s", args, err, out)
		}
		return out
	}

	must("init")
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Replay safe", "--type", "task", "--actor", "human:owner")
	must("automation", "create", "comment-on-comment", "--on", "ticket.commented", "--action", "comment:automation follow-up")
	must("ticket", "comment", "APP-1", "--body", "seed", "--actor", "agent:builder-1")

	historyBefore := must("ticket", "history", "APP-1", "--json")
	if strings.Count(historyBefore, "\"type\": \"ticket.commented\"") != 2 {
		t.Fatalf("expected one human comment and one automation comment before reindex: %s", historyBefore)
	}

	must("reindex")

	historyAfter := must("ticket", "history", "APP-1", "--json")
	if strings.Count(historyAfter, "\"type\": \"ticket.commented\"") != 2 {
		t.Fatalf("expected reindex to avoid rerunning automation: %s", historyAfter)
	}
}

func TestDoctorRepairDoesNotDuplicateNotificationsAndUsesWorkspaceLock(t *testing.T) {
	withTempWorkspace(t)

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("command failed %v: %v\n%s", args, err, out)
		}
		return out
	}

	must("init")
	must("config", "set", "notifications.file_enabled", "true")
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Repair without spam", "--type", "task", "--actor", "human:owner")
	must("ticket", "move", "APP-1", "ready", "--actor", "human:owner")
	must("ticket", "move", "APP-1", "in_progress", "--actor", "human:owner")
	must("ticket", "request-review", "APP-1", "--actor", "human:owner")

	root := mustGetwd(t)
	before := must("notify", "log", "--json")

	store := mdstore.TicketStore{RootDir: root, Clock: defaultNow}
	ticket, err := store.GetTicket(context.Background(), "APP-1")
	if err != nil {
		t.Fatalf("load ticket: %v", err)
	}
	now := time.Now().UTC()
	ticket.Status = contracts.StatusReady
	ticket.UpdatedAt = now
	if err := store.UpdateTicket(context.Background(), ticket); err != nil {
		t.Fatalf("update ticket: %v", err)
	}
	journal := service.MutationJournal{Root: root, Clock: func() time.Time { return now }}
	entry := service.MutationJournalEntry{
		Purpose:       "repair replay",
		CanonicalKind: "ticket_snapshot",
		Event: contracts.Event{
			EventID:       3,
			Timestamp:     now,
			Actor:         contracts.Actor("human:owner"),
			Type:          contracts.EventTicketUpdated,
			Project:       "APP",
			TicketID:      "APP-1",
			Payload:       ticket,
			SchemaVersion: contracts.CurrentSchemaVersion,
		},
		Stage: service.MutationStageCanonicalWritten,
	}
	if err := testutil.SeedPendingMutationJournal(root, journal, entry); err != nil {
		t.Fatalf("seed pending journal: %v", err)
	}
	if err := testutil.CorruptProjection(root); err != nil {
		t.Fatalf("corrupt projection: %v", err)
	}

	locks := service.FileLockManager{Root: root, Wait: time.Second, RetryEvery: 10 * time.Millisecond}
	unlock, err := locks.Acquire(context.Background(), "hold doctor repair lock")
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	done := make(chan struct{})
	go func() {
		time.Sleep(150 * time.Millisecond)
		_ = unlock()
		close(done)
	}()
	start := time.Now()
	must("doctor", "--repair", "--json")
	<-done
	if time.Since(start) < 100*time.Millisecond {
		t.Fatalf("expected doctor --repair to wait on the workspace lock")
	}
	must("doctor", "--repair", "--json")

	after := must("notify", "log", "--json")
	if before != after {
		t.Fatalf("expected repair to avoid duplicate notifications\nbefore=%s\nafter=%s", before, after)
	}
}

func TestReindexUsesWorkspaceLock(t *testing.T) {
	withTempWorkspace(t)

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("command failed %v: %v\n%s", args, err, out)
		}
		return out
	}

	must("init")
	must("project", "create", "APP", "App Project")

	root := mustGetwd(t)
	locks := service.FileLockManager{Root: root, Wait: time.Second, RetryEvery: 10 * time.Millisecond}
	unlock, err := locks.Acquire(context.Background(), "hold reindex lock")
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	done := make(chan struct{})
	go func() {
		time.Sleep(150 * time.Millisecond)
		_ = unlock()
		close(done)
	}()
	start := time.Now()
	must("reindex")
	<-done
	if time.Since(start) < 100*time.Millisecond {
		t.Fatalf("expected reindex to wait on the workspace lock")
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return cwd
}

func packagedArchiveName(version string) string {
	return "tracker_" + strings.TrimPrefix(version, "v") + "_" + runtime.GOOS + "_" + runtimeArch(runtime.GOARCH) + ".tar.gz"
}

func runtimeArch(arch string) string {
	switch arch {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	default:
		return arch
	}
}

func gitRunInDir(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}
