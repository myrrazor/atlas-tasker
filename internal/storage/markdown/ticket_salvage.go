package markdown

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

// DamagedTicketRepair is the outcome of RestoreDamagedTickets.
type DamagedTicketRepair struct {
	Actions    []string
	Unresolved []storage.DamagedTicketFile
	Backups    map[string]string
}

// RestoreDamagedTickets recovers ticket files that no longer decode (rough
// edge 18: a file cut short mid-frontmatter). Each damaged file is first
// copied byte-for-byte to .tracker/quarantine/<stamp>/ and only then
// rewritten from the newest snapshot of that ticket in the event log. A
// ticket with no snapshot in the log is left exactly as it is and reported,
// so nothing is ever dropped. Callers must hold the workspace write lock.
func RestoreDamagedTickets(ctx context.Context, root string, damaged []storage.DamagedTicketFile, events contracts.EventLog, clock func() time.Time) (DamagedTicketRepair, error) {
	result := DamagedTicketRepair{Backups: map[string]string{}}
	if len(damaged) == 0 {
		return result, nil
	}
	now := time.Now().UTC()
	if clock != nil {
		now = clock().UTC()
	}
	quarantine := filepath.Join(storage.TrackerDir(root), "quarantine", now.Format("20060102T150405Z"))
	latest, err := latestTicketSnapshots(ctx, events)
	if err != nil {
		return result, err
	}
	for _, file := range damaged {
		raw, err := os.ReadFile(file.Path)
		if err != nil {
			return result, fmt.Errorf("read damaged ticket %s: %w", file.Path, err)
		}
		backup := filepath.Join(quarantine, storage.RelativeToRoot(root, file.Path))
		if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
			return result, fmt.Errorf("create quarantine dir: %w", err)
		}
		if err := os.WriteFile(backup, raw, 0o644); err != nil {
			return result, fmt.Errorf("back up damaged ticket %s: %w", file.Path, err)
		}
		relBackup := storage.RelativeToRoot(root, backup)
		result.Backups[file.Path] = relBackup
		snapshot, ok := latest[file.TicketID]
		if !ok || snapshot.Project != file.Project {
			result.Unresolved = append(result.Unresolved, file)
			result.Actions = append(result.Actions, fmt.Sprintf("kept damaged %s: no snapshot in the event log (backup: %s)", file.TicketID, relBackup))
			continue
		}
		content, err := EncodeTicketMarkdown(snapshot)
		if err != nil {
			return result, err
		}
		tmp := file.Path + ".restore.tmp"
		if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
			return result, fmt.Errorf("write restored ticket %s: %w", file.TicketID, err)
		}
		if err := os.Rename(tmp, file.Path); err != nil {
			_ = os.Remove(tmp)
			return result, fmt.Errorf("replace damaged ticket %s: %w", file.TicketID, err)
		}
		result.Actions = append(result.Actions, fmt.Sprintf("restored %s from event log (backup: %s)", file.TicketID, relBackup))
	}
	return result, nil
}

// UnresolvedDamageError explains which damaged tickets repair could not restore.
func UnresolvedDamageError(root string, repair DamagedTicketRepair) error {
	parts := make([]string, 0, len(repair.Unresolved))
	for _, file := range repair.Unresolved {
		parts = append(parts, fmt.Sprintf("%s (%s; backup: %s)", storage.RelativeToRoot(root, file.Path), file.Reason, repair.Backups[file.Path]))
	}
	return apperr.New(apperr.CodeRepairNeeded, fmt.Sprintf("could not restore %d damaged ticket file(s) because the event log has no snapshot of them: %s; the files were left in place, fix or remove them by hand, then rerun 'tracker doctor --repair'", len(parts), strings.Join(parts, ", ")))
}

func latestTicketSnapshots(ctx context.Context, events contracts.EventLog) (map[string]contracts.TicketSnapshot, error) {
	out := map[string]contracts.TicketSnapshot{}
	if events == nil {
		return out, nil
	}
	stream, err := events.StreamEvents(ctx, "", 0)
	if err != nil {
		return nil, err
	}
	for _, event := range stream {
		if event.Payload == nil {
			continue
		}
		raw, err := json.Marshal(event.Payload)
		if err != nil {
			continue
		}
		var wrapped struct {
			Ticket      contracts.TicketSnapshot `json:"ticket"`
			OtherTicket contracts.TicketSnapshot `json:"other_ticket"`
		}
		var direct contracts.TicketSnapshot
		candidates := []contracts.TicketSnapshot{}
		if json.Unmarshal(raw, &wrapped) == nil {
			candidates = append(candidates, wrapped.Ticket, wrapped.OtherTicket)
		}
		if json.Unmarshal(raw, &direct) == nil {
			candidates = append(candidates, direct)
		}
		for _, ticket := range candidates {
			ticket = contracts.NormalizeTicketSnapshot(ticket)
			if ticket.ValidateForCreate() != nil {
				continue
			}
			out[ticket.ID] = ticket // later events win
		}
	}
	return out, nil
}
