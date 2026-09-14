package mcp

import (
	"fmt"
	"regexp"
	"strings"
)

// Grok qualifies tools as server__name and skips names that still contain a
// dot (atlas.status -> atlas-xxxxxxxx__atlas.status is "invalid or ambiguous").
// Portable names are therefore [A-Za-z][A-Za-z0-9_]* with no "__".
var grokSafeToolName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

// PortableToolName is the Grok-safe form of a canonical Atlas tool name.
func PortableToolName(canonical string) string {
	return strings.ReplaceAll(strings.TrimSpace(canonical), ".", "_")
}

func advertisedName(canonical string, opts Options) string {
	if opts.toolNameStyle() != ToolNameStylePortable {
		return canonical
	}
	return PortableToolName(canonical)
}

func advertisedDescription(spec ToolSpec, opts Options) string {
	if opts.toolNameStyle() != ToolNameStylePortable {
		return spec.Description
	}
	desc := strings.TrimSpace(spec.Description)
	if desc == "" {
		return "Canonical Atlas tool: " + spec.Name + "."
	}
	if strings.Contains(desc, spec.Name) {
		return spec.Description
	}
	return spec.Description + " Canonical Atlas tool: " + spec.Name + "."
}

func grokSafeAdvertisedName(name string) error {
	if strings.Contains(name, ".") {
		return fmt.Errorf("portable MCP tool name %q still contains a dot", name)
	}
	if strings.Contains(name, "__") {
		return fmt.Errorf("portable MCP tool name %q contains '__' and is ambiguous under Grok's server__tool qualifier", name)
	}
	if !grokSafeToolName.MatchString(name) {
		return fmt.Errorf("portable MCP tool name %q is not Grok-safe", name)
	}
	return nil
}

// ValidateAdvertisedToolNames checks portable names are unique and Grok-safe.
func ValidateAdvertisedToolNames(opts Options) error {
	opts = opts.Normalized()
	specs := ToolSpecsFor(opts)
	seen := make(map[string]string, len(specs))
	for _, spec := range specs {
		name := advertisedName(spec.Name, opts)
		if name == "" {
			return fmt.Errorf("MCP tool %q has an empty advertised name", spec.Name)
		}
		if opts.toolNameStyle() == ToolNameStylePortable {
			if err := grokSafeAdvertisedName(name); err != nil {
				return err
			}
		}
		if other, dup := seen[name]; dup {
			return fmt.Errorf("advertised MCP tool name %q collides (%s and %s)", name, other, spec.Name)
		}
		seen[name] = spec.Name
	}
	if len(seen) != len(specs) {
		return fmt.Errorf("advertised MCP tool catalog lost names: %d specs, %d unique", len(specs), len(seen))
	}
	return nil
}
