package cli

import (
	"encoding/json"
	"testing"
)

func TestAgentCommandsAndEligibility(t *testing.T) {
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
	must("ticket", "create", "--project", "APP", "--title", "Need builder", "--type", "task", "--actor", "human:owner")
	must("ticket", "edit", "APP-1", "--labels", "ops")
	must("agent", "create", "builder-1", "--name", "Builder One", "--provider", "codex", "--capability", "go", "--capability", "tests", "--max-active-runs", "2", "--actor", "human:owner")
	must("agent", "create", "builder-2", "--name", "Builder Two", "--provider", "claude", "--capability", "docs", "--enabled=false", "--actor", "human:owner")

	listOut := must("agent", "list", "--json")
	var list struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Items         []any  `json:"items"`
	}
	if err := json.Unmarshal([]byte(listOut), &list); err != nil {
		t.Fatalf("parse agent list: %v\nraw=%s", err, listOut)
	}
	if list.FormatVersion != jsonFormatVersion || list.Kind != "agent_list" || len(list.Items) != 2 {
		t.Fatalf("unexpected list payload: %#v", list)
	}

	viewOut := must("agent", "view", "builder-1", "--json")
	var view struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
	}
	if err := json.Unmarshal([]byte(viewOut), &view); err != nil {
		t.Fatalf("parse agent view: %v\nraw=%s", err, viewOut)
	}
	if view.Kind != "agent_detail" {
		t.Fatalf("unexpected view payload: %#v", view)
	}

	eligibleOut := must("agent", "eligible", "APP-1", "--json")
	var report struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Payload       struct {
			Entries []struct {
				Eligible bool `json:"eligible"`
				Agent    struct {
					AgentID string `json:"agent_id"`
				} `json:"agent"`
			} `json:"entries"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(eligibleOut), &report); err != nil {
		t.Fatalf("parse agent eligible: %v\nraw=%s", err, eligibleOut)
	}
	if report.Kind != "agent_eligibility_report" || len(report.Payload.Entries) != 2 || !report.Payload.Entries[0].Eligible {
		t.Fatalf("unexpected eligibility payload: %#v", report)
	}

	must("agent", "disable", "builder-1", "--actor", "human:owner")
	disabled := must("agent", "view", "builder-1", "--pretty")
	if disabled == "" {
		t.Fatalf("expected agent pretty output")
	}
}
