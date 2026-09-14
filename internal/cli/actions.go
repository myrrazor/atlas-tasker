package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/app"
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

// noticeOut is where one-line operational notices go (the index rebuild
// message, for now). Execute points it at the caller's stderr so tests can
// read it; outside Execute it is the process stderr.
var noticeOut io.Writer = os.Stderr

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

type openOptions struct {
	// skipIndexFreshness: the caller is about to rebuild anyway (reindex), so
	// don't do it twice
	skipIndexFreshness bool
}

func openWorkspace() (*workspace, error) {
	return openWorkspaceWith(openOptions{})
}

func openWorkspaceWith(opts openOptions) (*workspace, error) {
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	root, err = service.CanonicalWorkspaceRoot(root)
	if err != nil {
		return nil, err
	}
	if err := requireInitializedWorkspace(root); err != nil {
		return nil, err
	}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: defaultNow}
	eventLog := &eventstore.Log{RootDir: root}
	indexPath := filepath.Join(storage.TrackerDir(root), "index.sqlite")
	projection, err := sqlitestore.Open(indexPath, ticketStore, eventLog)

	if err != nil {
		if sqlitestore.IsCorrupt(err) {
			return nil, apperr.Wrap(apperr.CodeRepairNeeded, err, "ticket index is unreadable; run 'tracker doctor --repair' to rebuild it")
		}
		return nil, err
	}
	// every write stamps the source fingerprint through this, so it has to be
	// set even when the freshness check below is skipped
	projection.Root = root
	locks := service.FileLockManager{Root: root}
	if !opts.skipIndexFreshness {
		if _, err := service.EnsureFreshProjection(context.Background(), root, locks, projection, noticeOut); err != nil {
			_ = projection.Close()
			return nil, err
		}
	}
	projectStore := mdstore.ProjectStore{RootDir: root}
	cfg, err := config.Load(root)
	if err != nil {
		_ = projection.Close()
		return nil, err
	}
	w := &workspace{
		root:       root,
		project:    projectStore,
		ticket:     ticketStore,
		events:     eventLog,
		projection: projection,
		locks:      locks,
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
	home, _ := os.UserHomeDir()
	service.AttachUserState(w.actions, w.queries, home, "")
	return w, nil
}

// init and integrations install bootstrap explicitly; every other workspace
// opener shares the same side-effect-free root validation with MCP and the TUI.
func requireInitializedWorkspace(root string) error {
	_, err := service.InitializedWorkspaceRoot(root)
	return err
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

func refuseNestedWorkspaceInit(root string) error {
	return app.RefuseNestedWorkspaceInit(root)
}

func ensureInitArtifacts(root string) (initResult, error) {
	res, err := app.ScaffoldWorkspace(root, app.ScaffoldOptions{Now: defaultNow, GitMode: app.GitModeShared})
	if err != nil {
		return initResult{}, err
	}
	return initResult{Kind: res.Kind, Workspace: res.Root, Created: res.Created}, nil
}

func isMissing(path string) (bool, error) {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return true, nil
	}
	return false, err
}

const (
	workspaceGitignoreBegin = app.ManagedGitignoreBegin
	workspaceGitignoreEnd   = app.ManagedGitignoreEnd
)

func workspaceGitignoreBlock() string {
	return strings.Join([]string{
		workspaceGitignoreBegin,
		"# Local-only Atlas paths (not ticket markdown under projects/).",
		"/.tracker/mutations/",
		"/.tracker/runtime/",
		"/.tracker/*.log",
		"/.tracker/sync/mirror/",
		"/.tracker/sync/staging/",
		"/.tracker/sync/bundles/",
		"/.tracker/archives/*",
		"!/.tracker/archives/*.md",
		"/.tracker/exports/*",
		"!/.tracker/exports/*.md",
		"/.tracker/security/keys/private/",
		"/.tracker/security/trust/",
		"/.tracker/redaction/previews/",
		"/.tracker/backups/snapshots/",
		"/.tracker/goal/",
		"/.tracker/evidence/**",
		"!/.tracker/evidence/",
		"!/.tracker/evidence/*/",
		"!/.tracker/evidence/**/*.md",
		"/.tracker/index.sqlite",
		"/.tracker/index.sqlite-*",
		"/.tracker/write.lock",
		workspaceGitignoreEnd,
		"",
	}, "\n")
}

// ensureWorkspaceGitignore writes or refreshes the managed local-ignore block.
// Returns true when the file was created or the managed block changed.
func ensureWorkspaceGitignore(root string) (bool, error) {
	path := filepath.Join(root, ".gitignore")
	block := workspaceGitignoreBlock()
	current, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	body := string(current)
	begin := strings.Index(body, workspaceGitignoreBegin)
	end := strings.Index(body, workspaceGitignoreEnd)
	if begin >= 0 && end > begin {
		end += len(workspaceGitignoreEnd)
		if end < len(body) && body[end] == '\n' {
			end++
		}
		updated := body[:begin] + block + body[end:]
		if updated == body {
			return false, nil
		}
		if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
			return false, err
		}
		return true, nil
	}
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	if body != "" {
		body += "\n"
	}
	body += block
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return false, err
	}
	return true, nil
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
	return contracts.Actor(strings.TrimSpace(raw))
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
