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

type ImportJobStore struct {
	Root string
}

type ExportBundleStore struct {
	Root string
}

type importJobFrontmatter struct {
	contracts.ImportJob `yaml:",inline"`
}

type exportBundleFrontmatter struct {
	contracts.ExportBundle `yaml:",inline"`
}

func (s ImportJobStore) SaveImportJob(_ context.Context, job contracts.ImportJob) error {
	job = normalizeImportJob(job)
	if err := job.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.ImportsDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create imports dir: %w", err)
	}
	raw, err := yaml.Marshal(importJobFrontmatter{ImportJob: job})
	if err != nil {
		return fmt.Errorf("marshal import job %s: %w", job.JobID, err)
	}
	body := strings.TrimSpace(job.Summary)
	if body == "" {
		body = fmt.Sprintf("Import job `%s` from `%s`.", job.JobID, job.SourceType)
	}
	doc := fmt.Sprintf("---\n%s---\n\n%s\n", string(raw), body)
	if err := os.WriteFile(storage.ImportJobFile(s.Root, job.JobID), []byte(doc), 0o644); err != nil {
		return fmt.Errorf("write import job %s: %w", job.JobID, err)
	}
	return nil
}

func (s ImportJobStore) LoadImportJob(_ context.Context, jobID string) (contracts.ImportJob, error) {
	raw, err := os.ReadFile(storage.ImportJobFile(s.Root, jobID))
	if err != nil {
		return contracts.ImportJob{}, fmt.Errorf("read import job %s: %w", jobID, err)
	}
	fmRaw, body, err := splitDocument(string(raw))
	if err != nil {
		return contracts.ImportJob{}, err
	}
	var fm importJobFrontmatter
	if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
		return contracts.ImportJob{}, fmt.Errorf("parse import job %s: %w", jobID, err)
	}
	job := fm.ImportJob
	if strings.TrimSpace(job.Summary) == "" {
		job.Summary = strings.TrimSpace(body)
	}
	return normalizeImportJob(job), nil
}

func (s ImportJobStore) ListImportJobs(_ context.Context) ([]contracts.ImportJob, error) {
	entries, err := os.ReadDir(storage.ImportsDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.ImportJob{}, nil
		}
		return nil, fmt.Errorf("read imports dir: %w", err)
	}
	items := make([]contracts.ImportJob, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		job, err := s.LoadImportJob(context.Background(), strings.TrimSuffix(entry.Name(), ".md"))
		if err != nil {
			return nil, err
		}
		items = append(items, job)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].JobID < items[j].JobID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items, nil
}

func (s ExportBundleStore) SaveExportBundle(_ context.Context, bundle contracts.ExportBundle) error {
	bundle = normalizeExportBundle(bundle)
	if err := bundle.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.ExportsDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create exports dir: %w", err)
	}
	raw, err := yaml.Marshal(exportBundleFrontmatter{ExportBundle: bundle})
	if err != nil {
		return fmt.Errorf("marshal export bundle %s: %w", bundle.BundleID, err)
	}
	body := fmt.Sprintf("Export bundle `%s`.", bundle.BundleID)
	doc := fmt.Sprintf("---\n%s---\n\n%s\n", string(raw), body)
	if err := os.WriteFile(storage.ExportBundleFile(s.Root, bundle.BundleID), []byte(doc), 0o644); err != nil {
		return fmt.Errorf("write export bundle %s: %w", bundle.BundleID, err)
	}
	return nil
}

func (s ExportBundleStore) LoadExportBundle(_ context.Context, bundleID string) (contracts.ExportBundle, error) {
	raw, err := os.ReadFile(storage.ExportBundleFile(s.Root, bundleID))
	if err != nil {
		return contracts.ExportBundle{}, fmt.Errorf("read export bundle %s: %w", bundleID, err)
	}
	fmRaw, _, err := splitDocument(string(raw))
	if err != nil {
		return contracts.ExportBundle{}, err
	}
	var fm exportBundleFrontmatter
	if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
		return contracts.ExportBundle{}, fmt.Errorf("parse export bundle %s: %w", bundleID, err)
	}
	return normalizeExportBundle(fm.ExportBundle), nil
}

func (s ExportBundleStore) ListExportBundles(_ context.Context) ([]contracts.ExportBundle, error) {
	entries, err := os.ReadDir(storage.ExportsDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.ExportBundle{}, nil
		}
		return nil, fmt.Errorf("read exports dir: %w", err)
	}
	items := make([]contracts.ExportBundle, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		bundle, err := s.LoadExportBundle(context.Background(), strings.TrimSuffix(entry.Name(), ".md"))
		if err != nil {
			return nil, err
		}
		items = append(items, bundle)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].BundleID < items[j].BundleID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items, nil
}

func normalizeImportJob(job contracts.ImportJob) contracts.ImportJob {
	if job.SchemaVersion == 0 {
		job.SchemaVersion = contracts.CurrentSchemaVersion
	}
	if job.ImportJobUID == "" {
		job.ImportJobUID = contracts.ImportJobUID(job.JobID)
	}
	return job
}

func normalizeExportBundle(bundle contracts.ExportBundle) contracts.ExportBundle {
	if bundle.SchemaVersion == 0 {
		bundle.SchemaVersion = contracts.CurrentSchemaVersion
	}
	if bundle.ExportBundleUID == "" {
		bundle.ExportBundleUID = contracts.ExportBundleUID(bundle.BundleID)
	}
	return bundle
}
