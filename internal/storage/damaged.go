package storage

import (
	"fmt"
	"strings"
)

// DamagedTicketFile is a ticket markdown file that exists on disk but can't be
// decoded (for example a file cut short mid-frontmatter). Callers keep going
// with the healthy tickets and report these by path instead of failing the
// whole command on the first bad file.
type DamagedTicketFile struct {
	Path     string `json:"path"`
	Project  string `json:"project"`
	TicketID string `json:"ticket_id"`
	Reason   string `json:"reason"`
}

// DamagedTicketsError reports every damaged ticket file found in one pass.
type DamagedTicketsError struct {
	Root  string
	Files []DamagedTicketFile
}

// RepairNeeded maps the error to exit 7 (repair_needed).
func (e *DamagedTicketsError) RepairNeeded() bool { return true }

func (e *DamagedTicketsError) Error() string {
	parts := make([]string, 0, len(e.Files))
	for _, file := range e.Files {
		parts = append(parts, fmt.Sprintf("%s (%s)", RelativeToRoot(e.Root, file.Path), file.Reason))
	}
	noun := "file is"
	if len(e.Files) != 1 {
		noun = "files are"
	}
	return fmt.Sprintf("%d ticket %s damaged: %s; run 'tracker doctor --repair' to back them up and restore them from the event log", len(e.Files), noun, strings.Join(parts, ", "))
}

// RelativeToRoot shows path relative to the workspace root when it lives inside it.
func RelativeToRoot(root, path string) string {
	if root == "" {
		return path
	}
	prefix := strings.TrimSuffix(root, "/") + "/"
	if strings.HasPrefix(path, prefix) {
		return strings.TrimPrefix(path, prefix)
	}
	return path
}
