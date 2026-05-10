package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

type BackupSnapshotStore struct {
	Root string
}

type RestorePlanStore struct {
	Root string
}

type GoalManifestStore struct {
	Root string
}

func (s BackupSnapshotStore) SaveBackupSnapshot(_ context.Context, snapshot contracts.BackupSnapshot) error {
	snapshot = normalizeBackupSnapshot(snapshot)
	if err := snapshot.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.BackupManifestsDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create backup manifests dir: %w", err)
	}
	raw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("encode backup %s: %w", snapshot.BackupID, err)
	}
	return os.WriteFile(backupSnapshotRecordPath(s.Root, snapshot.BackupID), append(raw, '\n'), 0o644)
}

func (s BackupSnapshotStore) LoadBackupSnapshot(_ context.Context, backupID string) (contracts.BackupSnapshot, error) {
	raw, err := os.ReadFile(backupSnapshotRecordPath(s.Root, backupID))
	if err != nil {
		return contracts.BackupSnapshot{}, fmt.Errorf("read backup %s: %w", backupID, err)
	}
	var snapshot contracts.BackupSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return contracts.BackupSnapshot{}, fmt.Errorf("decode backup %s: %w", backupID, err)
	}
	snapshot = normalizeBackupSnapshot(snapshot)
	return snapshot, snapshot.Validate()
}

func (s BackupSnapshotStore) ListBackupSnapshots(ctx context.Context) ([]contracts.BackupSnapshot, error) {
	entries, err := os.ReadDir(storage.BackupManifestsDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.BackupSnapshot{}, nil
		}
		return nil, fmt.Errorf("read backup manifests dir: %w", err)
	}
	items := make([]contracts.BackupSnapshot, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || strings.HasSuffix(entry.Name(), ".manifest.json") {
			continue
		}
		item, err := s.LoadBackupSnapshot(ctx, strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].BackupID < items[j].BackupID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items, nil
}

func (s RestorePlanStore) SaveRestorePlan(_ context.Context, plan contracts.RestorePlan) error {
	plan = normalizeRestorePlan(plan)
	if err := plan.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(restorePlansDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create restore plans dir: %w", err)
	}
	raw, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("encode restore plan %s: %w", plan.RestorePlanID, err)
	}
	return os.WriteFile(restorePlanPath(s.Root, plan.RestorePlanID), append(raw, '\n'), 0o644)
}

func (s RestorePlanStore) LoadRestorePlan(_ context.Context, planID string) (contracts.RestorePlan, error) {
	raw, err := os.ReadFile(restorePlanPath(s.Root, planID))
	if err != nil {
		return contracts.RestorePlan{}, fmt.Errorf("read restore plan %s: %w", planID, err)
	}
	var plan contracts.RestorePlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return contracts.RestorePlan{}, fmt.Errorf("decode restore plan %s: %w", planID, err)
	}
	plan = normalizeRestorePlan(plan)
	return plan, plan.Validate()
}

func (s RestorePlanStore) ListRestorePlans(ctx context.Context) ([]contracts.RestorePlan, error) {
	entries, err := os.ReadDir(restorePlansDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.RestorePlan{}, nil
		}
		return nil, fmt.Errorf("read restore plans dir: %w", err)
	}
	items := make([]contracts.RestorePlan, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		plan, err := s.LoadRestorePlan(ctx, strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		items = append(items, plan)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].GeneratedAt.Equal(items[j].GeneratedAt) {
			return items[i].RestorePlanID < items[j].RestorePlanID
		}
		return items[i].GeneratedAt.Before(items[j].GeneratedAt)
	})
	return items, nil
}

func (s GoalManifestStore) SaveGoalManifest(_ context.Context, manifest contracts.GoalManifest) error {
	manifest = normalizeGoalManifest(manifest)
	if err := manifest.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.GoalManifestsDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create goal manifests dir: %w", err)
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode goal manifest %s: %w", manifest.ManifestID, err)
	}
	return os.WriteFile(goalManifestPath(s.Root, manifest.ManifestID), append(raw, '\n'), 0o644)
}

func (s GoalManifestStore) LoadGoalManifest(_ context.Context, manifestID string) (contracts.GoalManifest, error) {
	raw, err := os.ReadFile(goalManifestPath(s.Root, manifestID))
	if err != nil {
		return contracts.GoalManifest{}, fmt.Errorf("read goal manifest %s: %w", manifestID, err)
	}
	return decodeGoalManifest(raw, manifestID)
}

func (s GoalManifestStore) ListGoalManifests(ctx context.Context) ([]contracts.GoalManifest, error) {
	entries, err := os.ReadDir(storage.GoalManifestsDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.GoalManifest{}, nil
		}
		return nil, fmt.Errorf("read goal manifests dir: %w", err)
	}
	items := make([]contracts.GoalManifest, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		item, err := s.LoadGoalManifest(ctx, strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].GeneratedAt.Equal(items[j].GeneratedAt) {
			return items[i].ManifestID < items[j].ManifestID
		}
		return items[i].GeneratedAt.Before(items[j].GeneratedAt)
	})
	return items, nil
}

func decodeGoalManifest(raw []byte, fallbackID string) (contracts.GoalManifest, error) {
	var manifest contracts.GoalManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return contracts.GoalManifest{}, fmt.Errorf("decode goal manifest: %w", err)
	}
	if strings.TrimSpace(manifest.ManifestID) == "" {
		manifest.ManifestID = strings.TrimSuffix(filepath.Base(fallbackID), ".json")
	}
	manifest = normalizeGoalManifest(manifest)
	return manifest, manifest.Validate()
}

func normalizeBackupSnapshot(snapshot contracts.BackupSnapshot) contracts.BackupSnapshot {
	snapshot.BackupID = sanitizeSecurityID(firstNonEmpty(snapshot.BackupID, "backup"))
	snapshot.ScopeID = strings.TrimSpace(snapshot.ScopeID)
	snapshot.ManifestHash = strings.TrimSpace(snapshot.ManifestHash)
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = timeNowUTC()
	}
	if snapshot.ScopeKind == "" {
		snapshot.ScopeKind = contracts.BackupScopeWorkspace
	}
	if snapshot.SchemaVersion == 0 {
		snapshot.SchemaVersion = contracts.CurrentSchemaVersion
	}
	snapshot.IncludedSections = normalizeBackupSections(snapshot.IncludedSections)
	snapshot.SignatureEnvelopes = normalizeStoredSignatureEnvelopes(snapshot.SignatureEnvelopes)
	return snapshot
}

func normalizeRestorePlan(plan contracts.RestorePlan) contracts.RestorePlan {
	plan.RestorePlanID = sanitizeSecurityID(firstNonEmpty(plan.RestorePlanID, "restore-plan"))
	plan.BackupID = strings.TrimSpace(plan.BackupID)
	if plan.GeneratedAt.IsZero() {
		plan.GeneratedAt = timeNowUTC()
	}
	if plan.SchemaVersion == 0 {
		plan.SchemaVersion = contracts.CurrentSchemaVersion
	}
	for i := range plan.Items {
		plan.Items[i].Path = filepath.ToSlash(strings.TrimSpace(plan.Items[i].Path))
	}
	sort.Slice(plan.Items, func(i, j int) bool {
		if plan.Items[i].Action != plan.Items[j].Action {
			return plan.Items[i].Action < plan.Items[j].Action
		}
		return plan.Items[i].Path < plan.Items[j].Path
	})
	sort.Strings(plan.Warnings)
	return plan
}

func normalizeGoalManifest(manifest contracts.GoalManifest) contracts.GoalManifest {
	manifest.ManifestID = sanitizeSecurityID(firstNonEmpty(manifest.ManifestID, "goal"))
	manifest.TargetID = strings.TrimSpace(manifest.TargetID)
	manifest.Objective = strings.TrimSpace(manifest.Objective)
	manifest.SourceHash = strings.TrimSpace(manifest.SourceHash)
	manifest.Reason = strings.TrimSpace(manifest.Reason)
	if manifest.GeneratedAt.IsZero() {
		manifest.GeneratedAt = timeNowUTC()
	}
	if manifest.SchemaVersion == 0 {
		manifest.SchemaVersion = contracts.CurrentSchemaVersion
	}
	manifest.SignatureEnvelopes = normalizeStoredSignatureEnvelopes(manifest.SignatureEnvelopes)
	return manifest
}

func normalizeBackupSections(sections []contracts.BackupSection) []contracts.BackupSection {
	if len(sections) == 0 {
		return []contracts.BackupSection{}
	}
	seen := map[contracts.BackupSection]struct{}{}
	out := make([]contracts.BackupSection, 0, len(sections))
	for _, section := range sections {
		if !section.IsValid() {
			out = append(out, section)
			continue
		}
		if _, ok := seen[section]; ok {
			continue
		}
		seen[section] = struct{}{}
		out = append(out, section)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func normalizeStoredSignatureEnvelopes(items []contracts.SignatureEnvelope) []contracts.SignatureEnvelope {
	if len(items) == 0 {
		return []contracts.SignatureEnvelope{}
	}
	out := append([]contracts.SignatureEnvelope{}, items...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].SignedAt.Equal(out[j].SignedAt) {
			return out[i].SignatureID < out[j].SignatureID
		}
		return out[i].SignedAt.Before(out[j].SignedAt)
	})
	return out
}

func backupSnapshotRecordPath(root string, backupID string) string {
	return filepath.Join(storage.BackupManifestsDir(root), sanitizeSecurityID(backupID)+".json")
}

func backupManifestPath(root string, backupID string) string {
	return filepath.Join(storage.BackupManifestsDir(root), sanitizeSecurityID(backupID)+".manifest.json")
}

func backupArchivePath(root string, backupID string) string {
	return filepath.Join(storage.BackupSnapshotsDir(root), sanitizeSecurityID(backupID)+".tar.gz")
}

func restorePlansDir(root string) string {
	return filepath.Join(storage.TrackerDir(root), "backups", "restore-plans")
}

func restorePlanPath(root string, planID string) string {
	return filepath.Join(restorePlansDir(root), sanitizeSecurityID(planID)+".json")
}

func goalManifestPath(root string, manifestID string) string {
	return filepath.Join(storage.GoalManifestsDir(root), sanitizeSecurityID(manifestID)+".json")
}
