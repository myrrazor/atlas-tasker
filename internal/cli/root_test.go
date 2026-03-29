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
