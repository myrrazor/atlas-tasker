package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/render"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
	"github.com/spf13/cobra"
)

const jsonFormatVersion = "v1"

type workspace struct {
	root       string
	project    mdstore.ProjectStore
	ticket     mdstore.TicketStore
	events     *eventstore.Log
	projection *sqlitestore.Store
	locks      service.WriteLockManager
	actions    *service.ActionService
	queries    *service.QueryService
}

func openWorkspace() (*workspace, error) {
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	root, err = service.CanonicalWorkspaceRoot(root)
	if err != nil {
		return nil, err
	}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: defaultNow}
	eventLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventLog)
	if err != nil {
		if sqlitestore.IsCorrupt(err) {
			return nil, apperr.Wrap(apperr.CodeRepairNeeded, err, "ticket index is unreadable; run 'tracker doctor --repair' to rebuild it")
		}
		return nil, err
	}
	projectStore := mdstore.ProjectStore{RootDir: root}
	cfg, err := config.Load(root)
	if err != nil {
		return nil, err
	}
	w := &workspace{
		root:       root,
		project:    projectStore,
		ticket:     ticketStore,
		events:     eventLog,
		projection: projection,
		locks:      service.FileLockManager{Root: root},
	}
	w.queries = service.NewQueryService(root, projectStore, ticketStore, eventLog, projection, defaultNow)
	notifier, err := service.BuildNotifier(root, cfg, os.Stderr, service.SubscriptionResolver{
		Store:   service.SubscriptionStore{Root: root},
		Queries: w.queries,
	})
	if err != nil {
		return nil, err
	}
	automation := &service.AutomationEngine{
		Store:    service.AutomationStore{Root: root},
		Notifier: notifier,
	}
	w.actions = service.NewActionService(root, projectStore, ticketStore, eventLog, projection, defaultNow, w.locks, notifier, automation)
	return w, nil
}

func (w *workspace) close() {
	if w.projection != nil {
		_ = w.projection.Close()
	}
}

func (w *workspace) withWriteLock(ctx context.Context, purpose string, fn func(context.Context) error) error {
	return service.WithWriteLock(ctx, w.locks, purpose, fn)
}

func (w *workspace) nextEventID(ctx context.Context, project string) (int64, error) {
	return w.actions.NextEventID(ctx, project)
}

func (w *workspace) appendAndProject(ctx context.Context, event contracts.Event) error {
	return w.actions.AppendAndProject(ctx, event)
}

func writeCommandOutput(cmd *cobra.Command, data any, markdown string, pretty string) error {
	jsonMode, _ := cmd.Flags().GetBool("json")
	mdMode, _ := cmd.Flags().GetBool("md")
	if jsonMode {
		payload, err := versionedJSONPayload(data)
		if err != nil {
			return err
		}
		raw, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(raw))
		return nil
	}
	if mdMode {
		fmt.Fprintln(cmd.OutOrStdout(), render.SanitizeTerminalOutput(markdown))
		return nil
	}
	fmt.Fprintln(cmd.OutOrStdout(), render.SanitizeTerminalOutput(pretty))
	return nil
}

func parseLabels(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	parts := strings.Split(raw, ",")
	labels := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			labels = append(labels, trimmed)
		}
	}
	return labels
}

func listTicketEvents(ctx context.Context, w *workspace, ticketID string) ([]contracts.Event, error) {
	events, err := w.events.StreamEvents(ctx, "", 0)
	if err != nil {
		return nil, err
	}
	filtered := make([]contracts.Event, 0)
	for _, event := range events {
		if event.TicketID == ticketID {
			filtered = append(filtered, event)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].EventID == filtered[j].EventID {
			return filtered[i].Timestamp.Before(filtered[j].Timestamp)
		}
		return filtered[i].EventID < filtered[j].EventID
	})
	return filtered, nil
}

func defaultNow() time.Time {
	return time.Now().UTC()
}

// initResult is what `tracker init` reports back: where the workspace landed and
// what scaffolding this run had to lay down. A second init reports an empty
// Created, which is how a script tells "already bootstrapped" from "just
// bootstrapped". Derived state the first read builds — the sqlite index, the sync
// subtree — is not listed; it is rebuildable and not worth reporting.
type initResult struct {
	Kind      string   `json:"kind"`
	Workspace string   `json:"workspace"`
	Created   []string `json:"created"`
}

func ensureInitArtifacts(root string) (initResult, error) {
	root, err := service.CanonicalWorkspaceRoot(root)
	if err != nil {
		return initResult{}, err
	}
	result := initResult{Kind: "workspace_init", Workspace: root, Created: []string{}}
	trackerDir := storage.TrackerDir(root)
	for _, dir := range []string{
		trackerDir,
		storage.EventsDir(root),
		storage.AutomationsDir(root),
		storage.ViewsDir(root),
		storage.SubscriptionsDir(root),
		storage.AgentsDir(root),
		storage.RunbooksDir(root),
		storage.RunsDir(root),
		storage.GatesDir(root),
		storage.ChangesDir(root),
		storage.ChecksDir(root),
		storage.PermissionProfilesDir(root),
		storage.HandoffsDir(root),
		storage.ImportsDir(root),
		storage.ExportsDir(root),
		storage.RetentionPoliciesDir(root),
		storage.ArchivesDir(root),
		filepath.Join(trackerDir, "evidence"),
		filepath.Join(trackerDir, "runtime"),
		filepath.Join(trackerDir, "templates"),
		storage.ProjectsDir(root),
	} {
		missing, err := isMissing(dir)
		if err != nil {
			return initResult{}, err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return initResult{}, err
		}
		if missing {
			result.Created = append(result.Created, relativeToRoot(root, dir))
		}
	}
	identityMissing, err := isMissing(storage.WorkspaceMetadataFile(root))
	if err != nil {
		return initResult{}, err
	}
	if _, err := service.EnsureWorkspaceIdentityForCLI(root); err != nil {
		return initResult{}, err
	}
	if identityMissing {
		result.Created = append(result.Created, relativeToRoot(root, storage.WorkspaceMetadataFile(root)))
	}
	configMissing, err := isMissing(config.Path(root))
	if err != nil {
		return initResult{}, err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return initResult{}, err
	}
	if err := config.Save(root, cfg); err != nil {
		return initResult{}, err
	}
	if configMissing {
		result.Created = append(result.Created, relativeToRoot(root, config.Path(root)))
	}
	monthFile := filepath.Join(storage.EventsDir(root), defaultNow().Format("2006-01")+".jsonl")
	if missing, err := isMissing(monthFile); err != nil {
		return initResult{}, err
	} else if missing {
		if err := os.WriteFile(monthFile, []byte(""), 0o644); err != nil {
			return initResult{}, err
		}
		result.Created = append(result.Created, relativeToRoot(root, monthFile))
	}
	templates := map[string]string{
		"epic.md": `---
type: epic
blueprint: design
---
# Summary

## Description

Shape the full slice before you break it into child work.

## Acceptance Criteria
- Scope is clear
- Child tickets can be created from this epic
`,
		"task.md": `---
type: task
blueprint: implement
---
# Summary

## Description

Implement the scoped change.

## Acceptance Criteria
- Code is merged locally
- Tests cover the new behavior
`,
		"bug.md": `---
type: bug
blueprint: qa
---
# Summary

## Description

Describe the broken behavior and the expected fix.

## Acceptance Criteria
- Repro is documented
- Fix is verified
`,
		"subtask.md": `---
type: subtask
blueprint: implement
---
# Summary

## Description

Small child task under a parent item.

## Acceptance Criteria
- Parent stays up to date
`,
		"design.md": `---
type: task
labels:
  - design
blueprint: design
skill_hint: design
---
# Summary

## Description

Capture the UX, constraints, and acceptance shape before implementation.

## Acceptance Criteria
- Design direction is written down
- Open questions are resolved or tracked
`,
		"implement.md": `---
type: task
labels:
  - implementation
blueprint: implement
skill_hint: implement
---
# Summary

## Description

Build the scoped change and keep the diff reviewable.

## Acceptance Criteria
- Behavior works locally
- Tests are updated
`,
		"review.md": `---
type: task
labels:
  - review
blueprint: review
skill_hint: review
---
# Summary

## Description

Audit the implementation for regressions and missing tests.

## Acceptance Criteria
- Findings are documented
- Blocking issues are fixed or tracked
`,
		"qa.md": `---
type: task
labels:
  - qa
blueprint: qa
skill_hint: qa
---
# Summary

## Description

Run end-to-end validation and record the results.

## Acceptance Criteria
- Happy path is verified
- Edge cases are covered
`,
		"spike.md": `---
type: task
labels:
  - spike
blueprint: spike
skill_hint: spike
---
# Summary

## Description

Time-boxed investigation with explicit follow-up output.

## Acceptance Criteria
- Findings are written down
- Next steps are clear
`,
	}
	names := make([]string, 0, len(templates))
	for name := range templates {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(trackerDir, "templates", name)
		missing, err := isMissing(path)
		if err != nil {
			return initResult{}, err
		}
		if !missing {
			continue
		}
		if err := os.WriteFile(path, []byte(templates[name]), 0o644); err != nil {
			return initResult{}, err
		}
		result.Created = append(result.Created, relativeToRoot(root, path))
	}
	return result, nil
}

func isMissing(path string) (bool, error) {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return true, nil
	}
	return false, err
}

func relativeToRoot(root string, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}

func loadTicketsMap(ctx context.Context, w *workspace) (map[string]contracts.TicketSnapshot, error) {
	tickets, err := w.ticket.ListTickets(ctx, contracts.TicketListOptions{IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	mapped := make(map[string]contracts.TicketSnapshot, len(tickets))
	for _, ticket := range tickets {
		mapped[ticket.ID] = ticket
	}
	return mapped, nil
}

func normalizeActor(raw string) contracts.Actor {
	actor := strings.TrimSpace(raw)
	if actor == "" {
		return contracts.Actor("human:owner")
	}
	return contracts.Actor(actor)
}

func versionedJSONPayload(data any) (any, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	switch value := decoded.(type) {
	case map[string]any:
		value["format_version"] = jsonFormatVersion
		return value, nil
	case []any:
		return map[string]any{
			"format_version": jsonFormatVersion,
			"items":          value,
		}, nil
	default:
		return map[string]any{
			"format_version": jsonFormatVersion,
			"value":          value,
		}, nil
	}
}
