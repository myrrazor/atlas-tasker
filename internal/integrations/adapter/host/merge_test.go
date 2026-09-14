package host

import (
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
	"github.com/myrrazor/atlas-tasker/internal/mcp/testhost"
)

func TestMergeJSONPreservesUnrelatedAndRejectsDuplicates(t *testing.T) {
	existing := []byte(`{
  "mcpServers": {
    "other": {"type": "stdio", "command": "/usr/bin/other", "args": []}
  },
  "note": "keep"
}
`)
	entry := adapter.StandardServerEntry{Type: "stdio", Command: "/usr/local/bin/tracker", Args: []string{"mcp", "serve"}}
	merged, err := MergeJSONServer(existing, "atlas-aaaaaaaaaaaa", entry)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(merged), `"other"`) || !strings.Contains(string(merged), `"atlas-aaaaaaaaaaaa"`) || !strings.Contains(string(merged), `"note"`) {
		t.Fatalf("lost unrelated keys:\n%s", merged)
	}
	dup := []byte(`{"mcpServers":{"a":{},"a":{}}}`)
	if err := testhost.RejectDuplicateJSONKeys(dup); err == nil {
		t.Fatal("expected duplicate key error")
	}
}

func TestMergeTOMLPreservesComments(t *testing.T) {
	existing := []byte("# keep me\n[other]\nvalue = 1\n")
	required := false
	merged, err := MergeTOMLServer(existing, "atlas-aaaaaaaaaaaa", []TOMLField{
		{Key: "command", Value: "/usr/local/bin/tracker"},
		{Key: "args", Array: []string{"mcp", "serve"}},
		{Key: "required", Bool: &required},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(merged), "# keep me") || !strings.Contains(string(merged), "[other]") {
		t.Fatalf("lost comments/tables:\n%s", merged)
	}
	if _, err := MergeTOMLServer([]byte("[[[not toml"), "atlas-aaaaaaaaaaaa", nil); err == nil {
		t.Fatal("expected malformed TOML error")
	}
	removed, err := RemoveTOMLServer(merged, "atlas-aaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(removed), "atlas-aaaaaaaaaaaa") {
		t.Fatalf("table not removed:\n%s", removed)
	}
	if !strings.Contains(string(removed), "# keep me") {
		t.Fatal("comment removed with table")
	}
}
