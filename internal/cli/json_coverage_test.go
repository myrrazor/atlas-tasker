package cli

import (
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// The README promises JSON output on every command and the agent skill takes that
// literally. These are the only leaves allowed to skip --json; each one either owns
// stdout for something else or has no machine consumer at all.
var commandsWithoutJSON = map[string]string{
	"mcp serve": "owns stdout for the MCP JSON-RPC stream",
	"shell":     "interactive REPL; the commands it dispatches carry the flags",
	"tui":       "full-screen terminal UI",
	"web serve": "long-running server that streams status lines until it stops",
	"web open":  "hands a URL to the human browser board; agents use the CLI or MCP",
}

func TestEveryLeafCommandAcceptsJSON(t *testing.T) {
	var missing []string
	seen := map[string]bool{}
	for _, cmd := range leafCommands(t) {
		path := commandPath(cmd)
		seen[path] = true
		if cmd.Flags().Lookup("json") != nil {
			if _, allowed := commandsWithoutJSON[path]; allowed {
				t.Errorf("%q now has --json; drop it from commandsWithoutJSON", path)
			}
			continue
		}
		if _, allowed := commandsWithoutJSON[path]; allowed {
			continue
		}
		missing = append(missing, path)
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("commands missing --json:\n  %s", strings.Join(missing, "\n  "))
	}
	for path := range commandsWithoutJSON {
		if !seen[path] {
			t.Errorf("commandsWithoutJSON lists %q, which is no longer a leaf command", path)
		}
	}
}

// --json is only half the contract: the payload has to carry the envelope version
// too, which means going through writeCommandOutput rather than printing by hand.
func TestJSONOutputCarriesFormatVersion(t *testing.T) {
	withTempWorkspace(t)
	for _, args := range [][]string{
		{"init", "--json"},
		{"project", "create", "APP", "App", "--json"},
		{"ticket", "create", "--project", "APP", "--title", "First", "--type", "task", "--actor", "human:owner", "--reason", "seed", "--json"},
		{"ticket", "claim", "APP-1", "--actor", "human:owner", "--reason", "start", "--json"},
		{"ticket", "comment", "APP-1", "--body", "note", "--actor", "human:owner", "--reason", "context", "--json"},
		{"config", "get", "actor.default", "--json"},
	} {
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if !strings.Contains(out, `"format_version": "v1"`) {
			t.Fatalf("%v did not emit a versioned envelope:\n%s", args, out)
		}
	}
}

func leafCommands(t *testing.T) []*cobra.Command {
	t.Helper()
	var leaves []*cobra.Command
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		subs := cmd.Commands()
		if len(subs) == 0 {
			leaves = append(leaves, cmd)
			return
		}
		for _, sub := range subs {
			// cobra bolts these on at execute time; they are not ours to flag
			if sub.Name() == "help" || sub.Name() == "completion" {
				continue
			}
			walk(sub)
		}
	}
	walk(NewRootCommand())
	if len(leaves) < 100 {
		t.Fatalf("walked only %d leaf commands; the tree walk is broken", len(leaves))
	}
	return leaves
}

func commandPath(cmd *cobra.Command) string {
	parts := []string{}
	for current := cmd; current != nil && current.Parent() != nil; current = current.Parent() {
		parts = append([]string{current.Name()}, parts...)
	}
	return strings.Join(parts, " ")
}
