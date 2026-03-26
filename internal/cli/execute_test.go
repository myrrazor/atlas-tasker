package cli

import (
	"bytes"
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

func TestExecuteUsesStructuredJSONErrorsAndExitCodes(t *testing.T) {
	withTempWorkspace(t)

	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if _, err := runCLI(t, "project", "create", "APP", "App Project"); err != nil {
		t.Fatalf("project create failed: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := Execute([]string{"inspect", "APP-99", "--json"}, &stdout, &stderr)
	if exit != 3 {
		t.Fatalf("expected exit code 3, got %d", exit)
	}
	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
			Exit int    `json:"exit"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatalf("parse json error envelope: %v\nraw=%s", err, stderr.String())
	}
	if envelope.OK {
		t.Fatal("expected failed envelope")
	}
	if envelope.Error.Code != "not_found" {
		t.Fatalf("unexpected error code: %s", envelope.Error.Code)
	}
	if envelope.Error.Exit != 3 {
		t.Fatalf("unexpected envelope exit code: %d", envelope.Error.Exit)
	}
}

func TestDoctorJSONReportsStructuredSuccess(t *testing.T) {
	withTempWorkspace(t)

	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if _, err := runCLI(t, "project", "create", "APP", "App Project"); err != nil {
		t.Fatalf("project create failed: %v", err)
	}

	out, err := runCLI(t, "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor failed: %v", err)
	}
	var payload struct {
		FormatVersion string   `json:"format_version"`
		OK            bool     `json:"ok"`
		RepairRan     bool     `json:"repair_ran"`
		RepairPending int      `json:"repair_pending"`
		RepairActions []string `json:"repair_actions"`
		IssueCodes    []string `json:"issue_codes"`
		EventsCount   int      `json:"events_scanned"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("parse doctor payload: %v\nraw=%s", err, out)
	}
	if payload.FormatVersion != jsonFormatVersion {
		t.Fatalf("unexpected format version: %s", payload.FormatVersion)
	}
	if !payload.OK {
		t.Fatal("expected doctor ok=true")
	}
	if payload.RepairRan {
		t.Fatal("expected repair_ran=false")
	}
	if payload.IssueCodes == nil {
		t.Fatal("expected issue_codes field to be present")
	}
	if payload.RepairActions == nil {
		t.Fatal("expected repair_actions field to be present")
	}
}

func TestDoctorJSONReportsOrchestrationIntegrityIssues(t *testing.T) {
	withTempWorkspace(t)

	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if _, err := runCLI(t, "project", "create", "APP", "App Project"); err != nil {
		t.Fatalf("project create failed: %v", err)
	}
	if _, err := runCLI(t, "ticket", "create", "--project", "APP", "--title", "Doctor drift", "--type", "task", "--actor", "human:owner"); err != nil {
		t.Fatalf("ticket create failed: %v", err)
	}

	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	now := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	run := contracts.RunSnapshot{
		RunID:         "run_1",
		TicketID:      "APP-1",
		Project:       "APP",
		Status:        contracts.RunStatusActive,
		Kind:          contracts.RunKindWork,
		CreatedAt:     now,
		WorktreePath:  filepath.Join(root, "missing-worktree"),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := (service.RunStore{Root: root}).SaveRun(context.Background(), run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	if err := os.MkdirAll(storage.RuntimeDir(root, run.RunID), 0o755); err != nil {
		t.Fatalf("mkdir runtime: %v", err)
	}
	if err := os.WriteFile(storage.RuntimeBriefFile(root, run.RunID), []byte("brief"), 0o644); err != nil {
		t.Fatalf("write runtime brief: %v", err)
	}
	evidence := contracts.EvidenceItem{
		EvidenceID:    "evidence_missing",
		RunID:         run.RunID,
		TicketID:      "APP-1",
		Type:          contracts.EvidenceTypeLogExcerpt,
		Title:         "missing artifact",
		Body:          "artifact drift",
		ArtifactPath:  filepath.Join(storage.EvidenceDir(root, run.RunID), "missing.log"),
		Actor:         contracts.Actor("human:owner"),
		CreatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := (service.EvidenceStore{Root: root}).SaveEvidence(context.Background(), evidence); err != nil {
		t.Fatalf("save evidence: %v", err)
	}

	out, err := runCLI(t, "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor failed: %v", err)
	}
	var payload struct {
		IssueCodes []string `json:"issue_codes"`
		Issues     struct {
			Orchestration struct {
				RuntimeIssues  int `json:"runtime_issues"`
				WorktreeIssues int `json:"worktree_issues"`
				EvidenceIssues int `json:"evidence_issues"`
			} `json:"orchestration"`
		} `json:"issues"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("parse doctor payload: %v\nraw=%s", err, out)
	}
	for _, code := range []string{"runtime_artifacts_partial", "worktree_missing", "evidence_artifact_missing"} {
		found := false
		for _, got := range payload.IssueCodes {
			if got == code {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected issue code %s in %v", code, payload.IssueCodes)
		}
	}
	if payload.Issues.Orchestration.RuntimeIssues == 0 || payload.Issues.Orchestration.WorktreeIssues == 0 || payload.Issues.Orchestration.EvidenceIssues == 0 {
		t.Fatalf("expected orchestration issue counts, got %#v", payload.Issues.Orchestration)
	}
}

func TestExecuteHonorsJSONFalseOnErrors(t *testing.T) {
	withTempWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := Execute([]string{"inspect", "APP-99", "--json=false"}, &stdout, &stderr)
	if exit != 3 {
		t.Fatalf("expected exit code 3, got %d", exit)
	}
	if json.Valid(stderr.Bytes()) {
		t.Fatalf("expected plain text error, got json: %s", stderr.String())
	}
	if got := stderr.String(); got == "" {
		t.Fatal("expected error output")
	}
}
