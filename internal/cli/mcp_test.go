package cli

import (
	"encoding/json"
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
