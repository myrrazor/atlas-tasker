package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/storage"
)

// truncateTicket cuts a ticket file to its first 20 bytes, the same damage
// Asta's acceptance run used for rough edge 18.
func truncateTicket(t *testing.T, project, id string) (string, []byte) {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	path := storage.TicketFile(root, project, id)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	damaged := raw[:20]
	if err := os.WriteFile(path, damaged, 0o644); err != nil {
		t.Fatalf("truncate %s: %v", path, err)
	}
	return path, damaged
}

func execJSON(t *testing.T, args ...string) (int, map[string]any, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	exit := Execute(args, &stdout, &stderr)
	payload := map[string]any{}
	_ = json.Unmarshal(stdout.Bytes(), &payload)
	return exit, payload, stdout.String() + stderr.String()
}

func TestTruncatedTicketIsReportedAndRepairable(t *testing.T) {
	withTempWorkspace(t)
	seedTwoTickets(t)
	path, damaged := truncateTicket(t, "APP", "APP-1")

	// doctor names the damaged file and asks for repair instead of failing
	// with a bare internal error
	exit, _, out := execJSON(t, "doctor", "--json")
	if exit != 7 {
		t.Fatalf("doctor on truncated ticket exited %d, want 7 (repair_needed)\n%s", exit, out)
	}
	if !strings.Contains(out, "APP-1.md") || !strings.Contains(out, "doctor --repair") {
		t.Fatalf("doctor should name the damaged file and the repair command\n%s", out)
	}

	// reindex keeps indexing the healthy ticket and reports the damaged one
	exit, _, out = execJSON(t, "reindex", "--json")
	if exit != 7 {
		t.Fatalf("reindex on truncated ticket exited %d, want 7\n%s", exit, out)
	}
	if !strings.Contains(out, "APP-1.md") {
		t.Fatalf("reindex should name the damaged file\n%s", out)
	}
	var board bytes.Buffer
	if exit := Execute([]string{"board", "--json"}, &board, &bytes.Buffer{}); exit != 0 {
		t.Fatalf("board after reindex exited %d", exit)
	}
	if got := boardTicketCount(t, board.String()); got != 2 {
		t.Fatalf("reindex should keep the healthy ticket and the damaged one's last known state, got %d\n%s", got, board.String())
	}

	// repair backs the damaged bytes up and restores the ticket from the event log
	exit, payload, out := execJSON(t, "doctor", "--repair", "--json")
	if exit != 0 {
		t.Fatalf("doctor --repair exited %d\n%s", exit, out)
	}
	actions, _ := payload["repair_actions"].([]any)
	var backup string
	for _, action := range actions {
		text, _ := action.(string)
		if strings.Contains(text, "APP-1") && strings.Contains(text, "backup:") {
			backup = strings.TrimSpace(text[strings.Index(text, "backup:")+len("backup:"):])
			backup = strings.TrimSuffix(backup, ")")
		}
	}
	if backup == "" {
		t.Fatalf("repair actions should record the restore and the backup path: %v", actions)
	}
	root, _ := os.Getwd()
	saved, err := os.ReadFile(filepath.Join(root, backup))
	if err != nil {
		t.Fatalf("backup %s not readable: %v", backup, err)
	}
	if !bytes.Equal(saved, damaged) {
		t.Fatalf("backup should hold the damaged bytes verbatim, got %q", saved)
	}
	restored, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(restored), "First") {
		t.Fatalf("ticket should be restored with its title, err=%v\n%s", err, restored)
	}
	if exit, _, out := execJSON(t, "doctor", "--json"); exit != 0 {
		t.Fatalf("doctor after repair exited %d\n%s", exit, out)
	}
}

func TestTruncatedTicketWithoutHistoryIsKeptAndReported(t *testing.T) {
	withTempWorkspace(t)
	seedTwoTickets(t)
	root, _ := os.Getwd()
	// a hand-written ticket the event log has never seen
	orphan := storage.TicketFile(root, "APP", "APP-99")
	if err := os.WriteFile(orphan, []byte("---\nid: APP-99\ntitle: hand"), 0o644); err != nil {
		t.Fatal(err)
	}
	exit, _, out := execJSON(t, "doctor", "--repair", "--json")
	if exit != 7 {
		t.Fatalf("repair with no history to restore from exited %d, want 7\n%s", exit, out)
	}
	if !strings.Contains(out, "APP-99.md") || !strings.Contains(out, "backup") {
		t.Fatalf("repair should name the file and the backup it made\n%s", out)
	}
	if raw, _ := os.ReadFile(orphan); string(raw) != "---\nid: APP-99\ntitle: hand" {
		t.Fatalf("a ticket with no history must be left in place, got %q", raw)
	}
}
