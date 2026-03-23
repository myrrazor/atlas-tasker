package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
	"github.com/spf13/cobra"
)

type workspace struct {
	root       string
	project    mdstore.ProjectStore
	ticket     mdstore.TicketStore
	events     *eventstore.Log
	projection *sqlitestore.Store
	actions    *service.ActionService
	queries    *service.QueryService
}

func openWorkspace() (*workspace, error) {
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	ticketStore := mdstore.TicketStore{RootDir: root}
	eventLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventLog)
	if err != nil {
		return nil, err
	}
	projectStore := mdstore.ProjectStore{RootDir: root}
	w := &workspace{
		root:       root,
		project:    projectStore,
		ticket:     ticketStore,
		events:     eventLog,
		projection: projection,
	}
	w.actions = service.NewActionService(projectStore, ticketStore, eventLog, projection, defaultNow)
	w.queries = service.NewQueryService(root, projectStore, ticketStore, eventLog, projection, defaultNow)
	return w, nil
}

func (w *workspace) close() {
	if w.projection != nil {
		_ = w.projection.Close()
	}
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
		raw, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(raw))
		return nil
	}
	if mdMode {
		fmt.Fprintln(cmd.OutOrStdout(), markdown)
		return nil
	}
	fmt.Fprintln(cmd.OutOrStdout(), pretty)
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

func nextTicketID(project string, existing []contracts.TicketSnapshot) string {
	max := 0
	prefix := project + "-"
	for _, ticket := range existing {
		if !strings.HasPrefix(ticket.ID, prefix) {
			continue
		}
		raw := strings.TrimPrefix(ticket.ID, prefix)
		n, err := strconv.Atoi(raw)
		if err == nil && n > max {
			max = n
		}
	}
	return fmt.Sprintf("%s-%d", project, max+1)
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

func ensureInitArtifacts(root string) error {
	if err := os.MkdirAll(storage.TrackerDir(root), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.EventsDir(root), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(storage.TrackerDir(root), "templates"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.ProjectsDir(root), 0o755); err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	if err := config.Save(root, cfg); err != nil {
		return err
	}
	monthFile := filepath.Join(storage.EventsDir(root), defaultNow().Format("2006-01")+".jsonl")
	if _, err := os.Stat(monthFile); os.IsNotExist(err) {
		if err := os.WriteFile(monthFile, []byte(""), 0o644); err != nil {
			return err
		}
	}
	templates := map[string]string{
		"epic.md":    "# Summary\n\n## Description\n\n## Acceptance Criteria\n\n## Notes\n",
		"task.md":    "# Summary\n\n## Description\n\n## Acceptance Criteria\n\n## Notes\n",
		"bug.md":     "# Summary\n\n## Description\n\n## Acceptance Criteria\n\n## Notes\n",
		"subtask.md": "# Summary\n\n## Description\n\n## Acceptance Criteria\n\n## Notes\n",
	}
	for name, body := range templates {
		path := filepath.Join(storage.TrackerDir(root), "templates", name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				return err
			}
		}
	}
	return nil
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
