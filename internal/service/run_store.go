package service

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	"gopkg.in/yaml.v3"
)

type RunStore struct {
	Root string
}

type runFrontmatter struct {
	contracts.RunSnapshot `yaml:",inline"`
}

func (s RunStore) SaveRun(_ context.Context, run contracts.RunSnapshot) error {
	run = normalizeRunSnapshot(run)
	if err := run.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.RunsDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create runs dir: %w", err)
	}
	raw, err := yaml.Marshal(runFrontmatter{RunSnapshot: run})
	if err != nil {
		return fmt.Errorf("marshal run %s: %w", run.RunID, err)
	}
	body := strings.TrimSpace(run.Summary)
	if body == "" {
		body = fmt.Sprintf("Run `%s` for ticket `%s`.", run.RunID, run.TicketID)
	}
	doc := fmt.Sprintf("---\n%s---\n\n%s\n", string(raw), body)
	if err := os.WriteFile(storage.RunFile(s.Root, run.RunID), []byte(doc), 0o644); err != nil {
		return fmt.Errorf("write run %s: %w", run.RunID, err)
	}
	return nil
}

func (s RunStore) LoadRun(_ context.Context, runID string) (contracts.RunSnapshot, error) {
	raw, err := os.ReadFile(storage.RunFile(s.Root, runID))
	if err != nil {
		return contracts.RunSnapshot{}, fmt.Errorf("read run %s: %w", runID, err)
	}
	fmRaw, body, err := splitDocument(string(raw))
	if err != nil {
		return contracts.RunSnapshot{}, err
	}
	var fm runFrontmatter
	if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
		return contracts.RunSnapshot{}, fmt.Errorf("parse run %s: %w", runID, err)
	}
	run := normalizeRunSnapshot(fm.RunSnapshot)
	if strings.TrimSpace(run.Summary) == "" {
		run.Summary = strings.TrimSpace(body)
	}
	return run, nil
}

func (s RunStore) ListRuns(_ context.Context, ticketID string) ([]contracts.RunSnapshot, error) {
	entries, err := os.ReadDir(storage.RunsDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.RunSnapshot{}, nil
		}
		return nil, fmt.Errorf("read runs dir: %w", err)
	}
	items := make([]contracts.RunSnapshot, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		runID := strings.TrimSuffix(entry.Name(), ".md")
		run, err := s.LoadRun(context.Background(), runID)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(ticketID) != "" && run.TicketID != ticketID {
			continue
		}
		items = append(items, run)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].RunID < items[j].RunID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items, nil
}

func (s RunStore) DeleteRun(_ context.Context, runID string) error {
	if err := os.Remove(storage.RunFile(s.Root, runID)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete run %s: %w", runID, err)
	}
	return nil
}

func normalizeRunSnapshot(run contracts.RunSnapshot) contracts.RunSnapshot {
	if run.SchemaVersion == 0 {
		run.SchemaVersion = contracts.CurrentSchemaVersion
	}
	if run.Status == "" {
		run.Status = contracts.RunStatusPlanned
	}
	if run.Kind == "" {
		run.Kind = contracts.RunKindWork
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = timeNowUTC()
	}
	return run
}

func splitDocument(doc string) (string, string, error) {
	normalized := strings.ReplaceAll(doc, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return "", "", fmt.Errorf("missing frontmatter start")
	}
	parts := strings.SplitN(strings.TrimPrefix(normalized, "---\n"), "\n---\n", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("missing frontmatter end")
	}
	return parts[0], parts[1], nil
}

func timeNowUTC() time.Time {
	return time.Now().UTC()
}
