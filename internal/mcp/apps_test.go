package mcp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/render"
)

func TestBoardAppHTMLIsDiscoverableAndSafe(t *testing.T) {
	html := boardAppHTML()
	if !strings.Contains(html, BoardAppCSP) {
		t.Fatal("missing CSP")
	}
	if !strings.Contains(html, render.BoardAppCSS) {
		t.Fatal("must reuse BoardAppCSS")
	}
	if !strings.Contains(html, `width=device-width`) {
		t.Fatal("missing responsive viewport")
	}
	if !boardAppUsesSafeDOM(html) {
		t.Fatal("unsafe DOM or missing Apps handshake")
	}
	if strings.Contains(strings.ToLower(html), "<script src=") {
		t.Fatal("external script")
	}
	if strings.Contains(html, "Object.keys") {
		t.Fatal("legacy map-of-tickets renderer still present")
	}
}

func TestBoardAppViewLinesPreferMarkdownFallback(t *testing.T) {
	lines := boardAppViewLines(map[string]any{
		"payload": map[string]any{"markdown": "# Board APP\n- APP-1 Ready"},
	})
	if len(lines) == 0 || !strings.Contains(lines[0], "Board APP") {
		t.Fatalf("markdown fallback missing: %#v", lines)
	}
}

func TestBoardAppJavaScriptRendersCompactBoard(t *testing.T) {
	outside := t.TempDir()
	machine := testGlobalMachine(t, outside)
	root := filepath.Join(outside, "repo")
	id := initRegisteredWorkspace(t, machine, root)
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	server := NewGlobalServer(machine, Options{Profile: ProfileWorkflow, Now: func() time.Time { return now }, CWD: outside, Machine: machine})
	if _, err := server.CallTool(ctx, "atlas.project.create", map[string]any{"workspace_id": id, "key": "APP", "name": "App"}); err != nil {
		t.Fatalf("project: %v", err)
	}
	if _, err := server.CallTool(ctx, "atlas.ticket.create", map[string]any{
		"workspace_id": id,
		"project":      "APP",
		"title":        "Ship the board",
		"type":         "task",
		"priority":     "high",
		"actor":        "human:owner",
		"reason":       "seed board",
	}); err != nil {
		t.Fatalf("ticket: %v", err)
	}
	if _, err := server.CallTool(ctx, "atlas.ticket.move", map[string]any{
		"workspace_id": id,
		"ticket_id":    "APP-1",
		"status":       string(contracts.StatusReady),
		"actor":        "human:owner",
		"reason":       "ready for board",
	}); err != nil {
		t.Fatalf("move: %v", err)
	}
	result, err := server.CallTool(ctx, "atlas.board", map[string]any{"workspace_id": id, "project": "APP"})
	if err != nil {
		t.Fatalf("board: %v", err)
	}
	inner, _ := result["payload"].(map[string]any)
	if inner == nil {
		t.Fatalf("atlas.board payload missing: %#v", result)
	}
	switch board := inner["board"].(type) {
	case render.CompactBoard:
		if len(board.Columns) == 0 {
			t.Fatalf("compact columns missing: %#v", board)
		}
	case map[string]any:
		if _, ok := board["columns"]; !ok {
			t.Fatalf("compact columns missing: %#v", board)
		}
	default:
		t.Fatalf("atlas.board payload missing compact board: %#v", inner)
	}
	url, _ := inner["board_url"].(string)
	if !strings.Contains(url, "/w/"+id) {
		t.Fatalf("global board_url = %q", url)
	}
	if strings.Contains(strings.ToLower(url), "session") || strings.Contains(strings.ToLower(url), "claim") {
		t.Fatalf("board url leaked a secret: %q", url)
	}

	dir := t.TempDir()
	writeAppsHarness(t, dir, result)
	copyFile(t, filepath.Join("testdata", "apps-host", "run.mjs"), filepath.Join(dir, "run.mjs"))
	copyFile(t, filepath.Join("testdata", "apps-host", "crowded-board.json"), filepath.Join(dir, "crowded-board.json"))

	cmd := exec.Command("node", "run.mjs")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node host harness: %v\n%s", err, out)
	}
	got := string(out)
	if !strings.Contains(got, "PASS") {
		t.Fatalf("harness did not pass:\n%s", got)
	}
	if !strings.Contains(got, "APP-1") || !strings.Contains(got, "Ship the board") {
		t.Fatalf("JS DOM did not render compact cards:\n%s", got)
	}

	sdk := server.SDKServer()
	clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
	if _, err := sdk.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "atlas-test", Version: "v0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	res, err := session.ReadResource(ctx, &mcpsdk.ReadResourceParams{URI: BoardAppResourceURI})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Contents) == 0 || res.Contents[0].Text != boardAppHTML() {
		t.Fatal("resources/read did not return the live board app document")
	}
	if !resourceHasUICSP(res.Contents[0].Meta) {
		t.Fatalf("resources/read content missing _meta.ui.csp: %#v", res.Contents[0].Meta)
	}
}

func TestBoardAppSyntheticFixtureIsDeterministic(t *testing.T) {
	html, err := os.ReadFile(filepath.Join("testdata", "apps-host", "app.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(html) != boardAppHTML() {
		t.Fatal("testdata/apps-host/app.html drifted from boardAppHTML(); rerun with ATLAS_WRITE_APPS_FIXTURE=1")
	}
	raw, err := os.ReadFile(filepath.Join("testdata", "apps-host", "board-payload.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), os.TempDir()) || strings.Contains(string(raw), "/var/folders/") {
		t.Fatal("synthetic fixture contains runtime temp paths")
	}
	if !strings.Contains(string(raw), "ws-atlas-demo") {
		t.Fatal("synthetic fixture must use the fixed workspace id ws-atlas-demo")
	}
}

func TestBoardAppRegenerateSyntheticFixture(t *testing.T) {
	if os.Getenv("ATLAS_WRITE_APPS_FIXTURE") != "1" {
		t.Skip("set ATLAS_WRITE_APPS_FIXTURE=1 to rewrite testdata/apps-host/app.html")
	}
	dir := filepath.Join("testdata", "apps-host")
	if err := os.WriteFile(filepath.Join(dir, "app.html"), []byte(boardAppHTML()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeAppsHarness(t *testing.T, dir string, payload any) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "app.html"), []byte(boardAppHTML()), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "board-payload.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func resourceHasUICSP(meta mcpsdk.Meta) bool {
	if meta == nil {
		return false
	}
	ui, _ := meta["ui"].(map[string]any)
	csp, _ := ui["csp"].(map[string]any)
	if csp == nil {
		return false
	}
	for _, key := range []string{"connectDomains", "resourceDomains", "frameDomains"} {
		raw, ok := csp[key]
		if !ok {
			return false
		}
		switch typed := raw.(type) {
		case []any:
			if len(typed) != 0 {
				return false
			}
		case []string:
			if len(typed) != 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}
