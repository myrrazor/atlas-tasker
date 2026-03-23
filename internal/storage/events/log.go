package events

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

// Log is an append-only JSONL event store.
type Log struct {
	RootDir string
	mu      sync.Mutex
}

func (l *Log) AppendEvent(_ context.Context, event contracts.Event) error {
	if err := event.Validate(); err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	eventsDir := storage.EventsDir(l.RootDir)
	if err := os.MkdirAll(eventsDir, 0o755); err != nil {
		return fmt.Errorf("create events dir: %w", err)
	}
	filePath := filepath.Join(eventsDir, event.Timestamp.UTC().Format("2006-01")+".jsonl")
	lastID, err := l.maxEventIDForProject(eventsDir, event.Project)
	if err != nil {
		return fmt.Errorf("read last event id: %w", err)
	}
	if event.EventID <= lastID {
		return fmt.Errorf("event_id %d must be greater than %d", event.EventID, lastID)
	}

	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open event file: %w", err)
	}
	defer file.Close()

	raw, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	if _, err := file.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	return nil
}

func (l *Log) StreamEvents(_ context.Context, project string, afterEventID int64) ([]contracts.Event, error) {
	eventsDir := storage.EventsDir(l.RootDir)
	entries, err := os.ReadDir(eventsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.Event{}, nil
		}
		return nil, fmt.Errorf("read events dir: %w", err)
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		files = append(files, filepath.Join(eventsDir, entry.Name()))
	}
	sort.Strings(files)

	events := make([]contracts.Event, 0)
	for _, filePath := range files {
		fileEvents, err := readEventFile(filePath)
		if err != nil {
			return nil, err
		}
		for _, event := range fileEvents {
			if project != "" && event.Project != project {
				continue
			}
			if event.EventID <= afterEventID {
				continue
			}
			events = append(events, event)
		}
	}

	sort.Slice(events, func(i, j int) bool {
		if events[i].EventID == events[j].EventID {
			return events[i].Timestamp.Before(events[j].Timestamp)
		}
		return events[i].EventID < events[j].EventID
	})

	return events, nil
}

func readEventFile(path string) ([]contracts.Event, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open event file %s: %w", path, err)
	}
	defer file.Close()

	events := make([]contracts.Event, 0)
	scanner := bufio.NewScanner(file)
	line := 0
	for scanner.Scan() {
		line++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		var event contracts.Event
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			return nil, fmt.Errorf("decode event line %d in %s: %w", line, path, err)
		}
		if err := event.Validate(); err != nil {
			return nil, fmt.Errorf("invalid event line %d in %s: %w", line, path, err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan event file %s: %w", path, err)
	}
	return events, nil
}

func MonthFileName(t time.Time) string {
	return t.UTC().Format("2006-01") + ".jsonl"
}

func (l *Log) maxEventIDForProject(eventsDir string, project string) (int64, error) {
	entries, err := os.ReadDir(eventsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		files = append(files, filepath.Join(eventsDir, entry.Name()))
	}
	sort.Strings(files)
	var lastID int64
	for _, file := range files {
		events, err := readEventFile(file)
		if err != nil {
			return 0, err
		}
		for _, event := range events {
			if project != "" && event.Project != project {
				continue
			}
			if event.EventID > lastID {
				lastID = event.EventID
			}
		}
	}
	return lastID, nil
}
