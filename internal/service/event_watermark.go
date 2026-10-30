package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

// eventWatermark is the highest event id (per project) and logical clock this
// workspace has ever issued. A backup restore replaces the event log with an
// older one; without the watermark, new events would reuse ids of discarded
// events that sync peers may already hold (PR #185 retest M1). The file lives
// outside every backup and sync path, so a restore never rolls it back.
type eventWatermark struct {
	Format       string           `json:"format"`
	Projects     map[string]int64 `json:"projects"`
	LogicalClock int64            `json:"logical_clock"`
}

const eventWatermarkFormat = "atlas_event_watermark_v1"

func eventWatermarkPath(root string) string {
	return filepath.Join(storage.TrackerDir(root), "event-watermark.json")
}

func loadEventWatermark(root string) (eventWatermark, error) {
	mark := eventWatermark{Format: eventWatermarkFormat, Projects: map[string]int64{}}
	if root == "" {
		return mark, nil
	}
	raw, err := os.ReadFile(eventWatermarkPath(root))
	if errors.Is(err, os.ErrNotExist) {
		return mark, nil
	}
	if err != nil {
		return mark, fmt.Errorf("read event watermark: %w", err)
	}
	if err := json.Unmarshal(raw, &mark); err != nil {
		return eventWatermark{Format: eventWatermarkFormat, Projects: map[string]int64{}}, fmt.Errorf("decode event watermark: %w", err)
	}
	if mark.Projects == nil {
		mark.Projects = map[string]int64{}
	}
	mark.Format = eventWatermarkFormat
	return mark, nil
}

// raiseEventWatermark records every id and clock in log into the watermark.
// It runs before a restore swaps the log, so the ids about to be discarded
// are never issued again.
func raiseEventWatermark(ctx context.Context, root string, log contracts.EventLog) error {
	mark, err := loadEventWatermark(root)
	if err != nil {
		return err
	}
	all, err := log.StreamEvents(ctx, "", 0)
	if err != nil {
		return err
	}
	changed := false
	for _, event := range all {
		if event.EventID > mark.Projects[event.Project] {
			mark.Projects[event.Project] = event.EventID
			changed = true
		}
		clock := event.LogicalClock
		if clock == 0 {
			clock = event.EventID
		}
		if clock > mark.LogicalClock {
			mark.LogicalClock = clock
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return atomicWriteJSON(eventWatermarkPath(root), mark)
}

// EventIDReuse is one (project, id) that its origin workspace issued for more
// than one event. Peers number events independently, so the same id from two
// origins is normal; the same id twice from one origin means ids were reused
// (for example by a restore before the watermark existed) and peers that saw
// both events cannot tell them apart by id.
type EventIDReuse struct {
	Project           string   `json:"project"`
	EventID           int64    `json:"event_id"`
	OriginWorkspaceID string   `json:"origin_workspace_id"`
	EventUIDs         []string `json:"event_uids"`
}

// DetectEventIDReuse reports every (project, id, origin) with more than one
// distinct event_uid, in log order.
func DetectEventIDReuse(events []contracts.Event) []EventIDReuse {
	type key struct {
		project string
		id      int64
		origin  string
	}
	out := []EventIDReuse{}
	seen := map[key][]string{}
	order := []key{}
	for _, event := range events {
		k := key{event.Project, event.EventID, event.OriginWorkspaceID}
		uids, ok := seen[k]
		if !ok {
			order = append(order, k)
		}
		if event.EventUID != "" && !containsString(uids, event.EventUID) {
			seen[k] = append(uids, event.EventUID)
		} else if !ok {
			seen[k] = uids
		}
	}
	for _, k := range order {
		if len(seen[k]) > 1 {
			out = append(out, EventIDReuse{Project: k.project, EventID: k.id, OriginWorkspaceID: k.origin, EventUIDs: seen[k]})
		}
	}
	return out
}
