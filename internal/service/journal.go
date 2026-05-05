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
	entry := MutationJournalEntry{
		ID:            journalID(event),
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
		exists, err := eventExists(ctx, events, entry.Event.Project, entry.Event.EventID)
		if err != nil {
			return nil, err
		}
		if !exists {
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
