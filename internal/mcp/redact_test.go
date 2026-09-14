package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRedactKeepsTicketProseAndScrubsSecretsAndPaths(t *testing.T) {
	prose := "Ship APP-1 after reading https://example.com/docs"
	if got := redactStringField("title", prose, false); got != prose {
		t.Fatalf("ticket prose changed: %q", got)
	}
	secret := "token ghp_aaaaaaaaaaaaaaaaaaaa leaked"
	if got := redactStringField("title", secret, false); strings.Contains(got, "ghp_") {
		t.Fatalf("secret left in title: %q", got)
	}
	detail := "moved from /Users/alice/secret/project/.tracker"
	got := redactMixedString(detail, false)
	if strings.Contains(got, "/Users/") || strings.Contains(got, "alice/secret") {
		t.Fatalf("path suffix left in health_detail: %q", got)
	}
	cred := "fetch https://user:supersecret@github.com/org/repo.git failed"
	got = redactMixedString(cred, false)
	if strings.Contains(got, "supersecret") || strings.Contains(got, "user:supersecret") {
		t.Fatalf("credential left: %q", got)
	}
	if !strings.Contains(got, "***") {
		t.Fatalf("credential URL missing redaction marker: %q", got)
	}
	if strings.Contains(redactMixedString("see https://example.com/docs", false), "example.com/docs") == false {
		t.Fatal("legitimate https prose was destroyed")
	}
}

func TestToolsAndResourcesRedactSentinelsIncludingUnavailable(t *testing.T) {
	outside := t.TempDir()
	machine := testGlobalMachine(t, outside)
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	root := filepath.Join(outside, "repo")
	id := initRegisteredWorkspace(t, machine, root)
	server := NewGlobalServer(machine, Options{Profile: ProfileWorkflow, Now: func() time.Time { return now }, CWD: outside, Machine: machine})
	if _, err := server.CallTool(ctx, "atlas.project.create", map[string]any{"workspace_id": id, "key": "APP", "name": "App"}); err != nil {
		t.Fatal(err)
	}
	created, err := server.CallTool(ctx, "atlas.ticket.create", map[string]any{
		"workspace_id": id,
		"project":      "APP",
		"title":        "Keep APP-1 prose",
		"description":  "docs at https://example.com/guide and token ghp_bbbbbbbbbbbbbbbbbbbb",
		"type":         "task",
		"actor":        "human:owner",
		"reason":       "seed redaction",
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(created)
	got := string(raw)
	if !strings.Contains(got, "Keep APP-1 prose") {
		t.Fatalf("ticket title missing: %s", got)
	}
	if !strings.Contains(got, "https://example.com/guide") {
		t.Fatalf("legitimate docs URL stripped: %s", got)
	}
	if strings.Contains(got, "ghp_bbbbbbbbbbbbbbbbbbbb") {
		t.Fatalf("sentinel credential leaked from tool: %s", got)
	}

	if err := os.Rename(root, root+"-gone"); err != nil {
		t.Fatal(err)
	}
	listed, err := server.CallTool(ctx, "atlas.workspace.list", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	listRaw, _ := json.Marshal(listed)
	if strings.Contains(string(listRaw), root+"-gone") && strings.Contains(string(listRaw), "/Users/") {
		t.Fatalf("unavailable workspace leaked private path: %s", listRaw)
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
	res, err := session.ReadResource(ctx, &mcpsdk.ReadResourceParams{URI: "atlas://workspaces"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Contents) == 0 {
		t.Fatal("empty workspaces resource")
	}
	body := res.Contents[0].Text
	if strings.Contains(body, "ghp_") {
		t.Fatalf("resource leaked credential: %s", body)
	}
	wsRes, err := session.ReadResource(ctx, &mcpsdk.ReadResourceParams{URI: "atlas://workspace/" + id})
	if err != nil {
		t.Fatal(err)
	}
	detail := wsRes.Contents[0].Text
	if strings.Contains(detail, root+"-gone") {
		t.Fatalf("unavailable workspace resource leaked full path: %s", detail)
	}
}

func TestRedactLivePointersNilAndCredentialWithMarker(t *testing.T) {
	type box struct {
		Path string `json:"path"`
	}
	p := &box{Path: "/tmp/secret/project"}
	got := walkRedactLive(p, "", false)
	out, ok := got.(*box)
	if !ok {
		t.Fatalf("pointer type lost: %#v", got)
	}
	if out.Path != "[redacted-path]" || p.Path != "[redacted-path]" {
		t.Fatalf("pointer path not redacted in place: %#v %#v", out, p)
	}

	items := []any{nil, "Keep APP-1 prose", (*string)(nil)}
	walkRedactLive(items, "title", false)

	mixed := "already *** plus https://user:pass@example.com/repo.git"
	scrubbed := redactSecretsAndCredentials(mixed)
	if strings.Contains(scrubbed, "user:pass") || strings.Contains(scrubbed, "pass@example") {
		t.Fatalf("credential URL survived *** short-circuit: %q", scrubbed)
	}

	pem := "-----BEGIN RSA PRIVATE KEY-----\nMIIEsecretbodyDATA\n-----END RSA PRIVATE KEY-----"
	pemOut := redactSecretsAndCredentials(pem)
	if strings.Contains(pemOut, "MIIEsecretbodyDATA") || strings.Contains(pemOut, "BEGIN RSA") {
		t.Fatalf("PEM block not fully removed: %q", pemOut)
	}

	if redactStringField("path", "/Volumes/Disk/secret", false) != "[redacted-path]" {
		t.Fatal("/Volumes path field leaked")
	}
	if redactStringField("path", "/tmp/secret", false) != "[redacted-path]" {
		t.Fatal("/tmp path field leaked")
	}
	detail := redactMixedString("unavailable at /Volumes/Disk/secret and /tmp/hidden/repo", false)
	if strings.Contains(detail, "Disk/secret") || strings.Contains(detail, "/tmp/hidden") {
		t.Fatalf("portable absolute paths left in mixed string: %q", detail)
	}

	boardURL := "http://127.0.0.1:7432/w/ws-atlas-demo/projects/APP"
	if redactStringField("board_url", boardURL, false) != boardURL {
		t.Fatalf("board_url was damaged: %q", redactStringField("board_url", boardURL, false))
	}
	digest := "cafebabedeadbeefcafebabedeadbeefcafebabe"
	if redactStringField("plan_digest", digest, false) != digest {
		t.Fatalf("plan digest was damaged: %q", redactStringField("plan_digest", digest, false))
	}
	if redactStringField("title", "Ship APP-1 after reading https://example.com/docs", false) != "Ship APP-1 after reading https://example.com/docs" {
		t.Fatal("ticket prose was damaged")
	}
}

func TestSDKCallToolErrorIsRedacted(t *testing.T) {
	err := os.ErrNotExist
	wrapped := fmt.Errorf("open /tmp/secret-dir/project: token ghp_%s: %w", strings.Repeat("d", 22), err)
	text, env := redactCallToolError(wrapped, false)
	if strings.Contains(text, "/tmp/secret-dir") || strings.Contains(text, "ghp_") {
		t.Fatalf("error text leaked: %q", text)
	}
	raw, _ := json.Marshal(env)
	if strings.Contains(string(raw), "/tmp/secret-dir") || strings.Contains(string(raw), "ghp_") {
		t.Fatalf("error envelope leaked: %s", raw)
	}

	outside := t.TempDir()
	machine := testGlobalMachine(t, outside)
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	server := NewGlobalServer(machine, Options{Profile: ProfileWorkflow, Now: func() time.Time { return now }, CWD: outside, Machine: machine})
	clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
	if _, err := server.SDKServer().Connect(context.Background(), serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "atlas-test", Version: "v0"}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	res, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name: "atlas.workspace.init",
		Arguments: map[string]any{
			"path":   "/Volumes/Hidden/secret-ws",
			"actor":  "human:owner",
			"reason": "should fail",
			"agents": false,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	blob, _ := json.Marshal(res)
	if strings.Contains(string(blob), "/Volumes/Hidden/secret-ws") {
		t.Fatalf("SDK error leaked init path: %s", blob)
	}
}
