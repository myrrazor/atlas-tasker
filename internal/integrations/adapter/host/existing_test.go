package host

import (
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
)

func TestServersFromTOMLAndJSONOwnership(t *testing.T) {
	const ws = "ws-adapter-test"
	name, err := adapter.ServerNameFor(ws)
	if err != nil {
		t.Fatal(err)
	}
	tomlRaw := []byte("[mcp_servers." + name + "]\ncommand = \"/usr/local/bin/tracker\"\nargs = [\"mcp\", \"serve\", \"--expected-workspace-id\", \"" + ws + "\"]\nrequired = false\n")
	found := serversFromTOML(tomlRaw)
	if len(found) != 1 || found[0].Name != name || !atlasOwnedEntry(found[0].Entry, ws) {
		t.Fatalf("owned TOML: %#v", found)
	}
	foreign := []byte("[mcp_servers." + name + "]\ncommand = \"/usr/bin/other\"\nargs = [\"serve\"]\n")
	if !TOMLHasUnmanaged(foreign, name, ws) {
		t.Fatal("expected unmanaged TOML")
	}
	nested := []byte(`{"projects":{"/tmp/ws":{"mcpServers":{"` + name + `":{"type":"stdio","command":"/usr/local/bin/tracker","args":["mcp","serve","--expected-workspace-id","` + ws + `"]}}}}}`)
	jsonFound := serversFromJSON(nested)
	if len(jsonFound) != 1 || !atlasOwnedEntry(jsonFound[0].Entry, ws) {
		t.Fatalf("nested JSON: %#v", jsonFound)
	}
}

func TestServerNamesFromCLIIgnoreUnrelated(t *testing.T) {
	names := serverNamesFromCLI("other-server\nfilesystem\n", "atlas-aaaaaaaaaaaa")
	if strings.Contains(strings.Join(names, ","), "atlas-aaaaaaaaaaaa") {
		t.Fatalf("unrelated list mentioned expected name: %v", names)
	}
	names = serverNamesFromCLI("{\"mcpServers\":{\"other\":{}}}", "atlas-aaaaaaaaaaaa")
	if len(names) != 1 || names[0] != "other" {
		t.Fatalf("json list: %v", names)
	}
}

func TestCLIOutputOwnedRequiresPerEntryArgs(t *testing.T) {
	const ws = "ws-adapter-test"
	ownedName, err := adapter.ServerNameFor(ws)
	if err != nil {
		t.Fatal(err)
	}
	foreign := "atlas-deadbeefdead"
	blob := foreign + "\n" + ownedName + ": /usr/local/bin/tracker mcp serve --expected-workspace-id " + ws + "\n"
	if cliOutputOwned(blob, foreign, ws) {
		t.Fatal("foreign atlas name must not inherit another entry's command tokens")
	}
	if !cliOutputOwned(blob, ownedName, ws) {
		t.Fatal("canonical line should be owned")
	}
	if cliOutputOwned("other-server\nfilesystem\n", ownedName, ws) {
		t.Fatal("unrelated list must not be owned")
	}
}

func TestGetShowWithoutServerNameIsNotExisting(t *testing.T) {
	const ws = "ws-adapter-test"
	name, err := adapter.ServerNameFor(ws)
	if err != nil {
		t.Fatal(err)
	}
	cmd := adapter.Command{Purpose: adapter.CommandPurposeInspect, Executable: "/usr/bin/claude", Args: []string{"mcp", "get", name}}
	found := serversFromCLIOutput(cmd, "other-server\nfilesystem\n", name, ws)
	if len(found) != 0 {
		t.Fatalf("unrelated get/show must not invent %s: %#v", name, found)
	}
}
