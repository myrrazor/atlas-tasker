package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestRootCommandIncludesRequiredTopLevelCommands(t *testing.T) {
	root := NewRootCommand()
	required := []string{
		"init", "doctor", "reindex", "config", "project", "agent", "run", "worktree", "dispatch", "approvals", "gate", "inbox", "change", "checks", "evidence", "handoff", "ticket",
		"permission-profile", "permissions", "import", "export", "archive", "compact", "dashboard", "timeline",
		"board", "backlog", "next", "blocked", "queue", "review-queue", "owner-queue",
		"who", "sweep", "inspect", "automation", "notify", "git", "views", "watch", "unwatch", "bulk", "templates", "integrations", "search", "render", "version", "shell", "mcp", "tui",
	}
	for _, name := range required {
		if _, _, err := root.Find([]string{name}); err != nil {
			t.Fatalf("expected top-level command %q to exist: %v", name, err)
		}
	}
}

func TestSearchAndReviewHelpDocumentWorkflowSyntax(t *testing.T) {
	root := NewRootCommand()
	search, _, err := root.Find([]string{"search"})
	if err != nil {
		t.Fatalf("find search command: %v", err)
	}
	searchHelp := search.Long
	for _, expected := range []string{"status=<STATUS>", "type=<TYPE>", "project=<KEY>", "assignee=<ACTOR>", "label=<LABEL>", "text~<TEXT>", "tracker search 'status=in_progress'", "tracker search 'project=AUTH text~logout'"} {
		if !strings.Contains(searchHelp, expected) {
			t.Fatalf("search help missing %q:\n%s", expected, searchHelp)
		}
	}
	assign, _, err := root.Find([]string{"ticket", "assign"})
	if err != nil {
		t.Fatalf("find ticket assign: %v", err)
	}
	if !strings.Contains(assign.Short+" "+assign.Long, "assignee only") || !strings.Contains(assign.Long, "request-review <ID> --reviewer") {
		t.Fatalf("assign help should clarify reviewer commands:\nshort=%s\nlong=%s", assign.Short, assign.Long)
	}
	requestReview, _, err := root.Find([]string{"ticket", "request-review"})
	if err != nil {
		t.Fatalf("find ticket request-review: %v", err)
	}
	if requestReview.Flag("reviewer") == nil || !strings.Contains(requestReview.Long, "--reviewer") {
		t.Fatalf("request-review help should document --reviewer")
	}
}

func TestV17BaseMetadataExists(t *testing.T) {
	raw, err := os.ReadFile("../../docs/v1.7-base.json")
	if err != nil {
		t.Fatalf("read v1.7 base json: %v", err)
	}
	var doc struct {
		Version    string `json:"version"`
		BaseSHA    string `json:"base_sha"`
		BaseBranch string `json:"base_branch"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse v1.7 base json: %v", err)
	}
	if doc.Version != "v1.7" || doc.BaseBranch != "origin/main" || doc.BaseSHA == "" {
		t.Fatalf("unexpected v1.7 base metadata: %+v", doc)
	}
	marker, err := os.ReadFile("../../docs/v1.7-base-marker.txt")
	if err != nil {
		t.Fatalf("read v1.7 base marker: %v", err)
	}
	if strings.TrimSpace(string(marker)) != doc.BaseSHA {
		t.Fatalf("base marker %q does not match json %q", strings.TrimSpace(string(marker)), doc.BaseSHA)
	}
}

func TestRootCommandIncludesRequiredV15SubcommandsAndFlags(t *testing.T) {
	root := NewRootCommand()
	type commandContract struct {
		path          []string
		mutating      bool
		hasOutputMode bool
	}
	contracts := []commandContract{
		{path: []string{"change", "list"}, hasOutputMode: true},
		{path: []string{"change", "view"}, hasOutputMode: true},
		{path: []string{"change", "create"}, mutating: true, hasOutputMode: true},
		{path: []string{"change", "status"}, hasOutputMode: true},
		{path: []string{"change", "sync"}, mutating: true, hasOutputMode: true},
		{path: []string{"change", "review-request"}, mutating: true, hasOutputMode: true},
		{path: []string{"change", "merge"}, mutating: true, hasOutputMode: true},
		{path: []string{"change", "link"}, mutating: true, hasOutputMode: true},
		{path: []string{"change", "import-url"}, mutating: true, hasOutputMode: true},
		{path: []string{"change", "unlink"}, mutating: true, hasOutputMode: true},
		{path: []string{"checks", "list"}, hasOutputMode: true},
		{path: []string{"checks", "view"}, hasOutputMode: true},
		{path: []string{"checks", "record"}, mutating: true, hasOutputMode: true},
		{path: []string{"checks", "sync"}, mutating: true, hasOutputMode: true},
		{path: []string{"permission-profile", "list"}, hasOutputMode: true},
		{path: []string{"permission-profile", "view"}, hasOutputMode: true},
		{path: []string{"permission-profile", "create"}, mutating: true, hasOutputMode: true},
		{path: []string{"permission-profile", "edit"}, mutating: true, hasOutputMode: true},
		{path: []string{"permission-profile", "bind"}, mutating: true, hasOutputMode: true},
		{path: []string{"permission-profile", "unbind"}, mutating: true, hasOutputMode: true},
		{path: []string{"permissions", "view"}, hasOutputMode: true},
		{path: []string{"import", "preview"}, mutating: true, hasOutputMode: true},
		{path: []string{"import", "apply"}, mutating: true, hasOutputMode: true},
		{path: []string{"import", "list"}, hasOutputMode: true},
		{path: []string{"import", "view"}, hasOutputMode: true},
		{path: []string{"export", "create"}, mutating: true, hasOutputMode: true},
		{path: []string{"export", "list"}, hasOutputMode: true},
		{path: []string{"export", "view"}, hasOutputMode: true},
		{path: []string{"export", "verify"}, mutating: true, hasOutputMode: true},
		{path: []string{"archive", "plan"}, hasOutputMode: true},
		{path: []string{"archive", "apply"}, mutating: true, hasOutputMode: true},
		{path: []string{"archive", "list"}, hasOutputMode: true},
		{path: []string{"archive", "restore"}, mutating: true, hasOutputMode: true},
		{path: []string{"compact"}, mutating: true, hasOutputMode: true},
		{path: []string{"dashboard"}, hasOutputMode: true},
		{path: []string{"timeline"}, hasOutputMode: true},
	}

	for _, item := range contracts {
		cmd, _, err := root.Find(item.path)
		if err != nil {
			t.Fatalf("expected command %q to exist: %v", item.path, err)
		}
		if cmd.Short == "" {
			t.Fatalf("expected command %q to have help text", item.path)
		}
		if item.hasOutputMode {
			for _, flag := range []string{"pretty", "md", "json"} {
				if cmd.Flag(flag) == nil {
					t.Fatalf("expected command %q to expose --%s", item.path, flag)
				}
			}
		}
		if item.mutating {
			for _, flag := range []string{"actor", "reason"} {
				if cmd.Flag(flag) == nil {
					t.Fatalf("expected mutating command %q to expose --%s", item.path, flag)
				}
			}
		}
	}
}

func TestRootCommandIncludesRequiredV16CommandsAndFlags(t *testing.T) {
	root := NewRootCommand()
	requiredTopLevel := []string{"collaborator", "membership", "remote", "sync", "bundle", "conflict", "mentions"}
	for _, name := range requiredTopLevel {
		if _, _, err := root.Find([]string{name}); err != nil {
			t.Fatalf("expected v1.6 top-level command %q to exist: %v", name, err)
		}
	}

	type commandContract struct {
		path          []string
		mutating      bool
		hasOutputMode bool
	}
	contracts := []commandContract{
		{path: []string{"collaborator", "list"}, hasOutputMode: true},
		{path: []string{"collaborator", "view"}, hasOutputMode: true},
		{path: []string{"collaborator", "add"}, mutating: true, hasOutputMode: true},
		{path: []string{"collaborator", "edit"}, mutating: true, hasOutputMode: true},
		{path: []string{"collaborator", "trust"}, mutating: true, hasOutputMode: true},
		{path: []string{"collaborator", "suspend"}, mutating: true, hasOutputMode: true},
		{path: []string{"collaborator", "remove"}, mutating: true, hasOutputMode: true},
		{path: []string{"membership", "list"}, hasOutputMode: true},
		{path: []string{"membership", "bind"}, mutating: true, hasOutputMode: true},
		{path: []string{"membership", "unbind"}, mutating: true, hasOutputMode: true},
		{path: []string{"remote", "list"}, hasOutputMode: true},
		{path: []string{"remote", "view"}, hasOutputMode: true},
		{path: []string{"remote", "add"}, mutating: true, hasOutputMode: true},
		{path: []string{"remote", "edit"}, mutating: true, hasOutputMode: true},
		{path: []string{"remote", "remove"}, mutating: true, hasOutputMode: true},
		{path: []string{"sync", "status"}, hasOutputMode: true},
		{path: []string{"sync", "jobs"}, hasOutputMode: true},
		{path: []string{"sync", "view"}, hasOutputMode: true},
		{path: []string{"sync", "fetch"}, mutating: true, hasOutputMode: true},
		{path: []string{"sync", "pull"}, mutating: true, hasOutputMode: true},
		{path: []string{"sync", "push"}, mutating: true, hasOutputMode: true},
		{path: []string{"sync", "run"}, mutating: true, hasOutputMode: true},
		{path: []string{"bundle", "create"}, mutating: true, hasOutputMode: true},
		{path: []string{"bundle", "list"}, hasOutputMode: true},
		{path: []string{"bundle", "view"}, hasOutputMode: true},
		{path: []string{"bundle", "verify"}, mutating: true, hasOutputMode: true},
		{path: []string{"bundle", "import"}, mutating: true, hasOutputMode: true},
		{path: []string{"conflict", "list"}, hasOutputMode: true},
		{path: []string{"conflict", "view"}, hasOutputMode: true},
		{path: []string{"conflict", "resolve"}, mutating: true, hasOutputMode: true},
		{path: []string{"mentions", "list"}, hasOutputMode: true},
		{path: []string{"mentions", "view"}, hasOutputMode: true},
		{path: []string{"project", "codeowners", "render"}, hasOutputMode: true},
		{path: []string{"project", "codeowners", "write"}, mutating: true, hasOutputMode: true},
		{path: []string{"project", "rules", "render"}, hasOutputMode: true},
		{path: []string{"inbox"}, hasOutputMode: true},
		{path: []string{"approvals"}, hasOutputMode: true},
		{path: []string{"dashboard"}, hasOutputMode: true},
		{path: []string{"timeline"}, hasOutputMode: true},
	}

	for _, item := range contracts {
		cmd, _, err := root.Find(item.path)
		if err != nil {
			t.Fatalf("expected command %q to exist: %v", item.path, err)
		}
		if cmd.Short == "" {
			t.Fatalf("expected command %q to have help text", item.path)
		}
		if item.hasOutputMode {
			for _, flag := range []string{"pretty", "md", "json"} {
				if cmd.Flag(flag) == nil {
					t.Fatalf("expected command %q to expose --%s", item.path, flag)
				}
			}
		}
		if item.mutating {
			for _, flag := range []string{"actor", "reason"} {
				if cmd.Flag(flag) == nil {
					t.Fatalf("expected mutating command %q to expose --%s", item.path, flag)
				}
			}
		}
	}

	for _, path := range [][]string{{"inbox"}, {"approvals"}, {"dashboard"}, {"timeline"}} {
		cmd, _, err := root.Find(path)
		if err != nil {
			t.Fatalf("expected collaborator-aware command %q to exist: %v", path, err)
		}
		if cmd.Flag("collaborator") == nil {
			t.Fatalf("expected command %q to expose --collaborator", path)
		}
	}
}

func TestRootCommandIncludesRequiredV17ContractCommandsAndFlags(t *testing.T) {
	root := NewRootCommand()
	requiredTopLevel := []string{"key", "trust", "sign", "verify", "governance", "classify", "redact", "audit", "backup", "admin", "goal"}
	for _, name := range requiredTopLevel {
		if _, _, err := root.Find([]string{name}); err != nil {
			t.Fatalf("expected v1.7 top-level command %q to exist: %v", name, err)
		}
	}

	type commandContract struct {
		path          []string
		mutating      bool
		hasOutputMode bool
		extraFlags    []string
	}
	contracts := []commandContract{
		{path: []string{"key", "list"}, hasOutputMode: true},
		{path: []string{"key", "view"}, hasOutputMode: true},
		{path: []string{"key", "generate"}, mutating: true, hasOutputMode: true, extraFlags: []string{"scope"}},
		{path: []string{"key", "export-public"}, hasOutputMode: true},
		{path: []string{"key", "import-public"}, mutating: true, hasOutputMode: true},
		{path: []string{"key", "rotate"}, mutating: true, hasOutputMode: true},
		{path: []string{"key", "revoke"}, mutating: true, hasOutputMode: true},
		{path: []string{"key", "verify"}, hasOutputMode: true},
		{path: []string{"trust", "status"}, hasOutputMode: true},
		{path: []string{"trust", "list"}, hasOutputMode: true},
		{path: []string{"trust", "collaborator"}, hasOutputMode: true},
		{path: []string{"trust", "bind-key"}, mutating: true, hasOutputMode: true},
		{path: []string{"trust", "revoke-key"}, mutating: true, hasOutputMode: true},
		{path: []string{"trust", "explain"}, hasOutputMode: true},
		{path: []string{"sign", "bundle"}, mutating: true, hasOutputMode: true, extraFlags: []string{"signing-key"}},
		{path: []string{"sign", "sync-publication"}, mutating: true, hasOutputMode: true, extraFlags: []string{"signing-key"}},
		{path: []string{"sign", "approval"}, mutating: true, hasOutputMode: true, extraFlags: []string{"signing-key"}},
		{path: []string{"sign", "handoff"}, mutating: true, hasOutputMode: true, extraFlags: []string{"signing-key"}},
		{path: []string{"sign", "evidence"}, mutating: true, hasOutputMode: true, extraFlags: []string{"signing-key"}},
		{path: []string{"sign", "audit"}, mutating: true, hasOutputMode: true, extraFlags: []string{"signing-key"}},
		{path: []string{"sign", "audit-packet"}, mutating: true, hasOutputMode: true, extraFlags: []string{"signing-key"}},
		{path: []string{"sign", "backup"}, mutating: true, hasOutputMode: true, extraFlags: []string{"signing-key"}},
		{path: []string{"sign", "goal"}, mutating: true, hasOutputMode: true, extraFlags: []string{"signing-key"}},
		{path: []string{"verify", "bundle"}, hasOutputMode: true},
		{path: []string{"verify", "sync-publication"}, hasOutputMode: true},
		{path: []string{"verify", "approval"}, hasOutputMode: true},
		{path: []string{"verify", "handoff"}, hasOutputMode: true},
		{path: []string{"verify", "evidence"}, hasOutputMode: true},
		{path: []string{"verify", "audit"}, hasOutputMode: true},
		{path: []string{"verify", "audit-packet"}, hasOutputMode: true},
		{path: []string{"verify", "backup"}, hasOutputMode: true},
		{path: []string{"verify", "goal"}, hasOutputMode: true},
		{path: []string{"governance", "pack", "list"}, hasOutputMode: true},
		{path: []string{"governance", "pack", "view"}, hasOutputMode: true},
		{path: []string{"governance", "pack", "create"}, mutating: true, hasOutputMode: true},
		{path: []string{"governance", "pack", "apply"}, mutating: true, hasOutputMode: true, extraFlags: []string{"scope"}},
		{path: []string{"governance", "validate"}, hasOutputMode: true},
		{path: []string{"governance", "explain"}, hasOutputMode: true},
		{path: []string{"governance", "simulate"}, hasOutputMode: true, extraFlags: []string{"actor", "ticket", "run", "change"}},
		{path: []string{"classify", "get"}, hasOutputMode: true},
		{path: []string{"classify", "set"}, mutating: true, hasOutputMode: true},
		{path: []string{"classify", "list"}, hasOutputMode: true, extraFlags: []string{"project"}},
		{path: []string{"classify", "explain"}, hasOutputMode: true},
		{path: []string{"redact", "preview"}, mutating: true, hasOutputMode: true, extraFlags: []string{"scope", "target"}},
		{path: []string{"redact", "export"}, mutating: true, hasOutputMode: true, extraFlags: []string{"scope", "preview-id"}},
		{path: []string{"redact", "verify"}, hasOutputMode: true},
		{path: []string{"audit", "report"}, mutating: true, hasOutputMode: true, extraFlags: []string{"scope"}},
		{path: []string{"audit", "list"}, hasOutputMode: true},
		{path: []string{"audit", "view"}, hasOutputMode: true},
		{path: []string{"audit", "export"}, mutating: true, hasOutputMode: true},
		{path: []string{"audit", "verify"}, hasOutputMode: true},
		{path: []string{"audit", "explain-policy"}, hasOutputMode: true},
		{path: []string{"backup", "create"}, mutating: true, hasOutputMode: true, extraFlags: []string{"scope"}},
		{path: []string{"backup", "list"}, hasOutputMode: true},
		{path: []string{"backup", "view"}, hasOutputMode: true},
		{path: []string{"backup", "verify"}, hasOutputMode: true},
		{path: []string{"backup", "restore-plan"}, hasOutputMode: true},
		{path: []string{"backup", "restore-apply"}, mutating: true, hasOutputMode: true, extraFlags: []string{"yes"}},
		{path: []string{"backup", "drill"}, hasOutputMode: true},
		{path: []string{"admin", "security-status"}, hasOutputMode: true},
		{path: []string{"admin", "trust-store"}, hasOutputMode: true},
		{path: []string{"admin", "recovery-status"}, hasOutputMode: true},
		{path: []string{"goal", "brief"}, hasOutputMode: true},
		{path: []string{"goal", "manifest"}, hasOutputMode: true},
		{path: []string{"goal", "verify"}, hasOutputMode: true},
	}

	for _, item := range contracts {
		cmd, _, err := root.Find(item.path)
		if err != nil {
			t.Fatalf("expected command %q to exist: %v", item.path, err)
		}
		if cmd.Short == "" {
			t.Fatalf("expected command %q to have help text", item.path)
		}
		if item.hasOutputMode {
			for _, flag := range []string{"pretty", "md", "json"} {
				if cmd.Flag(flag) == nil {
					t.Fatalf("expected command %q to expose --%s", item.path, flag)
				}
			}
		}
		if item.mutating {
			for _, flag := range []string{"actor", "reason"} {
				if cmd.Flag(flag) == nil {
					t.Fatalf("expected mutating command %q to expose --%s", item.path, flag)
				}
			}
		}
		for _, flag := range item.extraFlags {
			if cmd.Flag(flag) == nil {
				t.Fatalf("expected command %q to expose --%s", item.path, flag)
			}
		}
	}
}

func TestV16HelpTextIncludesKeyFlagsAndSubcommands(t *testing.T) {
	root := NewRootCommand()
	type helpCheck struct {
		path     []string
		snippets []string
	}
	checks := []helpCheck{
		{path: []string{"collaborator"}, snippets: []string{"add", "trust", "remove"}},
		{path: []string{"membership", "bind"}, snippets: []string{"--scope-kind", "--scope-id", "--role"}},
		{path: []string{"remote", "add"}, snippets: []string{"--location", "--default-action"}},
		{path: []string{"sync", "pull"}, snippets: []string{"--remote", "--workspace"}},
		{path: []string{"bundle", "import"}, snippets: []string{"--json", "--actor", "--reason"}},
		{path: []string{"conflict", "resolve"}, snippets: []string{"--resolution", "--actor", "--reason"}},
		{path: []string{"mentions", "list"}, snippets: []string{"--collaborator", "--json"}},
		{path: []string{"inbox"}, snippets: []string{"--collaborator", "--json"}},
		{path: []string{"approvals"}, snippets: []string{"--collaborator", "--json"}},
		{path: []string{"dashboard"}, snippets: []string{"--collaborator", "--json"}},
		{path: []string{"timeline"}, snippets: []string{"--collaborator", "--json"}},
		{path: []string{"project", "codeowners", "write"}, snippets: []string{"--actor", "--reason", "--json"}},
		{path: []string{"project", "rules", "render"}, snippets: []string{"--json"}},
	}

	for _, check := range checks {
		cmd, _, err := root.Find(check.path)
		if err != nil {
			t.Fatalf("find %q: %v", check.path, err)
		}
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		if err := cmd.Help(); err != nil {
			t.Fatalf("help for %q: %v", check.path, err)
		}
		help := buf.String()
		for _, snippet := range check.snippets {
			if !strings.Contains(help, snippet) {
				t.Fatalf("expected help for %q to contain %q\n%s", check.path, snippet, help)
			}
		}
	}
}
