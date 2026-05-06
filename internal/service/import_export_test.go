package service

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

func TestCreateAndVerifyExportBundleRoundTrip(t *testing.T) {
	root, actions, queries, projectStore, _, eventsLog := newImportExportHarness(t)
	ctx := context.Background()
	now := actions.now()

	if err := projectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "App", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := actions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "Ship export",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed ticket"); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	created, err := actions.CreateExportBundle(ctx, "workspace", contracts.Actor("human:owner"), "export workspace")
	if err != nil {
		t.Fatalf("create export bundle: %v", err)
	}
	if created.Bundle.BundleID == "" || created.Bundle.Status != contracts.ExportBundleCreated || created.FileCount == 0 {
		t.Fatalf("unexpected export bundle detail: %#v", created)
	}
	for _, path := range []string{created.Bundle.ArtifactPath, created.Bundle.ManifestPath, created.Bundle.ChecksumPath, storage.ExportBundleFile(root, created.Bundle.BundleID)} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected export artifact %s: %v", path, err)
		}
	}

	detail, err := queries.ExportBundleDetail(ctx, created.Bundle.BundleID)
	if err != nil {
		t.Fatalf("query export bundle detail: %v", err)
	}
	if detail.Bundle.BundleID != created.Bundle.BundleID || detail.FileCount != created.FileCount {
		t.Fatalf("unexpected queried bundle detail: %#v", detail)
	}

	verifiedByID, err := actions.VerifyExportBundle(ctx, created.Bundle.BundleID, contracts.Actor("human:owner"), "verify export bundle")
	if err != nil {
		t.Fatalf("verify export bundle by id: %v", err)
	}
	if !verifiedByID.Verified || len(verifiedByID.Errors) != 0 {
		t.Fatalf("expected clean verification by id, got %#v", verifiedByID)
	}

	verifiedByPath, err := actions.VerifyExportBundle(ctx, created.Bundle.ArtifactPath, contracts.Actor("human:owner"), "verify export path")
	if err != nil {
		t.Fatalf("verify export bundle by path: %v", err)
	}
	if !verifiedByPath.Verified || len(verifiedByPath.Errors) != 0 {
		t.Fatalf("expected clean verification by path, got %#v", verifiedByPath)
	}

	events, err := eventsLog.StreamEvents(ctx, workspaceProjectKey, 0)
	if err != nil {
		t.Fatalf("stream workspace events: %v", err)
	}
	types := make([]contracts.EventType, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	if !slices.Contains(types, contracts.EventExportCreated) || !slices.Contains(types, contracts.EventExportVerified) {
		t.Fatalf("expected export create/verify events, got %#v", types)
	}
}

func TestExportBundleIncludesGovernanceStores(t *testing.T) {
	_, actions, _, _, _, _ := newImportExportHarness(t)
	ctx := context.Background()
	if _, err := actions.CreateGovernancePack(ctx, GovernancePackCreateOptions{
		Name:             "Release quorum",
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionTicketComplete},
		QuorumCount:      1,
	}, contracts.Actor("human:owner"), "create governance pack"); err != nil {
		t.Fatalf("create governance pack: %v", err)
	}
	if _, err := actions.ApplyGovernancePack(ctx, "release-quorum", "", contracts.Actor("human:owner"), "apply governance pack"); err != nil {
		t.Fatalf("apply governance pack: %v", err)
	}
	created, err := actions.CreateExportBundle(ctx, "workspace", contracts.Actor("human:owner"), "export workspace")
	if err != nil {
		t.Fatalf("create export bundle: %v", err)
	}
	names := exportBundleEntryNames(t, created.Bundle.ArtifactPath)
	for _, want := range []string{
		filepath.ToSlash(filepath.Join(".tracker", "governance", "packs", "release-quorum.toml")),
		filepath.ToSlash(filepath.Join(".tracker", "governance", "policies", "release-quorum.toml")),
	} {
		if !names[want] {
			t.Fatalf("expected export bundle to include %s; names=%#v", want, names)
		}
	}
}

func TestAtlasBundlePreviewApplyAndLifecycle(t *testing.T) {
	sourceRoot, sourceActions, _, sourceProjectStore, _, _ := newImportExportHarness(t)
	ctx := context.Background()
	now := sourceActions.now()

	if err := sourceProjectStore.CreateProject(ctx, contracts.Project{Key: "SRC", Name: "Source", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create source project: %v", err)
	}
	if _, err := sourceActions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "SRC",
		Title:         "Import me",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed source ticket"); err != nil {
		t.Fatalf("create source ticket: %v", err)
	}
	bundle, err := sourceActions.CreateExportBundle(ctx, "workspace", contracts.Actor("human:owner"), "export source")
	if err != nil {
		t.Fatalf("create source export bundle: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sourceRoot, ".tracker")); err != nil {
		t.Fatalf("expected source tracker dir: %v", err)
	}

	_, targetActions, _, _, targetTicketStore, targetEventsLog := newImportExportHarness(t)
	preview, err := targetActions.PreviewImport(ctx, bundle.Bundle.ArtifactPath, contracts.Actor("human:owner"), "preview atlas bundle")
	if err != nil {
		t.Fatalf("preview atlas bundle import: %v", err)
	}
	if preview.Job.Status != contracts.ImportJobPreviewed || preview.Plan.SourceType != contracts.ImportSourceAtlasBundle {
		t.Fatalf("unexpected preview result: %#v", preview)
	}
	if len(preview.Plan.Items) == 0 || preview.Plan.Items[0].TicketID != "SRC-1" {
		t.Fatalf("expected imported ticket in preview, got %#v", preview.Plan.Items)
	}

	applied, err := targetActions.ApplyImport(ctx, preview.Job.JobID, contracts.Actor("human:owner"), "apply atlas bundle")
	if err != nil {
		t.Fatalf("apply atlas bundle import: %v", err)
	}
	if applied.Job.Status != contracts.ImportJobApplied || applied.Job.PartialApplied {
		t.Fatalf("unexpected applied job state: %#v", applied.Job)
	}
	imported, err := targetTicketStore.GetTicket(ctx, "SRC-1")
	if err != nil {
		t.Fatalf("expected imported ticket: %v", err)
	}
	if imported.Project != "SRC" || imported.Title != "Import me" {
		t.Fatalf("unexpected imported ticket: %#v", imported)
	}

	events, err := targetEventsLog.StreamEvents(ctx, workspaceProjectKey, 0)
	if err != nil {
		t.Fatalf("stream target workspace events: %v", err)
	}
	types := make([]contracts.EventType, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	for _, expected := range []contracts.EventType{contracts.EventImportPreviewed, contracts.EventImportValidated, contracts.EventImportStarted, contracts.EventImportApplied} {
		if !slices.Contains(types, expected) {
			t.Fatalf("expected import lifecycle event %s in %#v", expected, types)
		}
	}
}

func TestApplyImportFailsCleanlyWhenPreviewHasConflicts(t *testing.T) {
	sourceRoot, sourceActions, _, sourceProjectStore, _, _ := newImportExportHarness(t)
	ctx := context.Background()
	now := sourceActions.now()

	if err := sourceProjectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "Source", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create source project: %v", err)
	}
	if _, err := sourceActions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "Existing ticket",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityMedium,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed source ticket"); err != nil {
		t.Fatalf("create source ticket: %v", err)
	}
	bundle, err := sourceActions.CreateExportBundle(ctx, "workspace", contracts.Actor("human:owner"), "export source")
	if err != nil {
		t.Fatalf("create source export bundle: %v", err)
	}
	if _, err := os.Stat(sourceRoot); err != nil {
		t.Fatalf("expected source root to exist: %v", err)
	}

	_, targetActions, targetQueries, targetProjectStore, _, targetEventsLog := newImportExportHarness(t)
	if err := targetProjectStore.CreateProject(ctx, contracts.Project{Key: "APP", Name: "Target", CreatedAt: now, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("create target project: %v", err)
	}
	if _, err := targetActions.CreateTrackedTicket(ctx, contracts.TicketSnapshot{
		Project:       "APP",
		Title:         "Already here",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityHigh,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, contracts.Actor("human:owner"), "seed target ticket"); err != nil {
		t.Fatalf("create target ticket: %v", err)
	}

	preview, err := targetActions.PreviewImport(ctx, bundle.Bundle.ArtifactPath, contracts.Actor("human:owner"), "preview conflict bundle")
	if err != nil {
		t.Fatalf("preview conflict bundle: %v", err)
	}
	if len(preview.Plan.Conflicts) == 0 {
		t.Fatalf("expected preview conflicts, got %#v", preview.Plan)
	}
	if _, err := targetActions.ApplyImport(ctx, preview.Job.JobID, contracts.Actor("human:owner"), "apply conflict bundle"); err == nil || apperr.CodeOf(err) != apperr.CodeConflict {
		t.Fatalf("expected conflict apply failure, got %v", err)
	}

	job, err := targetQueries.ImportJobDetail(ctx, preview.Job.JobID)
	if err != nil {
		t.Fatalf("load failed import job: %v", err)
	}
	if job.Job.Status != contracts.ImportJobFailed || job.Job.PartialApplied {
		t.Fatalf("unexpected failed import job: %#v", job.Job)
	}

	events, err := targetEventsLog.StreamEvents(ctx, workspaceProjectKey, 0)
	if err != nil {
		t.Fatalf("stream target workspace events: %v", err)
	}
	types := make([]contracts.EventType, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	if !slices.Contains(types, contracts.EventImportFailed) {
		t.Fatalf("expected import.failed event, got %#v", types)
	}
}

func TestPreviewImportRejectsPathTraversalBundle(t *testing.T) {
	root, actions, _, _, _, _ := newImportExportHarness(t)
	ctx := context.Background()
	archivePath := filepath.Join(t.TempDir(), "malicious.tar.gz")

	if err := writeTestBundle(archivePath, map[string][]byte{
		"manifest.json": mustJSON(t, bundleManifest{
			FormatVersion: "v1",
			BundleID:      "bundle_bad",
			Scope:         "workspace",
			CreatedAt:     actions.now(),
			Files: []bundleFileRecord{
				{Path: "../escape.txt", SHA256: strings.Repeat("0", 64), Size: int64(len("boom"))},
			},
		}),
		"../escape.txt": []byte("boom"),
	}); err != nil {
		t.Fatalf("write malicious bundle: %v", err)
	}

	if _, err := actions.PreviewImport(ctx, archivePath, contracts.Actor("human:owner"), "preview malicious bundle"); err == nil || apperr.CodeOf(err) != apperr.CodeInvalidInput {
		t.Fatalf("expected invalid input for traversal bundle, got %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("expected workspace root to remain intact: %v", err)
	}
}

func TestPreviewImportRejectsAbsolutePathBundle(t *testing.T) {
	_, actions, _, _, _, _ := newImportExportHarness(t)
	ctx := context.Background()
	archivePath := filepath.Join(t.TempDir(), "absolute.tar.gz")

	if err := writeTestBundle(archivePath, map[string][]byte{
		"manifest.json": mustJSON(t, bundleManifest{
			FormatVersion: "v1",
			BundleID:      "bundle_absolute",
			Scope:         "workspace",
			CreatedAt:     actions.now(),
			Files: []bundleFileRecord{
				{Path: "/escape.txt", SHA256: strings.Repeat("0", 64), Size: int64(len("boom"))},
			},
		}),
		"/escape.txt": []byte("boom"),
	}); err != nil {
		t.Fatalf("write absolute bundle: %v", err)
	}

	if _, err := actions.PreviewImport(ctx, archivePath, contracts.Actor("human:owner"), "preview absolute bundle"); err == nil || apperr.CodeOf(err) != apperr.CodeInvalidInput {
		t.Fatalf("expected invalid input for absolute path bundle, got %v", err)
	}
}

func TestPreviewImportRejectsSymlinkBundle(t *testing.T) {
	_, actions, _, _, _, _ := newImportExportHarness(t)
	ctx := context.Background()
	archivePath := filepath.Join(t.TempDir(), "symlink.tar.gz")

	if err := writeTestBundleEntries(archivePath, []testTarEntry{
		{
			Header: tar.Header{
				Name:     "manifest.json",
				Mode:     0o644,
				Size:     int64(len(mustJSON(t, bundleManifest{FormatVersion: "v1", BundleID: "bundle_symlink", Scope: "workspace", CreatedAt: actions.now()}))),
				ModTime:  time.Now().UTC(),
				Typeflag: tar.TypeReg,
			},
			Body: mustJSON(t, bundleManifest{FormatVersion: "v1", BundleID: "bundle_symlink", Scope: "workspace", CreatedAt: actions.now()}),
		},
		{
			Header: tar.Header{
				Name:     "linked.txt",
				Typeflag: tar.TypeSymlink,
				Linkname: "target.txt",
				ModTime:  time.Now().UTC(),
			},
		},
	}); err != nil {
		t.Fatalf("write symlink bundle: %v", err)
	}

	if _, err := actions.PreviewImport(ctx, archivePath, contracts.Actor("human:owner"), "preview symlink bundle"); err == nil || apperr.CodeOf(err) != apperr.CodeInvalidInput || !strings.Contains(err.Error(), "bundle_symlink_rejected") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
}

func TestPreviewImportRejectsHardlinkBundle(t *testing.T) {
	_, actions, _, _, _, _ := newImportExportHarness(t)
	ctx := context.Background()
	archivePath := filepath.Join(t.TempDir(), "hardlink.tar.gz")

	if err := writeTestBundleEntries(archivePath, []testTarEntry{
		{
			Header: tar.Header{
				Name:     "manifest.json",
				Mode:     0o644,
				Size:     int64(len(mustJSON(t, bundleManifest{FormatVersion: "v1", BundleID: "bundle_hardlink", Scope: "workspace", CreatedAt: actions.now()}))),
				ModTime:  time.Now().UTC(),
				Typeflag: tar.TypeReg,
			},
			Body: mustJSON(t, bundleManifest{FormatVersion: "v1", BundleID: "bundle_hardlink", Scope: "workspace", CreatedAt: actions.now()}),
		},
		{
			Header: tar.Header{
				Name:     "linked.txt",
				Typeflag: tar.TypeLink,
				Linkname: "target.txt",
				ModTime:  time.Now().UTC(),
			},
		},
	}); err != nil {
		t.Fatalf("write hardlink bundle: %v", err)
	}

	if _, err := actions.PreviewImport(ctx, archivePath, contracts.Actor("human:owner"), "preview hardlink bundle"); err == nil || apperr.CodeOf(err) != apperr.CodeInvalidInput || !strings.Contains(err.Error(), "bundle_hardlink_rejected") {
		t.Fatalf("expected hardlink rejection, got %v", err)
	}
}

func TestPreviewImportRejectsCompressionBombBundle(t *testing.T) {
	_, actions, _, _, _, _ := newImportExportHarness(t)
	ctx := context.Background()
	archivePath := filepath.Join(t.TempDir(), "bomb.tar.gz")
	huge := bytes.Repeat([]byte("0"), 1<<20)

	if err := writeTestBundle(archivePath, map[string][]byte{
		"manifest.json": mustJSON(t, bundleManifest{
			FormatVersion: "v1",
			BundleID:      "bundle_bomb",
			Scope:         "workspace",
			CreatedAt:     actions.now(),
			Files: []bundleFileRecord{
				{Path: "projects/APP/tickets/APP-1.md", SHA256: strings.Repeat("0", 64), Size: int64(len(huge))},
			},
		}),
		"projects/APP/tickets/APP-1.md": huge,
	}); err != nil {
		t.Fatalf("write compression bomb bundle: %v", err)
	}

	if _, err := actions.PreviewImport(ctx, archivePath, contracts.Actor("human:owner"), "preview compression bomb"); err == nil || apperr.CodeOf(err) != apperr.CodeInvalidInput || !strings.Contains(err.Error(), "compression ratio limit") {
		t.Fatalf("expected compression ratio rejection, got %v", err)
	}
}

func TestPreviewImportRejectsOverfullBundle(t *testing.T) {
	_, actions, _, _, _, _ := newImportExportHarness(t)
	ctx := context.Background()
	archivePath := filepath.Join(t.TempDir(), "overfull.tar.gz")

	files := map[string][]byte{
		"manifest.json": mustJSON(t, bundleManifest{FormatVersion: "v1", BundleID: "bundle_overfull", Scope: "workspace", CreatedAt: actions.now()}),
	}
	for i := 0; i < bundleArchiveMaxFiles+1; i++ {
		files[fmt.Sprintf("projects/APP/tickets/APP-%d.md", i)] = []byte("x")
	}
	if err := writeTestBundle(archivePath, files); err != nil {
		t.Fatalf("write overfull bundle: %v", err)
	}

	if _, err := actions.PreviewImport(ctx, archivePath, contracts.Actor("human:owner"), "preview overfull bundle"); err == nil || apperr.CodeOf(err) != apperr.CodeInvalidInput || !strings.Contains(err.Error(), "file count limit") {
		t.Fatalf("expected file count rejection, got %v", err)
	}
}

func newImportExportHarness(t *testing.T) (string, *ActionService, *QueryService, mdstore.ProjectStore, mdstore.TicketStore, *eventstore.Log) {
	t.Helper()
	root := t.TempDir()
	now := time.Date(2026, 3, 27, 9, 0, 0, 0, time.UTC)

	projectStore := mdstore.ProjectStore{RootDir: root}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: func() time.Time { return now }}
	eventsLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventsLog)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = projection.Close() })

	actions := NewActionService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now }, FileLockManager{Root: root}, nil, nil)
	queries := NewQueryService(root, projectStore, ticketStore, eventsLog, projection, func() time.Time { return now })
	return root, actions, queries, projectStore, ticketStore, eventsLog
}

func writeTestBundle(path string, files map[string][]byte) error {
	entries := make([]testTarEntry, 0, len(files))
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		raw := files[name]
		entries = append(entries, testTarEntry{
			Header: tar.Header{Name: name, Mode: 0o644, Size: int64(len(raw)), ModTime: time.Now().UTC(), Typeflag: tar.TypeReg},
			Body:   raw,
		})
	}
	return writeTestBundleEntries(path, entries)
}

type testTarEntry struct {
	Header tar.Header
	Body   []byte
}

func writeTestBundleEntries(path string, entries []testTarEntry) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	gz := gzip.NewWriter(file)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	for _, entry := range entries {
		if err := tw.WriteHeader(&entry.Header); err != nil {
			return err
		}
		if len(entry.Body) == 0 {
			continue
		}
		if _, err := tw.Write(entry.Body); err != nil {
			return err
		}
	}
	return nil
}

func exportBundleEntryNames(t *testing.T, path string) map[string]bool {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open export bundle: %v", err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("read export gzip: %v", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	names := map[string]bool{}
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read export tar: %v", err)
		}
		names[filepath.ToSlash(header.Name)] = true
	}
	return names
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return raw
}
