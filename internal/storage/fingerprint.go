package storage

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SourceFingerprint is a cheap summary of the canonical sources: how many
// lines the event log has and how many ticket files exist. It is deliberately
// count-based, not content-based. A rebuild replays every event and then
// inserts whichever tickets the projection is missing, so count drift is
// exactly the drift a rebuild is guaranteed to clear — and every AppendEvent
// grows one file by one line, so a projection that missed a write always
// shows up here.
type SourceFingerprint struct {
	EventLines  int
	TicketFiles int
}

func (f SourceFingerprint) String() string {
	return fmt.Sprintf("events=%d tickets=%d", f.EventLines, f.TicketFiles)
}

func (f SourceFingerprint) IsZero() bool {
	return f.EventLines == 0 && f.TicketFiles == 0
}

// ComputeSourceFingerprint counts newline bytes across the *.jsonl files
// directly under the events dir and the ticket files under projects/*/tickets.
// No JSON parsing on purpose: this runs on every workspace open and a
// malformed line is doctor's problem, not ours. It does read every event
// file end to end though, so the cost grows with the log — DEC-050 names
// that as the trigger for switching to a (size, mtime) stat instead.
func ComputeSourceFingerprint(root string) (SourceFingerprint, error) {
	var fp SourceFingerprint
	entries, err := os.ReadDir(EventsDir(root))
	if err != nil && !os.IsNotExist(err) {
		return SourceFingerprint{}, fmt.Errorf("read events dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(EventsDir(root), entry.Name()))
		if err != nil {
			return SourceFingerprint{}, fmt.Errorf("read event file %s: %w", entry.Name(), err)
		}
		fp.EventLines += bytes.Count(raw, []byte{'\n'})
	}
	tickets, err := filepath.Glob(filepath.Join(ProjectsDir(root), "*", "tickets", "*.md"))
	if err != nil {
		return SourceFingerprint{}, fmt.Errorf("glob ticket files: %w", err)
	}
	fp.TicketFiles = len(tickets)
	return fp, nil
}
