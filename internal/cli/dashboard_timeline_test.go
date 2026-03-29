package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDashboardCommandReturnsSummaryEnvelope(t *testing.T) {
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
	must("ticket", "create", "--project", "APP", "--title", "Dashboard seed", "--type", "task", "--actor", "human:owner")

	out := must("dashboard", "--json")
	var payload struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Payload       struct {
			ActiveRuns int `json:"active_runs"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("parse dashboard output: %v\nraw=%s", err, out)
	}
	if payload.FormatVersion != jsonFormatVersion || payload.Kind != "dashboard_summary" {
		t.Fatalf("unexpected dashboard payload: %#v", payload)
	}
}

func TestTimelineCommandReturnsTicketHistory(t *testing.T) {
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
	must("ticket", "create", "--project", "APP", "--title", "Timeline seed", "--type", "task", "--actor", "human:owner")
	must("ticket", "comment", "APP-1", "--body", "left a note", "--actor", "agent:builder-1")

	out := must("timeline", "APP-1", "--json")
	var payload struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Payload       struct {
			TicketID string `json:"ticket_id"`
			Entries  []struct {
				Type    string `json:"type"`
				Summary string `json:"summary"`
			} `json:"entries"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("parse timeline output: %v\nraw=%s", err, out)
	}
	if payload.FormatVersion != jsonFormatVersion || payload.Kind != "timeline_detail" || payload.Payload.TicketID != "APP-1" || len(payload.Payload.Entries) < 2 {
		t.Fatalf("unexpected timeline payload: %#v", payload)
	}
	last := payload.Payload.Entries[len(payload.Payload.Entries)-1]
	if last.Type == "" || !strings.Contains(strings.ToLower(last.Summary), "comment") {
		t.Fatalf("unexpected timeline entry detail: %#v", payload.Payload.Entries)
	}
}

func TestDashboardAndTimelineAcceptCollaboratorFilters(t *testing.T) {
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
	must("ticket", "create", "--project", "APP", "--title", "Collab seed", "--type", "task", "--actor", "human:owner")
	must("collaborator", "add", "rev-1", "--name", "Rev One", "--actor-map", "agent:reviewer-1", "--actor", "human:owner")
	must("collaborator", "trust", "rev-1", "--actor", "human:owner")
	must("membership", "bind", "rev-1", "--scope-kind", "project", "--scope-id", "APP", "--role", "reviewer", "--actor", "human:owner")
	must("ticket", "comment", "APP-1", "--body", "ping @rev-1", "--actor", "agent:builder-1")

	dashboardOut := must("dashboard", "--collaborator", "rev-1", "--json")
	var dashboard struct {
		Payload struct {
			CollaboratorFilter string `json:"collaborator_filter"`
			MentionQueue       []struct {
				CollaboratorID string `json:"collaborator_id"`
			} `json:"mention_queue"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(dashboardOut), &dashboard); err != nil {
		t.Fatalf("parse filtered dashboard output: %v\nraw=%s", err, dashboardOut)
	}
	if dashboard.Payload.CollaboratorFilter != "rev-1" || len(dashboard.Payload.MentionQueue) != 1 || dashboard.Payload.MentionQueue[0].CollaboratorID != "rev-1" {
		t.Fatalf("unexpected filtered dashboard payload: %#v", dashboard)
	}

	timelineOut := must("timeline", "APP-1", "--collaborator", "rev-1", "--json")
	var timeline struct {
		Payload struct {
			CollaboratorFilter string `json:"collaborator_filter"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(timelineOut), &timeline); err != nil {
		t.Fatalf("parse filtered timeline output: %v\nraw=%s", err, timelineOut)
	}
	if timeline.Payload.CollaboratorFilter != "rev-1" {
		t.Fatalf("unexpected filtered timeline payload: %#v", timeline)
	}
}
