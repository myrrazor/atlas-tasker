package cli

import (
	"strings"
	"testing"
)

func TestMCPSchemaCommand(t *testing.T) {
	withTempWorkspace(t)
	out, err := runCLI(t, "mcp", "schema", "--json")
	if err != nil {
		t.Fatalf("mcp schema failed: %v\n%s", err, out)
	}
	for _, tool := range []string{"atlas_queue", "atlas_ticket_view", "atlas_ticket_comment"} {
		if !strings.Contains(out, tool) {
			t.Fatalf("expected MCP schema to include %s, got %s", tool, out)
		}
	}
}
