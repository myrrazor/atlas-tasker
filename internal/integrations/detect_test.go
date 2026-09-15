package integrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeFileInfo struct {
	dir bool
}

func (f fakeFileInfo) Name() string       { return "x" }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return 0o755 }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.dir }
func (f fakeFileInfo) Sys() any           { return nil }

func TestDetectFindsClaudeCodexCursorOpenClaw(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "CLAUDE.md"), []byte("# claude"), 0o644); err != nil {
		t.Fatal(err)
	}

	bins := map[string]string{"openclaw": "/usr/bin/openclaw", "grok": "/usr/local/bin/grok"}
	got := Detect(DetectOptions{
		Workspace: workspace,
		Home:      home,
		LookPath: func(name string) (string, error) {
			if path, ok := bins[name]; ok {
				return path, nil
			}
			return "", os.ErrNotExist
		},
	})

	byTarget := map[Target]Detection{}
	for _, item := range got {
		byTarget[item.Target] = item
	}
	for _, target := range []Target{TargetClaude, TargetCodex, TargetCursor, TargetOpenClaw, TargetGrok} {
		item := byTarget[target]
		if !item.Found {
			t.Fatalf("expected %s to be detected: %+v", target, item)
		}
		if len(item.Reasons) == 0 {
			t.Fatalf("expected reasons for %s", target)
		}
	}
	if byTarget[TargetGeneric].Found {
		t.Fatal("generic must not auto-detect as found")
	}
}

func TestDetectCursorAgentCLIOnly(t *testing.T) {
	got := Detect(DetectOptions{
		Workspace: t.TempDir(),
		Home:      t.TempDir(),
		LookPath: func(name string) (string, error) {
			if name == "cursor-agent" {
				return "/usr/local/bin/cursor-agent", nil
			}
			return "", os.ErrNotExist
		},
		Stat: func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
	})
	byTarget := map[Target]Detection{}
	for _, item := range got {
		byTarget[item.Target] = item
	}
	if !byTarget[TargetCursor].Found {
		t.Fatalf("cursor-agent on PATH must detect Cursor: %+v", byTarget[TargetCursor])
	}
	foundAgent := false
	for _, reason := range byTarget[TargetCursor].Reasons {
		if strings.Contains(reason, "cursor-agent") {
			foundAgent = true
		}
	}
	if !foundAgent {
		t.Fatalf("expected cursor-agent reason, got %v", byTarget[TargetCursor].Reasons)
	}
	if byTarget[TargetOpenClaw].Found {
		t.Fatalf("OpenClaw must not be inferred from cursor-agent: %+v", byTarget[TargetOpenClaw])
	}
}

func TestDetectOpenClawIgnoresWorkspaceAgentsDir(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".agents", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := Detect(DetectOptions{
		Workspace: workspace,
		Home:      t.TempDir(),
		LookPath:  func(string) (string, error) { return "", os.ErrNotExist },
	})
	for _, item := range got {
		if item.Target == TargetOpenClaw && item.Found {
			t.Fatalf("workspace .agents must not detect OpenClaw: %+v", item)
		}
		if item.Target == TargetCodex && item.Found {
			t.Fatalf("workspace .agents must not detect Codex: %+v", item)
		}
	}
}

func TestDetectedTargetsOmitsGenericAndMissing(t *testing.T) {
	got := DetectedTargets(DetectOptions{
		Workspace: t.TempDir(),
		Home:      t.TempDir(),
		LookPath:  func(string) (string, error) { return "", os.ErrNotExist },
		Stat:      func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
	})
	if len(got) != 0 {
		t.Fatalf("expected no detections, got %+v", got)
	}
}

func TestParseTargetList(t *testing.T) {
	got, err := ParseTargetList("claude, cursor grok")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != TargetClaude || got[1] != TargetCursor || got[2] != TargetGrok {
		t.Fatalf("unexpected parse: %+v", got)
	}
	if _, err := ParseTargetList("nope"); err == nil {
		t.Fatal("expected unknown target error")
	}
}

func TestSelectTargetsInteractivelyDefaultsAndOverrides(t *testing.T) {
	detections := []Detection{
		{Target: TargetClaude, Found: true, Reasons: []string{"binary on PATH"}},
		{Target: TargetCodex, Found: true, Reasons: []string{"config dir ~/.codex"}},
		{Target: TargetCursor},
		{Target: TargetGeneric},
	}

	got, err := SelectTargetsInteractively(SelectOptions{
		Detections:        detections,
		Stdin:             strings.NewReader("\n"),
		Stdout:            &strings.Builder{},
		DefaultToDetected: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != TargetClaude || got[1] != TargetCodex {
		t.Fatalf("default selection = %+v", got)
	}

	got, err = SelectTargetsInteractively(SelectOptions{
		Detections:        detections,
		Stdin:             strings.NewReader("1,3\n"),
		Stdout:            &strings.Builder{},
		DefaultToDetected: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != TargetClaude || got[1] != TargetCursor {
		t.Fatalf("override selection = %+v", got)
	}

	got, err = SelectTargetsInteractively(SelectOptions{
		Detections: detections,
		Stdin:      strings.NewReader("none\n"),
		Stdout:     &strings.Builder{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty selection, got %+v", got)
	}
}

func TestSelectTargetsInteractivelyEOFDoesNotInstallDetectedAgents(t *testing.T) {
	got, err := SelectTargetsInteractively(SelectOptions{
		Detections:        []Detection{{Target: TargetClaude, Found: true}},
		Stdin:             strings.NewReader(""),
		DefaultToDetected: true,
	})
	if err != nil || len(got) != 0 {
		t.Fatalf("EOF selected integrations without consent: %v, %v", got, err)
	}
}

func TestSelectTargetsInteractivelyRejectsDelimiterOnlyInput(t *testing.T) {
	for _, input := range []string{",\n", " , , \t\n"} {
		_, err := SelectTargetsInteractively(SelectOptions{
			Detections: []Detection{{Target: TargetCodex, Found: true}},
			Stdin:      strings.NewReader(input),
		})
		if err == nil {
			t.Fatalf("malformed selection %q reported a successful skip", input)
		}
	}
}
