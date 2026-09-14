package app

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

type OpenOptions struct {
	Home               string
	StateDir           string
	Now                func() time.Time
	Notice             io.Writer
	SkipIndexFreshness bool
	LookPath           func(string) (string, error)
	Getenv             func(string) string
	GOOS               string
}

func OpenWorkspace(root string, opts OpenOptions) (*Workspace, error) {
	root, err := service.InitializedWorkspaceRoot(root)
	if err != nil {
		return nil, err
	}
	id, err := service.LoadWorkspaceIdentity(root)
	if err != nil {
		return nil, err
	}
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	notice := opts.Notice
	if notice == nil {
		notice = io.Discard
	}
	ticketStore := mdstore.TicketStore{RootDir: root, Clock: now}
	eventLog := &eventstore.Log{RootDir: root}
	projection, err := sqlitestore.Open(filepath.Join(storage.TrackerDir(root), "index.sqlite"), ticketStore, eventLog)
	if err != nil {
		if sqlitestore.IsCorrupt(err) {
			return nil, apperr.Wrap(apperr.CodeRepairNeeded, err, "ticket index is unreadable; run 'tracker doctor --repair' to rebuild it")
		}
		return nil, err
	}
	locks := service.FileLockManager{Root: root}
	projection.SetRoot(root)
	if !opts.SkipIndexFreshness {
		if _, err := service.EnsureFreshProjection(context.Background(), root, locks, projection, notice); err != nil {
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
	queries := service.NewQueryService(root, projectStore, ticketStore, eventLog, projection, now)
	notifier, err := service.BuildNotifier(root, cfg, notice, service.SubscriptionResolver{
		Store:   service.SubscriptionStore{Root: root},
		Queries: queries,
	})
	if err != nil {
		_ = projection.Close()
		return nil, err
	}
	automation := &service.AutomationEngine{
		Store:    service.AutomationStore{Root: root},
		Notifier: notifier,
	}
	actions := service.NewActionService(root, projectStore, ticketStore, eventLog, projection, now, locks, notifier, automation)
	home := opts.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	service.AttachUserState(actions, queries, home, opts.StateDir)
	service.AttachUserStateEnv(actions, queries, opts.Getenv, opts.GOOS)
	return &Workspace{
		Root:    root,
		ID:      id,
		Actions: actions,
		Queries: queries,
		Locks:   locks,
		closeFn: func() error { return projection.Close() },
	}, nil
}

func OpenWorkspaceByID(a *App, id string, opts OpenOptions) (*Workspace, error) {
	if a == nil {
		return nil, apperr.New(apperr.CodeInvalidInput, "app is required")
	}
	row, ok, err := a.lookup(id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperr.New(apperr.CodeNotFound, "workspace is not registered")
	}
	rec := a.decorate(row)
	switch rec.Health {
	case HealthAvailable:
	case HealthDisabled:
		return nil, apperr.New(apperr.CodePermissionDenied, "workspace is hidden")
	default:
		return nil, apperr.New(apperr.CodeRepairNeeded, rec.HealthDetail)
	}
	if opts.Home == "" {
		opts.Home = a.home
	}
	if opts.StateDir == "" {
		opts.StateDir = a.stateDir
	}
	if opts.Now == nil {
		opts.Now = a.opts.Now
	}
	if opts.Notice == nil {
		opts.Notice = a.opts.Notice
	}
	if opts.Getenv == nil {
		opts.Getenv = a.getenv()
	}
	if opts.GOOS == "" {
		opts.GOOS = a.opts.GOOS
	}
	return OpenWorkspace(rec.Path, opts)
}
