package app

import (
	"context"
	"path/filepath"
	"sync"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

type Hub struct {
	app   *App
	mu    sync.Mutex
	bound map[string]*Workspace
}

func newHub(a *App) *Hub {
	return &Hub{app: a, bound: map[string]*Workspace{}}
}

func (h *Hub) Bind(ctx context.Context, id string) (*Workspace, error) {
	_ = ctx
	if h == nil || h.app == nil {
		return nil, apperr.New(apperr.CodeInvalidInput, "hub is not initialized")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	row, found, err := h.app.lookup(id)
	if err != nil {
		return nil, err
	}
	if !found {
		if w, ok := h.bound[id]; ok {
			_ = w.Close()
			delete(h.bound, id)
		}
		return nil, apperr.New(apperr.CodeNotFound, "workspace is not registered")
	}
	rec := h.app.decorate(row)
	if rec.Health != HealthAvailable {
		if w, ok := h.bound[id]; ok {
			_ = w.Close()
			delete(h.bound, id)
		}
		return nil, apperr.New(apperr.CodeRepairNeeded, rec.HealthDetail)
	}
	if w, ok := h.bound[id]; ok {
		if filepath.Clean(w.Root) != filepath.Clean(rec.Path) {
			_ = w.Close()
			delete(h.bound, id)
		} else {
			return w, nil
		}
	}
	w, err := OpenWorkspaceByID(h.app, id, OpenOptions{
		Home:     h.app.home,
		StateDir: h.app.stateDir,
		Now:      h.app.opts.Now,
		Notice:   h.app.opts.Notice,
	})
	if err != nil {
		return nil, err
	}
	h.bound[id] = w
	return w, nil
}

func (h *Hub) BindRoot(ctx context.Context, root string) (*Workspace, error) {
	_ = ctx
	root, err := service.CanonicalWorkspaceRoot(root)
	if err != nil {
		return nil, err
	}
	w, err := OpenWorkspace(root, OpenOptions{
		Home:     h.app.home,
		StateDir: h.app.stateDir,
		Now:      h.app.opts.Now,
		Notice:   h.app.opts.Notice,
	})
	if err != nil {
		return nil, err
	}
	row, found, err := h.app.lookup(w.ID)
	if err != nil {
		_ = w.Close()
		return nil, err
	}
	if found {
		rec := h.app.decorate(row)
		if rec.Health != HealthAvailable {
			_ = w.Close()
			return nil, apperr.New(apperr.CodeRepairNeeded, rec.HealthDetail)
		}
		if filepath.Clean(rec.Path) != filepath.Clean(w.Root) {
			_ = w.Close()
			return nil, apperr.New(apperr.CodeRepairNeeded, "workspace identity is bound to a different path; repair required")
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if existing, ok := h.bound[w.ID]; ok {
		if filepath.Clean(existing.Root) != filepath.Clean(w.Root) {
			_ = existing.Close()
			delete(h.bound, w.ID)
		} else {
			_ = w.Close()
			return existing, nil
		}
	}
	h.bound[w.ID] = w
	return w, nil
}

func (h *Hub) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	var first error
	for id, w := range h.bound {
		if err := w.Close(); err != nil && first == nil {
			first = err
		}
		delete(h.bound, id)
	}
	return first
}
