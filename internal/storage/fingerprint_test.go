package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestComputeSourceFingerprintCountsLinesAndTicketFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(EventsDir(root), "2026-08.jsonl"), "{\"a\":1}\n{\"a\":2}\n{\"a\":3}\n")
	writeFile(t, filepath.Join(EventsDir(root), "2026-09.jsonl"), "{\"a\":4}\n{\"a\":5}\n")
	// nested dirs and non-jsonl files are not the log
	writeFile(t, filepath.Join(EventsDir(root), "notes.txt"), "one\ntwo\n")
	writeFile(t, filepath.Join(EventsDir(root), "nested", "2026-01.jsonl"), "{\"a\":9}\n")
	writeFile(t, TicketFile(root, "APP", "APP-1"), "---\nid: APP-1\n---\n")
	writeFile(t, TicketFile(root, "APP", "APP-2"), "---\nid: APP-2\n---\n")
	writeFile(t, TicketFile(root, "OPS", "OPS-1"), "---\nid: OPS-1\n---\n")
	// project.md sits next to tickets/, not in it
	writeFile(t, ProjectFile(root, "APP"), "---\nkey: APP\n---\n")

	fp, err := ComputeSourceFingerprint(root)
	if err != nil {
		t.Fatalf("compute fingerprint: %v", err)
	}
	if fp.EventLines != 5 || fp.TicketFiles != 3 {
		t.Fatalf("unexpected fingerprint: %+v", fp)
	}
	if got := fp.String(); got != "events=5 tickets=3" {
		t.Fatalf("unexpected fingerprint string: %q", got)
	}
	if fp.IsZero() {
		t.Fatal("non-empty workspace must not read as zero")
	}
}

func TestComputeSourceFingerprintDoesNotParseJSON(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(EventsDir(root), "2026-09.jsonl"), "{\"ok\":true}\nthis is not json at all\n")

	fp, err := ComputeSourceFingerprint(root)
	if err != nil {
		t.Fatalf("a malformed line must still count, got error: %v", err)
	}
	if fp.EventLines != 2 {
		t.Fatalf("expected 2 lines regardless of content, got %+v", fp)
	}
}

func TestComputeSourceFingerprintTreatsMissingWorkspaceAsZero(t *testing.T) {
	root := t.TempDir()

	fp, err := ComputeSourceFingerprint(root)
	if err != nil {
		t.Fatalf("missing events dir should be zero, not an error: %v", err)
	}
	if !fp.IsZero() {
		t.Fatalf("expected zero fingerprint, got %+v", fp)
	}
	if got := fp.String(); got != "events=0 tickets=0" {
		t.Fatalf("unexpected zero string: %q", got)
	}
}
