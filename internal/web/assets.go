package web

import (
	"embed"
	"html/template"
	"io/fs"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

//go:embed templates static
var embeddedFiles embed.FS

func parseTemplates() (*template.Template, error) {
	funcs := template.FuncMap{
		"actorInitials": actorInitials,
		"formatTime":    formatTime,
		"join":          strings.Join,
		"priorityDot":   priorityDot,
		"statusKey":     statusKey,
		"statusLabel":   statusLabel,
	}
	return template.New("atlas-web").Funcs(funcs).ParseFS(embeddedFiles, "templates/*.html")
}

func staticFS() (fs.FS, error) {
	return fs.Sub(embeddedFiles, "static")
}

func statusKey(status contracts.Status) string {
	return strings.ReplaceAll(string(status), "_", "-")
}

func statusLabel(status contracts.Status) string {
	switch status {
	case contracts.StatusBacklog:
		return "Backlog"
	case contracts.StatusReady:
		return "Ready"
	case contracts.StatusInProgress:
		return "In Progress"
	case contracts.StatusInReview:
		return "In Review"
	case contracts.StatusBlocked:
		return "Blocked"
	case contracts.StatusDone:
		return "Done"
	case contracts.StatusCanceled:
		return "Canceled"
	default:
		return strings.TrimSpace(string(status))
	}
}

func priorityDot(priority contracts.Priority) string {
	switch priority {
	case contracts.PriorityCritical:
		return "critical"
	case contracts.PriorityHigh:
		return "high"
	case contracts.PriorityLow:
		return "low"
	default:
		return "medium"
	}
}

func actorInitials(actor contracts.Actor) string {
	raw := strings.TrimSpace(string(actor))
	if raw == "" {
		return "--"
	}
	if after, ok := strings.CutPrefix(raw, "agent:"); ok {
		raw = after
	}
	if after, ok := strings.CutPrefix(raw, "human:"); ok {
		raw = after
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '-' || r == '_' || r == ':' || r == '.'
	})
	out := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		out += strings.ToUpper(part[:1])
		if len(out) >= 2 {
			break
		}
	}
	if out == "" {
		return strings.ToUpper(raw[:1])
	}
	return out
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.Local().Format("Jan 2, 15:04")
}
