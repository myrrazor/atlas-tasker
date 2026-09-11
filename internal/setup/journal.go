package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
)

const journalFormat = "atlas_setup_journal_v1"

// JournalTargetKind distinguishes provider transactions from the managed-mode
// and backup groups.
type JournalTargetKind string

const (
	JournalKindProvider    JournalTargetKind = "provider"
	JournalKindManagedMode JournalTargetKind = "managed_mode"
	JournalKindBackup      JournalTargetKind = "backup"
)

type journalStepRecord struct {
	StepID       string                  `json:"step_id"`
	Kind         adapter.StepKind        `json:"kind"`
	Path         string                  `json:"path,omitempty"`
	Started      bool                    `json:"started"`
	Done         bool                    `json:"done"`
	RollbackDone bool                    `json:"rollback_done"`
	Created      bool                    `json:"created"`
	Before       *adapter.FileIdentity   `json:"before,omitempty"`
	After        *adapter.FileIdentity   `json:"after,omitempty"`
	Rollback     *adapter.RollbackAction `json:"rollback,omitempty"`
	Error        string                  `json:"error,omitempty"`
}

// JournalEntry is one setup operation. It lives only in the private state
// directory and never includes file contents or secrets.
type JournalEntry struct {
	Format          string              `json:"format"`
	OperationID     string              `json:"operation_id"`
	Kind            JournalTargetKind   `json:"kind"`
	Target          integrations.Target `json:"target,omitempty"`
	WorkspaceID     string              `json:"workspace_id"`
	WorkspaceRoot   string              `json:"workspace_root"`
	PlanFingerprint string              `json:"plan_fingerprint"`
	State           OperationState      `json:"state"`
	Integration     adapter.State       `json:"integration_state,omitempty"`
	Steps           []journalStepRecord `json:"steps"`
	LeftBehind      []string            `json:"left_behind,omitempty"`
	CreatedAt       time.Time           `json:"created_at"`
	UpdatedAt       time.Time           `json:"updated_at"`
}

func journalPath(stateDir, operationID string) string {
	return filepath.Join(journalsDir(stateDir), operationID+".json")
}

func writeJournal(stateDir string, entry *JournalEntry) error {
	if err := ensurePrivateDir(journalsDir(stateDir)); err != nil {
		return err
	}
	entry.Format = journalFormat
	entry.UpdatedAt = time.Now().UTC()
	raw, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("encode setup journal: %w", err)
	}
	return atomicWriteFile(journalPath(stateDir, entry.OperationID), append(raw, '\n'), filePerm)
}

func readJournal(stateDir, operationID string) (*JournalEntry, error) {
	raw, err := os.ReadFile(journalPath(stateDir, operationID))
	if err != nil {
		return nil, err
	}
	var entry JournalEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return nil, fmt.Errorf("decode setup journal: %w", err)
	}
	return &entry, nil
}

func listJournals(stateDir string) ([]*JournalEntry, error) {
	entries, err := os.ReadDir(journalsDir(stateDir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*JournalEntry
	for _, item := range entries {
		if item.IsDir() || filepath.Ext(item.Name()) != ".json" {
			continue
		}
		entry, err := readJournal(stateDir, item.Name()[:len(item.Name())-len(".json")])
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func deleteJournal(stateDir, operationID string) error {
	_ = os.RemoveAll(rollbackDir(stateDir, operationID))
	err := os.Remove(journalPath(stateDir, operationID))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func markState(entry *JournalEntry, next OperationState) error {
	moved, err := entry.State.Transition(next)
	if err != nil {
		return err
	}
	entry.State = moved
	return nil
}
