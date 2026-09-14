package app

import (
	"context"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/service"
)

// RunWorkspaceJobs reindexes and ticks local checkpoints for available
// registered workspaces. Home Serve runs this in the daemon process.
func (a *App) RunWorkspaceJobs(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	a.tickWorkspaces(ctx)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			a.tickWorkspaces(ctx)
		}
	}
}

func (a *App) tickWorkspaces(ctx context.Context) {
	list, err := a.ListWorkspaces(ctx, ListOptions{IncludeHidden: true})
	if err != nil {
		return
	}
	for _, rec := range list {
		if rec.Health != HealthAvailable {
			continue
		}
		ws, err := a.hub.Bind(ctx, rec.WorkspaceID)
		if err != nil {
			continue
		}
		if ws.Actions != nil {
			if proj, ok := ws.Actions.Projection.(service.RebuildableProjection); ok {
				_, _ = service.EnsureFreshProjection(ctx, ws.Root, ws.Locks, proj, a.opts.Notice)
			}
		}
		_, _ = ws.Actions.BackupTick(ctx, false)
	}
}
