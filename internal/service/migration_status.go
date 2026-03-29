package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	"gopkg.in/yaml.v3"
)

type migrationCounter struct {
	total     int
	stamped   int
	unstamped int
	divergent int
	unknown   int
	examples  []string
}

func (s *QueryService) MigrationStatus(ctx context.Context) (MigrationStatusView, error) {
	workspaceID, workspaceErr := loadWorkspaceIdentity(s.Root)
	state, exists, stateErr := loadSyncMigrationState(s.Root)
	entities, err := s.scanMigrationEntities(ctx)
	if err != nil {
		return MigrationStatusView{}, err
	}
	sort.Slice(entities, func(i, j int) bool {
		return entities[i].Kind < entities[j].Kind
	})

	view := MigrationStatusView{
		WorkspaceID: workspaceID,
		State:       aggregateMigrationState(entities),
		Entities:    entities,
		CheckedAt:   s.now(),
	}
	if exists {
		view.StampedAt = state.StampedAt
		view.SchemaVersion = state.SchemaVersion
	}
	if workspaceErr != nil {
		addMigrationDiagnostic(&view, "migration_unknown", fmt.Sprintf("workspace identity could not be read: %v", workspaceErr), "repair the workspace metadata file, then rerun doctor or sync status")
	}
	if stateErr != nil {
		addMigrationDiagnostic(&view, "migration_unknown", fmt.Sprintf("migration state could not be read: %v", stateErr), "repair the sync migration state file, then rerun doctor or sync status")
	}

	persistedComplete := exists && state.Complete
	if view.State == MigrationStateStamped && (!persistedComplete || strings.TrimSpace(workspaceID) == "") {
		view.State = MigrationStateUnstamped
	}
	if persistedComplete {
		switch view.State {
		case MigrationStateUnstamped:
			view.State = MigrationStateDivergent
		case MigrationStateStamped:
			switch {
			case strings.TrimSpace(workspaceID) == "":
				view.State = MigrationStateDivergent
			case strings.TrimSpace(state.WorkspaceID) == "":
				view.State = MigrationStateDivergent
			case strings.TrimSpace(state.WorkspaceID) != strings.TrimSpace(workspaceID):
				view.State = MigrationStateDivergent
			}
		}
	}

	switch view.State {
	case MigrationStateUnstamped:
		addMigrationDiagnostic(&view, "migration_incomplete", "workspace migration stamping has not completed yet", "run a sync-capable mutation in this upgraded workspace to finish deterministic stamping, then rerun sync status")
	case MigrationStateDivergent:
		addMigrationDiagnostic(&view, "migration_divergent", "deterministic migration checks found divergent canonical identities", "inspect the migration report, repair the affected records, and do not retry sync apply until doctor is clean")
	case MigrationStateUnknown:
		addMigrationDiagnostic(&view, "migration_unknown", "migration state could not be determined from the current workspace data", "run doctor --json, repair the unreadable records, and rerun the migration checks")
	}
	if persistedComplete && strings.TrimSpace(state.WorkspaceID) != "" && strings.TrimSpace(workspaceID) != "" && strings.TrimSpace(state.WorkspaceID) != strings.TrimSpace(workspaceID) {
		addMigrationDiagnostic(&view, "migration_divergent", "workspace identity does not match the recorded migration stamp", "restore the correct workspace metadata or restamp from a clean workspace copy before syncing")
	}
	view.Ready = view.State == MigrationStateStamped && persistedComplete && strings.TrimSpace(workspaceID) != ""
	return view, nil
}

func (s *ActionService) ensureSyncMigrationReady(ctx context.Context) error {
	view, err := NewQueryService(s.Root, s.Projects, s.Tickets, s.Events, s.Projection, s.Clock).MigrationStatus(ctx)
	if err != nil {
		return err
	}
	if view.Ready {
		return nil
	}
	switch view.State {
	case MigrationStateDivergent:
		return apperr.New(apperr.CodeRepairNeeded, "migration divergence detected; repair the workspace before retrying sync apply")
	case MigrationStateUnknown:
		return apperr.New(apperr.CodeRepairNeeded, "migration state is unknown; repair the workspace before retrying sync apply")
	default:
		return apperr.New(apperr.CodeConflict, "migration_incomplete: deterministic migration stamping has not completed yet")
	}
}

func restampLegacyEventFiles(root string) error {
	err := filepath.WalkDir(storage.EventsDir(root), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		lines := []string{}
		for scanner.Scan() {
			rawLine := strings.TrimSpace(scanner.Text())
			if rawLine == "" {
				continue
			}
			var event contracts.Event
			if err := json.Unmarshal([]byte(rawLine), &event); err != nil {
				return fmt.Errorf("decode legacy event in %s: %w", path, err)
			}
			event = contracts.NormalizeEvent(event)
			if err := event.Validate(); err != nil {
				return fmt.Errorf("validate legacy event in %s: %w", path, err)
			}
			normalized, err := json.Marshal(event)
			if err != nil {
				return fmt.Errorf("marshal legacy event in %s: %w", path, err)
			}
			lines = append(lines, string(normalized))
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("scan legacy event file %s: %w", path, err)
		}
		content := ""
		if len(lines) > 0 {
			content = strings.Join(lines, "\n") + "\n"
		}
		return os.WriteFile(path, []byte(content), 0o644)
	})
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func loadSyncMigrationState(root string) (syncMigrationState, bool, error) {
	raw, err := os.ReadFile(syncMigrationPath(root))
	if err != nil {
		if os.IsNotExist(err) {
			return syncMigrationState{}, false, nil
		}
		return syncMigrationState{}, false, fmt.Errorf("read sync migration state: %w", err)
	}
	var state syncMigrationState
	if err := json.Unmarshal(raw, &state); err != nil {
		return syncMigrationState{}, true, fmt.Errorf("decode sync migration state: %w", err)
	}
	return state, true, nil
}

func (s *QueryService) scanMigrationEntities(ctx context.Context) ([]MigrationEntityStatusView, error) {
	items := make([]MigrationEntityStatusView, 0, 12)

	add := func(view MigrationEntityStatusView, err error) error {
		if err != nil {
			return err
		}
		items = append(items, view)
		return nil
	}

	if err := add(scanTicketMigrationStatus(s.Root), nil); err != nil {
		return nil, err
	}
	if err := add(scanCollaboratorMigrationStatus(s.Root), nil); err != nil {
		return nil, err
	}
	if err := add(scanMembershipMigrationStatus(s.Root), nil); err != nil {
		return nil, err
	}
	if err := add(scanRunMigrationStatus(s.Root), nil); err != nil {
		return nil, err
	}
	if err := add(scanGateMigrationStatus(s.Root), nil); err != nil {
		return nil, err
	}
	if err := add(scanHandoffMigrationStatus(s.Root), nil); err != nil {
		return nil, err
	}
	if err := add(scanEvidenceMigrationStatus(s.Root), nil); err != nil {
		return nil, err
	}
	if err := add(scanChangeMigrationStatus(s.Root), nil); err != nil {
		return nil, err
	}
	if err := add(scanCheckMigrationStatus(s.Root), nil); err != nil {
		return nil, err
	}
	if err := add(scanImportJobMigrationStatus(s.Root), nil); err != nil {
		return nil, err
	}
	if err := add(scanExportBundleMigrationStatus(s.Root), nil); err != nil {
		return nil, err
	}
	if err := add(scanArchiveMigrationStatus(s.Root), nil); err != nil {
		return nil, err
	}
	events, err := scanEventMigrationStatus(ctx, s.Root, s.Events)
	if err != nil {
		return nil, err
	}
	items = append(items, events)
	return items, nil
}

func scanTicketMigrationStatus(root string) MigrationEntityStatusView {
	counter := migrationCounter{}
	_ = filepath.WalkDir(storage.ProjectsDir(root), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") || !strings.Contains(path, string(filepath.Separator)+"tickets"+string(filepath.Separator)) {
			return nil
		}
		fmRaw, sample, err := readMarkdownFrontmatter(path)
		if err != nil {
			counter.add(MigrationStateUnknown, strings.TrimSuffix(filepath.Base(path), ".md"))
			return nil
		}
		var fm ticketConflictFrontmatter
		if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
			counter.add(MigrationStateUnknown, sample)
			return nil
		}
		counter.classifyUIDField(MigrationStateStamped, yamlFieldPresent(fmRaw, "ticket_uid"), strings.TrimSpace(fm.TicketUID), contracts.TicketUID(fm.Project, fm.ID), sample)
		return nil
	})
	return counter.view("ticket")
}

func scanCollaboratorMigrationStatus(root string) MigrationEntityStatusView {
	counter := migrationCounter{}
	_ = filepath.WalkDir(storage.CollaboratorsDir(root), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		fmRaw, sample, err := readMarkdownFrontmatter(path)
		if err != nil {
			counter.add(MigrationStateUnknown, strings.TrimSuffix(filepath.Base(path), ".md"))
			return nil
		}
		var fm collaboratorFrontmatter
		if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
			counter.add(MigrationStateUnknown, sample)
			return nil
		}
		counter.total++
		actual := strings.TrimSpace(fm.CollaboratorID)
		expected := strings.TrimSuffix(filepath.Base(path), ".md")
		switch {
		case !yamlFieldPresent(fmRaw, "collaborator_id") || actual == "":
			counter.add(MigrationStateUnknown, sample)
		case actual != expected:
			counter.add(MigrationStateDivergent, sample)
		default:
			counter.add(MigrationStateStamped, sample)
		}
		return nil
	})
	return counter.view("collaborator")
}

func scanMembershipMigrationStatus(root string) MigrationEntityStatusView {
	counter := migrationCounter{}
	_ = filepath.WalkDir(storage.MembershipsDir(root), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		fmRaw, sample, err := readMarkdownFrontmatter(path)
		if err != nil {
			counter.add(MigrationStateUnknown, strings.TrimSuffix(filepath.Base(path), ".md"))
			return nil
		}
		var fm membershipFrontmatter
		if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
			counter.add(MigrationStateUnknown, sample)
			return nil
		}
		expected := contracts.MembershipUID(fm.CollaboratorID, fm.ScopeKind, fm.ScopeID, fm.Role)
		counter.classifyUIDField(MigrationStateUnstamped, yamlFieldPresent(fmRaw, "membership_uid"), strings.TrimSpace(fm.MembershipUID), expected, sample)
		return nil
	})
	return counter.view("membership")
}

func scanRunMigrationStatus(root string) MigrationEntityStatusView {
	counter := migrationCounter{}
	_ = filepath.WalkDir(storage.RunsDir(root), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		fmRaw, sample, err := readMarkdownFrontmatter(path)
		if err != nil {
			counter.add(MigrationStateUnknown, strings.TrimSuffix(filepath.Base(path), ".md"))
			return nil
		}
		var fm runFrontmatter
		if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
			counter.add(MigrationStateUnknown, sample)
			return nil
		}
		counter.classifyUIDField(MigrationStateUnstamped, yamlFieldPresent(fmRaw, "run_uid"), strings.TrimSpace(fm.RunUID), contracts.RunUID(fm.RunID), sample)
		return nil
	})
	return counter.view("run")
}

func scanGateMigrationStatus(root string) MigrationEntityStatusView {
	counter := migrationCounter{}
	_ = filepath.WalkDir(storage.GatesDir(root), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		fmRaw, sample, err := readMarkdownFrontmatter(path)
		if err != nil {
			counter.add(MigrationStateUnknown, strings.TrimSuffix(filepath.Base(path), ".md"))
			return nil
		}
		var fm gateFrontmatter
		if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
			counter.add(MigrationStateUnknown, sample)
			return nil
		}
		counter.classifyUIDField(MigrationStateUnstamped, yamlFieldPresent(fmRaw, "gate_uid"), strings.TrimSpace(fm.GateUID), contracts.GateUID(fm.GateID), sample)
		return nil
	})
	return counter.view("gate")
}

func scanHandoffMigrationStatus(root string) MigrationEntityStatusView {
	counter := migrationCounter{}
	_ = filepath.WalkDir(storage.HandoffsDir(root), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		fmRaw, sample, err := readMarkdownFrontmatter(path)
		if err != nil {
			counter.add(MigrationStateUnknown, strings.TrimSuffix(filepath.Base(path), ".md"))
			return nil
		}
		var fm handoffFrontmatter
		if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
			counter.add(MigrationStateUnknown, sample)
			return nil
		}
		counter.classifyUIDField(MigrationStateUnstamped, yamlFieldPresent(fmRaw, "handoff_uid"), strings.TrimSpace(fm.HandoffUID), contracts.HandoffUID(fm.HandoffID), sample)
		return nil
	})
	return counter.view("handoff")
}

func scanEvidenceMigrationStatus(root string) MigrationEntityStatusView {
	counter := migrationCounter{}
	_ = filepath.WalkDir(filepath.Join(storage.TrackerDir(root), "evidence"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		fmRaw, sample, err := readMarkdownFrontmatter(path)
		if err != nil {
			counter.add(MigrationStateUnknown, strings.TrimSuffix(filepath.Base(path), ".md"))
			return nil
		}
		var fm evidenceFrontmatter
		if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
			counter.add(MigrationStateUnknown, sample)
			return nil
		}
		counter.classifyUIDField(MigrationStateUnstamped, yamlFieldPresent(fmRaw, "evidence_uid"), strings.TrimSpace(fm.EvidenceUID), contracts.EvidenceUID(fm.RunID, fm.EvidenceID), sample)
		return nil
	})
	return counter.view("evidence")
}

func scanChangeMigrationStatus(root string) MigrationEntityStatusView {
	counter := migrationCounter{}
	_ = filepath.WalkDir(storage.ChangesDir(root), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		fmRaw, sample, err := readMarkdownFrontmatter(path)
		if err != nil {
			counter.add(MigrationStateUnknown, strings.TrimSuffix(filepath.Base(path), ".md"))
			return nil
		}
		var fm changeFrontmatter
		if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
			counter.add(MigrationStateUnknown, sample)
			return nil
		}
		counter.classifyUIDField(MigrationStateUnstamped, yamlFieldPresent(fmRaw, "change_uid"), strings.TrimSpace(fm.ChangeUID), contracts.ChangeUID(fm.ChangeID), sample)
		return nil
	})
	return counter.view("change")
}

func scanCheckMigrationStatus(root string) MigrationEntityStatusView {
	counter := migrationCounter{}
	_ = filepath.WalkDir(storage.ChecksDir(root), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		fmRaw, sample, err := readMarkdownFrontmatter(path)
		if err != nil {
			counter.add(MigrationStateUnknown, strings.TrimSuffix(filepath.Base(path), ".md"))
			return nil
		}
		var fm checkFrontmatter
		if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
			counter.add(MigrationStateUnknown, sample)
			return nil
		}
		counter.classifyUIDField(MigrationStateUnstamped, yamlFieldPresent(fmRaw, "check_uid"), strings.TrimSpace(fm.CheckUID), contracts.CheckUID(fm.CheckID), sample)
		return nil
	})
	return counter.view("check")
}

func scanImportJobMigrationStatus(root string) MigrationEntityStatusView {
	counter := migrationCounter{}
	_ = filepath.WalkDir(storage.ImportsDir(root), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		fmRaw, sample, err := readMarkdownFrontmatter(path)
		if err != nil {
			counter.add(MigrationStateUnknown, strings.TrimSuffix(filepath.Base(path), ".md"))
			return nil
		}
		var fm importJobFrontmatter
		if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
			counter.add(MigrationStateUnknown, sample)
			return nil
		}
		counter.classifyUIDField(MigrationStateUnstamped, yamlFieldPresent(fmRaw, "import_job_uid"), strings.TrimSpace(fm.ImportJobUID), contracts.ImportJobUID(fm.JobID), sample)
		return nil
	})
	return counter.view("import_job")
}

func scanExportBundleMigrationStatus(root string) MigrationEntityStatusView {
	counter := migrationCounter{}
	_ = filepath.WalkDir(storage.ExportsDir(root), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		fmRaw, sample, err := readMarkdownFrontmatter(path)
		if err != nil {
			counter.add(MigrationStateUnknown, strings.TrimSuffix(filepath.Base(path), ".md"))
			return nil
		}
		var fm exportBundleFrontmatter
		if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
			counter.add(MigrationStateUnknown, sample)
			return nil
		}
		counter.classifyUIDField(MigrationStateUnstamped, yamlFieldPresent(fmRaw, "export_bundle_uid"), strings.TrimSpace(fm.ExportBundleUID), contracts.ExportBundleUID(fm.BundleID), sample)
		return nil
	})
	return counter.view("export_bundle")
}

func scanArchiveMigrationStatus(root string) MigrationEntityStatusView {
	counter := migrationCounter{}
	_ = filepath.WalkDir(storage.ArchivesDir(root), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		fmRaw, sample, err := readMarkdownFrontmatter(path)
		if err != nil {
			counter.add(MigrationStateUnknown, strings.TrimSuffix(filepath.Base(path), ".md"))
			return nil
		}
		var fm archiveRecordFrontmatter
		if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
			counter.add(MigrationStateUnknown, sample)
			return nil
		}
		counter.classifyUIDField(MigrationStateUnstamped, yamlFieldPresent(fmRaw, "archive_record_uid"), strings.TrimSpace(fm.ArchiveRecordUID), contracts.ArchiveRecordUID(fm.ArchiveID), sample)
		return nil
	})
	return counter.view("archive_record")
}

func scanEventMigrationStatus(_ context.Context, root string, _ contracts.EventLog) (MigrationEntityStatusView, error) {
	counter := migrationCounter{}
	err := filepath.WalkDir(storage.EventsDir(root), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			rawLine := strings.TrimSpace(scanner.Text())
			if rawLine == "" {
				continue
			}
			sample := fmt.Sprintf("%s:%d", filepath.Base(path), lineNo)
			var rawMap map[string]json.RawMessage
			if err := json.Unmarshal([]byte(rawLine), &rawMap); err != nil {
				counter.add(MigrationStateUnknown, sample)
				continue
			}
			var event contracts.Event
			if err := json.Unmarshal([]byte(rawLine), &event); err != nil {
				counter.add(MigrationStateUnknown, sample)
				continue
			}
			if err := contracts.NormalizeEvent(event).Validate(); err != nil {
				counter.add(MigrationStateUnknown, sample)
				continue
			}
			actual := strings.TrimSpace(event.EventUID)
			counter.classifyUIDField(MigrationStateUnstamped, jsonFieldPresent(rawMap, "event_uid"), actual, contracts.CanonicalEventUID(event), sample)
		}
		return scanner.Err()
	})
	if err != nil && !os.IsNotExist(err) {
		return MigrationEntityStatusView{}, err
	}
	return counter.view("event"), nil
}

func aggregateMigrationState(entities []MigrationEntityStatusView) MigrationState {
	state := MigrationStateStamped
	for _, entity := range entities {
		switch entity.State {
		case MigrationStateDivergent:
			return MigrationStateDivergent
		case MigrationStateUnknown:
			state = MigrationStateUnknown
		case MigrationStateUnstamped:
			if state == MigrationStateStamped {
				state = MigrationStateUnstamped
			}
		}
	}
	return state
}

func addMigrationDiagnostic(view *MigrationStatusView, code string, message string, nextStep string) {
	code = strings.TrimSpace(code)
	if code == "" {
		return
	}
	for _, existing := range view.Diagnostics {
		if existing.Code == code {
			return
		}
	}
	view.Diagnostics = append(view.Diagnostics, MigrationDiagnosticView{
		Code:     code,
		Message:  strings.TrimSpace(message),
		NextStep: strings.TrimSpace(nextStep),
	})
	view.ReasonCodes = append(view.ReasonCodes, code)
}

func readMarkdownFrontmatter(path string) (string, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	fmRaw, _, err := splitDocument(string(raw))
	if err != nil {
		return "", "", err
	}
	return fmRaw, strings.TrimSuffix(filepath.Base(path), ".md"), nil
}

func yamlFieldPresent(frontmatter string, field string) bool {
	prefix := strings.TrimSpace(field) + ":"
	for _, line := range strings.Split(frontmatter, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			return true
		}
	}
	return false
}

func jsonFieldPresent(raw map[string]json.RawMessage, field string) bool {
	_, ok := raw[strings.TrimSpace(field)]
	return ok
}

func (c *migrationCounter) add(state MigrationState, sample string) {
	c.total++
	switch state {
	case MigrationStateStamped:
		c.stamped++
	case MigrationStateUnstamped:
		c.unstamped++
	case MigrationStateDivergent:
		c.divergent++
	default:
		c.unknown++
	}
	sample = strings.TrimSpace(sample)
	if sample != "" && state != MigrationStateStamped && len(c.examples) < 5 {
		c.examples = append(c.examples, sample)
	}
}

func (c *migrationCounter) classifyUIDField(unstampedState MigrationState, fieldPresent bool, actual string, expected string, sample string) {
	actual = strings.TrimSpace(actual)
	expected = strings.TrimSpace(expected)
	switch {
	case expected == "":
		c.add(MigrationStateUnknown, sample)
	case !fieldPresent || actual == "":
		c.add(unstampedState, sample)
	case actual != expected:
		c.add(MigrationStateDivergent, sample)
	default:
		c.add(MigrationStateStamped, sample)
	}
}

func (c migrationCounter) view(kind string) MigrationEntityStatusView {
	return MigrationEntityStatusView{
		Kind:      kind,
		State:     aggregateMigrationState([]MigrationEntityStatusView{{State: c.state()}}),
		Total:     c.total,
		Stamped:   c.stamped,
		Unstamped: c.unstamped,
		Divergent: c.divergent,
		Unknown:   c.unknown,
		Examples:  append([]string{}, c.examples...),
	}
}

func (c migrationCounter) state() MigrationState {
	switch {
	case c.divergent > 0:
		return MigrationStateDivergent
	case c.unknown > 0:
		return MigrationStateUnknown
	case c.unstamped > 0:
		return MigrationStateUnstamped
	default:
		return MigrationStateStamped
	}
}
