package uninstall

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter/host"
	toml "github.com/pelletier/go-toml/v2"
)

type jsonMCPFile struct {
	MCPServers map[string]jsonMCPEntry `json:"mcpServers"`
}

type jsonMCPEntry struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Type    string   `json:"type"`
}

func stripMarkedBlock(body, begin, end string) (string, bool, error) {
	if !allowedMarkerPair(begin, end) {
		return body, false, apperr.New(apperr.CodeConflict, "refusing unknown managed-block markers")
	}
	start := strings.Index(body, begin)
	if start < 0 {
		return body, false, nil
	}
	stop := strings.Index(body[start:], end)
	if stop < 0 {
		return body, false, apperr.New(apperr.CodeConflict, "managed block is missing its end marker")
	}
	stop = start + stop + len(end)
	return body[:start] + body[stop:], true, nil
}

func atlasJSONEntry(raw []byte, key, binary string, args []string) (bool, error) {
	var doc jsonMCPFile
	if err := json.Unmarshal(raw, &doc); err != nil {
		return false, apperr.New(apperr.CodeConflict, "client JSON is not a mcpServers document")
	}
	entry, ok := doc.MCPServers[key]
	if !ok {
		return false, nil
	}
	if entry.Command != binary {
		return false, apperr.New(apperr.CodeConflict, "mcpServers entry command does not match the install receipt")
	}
	if !stringSliceEqual(entry.Args, args) {
		return false, apperr.New(apperr.CodeConflict, "mcpServers entry args do not match the recorded Atlas registration")
	}
	return true, nil
}

func removeJSONEntry(path, key, binary string, args []string) (bool, error) {
	if err := requireOwnedRegularPath(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	owned, err := atlasJSONEntry(raw, key, binary, args)
	if err != nil || !owned {
		return false, err
	}
	next, err := host.RemoveJSONServer(raw, key)
	if err != nil {
		return false, err
	}
	if err := writeReplacingRegularFile(path, append(trimNL(next), '\n')); err != nil {
		return false, err
	}
	return true, nil
}

func atlasTOMLEntry(raw []byte, key, binary string, args []string) (bool, error) {
	var doc map[string]any
	if err := toml.Unmarshal(raw, &doc); err != nil {
		return false, apperr.New(apperr.CodeConflict, "client TOML is malformed")
	}
	servers, _ := doc["mcp_servers"].(map[string]any)
	if servers == nil {
		return false, nil
	}
	entry, _ := servers[key].(map[string]any)
	if entry == nil {
		return false, nil
	}
	command, _ := entry["command"].(string)
	if command != binary {
		return false, apperr.New(apperr.CodeConflict, "mcp_servers entry command does not match the install receipt")
	}
	if !stringSliceEqual(tomlStringSlice(entry["args"]), args) {
		return false, apperr.New(apperr.CodeConflict, "mcp_servers entry args do not match the recorded Atlas registration")
	}
	return true, nil
}

func removeTOMLEntry(path, key, binary string, args []string) (bool, error) {
	if err := requireOwnedRegularPath(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	owned, err := atlasTOMLEntry(raw, key, binary, args)
	if err != nil || !owned {
		return false, err
	}
	next, err := host.RemoveTOMLServer(raw, key)
	if err != nil {
		return false, err
	}
	if err := writeReplacingRegularFile(path, next); err != nil {
		return false, err
	}
	return true, nil
}

func removeMarkdownBlock(path, marker string) (bool, error) {
	if err := requireOwnedRegularPath(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	begin := markerBegin(marker)
	end := markerEnd(begin)
	next, changed, err := stripMarkedBlock(string(raw), begin, end)
	if err != nil || !changed {
		return false, err
	}
	if err := writeReplacingRegularFile(path, []byte(next)); err != nil {
		return false, err
	}
	return true, nil
}

func writeReplacingRegularFile(path string, data []byte) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return apperr.New(apperr.CodeConflict, "refusing to write through a symlink")
	}
	if info.IsDir() {
		return apperr.New(apperr.CodeConflict, "refusing to overwrite a directory")
	}
	tmp := path + ".atlas-uninstall-tmp"
	if err := os.WriteFile(tmp, data, info.Mode().Perm()); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func tomlStringSlice(v any) []string {
	switch items := v.(type) {
	case []any:
		out := make([]string, 0, len(items))
		for _, item := range items {
			s, _ := item.(string)
			out = append(out, s)
		}
		return out
	case []string:
		return items
	default:
		return nil
	}
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func trimNL(raw []byte) []byte {
	return []byte(strings.TrimRight(string(raw), "\n"))
}
