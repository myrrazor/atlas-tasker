package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
)

func chdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
}

func assertNoDuplicateEventIDs(t *testing.T, root string) {
	t.Helper()
	events, err := (&eventstore.Log{RootDir: root}).StreamEvents(t.Context(), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, event := range events {
		key := event.Project + "#" + itoa64(event.EventID) + "@" + event.OriginWorkspaceID
		if uid, ok := seen[key]; ok && uid != event.EventUID {
			t.Fatalf("%s: event id %s reused by its origin (uids %s and %s)", root, key, uid, event.EventUID)
		}
		seen[key] = event.EventUID
	}
}

func itoa64(v int64) string { return toJSON(v) }

// Asta's 185b M1 repro: a local restore-apply must not reuse event ids that a
// sync peer already pulled, or the peer ends up with duplicate ids and a
// stuck title.
func TestLocalRestoreApplyDoesNotReuseEventIDsSeenBySyncPeer(t *testing.T) {
	withTempWorkspace(t)
	src, _ := os.Getwd()
	base := t.TempDir()
	remote := filepath.Join(base, "remote")
	dst := filepath.Join(base, "dst")
	for _, dir := range []string{remote, dst} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mustJSON(t, "init", "--no-agents", "--no-open")
	mustJSON(t, "project", "create", "APP", "App")
	mustJSON(t, "ticket", "create", "--project", "APP", "--title", "Sync title", "--type", "task", "--actor", "human:owner")
	mustJSON(t, "remote", "add", "local-sync", "--kind", "path", "--location", remote, "--default-action", "push", "--actor", "human:owner")
	backup := mustJSON(t, "backup", "create", "--scope", "workspace", "--actor", "human:owner", "--reason", "qa")
	backupID, _ := dig(backup, "backup", "backup_id").(string)
	if backupID == "" {
		backupID, _ = dig(backup, "snapshot", "backup_id").(string)
	}
	mustJSON(t, "ticket", "edit", "APP-1", "--title", "Sync tampered", "--actor", "human:owner")
	mustJSON(t, "sync", "push", "--remote", "local-sync", "--actor", "human:owner")

	chdir(t, dst)
	mustJSON(t, "init", "--no-agents", "--no-open")
	mustJSON(t, "remote", "add", "local-sync", "--kind", "path", "--location", remote, "--default-action", "pull", "--actor", "human:owner")
	if exit, _, out := execJSON(t, "sync", "pull", "--remote", "local-sync", "--actor", "human:owner", "--json"); exit != 0 {
		t.Fatalf("first pull exited %d\n%s", exit, out)
	}

	chdir(t, src)
	plan := mustJSON(t, "backup", "restore-plan", backupID)
	mustJSON(t, "backup", "restore-apply", dig(plan, "plan", "restore_plan_id").(string), "--yes", "--actor", "human:owner", "--reason", "qa")
	mustJSON(t, "ticket", "edit", "APP-1", "--title", "Sync after restore", "--actor", "human:owner")
	mustJSON(t, "sync", "push", "--remote", "local-sync", "--actor", "human:owner")
	assertNoDuplicateEventIDs(t, src)

	chdir(t, dst)
	exit, _, out := execJSON(t, "sync", "pull", "--remote", "local-sync", "--actor", "human:owner", "--json")
	// the event logs must merge without id collisions either way
	assertNoDuplicateEventIDs(t, dst)
	switch exit {
	case 0:
	case 4:
		// Sync compares ticket files two-way, so a ticket edited on both
		// sides of a pull opens a scalar_divergence on the ticket file; main
		// does the same for a plain edit with no restore. It must be the only
		// kind of conflict, and taking the remote side must converge.
		listed := mustJSON(t, "conflict", "list")
		items, _ := listed["items"].([]any)
		for _, raw := range items {
			item := raw.(map[string]any)
			if item["entity_kind"] != "ticket" || item["conflict_type"] != "scalar_divergence" {
				t.Fatalf("unexpected conflict after the source's local restore: %v", item)
			}
			mustJSON(t, "conflict", "resolve", item["conflict_id"].(string), "--resolution", "use_remote", "--actor", "human:owner", "--reason", "qa")
		}
	default:
		t.Fatalf("pull after the source's local restore exited %d\n%s", exit, out)
	}
	assertNoDuplicateEventIDs(t, dst)
	history := mustJSON(t, "ticket", "history", "APP-1")
	if !strings.Contains(toJSON(history), "Sync after restore") {
		t.Fatalf("destination history should include the post-restore edit: %s", toJSON(history))
	}
	view := mustJSON(t, "ticket", "view", "APP-1")
	if title, _ := dig(view, "ticket", "title").(string); title != "Sync after restore" {
		t.Fatalf("destination title should converge to 'Sync after restore', got %q\n%s", title, toJSON(view))
	}
	var stdout, stderr bytes.Buffer
	if exit := Execute([]string{"doctor", "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("doctor on destination exited %d\n%s%s", exit, stdout.String(), stderr.String())
	}
}

// doctor flags the same (project, id, origin) issued for two different
// events, the state an older restore could leave behind on a sync peer.
func TestDoctorReportsReusedEventIDs(t *testing.T) {
	withTempWorkspace(t)
	mustJSON(t, "init", "--no-agents", "--no-open")
	mustJSON(t, "project", "create", "APP", "App")
	mustJSON(t, "ticket", "create", "--project", "APP", "--title", "First", "--type", "task", "--actor", "human:owner")
	if exit, payload, out := execJSON(t, "doctor", "--json"); exit != 0 || strings.Contains(toJSON(payload["issue_codes"]), "event_id_reused") {
		t.Fatalf("clean workspace: doctor exit %d\n%s", exit, out)
	}
	root, _ := os.Getwd()
	events, err := (&eventstore.Log{RootDir: root}).StreamEvents(t.Context(), "APP", 0)
	if err != nil || len(events) == 0 {
		t.Fatalf("need an APP event: %v", err)
	}
	reused := events[0]
	reused.Timestamp = reused.Timestamp.Add(1)
	reused.Reason = "reused id"
	reused.EventUID = contracts.CanonicalEventUID(reused)
	raw, _ := json.Marshal(reused)
	files, _ := filepath.Glob(filepath.Join(storage.EventsDir(root), "*.jsonl"))
	if len(files) == 0 {
		t.Fatal("no event files")
	}
	f, err := os.OpenFile(files[len(files)-1], os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write(append(raw, '\n'))
	_ = f.Close()
	mustJSON(t, "reindex")

	for _, args := range [][]string{{"doctor", "--json"}, {"doctor", "--repair", "--json"}} {
		exit, payload, out := execJSON(t, args...)
		if exit != 0 {
			t.Fatalf("%v exited %d\n%s", args, exit, out)
		}
		if !strings.Contains(toJSON(payload["issue_codes"]), "event_id_reused") {
			t.Fatalf("%v should report event_id_reused: %s", args, out)
		}
		reuse, _ := dig(payload, "issues", "event_id_reuse").([]any)
		if len(reuse) != 1 || !strings.Contains(toJSON(reuse[0]), events[0].EventUID) || !strings.Contains(toJSON(reuse[0]), reused.EventUID) {
			t.Fatalf("%v should list APP #%d with both uids: %s", args, events[0].EventID, toJSON(reuse))
		}
	}
	var stdout, stderr bytes.Buffer
	Execute([]string{"doctor"}, &stdout, &stderr)
	if !strings.Contains(stdout.String(), "warning: event id APP #") {
		t.Fatalf("plain doctor output should warn: %s%s", stdout.String(), stderr.String())
	}
}
