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

func TestArchiveCommandsPlanApplyListAndRestore(t *testing.T) {
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
	must("ticket", "create", "--project", "APP", "--title", "Archive runtime", "--type", "task", "--actor", "human:owner")

	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	old := time.Now().UTC().AddDate(0, 0, -10)
	run := contracts.RunSnapshot{RunID: "run_cli_archive", TicketID: "APP-1", Project: "APP", Status: contracts.RunStatusCompleted, Kind: contracts.RunKindWork, CreatedAt: old, CompletedAt: old, SchemaVersion: contracts.CurrentSchemaVersion}
	if err := (service.RunStore{Root: root}).SaveRun(context.Background(), run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	for _, path := range []string{storage.RuntimeBriefFile(root, run.RunID), storage.RuntimeContextFile(root, run.RunID), storage.RuntimeLaunchFile(root, run.RunID, "codex"), storage.RuntimeLaunchFile(root, run.RunID, "claude")} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir runtime dir: %v", err)
		}
		if err := os.WriteFile(path, []byte("runtime"), 0o644); err != nil {
			t.Fatalf("write runtime artifact: %v", err)
		}
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatalf("chtimes %s: %v", path, err)
		}
	}
	if err := os.Chtimes(storage.RuntimeDir(root, run.RunID), old, old); err != nil {
		t.Fatalf("chtimes runtime dir: %v", err)
	}

	planOut := must("archive", "plan", "--target", "runtime", "--project", "APP", "--json")
	var plan struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Payload       struct {
			Policy struct {
				PolicyID string `json:"policy_id"`
			} `json:"policy"`
			Items []struct {
				Path string `json:"path"`
			} `json:"items"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(planOut), &plan); err != nil {
		t.Fatalf("parse archive plan: %v\nraw=%s", err, planOut)
	}
	if plan.FormatVersion != jsonFormatVersion || plan.Kind != "archive_plan" || plan.Payload.Policy.PolicyID != "runtime-default" || len(plan.Payload.Items) != 1 {
		t.Fatalf("unexpected archive plan payload: %#v", plan)
	}

	applyOut := must("archive", "apply", "--target", "runtime", "--project", "APP", "--yes", "--actor", "human:owner", "--json")
	var applied struct {
		Kind    string `json:"kind"`
		Payload struct {
			Record struct {
				ArchiveID string `json:"archive_id"`
				State     string `json:"state"`
				ItemCount int    `json:"item_count"`
			} `json:"record"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(applyOut), &applied); err != nil {
		t.Fatalf("parse archive apply: %v\nraw=%s", err, applyOut)
	}
	if applied.Kind != "archive_apply_result" || applied.Payload.Record.ArchiveID == "" || applied.Payload.Record.State != "archived" || applied.Payload.Record.ItemCount != 1 {
		t.Fatalf("unexpected archive apply payload: %#v", applied)
	}
	if _, err := os.Stat(storage.RuntimeDir(root, run.RunID)); !os.IsNotExist(err) {
		t.Fatalf("expected runtime dir to be archived, got err=%v", err)
	}

	listOut := must("archive", "list", "--target", "runtime", "--json")
	var listed struct {
		Kind  string `json:"kind"`
		Items []struct {
			ArchiveID string `json:"archive_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(listOut), &listed); err != nil {
		t.Fatalf("parse archive list: %v\nraw=%s", err, listOut)
	}
	if listed.Kind != "archive_list" || len(listed.Items) != 1 || listed.Items[0].ArchiveID != applied.Payload.Record.ArchiveID {
		t.Fatalf("unexpected archive list payload: %#v", listed)
	}

	restoreOut := must("archive", "restore", applied.Payload.Record.ArchiveID, "--actor", "human:owner", "--json")
	var restored struct {
		Kind    string `json:"kind"`
		Payload struct {
			Record struct {
				State string `json:"state"`
			} `json:"record"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(restoreOut), &restored); err != nil {
		t.Fatalf("parse archive restore: %v\nraw=%s", err, restoreOut)
	}
	if restored.Kind != "archive_restore_result" || restored.Payload.Record.State != "restored" {
		t.Fatalf("unexpected archive restore payload: %#v", restored)
	}
	if _, err := os.Stat(storage.RuntimeBriefFile(root, run.RunID)); err != nil {
		t.Fatalf("expected restored runtime artifact: %v", err)
	}
}

func TestArchiveListFiltersByProject(t *testing.T) {
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
	must("project", "create", "OPS", "Ops Project")
	must("ticket", "create", "--project", "APP", "--title", "Archive app", "--type", "task", "--actor", "human:owner")
	must("ticket", "create", "--project", "OPS", "--title", "Archive ops", "--type", "task", "--actor", "human:owner")

	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	old := time.Now().UTC().AddDate(0, 0, -10)
	for _, run := range []contracts.RunSnapshot{
		{RunID: "run_cli_archive_app", TicketID: "APP-1", Project: "APP", Status: contracts.RunStatusCompleted, Kind: contracts.RunKindWork, CreatedAt: old, CompletedAt: old, SchemaVersion: contracts.CurrentSchemaVersion},
		{RunID: "run_cli_archive_ops", TicketID: "OPS-2", Project: "OPS", Status: contracts.RunStatusCompleted, Kind: contracts.RunKindWork, CreatedAt: old, CompletedAt: old, SchemaVersion: contracts.CurrentSchemaVersion},
	} {
		if err := (service.RunStore{Root: root}).SaveRun(context.Background(), run); err != nil {
			t.Fatalf("save run %s: %v", run.RunID, err)
		}
		if err := os.MkdirAll(storage.RuntimeDir(root, run.RunID), 0o755); err != nil {
			t.Fatalf("mkdir runtime dir %s: %v", run.RunID, err)
		}
		if err := os.WriteFile(storage.RuntimeBriefFile(root, run.RunID), []byte(run.Project), 0o644); err != nil {
			t.Fatalf("write runtime brief %s: %v", run.RunID, err)
		}
		if err := os.Chtimes(storage.RuntimeDir(root, run.RunID), old, old); err != nil {
			t.Fatalf("chtimes runtime dir %s: %v", run.RunID, err)
		}
		if err := os.Chtimes(storage.RuntimeBriefFile(root, run.RunID), old, old); err != nil {
			t.Fatalf("chtimes runtime brief %s: %v", run.RunID, err)
		}
	}

	must("archive", "apply", "--target", "runtime", "--project", "APP", "--yes", "--actor", "human:owner")
	must("archive", "apply", "--target", "runtime", "--project", "OPS", "--yes", "--actor", "human:owner")

	listOut := must("archive", "list", "--target", "runtime", "--project", "APP", "--json")
	var listed struct {
		Items []struct {
			ProjectKey string `json:"project_key"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(listOut), &listed); err != nil {
		t.Fatalf("parse filtered archive list: %v\nraw=%s", err, listOut)
	}
	if len(listed.Items) != 1 || listed.Items[0].ProjectKey != "APP" {
		t.Fatalf("expected one APP archive, got %#v", listed.Items)
	}
}

func TestArchiveRestoreAfterCompactSurvivesReindexAndDoctor(t *testing.T) {
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
	must("ticket", "create", "--project", "APP", "--title", "Archive compact restore", "--type", "task", "--actor", "human:owner")

	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	old := time.Now().UTC().AddDate(0, 0, -10)
	run := contracts.RunSnapshot{RunID: "run_cli_archive_compact", TicketID: "APP-1", Project: "APP", Status: contracts.RunStatusCompleted, Kind: contracts.RunKindWork, CreatedAt: old, CompletedAt: old, SchemaVersion: contracts.CurrentSchemaVersion}
	if err := (service.RunStore{Root: root}).SaveRun(context.Background(), run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	for _, item := range []struct {
		path string
		body string
	}{
		{storage.RuntimeBriefFile(root, run.RunID), "brief"},
		{storage.RuntimeContextFile(root, run.RunID), "{}"},
		{storage.RuntimeLaunchFile(root, run.RunID, "codex"), "codex"},
		{storage.RuntimeLaunchFile(root, run.RunID, "claude"), "claude"},
	} {
		if err := os.MkdirAll(filepath.Dir(item.path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", item.path, err)
		}
		if err := os.WriteFile(item.path, []byte(item.body), 0o644); err != nil {
			t.Fatalf("write %s: %v", item.path, err)
		}
		if err := os.Chtimes(item.path, old, old); err != nil {
			t.Fatalf("chtimes %s: %v", item.path, err)
		}
	}
	if err := os.Chtimes(storage.RuntimeDir(root, run.RunID), old, old); err != nil {
		t.Fatalf("chtimes runtime dir: %v", err)
	}

	must("archive", "apply", "--target", "runtime", "--project", "APP", "--yes", "--actor", "human:owner")
	listOut := must("archive", "list", "--target", "runtime", "--project", "APP", "--json")
	var listed struct {
		Items []struct {
			ArchiveID string `json:"archive_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(listOut), &listed); err != nil {
		t.Fatalf("parse archive list: %v\nraw=%s", err, listOut)
	}
	if len(listed.Items) != 1 || listed.Items[0].ArchiveID == "" {
		t.Fatalf("expected archive id, got %#v", listed.Items)
	}

	must("compact", "--yes", "--actor", "human:owner")
	must("archive", "restore", listed.Items[0].ArchiveID, "--actor", "human:owner")
	must("reindex")

	doctorOut := must("doctor", "--json")
	var doctor struct {
		IssueCodes []string `json:"issue_codes"`
	}
	if err := json.Unmarshal([]byte(doctorOut), &doctor); err != nil {
		t.Fatalf("parse doctor output: %v\nraw=%s", err, doctorOut)
	}
	for _, blocked := range []string{"runtime_artifacts_partial", "archive_restore_incomplete"} {
		for _, code := range doctor.IssueCodes {
			if code == blocked {
				t.Fatalf("expected %s to stay clear after compact+restore, got %#v", blocked, doctor.IssueCodes)
			}
		}
	}
}
