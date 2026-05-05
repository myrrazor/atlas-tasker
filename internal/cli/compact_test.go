package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestCompactCommandRemovesGeneratedArtifacts(t *testing.T) {
	withTempWorkspace(t)

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
		return out
	}

	must("init")
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Compact runtime", "--type", "task", "--actor", "human:owner")

	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	old := time.Now().UTC().AddDate(0, 0, -10)
	run := contracts.RunSnapshot{RunID: "run_cli_compact", TicketID: "APP-1", Project: "APP", Status: contracts.RunStatusCompleted, Kind: contracts.RunKindWork, CreatedAt: old, CompletedAt: old, SchemaVersion: contracts.CurrentSchemaVersion}
	if err := (service.RunStore{Root: root}).SaveRun(context.Background(), run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	for path, body := range map[string]string{
		storage.RuntimeBriefFile(root, run.RunID):                   "brief",
		storage.RuntimeContextFile(root, run.RunID):                 "context",
		storage.RuntimeLaunchFile(root, run.RunID, "codex"):         "codex launch",
		storage.RuntimeLaunchFile(root, run.RunID, "claude"):        "claude launch",
		filepath.Join(storage.TrackerDir(root), "index.sqlite-wal"): "wal",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir path: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}

	out := must("compact", "--yes", "--actor", "human:owner", "--json")
	var payload struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Payload       struct {
			RemovedPaths []string `json:"removed_paths"`
			BytesFreed   int64    `json:"bytes_freed"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("parse compact output: %v\nraw=%s", err, out)
	}
	if payload.FormatVersion != jsonFormatVersion || payload.Kind != "compact_result" || payload.Payload.BytesFreed == 0 || len(payload.Payload.RemovedPaths) < 2 {
		t.Fatalf("unexpected compact payload: %#v", payload)
	}
	if _, err := os.Stat(storage.RuntimeLaunchFile(root, run.RunID, "codex")); !os.IsNotExist(err) {
		t.Fatalf("expected codex launch file removed, got err=%v", err)
	}
	if _, err := os.Stat(storage.RuntimeBriefFile(root, run.RunID)); err != nil {
		t.Fatalf("expected brief to remain: %v", err)
	}
}
