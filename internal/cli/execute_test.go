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
		Migration     struct {
			State string `json:"state"`
		} `json:"migration"`
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
	if payload.Migration.State == "" {
		t.Fatal("expected migration field to be present")
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

func TestExecuteUsesStructuredJSONErrorsForV16CommandFamilies(t *testing.T) {
	withTempWorkspace(t)

	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if _, err := runCLI(t, "project", "create", "APP", "App Project"); err != nil {
		t.Fatalf("project create failed: %v", err)
	}

	type errorCase struct {
		name string
		args []string
		code string
		exit int
	}
	cases := []errorCase{
		{name: "collaborator", args: []string{"collaborator", "view", "ghost", "--json"}, code: "not_found", exit: 3},
		{name: "membership", args: []string{"membership", "unbind", "membership_missing", "--json"}, code: "not_found", exit: 3},
		{name: "remote", args: []string{"remote", "view", "origin", "--json"}, code: "not_found", exit: 3},
		{name: "sync", args: []string{"sync", "view", "sync_missing", "--json"}, code: "not_found", exit: 3},
		{name: "bundle", args: []string{"bundle", "view", "bundle_missing", "--json"}, code: "not_found", exit: 3},
		{name: "conflict", args: []string{"conflict", "view", "conflict_missing", "--json"}, code: "not_found", exit: 3},
		{name: "mentions", args: []string{"mentions", "view", "mention_missing", "--json"}, code: "not_found", exit: 3},
		{name: "timeline", args: []string{"timeline", "APP-99", "--collaborator", "ghost", "--json"}, code: "not_found", exit: 3},
		{name: "codeowners", args: []string{"project", "codeowners", "render", "NOPE", "--json"}, code: "not_found", exit: 3},
		{name: "rules", args: []string{"project", "rules", "render", "NOPE", "--json"}, code: "not_found", exit: 3},
		{name: "remote_add_invalid", args: []string{"remote", "add", "origin", "--kind", "git", "--location", "https://user:secret@example.com/acme/repo.git", "--json"}, code: "invalid_input", exit: 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			exit := Execute(tc.args, &stdout, &stderr)
			if exit != tc.exit {
				t.Fatalf("expected exit %d, got %d\nstdout=%s\nstderr=%s", tc.exit, exit, stdout.String(), stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("expected no stdout on error, got %s", stdout.String())
			}
			var envelope struct {
				FormatVersion string `json:"format_version"`
				OK            bool   `json:"ok"`
				Error         struct {
					Code string `json:"code"`
					Exit int    `json:"exit"`
				} `json:"error"`
			}
			if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
				t.Fatalf("parse json error envelope: %v\nraw=%s", err, stderr.String())
			}
			if envelope.FormatVersion != jsonFormatVersion || envelope.OK {
				t.Fatalf("unexpected envelope header: %#v", envelope)
			}
			if envelope.Error.Code != tc.code || envelope.Error.Exit != tc.exit {
				t.Fatalf("unexpected envelope for %s: %#v", tc.name, envelope)
			}
		})
	}
}

func TestDoctorRepairReportsCorruptOrchestrationDocsWithoutFailing(t *testing.T) {
	withTempWorkspace(t)

	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if _, err := runCLI(t, "project", "create", "APP", "App Project"); err != nil {
		t.Fatalf("project create failed: %v", err)
	}
	if _, err := runCLI(t, "ticket", "create", "--project", "APP", "--title", "Doctor corruption", "--type", "task", "--actor", "human:owner"); err != nil {
		t.Fatalf("ticket create failed: %v", err)
	}

	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for _, item := range []struct {
		path string
		body string
	}{
		{storage.RunFile(root, "run_broken"), "---\nrun_id: run_broken\nstatus: [\n"},
		{storage.GateFile(root, "gate_broken"), "---\ngate_id: gate_broken\nkind: review\nstate: [\n"},
	} {
		if err := os.MkdirAll(filepath.Dir(item.path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", item.path, err)
		}
		if err := os.WriteFile(item.path, []byte(item.body), 0o644); err != nil {
			t.Fatalf("write %s: %v", item.path, err)
		}
	}

	out, err := runCLI(t, "doctor", "--repair", "--json")
	if err != nil {
		t.Fatalf("doctor --repair failed: %v\n%s", err, out)
	}
	var payload struct {
		OK         bool     `json:"ok"`
		IssueCodes []string `json:"issue_codes"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("parse doctor payload: %v\nraw=%s", err, out)
	}
	if !payload.OK {
		t.Fatalf("expected doctor ok=true, got %#v", payload)
	}
	for _, code := range []string{"run_doc_corrupt", "gate_doc_corrupt"} {
		found := false
		for _, got := range payload.IssueCodes {
			if got == code {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected doctor issue code %s in %v", code, payload.IssueCodes)
		}
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
