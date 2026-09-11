package host

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
	"github.com/myrrazor/atlas-tasker/internal/mcp/testhost"
	toml "github.com/pelletier/go-toml/v2"
)

// MergeJSONServer inserts or replaces one Atlas mcpServers entry and keeps
// every other key. Duplicate keys are refused. Foreign keys on the Atlas
// entry are preserved so ownership checks can see user edits.
func MergeJSONServer(existing []byte, name string, entry adapter.StandardServerEntry) ([]byte, error) {
	doc := map[string]json.RawMessage{}
	servers := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(existing)) > 0 {
		if err := testhost.RejectDuplicateJSONKeys(existing); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(existing, &doc); err != nil {
			return nil, fmt.Errorf("malformed json: %w", err)
		}
		if raw, ok := doc["mcpServers"]; ok {
			if err := testhost.RejectDuplicateJSONKeys(raw); err != nil {
				return nil, err
			}
			if err := json.Unmarshal(raw, &servers); err != nil {
				return nil, fmt.Errorf("mcpServers: %w", err)
			}
		}
	}
	owned := map[string]any{"type": entry.Type, "command": entry.Command, "args": entry.Args}
	body, err := json.Marshal(owned)
	if err != nil {
		return nil, err
	}
	servers[name] = body
	encodedServers, err := json.Marshal(servers)
	if err != nil {
		return nil, err
	}
	doc["mcpServers"] = encodedServers
	return json.MarshalIndent(json.RawMessage(mustObject(doc)), "", "  ")
}

func mustObject(doc map[string]json.RawMessage) []byte {
	out := map[string]any{}
	for key, raw := range doc {
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			out[key] = string(raw)
			continue
		}
		out[key] = value
	}
	encoded, _ := json.Marshal(out)
	return encoded
}

// RemoveJSONServer deletes only the named Atlas-owned entry.
func RemoveJSONServer(existing []byte, name string) ([]byte, error) {
	if len(bytes.TrimSpace(existing)) == 0 {
		return existing, nil
	}
	if err := testhost.RejectDuplicateJSONKeys(existing); err != nil {
		return nil, err
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(existing, &doc); err != nil {
		return nil, fmt.Errorf("malformed json: %w", err)
	}
	raw, ok := doc["mcpServers"]
	if !ok {
		return existing, nil
	}
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(raw, &servers); err != nil {
		return nil, err
	}
	delete(servers, name)
	encoded, err := json.Marshal(servers)
	if err != nil {
		return nil, err
	}
	doc["mcpServers"] = encoded
	return json.MarshalIndent(json.RawMessage(mustObject(doc)), "", "  ")
}

// DecodeAtlasEntry reads one mcpServers entry and reports Atlas ownership.
func DecodeAtlasEntry(raw []byte, reg adapter.MCPRegistration) (adapter.StandardServerEntry, bool, error) {
	var cfg adapter.StandardConfig
	if err := testhost.RejectDuplicateJSONKeys(raw); err != nil {
		return adapter.StandardServerEntry{}, false, err
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return adapter.StandardServerEntry{}, false, err
	}
	entry, ok := cfg.MCPServers[reg.ServerName]
	if !ok {
		return adapter.StandardServerEntry{}, false, nil
	}
	return entry, entry.Matches(reg), nil
}

// MergeTOMLServer inserts or replaces [mcp_servers.<name>] while leaving the
// rest of the file, including comments outside that table, intact.
func MergeTOMLServer(existing []byte, name string, fields []TOMLField) ([]byte, error) {
	if len(bytes.TrimSpace(existing)) > 0 {
		var check map[string]any
		if err := toml.Unmarshal(existing, &check); err != nil {
			return nil, fmt.Errorf("malformed TOML: %w", err)
		}
	}
	table := renderTOMLTable(name, fields)
	return replaceTOMLTable(existing, "mcp_servers."+name, table), nil
}

// TOMLField is one key written into a managed [mcp_servers.<name>] table.
type TOMLField struct {
	Key   string
	Value string
	Array []string
	Bool  *bool
}

func renderTOMLTable(name string, fields []TOMLField) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[mcp_servers.%s]\n", quoteTOMLKey(name))
	for _, field := range fields {
		switch {
		case field.Array != nil:
			fmt.Fprintf(&b, "%s = [", field.Key)
			for i, item := range field.Array {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(quoteTOMLString(item))
			}
			b.WriteString("]\n")
		case field.Bool != nil:
			fmt.Fprintf(&b, "%s = %t\n", field.Key, *field.Bool)
		default:
			fmt.Fprintf(&b, "%s = %s\n", field.Key, quoteTOMLString(field.Value))
		}
	}
	return b.String()
}

func quoteTOMLKey(name string) string {
	if adapterServerNameBare(name) {
		return name
	}
	return quoteTOMLString(name)
}

func adapterServerNameBare(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		ok := r == '_' || r == '-' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if !ok || (i == 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func quoteTOMLString(v string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range v {
		switch r {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func replaceTOMLTable(existing []byte, header string, table string) []byte {
	lines := splitKeep(string(existing))
	start, end := findTOMLTable(lines, header)
	if start < 0 {
		body := strings.TrimRight(string(existing), "\n")
		if body != "" {
			body += "\n\n"
		}
		return []byte(body + strings.TrimRight(table, "\n") + "\n")
	}
	var out strings.Builder
	for i := 0; i < start; i++ {
		out.WriteString(lines[i])
	}
	out.WriteString(strings.TrimRight(table, "\n") + "\n")
	if end < len(lines) && strings.TrimSpace(lines[end]) != "" {
		out.WriteByte('\n')
	}
	for i := end; i < len(lines); i++ {
		out.WriteString(lines[i])
	}
	return []byte(out.String())
}

func findTOMLTable(lines []string, header string) (int, int) {
	want := "[" + header + "]"
	alt := "[\"" + strings.TrimPrefix(header, "mcp_servers.") + "\"]"
	start := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == want || trimmed == "[mcp_servers."+quoteTOMLKey(strings.TrimPrefix(header, "mcp_servers."))+"]" || strings.HasPrefix(trimmed, "[mcp_servers.") && strings.Contains(trimmed, alt) {
			start = i
			break
		}
	}
	if start < 0 {
		return -1, -1
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "[") && !strings.HasPrefix(trimmed, "[mcp_servers."+strings.TrimPrefix(header, "mcp_servers.")+".") {
			end = i
			break
		}
	}
	return start, end
}

func splitKeep(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.SplitAfter(s, "\n")
	return parts
}

// RemoveTOMLServer deletes only the named table.
func RemoveTOMLServer(existing []byte, name string) ([]byte, error) {
	if len(bytes.TrimSpace(existing)) == 0 {
		return existing, nil
	}
	var check map[string]any
	if err := toml.Unmarshal(existing, &check); err != nil {
		return nil, fmt.Errorf("malformed TOML: %w", err)
	}
	lines := splitKeep(string(existing))
	start, end := findTOMLTable(lines, "mcp_servers."+name)
	if start < 0 {
		return existing, nil
	}
	var out strings.Builder
	for i, line := range lines {
		if i >= start && i < end {
			continue
		}
		out.WriteString(line)
	}
	return []byte(out.String()), nil
}

// NativeFingerprint hashes the Atlas-owned native entry bytes.
func NativeFingerprint(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func ReadExisting(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return raw, err
}
