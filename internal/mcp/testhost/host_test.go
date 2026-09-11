package testhost

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	"github.com/myrrazor/atlas-tasker/internal/testutil"
)

func TestLoadRejectsDuplicateKeysAndEmptyServers(t *testing.T) {
	dir := t.TempDir()
	dup := filepath.Join(dir, "dup.json")
	if err := os.WriteFile(dup, []byte(`{"mcpServers":{"a":{},"a":{}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dup); err == nil {
		t.Fatal("expected duplicate key error")
	}
	empty := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(empty, []byte(`{"mcpServers":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(empty); err == nil {
		t.Fatal("expected empty descriptor error")
	}
}

func TestConformanceHostDiscoversAndInvokesAtlas(t *testing.T) {
	tracker := os.Getenv("TRACKER_BIN")
	if tracker == "" {
		bin := filepath.Join(t.TempDir(), "tracker")
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/tracker")
		cmd.Dir = testutil.RepoRoot()
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build tracker: %v\n%s", err, out)
		}
		if resolved, err := filepath.EvalSymlinks(bin); err == nil {
			bin = resolved
		}
		tracker = filepath.Clean(bin)
	}
	ws := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(ws); err == nil {
		ws = resolved
	}
	ws = filepath.Clean(ws)
	init := exec.Command(tracker, "init", "--skip-integrations")
	init.Dir = ws
	if out, err := init.CombinedOutput(); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(storage.WorkspaceMetadataFile(ws))
	if err != nil {
		t.Fatal(err)
	}
	var meta struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil || meta.WorkspaceID == "" {
		t.Fatalf("workspace id: %v %s", err, raw)
	}
	reg, err := adapter.NewRegistration(tracker, meta.WorkspaceID, adapter.WorkspaceBinding{
		Kind:          adapter.WorkspaceBindingAbsolutePath,
		WorkspaceRoot: ws,
	}, "agent:builder-1", false)
	if err != nil {
		t.Fatal(err)
	}
	body, err := reg.StandardConfigJSON()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ws, ".tracker", "integrations", "atlas-mcp.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	desc, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(desc.DiscoverNames()) != 1 || desc.DiscoverNames()[0] != reg.ServerName {
		t.Fatalf("names %v want %s", desc.DiscoverNames(), reg.ServerName)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	report, err := Probe(ctx, ProbeOptions{DescriptorPath: path, ServerName: reg.ServerName, Executable: tracker, WorkspaceRoot: ws})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed() {
		t.Fatalf("conformance probe failed: %+v", report)
	}
	have := map[string]bool{}
	for _, name := range report.Tools {
		have[name] = true
	}
	for _, name := range []string{"atlas.context", "atlas.status", "atlas.board"} {
		if !have[name] {
			t.Fatalf("conformance host missing %s in %v", name, report.Tools)
		}
	}
}
