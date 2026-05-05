package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

type OrchestrationDoctorReport struct {
	RunIssues      int      `json:"run_issues"`
	GateIssues     int      `json:"gate_issues"`
	HandoffIssues  int      `json:"handoff_issues"`
	EvidenceIssues int      `json:"evidence_issues"`
	RuntimeIssues  int      `json:"runtime_issues"`
	WorktreeIssues int      `json:"worktree_issues"`
	ArchiveIssues  int      `json:"archive_issues"`
	ChangeIssues   int      `json:"change_issues"`
	CheckIssues    int      `json:"check_issues"`
	IssueCodes     []string `json:"issue_codes"`
}

func (r OrchestrationDoctorReport) TotalIssues() int {
	return r.RunIssues + r.GateIssues + r.HandoffIssues + r.EvidenceIssues + r.RuntimeIssues + r.WorktreeIssues + r.ArchiveIssues + r.ChangeIssues + r.CheckIssues
}

func AuditOrchestration(ctx context.Context, root string, tickets contracts.TicketStore) (OrchestrationDoctorReport, error) {
	canonicalRoot, err := CanonicalWorkspaceRoot(root)
	if err == nil {
		root = canonicalRoot
	}
	report := OrchestrationDoctorReport{IssueCodes: []string{}}
	issueCodes := map[string]struct{}{}
	addIssue := func(code string, bucket *int) {
		if strings.TrimSpace(code) == "" {
			return
		}
		*bucket = *bucket + 1
		issueCodes[code] = struct{}{}
	}
	allTickets, err := tickets.ListTickets(ctx, contracts.TicketListOptions{IncludeArchived: true})
	if err != nil {
		return OrchestrationDoctorReport{}, err
	}
	runs, err := auditRunDocs(ctx, root, &report, addIssue)
	if err != nil {
		return OrchestrationDoctorReport{}, err
	}
	gates, err := auditGateDocs(ctx, root, &report, addIssue)
	if err != nil {
		return OrchestrationDoctorReport{}, err
	}
	handoffs, err := auditHandoffDocs(ctx, root, &report, addIssue)
	if err != nil {
		return OrchestrationDoctorReport{}, err
	}
	archives, err := auditArchiveDocs(ctx, root, &report, addIssue)
	if err != nil {
		return OrchestrationDoctorReport{}, err
	}
	changes, err := auditChangeDocs(ctx, root, &report, addIssue)
	if err != nil {
		return OrchestrationDoctorReport{}, err
	}
	checks, err := auditCheckDocs(ctx, root, &report, addIssue)
	if err != nil {
		return OrchestrationDoctorReport{}, err
	}

	ticketsByID := make(map[string]contracts.TicketSnapshot, len(allTickets))
	for _, ticket := range allTickets {
		ticketsByID[ticket.ID] = ticket
	}
	runsByID := make(map[string]contracts.RunSnapshot, len(runs))
	worktrees := WorktreeManager{Root: root}
	archivedPaths := archivePathState(archives, contracts.ArchiveRecordArchived)
	for _, run := range runs {
		runsByID[run.RunID] = run
		if err := run.Validate(); err != nil {
			addIssue("run_invalid", &report.RunIssues)
		}
		ticket, ok := ticketsByID[run.TicketID]
		if !ok || ticket.Project != run.Project {
			addIssue("run_ticket_missing", &report.RunIssues)
		}
		if err := auditRuntimeForRun(root, run, archivedPaths, &report, addIssue); err != nil {
			return OrchestrationDoctorReport{}, err
		}
		if err := auditWorktreeForRun(ctx, worktrees, run, &report, addIssue); err != nil {
			return OrchestrationDoctorReport{}, err
		}
	}

	evidenceByID := map[string]contracts.EvidenceItem{}
	if err := auditEvidenceDirs(root, runsByID, archivedPaths, &report, addIssue); err != nil {
		return OrchestrationDoctorReport{}, err
	}
	for _, run := range runs {
		items, err := auditEvidenceDocs(ctx, root, run.RunID, &report, addIssue)
		if err != nil {
			return OrchestrationDoctorReport{}, err
		}
		for _, item := range items {
			evidenceByID[item.EvidenceID] = item
			if err := item.Validate(); err != nil {
				addIssue("evidence_invalid", &report.EvidenceIssues)
			}
			if _, ok := runsByID[item.RunID]; !ok {
				addIssue("evidence_run_missing", &report.EvidenceIssues)
			}
			if code := auditEvidenceArtifact(root, item, archivedPaths); code != "" {
				addIssue(code, &report.EvidenceIssues)
			}
		}
	}

	for _, gate := range gates {
		if err := gate.Validate(); err != nil {
			addIssue("gate_invalid", &report.GateIssues)
		}
		ticket, ok := ticketsByID[gate.TicketID]
		if !ok {
			addIssue("gate_ticket_missing", &report.GateIssues)
			continue
		}
		if gate.RunID != "" {
			if _, ok := runsByID[gate.RunID]; !ok {
				addIssue("gate_run_missing", &report.GateIssues)
			}
		}
		if gate.State == contracts.GateStateOpen && !stringSliceContains(ticket.OpenGateIDs, gate.GateID) {
			addIssue("gate_ticket_mismatch", &report.GateIssues)
		}
		if gate.State != contracts.GateStateOpen && stringSliceContains(ticket.OpenGateIDs, gate.GateID) {
			addIssue("gate_ticket_mismatch", &report.GateIssues)
		}
	}

	for _, handoff := range handoffs {
		if err := handoff.Validate(); err != nil {
			addIssue("handoff_invalid", &report.HandoffIssues)
		}
		if _, ok := ticketsByID[handoff.TicketID]; !ok {
			addIssue("handoff_ticket_missing", &report.HandoffIssues)
		}
		if _, ok := runsByID[handoff.SourceRunID]; !ok {
			addIssue("handoff_run_missing", &report.HandoffIssues)
		}
		for _, evidenceID := range handoff.EvidenceLinks {
			if _, ok := evidenceByID[evidenceID]; !ok {
				addIssue("handoff_evidence_missing", &report.HandoffIssues)
				break
			}
		}
	}

	for _, record := range archives {
		if err := record.Validate(); err != nil {
			addIssue("archive_invalid", &report.ArchiveIssues)
			continue
		}
		if strings.TrimSpace(record.PayloadDir) == "" || !pathWithinDir(storage.ArchivesDir(root), record.PayloadDir) {
			addIssue("archive_payload_invalid", &report.ArchiveIssues)
			continue
		}
		payloadInfo, err := os.Stat(record.PayloadDir)
		if err != nil || !payloadInfo.IsDir() {
			addIssue("archive_payload_missing", &report.ArchiveIssues)
		}
		for _, path := range record.SourcePaths {
			if strings.TrimSpace(path) == "" || filepath.IsAbs(path) || strings.HasPrefix(filepath.Clean(path), "..") {
				addIssue("archive_source_invalid", &report.ArchiveIssues)
				continue
			}
			live := filepath.Join(root, path)
			_, liveErr := os.Stat(live)
			if record.State == contracts.ArchiveRecordArchived && liveErr == nil {
				addIssue("archive_live_source_stale", &report.ArchiveIssues)
			}
			if record.State == contracts.ArchiveRecordRestored && liveErr != nil {
				addIssue("archive_restore_incomplete", &report.ArchiveIssues)
			}
		}
	}

	for _, change := range changes {
		if err := change.Validate(); err != nil {
			addIssue("change_invalid", &report.ChangeIssues)
		}
		if _, ok := ticketsByID[change.TicketID]; !ok {
			addIssue("change_ticket_missing", &report.ChangeIssues)
		}
		if change.RunID != "" {
			if _, ok := runsByID[change.RunID]; !ok {
				addIssue("change_run_missing", &report.ChangeIssues)
			}
		}
	}

	changeByID := make(map[string]contracts.ChangeRef, len(changes))
	for _, change := range changes {
		changeByID[change.ChangeID] = change
	}
	for _, check := range checks {
		if err := check.Validate(); err != nil {
			addIssue("check_invalid", &report.CheckIssues)
			continue
		}
		switch check.Scope {
		case contracts.CheckScopeRun:
			if _, ok := runsByID[check.ScopeID]; !ok {
				addIssue("check_scope_missing", &report.CheckIssues)
			}
		case contracts.CheckScopeChange:
			if _, ok := changeByID[check.ScopeID]; !ok {
				addIssue("check_scope_missing", &report.CheckIssues)
			}
		case contracts.CheckScopeTicket:
			if _, ok := ticketsByID[check.ScopeID]; !ok {
				addIssue("check_scope_missing", &report.CheckIssues)
			}
		}
	}

	for _, code := range retentionOverdueCodes(ctx, root, allTickets) {
		addIssue(code, &report.ArchiveIssues)
	}
	for _, code := range sortedIssueCodes(issueCodes) {
		report.IssueCodes = append(report.IssueCodes, code)
	}
	return report, nil
}

func RepairOrchestration(ctx context.Context, root string) ([]string, error) {
	canonicalRoot, err := CanonicalWorkspaceRoot(root)
	if err == nil {
		root = canonicalRoot
	}
	runs, err := auditRunDocs(ctx, root, nil, nil)
	if err != nil {
		return nil, err
	}
	hasTrackedWorktrees := false
	for _, run := range runs {
		if strings.TrimSpace(run.WorktreePath) != "" {
			hasTrackedWorktrees = true
			break
		}
	}
	if !hasTrackedWorktrees {
		return []string{}, nil
	}
	scm := SCMService{Root: root}
	repo, err := scm.RepoStatus(ctx)
	if err != nil {
		return nil, err
	}
	if !repo.Present {
		return []string{}, nil
	}
	worktrees := WorktreeManager{Root: root}
	if _, err := worktrees.Repair(ctx, runs); err != nil {
		return nil, err
	}
	if _, err := worktrees.Prune(ctx, runs); err != nil {
		return nil, err
	}
	return []string{"repaired tracked worktree metadata", "pruned stale worktree metadata"}, nil
}

func auditRunDocs(ctx context.Context, root string, report *OrchestrationDoctorReport, addIssue func(string, *int)) ([]contracts.RunSnapshot, error) {
	entries, err := os.ReadDir(storage.RunsDir(root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.RunSnapshot{}, nil
		}
		return nil, fmt.Errorf("read runs dir: %w", err)
	}
	store := RunStore{Root: root}
	items := make([]contracts.RunSnapshot, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		runID := strings.TrimSuffix(entry.Name(), ".md")
		run, err := store.LoadRun(ctx, runID)
		if err != nil {
			recordDoctorIssue(report, addIssue, "run_doc_corrupt", "RunIssues")
			continue
		}
		items = append(items, run)
	}
	sortRuns(items)
	return items, nil
}

func auditGateDocs(ctx context.Context, root string, report *OrchestrationDoctorReport, addIssue func(string, *int)) ([]contracts.GateSnapshot, error) {
	entries, err := os.ReadDir(storage.GatesDir(root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.GateSnapshot{}, nil
		}
		return nil, fmt.Errorf("read gates dir: %w", err)
	}
	store := GateStore{Root: root}
	items := make([]contracts.GateSnapshot, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		gateID := strings.TrimSuffix(entry.Name(), ".md")
		gate, err := store.LoadGate(ctx, gateID)
		if err != nil {
			recordDoctorIssue(report, addIssue, "gate_doc_corrupt", "GateIssues")
			continue
		}
		items = append(items, gate)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].GateID < items[j].GateID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items, nil
}

func auditHandoffDocs(ctx context.Context, root string, report *OrchestrationDoctorReport, addIssue func(string, *int)) ([]contracts.HandoffPacket, error) {
	entries, err := os.ReadDir(storage.HandoffsDir(root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.HandoffPacket{}, nil
		}
		return nil, fmt.Errorf("read handoffs dir: %w", err)
	}
	store := HandoffStore{Root: root}
	items := make([]contracts.HandoffPacket, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		handoffID := strings.TrimSuffix(entry.Name(), ".md")
		handoff, err := store.LoadHandoff(ctx, handoffID)
		if err != nil {
			recordDoctorIssue(report, addIssue, "handoff_doc_corrupt", "HandoffIssues")
			continue
		}
		items = append(items, handoff)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].GeneratedAt.Equal(items[j].GeneratedAt) {
			return items[i].HandoffID < items[j].HandoffID
		}
		return items[i].GeneratedAt.Before(items[j].GeneratedAt)
	})
	return items, nil
}

func auditArchiveDocs(ctx context.Context, root string, report *OrchestrationDoctorReport, addIssue func(string, *int)) ([]contracts.ArchiveRecord, error) {
	entries, err := os.ReadDir(storage.ArchivesDir(root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.ArchiveRecord{}, nil
		}
		return nil, fmt.Errorf("read archives dir: %w", err)
	}
	store := ArchiveRecordStore{Root: root}
	items := make([]contracts.ArchiveRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		record, err := store.LoadArchiveRecord(ctx, strings.TrimSuffix(entry.Name(), ".md"))
		if err != nil {
			recordDoctorIssue(report, addIssue, "archive_doc_corrupt", "ArchiveIssues")
			continue
		}
		items = append(items, record)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ArchiveID < items[j].ArchiveID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items, nil
}

func auditChangeDocs(ctx context.Context, root string, report *OrchestrationDoctorReport, addIssue func(string, *int)) ([]contracts.ChangeRef, error) {
	entries, err := os.ReadDir(storage.ChangesDir(root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.ChangeRef{}, nil
		}
		return nil, fmt.Errorf("read changes dir: %w", err)
	}
	store := ChangeStore{Root: root}
	items := make([]contracts.ChangeRef, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		change, err := store.LoadChange(ctx, strings.TrimSuffix(entry.Name(), ".md"))
		if err != nil {
			recordDoctorIssue(report, addIssue, "change_doc_corrupt", "ChangeIssues")
			continue
		}
		items = append(items, change)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ChangeID < items[j].ChangeID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items, nil
}

func auditCheckDocs(ctx context.Context, root string, report *OrchestrationDoctorReport, addIssue func(string, *int)) ([]contracts.CheckResult, error) {
	entries, err := os.ReadDir(storage.ChecksDir(root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.CheckResult{}, nil
		}
		return nil, fmt.Errorf("read checks dir: %w", err)
	}
	store := CheckStore{Root: root}
	items := make([]contracts.CheckResult, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		check, err := store.LoadCheck(ctx, strings.TrimSuffix(entry.Name(), ".md"))
		if err != nil {
			recordDoctorIssue(report, addIssue, "check_doc_corrupt", "CheckIssues")
			continue
		}
		items = append(items, check)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].CheckID < items[j].CheckID
		}
		return items[i].UpdatedAt.Before(items[j].UpdatedAt)
	})
	return items, nil
}

func auditEvidenceDocs(ctx context.Context, root string, runID string, report *OrchestrationDoctorReport, addIssue func(string, *int)) ([]contracts.EvidenceItem, error) {
	entries, err := os.ReadDir(storage.EvidenceDir(root, runID))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.EvidenceItem{}, nil
		}
		return nil, fmt.Errorf("read evidence dir: %w", err)
	}
	store := EvidenceStore{Root: root}
	items := make([]contracts.EvidenceItem, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		evidenceID := strings.TrimSuffix(entry.Name(), ".md")
		item, err := store.LoadEvidenceForRun(ctx, runID, evidenceID)
		if err != nil {
			recordDoctorIssue(report, addIssue, "evidence_doc_corrupt", "EvidenceIssues")
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].EvidenceID < items[j].EvidenceID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items, nil
}

func recordDoctorIssue(report *OrchestrationDoctorReport, addIssue func(string, *int), code string, bucket string) {
	if report == nil || addIssue == nil {
		return
	}
	switch bucket {
	case "RunIssues":
		addIssue(code, &report.RunIssues)
	case "GateIssues":
		addIssue(code, &report.GateIssues)
	case "HandoffIssues":
		addIssue(code, &report.HandoffIssues)
	case "EvidenceIssues":
		addIssue(code, &report.EvidenceIssues)
	case "RuntimeIssues":
		addIssue(code, &report.RuntimeIssues)
	case "WorktreeIssues":
		addIssue(code, &report.WorktreeIssues)
	case "ArchiveIssues":
		addIssue(code, &report.ArchiveIssues)
	case "ChangeIssues":
		addIssue(code, &report.ChangeIssues)
	case "CheckIssues":
		addIssue(code, &report.CheckIssues)
	}
}

func auditRuntimeForRun(root string, run contracts.RunSnapshot, archivedPaths map[string]struct{}, report *OrchestrationDoctorReport, addIssue func(string, *int)) error {
	runtimeRel := filepath.Clean(filepath.Join(storage.TrackerDirName, "runtime", run.RunID))
	runtimeDir := filepath.Join(root, runtimeRel)
	info, err := os.Stat(runtimeDir)
	runtimeExists := err == nil && info.IsDir()
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat runtime dir for %s: %w", run.RunID, err)
	}
	_, archived := archivedPaths[runtimeRel]
	if run.Status == contracts.RunStatusCleanedUp {
		if runtimeExists && !archived {
			addIssue("runtime_dir_stale", &report.RuntimeIssues)
		}
	} else if !runtimeExists && !archived {
		addIssue("runtime_dir_missing", &report.RuntimeIssues)
	}
	if runtimeExists {
		briefExists, err := regularFileExists(storage.RuntimeBriefFile(root, run.RunID))
		if err != nil {
			return err
		}
		contextExists, err := regularFileExists(storage.RuntimeContextFile(root, run.RunID))
		if err != nil {
			return err
		}
		codexLaunchExists, err := regularFileExists(storage.RuntimeLaunchFile(root, run.RunID, "codex"))
		if err != nil {
			return err
		}
		claudeLaunchExists, err := regularFileExists(storage.RuntimeLaunchFile(root, run.RunID, "claude"))
		if err != nil {
			return err
		}

		requiredSeen := 0
		if briefExists {
			requiredSeen++
		}
		if contextExists {
			requiredSeen++
		}
		launchSeen := 0
		if codexLaunchExists {
			launchSeen++
		}
		if claudeLaunchExists {
			launchSeen++
		}

		// brief/context are the restoreable runtime core; launch files are derived
		// and can be compacted away cleanly as long as they disappear together.
		if requiredSeen == 0 || requiredSeen == 1 || (requiredSeen == 2 && launchSeen == 1) || (requiredSeen == 0 && launchSeen > 0) {
			addIssue("runtime_artifacts_partial", &report.RuntimeIssues)
		}
	}
	return nil
}

func auditWorktreeForRun(ctx context.Context, worktrees WorktreeManager, run contracts.RunSnapshot, report *OrchestrationDoctorReport, addIssue func(string, *int)) error {
	if strings.TrimSpace(run.WorktreePath) == "" {
		return nil
	}
	status, err := worktrees.Inspect(ctx, run)
	if err != nil {
		return err
	}
	if run.Status == contracts.RunStatusCleanedUp {
		if status.Present {
			addIssue("worktree_stale", &report.WorktreeIssues)
		}
		return nil
	}
	if !status.Present {
		addIssue("worktree_missing", &report.WorktreeIssues)
	}
	return nil
}

func auditEvidenceDirs(root string, runsByID map[string]contracts.RunSnapshot, archivedPaths map[string]struct{}, report *OrchestrationDoctorReport, addIssue func(string, *int)) error {
	entries, err := os.ReadDir(filepath.Join(storage.TrackerDir(root), "evidence"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read evidence dir: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, ok := runsByID[entry.Name()]; !ok {
			addIssue("evidence_dir_orphaned", &report.EvidenceIssues)
		}
	}
	runtimeEntries, err := os.ReadDir(filepath.Join(storage.TrackerDir(root), "runtime"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read runtime dir: %w", err)
	}
	for _, entry := range runtimeEntries {
		if !entry.IsDir() {
			continue
		}
		rel := filepath.Clean(filepath.Join(storage.TrackerDirName, "runtime", entry.Name()))
		if _, archived := archivedPaths[rel]; archived {
			continue
		}
		if _, ok := runsByID[entry.Name()]; !ok {
			addIssue("runtime_dir_orphaned", &report.RuntimeIssues)
		}
	}
	return nil
}

func auditEvidenceArtifact(root string, item contracts.EvidenceItem, archivedPaths map[string]struct{}) string {
	path := strings.TrimSpace(item.ArtifactPath)
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		return "evidence_artifact_invalid"
	}
	rel, ok := relativeWorkspacePath(root, path)
	if !ok {
		return "evidence_artifact_invalid"
	}
	expectedPrefix := filepath.Clean(filepath.Join(storage.TrackerDirName, "evidence", item.RunID))
	if rel != expectedPrefix && !strings.HasPrefix(rel, expectedPrefix+string(os.PathSeparator)) {
		return "evidence_artifact_invalid"
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			if _, archived := archivedPaths[rel]; archived {
				return ""
			}
			return "evidence_artifact_missing"
		}
		return "evidence_artifact_invalid"
	}
	if info.IsDir() {
		return "evidence_artifact_invalid"
	}
	return ""
}

func retentionOverdueCodes(ctx context.Context, root string, tickets []contracts.TicketSnapshot) []string {
	queries := NewQueryService(root, nil, nil, nil, nil, timeNowUTC)
	codes := map[string]struct{}{}
	projects := map[string]struct{}{}
	for _, ticket := range tickets {
		projects[ticket.Project] = struct{}{}
	}
	for _, target := range []contracts.RetentionTarget{contracts.RetentionTargetRuntime, contracts.RetentionTargetEvidenceArtifacts, contracts.RetentionTargetExportBundles, contracts.RetentionTargetLogs} {
		plan, err := queries.ArchivePlan(ctx, target, "")
		if err == nil && len(plan.Items) > 0 {
			codes["retention_overdue"] = struct{}{}
			break
		}
		for projectKey := range projects {
			plan, err := queries.ArchivePlan(ctx, target, projectKey)
			if err == nil && len(plan.Items) > 0 {
				codes["retention_overdue"] = struct{}{}
				break
			}
		}
	}
	return sortedIssueCodes(codes)
}

func archivePathState(records []contracts.ArchiveRecord, state contracts.ArchiveRecordState) map[string]struct{} {
	paths := map[string]struct{}{}
	for _, record := range records {
		if record.State != state {
			continue
		}
		for _, path := range record.SourcePaths {
			paths[filepath.Clean(path)] = struct{}{}
		}
	}
	return paths
}

func pathWithinDir(dir string, path string) bool {
	dir = canonicalComparablePath(dir)
	path = canonicalComparablePath(path)
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func regularFileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat %s: %w", filepath.Base(path), err)
	}
	return !info.IsDir(), nil
}

func sortedIssueCodes(codes map[string]struct{}) []string {
	items := make([]string, 0, len(codes))
	for code := range codes {
		items = append(items, code)
	}
	sort.Strings(items)
	return items
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func canonicalComparablePath(path string) string {
	path = filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	parent := filepath.Dir(path)
	if resolvedParent, err := filepath.EvalSymlinks(parent); err == nil {
		return filepath.Join(resolvedParent, filepath.Base(path))
	}
	return path
}
