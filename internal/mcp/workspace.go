package mcp

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/app"
	"github.com/myrrazor/atlas-tasker/internal/service"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

type Workspace struct {
	Root       string
	ID         string
	Actions    *service.ActionService
	Queries    *service.QueryService
	Projection *sqlitestore.Store
	Locks      service.WriteLockManager
	closeFn    func() error
}

type WorkspaceOpenOptions struct {
	Home               string
	StateDir           string
	Notice             io.Writer
	Now                func() time.Time
	SkipIndexFreshness bool
}

func OpenWorkspace(root string, stderr io.Writer, now func() time.Time) (*Workspace, error) {
	return OpenWorkspaceWith(root, WorkspaceOpenOptions{Notice: stderr, Now: now})
}

func OpenWorkspaceWith(root string, opts WorkspaceOpenOptions) (*Workspace, error) {
	if strings.TrimSpace(root) == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	w, err := app.OpenWorkspace(root, app.OpenOptions{
		Home:               opts.Home,
		StateDir:           opts.StateDir,
		Now:                opts.Now,
		Notice:             opts.Notice,
		SkipIndexFreshness: opts.SkipIndexFreshness,
	})
	if err != nil {
		return nil, err
	}
	return adaptWorkspace(w), nil
}

func (w *Workspace) Close() {
	if w == nil {
		return
	}
	if w.closeFn != nil {
		_ = w.closeFn()
		return
	}
	if w.Projection != nil {
		_ = w.Projection.Close()
	}
}
