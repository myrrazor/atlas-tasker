package service

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	"gopkg.in/yaml.v3"
)

type SyncRemoteStore struct {
	Root string
}

type SyncJobStore struct {
	Root string
}

type ConflictStore struct {
	Root string
}

type syncRemoteFrontmatter struct {
	contracts.SyncRemote `yaml:",inline"`
}

type syncJobFrontmatter struct {
	contracts.SyncJob `yaml:",inline"`
}

type conflictFrontmatter struct {
	contracts.ConflictRecord `yaml:",inline"`
}

func (s SyncRemoteStore) SaveSyncRemote(_ context.Context, remote contracts.SyncRemote) error {
	remote = normalizeSyncRemote(remote)
	if err := remote.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.SyncRemotesDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create sync remotes dir: %w", err)
	}
	raw, err := yaml.Marshal(syncRemoteFrontmatter{SyncRemote: remote})
	if err != nil {
		return fmt.Errorf("marshal sync remote %s: %w", remote.RemoteID, err)
	}
	body := fmt.Sprintf("%s remote -> %s", remote.Kind, remote.Location)
	doc := fmt.Sprintf("---\n%s---\n\n%s\n", string(raw), body)
	if err := os.WriteFile(storage.SyncRemoteFile(s.Root, remote.RemoteID), []byte(doc), 0o644); err != nil {
		return fmt.Errorf("write sync remote %s: %w", remote.RemoteID, err)
	}
	return nil
}

func (s SyncRemoteStore) LoadSyncRemote(_ context.Context, remoteID string) (contracts.SyncRemote, error) {
	raw, err := os.ReadFile(storage.SyncRemoteFile(s.Root, strings.TrimSpace(remoteID)))
	if err != nil {
		return contracts.SyncRemote{}, fmt.Errorf("read sync remote %s: %w", remoteID, err)
	}
	fmRaw, _, err := splitDocument(string(raw))
	if err != nil {
		return contracts.SyncRemote{}, err
	}
	var fm syncRemoteFrontmatter
	if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
		return contracts.SyncRemote{}, fmt.Errorf("parse sync remote %s: %w", remoteID, err)
	}
	remote := normalizeSyncRemote(fm.SyncRemote)
	return remote, remote.Validate()
}

func (s SyncRemoteStore) ListSyncRemotes(_ context.Context) ([]contracts.SyncRemote, error) {
	entries, err := os.ReadDir(storage.SyncRemotesDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.SyncRemote{}, nil
		}
		return nil, fmt.Errorf("read sync remotes dir: %w", err)
	}
	items := make([]contracts.SyncRemote, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		remoteID := strings.TrimSuffix(entry.Name(), ".md")
		remote, err := s.LoadSyncRemote(context.Background(), remoteID)
		if err != nil {
			return nil, err
		}
		items = append(items, remote)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].RemoteID < items[j].RemoteID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items, nil
}

func (s SyncJobStore) SaveSyncJob(_ context.Context, job contracts.SyncJob) error {
	job = normalizeSyncJob(job)
	if err := job.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.SyncJobsDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create sync jobs dir: %w", err)
	}
	raw, err := yaml.Marshal(syncJobFrontmatter{SyncJob: job})
	if err != nil {
		return fmt.Errorf("marshal sync job %s: %w", job.JobID, err)
	}
	body := fmt.Sprintf("%s %s", job.Mode, job.State)
	if strings.TrimSpace(job.BundleRef) != "" {
		body += " -> " + job.BundleRef
	}
	doc := fmt.Sprintf("---\n%s---\n\n%s\n", string(raw), body)
	if err := os.WriteFile(storage.SyncJobFile(s.Root, job.JobID), []byte(doc), 0o644); err != nil {
		return fmt.Errorf("write sync job %s: %w", job.JobID, err)
	}
	return nil
}

func (s SyncJobStore) LoadSyncJob(_ context.Context, jobID string) (contracts.SyncJob, error) {
	raw, err := os.ReadFile(storage.SyncJobFile(s.Root, strings.TrimSpace(jobID)))
	if err != nil {
		return contracts.SyncJob{}, fmt.Errorf("read sync job %s: %w", jobID, err)
	}
	fmRaw, _, err := splitDocument(string(raw))
	if err != nil {
		return contracts.SyncJob{}, err
	}
	var fm syncJobFrontmatter
	if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
		return contracts.SyncJob{}, fmt.Errorf("parse sync job %s: %w", jobID, err)
	}
	job := normalizeSyncJob(fm.SyncJob)
	return job, job.Validate()
}

func (s SyncJobStore) ListSyncJobs(_ context.Context, remoteID string) ([]contracts.SyncJob, error) {
	entries, err := os.ReadDir(storage.SyncJobsDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.SyncJob{}, nil
		}
		return nil, fmt.Errorf("read sync jobs dir: %w", err)
	}
	filterID := strings.TrimSpace(remoteID)
	items := make([]contracts.SyncJob, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		jobID := strings.TrimSuffix(entry.Name(), ".md")
		job, err := s.LoadSyncJob(context.Background(), jobID)
		if err != nil {
			return nil, err
		}
		if filterID != "" && job.RemoteID != filterID {
			continue
		}
		items = append(items, job)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].StartedAt.Equal(items[j].StartedAt) {
			return items[i].JobID < items[j].JobID
		}
		return items[i].StartedAt.Before(items[j].StartedAt)
	})
	return items, nil
}

func (s ConflictStore) SaveConflict(_ context.Context, conflict contracts.ConflictRecord) error {
	conflict = normalizeConflictRecord(conflict)
	if err := conflict.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.SyncConflictsDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create sync conflicts dir: %w", err)
	}
	raw, err := yaml.Marshal(conflictFrontmatter{ConflictRecord: conflict})
	if err != nil {
		return fmt.Errorf("marshal conflict %s: %w", conflict.ConflictID, err)
	}
	body := fmt.Sprintf("%s %s", conflict.EntityKind, conflict.ConflictType)
	doc := fmt.Sprintf("---\n%s---\n\n%s\n", string(raw), body)
	if err := os.WriteFile(storage.SyncConflictFile(s.Root, conflict.ConflictID), []byte(doc), 0o644); err != nil {
		return fmt.Errorf("write conflict %s: %w", conflict.ConflictID, err)
	}
	return nil
}

func (s ConflictStore) LoadConflict(_ context.Context, conflictID string) (contracts.ConflictRecord, error) {
	raw, err := os.ReadFile(storage.SyncConflictFile(s.Root, strings.TrimSpace(conflictID)))
	if err != nil {
		return contracts.ConflictRecord{}, fmt.Errorf("read conflict %s: %w", conflictID, err)
	}
	fmRaw, _, err := splitDocument(string(raw))
	if err != nil {
		return contracts.ConflictRecord{}, err
	}
	var fm conflictFrontmatter
	if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
		return contracts.ConflictRecord{}, fmt.Errorf("parse conflict %s: %w", conflictID, err)
	}
	conflict := normalizeConflictRecord(fm.ConflictRecord)
	return conflict, conflict.Validate()
}

func (s ConflictStore) ListConflicts(_ context.Context) ([]contracts.ConflictRecord, error) {
	entries, err := os.ReadDir(storage.SyncConflictsDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.ConflictRecord{}, nil
		}
		return nil, fmt.Errorf("read sync conflicts dir: %w", err)
	}
	items := make([]contracts.ConflictRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		conflictID := strings.TrimSuffix(entry.Name(), ".md")
		conflict, err := s.LoadConflict(context.Background(), conflictID)
		if err != nil {
			return nil, err
		}
		items = append(items, conflict)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].OpenedAt.Equal(items[j].OpenedAt) {
			return items[i].ConflictID < items[j].ConflictID
		}
		return items[i].OpenedAt.Before(items[j].OpenedAt)
	})
	return items, nil
}

func normalizeSyncRemote(remote contracts.SyncRemote) contracts.SyncRemote {
	remote.RemoteID = strings.TrimSpace(remote.RemoteID)
	remote.Location = strings.TrimSpace(remote.Location)
	if remote.DefaultAction == "" {
		remote.DefaultAction = contracts.SyncDefaultActionFetch
	}
	if remote.CreatedAt.IsZero() {
		remote.CreatedAt = timeNowUTC()
	}
	if remote.UpdatedAt.IsZero() {
		remote.UpdatedAt = remote.CreatedAt
	}
	return remote
}

func normalizeSyncJob(job contracts.SyncJob) contracts.SyncJob {
	job.JobID = strings.TrimSpace(job.JobID)
	job.RemoteID = strings.TrimSpace(job.RemoteID)
	job.BundleRef = strings.TrimSpace(job.BundleRef)
	if job.StartedAt.IsZero() {
		job.StartedAt = timeNowUTC()
	}
	job.Warnings = uniqueStrings(job.Warnings)
	job.ReasonCodes = uniqueStrings(job.ReasonCodes)
	job.ConflictIDs = uniqueStrings(job.ConflictIDs)
	if job.Counts == nil {
		job.Counts = map[string]int{}
	}
	if job.SchemaVersion == 0 {
		job.SchemaVersion = contracts.CurrentSchemaVersion
	}
	return job
}

func normalizeConflictRecord(conflict contracts.ConflictRecord) contracts.ConflictRecord {
	conflict.ConflictID = strings.TrimSpace(conflict.ConflictID)
	conflict.EntityKind = strings.TrimSpace(conflict.EntityKind)
	conflict.EntityUID = strings.TrimSpace(conflict.EntityUID)
	conflict.LocalRef = strings.TrimSpace(conflict.LocalRef)
	conflict.RemoteRef = strings.TrimSpace(conflict.RemoteRef)
	conflict.OpenedByJob = strings.TrimSpace(conflict.OpenedByJob)
	if conflict.OpenedAt.IsZero() {
		conflict.OpenedAt = timeNowUTC()
	}
	if conflict.SchemaVersion == 0 {
		conflict.SchemaVersion = contracts.CurrentSchemaVersion
	}
	return conflict
}
