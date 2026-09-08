package web

import (
	"embed"
	"html/template"
	"io/fs"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

//go:embed templates static
var embeddedFiles embed.FS

func parseTemplates() (*template.Template, error) {
	funcs := template.FuncMap{
		"actorInitials": actorInitials,
		"formValue":     formValue,
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
	if after, ok := strings.CutPrefix(raw, "agent:"); ok {
		raw = after
	}
	if after, ok := strings.CutPrefix(raw, "human:"); ok {
		raw = after
	}
	// a bare "agent:" / "human:" leaves nothing to abbreviate
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "--"
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '-' || r == '_' || r == ':' || r == '.'
	})
	initials := make([]rune, 0, 2)
	for _, part := range parts {
		r, _ := utf8.DecodeRuneInString(part)
		if r == utf8.RuneError {
			continue
		}
		initials = append(initials, unicode.ToUpper(r))
		if len(initials) >= 2 {
			break
		}
	}
	if len(initials) == 0 {
		r, _ := utf8.DecodeRuneInString(raw)
		if r == utf8.RuneError {
			return "--"
		}
		return string(unicode.ToUpper(r))
	}
	return string(initials)
}

// formValue echoes what the user submitted on a rejected form, falling back
// to the stored value on a fresh render. A submitted-but-empty value wins
// over the fallback: the user cleared that field on purpose.
func formValue(form url.Values, key string, fallback string) string {
	if form == nil {
		return fallback
	}
	if _, ok := form[key]; ok {
		return form.Get(key)
	}
	return fallback
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.Local().Format("Jan 2, 15:04")
}
