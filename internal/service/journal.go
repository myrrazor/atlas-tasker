package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

type MutationStage string

const (
	MutationStagePrepared          MutationStage = "prepared"
	MutationStageCanonicalWritten  MutationStage = "canonical_written"
	MutationStageEventAppended     MutationStage = "event_appended"
	MutationStageProjectionApplied MutationStage = "projection_applied"
)

type MutationJournalEntry struct {
	ID            string          `json:"id"`
	Purpose       string          `json:"purpose"`
	CanonicalKind string          `json:"canonical_kind"`
	Event         contracts.Event `json:"event"`
	Stage         MutationStage   `json:"stage"`
	StartedAt     time.Time       `json:"started_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	LastError     string          `json:"last_error,omitempty"`
}

type MutationJournal struct {
	Root  string
	Clock func() time.Time
}

type RepairReport struct {
	Pending int
	Actions []string
}

func (j MutationJournal) now() time.Time {
	if j.Clock != nil {
		return j.Clock().UTC()
	}
	return time.Now().UTC()
}

func (j MutationJournal) Begin(purpose string, canonicalKind string, event contracts.Event) (MutationJournalEntry, error) {
	if err := event.Validate(); err != nil {
		return MutationJournalEntry{}, err
	}
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		purpose = "mutation"
	}
	canonicalKind = strings.TrimSpace(canonicalKind)
	if canonicalKind == "" {
		canonicalKind = "unknown"
	}
	id := journalID(event)
	// An entry already on disk is a write that died between the canonical file
	// and the event log. Since that event never landed, NextEventID hands the
	// same id to whoever writes next, and overwriting here would erase the
	// only record doctor has of the half-applied mutation.
	if _, err := os.Stat(j.entryPath(id)); err == nil {
		return MutationJournalEntry{}, apperr.New(apperr.CodeRepairNeeded, fmt.Sprintf("mutation journal %s is still pending from an earlier write; run 'tracker doctor --repair' before writing again", id))
	} else if !os.IsNotExist(err) {
		return MutationJournalEntry{}, fmt.Errorf("check mutation journal %s: %w", id, err)
	}
	entry := MutationJournalEntry{
		ID:            id,
		Purpose:       purpose,
		CanonicalKind: canonicalKind,
		Event:         event,
		Stage:         MutationStagePrepared,
		StartedAt:     j.now(),
		UpdatedAt:     j.now(),
	}
	if err := j.writeEntry(entry); err != nil {
		return MutationJournalEntry{}, err
	}
	return entry, nil
}

func (j MutationJournal) Mark(entry MutationJournalEntry, stage MutationStage, errText string) (MutationJournalEntry, error) {
	entry.Stage = stage
	entry.UpdatedAt = j.now()
	entry.LastError = strings.TrimSpace(errText)
	if err := j.writeEntry(entry); err != nil {
		return MutationJournalEntry{}, err
	}
	return entry, nil
}

func (j MutationJournal) Complete(id string) error {
	if strings.TrimSpace(id) == "" {
		return nil
	}
	if err := os.Remove(j.entryPath(id)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove mutation journal %s: %w", id, err)
	}
	return nil
}

func (j MutationJournal) List() ([]MutationJournalEntry, error) {
	entries, err := os.ReadDir(storage.MutationsDir(j.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []MutationJournalEntry{}, nil
		}
		return nil, fmt.Errorf("read mutation journal dir: %w", err)
	}
	out := make([]MutationJournalEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(storage.MutationsDir(j.Root), entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read mutation journal %s: %w", entry.Name(), err)
		}
		var item MutationJournalEntry
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, fmt.Errorf("decode mutation journal %s: %w", entry.Name(), err)
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, k int) bool {
		if out[i].StartedAt.Equal(out[k].StartedAt) {
			return out[i].ID < out[k].ID
		}
		return out[i].StartedAt.Before(out[k].StartedAt)
	})
	return out, nil
}

func (j MutationJournal) Reconcile(ctx context.Context, events contracts.EventLog, projection contracts.ProjectionStore) ([]string, error) {
	ctx = WithHistoricalReplay(ctx)
	entries, err := j.List()
	if err != nil {
		return nil, err
	}
	actions := make([]string, 0)
	if len(entries) == 0 {
		return actions, nil
	}
	for _, entry := range entries {
		state, err := journalEventState(ctx, events, entry.Event)
		if err != nil {
			return nil, err
		}
		if state == journalEventCollides {
			// The event never landed, but its id is taken in the log (rough
			// edge 19: a restore swapped in a log that already reached it).
			// Keep the original entry in quarantine and replay the same event
			// above the current max id instead of failing or skipping it.
			backup, err := j.quarantineEntry(entry)
			if err != nil {
				return nil, err
			}
			nextID, nextClock, err := nextEventPosition(ctx, j.Root, events, entry.Event.Project)
			if err != nil {
				return nil, err
			}
			oldID := entry.Event.EventID
			entry.Event = renumberEvent(entry.Event, nextID, max(nextClock, entry.Event.LogicalClock))
			if err := events.AppendEvent(ctx, entry.Event); err != nil {
				_, _ = j.Mark(entry, entry.Stage, err.Error())
				return nil, apperr.Wrap(apperr.CodeRepairNeeded, err, "reconcile renumbered mutation event")
			}
			entry, err = j.Mark(entry, MutationStageEventAppended, "")
			if err != nil {
				return nil, apperr.Wrap(apperr.CodeRepairNeeded, err, "mark reconciled event append")
			}
			actions = append(actions, fmt.Sprintf("renumbered %s #%d to #%d and replayed it (journal backup: %s)", entry.Event.Project, oldID, nextID, backup))
			continue
		}
		if state == journalEventMissing {
			if err := events.AppendEvent(ctx, entry.Event); err != nil {
				_, _ = j.Mark(entry, entry.Stage, err.Error())
				return nil, apperr.Wrap(apperr.CodeRepairNeeded, err, "reconcile pending mutation event")
			}
			entry, err = j.Mark(entry, MutationStageEventAppended, "")
			if err != nil {
				return nil, apperr.Wrap(apperr.CodeRepairNeeded, err, "mark reconciled event append")
			}
			actions = append(actions, fmt.Sprintf("replayed %s #%d", entry.Event.Project, entry.Event.EventID))
		}
	}
	if projection != nil {
		if err := projection.Rebuild(ctx, ""); err != nil {
			return nil, apperr.Wrap(apperr.CodeRepairNeeded, err, "rebuild projection during repair")
		}
		actions = append(actions, "rebuilt projection")
	}
	for _, entry := range entries {
		if err := j.Complete(entry.ID); err != nil {
			return nil, err
		}
	}
	return actions, nil
}

func RepairWorkspace(ctx context.Context, root string, clock func() time.Time, events contracts.EventLog, projection contracts.ProjectionStore) (RepairReport, error) {
	ctx = WithHistoricalReplay(ctx)
	journal := MutationJournal{Root: root, Clock: clock}
	entries, err := journal.List()
	if err != nil {
		return RepairReport{}, err
	}
	actions, err := journal.Reconcile(ctx, events, projection)
	if err != nil {
		return RepairReport{}, err
	}
	if len(entries) == 0 && projection != nil {
		if err := projection.Rebuild(ctx, ""); err != nil {
			return RepairReport{}, apperr.Wrap(apperr.CodeRepairNeeded, err, "rebuild projection during repair")
		}
		actions = append(actions, "rebuilt projection")
	}
	return RepairReport{Pending: len(entries), Actions: actions}, nil
}

type journalEventPresence int

const (
	journalEventMissing journalEventPresence = iota
	journalEventLanded
	journalEventCollides
)

// journalEventState says whether a journaled event is in the log. An event is
// only "landed" if the event with its id is the same event (same uid); a
// different event at that id, or a log that is already past it, is a
// collision that a plain append would reject.
func journalEventState(ctx context.Context, log contracts.EventLog, event contracts.Event) (journalEventPresence, error) {
	all, err := log.StreamEvents(ctx, event.Project, 0)
	if err != nil {
		return journalEventMissing, err
	}
	var maxID int64
	for _, existing := range all {
		if existing.Project != event.Project {
			continue
		}
		if event.EventUID != "" && existing.EventUID == event.EventUID {
			return journalEventLanded, nil
		}
		if existing.EventID == event.EventID && (event.EventUID == "" || existing.EventUID == "") {
			return journalEventLanded, nil
		}
		if existing.EventID > maxID {
			maxID = existing.EventID
		}
	}
	if event.EventID <= maxID {
		return journalEventCollides, nil
	}
	return journalEventMissing, nil
}

func nextEventPosition(ctx context.Context, root string, log contracts.EventLog, project string) (int64, int64, error) {
	all, err := log.StreamEvents(ctx, "", 0)
	if err != nil {
		return 0, 0, err
	}
	mark, err := loadEventWatermark(root)
	if err != nil {
		return 0, 0, err
	}
	maxID, maxClock := mark.Projects[project], mark.LogicalClock
	for _, existing := range all {
		if existing.Project == project && existing.EventID > maxID {
			maxID = existing.EventID
		}
		clock := existing.LogicalClock
		if clock == 0 {
			clock = existing.EventID
		}
		if clock > maxClock {
			maxClock = clock
		}
	}
	return maxID + 1, maxClock + 1, nil
}

// quarantineEntry copies a journal entry to .tracker/quarantine/<time>/mutations/
// before repair changes it, and returns the copy's workspace-relative path.
func (j MutationJournal) quarantineEntry(entry MutationJournalEntry) (string, error) {
	raw, err := os.ReadFile(j.entryPath(entry.ID))
	if err != nil {
		return "", fmt.Errorf("read mutation journal %s: %w", entry.ID, err)
	}
	dir := filepath.Join(storage.TrackerDir(j.Root), "quarantine", j.now().Format("20060102T150405Z"), "mutations")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create quarantine dir: %w", err)
	}
	backup := filepath.Join(dir, entry.ID)
	if err := os.WriteFile(backup, raw, 0o644); err != nil {
		return "", fmt.Errorf("back up mutation journal %s: %w", entry.ID, err)
	}
	return storage.RelativeToRoot(j.Root, backup), nil
}

// Rekey moves a pending entry to the journal id of its reassigned event.
func (j MutationJournal) Rekey(entry MutationJournalEntry, event contracts.Event) (MutationJournalEntry, error) {
	oldID := entry.ID
	entry.Event = event
	entry.ID = journalID(event)
	entry.UpdatedAt = j.now()
	if entry.ID == oldID {
		return entry, j.writeEntry(entry)
	}
	if _, err := os.Stat(j.entryPath(entry.ID)); err == nil {
		return MutationJournalEntry{}, apperr.New(apperr.CodeRepairNeeded, fmt.Sprintf("mutation journal %s is still pending from an earlier write; run 'tracker doctor --repair'", entry.ID))
	}
	if err := j.writeEntry(entry); err != nil {
		return MutationJournalEntry{}, err
	}
	if err := j.Complete(oldID); err != nil {
		return MutationJournalEntry{}, err
	}
	return entry, nil
}

func eventExists(ctx context.Context, log contracts.EventLog, project string, eventID int64) (bool, error) {
	events, err := log.StreamEvents(ctx, project, maxInt64(0, eventID-1))
	if err != nil {
		return false, err
	}
	for _, event := range events {
		if event.Project == project && event.EventID == eventID {
			return true, nil
		}
	}
	return false, nil
}

func journalID(event contracts.Event) string {
	project := strings.TrimSpace(strings.ToLower(event.Project))
	if project == "" {
		project = "workspace"
	}
	return fmt.Sprintf("%s-%012d.json", project, event.EventID)
}

func (j MutationJournal) entryPath(id string) string {
	return filepath.Join(storage.MutationsDir(j.Root), id)
}

func (j MutationJournal) writeEntry(entry MutationJournalEntry) error {
	dir := storage.MutationsDir(j.Root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create mutation journal dir: %w", err)
	}
	raw, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal mutation journal: %w", err)
	}
	if err := os.WriteFile(j.entryPath(entry.ID), append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("write mutation journal %s: %w", entry.ID, err)
	}
	return nil
}

func maxInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
