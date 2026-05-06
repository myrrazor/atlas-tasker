package contracts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"
)

// CanonicalizeAtlasV1 returns the stable byte form signed by v1.7 security
// envelopes. It intentionally rejects floats and invalid UTF-8 so signed payloads
// do not depend on lossy JSON behavior.
func CanonicalizeAtlasV1(value any) ([]byte, error) {
	normalized, err := normalizeCanonicalValue(reflect.ValueOf(value))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(normalized); err != nil {
		return nil, fmt.Errorf("encode canonical json: %w", err)
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

var timeType = reflect.TypeOf(time.Time{})

func normalizeCanonicalValue(value reflect.Value) (any, error) {
	if !value.IsValid() {
		return nil, nil
	}
	for value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface {
		if value.IsNil() {
			return nil, nil
		}
		value = value.Elem()
	}
	if value.Type() == timeType {
		return value.Interface().(time.Time).UTC().Format(time.RFC3339Nano), nil
	}
	switch value.Kind() {
	case reflect.String:
		raw := value.String()
		if !utf8.ValidString(raw) {
			return nil, fmt.Errorf("canonical string contains invalid utf-8")
		}
		return strings.ReplaceAll(strings.ReplaceAll(raw, "\r\n", "\n"), "\r", "\n"), nil
	case reflect.Bool:
		return value.Bool(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return value.Uint(), nil
	case reflect.Float32, reflect.Float64:
		return nil, fmt.Errorf("canonical payloads must not contain floats")
	case reflect.Slice, reflect.Array:
		items := make([]any, 0, value.Len())
		for i := 0; i < value.Len(); i++ {
			item, err := normalizeCanonicalValue(value.Index(i))
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		return items, nil
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String {
			return nil, fmt.Errorf("canonical maps must use string keys")
		}
		out := make(map[string]any, value.Len())
		iter := value.MapRange()
		for iter.Next() {
			key := iter.Key().String()
			if !utf8.ValidString(key) {
				return nil, fmt.Errorf("canonical map key contains invalid utf-8")
			}
			item, err := normalizeCanonicalValue(iter.Value())
			if err != nil {
				return nil, err
			}
			out[key] = item
		}
		return out, nil
	case reflect.Struct:
		out := make(map[string]any)
		typ := value.Type()
		for i := 0; i < value.NumField(); i++ {
			field := typ.Field(i)
			if field.PkgPath != "" {
				continue
			}
			name, omitEmpty, skip := canonicalJSONFieldName(field)
			if skip {
				continue
			}
			fieldValue := value.Field(i)
			if omitEmpty && isCanonicalEmptyValue(fieldValue) {
				continue
			}
			item, err := normalizeCanonicalValue(fieldValue)
			if err != nil {
				return nil, err
			}
			out[name] = item
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported canonical value kind: %s", value.Kind())
	}
}

func isCanonicalEmptyValue(value reflect.Value) bool {
	if value.IsValid() && value.Type() == timeType {
		return value.Interface().(time.Time).IsZero()
	}
	switch value.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return value.Len() == 0
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.Interface, reflect.Pointer:
		return value.IsZero()
	default:
		return false
	}
}

func canonicalJSONFieldName(field reflect.StructField) (name string, omitEmpty bool, skip bool) {
	if field.Tag.Get("atlasc14n") == "-" {
		return "", false, true
	}
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false, true
	}
	if tag == "" {
		return field.Name, false, false
	}
	parts := strings.Split(tag, ",")
	if parts[0] == "-" {
		return "", false, true
	}
	name = parts[0]
	if name == "" {
		name = field.Name
	}
	for _, part := range parts[1:] {
		if part == "omitempty" {
			omitEmpty = true
		}
	}
	return name, omitEmpty, false
}
