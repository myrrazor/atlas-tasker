package cli

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestBulkCommandsSupportDryRunViewTargetsAndBatchMetadata(t *testing.T) {
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
	must("config", "set", "actor.default", "human:owner")
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "One", "--type", "task", "--actor", "human:owner")
	must("ticket", "create", "--project", "APP", "--title", "Two", "--type", "task", "--actor", "human:owner")
	must("ticket", "move", "APP-1", "ready", "--actor", "human:owner")
	must("views", "save", "ready-search", "--kind", "search", "--query", "status=ready")

	dryRun := must("bulk", "move", "in_progress", "--view", "ready-search", "--ticket", "APP-2", "--dry-run", "--json")
	var preview struct {
		FormatVersion string `json:"format_version"`
		BatchID       string `json:"batch_id"`
		Preview       struct {
			TicketCount int      `json:"ticket_count"`
			TicketIDs   []string `json:"ticket_ids"`
			DryRun      bool     `json:"dry_run"`
		} `json:"preview"`
		Summary struct {
			Succeeded int `json:"succeeded"`
			Failed    int `json:"failed"`
			Skipped   int `json:"skipped"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(dryRun), &preview); err != nil {
		t.Fatalf("parse dry-run json: %v\nraw=%s", err, dryRun)
	}
	if preview.FormatVersion != jsonFormatVersion {
		t.Fatalf("unexpected format version: %s", preview.FormatVersion)
	}
	if preview.Preview.TicketCount != 2 || !preview.Preview.DryRun {
		t.Fatalf("unexpected dry-run preview: %#v", preview)
	}
	if preview.Summary.Skipped != 1 || preview.Summary.Failed != 1 {
		t.Fatalf("unexpected dry-run summary: %#v", preview.Summary)
	}

	if out, err := runCLI(t, "bulk", "move", "in_progress", "--ticket", "APP-1"); err == nil || !strings.Contains(err.Error(), "--yes or --dry-run") {
		t.Fatalf("expected confirmation error, got err=%v out=%s", err, out)
	}

	must("bulk", "move", "in_progress", "--view", "ready-search", "--yes", "--json")
	history := must("ticket", "history", "APP-1", "--json")
	var payload struct {
		FormatVersion string `json:"format_version"`
		Events        []struct {
			Metadata struct {
				BatchID string `json:"batch_id"`
			} `json:"metadata"`
		} `json:"events"`
	}
	if err := json.Unmarshal([]byte(history), &payload); err != nil {
		t.Fatalf("parse history json: %v\nraw=%s", err, history)
	}
	if payload.FormatVersion != jsonFormatVersion {
		t.Fatalf("unexpected format version: %s", payload.FormatVersion)
	}
	if payload.Events[len(payload.Events)-1].Metadata.BatchID == "" {
		t.Fatalf("expected bulk history event to carry batch id: %#v", payload.Events[len(payload.Events)-1])
	}
}

func TestBulkViewTargetsMatchSavedViewRunExactly(t *testing.T) {
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
	must("config", "set", "actor.default", "human:owner")
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "One", "--type", "task", "--actor", "human:owner")
	must("ticket", "create", "--project", "APP", "--title", "Two", "--type", "task", "--actor", "human:owner")
	must("ticket", "create", "--project", "APP", "--title", "Three", "--type", "task", "--actor", "human:owner")
	must("ticket", "move", "APP-1", "ready", "--actor", "human:owner")
	must("ticket", "move", "APP-3", "ready", "--actor", "human:owner")
	must("views", "save", "ready-search", "--kind", "search", "--query", "status=ready")

	runOut := must("views", "run", "ready-search", "--json")
	var runResult struct {
		FormatVersion string `json:"format_version"`
		Tickets       []struct {
			ID string `json:"id"`
		} `json:"tickets"`
	}
	if err := json.Unmarshal([]byte(runOut), &runResult); err != nil {
		t.Fatalf("parse views run json: %v\nraw=%s", err, runOut)
	}
	if runResult.FormatVersion != jsonFormatVersion {
		t.Fatalf("unexpected format version: %s", runResult.FormatVersion)
	}
	viewTicketIDs := make([]string, 0, len(runResult.Tickets))
	for _, ticket := range runResult.Tickets {
		viewTicketIDs = append(viewTicketIDs, ticket.ID)
	}

	dryRun := must("bulk", "claim", "--view", "ready-search", "--dry-run", "--json")
	var preview struct {
		FormatVersion string `json:"format_version"`
		Preview       struct {
			TicketIDs []string `json:"ticket_ids"`
		} `json:"preview"`
	}
	if err := json.Unmarshal([]byte(dryRun), &preview); err != nil {
		t.Fatalf("parse bulk dry-run json: %v\nraw=%s", err, dryRun)
	}
	if preview.FormatVersion != jsonFormatVersion {
		t.Fatalf("unexpected format version: %s", preview.FormatVersion)
	}
	if !reflect.DeepEqual(preview.Preview.TicketIDs, viewTicketIDs) {
		t.Fatalf("expected bulk view expansion to match views run exactly, got bulk=%v view=%v", preview.Preview.TicketIDs, viewTicketIDs)
	}
}
