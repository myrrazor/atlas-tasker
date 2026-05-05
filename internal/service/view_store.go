package service

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	"github.com/pelletier/go-toml/v2"
)

// ViewStore persists saved views under the local tracker workspace.
type ViewStore struct {
	Root string
}

// ListViews returns all saved views sorted by name.
func (s ViewStore) ListViews() ([]contracts.SavedView, error) {
	entries, err := os.ReadDir(storage.ViewsDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.SavedView{}, nil
		}
		return nil, fmt.Errorf("read views dir: %w", err)
	}
	views := make([]contracts.SavedView, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		view, err := s.LoadView(strings.TrimSuffix(entry.Name(), ".toml"))
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	sort.Slice(views, func(i, j int) bool {
		return views[i].Name < views[j].Name
	})
	return views, nil
}

// LoadView reads one saved view by name.
func (s ViewStore) LoadView(name string) (contracts.SavedView, error) {
	raw, err := os.ReadFile(s.viewPath(name))
	if err != nil {
		return contracts.SavedView{}, fmt.Errorf("read saved view %s: %w", name, err)
	}
	var view contracts.SavedView
	if err := toml.Unmarshal(raw, &view); err != nil {
		return contracts.SavedView{}, fmt.Errorf("parse saved view %s: %w", name, err)
	}
	if strings.TrimSpace(view.Name) == "" {
		view.Name = sanitizeViewName(name)
	}
	if err := view.Validate(); err != nil {
		return contracts.SavedView{}, err
	}
	return view, nil
}

// SaveView writes one saved view file.
func (s ViewStore) SaveView(view contracts.SavedView) error {
	view.Name = sanitizeViewName(view.Name)
	if err := view.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.ViewsDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create views dir: %w", err)
	}
	raw, err := toml.Marshal(view)
	if err != nil {
		return fmt.Errorf("encode saved view %s: %w", view.Name, err)
	}
	if err := os.WriteFile(s.viewPath(view.Name), raw, 0o644); err != nil {
		return fmt.Errorf("write saved view %s: %w", view.Name, err)
	}
	return nil
}

// DeleteView removes one saved view file.
func (s ViewStore) DeleteView(name string) error {
	if err := os.Remove(s.viewPath(name)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete saved view %s: %w", name, err)
	}
	return nil
}

func (s ViewStore) viewPath(name string) string {
	return filepath.Join(storage.ViewsDir(s.Root), sanitizeViewName(name)+".toml")
}

func sanitizeViewName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "_", "-")
	if name == "" {
		return "view"
	}
	return name
}
