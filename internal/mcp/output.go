package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type pageResult struct {
	Items      any    `json:"items"`
	Total      int    `json:"total"`
	NextCursor string `json:"next_cursor,omitempty"`
	Truncated  bool   `json:"truncated,omitempty"`
}

func paginateSlice(items any, args map[string]any, defaultLimit int, maxItems int) pageResult {
	total := sliceLen(items)
	if total == 0 {
		return pageResult{Items: items, Total: 0}
	}
	offset := intArg(args, "cursor", 0)
	limit := intArg(args, "limit", defaultLimit)
	if limit <= 0 {
		limit = defaultLimit
	}
	if maxItems > 0 && limit > maxItems {
		limit = maxItems
	}
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	sliced := reflect.ValueOf(items).Slice(offset, end).Interface()
	next := ""
	if end < total {
		next = strconv.Itoa(end)
	}
	return pageResult{Items: sliced, Total: total, NextCursor: next, Truncated: next != ""}
}

func applyResultLimits(kind string, generatedAt time.Time, payload any, opts Options) (map[string]any, bool, error) {
	result := toolResult(kind, generatedAt, payload)
	if opts.MaxResultBytes <= 0 {
		return result, false, nil
	}
	writer := &limitWriter{Limit: opts.MaxResultBytes}
	if err := json.NewEncoder(writer).Encode(result); err != nil {
		return nil, false, err
	}
	if !writer.Truncated {
		return result, false, nil
	}
	limited := toolResult(kind, generatedAt, map[string]any{
		"truncated":        true,
		"original_bytes":   writer.BytesSeen,
		"max_result_bytes": opts.MaxResultBytes,
		"hint":             "Use filters, cursor, limit, or a narrower Atlas MCP tool call.",
	})
	return limited, true, nil
}

type limitWriter struct {
	Limit     int
	BytesSeen int
	Truncated bool
	buf       bytes.Buffer
}

func (w *limitWriter) Write(p []byte) (int, error) {
	w.BytesSeen += len(p)
	if w.Limit <= 0 {
		return len(p), nil
	}
	remaining := w.Limit - w.buf.Len()
	if remaining > 0 {
		if len(p) <= remaining {
			_, _ = w.buf.Write(p)
		} else {
			_, _ = w.buf.Write(p[:remaining])
			w.Truncated = true
		}
	} else if len(p) > 0 {
		w.Truncated = true
	}
	return len(p), nil
}

func textFallback(kind string, payload any, truncated bool, maxTokensEstimate int) string {
	raw, err := json.Marshal(payload)
	if err != nil {
		return kind
	}
	text := string(raw)
	maxChars := 1200
	if maxTokensEstimate > 0 {
		maxChars = maxTokensEstimate * 4
	}
	if maxChars < 200 {
		maxChars = 200
	}
	if len(text) > maxChars {
		text = text[:maxChars] + "..."
	}
	if truncated {
		return fmt.Sprintf("%s returned a truncated result: %s", kind, text)
	}
	return fmt.Sprintf("%s returned: %s", kind, text)
}

func sliceLen(items any) int {
	if items == nil {
		return 0
	}
	value := reflect.ValueOf(items)
	if value.Kind() != reflect.Slice && value.Kind() != reflect.Array {
		return 0
	}
	return value.Len()
}

func stringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	raw, ok := args[key]
	if !ok || raw == nil {
		return ""
	}
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func boolArg(args map[string]any, key string) bool {
	raw, ok := args[key]
	if !ok || raw == nil {
		return false
	}
	switch value := raw.(type) {
	case bool:
		return value
	case string:
		parsed, _ := strconv.ParseBool(strings.TrimSpace(value))
		return parsed
	default:
		return false
	}
}

func intArg(args map[string]any, key string, fallback int) int {
	raw, ok := args[key]
	if !ok || raw == nil {
		return fallback
	}
	switch value := raw.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			return parsed
		}
	}
	return fallback
}
