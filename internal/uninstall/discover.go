package uninstall

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

func goosOf(opts Options) string {
	if opts.GOOS != "" {
		return opts.GOOS
	}
	return runtime.GOOS
}

func launchAgentsDir(home string) string {
	return filepath.Join(home, "Library", "LaunchAgents")
}

func systemdUserDir(home string) string {
	return filepath.Join(home, ".config", "systemd", "user")
}

func discoverUnitActions(home, goos, binary string) []ManifestAction {
	var actions []ManifestAction
	switch goos {
	case "darwin":
		dir := launchAgentsDir(home)
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !allowedUnitName(name) {
				continue
			}
			path := filepath.Join(dir, name)
			owned, marker := inspectOwnedUnit(path, binary)
			if !owned {
				continue
			}
			unit := unitLabel(path)
			actions = append(actions, ManifestAction{
				ID: "stop-" + unit, Kind: KindStopService, UnitName: unit, Path: path,
				Marker: marker, BinaryPath: binary, Label: unit,
			})
		}
	default:
		dir := systemdUserDir(home)
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !allowedUnitName(name) {
				continue
			}
			path := filepath.Join(dir, name)
			if strings.HasSuffix(name, ".timer") {
				continue
			}
			owned, marker := inspectOwnedUnit(path, binary)
			if !owned {
				continue
			}
			action := ManifestAction{
				ID: "stop-" + name, Kind: KindStopService, UnitName: name, Path: path,
				Marker: marker, BinaryPath: binary, Label: name,
			}
			if timer := pairedTimerPath(path); timerHasMarker(timer) {
				action.PairPath = timer
			}
			actions = append(actions, action)
		}
	}
	return actions
}

func inspectOwnedUnit(path, binary string) (bool, string) {
	if requireOwnedRegularPath(path) != nil {
		return false, ""
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, ""
	}
	body := string(raw)
	if !unitHasMarker(body) {
		return false, ""
	}
	var prog []string
	switch {
	case strings.HasSuffix(path, ".plist"):
		prog = parsePlistProgramArgs(body)
	case strings.HasSuffix(path, ".service"):
		prog = parseSystemdExecStart(body)
	default:
		return false, ""
	}
	if !exactProgramBinary(prog, binary) {
		return false, ""
	}
	marker := scheduleMarker
	if strings.Contains(body, homeServiceMarker) {
		marker = homeServiceMarker
	}
	return true, marker
}

func timerHasMarker(path string) bool {
	if requireOwnedRegularPath(path) != nil {
		return false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return unitHasMarker(string(raw))
}

func atlasOwnedUnit(path, binary string) bool {
	owned, _ := inspectOwnedUnit(path, binary)
	return owned
}

func discoverClientActions(home, stateDir, binary string) []ManifestAction {
	var actions []ManifestAction
	seen := map[string]struct{}{}
	addAll := func(items []ManifestAction) {
		for _, action := range items {
			if action.Path == "" {
				continue
			}
			key := action.Kind + ":" + action.Path + ":" + action.EntryKey + ":" + action.Marker
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			actions = append(actions, action)
		}
	}
	for _, path := range globalClientJSONPaths(home, stateDir) {
		addAll(scanJSONConfig(path, binary))
	}
	for _, path := range globalClientTOMLPaths(home) {
		addAll(scanTOMLConfig(path, binary))
	}
	roots := registryWorkspaceRoots(stateDir)
	for _, root := range roots {
		addAll(scanJSONConfig(filepath.Join(root, ".cursor", "mcp.json"), binary))
		addAll(scanJSONConfig(filepath.Join(root, ".codex", "mcp.json"), binary))
		addAll(scanTOMLConfig(filepath.Join(root, ".codex", "config.toml"), binary))
		addAll(scanMarkdown(filepath.Join(root, "AGENTS.md")))
		addAll(scanMarkdown(filepath.Join(root, "CLAUDE.md")))
	}
	return actions
}

func globalClientJSONPaths(home, stateDir string) []string {
	return []string{
		filepath.Join(home, ".cursor", "mcp.json"),
		filepath.Join(home, ".claude.json"),
		filepath.Join(stateDir, "integrations", "atlas-mcp.json"),
		filepath.Join(home, ".grok", "mcp.json"),
		filepath.Join(home, ".codex", "mcp.json"),
		filepath.Join(home, ".atlas-tasker", "claude-mcp.json"),
		filepath.Join(home, ".atlas-tasker", "openclaw-mcp.json"),
		filepath.Join(home, ".atlas-tasker", "generic-mcp.json"),
	}
}

func globalClientTOMLPaths(home string) []string {
	return []string{
		filepath.Join(home, ".codex", "config.toml"),
		filepath.Join(home, ".grok", "config.toml"),
	}
}

func scanJSONConfig(path, binary string) []ManifestAction {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var doc any
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	var out []ManifestAction
	collectJSONServers(doc, path, binary, &out)
	return out
}

func collectJSONServers(node any, path, binary string, out *[]ManifestAction) {
	obj, ok := node.(map[string]any)
	if !ok {
		return
	}
	if raw, ok := obj["mcpServers"]; ok {
		servers, _ := raw.(map[string]any)
		for key, value := range servers {
			if !allowedServerKey(key) {
				continue
			}
			entry, _ := value.(map[string]any)
			command, _ := entry["command"].(string)
			if command != binary {
				continue
			}
			*out = append(*out, ManifestAction{
				ID: "json-" + key + "-" + strconv.Itoa(len(*out)), Kind: KindRemoveConfigEntry,
				Path: path, EntryKey: key, ConfigFormat: "json", BinaryPath: binary,
				Args: jsonStringSlice(entry["args"]),
			})
		}
	}
	for _, value := range obj {
		collectJSONServers(value, path, binary, out)
	}
}

func jsonStringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, _ := item.(string)
		out = append(out, s)
	}
	return out
}

func scanTOMLConfig(path, binary string) []ManifestAction {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var doc map[string]any
	if err := toml.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	servers, _ := doc["mcp_servers"].(map[string]any)
	var out []ManifestAction
	for key, value := range servers {
		if !allowedServerKey(key) {
			continue
		}
		entry, _ := value.(map[string]any)
		command, _ := entry["command"].(string)
		if command != binary {
			continue
		}
		out = append(out, ManifestAction{
			ID: "toml-" + key, Kind: KindRemoveConfigEntry,
			Path: path, EntryKey: key, ConfigFormat: "toml", BinaryPath: binary,
			Args: tomlStringSlice(entry["args"]),
		})
	}
	return out
}

func scanMarkdown(path string) []ManifestAction {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	body := string(raw)
	var out []ManifestAction
	for i, pair := range allowedMarkdownMarkers {
		if strings.Contains(body, pair[0]) && strings.Contains(body, pair[1]) {
			out = append(out, ManifestAction{
				ID:   "md-" + filepath.Base(path) + "-" + strconv.Itoa(i),
				Kind: KindRemoveManagedBlock, Path: path, Marker: pair[0], ConfigFormat: "markdown",
			})
		}
	}
	return out
}

func registryWorkspaceRoots(stateDir string) []string {
	raw, err := os.ReadFile(filepath.Join(stateDir, "registry.json"))
	if err != nil {
		return nil
	}
	var probe struct {
		Workspaces json.RawMessage `json:"workspaces"`
	}
	if json.Unmarshal(raw, &probe) != nil || len(probe.Workspaces) == 0 {
		return nil
	}
	type entry struct {
		CanonicalPath string `json:"canonical_path"`
		Path          string `json:"path"`
	}
	var roots []string
	switch probe.Workspaces[0] {
	case '{':
		var m map[string]entry
		if json.Unmarshal(probe.Workspaces, &m) != nil {
			return nil
		}
		for _, item := range m {
			if path := firstAbs(item.CanonicalPath, item.Path); path != "" {
				roots = append(roots, path)
			}
		}
	case '[':
		var items []entry
		if json.Unmarshal(probe.Workspaces, &items) != nil {
			return nil
		}
		for _, item := range items {
			if path := firstAbs(item.CanonicalPath, item.Path); path != "" {
				roots = append(roots, path)
			}
		}
	}
	return roots
}

func firstAbs(values ...string) string {
	for _, value := range values {
		if filepath.IsAbs(value) {
			return filepath.Clean(value)
		}
	}
	return ""
}
