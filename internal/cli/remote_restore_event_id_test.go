package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
)

func mustJSON(t *testing.T, args ...string) map[string]any {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if exit := Execute(append(args, "--json"), &stdout, &stderr); exit != 0 {
		t.Fatalf("%v exited %d\n%s%s", args, exit, stdout.String(), stderr.String())
	}
	out := map[string]any{}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("%v: decode json: %v\n%s", args, err, stdout.String())
	}
	return out
}

func dig(m map[string]any, keys ...string) any {
	var cur any = m
	for _, k := range keys {
		next, _ := cur.(map[string]any)
		cur = next[k]
	}
	return cur
}

// sourceWithPriorLocalRestore builds Asta's edge-19 source workspace: a local
// restore before the checkpoint leaves a _workspace backup.restored event in
// the log that the checkpoint carries to the destination.
func sourceWithPriorLocalRestore(t *testing.T) (target []string) {
	t.Helper()
	bare := filepath.Join(t.TempDir(), "backup.git")
	if out, err := exec.Command("git", "init", "-q", "--bare", bare).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v %s", err, out)
	}
	target = []string{"--url", "file://" + bare, "--scope", "workspace", "--allow-local-file", "--acknowledge-data-boundary", "--attest-private"}
	mustJSON(t, "init", "--no-agents", "--no-open")
	mustJSON(t, "project", "create", "APP", "App")
	mustJSON(t, "ticket", "create", "--project", "APP", "--title", "Original title", "--type", "task", "--actor", "human:owner")
	backup := mustJSON(t, "backup", "create", "--actor", "human:owner", "--reason", "qa")
	backupID, _ := dig(backup, "backup", "backup_id").(string)
	if backupID == "" {
		backupID, _ = dig(backup, "snapshot", "backup_id").(string)
	}
	if backupID == "" {
		t.Fatalf("no backup id in %v", backup)
	}
	mustJSON(t, "ticket", "edit", "APP-1", "--title", "Tampered title", "--actor", "human:owner")
	plan := mustJSON(t, "backup", "restore-plan", backupID)
	mustJSON(t, "backup", "restore-apply", dig(plan, "plan", "restore_plan_id").(string), "--yes", "--actor", "human:owner", "--reason", "qa")
	mustJSON(t, append([]string{"backup", "target", "add", "--id", "local-drill"}, target...)...)
	mustJSON(t, "backup", "auto", "enable", "--target", "local-drill")
	mustJSON(t, "backup", "run", "--now")
	return target
}

func freshDestination(t *testing.T, target []string) string {
	t.Helper()
	dest := t.TempDir()
	if err := os.Chdir(dest); err != nil {
		t.Fatal(err)
	}
	mustJSON(t, "init", "--no-agents", "--no-open")
	mustJSON(t, append([]string{"backup", "target", "add", "--id", "local-drill"}, target...)...)
	listed := mustJSON(t, "backup", "remote", "list", "--target", "local-drill")
	items, _ := listed["items"].([]any)
	if len(items) == 0 {
		t.Fatalf("no remote checkpoints: %v", listed)
	}
	return items[0].(map[string]any)["checkpoint_id"].(string)
}

func assertRestoredCleanly(t *testing.T) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if exit := Execute([]string{"doctor", "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("doctor after remote restore exited %d\n%s%s", exit, stdout.String(), stderr.String())
	}
	view := mustJSON(t, "ticket", "view", "APP-1")
	if title, _ := dig(view, "ticket", "title").(string); title != "Original title" {
		if !strings.Contains(stdout.String()+toJSON(view), "Original title") {
			t.Fatalf("APP-1 should be back to Original title: %v", view)
		}
	}
	root, _ := os.Getwd()
	if entries, _ := os.ReadDir(storage.MutationsDir(root)); len(entries) != 0 {
		t.Fatalf("no mutation journal should be left pending, found %d", len(entries))
	}
	assertIncreasingEventIDs(t, root)
}

func toJSON(v any) string { raw, _ := json.Marshal(v); return string(raw) }

func assertIncreasingEventIDs(t *testing.T, root string) {
	t.Helper()
	events, err := (&eventstore.Log{RootDir: root}).StreamEvents(t.Context(), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	last := map[string]int64{}
	restored := 0
	for _, event := range events {
		if event.EventID <= last[event.Project] {
			t.Fatalf("event ids must increase per project: %s #%d after #%d", event.Project, event.EventID, last[event.Project])
		}
		last[event.Project] = event.EventID
		if event.Type == contracts.EventBackupRestored {
			restored++
		}
	}
	if restored < 2 {
		t.Fatalf("expected the source's restore event plus the destination's, got %d", restored)
	}
}

func TestRemoteRestoreApplyByPlanIDAfterSourceLocalRestore(t *testing.T) {
	withTempWorkspace(t)
	target := sourceWithPriorLocalRestore(t)
	checkpoint := freshDestination(t, target)
	plan := mustJSON(t, "backup", "remote", "restore-plan", checkpoint, "--target", "local-drill", "--allow-workspace-mismatch")
	planID := dig(plan, "plan", "restore_plan_id").(string)
	before := maxEventIDs(t)
	mustJSON(t, "backup", "remote", "restore-apply", planID, "--target", "local-drill", "--allow-workspace-mismatch", "--yes", "--actor", "human:owner", "--reason", "qa")
	assertRestoredCleanly(t)
	assertNewEventsAbove(t, before)
}

func TestRemoteRestoreApplyByCheckpointAfterSourceLocalRestore(t *testing.T) {
	withTempWorkspace(t)
	target := sourceWithPriorLocalRestore(t)
	checkpoint := freshDestination(t, target)
	mustJSON(t, "backup", "remote", "restore-plan", checkpoint, "--target", "local-drill", "--allow-workspace-mismatch")
	before := maxEventIDs(t)
	mustJSON(t, "backup", "remote", "restore-apply", checkpoint, "--target", "local-drill", "--allow-workspace-mismatch", "--yes", "--actor", "human:owner", "--reason", "qa")
	assertRestoredCleanly(t)
	assertNewEventsAbove(t, before)
}

// A workspace already stuck by the old ordering: the restore's canonical files
// landed, but its journal holds event _workspace #1 while the swapped-in log
// already reaches #2 with different events.
func TestDoctorRepairRenumbersStuckRestoreJournal(t *testing.T) {
	withTempWorkspace(t)
	sourceWithPriorLocalRestore(t)
	root, _ := os.Getwd()
	log := &eventstore.Log{RootDir: root}
	all, err := log.StreamEvents(t.Context(), "_workspace", 0)
	if err != nil {
		t.Fatal(err)
	}
	var maxID int64
	for _, event := range all {
		if event.EventID > maxID {
			maxID = event.EventID
		}
	}
	if maxID < 1 {
		t.Fatalf("setup needs a _workspace event, max is %d", maxID)
	}
	stuck := contracts.Event{
		EventID: 1, Timestamp: all[0].Timestamp.Add(1), OriginWorkspaceID: all[0].OriginWorkspaceID,
		LogicalClock: 1, Actor: contracts.Actor("human:owner"), Reason: "qa",
		Type: contracts.EventBackupRestored, Project: "_workspace",
		Payload: map[string]any{"restore_plan_id": "restore-stuck"}, SchemaVersion: contracts.CurrentSchemaVersion,
	}
	stuck.EventUID = contracts.CanonicalEventUID(stuck)
	journalName := "_workspace-000000000001.json"
	entry := map[string]any{
		"id": journalName, "purpose": "apply backup restore", "canonical_kind": "backup_restore",
		"event": stuck, "stage": "canonical_written", "started_at": stuck.Timestamp, "updated_at": stuck.Timestamp,
		"last_error": "event_id 1 must be greater than 2",
	}
	raw, _ := json.MarshalIndent(entry, "", "  ")
	if err := os.MkdirAll(storage.MutationsDir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storage.MutationsDir(root), journalName), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	if exit, _, out := execJSON(t, "doctor", "--json"); exit != 7 {
		t.Fatalf("doctor on a stuck journal exited %d, want 7\n%s", exit, out)
	}
	exit, payload, out := execJSON(t, "doctor", "--repair", "--json")
	if exit != 0 {
		t.Fatalf("doctor --repair on a stuck restore journal exited %d\n%s", exit, out)
	}
	if !strings.Contains(toJSON(payload["repair_actions"]), "renumbered") {
		t.Fatalf("repair should log the renumbering: %v", payload["repair_actions"])
	}
	if exit, _, out := execJSON(t, "doctor", "--json"); exit != 0 {
		t.Fatalf("doctor after repair exited %d\n%s", exit, out)
	}
	after, _ := log.StreamEvents(t.Context(), "_workspace", 0)
	found := false
	for _, event := range after {
		if event.Type == contracts.EventBackupRestored && event.EventID > maxID && strings.Contains(toJSON(event.Payload), "restore-stuck") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the stuck restore event must be appended above #%d, not dropped", maxID)
	}
	backups, _ := filepath.Glob(filepath.Join(storage.TrackerDir(root), "quarantine", "*", "mutations", journalName))
	if len(backups) != 1 {
		t.Fatalf("the original journal entry should be backed up once, found %v", backups)
	}
}

func maxEventIDs(t *testing.T) map[string]int64 {
	t.Helper()
	root, _ := os.Getwd()
	events, err := (&eventstore.Log{RootDir: root}).StreamEvents(t.Context(), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]int64{}
	for _, event := range events {
		out[event.Project] = max(out[event.Project], event.EventID)
	}
	return out
}

// every event appended by or after the restore lands above the ids the
// workspace had issued before it, even when the restored log is shorter
func assertNewEventsAbove(t *testing.T, before map[string]int64) {
	t.Helper()
	mustJSON(t, "ticket", "edit", "APP-1", "--title", "After remote restore", "--actor", "human:owner")
	root, _ := os.Getwd()
	events, err := (&eventstore.Log{RootDir: root}).StreamEvents(t.Context(), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	var restoreSeen, editSeen bool
	for _, event := range events {
		switch {
		case event.Type == contracts.EventBackupRestored && event.EventID > before[event.Project]:
			restoreSeen = true
		case event.Project == "APP" && strings.Contains(toJSON(event.Payload), "After remote restore"):
			if event.EventID <= before["APP"] {
				t.Fatalf("post-restore edit reused APP #%d (pre-restore max %d)", event.EventID, before["APP"])
			}
			editSeen = true
		}
	}
	if !restoreSeen || !editSeen {
		t.Fatalf("restore event above %v: %t, edit seen: %t", before, restoreSeen, editSeen)
	}
}
