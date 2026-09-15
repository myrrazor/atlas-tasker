package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestObjectSchemaNeverMarshalsRequiredNull(t *testing.T) {
	schema := objectSchema(nil, map[string]any{"cursor": stringProp("optional")})
	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"required":null`) {
		t.Fatalf("nil required marshaled as JSON null: %s", raw)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	required, ok := decoded["required"].([]any)
	if !ok {
		t.Fatalf("required must be a JSON array, got %T %s", decoded["required"], raw)
	}
	if len(required) != 0 {
		t.Fatalf("optional schema must keep an empty required array, got %v", required)
	}
}

func TestEmittedToolSchemasRejectNullRequiredAndKeepRequiredFields(t *testing.T) {
	profiles := []ToolProfile{ProfileRead, ProfileWorkflow, ProfileDelivery, ProfileAdmin}
	for _, global := range []bool{false, true} {
		for _, profile := range profiles {
			opts := Options{Profile: profile, Global: global, AllowHighImpactTools: true}.Normalized()
			seen := 0
			for _, spec := range ToolSpecsFor(opts) {
				enabled, _ := spec.Enabled(opts)
				if !enabled {
					continue
				}
				seen++
				assertSchemaJSON(t, spec.Name, spec.InputSchema)
			}
			if seen == 0 {
				t.Fatalf("no enabled tools for profile=%s global=%v", profile, global)
			}
		}
	}
}

func TestRequiredSchemaKeysAreEnforced(t *testing.T) {
	server := NewServer(nil, Options{Profile: ProfileAdmin, Global: true, AllowHighImpactTools: true}.Normalized())
	checked := 0
	for _, spec := range ToolSpecsFor(server.Options) {
		enabled, _ := spec.Enabled(server.Options)
		if !enabled {
			continue
		}
		keys := requiredSchemaKeys(spec.InputSchema["required"])
		if len(keys) == 0 {
			continue
		}
		props, _ := spec.InputSchema["properties"].(map[string]any)
		for _, missing := range keys {
			args := map[string]any{}
			for _, key := range keys {
				if key == missing {
					continue
				}
				args[key] = dummySchemaValue(props[key])
			}
			_, err := server.CallTool(context.Background(), spec.Name, args)
			if err == nil || !strings.Contains(err.Error(), missing+" is required") {
				t.Fatalf("%s omitted %s: %v", spec.Name, missing, err)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no required-field cases ran")
	}
}

func assertSchemaJSON(t *testing.T, name string, schema map[string]any) {
	t.Helper()
	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("%s: marshal schema: %v", name, err)
	}
	if strings.Contains(string(raw), `"required":null`) {
		t.Fatalf("%s: required marshaled as null: %s", name, raw)
	}
	if strings.Contains(string(raw), `"properties":null`) {
		t.Fatalf("%s: properties marshaled as null: %s", name, raw)
	}
	if strings.Contains(string(raw), `"additionalProperties":null`) {
		t.Fatalf("%s: additionalProperties marshaled as null: %s", name, raw)
	}
	if strings.Contains(string(raw), `"items":null`) {
		t.Fatalf("%s: items marshaled as null: %s", name, raw)
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("%s: schema is not JSON: %v", name, err)
	}
	assertSchemaNode(t, name, "", decoded)
}

func assertSchemaNode(t *testing.T, tool, path string, node any) {
	t.Helper()
	obj, ok := node.(map[string]any)
	if !ok {
		return
	}
	if raw, exists := obj["required"]; exists {
		arr, ok := raw.([]any)
		if !ok {
			t.Fatalf("%s%s: required must be a JSON array, got %T", tool, path, raw)
		}
		props, _ := obj["properties"].(map[string]any)
		for _, item := range arr {
			key, ok := item.(string)
			if !ok || strings.TrimSpace(key) == "" {
				t.Fatalf("%s%s: required entries must be strings, got %T", tool, path, item)
			}
			if props != nil {
				if _, found := props[key]; !found {
					t.Fatalf("%s%s: required %q is not in properties", tool, path, key)
				}
			}
		}
	}
	if raw, exists := obj["properties"]; exists && raw != nil {
		props, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("%s%s: properties must be an object, got %T", tool, path, raw)
		}
		for name, child := range props {
			assertSchemaNode(t, tool, path+".properties."+name, child)
		}
	}
	if raw, exists := obj["items"]; exists && raw != nil {
		assertSchemaNode(t, tool, path+".items", raw)
	}
	if raw, exists := obj["additionalProperties"]; exists && raw != nil {
		switch raw.(type) {
		case bool, map[string]any:
		default:
			t.Fatalf("%s%s: additionalProperties must be bool or object, got %T", tool, path, raw)
		}
		assertSchemaNode(t, tool, path+".additionalProperties", raw)
	}
}

func dummySchemaValue(prop any) any {
	obj, _ := prop.(map[string]any)
	switch fmt.Sprint(obj["type"]) {
	case "boolean":
		return true
	case "integer":
		return 1
	case "array":
		return []any{"x"}
	case "object":
		return map[string]any{"k": "v"}
	default:
		return "x"
	}
}
