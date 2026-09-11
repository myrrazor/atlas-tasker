package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	toml "github.com/pelletier/go-toml/v2"
)

var atlasServerNameRe = regexp.MustCompile(`atlas-[0-9a-f]{12}`)

type namedServer struct {
	Name  string
	Entry adapter.StandardServerEntry
	OK    bool
}

func (a *Adapter) scanExistingServers(ctx context.Context, detection *adapter.Detection, input adapter.DetectInput) error {
	caps := a.Capabilities()
	root := input.WorkspaceRoot
	home := input.Home
	read := input.ReadFile
	if read == nil {
		read = os.ReadFile
	}
	workspaceID := workspaceIDFromRoot(root, read)
	for _, scope := range caps.Scopes {
		path, ok := scope.ResolvePath(root, home)
		if !ok {
			continue
		}
		raw, err := read(path)
		if err != nil {
			continue
		}
		for _, found := range serversFromConfig(raw, scope.Format) {
			addExisting(detection, adapter.ExistingServer{
				Name:       found.Name,
				Scope:      scope.Scope,
				AtlasOwned: found.OK && atlasOwnedEntry(found.Entry, workspaceID),
			})
		}
	}
	for _, also := range caps.AlsoLoads {
		resolved, ok := (adapter.ScopeCapability{Path: also}).ResolvePath(root, home)
		if !ok {
			continue
		}
		raw, err := read(resolved)
		if err != nil {
			continue
		}
		for _, found := range serversFromConfig(raw, formatForPath(resolved)) {
			owned := found.OK && atlasOwnedEntry(found.Entry, workspaceID)
			addExisting(detection, adapter.ExistingServer{
				Name:       found.Name,
				Scope:      adapter.ScopeProjectShared,
				AtlasOwned: owned,
			})
			if strings.HasPrefix(found.Name, adapter.ServerNamePrefix) {
				detection.Reasons = append(detection.Reasons, "compatibility import also loads Atlas server "+found.Name+" from "+also)
			}
		}
	}
	a.scanCLIServers(ctx, detection, input, workspaceID)
	return nil
}

func (a *Adapter) scanCLIServers(ctx context.Context, detection *adapter.Detection, input adapter.DetectInput, workspaceID string) {
	if detection.ExecutablePath == "" || !preferredWriteIsCLI(a.target) {
		return
	}
	runner := input.Runner
	if runner == nil {
		runner = a.runner
	}
	if runner == nil {
		return
	}
	expectedName := ""
	if workspaceID != "" {
		if name, err := adapter.ServerNameFor(workspaceID); err == nil {
			expectedName = name
		}
	}
	for _, cmd := range clientInventoryCommands(a.target, detection.ExecutablePath, expectedName, input.WorkspaceRoot) {
		if err := cmd.Validate(); err != nil {
			continue
		}
		result, err := runner.Run(ctx, cmd)
		detail := strings.TrimSpace(string(result.Stdout) + "\n" + string(result.Stderr))
		probe := adapter.ProbeRecord{Command: cmd, ExitCode: result.ExitCode, TimedOut: result.TimedOut, Summary: truncateDetail(detail)}
		if err != nil && probe.Summary == "" {
			probe.Summary = err.Error()
		}
		detection.Probes = append(detection.Probes, probe)
		if result.ExitCode != 0 || result.TimedOut {
			continue
		}
		for _, found := range serversFromCLIOutput(cmd, detail, expectedName, workspaceID) {
			addExisting(detection, adapter.ExistingServer{
				Name:       found.Name,
				Scope:      a.Capabilities().PreferredScope,
				AtlasOwned: found.OK && atlasOwnedEntry(found.Entry, workspaceID),
			})
		}
	}
}

func preferredWriteIsCLI(target integrations.Target) bool {
	caps, err := adapter.CapabilitiesFor(target)
	if err != nil {
		return false
	}
	preferred, ok := caps.Preferred()
	return ok && preferred.WriteMethod == adapter.WriteMethodClientCLI
}

func addExisting(detection *adapter.Detection, server adapter.ExistingServer) {
	if server.Name == "" {
		return
	}
	for i, existing := range detection.ExistingServers {
		if existing.Name == server.Name && existing.Scope == server.Scope {
			if server.AtlasOwned || !existing.AtlasOwned {
				detection.ExistingServers[i] = server
			}
			return
		}
	}
	detection.ExistingServers = append(detection.ExistingServers, server)
}

func serversFromConfig(raw []byte, format adapter.ConfigFormat) []namedServer {
	switch format {
	case adapter.ConfigFormatTOML:
		return serversFromTOML(raw)
	default:
		return serversFromJSON(raw)
	}
}

func formatForPath(path string) adapter.ConfigFormat {
	if strings.HasSuffix(path, ".toml") {
		return adapter.ConfigFormatTOML
	}
	return adapter.ConfigFormatJSON
}

func serversFromJSON(raw []byte) []namedServer {
	var doc any
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	var out []namedServer
	walkMCPServers(doc, func(name string, value any) {
		entry, ok := decodeServerValue(value)
		out = append(out, namedServer{Name: name, Entry: entry, OK: ok})
	})
	return out
}

func walkMCPServers(node any, fn func(name string, value any)) {
	switch typed := node.(type) {
	case map[string]any:
		if servers, ok := typed["mcpServers"]; ok {
			if table, ok := servers.(map[string]any); ok {
				for name, value := range table {
					fn(name, value)
				}
			}
		}
		for _, value := range typed {
			walkMCPServers(value, fn)
		}
	case []any:
		for _, value := range typed {
			walkMCPServers(value, fn)
		}
	}
}

func serversFromTOML(raw []byte) []namedServer {
	var doc map[string]any
	if toml.Unmarshal(raw, &doc) != nil {
		return nil
	}
	table, _ := doc["mcp_servers"].(map[string]any)
	if table == nil {
		return nil
	}
	var out []namedServer
	for name, value := range table {
		entry, ok := decodeServerValue(value)
		out = append(out, namedServer{Name: name, Entry: entry, OK: ok})
	}
	return out
}

func decodeServerValue(value any) (adapter.StandardServerEntry, bool) {
	raw, err := json.Marshal(value)
	if err != nil {
		return adapter.StandardServerEntry{}, false
	}
	var entry adapter.StandardServerEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return adapter.StandardServerEntry{}, false
	}
	if entry.Command == "" {
		if fields, ok := value.(map[string]any); ok {
			if command, ok := fields["command"].(string); ok {
				entry.Command = command
			}
			entry.Args = stringSlice(fields["args"])
		}
	}
	return entry, entry.Command != ""
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			s, ok := item.(string)
			if !ok {
				return nil
			}
			out = append(out, s)
		}
		return out
	default:
		return nil
	}
}

func atlasOwnedEntry(entry adapter.StandardServerEntry, workspaceID string) bool {
	if workspaceID == "" || entry.Command == "" {
		return false
	}
	if entry.Type != "" && entry.Type != adapter.RegistrationTransportStdio {
		return false
	}
	base := filepath.Base(entry.Command)
	if base != adapter.PortableExecutableName && base != "tracker" {
		return false
	}
	hasMCP, hasServe, hasExpected := false, false, false
	for i, arg := range entry.Args {
		switch arg {
		case "mcp":
			hasMCP = true
		case "serve":
			hasServe = true
		case "--expected-workspace-id":
			if i+1 < len(entry.Args) && entry.Args[i+1] == workspaceID {
				hasExpected = true
			}
		}
	}
	return hasMCP && hasServe && hasExpected
}

func serversFromCLIOutput(cmd adapter.Command, raw, expectedName, workspaceID string) []namedServer {
	_ = workspaceID
	seen := map[string]namedServer{}
	add := func(name string, entry adapter.StandardServerEntry, ok bool) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if cur, exists := seen[name]; exists && cur.OK {
			return
		}
		seen[name] = namedServer{Name: name, Entry: entry, OK: ok}
	}
	for _, found := range serversFromJSON([]byte(raw)) {
		add(found.Name, found.Entry, found.OK)
	}
	for _, name := range namedServersInText(raw) {
		if _, exists := seen[name]; exists {
			continue
		}
		entry, ok := entryForNamedServer(raw, name)
		add(name, entry, ok)
	}
	if !isListCommand(cmd) && expectedName != "" {
		if mentionsServer(raw, expectedName) {
			if entry, ok := entryForNamedServer(raw, expectedName); ok {
				add(expectedName, entry, true)
			} else {
				add(expectedName, adapter.StandardServerEntry{}, false)
			}
		} else if entry, ok := singleInspectEntry(raw); ok {
			// get/show of this name may print only command/args.
			add(expectedName, entry, true)
		}
	}
	out := make([]namedServer, 0, len(seen))
	for _, found := range seen {
		out = append(out, found)
	}
	return out
}

func namedServersInText(raw string) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	var doc any
	if json.Unmarshal([]byte(strings.TrimSpace(raw)), &doc) == nil {
		walkMCPServers(doc, func(name string, _ any) { add(name) })
		collectJSONNames(doc, add)
	}
	for _, name := range atlasServerNameRe.FindAllString(raw, -1) {
		add(name)
	}
	return out
}

func serverNamesFromCLI(raw, extra string) []string {
	out := namedServersInText(raw)
	if extra != "" && mentionsServer(raw, extra) {
		found := false
		for _, name := range out {
			if name == extra {
				found = true
				break
			}
		}
		if !found {
			out = append(out, extra)
		}
	}
	return out
}

func collectJSONNames(node any, add func(string)) {
	switch typed := node.(type) {
	case map[string]any:
		if name, ok := typed["name"].(string); ok {
			add(name)
		}
		if name, ok := typed["server"].(string); ok {
			add(name)
		}
		for _, value := range typed {
			collectJSONNames(value, add)
		}
	case []any:
		for _, value := range typed {
			collectJSONNames(value, add)
		}
	}
}

func cliOutputOwned(raw, serverName, workspaceID string) bool {
	entry, ok := entryForNamedServer(raw, serverName)
	return ok && atlasOwnedEntry(entry, workspaceID)
}

func entryForNamedServer(raw, serverName string) (adapter.StandardServerEntry, bool) {
	if serverName == "" {
		return adapter.StandardServerEntry{}, false
	}
	for _, found := range serversFromJSON([]byte(raw)) {
		if found.Name == serverName && found.OK {
			return found.Entry, true
		}
	}
	if doc := jsonObjectOrNil(raw); doc != nil {
		if fields, ok := doc.(map[string]any); ok {
			name, _ := fields["name"].(string)
			entry, ok := decodeServerValue(doc)
			switch {
			case !ok:
			case name == serverName:
				return entry, true
			case name == "" && mentionsServer(raw, serverName) && len(atlasServerNameRe.FindAllString(raw, -1)) <= 1:
				return entry, true
			}
		}
	}
	var scoped []string
	for _, line := range strings.Split(raw, "\n") {
		if mentionsServer(line, serverName) {
			scoped = append(scoped, line)
		}
	}
	if len(scoped) == 0 {
		return adapter.StandardServerEntry{}, false
	}
	return parseCommandArgsFromText(strings.Join(scoped, "\n"))
}

func singleInspectEntry(raw string) (adapter.StandardServerEntry, bool) {
	if len(serversFromJSON([]byte(raw))) > 0 {
		return adapter.StandardServerEntry{}, false
	}
	if entry, ok := decodeServerValue(jsonObjectOrNil(raw)); ok {
		return entry, true
	}
	return adapter.StandardServerEntry{}, false
}

func parseCommandArgsFromText(raw string) (adapter.StandardServerEntry, bool) {
	fields := strings.Fields(raw)
	for i, field := range fields {
		cleaned := strings.Trim(field, `"'`)
		base := filepath.Base(cleaned)
		if base != adapter.PortableExecutableName && base != "tracker" && base != "atlas-tasker" {
			continue
		}
		return adapter.StandardServerEntry{
			Type:    adapter.RegistrationTransportStdio,
			Command: cleaned,
			Args:    append([]string(nil), fields[i+1:]...),
		}, true
	}
	return adapter.StandardServerEntry{}, false
}

func jsonObjectOrNil(raw string) any {
	var doc any
	if json.Unmarshal([]byte(strings.TrimSpace(raw)), &doc) != nil {
		return nil
	}
	return doc
}

func workspaceIDFromRoot(root string, read func(string) ([]byte, error)) string {
	if strings.TrimSpace(root) == "" {
		return ""
	}
	if read == nil {
		read = os.ReadFile
	}
	raw, err := read(storage.WorkspaceMetadataFile(root))
	if err != nil {
		return ""
	}
	var meta struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if json.Unmarshal(raw, &meta) != nil {
		return ""
	}
	return strings.TrimSpace(meta.WorkspaceID)
}

func mentionsServer(detail, serverName string) bool {
	if serverName == "" {
		return false
	}
	idx := strings.Index(detail, serverName)
	if idx < 0 {
		return false
	}
	if idx > 0 && isNameChar(detail[idx-1]) {
		return false
	}
	end := idx + len(serverName)
	if end < len(detail) && isNameChar(detail[end]) {
		return false
	}
	return true
}

func isNameChar(b byte) bool {
	return b == '_' || b == '-' || (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}

func TOMLHasUnmanaged(raw []byte, name, workspaceID string) bool {
	for _, found := range serversFromTOML(raw) {
		if found.Name == name {
			return !found.OK || !atlasOwnedEntry(found.Entry, workspaceID)
		}
	}
	return false
}

func AtlasOwnedSameName(detection adapter.Detection, name string) bool {
	for _, existing := range detection.ExistingServers {
		if existing.Name == name && existing.AtlasOwned {
			return true
		}
	}
	return false
}
