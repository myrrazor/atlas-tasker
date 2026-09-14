package mcp

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"unicode"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

var (
	pemBlockPattern       = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`)
	pemHeaderPattern      = regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`)
	secretReplacePatterns = []*regexp.Regexp{
		regexp.MustCompile(`\bghp_[A-Za-z0-9]{20,}\b`),
		regexp.MustCompile(`\bgho_[A-Za-z0-9]{20,}\b`),
		regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}\b`),
		regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
		regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`),
		regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._\-+=/]{20,}`),
	}
	credentialURLPattern = regexp.MustCompile(`(?i)\b(?:https?|git|ssh|ftp)://[^/\s"'\\]+:[^@\s"'\\]+@[^\s"'\\]+`)
	privatePathInText    = regexp.MustCompile(`(?i)(?:[A-Za-z]:\\|\\\\)[^\s"'\\]+|(?:/(?:Volumes|tmp|Users|home|private|var/folders|opt/homebrew|opt|mnt|media|root)/)[^\s"'\\]+`)
	fileURLPattern       = regexp.MustCompile(`(?i)\bfile://[^\s"'\\]+`)
)

func redactForOutput(payload any, includePaths bool) any {
	return walkRedactJSON(asJSONValue(payload), "", includePaths)
}

func redactLive(payload any, includePaths bool) any {
	return walkRedactLive(payload, "", includePaths)
}

func asJSONValue(payload any) any {
	raw, err := json.Marshal(payload)
	if err != nil {
		return payload
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return payload
	}
	return out
}

func walkRedactJSON(value any, key string, includePaths bool) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for k, v := range typed {
			out[k] = walkRedactJSON(v, k, includePaths)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, v := range typed {
			out[i] = walkRedactJSON(v, key, includePaths)
		}
		return out
	case string:
		return redactStringField(key, typed, includePaths)
	default:
		return value
	}
}

func walkRedactLive(value any, key string, includePaths bool) any {
	if value == nil {
		return nil
	}
	switch typed := value.(type) {
	case string:
		return redactStringField(key, typed, includePaths)
	case map[string]any:
		for k, v := range typed {
			typed[k] = walkRedactLive(v, k, includePaths)
		}
		return typed
	case []any:
		for i, v := range typed {
			typed[i] = walkRedactLive(v, key, includePaths)
		}
		return typed
	case []string:
		for i, v := range typed {
			typed[i] = redactStringField(key, v, includePaths)
		}
		return typed
	}
	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return value
	}
	switch rv.Kind() {
	case reflect.Pointer:
		if rv.IsNil() {
			return value
		}
		elem := rv.Elem()
		switch elem.Kind() {
		case reflect.Struct:
			redactStructValue(elem, includePaths)
			return value
		case reflect.String:
			if elem.CanSet() {
				elem.SetString(redactStringField(key, elem.String(), includePaths))
			}
			return value
		default:
			next := walkRedactLive(elem.Interface(), key, includePaths)
			setReflect(elem, next)
			return value
		}
	case reflect.Struct:
		cp := reflect.New(rv.Type()).Elem()
		cp.Set(rv)
		redactStructValue(cp, includePaths)
		return cp.Interface()
	case reflect.Slice:
		if rv.IsNil() {
			return value
		}
		out := reflect.MakeSlice(rv.Type(), rv.Len(), rv.Len())
		for i := 0; i < rv.Len(); i++ {
			item := rv.Index(i)
			if !item.CanInterface() {
				continue
			}
			setReflect(out.Index(i), walkRedactLive(item.Interface(), key, includePaths))
		}
		return out.Interface()
	case reflect.Map:
		if rv.IsNil() {
			return value
		}
		out := reflect.MakeMap(rv.Type())
		for _, mk := range rv.MapKeys() {
			mv := rv.MapIndex(mk)
			var next any
			if mv.IsValid() && mv.CanInterface() {
				next = walkRedactLive(mv.Interface(), fmt.Sprint(mk.Interface()), includePaths)
			}
			if next == nil {
				out.SetMapIndex(mk, reflect.Zero(rv.Type().Elem()))
				continue
			}
			nv := reflect.ValueOf(next)
			if nv.IsValid() && nv.Type().AssignableTo(rv.Type().Elem()) {
				out.SetMapIndex(mk, nv)
			}
		}
		return out.Interface()
	default:
		return value
	}
}

func setReflect(dst reflect.Value, value any) {
	if !dst.IsValid() || !dst.CanSet() {
		return
	}
	if value == nil {
		dst.Set(reflect.Zero(dst.Type()))
		return
	}
	nv := reflect.ValueOf(value)
	if !nv.IsValid() {
		dst.Set(reflect.Zero(dst.Type()))
		return
	}
	if nv.Type().AssignableTo(dst.Type()) {
		dst.Set(nv)
	}
}

func redactStructValue(val reflect.Value, includePaths bool) {
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return
		}
		redactStructValue(val.Elem(), includePaths)
		return
	}
	if val.Kind() != reflect.Struct {
		return
	}
	t := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue
		}
		name := jsonFieldName(field)
		fv := val.Field(i)
		if !fv.CanSet() {
			continue
		}
		switch fv.Kind() {
		case reflect.String:
			fv.SetString(redactStringField(name, fv.String(), includePaths))
		case reflect.Pointer:
			if fv.IsNil() {
				continue
			}
			walkRedactLive(fv.Interface(), name, includePaths)
		case reflect.Struct, reflect.Slice, reflect.Map, reflect.Interface:
			if fv.Kind() == reflect.Interface && fv.IsNil() {
				continue
			}
			if !fv.CanInterface() {
				continue
			}
			setReflect(fv, walkRedactLive(fv.Interface(), name, includePaths))
		}
	}
}

func jsonFieldName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "" || tag == "-" {
		return field.Name
	}
	name := strings.Split(tag, ",")[0]
	if name == "" {
		return field.Name
	}
	return name
}

func isPathField(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	switch k {
	case "path", "root", "workspace_root", "archive_path", "new_path", "source_path",
		"filepath", "file_path", "dir", "directory", "copy_path":
		return true
	}
	return strings.HasSuffix(k, "_path") || strings.HasSuffix(k, "_root") || strings.HasSuffix(k, "_dir")
}

func isProseField(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	switch k {
	case "title", "description", "summary", "body", "markdown", "name", "display_name",
		"acceptance", "label", "labels", "notes", "next_actions", "attention":
		return true
	}
	return false
}

func isPublicURLField(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	return k == "board_url" || k == "boardurl"
}

func isIdentityField(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	switch k {
	case "plan_id", "plan_digest", "digest", "restore_plan_id", "backup_id",
		"workspace_id", "ticket_id", "event_uid", "schema_hash":
		return true
	}
	return strings.HasSuffix(k, "_digest") || strings.HasSuffix(k, "_hash")
}

func redactStringField(key, value string, includePaths bool) string {
	if value == "" {
		return value
	}
	if isIdentityField(key) {
		return value
	}
	if isPublicURLField(key) {
		return redactSecretsAndCredentials(value)
	}
	if isPathField(key) {
		if includePaths {
			return redactSecretsAndCredentials(value)
		}
		if looksLikeAbsolutePath(value) || looksLikeFileURL(value) || len(service.SecretLikeFindings(value)) > 0 || credentialURLPattern.MatchString(value) {
			return "[redacted-path]"
		}
		return redactSecretsAndCredentials(value)
	}
	if isProseField(key) {
		return redactSecretsAndCredentials(value)
	}
	return redactMixedString(value, includePaths)
}

func redactMixedString(value string, includePaths bool) string {
	out := redactSecretsAndCredentials(value)
	if includePaths {
		return out
	}
	out = fileURLPattern.ReplaceAllString(out, "[redacted-path]")
	out = privatePathInText.ReplaceAllString(out, "[redacted-path]")
	return out
}

func redactSecretsAndCredentials(value string) string {
	out := pemBlockPattern.ReplaceAllString(value, "[redacted]")
	out = pemHeaderPattern.ReplaceAllString(out, "[redacted]")
	for _, re := range secretReplacePatterns {
		out = re.ReplaceAllString(out, "[redacted]")
	}
	out = credentialURLPattern.ReplaceAllString(out, "***")
	return out
}

func looksLikeAbsolutePath(value string) bool {
	v := strings.TrimSpace(value)
	if v == "" {
		return false
	}
	if strings.HasPrefix(v, "/") {
		return true
	}
	if strings.HasPrefix(v, `\\`) {
		return true
	}
	if len(v) >= 3 && unicode.IsLetter(rune(v[0])) && v[1] == ':' && (v[2] == '\\' || v[2] == '/') {
		return true
	}
	return false
}

func looksLikeFileURL(value string) bool {
	return fileURLPattern.MatchString(strings.TrimSpace(value))
}

func redactCallToolError(err error, includePaths bool) (string, any) {
	if err == nil {
		return "", nil
	}
	text := redactMixedString(err.Error(), includePaths)
	return text, redactLive(apperr.Envelope(err)["error"], includePaths)
}
