package cli

import "testing"

func TestRootCommandIncludesRequiredTopLevelCommands(t *testing.T) {
	root := NewRootCommand()
	required := []string{
		"init", "doctor", "reindex", "config", "project", "ticket",
		"board", "backlog", "next", "blocked", "queue", "review-queue", "owner-queue",
		"who", "sweep", "inspect", "automation", "notify", "git", "views", "watch", "unwatch", "templates", "integrations", "search", "render", "shell", "tui",
	}
	for _, name := range required {
		if _, _, err := root.Find([]string{name}); err != nil {
			t.Fatalf("expected top-level command %q to exist: %v", name, err)
		}
	}
}
