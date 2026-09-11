package testhost

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
	"github.com/myrrazor/atlas-tasker/internal/mcp/selfprobe"
)

// Descriptor is the portable mcpServers document generic setup writes.
type Descriptor struct {
	MCPServers map[string]adapter.StandardServerEntry `json:"mcpServers"`
}

// Load reads a portable or standard mcpServers JSON file.
func Load(path string) (Descriptor, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Descriptor{}, err
	}
	if err := RejectDuplicateJSONKeys(raw); err != nil {
		return Descriptor{}, err
	}
	var desc Descriptor
	if err := json.Unmarshal(raw, &desc); err != nil {
		return Descriptor{}, fmt.Errorf("portable descriptor: %w", err)
	}
	if len(desc.MCPServers) == 0 {
		return Descriptor{}, fmt.Errorf("portable descriptor has no mcpServers")
	}
	return desc, nil
}

// DiscoverNames lists server names in the descriptor.
func (d Descriptor) DiscoverNames() []string {
	out := make([]string, 0, len(d.MCPServers))
	for name := range d.MCPServers {
		out = append(out, name)
	}
	return out
}

// ProbeOptions starts the named Atlas entry from a descriptor.
type ProbeOptions struct {
	DescriptorPath string
	ServerName     string
	Executable     string
	WorkspaceRoot  string
	Timeout        time.Duration
	Mutate         bool
}

// Probe loads the descriptor and self-probes the named (or only) Atlas server.
func Probe(ctx context.Context, opts ProbeOptions) (selfprobe.Report, error) {
	desc, err := Load(opts.DescriptorPath)
	if err != nil {
		return selfprobe.Report{}, err
	}
	name := opts.ServerName
	entry, ok := desc.MCPServers[name]
	if !ok {
		if name == "" && len(desc.MCPServers) == 1 {
			for n, e := range desc.MCPServers {
				name, entry = n, e
				ok = true
				break
			}
		}
	}
	if !ok {
		return selfprobe.Report{}, fmt.Errorf("descriptor has no server %q", opts.ServerName)
	}
	exe := opts.Executable
	if exe == "" {
		exe = entry.Command
	}
	if !filepath.IsAbs(exe) {
		return selfprobe.Report{}, fmt.Errorf("conformance host needs an absolute executable, got %q", exe)
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	cmd := adapter.Command{
		Purpose:    adapter.CommandPurposeProbe,
		Executable: exe,
		Args:       append([]string(nil), entry.Args...),
		Dir:        opts.WorkspaceRoot,
		Timeout:    timeout,
	}
	if err := cmd.Validate(); err != nil {
		return selfprobe.Report{}, err
	}
	return selfprobe.Run(ctx, selfprobe.Options{Command: cmd, Mutate: opts.Mutate})
}

// RejectDuplicateJSONKeys walks a JSON document and refuses objects that
// repeat a key at the same level (AT114-205 / AT114-208 fail-closed).
func RejectDuplicateJSONKeys(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return walkObjectKeys(dec)
}

func walkObjectKeys(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	switch tok {
	case json.Delim('{'):
		seen := map[string]struct{}{}
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return err
			}
			key, ok := keyTok.(string)
			if !ok {
				return fmt.Errorf("json object key is not a string")
			}
			if _, dup := seen[key]; dup {
				return fmt.Errorf("duplicate json key %q", key)
			}
			seen[key] = struct{}{}
			if err := walkObjectKeys(dec); err != nil {
				return err
			}
		}
		end, err := dec.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return fmt.Errorf("expected end of object")
		}
	case json.Delim('['):
		for dec.More() {
			if err := walkObjectKeys(dec); err != nil {
				return err
			}
		}
		end, err := dec.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return fmt.Errorf("expected end of array")
		}
	default:
		// scalar
	}
	return nil
}
