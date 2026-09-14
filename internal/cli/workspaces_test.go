package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspacesGrantJSON(t *testing.T) {
	withTempWorkspace(t)
	dir := filepath.Join(t.TempDir(), "board")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := runCLI(t, "workspaces", "grant", dir, "--purpose", "init", "--json")
	if err != nil {
		t.Fatalf("grant: %v\n%s", err, out)
	}
	var payload struct {
		ID      string `json:"id"`
		Path    string `json:"path"`
		Purpose string `json:"purpose"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("parse %v\n%s", err, out)
	}
	if payload.ID == "" || payload.Purpose != "init" || payload.Path != dir {
		t.Fatalf("grant payload: %+v", payload)
	}
	if strings.Contains(out, "connected") {
		t.Fatal("grant output invented connection status")
	}
}
