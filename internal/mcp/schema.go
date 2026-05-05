package mcp

import (
	"fmt"
	"math"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
)

func objectSchema(required []string, props map[string]any) map[string]any {
	if props == nil {
		props = map[string]any{}
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             required,
		"properties":           props,
	}
}

func stringProp(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func boolProp(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}

func intProp(description string, minimum int) map[string]any {
	return map[string]any{"type": "integer", "minimum": minimum, "description": description}
}

func commonReadProps() map[string]any {
	return map[string]any{
		"cursor": stringProp("Opaque pagination cursor from a prior Atlas MCP response."),
		"limit":  intProp("Maximum items to return for this call.", 1),
	}
}

func actorReasonProps() map[string]any {
	return map[string]any{
		"actor":  stringProp("Atlas actor performing the mutation, for example human:owner or agent:builder-1."),
		"reason": stringProp("Human-readable reason recorded on the Atlas event."),
	}
}

func highImpactProps(_ string) map[string]any {
	props := actorReasonProps()
	props["operation_approval_id"] = stringProp("One-time operation approval created outside MCP.")
	props["confirm_text"] = stringProp("Typed acknowledgement. Expected format: execute <tool-name> <target>.")
	return props
}

func validateArgs(spec ToolSpec, args map[string]any) error {
	if args == nil {
		args = map[string]any{}
	}
	props, _ := spec.InputSchema["properties"].(map[string]any)
	for key := range args {
		if _, ok := props[key]; !ok {
			return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("unknown argument for %s: %s", spec.Name, key))
		}
	}
	for _, key := range requiredSchemaKeys(spec.InputSchema["required"]) {
		raw, ok := args[key]
		if !ok || raw == nil {
			return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("%s is required", key))
		}
		if text, ok := raw.(string); ok && strings.TrimSpace(text) == "" {
			return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("%s is required", key))
		}
	}
	for key, raw := range args {
		if raw == nil {
			continue
		}
		prop, _ := props[key].(map[string]any)
		kind, _ := prop["type"].(string)
		if err := validateArgType(key, raw, kind); err != nil {
			return err
		}
	}
	return nil
}

func validateArgType(key string, raw any, kind string) error {
	switch kind {
	case "string":
		if _, ok := raw.(string); !ok {
			return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("%s must be a string", key))
		}
	case "boolean":
		if _, ok := raw.(bool); !ok {
			return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("%s must be a boolean", key))
		}
	case "integer":
		if !isIntegerArg(raw) {
			return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("%s must be an integer", key))
		}
	case "array":
		switch value := raw.(type) {
		case []string:
			return nil
		case []any:
			for _, item := range value {
				if _, ok := item.(string); !ok {
					return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("%s items must be strings", key))
				}
			}
		default:
			return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("%s must be an array", key))
		}
	}
	return nil
}

func requiredSchemaKeys(raw any) []string {
	switch value := raw.(type) {
	case []string:
		return value
	case []any:
		keys := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok {
				keys = append(keys, text)
			}
		}
		return keys
	default:
		return nil
	}
}

func isIntegerArg(raw any) bool {
	switch value := raw.(type) {
	case int, int8, int16, int32, int64:
		return true
	case uint, uint8, uint16, uint32, uint64:
		return true
	case float64:
		return math.Trunc(value) == value
	case float32:
		return math.Trunc(float64(value)) == float64(value)
	default:
		return false
	}
}
