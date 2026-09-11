package selfprobe

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

func TestSelfProbeAgainstBuiltTracker(t *testing.T) {
	tracker := buildTracker(t)
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
	id := workspaceID(t, ws)
	reg, err := adapter.NewRegistration(tracker, id, adapter.WorkspaceBinding{
		Kind:          adapter.WorkspaceBindingAbsolutePath,
		WorkspaceRoot: ws,
	}, "agent:builder-1", false)
	if err != nil {
		t.Fatal(err)
	}
	cmd, err := reg.ServeCommand(tracker, ws, 25*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	report, err := Run(ctx, Options{
		Command: cmd,
		Mutate:  true,
		Actor:   "human:owner",
		Reason:  "AT114-201 self-probe mutation",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed() {
		t.Fatalf("probe failed: %+v", report)
	}
	if !report.Initialized || !report.DashboardOK || !report.BoardOK {
		t.Fatalf("incomplete probe: %+v", report)
	}
	if len(report.HighImpactFound) != 0 {
		t.Fatalf("high-impact tools present: %v", report.HighImpactFound)
	}
	if !report.MutationOK {
		t.Fatalf("workflow mutation failed: %+v", report)
	}
	if !report.ShutdownOK {
		t.Fatal("server did not shut down cleanly")
	}
}

func buildTracker(t *testing.T) string {
	t.Helper()
	if bin := os.Getenv("TRACKER_BIN"); bin != "" {
		return bin
	}
	bin := filepath.Join(t.TempDir(), "tracker")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/tracker")
	cmd.Dir = testutil.RepoRoot()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build tracker: %v\n%s", err, out)
	}
	if resolved, err := filepath.EvalSymlinks(bin); err == nil {
		bin = resolved
	}
	return filepath.Clean(bin)
}

func workspaceID(t *testing.T, root string) string {
	t.Helper()
	raw, err := os.ReadFile(storage.WorkspaceMetadataFile(root))
	if err != nil {
		t.Fatal(err)
	}
	var meta struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil || meta.WorkspaceID == "" {
		t.Fatalf("workspace id: %v %s", err, raw)
	}
	return meta.WorkspaceID
}
