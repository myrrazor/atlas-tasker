package cli

import "testing"

func TestRootCommandIncludesRequiredTopLevelCommands(t *testing.T) {
	root := NewRootCommand()
	required := []string{
		"init", "doctor", "reindex", "config", "project", "agent", "run", "worktree", "dispatch", "approvals", "gate", "inbox", "change", "checks", "evidence", "handoff", "ticket",
		"permission-profile", "permissions", "import", "export", "archive", "compact", "dashboard", "timeline",
		"board", "backlog", "next", "blocked", "queue", "review-queue", "owner-queue",
		"who", "sweep", "inspect", "automation", "notify", "git", "views", "watch", "unwatch", "bulk", "templates", "integrations", "search", "render", "shell", "tui",
	}
	for _, name := range required {
		if _, _, err := root.Find([]string{name}); err != nil {
			t.Fatalf("expected top-level command %q to exist: %v", name, err)
		}
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
