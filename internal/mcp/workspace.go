package mcp

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

type Workspace struct {
	Root       string
	Actions    *service.ActionService
	Queries    *service.QueryService
	Projection *sqlitestore.Store
	Locks      service.WriteLockManager
}

func OpenWorkspace(root string, stderr io.Writer, now func() time.Time) (*Workspace, error) {
	if strings.TrimSpace(root) == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	root, err := service.CanonicalWorkspaceRoot(root)
	if err != nil {
		return nil, err
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	if stderr == nil {
		stderr = io.Discard
	}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: now}
	eventLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventLog)
	if err != nil {
		return nil, err
	}
	projectStore := mdstore.ProjectStore{RootDir: root}
	cfg, err := config.Load(root)
	if err != nil {
		_ = projection.Close()
		return nil, err
	}
	w := &Workspace{
		Root:       root,
		Projection: projection,
		Locks:      service.FileLockManager{Root: root},
	}
	w.Queries = service.NewQueryService(root, projectStore, ticketStore, eventLog, projection, now)
	notifier, err := service.BuildNotifier(root, cfg, stderr, service.SubscriptionResolver{
		Store:   service.SubscriptionStore{Root: root},
		Queries: w.Queries,
	})
	if err != nil {
		_ = projection.Close()
		return nil, err
	}
	automation := &service.AutomationEngine{
		Store:    service.AutomationStore{Root: root},
		Notifier: notifier,
	}
	w.Actions = service.NewActionService(root, projectStore, ticketStore, eventLog, projection, now, w.Locks, notifier, automation)
	return w, nil
}

func (w *Workspace) Close() {
	if w != nil && w.Projection != nil {
		_ = w.Projection.Close()
	}
}
