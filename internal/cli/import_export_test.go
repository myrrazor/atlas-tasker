package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestExportCommandsCreateListViewAndVerify(t *testing.T) {
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
	must("ticket", "create", "--project", "APP", "--title", "Ship bundle", "--type", "task", "--actor", "human:owner")

	createOut := must("export", "create", "--actor", "human:owner", "--json")
	var created struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Payload       struct {
			Bundle struct {
				BundleID     string `json:"bundle_id"`
				ArtifactPath string `json:"artifact_path"`
			} `json:"bundle"`
			FileCount int `json:"file_count"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(createOut), &created); err != nil {
		t.Fatalf("parse export create output: %v\nraw=%s", err, createOut)
	}
	if created.FormatVersion != jsonFormatVersion || created.Kind != "export_bundle_create_result" || created.Payload.Bundle.BundleID == "" || created.Payload.FileCount == 0 {
		t.Fatalf("unexpected export create payload: %#v", created)
	}

	listOut := must("export", "list", "--json")
	var listed struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Items         []struct {
			BundleID string `json:"bundle_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(listOut), &listed); err != nil {
		t.Fatalf("parse export list output: %v\nraw=%s", err, listOut)
	}
	if listed.FormatVersion != jsonFormatVersion || listed.Kind != "export_bundle_list" || len(listed.Items) != 1 || listed.Items[0].BundleID != created.Payload.Bundle.BundleID {
		t.Fatalf("unexpected export list payload: %#v", listed)
	}

	viewOut := must("export", "view", created.Payload.Bundle.BundleID, "--json")
	var viewed struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Payload       struct {
			Bundle struct {
				BundleID string `json:"bundle_id"`
			} `json:"bundle"`
			FileCount int `json:"file_count"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(viewOut), &viewed); err != nil {
		t.Fatalf("parse export view output: %v\nraw=%s", err, viewOut)
	}
	if viewed.FormatVersion != jsonFormatVersion || viewed.Kind != "export_bundle_detail" || viewed.Payload.Bundle.BundleID != created.Payload.Bundle.BundleID || viewed.Payload.FileCount != created.Payload.FileCount {
		t.Fatalf("unexpected export view payload: %#v", viewed)
	}

	verifyOut := must("export", "verify", created.Payload.Bundle.BundleID, "--actor", "human:owner", "--json")
	var verified struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Payload       struct {
			Verified bool     `json:"verified"`
			Errors   []string `json:"errors"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(verifyOut), &verified); err != nil {
		t.Fatalf("parse export verify output: %v\nraw=%s", err, verifyOut)
	}
	if verified.FormatVersion != jsonFormatVersion || verified.Kind != "export_verify_result" || !verified.Payload.Verified || len(verified.Payload.Errors) != 0 {
		t.Fatalf("unexpected export verify payload: %#v", verified)
	}

	verifyPathOut := must("export", "verify", created.Payload.Bundle.ArtifactPath, "--actor", "human:owner", "--json")
	if err := json.Unmarshal([]byte(verifyPathOut), &verified); err != nil {
		t.Fatalf("parse export verify by path output: %v\nraw=%s", err, verifyPathOut)
	}
	if !verified.Payload.Verified || len(verified.Payload.Errors) != 0 {
		t.Fatalf("expected verification by artifact path to pass, got %#v", verified.Payload)
	}
}

func TestImportCommandsPreviewApplyListAndView(t *testing.T) {
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
	csvPath := filepath.Join(".", "jira-import.csv")
	csv := "Project Key,Project Name,Issue Key,Summary,Issue Type,Status,Priority,Parent,Blocks\nIMP,Imported Project,IMP-1,Imported epic,epic,backlog,high,,\nIMP,Imported Project,IMP-2,Imported task,task,ready,medium,IMP-1,IMP-1\n"
	if err := os.WriteFile(csvPath, []byte(csv), 0o644); err != nil {
		t.Fatalf("write jira csv: %v", err)
	}

	previewOut := must("import", "preview", csvPath, "--actor", "human:owner", "--json")
	var preview struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Payload       struct {
			Job struct {
				JobID   string `json:"job_id"`
				Status  string `json:"status"`
				Summary string `json:"summary"`
			} `json:"job"`
			Plan struct {
				SourceType string `json:"source_type"`
				Items      []struct {
					TicketID  string   `json:"ticket_id"`
					ParentRef string   `json:"parent_ref"`
					BlockedBy []string `json:"blocked_by"`
				} `json:"items"`
			} `json:"plan"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(previewOut), &preview); err != nil {
		t.Fatalf("parse import preview output: %v\nraw=%s", err, previewOut)
	}
	if preview.FormatVersion != jsonFormatVersion || preview.Kind != "import_preview" || preview.Payload.Job.JobID == "" || preview.Payload.Job.Status != "previewed" || preview.Payload.Plan.SourceType != "jira_csv" || len(preview.Payload.Plan.Items) != 2 {
		t.Fatalf("unexpected import preview payload: %#v", preview)
	}

	listOut := must("import", "list", "--json")
	var listed struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Items         []struct {
			JobID string `json:"job_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(listOut), &listed); err != nil {
		t.Fatalf("parse import list output: %v\nraw=%s", err, listOut)
	}
	if listed.FormatVersion != jsonFormatVersion || listed.Kind != "import_job_list" || len(listed.Items) != 1 || listed.Items[0].JobID != preview.Payload.Job.JobID {
		t.Fatalf("unexpected import list payload: %#v", listed)
	}

	viewOut := must("import", "view", preview.Payload.Job.JobID, "--json")
	var viewed struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Payload       struct {
			Job struct {
				JobID string `json:"job_id"`
			} `json:"job"`
			Plan struct {
				Items []struct {
					TicketID string `json:"ticket_id"`
				} `json:"items"`
			} `json:"plan"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(viewOut), &viewed); err != nil {
		t.Fatalf("parse import view output: %v\nraw=%s", err, viewOut)
	}
	if viewed.FormatVersion != jsonFormatVersion || viewed.Kind != "import_job_detail" || viewed.Payload.Job.JobID != preview.Payload.Job.JobID || len(viewed.Payload.Plan.Items) != 2 {
		t.Fatalf("unexpected import view payload: %#v", viewed)
	}

	applyOut := must("import", "apply", preview.Payload.Job.JobID, "--actor", "human:owner", "--json")
	var applied struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Payload       struct {
			Job struct {
				Status         string `json:"status"`
				PartialApplied bool   `json:"partial_applied"`
			} `json:"job"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(applyOut), &applied); err != nil {
		t.Fatalf("parse import apply output: %v\nraw=%s", err, applyOut)
	}
	if applied.FormatVersion != jsonFormatVersion || applied.Kind != "import_apply_result" || applied.Payload.Job.Status != "applied" || applied.Payload.Job.PartialApplied {
		t.Fatalf("unexpected import apply payload: %#v", applied)
	}

	ticketViewOut := must("ticket", "view", "IMP-2", "--json")
	var ticketView struct {
		Ticket struct {
			ID        string   `json:"id"`
			Parent    string   `json:"parent"`
			BlockedBy []string `json:"blocked_by"`
		} `json:"ticket"`
	}
	if err := json.Unmarshal([]byte(ticketViewOut), &ticketView); err != nil {
		t.Fatalf("parse imported ticket view: %v\nraw=%s", err, ticketViewOut)
	}
	if ticketView.Ticket.ID != "IMP-2" || ticketView.Ticket.Parent != "IMP-1" || len(ticketView.Ticket.BlockedBy) != 1 || ticketView.Ticket.BlockedBy[0] != "IMP-1" {
		t.Fatalf("unexpected imported ticket payload: %#v", ticketView)
	}
}

func TestGitHubImportPreviewAndApplyCommands(t *testing.T) {
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
	jsonPath := filepath.Join(".", "github-export.json")
	raw := `[{"project_key":"GHI","project_name":"GitHub Import","title":"Issue imported from GitHub","body":"details","type":"issue","status":"in_progress","priority":"low","url":"https://github.com/myrrazor/atlas-tasker/issues/9","number":9,"kind":"task"}]`
	if err := os.WriteFile(jsonPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("write github export json: %v", err)
	}

	previewOut := must("import", "preview", jsonPath, "--actor", "human:owner", "--json")
	var preview struct {
		Payload struct {
			Job struct {
				JobID string `json:"job_id"`
			} `json:"job"`
			Plan struct {
				SourceType string `json:"source_type"`
				Items      []struct {
					TicketID  string `json:"ticket_id"`
					SourceRef string `json:"source_ref"`
				} `json:"items"`
			} `json:"plan"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(previewOut), &preview); err != nil {
		t.Fatalf("parse github preview output: %v\nraw=%s", err, previewOut)
	}
	if preview.Payload.Job.JobID == "" || preview.Payload.Plan.SourceType != "github_export" || len(preview.Payload.Plan.Items) != 1 || preview.Payload.Plan.Items[0].TicketID != "GHI-9" {
		t.Fatalf("unexpected github import preview payload: %#v", preview)
	}

	must("import", "apply", preview.Payload.Job.JobID, "--actor", "human:owner", "--json")
	ticketViewOut := must("ticket", "view", "GHI-9", "--json")
	if !json.Valid([]byte(ticketViewOut)) {
		t.Fatalf("expected imported github ticket json view, got %s", ticketViewOut)
	}
}
