package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestMCPSchemaAndToolsReflectProfiles(t *testing.T) {
	out, err := runCLI(t, "mcp", "schema", "--json", "--tool-profile", "read")
	if err != nil {
		t.Fatalf("mcp schema failed: %v\n%s", err, out)
	}
	var schemaPayload struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal([]byte(out), &schemaPayload); err != nil {
		t.Fatalf("parse schema: %v\n%s", err, out)
	}
	schemaNames := map[string]bool{}
	for _, tool := range schemaPayload.Tools {
		schemaNames[tool.Name] = true
	}
	if !schemaNames["atlas.ticket.view"] {
		t.Fatalf("expected read schema to include ticket view")
	}
	if schemaNames["atlas.ticket.comment"] || schemaNames["atlas.change.merge"] {
		t.Fatalf("read schema leaked write/high-impact tools: %#v", schemaNames)
	}

	toolsOut, err := runCLI(t, "mcp", "tools", "--json", "--tool-profile", "admin")
	if err != nil {
		t.Fatalf("mcp tools failed: %v\n%s", err, toolsOut)
	}
	var payload struct {
		Tools []struct {
			Name           string `json:"name"`
			Enabled        bool   `json:"enabled"`
			DisabledReason string `json:"disabled_reason"`
		} `json:"tools"`
	}
	if err := json.Unmarshal([]byte(toolsOut), &payload); err != nil {
		t.Fatalf("parse tools: %v\n%s", err, toolsOut)
	}
	for _, tool := range payload.Tools {
		if tool.Name == "atlas.change.merge" {
			if tool.Enabled || tool.DisabledReason != "high_impact_tools_disabled" {
				t.Fatalf("expected merge disabled by high impact gate, got %#v", tool)
			}
			return
		}
	}
	t.Fatalf("expected inventory to include atlas.change.merge")
}

func TestMCPApproveOperationCreatesBoundApproval(t *testing.T) {
	withTempWorkspace(t)
	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	out, err := runCLI(t, "mcp", "approve-operation", "--operation", "change.merge", "--target", "CHG-1", "--actor", "human:owner", "--reason", "release merge", "--ttl", "10m", "--json")
	if err != nil {
		t.Fatalf("approve operation failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "mcp_approval_") || !strings.Contains(out, "execute atlas.change.merge CHG-1") {
		t.Fatalf("approval output missing token or confirm text:\n%s", out)
	}
	listOut, err := runCLI(t, "mcp", "approvals", "list", "--json")
	if err != nil {
		t.Fatalf("approval list failed: %v\n%s", err, listOut)
	}
	if !strings.Contains(listOut, "atlas.change.merge") || !strings.Contains(listOut, "CHG-1") {
		t.Fatalf("approval list missing created approval:\n%s", listOut)
	}
}

func TestMCPToolsDocsMatchJSONInventory(t *testing.T) {
	out, err := runCLI(t, "mcp", "tools", "--json", "--tool-profile", "admin", "--dangerously-allow-high-impact-tools")
	if err != nil {
		t.Fatalf("mcp tools failed: %v\n%s", err, out)
	}
	var payload struct {
		Tools []struct {
			Name               string   `json:"name"`
			Class              string   `json:"class"`
			Profiles           []string `json:"profiles"`
			RequiresActor      bool     `json:"requires_actor"`
			RequiresReason     bool     `json:"requires_reason"`
			RequiresApproval   bool     `json:"requires_approval_token_or_gate"`
			ProviderSideEffect bool     `json:"provider_live_side_effect"`
		} `json:"tools"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("parse tools: %v\n%s", err, out)
	}
	raw, err := os.ReadFile("../../docs/mcp-tools.md")
	if err != nil {
		t.Fatalf("read MCP tools docs: %v", err)
	}
	rows := parseMCPToolsTable(string(raw))
	if len(rows) != len(payload.Tools) {
		t.Fatalf("docs table row count mismatch: got %d want %d", len(rows), len(payload.Tools))
	}
	for _, tool := range payload.Tools {
		row, ok := rows[tool.Name]
		if !ok {
			t.Fatalf("docs table missing %s", tool.Name)
		}
		want := mcpToolDocRow{
			class:    tool.Class,
			def:      yesNo(hasProfile(tool.Profiles, "read")),
			actor:    yesNo(tool.RequiresActor),
			reason:   yesNo(tool.RequiresReason),
			approval: yesNo(tool.RequiresApproval),
			live:     yesNo(tool.ProviderSideEffect),
		}
		if row != want {
			t.Fatalf("docs row mismatch for %s:\ngot  %#v\nwant %#v", tool.Name, row, want)
		}
	}
}

type mcpToolDocRow struct {
	class    string
	def      string
	actor    string
	reason   string
	approval string
	live     string
}

func parseMCPToolsTable(markdown string) map[string]mcpToolDocRow {
	rows := map[string]mcpToolDocRow{}
	for _, line := range strings.Split(markdown, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "| `atlas.") {
			continue
		}
		cols := strings.Split(line, "|")
		if len(cols) < 8 {
			continue
		}
		name := strings.Trim(strings.TrimSpace(cols[1]), "`")
		rows[name] = mcpToolDocRow{
			class:    strings.TrimSpace(cols[2]),
			def:      strings.TrimSpace(cols[3]),
			actor:    strings.TrimSpace(cols[4]),
			reason:   strings.TrimSpace(cols[5]),
			approval: strings.TrimSpace(cols[6]),
			live:     strings.TrimSpace(cols[7]),
		}
	}
	return rows
}

func hasProfile(profiles []string, profile string) bool {
	for _, item := range profiles {
		if item == profile {
			return true
		}
	}
	return false
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
