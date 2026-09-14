package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

func (a *App) Doctor(ctx context.Context, opts DoctorOptions) (DoctorReport, error) {
	report := DoctorReport{
		Kind:    "doctor_report",
		OK:      true,
		Machine: map[string]any{},
	}
	workspaces, err := a.ListWorkspaces(ctx, ListOptions{IncludeHidden: true})
	if err != nil {
		return DoctorReport{}, err
	}
	unhealthy := 0
	for _, rec := range workspaces {
		if rec.Health != HealthAvailable && rec.Health != HealthDisabled {
			unhealthy++
		}
	}
	settings := a.snapshotSettings()
	report.Machine["registry_count"] = len(workspaces)
	report.Machine["unhealthy"] = unhealthy
	report.Machine["instance_id"] = settings.InstanceID
	report.Machine["service_port"] = settings.Service.Port
	report.Machine["agents_auto_install"] = settings.Agents.AutoInstall
	if unhealthy > 0 {
		report.OK = false
		report.IssueCodes = append(report.IssueCodes, "registry_unhealthy")
	}
	for _, rec := range workspaces {
		_, err := service.ResolveBackupStateDir(service.BackupStateDirOptions{
			Home:        a.home,
			StateDir:    a.stateDir,
			WorkspaceID: rec.WorkspaceID,
			Getenv:      a.getenv(),
			GOOS:        a.opts.GOOS,
		})
		if service.IsBackupStateConflict(err) {
			report.OK = false
			report.IssueCodes = append(report.IssueCodes, service.BackupStateConflictIssueCode)
			report.Machine["backup_state_conflict"] = rec.WorkspaceID
			break
		}
	}

	root := strings.TrimSpace(opts.Workspace)
	if root == "" {
		cwd, err := os.Getwd()
		if err == nil {
			root = cwd
		}
	}
	if root != "" {
		if wsRoot, err := service.InitializedWorkspaceRoot(root); err == nil {
			report.CurrentWorkspace = wsRoot
			wsReport, repairActions, issueCodes, err := doctorWorkspace(ctx, wsRoot, opts.Repair, a)
			if err != nil {
				return DoctorReport{}, err
			}
			report.Workspace = wsReport
			report.RepairRan = opts.Repair
			report.RepairActions = repairActions
			report.IssueCodes = append(report.IssueCodes, issueCodes...)
			if len(issueCodes) > 0 {
				report.OK = false
			}
		}
	}
	sort.Strings(report.IssueCodes)
	if report.CurrentWorkspace == "" {
		report.Summary = fmt.Sprintf("doctor machine: %d workspaces, %d unhealthy", len(workspaces), unhealthy)
	} else if report.OK {
		report.Summary = "doctor ok"
	} else {
		report.Summary = "doctor found issues"
	}
	if report.OK && report.CurrentWorkspace == "" {
		report.Summary = fmt.Sprintf("doctor ok: machine only, %d registered workspaces", len(workspaces))
	}
	return report, nil
}

func doctorWorkspace(ctx context.Context, root string, repair bool, a *App) (map[string]any, []string, []string, error) {
	ticketStore := mdstore.TicketStore{RootDir: root}
	eventLog := &eventstore.Log{RootDir: root}
	projects, err := (mdstore.ProjectStore{RootDir: root}).ListProjects(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	tickets, err := ticketStore.ListTickets(ctx, contracts.TicketListOptions{IncludeArchived: true})
	if err != nil {
		return nil, nil, nil, err
	}
	var events []any
	for _, project := range projects {
		stream, err := eventLog.StreamEvents(ctx, project.Key, 0)
		if err != nil {
			return nil, nil, nil, err
		}
		for range stream {
			events = append(events, struct{}{})
		}
	}
	cfg, err := config.Load(root)
	if err != nil {
		return nil, nil, nil, err
	}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventLog)
	if err != nil {
		if !repair {
			return nil, nil, nil, apperr.Wrap(apperr.CodeRepairNeeded, err, "ticket index is unreadable; run 'tracker doctor --repair'")
		}
	}
	repairActions := []string{}
	issueCodes := []string{}
	indexReport := map[string]any{}
	if projection != nil {
		defer func() { _ = projection.Close() }()
		stale, stored, current, err := projection.IsStale(ctx)
		if err != nil {
			return nil, nil, nil, err
		}
		indexReport["stale_before_repair"] = stale
		indexReport["stored_fingerprint"] = stored
		indexReport["current_fingerprint"] = current
		if stale && !repair {
			return nil, nil, nil, apperr.New(apperr.CodeRepairNeeded, "projection index is stale; run 'tracker doctor --repair' or 'tracker reindex'")
		}
		if repair {
			if err := service.WithWriteLock(ctx, service.FileLockManager{Root: root}, "doctor repair", func(ctx context.Context) error {
				rep, err := service.RepairWorkspace(ctx, root, a.now, eventLog, projection)
				if err != nil {
					return err
				}
				repairActions = append(repairActions, rep.Actions...)
				return nil
			}); err != nil {
				return nil, nil, nil, err
			}
		}
	}
	if repair {
		if err := service.WithWriteLock(ctx, service.FileLockManager{Root: root}, "doctor repair orchestration", func(ctx context.Context) error {
			extra, err := service.RepairOrchestration(ctx, root)
			repairActions = append(repairActions, extra...)
			return err
		}); err != nil {
			return nil, nil, nil, err
		}
	}
	payload := map[string]any{
		"events_scanned": len(events),
		"projects":       len(projects),
		"tickets":        len(tickets),
		"config":         config.MaskTrackerConfig(cfg),
		"index":          indexReport,
	}
	return payload, repairActions, issueCodes, nil
}
