package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	"github.com/myrrazor/atlas-tasker/internal/testutil"
)

func indexPath(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Join(storage.TrackerDir(root), "index.sqlite")
}

func dropIndex(t *testing.T) {
	t.Helper()
	path := indexPath(t)
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Remove(candidate); err != nil && !os.IsNotExist(err) {
			t.Fatalf("remove %s: %v", candidate, err)
		}
	}
}

func seedTwoTickets(t *testing.T) {
	t.Helper()
	for _, args := range [][]string{
		{"init"},
		{"project", "create", "APP", "App Project"},
		{"ticket", "create", "--project", "APP", "--title", "First", "--type", "task", "--actor", "human:owner"},
		{"ticket", "create", "--project", "APP", "--title", "Second", "--type", "task", "--actor", "human:owner"},
	} {
		if out, err := runCLI(t, args...); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}
}

func boardTicketCount(t *testing.T, raw string) int {
	t.Helper()
	var payload struct {
		Columns map[string][]struct {
			ID string `json:"id"`
		} `json:"columns"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("parse board json: %v\nraw=%s", err, raw)
	}
	total := 0
	for _, column := range payload.Columns {
		total += len(column)
	}
	return total
}

func TestBoardRebuildsDeletedIndexInsteadOfShowingEmptyBoard(t *testing.T) {
	withTempWorkspace(t)
	seedTwoTickets(t)
	dropIndex(t)

	var stdout, stderr bytes.Buffer
	if exit := Execute([]string{"board", "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("board after index delete exited %d: %s", exit, stderr.String())
	}
	if got := boardTicketCount(t, stdout.String()); got != 2 {
		t.Fatalf("expected both tickets back on the board, got %d\n%s", got, stdout.String())
	}
	if !strings.Contains(stderr.String(), "rebuilt it from markdown and events") {
		t.Fatalf("expected a rebuild notice on stderr, got %q", stderr.String())
	}
	if strings.Contains(stdout.String(), "[tracker]") {
		t.Fatalf("notice leaked into --json stdout: %s", stdout.String())
	}
}

func TestBoardRebuildsStaleIndex(t *testing.T) {
	withTempWorkspace(t)
	seedTwoTickets(t)
	path := indexPath(t)
	snapshot, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if out, err := runCLI(t, "ticket", "create", "--project", "APP", "--title", "Third", "--type", "task", "--actor", "human:owner"); err != nil {
		t.Fatalf("third ticket: %v\n%s", err, out)
	}
	// put the two-ticket index back over the three-ticket sources, the way a
	// restored backup or a checked-out .tracker would
	if err := os.WriteFile(path, snapshot, 0o644); err != nil {
		t.Fatalf("restore stale index: %v", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(path + suffix); err != nil && !os.IsNotExist(err) {
			t.Fatalf("remove %s: %v", suffix, err)
		}
	}

	var stdout, stderr bytes.Buffer
	if exit := Execute([]string{"board", "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("board on stale index exited %d: %s", exit, stderr.String())
	}
	if got := boardTicketCount(t, stdout.String()); got != 3 {
		t.Fatalf("expected the stale index to be rebuilt to 3 tickets, got %d\n%s", got, stdout.String())
	}
	if !strings.Contains(stderr.String(), "rebuilt it from markdown and events") {
		t.Fatalf("expected a rebuild notice on stderr, got %q", stderr.String())
	}
}

func TestFirstBoardAfterInitPrintsNoNotice(t *testing.T) {
	withTempWorkspace(t)
	var stdout, stderr bytes.Buffer
	if exit := Execute([]string{"init"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("init exited %d: %s", exit, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exit := Execute([]string{"board"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("board exited %d: %s", exit, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("README happy path must stay quiet on stderr, got %q", stderr.String())
	}
}

func TestSecondCommandAfterRebuildIsQuiet(t *testing.T) {
	withTempWorkspace(t)
	seedTwoTickets(t)
	dropIndex(t)

	var stdout, stderr bytes.Buffer
	if exit := Execute([]string{"board", "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("first board exited %d: %s", exit, stderr.String())
	}
	if !strings.Contains(stderr.String(), "rebuilt it from markdown and events") {
		t.Fatalf("expected the first command to rebuild, got %q", stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exit := Execute([]string{"board", "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("second board exited %d: %s", exit, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("rebuild must stamp the index so the next command is quiet, got %q", stderr.String())
	}
	if got := boardTicketCount(t, stdout.String()); got != 2 {
		t.Fatalf("expected 2 tickets on the second read, got %d", got)
	}
}

func TestArchivingATicketDoesNotTriggerARebuild(t *testing.T) {
	withTempWorkspace(t)
	seedTwoTickets(t)
	if out, err := runCLI(t, "ticket", "archive", "APP-2", "--actor", "human:owner", "--reason", "done with it"); err != nil {
		t.Fatalf("ticket archive: %v\n%s", err, out)
	}
	var stdout, stderr bytes.Buffer
	if exit := Execute([]string{"board", "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("board after archive exited %d: %s", exit, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("archiving a ticket must not leave the index looking stale, got %q", stderr.String())
	}
	if got := boardTicketCount(t, stdout.String()); got != 1 {
		t.Fatalf("expected the archived ticket off the board, got %d", got)
	}
}

func TestArchiveApplyMovingRuntimeFilesDoesNotTriggerARebuild(t *testing.T) {
	withTempWorkspace(t)
	seedTwoTickets(t)
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// a completed run old enough for the default runtime policy to sweep it,
	// with a real file under .tracker/runtime so ApplyArchive has a path to
	// move before it commits the archive_applied event
	old := time.Now().UTC().AddDate(0, 0, -10)
	run := contracts.RunSnapshot{RunID: "run_stale_index", TicketID: "APP-1", Project: "APP", Status: contracts.RunStatusCompleted, Kind: contracts.RunKindWork, CreatedAt: old, CompletedAt: old, SchemaVersion: contracts.CurrentSchemaVersion}
	if err := (service.RunStore{Root: root}).SaveRun(context.Background(), run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	brief := storage.RuntimeBriefFile(root, run.RunID)
	if err := os.MkdirAll(filepath.Dir(brief), 0o755); err != nil {
		t.Fatalf("mkdir runtime dir: %v", err)
	}
	if err := os.WriteFile(brief, []byte("runtime"), 0o644); err != nil {
		t.Fatalf("write runtime brief: %v", err)
	}
	for _, path := range []string{brief, storage.RuntimeDir(root, run.RunID)} {
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatalf("chtimes %s: %v", path, err)
		}
	}

	out, err := runCLI(t, "archive", "apply", "--target", "runtime", "--project", "APP", "--yes", "--actor", "human:owner", "--json")
	if err != nil {
		t.Fatalf("archive apply: %v\n%s", err, out)
	}
	var applied struct {
		Payload struct {
			Record struct {
				ItemCount int `json:"item_count"`
			} `json:"record"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(out), &applied); err != nil {
		t.Fatalf("parse archive apply: %v\nraw=%s", err, out)
	}
	if applied.Payload.Record.ItemCount != 1 {
		t.Fatalf("archive apply moved nothing, so this test proves nothing: %s", out)
	}
	if _, err := os.Stat(storage.RuntimeDir(root, run.RunID)); !os.IsNotExist(err) {
		t.Fatalf("expected the runtime dir to be moved into the archive payload, stat err=%v", err)
	}

	var stdout, stderr bytes.Buffer
	if exit := Execute([]string{"board", "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("board after archive apply exited %d: %s", exit, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("archive apply must not leave the index looking stale, got %q", stderr.String())
	}
	if got := boardTicketCount(t, stdout.String()); got != 2 {
		t.Fatalf("expected both tickets still on the board, got %d", got)
	}
}

func TestDoctorFailsLoudOnStaleIndexAndRepairFixesIt(t *testing.T) {
	withTempWorkspace(t)
	seedTwoTickets(t)
	dropIndex(t)

	var stdout, stderr bytes.Buffer
	if exit := Execute([]string{"doctor"}, &stdout, &stderr); exit != 7 {
		t.Fatalf("read-only doctor on a stale index should exit 7, got %d (stdout=%s stderr=%s)", exit, stdout.String(), stderr.String())
	}
	want := "projection index is stale (no recorded fingerprint; sources now events=2 tickets=2); run 'tracker doctor --repair' or 'tracker reindex'"
	if !strings.Contains(stderr.String(), want) {
		t.Fatalf("doctor should name the drift and the fix, want %q in %q", want, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if exit := Execute([]string{"doctor", "--repair", "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("doctor --repair exited %d: %s", exit, stderr.String())
	}
	var payload struct {
		OK            bool     `json:"ok"`
		RepairActions []string `json:"repair_actions"`
		Index         struct {
			StaleBeforeRepair  bool   `json:"stale_before_repair"`
			Rebuilt            bool   `json:"rebuilt"`
			StoredFingerprint  string `json:"stored_fingerprint"`
			CurrentFingerprint string `json:"current_fingerprint"`
		} `json:"index"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("parse doctor payload: %v\nraw=%s", err, stdout.String())
	}
	if !payload.OK {
		t.Fatalf("expected ok payload: %s", stdout.String())
	}
	found := false
	for _, action := range payload.RepairActions {
		if action == "rebuilt projection" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 'rebuilt projection' in repair_actions, got %v", payload.RepairActions)
	}
	if !payload.Index.StaleBeforeRepair || !payload.Index.Rebuilt {
		t.Fatalf("expected index.stale_before_repair and index.rebuilt, got %+v", payload.Index)
	}
	if payload.Index.CurrentFingerprint != "events=2 tickets=2" {
		t.Fatalf("unexpected current fingerprint %q", payload.Index.CurrentFingerprint)
	}

	stdout.Reset()
	stderr.Reset()
	if exit := Execute([]string{"doctor"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("doctor after repair should be clean, exit %d: %s", exit, stderr.String())
	}
	if !strings.Contains(stdout.String(), "doctor ok:") {
		t.Fatalf("expected the usual doctor ok line, got %s", stdout.String())
	}
}

func TestReindexRecoversACorruptIndex(t *testing.T) {
	withTempWorkspace(t)
	seedTwoTickets(t)
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := testutil.CorruptProjection(root); err != nil {
		t.Fatalf("corrupt projection: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if exit := Execute([]string{"reindex"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("reindex should recover a corrupt index, exit %d: %s", exit, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exit := Execute([]string{"board", "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("board after reindex exited %d: %s", exit, stderr.String())
	}
	if got := boardTicketCount(t, stdout.String()); got != 2 {
		t.Fatalf("expected both tickets after reindex, got %d", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("reindex should have stamped the index, got %q", stderr.String())
	}
}

func TestDoctorNamesBothFingerprintsOnAStampedStaleIndex(t *testing.T) {
	withTempWorkspace(t)
	seedTwoTickets(t)
	path := indexPath(t)
	snapshot, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if out, err := runCLI(t, "ticket", "create", "--project", "APP", "--title", "Third", "--type", "task", "--actor", "human:owner"); err != nil {
		t.Fatalf("third ticket: %v\n%s", err, out)
	}
	if err := os.WriteFile(path, snapshot, 0o644); err != nil {
		t.Fatalf("restore stale index: %v", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(path + suffix); err != nil && !os.IsNotExist(err) {
			t.Fatalf("remove %s: %v", suffix, err)
		}
	}

	var stdout, stderr bytes.Buffer
	if exit := Execute([]string{"doctor"}, &stdout, &stderr); exit != 7 {
		t.Fatalf("doctor on a stamped-but-stale index should exit 7, got %d (stdout=%s stderr=%s)", exit, stdout.String(), stderr.String())
	}
	want := "projection index is stale (index built from events=2 tickets=2, sources now events=3 tickets=3)"
	if !strings.Contains(stderr.String(), want) {
		t.Fatalf("doctor should show both fingerprints, want %q in %q", want, stderr.String())
	}
}
