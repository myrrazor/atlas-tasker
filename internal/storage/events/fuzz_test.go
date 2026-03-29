package events

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzReadEventFile(f *testing.F) {
	for _, seed := range []string{
		"{\"event_id\":1,\"timestamp\":\"2026-03-24T00:00:00Z\",\"actor\":\"human:owner\",\"type\":\"ticket.created\",\"project\":\"APP\",\"ticket_id\":\"APP-1\",\"schema_version\":2}\n",
		"not-json\n",
		"",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		path := filepath.Join(t.TempDir(), "events.jsonl")
		if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
			t.Fatalf("write event file: %v", err)
		}
		_, _ = readEventFile(path)
	})
}
