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
	IssueCodes     []string `json:"issue_codes"`
}

func (r OrchestrationDoctorReport) TotalIssues() int {
	return r.RunIssues + r.GateIssues + r.HandoffIssues + r.EvidenceIssues + r.RuntimeIssues + r.WorktreeIssues
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

	ticketsByID := make(map[string]contracts.TicketSnapshot, len(allTickets))
	for _, ticket := range allTickets {
		ticketsByID[ticket.ID] = ticket
	}
	runsByID := make(map[string]contracts.RunSnapshot, len(runs))
	worktrees := WorktreeManager{Root: root}
	for _, run := range runs {
		runsByID[run.RunID] = run
		if err := run.Validate(); err != nil {
			addIssue("run_invalid", &report.RunIssues)
		}
		ticket, ok := ticketsByID[run.TicketID]
		if !ok || ticket.Project != run.Project {
			addIssue("run_ticket_missing", &report.RunIssues)
		}
		if err := auditRuntimeForRun(root, run, &report, addIssue); err != nil {
			return OrchestrationDoctorReport{}, err
		}
		if err := auditWorktreeForRun(ctx, worktrees, run, &report, addIssue); err != nil {
			return OrchestrationDoctorReport{}, err
		}
	}

	evidenceByID := map[string]contracts.EvidenceItem{}
	if err := auditEvidenceDirs(root, runsByID, &report, addIssue); err != nil {
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
			if code := auditEvidenceArtifact(root, item); code != "" {
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
	// Keep repair conservative here: reconcile Git's worktree bookkeeping, but never recreate run sandboxes.
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
	}
}

func auditRuntimeForRun(root string, run contracts.RunSnapshot, report *OrchestrationDoctorReport, addIssue func(string, *int)) error {
	runtimeDir := storage.RuntimeDir(root, run.RunID)
	info, err := os.Stat(runtimeDir)
	runtimeExists := err == nil && info.IsDir()
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat runtime dir for %s: %w", run.RunID, err)
	}
	if run.Status == contracts.RunStatusCleanedUp {
		if runtimeExists {
			addIssue("runtime_dir_stale", &report.RuntimeIssues)
		}
	} else if !runtimeExists {
		addIssue("runtime_dir_missing", &report.RuntimeIssues)
	}
	if runtimeExists {
		seen := 0
		for _, path := range []string{
			storage.RuntimeBriefFile(root, run.RunID),
			storage.RuntimeContextFile(root, run.RunID),
			storage.RuntimeLaunchFile(root, run.RunID, "codex"),
			storage.RuntimeLaunchFile(root, run.RunID, "claude"),
		} {
			exists, statErr := regularFileExists(path)
			if statErr != nil {
				return statErr
			}
			if exists {
				seen++
			}
		}
		if seen > 0 && seen < 4 {
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

func auditEvidenceDirs(root string, runsByID map[string]contracts.RunSnapshot, report *OrchestrationDoctorReport, addIssue func(string, *int)) error {
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
		if _, ok := runsByID[entry.Name()]; !ok {
			addIssue("runtime_dir_orphaned", &report.RuntimeIssues)
		}
	}
	return nil
}

func auditEvidenceArtifact(root string, item contracts.EvidenceItem) string {
	path := strings.TrimSpace(item.ArtifactPath)
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		return "evidence_artifact_invalid"
	}
	expectedDir := storage.EvidenceDir(root, item.RunID)
	if !pathWithinDir(expectedDir, path) {
		return "evidence_artifact_invalid"
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "evidence_artifact_missing"
		}
		return "evidence_artifact_invalid"
	}
	if info.IsDir() {
		return "evidence_artifact_invalid"
	}
	return ""
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
