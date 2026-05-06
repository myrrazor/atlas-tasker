package cli

import (
	"bytes"
	"encoding/json"
	"testing"
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
