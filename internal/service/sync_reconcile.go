package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

type syncApplyResult struct {
	AppliedFiles int
	ConflictIDs  []string
}

type syncConflictError struct {
	ConflictIDs []string
	cause       error
}

func (e *syncConflictError) Error() string {
	if e == nil || e.cause == nil {
		return "sync conflicts detected"
	}
	return e.cause.Error()
}

func (e *syncConflictError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (s *QueryService) ListConflicts(ctx context.Context) ([]contracts.ConflictRecord, error) {
	return s.Conflicts.ListConflicts(ctx)
}

func (s *QueryService) ConflictDetail(ctx context.Context, conflictID string) (ConflictDetailView, error) {
	conflict, err := s.Conflicts.LoadConflict(ctx, conflictID)
	if err != nil {
		return ConflictDetailView{}, err
	}
	return ConflictDetailView{Conflict: conflict, GeneratedAt: s.now()}, nil
}

func (s *ActionService) ResolveConflict(ctx context.Context, conflictID string, resolution contracts.ConflictResolution, actor contracts.Actor, reason string) (ConflictDetailView, error) {
	return withWriteLock(ctx, s.LockManager, "resolve sync conflict", func(ctx context.Context) (ConflictDetailView, error) {
		if !actor.IsValid() {
			return ConflictDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if !resolution.IsValid() {
			return ConflictDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid conflict resolution: %s", resolution))
		}
		conflict, err := s.Conflicts.LoadConflict(ctx, conflictID)
		if err != nil {
			return ConflictDetailView{}, err
		}
		if conflict.Status != contracts.ConflictStatusOpen {
			return ConflictDetailView{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("conflict %s is not open", conflictID))
		}
		resolvedAt := s.now()
		conflict.Status = contracts.ConflictStatusResolved
		conflict.Resolution = resolution
		conflict.ResolvedAt = resolvedAt
		conflict.ResolvedBy = actor
		payload, err := conflictResolvedEventPayload(conflict, resolution)
		if err != nil {
			return ConflictDetailView{}, err
		}
		event, err := s.newEvent(ctx, workspaceEventProject, resolvedAt, actor, reason, contracts.EventConflictResolved, "", payload)
		if err != nil {
			return ConflictDetailView{}, err
		}
		if err := s.commitMutation(ctx, "resolve sync conflict", "sync_conflict", event, func(ctx context.Context) error {
			if err := applyConflictResolution(s.Root, conflict, resolution); err != nil {
				return err
			}
			return s.Conflicts.SaveConflict(ctx, conflict)
		}); err != nil {
			return ConflictDetailView{}, err
		}
		if s.Projection != nil {
			if err := s.Projection.Rebuild(ctx, ""); err != nil {
				return ConflictDetailView{}, err
			}
		}
		return ConflictDetailView{Conflict: conflict, GeneratedAt: s.now()}, nil
	})
}

func (s *ActionService) applyImportedSyncFiles(ctx context.Context, jobID string, files map[string][]byte, actor contracts.Actor, reason string) (syncApplyResult, error) {
	rels := make([]string, 0, len(files))
	for rel := range files {
		if rel == "manifest.json" {
			continue
		}
		rels = append(rels, rel)
	}
	sort.Strings(rels)

	result := syncApplyResult{ConflictIDs: []string{}}
	for _, rel := range rels {
		if !isSyncableRelativePath(rel) {
			return syncApplyResult{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("bundle contains unsupported path %s", rel))
		}
		remoteRaw := files[rel]
		dest := filepath.Join(s.Root, filepath.FromSlash(rel))
		localRaw, localExists, err := readOptionalFile(dest)
		if err != nil {
			return syncApplyResult{}, err
		}
		if !localExists {
			if err := writeCanonicalSyncFile(dest, remoteRaw); err != nil {
				return syncApplyResult{}, err
			}
			result.AppliedFiles++
			continue
		}
		if bytes.Equal(localRaw, remoteRaw) {
			continue
		}
		if strings.HasPrefix(rel, ".tracker/events/") {
			mergedRaw, conflictIDs, err := s.reconcileEventFile(ctx, jobID, rel, localRaw, remoteRaw, actor, reason)
			if err != nil {
				return syncApplyResult{}, err
			}
			if !bytes.Equal(localRaw, mergedRaw) {
				if err := writeCanonicalSyncFile(dest, mergedRaw); err != nil {
					return syncApplyResult{}, err
				}
				result.AppliedFiles++
			}
			result.ConflictIDs = append(result.ConflictIDs, conflictIDs...)
			continue
		}
		conflict, err := s.openFileConflict(ctx, jobID, rel, localRaw, remoteRaw, actor, reason)
		if err != nil {
			return syncApplyResult{}, err
		}
		result.ConflictIDs = append(result.ConflictIDs, conflict.ConflictID)
	}
	result.ConflictIDs = uniqueStrings(result.ConflictIDs)
	return result, nil
}

func (s *ActionService) reconcileEventFile(ctx context.Context, jobID string, rel string, localRaw []byte, remoteRaw []byte, actor contracts.Actor, reason string) ([]byte, []string, error) {
	localEvents, err := parseSyncEventFile(localRaw)
	if err != nil {
		return nil, nil, err
	}
	remoteEvents, err := parseSyncEventFile(remoteRaw)
	if err != nil {
		return nil, nil, err
	}
	merged := make(map[string]contracts.Event, len(localEvents)+len(remoteEvents))
	for uid, event := range localEvents {
		merged[uid] = event
	}
	conflictIDs := make([]string, 0)
	for uid, remoteEvent := range remoteEvents {
		localEvent, ok := localEvents[uid]
		if !ok {
			merged[uid] = remoteEvent
			continue
		}
		if syncEventsEqual(localEvent, remoteEvent) {
			continue
		}
		conflict, err := s.openEventConflict(ctx, jobID, rel, localEvent, remoteEvent, actor, reason)
		if err != nil {
			return nil, nil, err
		}
		conflictIDs = append(conflictIDs, conflict.ConflictID)
		merged[uid] = localEvent
	}
	mergedRaw, err := encodeSyncEventFile(merged)
	if err != nil {
		return nil, nil, err
	}
	return mergedRaw, uniqueStrings(conflictIDs), nil
}

func (s *ActionService) openFileConflict(ctx context.Context, jobID string, rel string, localRaw []byte, remoteRaw []byte, actor contracts.Actor, reason string) (contracts.ConflictRecord, error) {
	entityKind, entityUID, conflictType := conflictIdentityForPath(rel)
	conflictID := "conflict_" + NewOpaqueID()
	localRef, err := writeConflictFileSnapshot(s.Root, conflictID, "local", rel, localRaw)
	if err != nil {
		return contracts.ConflictRecord{}, err
	}
	remoteRef, err := writeConflictFileSnapshot(s.Root, conflictID, "remote", rel, remoteRaw)
	if err != nil {
		return contracts.ConflictRecord{}, err
	}
	conflict := normalizeConflictRecord(contracts.ConflictRecord{
		ConflictID:    conflictID,
		EntityKind:    entityKind,
		EntityUID:     entityUID,
		ConflictType:  conflictType,
		LocalRef:      localRef,
		RemoteRef:     remoteRef,
		Status:        contracts.ConflictStatusOpen,
		OpenedByJob:   jobID,
		OpenedAt:      s.now(),
		SchemaVersion: contracts.CurrentSchemaVersion,
	})
	if err := s.persistConflictOpen(ctx, conflict, actor, reason); err != nil {
		return contracts.ConflictRecord{}, err
	}
	return conflict, nil
}

func (s *ActionService) openEventConflict(ctx context.Context, jobID string, rel string, localEvent contracts.Event, remoteEvent contracts.Event, actor contracts.Actor, reason string) (contracts.ConflictRecord, error) {
	conflictID := "conflict_" + NewOpaqueID()
	localRef, err := writeConflictEventSnapshot(s.Root, conflictID, "local", rel, localEvent)
	if err != nil {
		return contracts.ConflictRecord{}, err
	}
	remoteRef, err := writeConflictEventSnapshot(s.Root, conflictID, "remote", rel, remoteEvent)
	if err != nil {
		return contracts.ConflictRecord{}, err
	}
	conflict := normalizeConflictRecord(contracts.ConflictRecord{
		ConflictID:    conflictID,
		EntityKind:    "event",
		EntityUID:     localEvent.EventUID,
		ConflictType:  contracts.ConflictTypeUIDCollision,
		LocalRef:      localRef,
		RemoteRef:     remoteRef,
		Status:        contracts.ConflictStatusOpen,
		OpenedByJob:   jobID,
		OpenedAt:      s.now(),
		SchemaVersion: contracts.CurrentSchemaVersion,
	})
	if err := s.persistConflictOpen(ctx, conflict, actor, reason); err != nil {
		return contracts.ConflictRecord{}, err
	}
	return conflict, nil
}

func (s *ActionService) persistConflictOpen(ctx context.Context, conflict contracts.ConflictRecord, actor contracts.Actor, reason string) error {
	payload, err := conflictEventPayload(conflict, conflict.LocalRef)
	if err != nil {
		return err
	}
	event, err := s.newEvent(ctx, workspaceEventProject, conflict.OpenedAt, actor, reason, contracts.EventConflictOpened, "", payload)
	if err != nil {
		return err
	}
	return s.commitMutation(ctx, "open sync conflict", "sync_conflict", event, func(ctx context.Context) error {
		return s.Conflicts.SaveConflict(ctx, conflict)
	})
}

func applyConflictResolution(root string, conflict contracts.ConflictRecord, resolution contracts.ConflictResolution) error {
	if conflict.ConflictType == contracts.ConflictTypeUIDCollision {
		return resolveEventConflict(root, conflict, resolution)
	}
	return resolveFileConflict(root, conflict, resolution)
}

func resolveFileConflict(root string, conflict contracts.ConflictRecord, resolution contracts.ConflictResolution) error {
	canonicalRel, err := conflictCanonicalRel(conflict)
	if err != nil {
		return err
	}
	canonicalPath := filepath.Join(root, filepath.FromSlash(canonicalRel))
	currentRaw, currentExists, err := readOptionalFile(canonicalPath)
	if err != nil {
		return err
	}
	localRaw, err := os.ReadFile(conflict.LocalRef)
	if err != nil {
		return fmt.Errorf("read local conflict snapshot %s: %w", conflict.LocalRef, err)
	}
	remoteRaw, err := os.ReadFile(conflict.RemoteRef)
	if err != nil {
		return fmt.Errorf("read remote conflict snapshot %s: %w", conflict.RemoteRef, err)
	}
	if !currentExists || (!bytes.Equal(currentRaw, localRaw) && !bytes.Equal(currentRaw, remoteRaw)) {
		return apperr.New(apperr.CodeConflict, fmt.Sprintf("stale_conflict_resolution: %s", conflict.ConflictID))
	}
	chosenRaw := localRaw
	if resolution == contracts.ConflictResolutionUseRemote {
		chosenRaw = remoteRaw
	}
	return writeCanonicalSyncFile(canonicalPath, chosenRaw)
}

func resolveEventConflict(root string, conflict contracts.ConflictRecord, resolution contracts.ConflictResolution) error {
	canonicalRel, err := eventConflictCanonicalRel(conflict)
	if err != nil {
		return err
	}
	canonicalPath := filepath.Join(root, filepath.FromSlash(canonicalRel))
	currentRaw, currentExists, err := readOptionalFile(canonicalPath)
	if err != nil {
		return err
	}
	if !currentExists {
		return apperr.New(apperr.CodeConflict, fmt.Sprintf("stale_conflict_resolution: %s", conflict.ConflictID))
	}
	currentEvents, err := parseSyncEventFile(currentRaw)
	if err != nil {
		return err
	}
	localEvent, err := readConflictEventSnapshot(conflict.LocalRef)
	if err != nil {
		return err
	}
	remoteEvent, err := readConflictEventSnapshot(conflict.RemoteRef)
	if err != nil {
		return err
	}
	currentEvent, ok := currentEvents[conflict.EntityUID]
	if !ok {
		return apperr.New(apperr.CodeConflict, fmt.Sprintf("stale_conflict_resolution: %s", conflict.ConflictID))
	}
	if !syncEventsEqual(currentEvent, localEvent) && !syncEventsEqual(currentEvent, remoteEvent) {
		return apperr.New(apperr.CodeConflict, fmt.Sprintf("stale_conflict_resolution: %s", conflict.ConflictID))
	}
	chosenEvent := localEvent
	if resolution == contracts.ConflictResolutionUseRemote {
		chosenEvent = remoteEvent
	}
	currentEvents[conflict.EntityUID] = chosenEvent
	mergedRaw, err := encodeSyncEventFile(currentEvents)
	if err != nil {
		return err
	}
	return writeCanonicalSyncFile(canonicalPath, mergedRaw)
}

func conflictIdentityForPath(rel string) (string, string, contracts.ConflictType) {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	switch {
	case strings.HasPrefix(rel, "projects/"):
		parts := strings.Split(rel, "/")
		if len(parts) >= 2 {
			projectKey := parts[1]
			if len(parts) == 3 && parts[2] == "project.md" {
				return "project", contracts.DeterministicUID("project", projectKey), contracts.ConflictTypeScalarDivergence
			}
			if len(parts) == 4 && parts[2] == "tickets" && strings.HasSuffix(parts[3], ".md") {
				ticketID := strings.TrimSuffix(parts[3], ".md")
				return "ticket", contracts.TicketUID(projectKey, ticketID), contracts.ConflictTypeScalarDivergence
			}
		}
	case strings.HasPrefix(rel, ".tracker/collaborators/"):
		return "collaborator", strings.TrimSuffix(filepath.Base(rel), ".md"), contracts.ConflictTypeTrustStateDivergence
	case strings.HasPrefix(rel, ".tracker/memberships/"):
		return "membership", strings.TrimSuffix(filepath.Base(rel), ".md"), contracts.ConflictTypeMembershipDivergence
	case strings.HasPrefix(rel, ".tracker/runs/"):
		runID := strings.TrimSuffix(filepath.Base(rel), ".md")
		return "run", contracts.RunUID(runID), contracts.ConflictTypeRunStateDivergence
	case strings.HasPrefix(rel, ".tracker/gates/"):
		gateID := strings.TrimSuffix(filepath.Base(rel), ".md")
		return "gate", contracts.GateUID(gateID), contracts.ConflictTypeGateDivergence
	case strings.HasPrefix(rel, ".tracker/handoffs/"):
		handoffID := strings.TrimSuffix(filepath.Base(rel), ".md")
		return "handoff", contracts.HandoffUID(handoffID), contracts.ConflictTypeScalarDivergence
	case strings.HasPrefix(rel, ".tracker/evidence/"):
		parts := strings.Split(rel, "/")
		if len(parts) == 4 {
			runID := parts[2]
			evidenceID := strings.TrimSuffix(parts[3], ".md")
			return "evidence", contracts.EvidenceUID(runID, evidenceID), contracts.ConflictTypeScalarDivergence
		}
	case strings.HasPrefix(rel, ".tracker/changes/"):
		changeID := strings.TrimSuffix(filepath.Base(rel), ".md")
		return "change", contracts.ChangeUID(changeID), contracts.ConflictTypeChangeDivergence
	case strings.HasPrefix(rel, ".tracker/checks/"):
		checkID := strings.TrimSuffix(filepath.Base(rel), ".md")
		return "check", contracts.CheckUID(checkID), contracts.ConflictTypeCheckDivergence
	}
	return "document", contracts.DeterministicUID("document", rel), contracts.ConflictTypeScalarDivergence
}

func conflictCanonicalRel(conflict contracts.ConflictRecord) (string, error) {
	for _, ref := range []string{conflict.LocalRef, conflict.RemoteRef} {
		rel, err := conflictSnapshotCanonicalRel(conflict, ref)
		if err == nil {
			return rel, nil
		}
	}
	return "", fmt.Errorf("derive canonical conflict path for %s", conflict.ConflictID)
}

func conflictSnapshotCanonicalRel(conflict contracts.ConflictRecord, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("empty conflict snapshot ref")
	}
	conflictRoot := filepath.Join(storage.SyncConflictsDir(filepath.Dir(filepath.Dir(filepath.Dir(ref)))), conflict.ConflictID)
	_ = conflictRoot
	clean := filepath.Clean(ref)
	for _, marker := range []string{string(filepath.Separator) + "local" + string(filepath.Separator), string(filepath.Separator) + "remote" + string(filepath.Separator)} {
		idx := strings.Index(clean, marker)
		if idx == -1 {
			continue
		}
		rel := clean[idx+len(marker):]
		return filepath.ToSlash(rel), nil
	}
	return "", fmt.Errorf("derive canonical path from %s", ref)
}

func eventConflictCanonicalRel(conflict contracts.ConflictRecord) (string, error) {
	for _, ref := range []string{conflict.LocalRef, conflict.RemoteRef} {
		ref = filepath.ToSlash(strings.TrimSpace(ref))
		idx := strings.Index(ref, ".jsonl/")
		if idx == -1 {
			continue
		}
		return ref[:idx+len(".jsonl")], nil
	}
	return "", fmt.Errorf("derive event conflict path for %s", conflict.ConflictID)
}

func writeConflictFileSnapshot(root string, conflictID string, side string, rel string, raw []byte) (string, error) {
	path := filepath.Join(storage.SyncConflictsDir(root), conflictID, side, filepath.FromSlash(rel))
	if err := writeCanonicalSyncFile(path, raw); err != nil {
		return "", err
	}
	return path, nil
}

func writeConflictEventSnapshot(root string, conflictID string, side string, rel string, event contracts.Event) (string, error) {
	raw, err := json.MarshalIndent(contracts.NormalizeEvent(event), "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal conflict event snapshot: %w", err)
	}
	path := filepath.Join(storage.SyncConflictsDir(root), conflictID, side, filepath.FromSlash(rel), event.EventUID+".json")
	if err := writeCanonicalSyncFile(path, append(raw, '\n')); err != nil {
		return "", err
	}
	return path, nil
}

func readConflictEventSnapshot(path string) (contracts.Event, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return contracts.Event{}, fmt.Errorf("read conflict event snapshot %s: %w", path, err)
	}
	var event contracts.Event
	if err := json.Unmarshal(raw, &event); err != nil {
		return contracts.Event{}, fmt.Errorf("decode conflict event snapshot %s: %w", path, err)
	}
	event = contracts.NormalizeEvent(event)
	return event, event.Validate()
}

func readOptionalFile(path string) ([]byte, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}
	return raw, true, nil
}

func writeCanonicalSyncFile(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create dir for %s: %w", path, err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func parseSyncEventFile(raw []byte) (map[string]contracts.Event, error) {
	items := map[string]contracts.Event{}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var event contracts.Event
		if err := json.Unmarshal([]byte(text), &event); err != nil {
			return nil, fmt.Errorf("decode sync event line %d: %w", line, err)
		}
		event = contracts.NormalizeEvent(event)
		if err := event.Validate(); err != nil {
			return nil, fmt.Errorf("invalid sync event line %d: %w", line, err)
		}
		existing, ok := items[event.EventUID]
		if ok && !syncEventsEqual(existing, event) {
			return nil, apperr.New(apperr.CodeConflict, fmt.Sprintf("uid_collision: %s", event.EventUID))
		}
		items[event.EventUID] = event
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan sync event file: %w", err)
	}
	return items, nil
}

func encodeSyncEventFile(items map[string]contracts.Event) ([]byte, error) {
	events := make([]contracts.Event, 0, len(items))
	for _, event := range items {
		events = append(events, contracts.NormalizeEvent(event))
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].LogicalClock != events[j].LogicalClock {
			return events[i].LogicalClock < events[j].LogicalClock
		}
		if !events[i].Timestamp.Equal(events[j].Timestamp) {
			return events[i].Timestamp.Before(events[j].Timestamp)
		}
		if events[i].OriginWorkspaceID != events[j].OriginWorkspaceID {
			return events[i].OriginWorkspaceID < events[j].OriginWorkspaceID
		}
		return events[i].EventUID < events[j].EventUID
	})
	var buf bytes.Buffer
	for _, event := range events {
		raw, err := json.Marshal(event)
		if err != nil {
			return nil, fmt.Errorf("marshal sync event %s: %w", event.EventUID, err)
		}
		buf.Write(raw)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

func syncEventsEqual(left contracts.Event, right contracts.Event) bool {
	left = contracts.NormalizeEvent(left)
	right = contracts.NormalizeEvent(right)
	leftRaw, leftErr := json.Marshal(left)
	rightRaw, rightErr := json.Marshal(right)
	if leftErr != nil || rightErr != nil {
		return left.EventUID == right.EventUID && left.EventID == right.EventID && left.Timestamp.Equal(right.Timestamp)
	}
	return bytes.Equal(leftRaw, rightRaw)
}

func saveSyncJobOnly(ctx context.Context, store contracts.SyncJobStore, job contracts.SyncJob) error {
	job = normalizeSyncJob(job)
	return store.SaveSyncJob(ctx, job)
}

func buildSyncConflictError(conflictIDs []string) error {
	return &syncConflictError{
		ConflictIDs: uniqueStrings(conflictIDs),
		cause:       apperr.New(apperr.CodeConflict, "sync conflicts detected; run tracker conflict list"),
	}
}

func conflictIDsFromError(err error) ([]string, bool) {
	var conflictErr *syncConflictError
	if !errors.As(err, &conflictErr) {
		return nil, false
	}
	return append([]string{}, conflictErr.ConflictIDs...), true
}

func conflictResolvedEventPayload(conflict contracts.ConflictRecord, resolution contracts.ConflictResolution) (map[string]any, error) {
	chosenRef := conflict.LocalRef
	if resolution == contracts.ConflictResolutionUseRemote {
		chosenRef = conflict.RemoteRef
	}
	return conflictEventPayload(conflict, chosenRef)
}

func conflictEventPayload(conflict contracts.ConflictRecord, snapshotRef string) (map[string]any, error) {
	payload := map[string]any{"conflict": conflict}
	if conflict.ConflictType == contracts.ConflictTypeUIDCollision {
		return payload, nil
	}
	if conflict.EntityKind != "ticket" {
		return payload, nil
	}
	raw, err := os.ReadFile(snapshotRef)
	if err != nil {
		return nil, fmt.Errorf("read ticket conflict snapshot %s: %w", snapshotRef, err)
	}
	ticket, err := decodeTicketConflictSnapshot(string(raw))
	if err != nil {
		return nil, fmt.Errorf("decode ticket conflict snapshot %s: %w", snapshotRef, err)
	}
	payload["ticket"] = ticket
	return payload, nil
}
