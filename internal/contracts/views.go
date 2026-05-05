package contracts

import (
	"fmt"
	"strings"
)

// SavedViewKind defines the read surface a saved view resolves into.
type SavedViewKind string

const (
	SavedViewKindBoard  SavedViewKind = "board"
	SavedViewKindSearch SavedViewKind = "search"
	SavedViewKindQueue  SavedViewKind = "queue"
	SavedViewKindNext   SavedViewKind = "next"
)

var validSavedViewKinds = map[SavedViewKind]struct{}{
	SavedViewKindBoard:  {},
	SavedViewKindSearch: {},
	SavedViewKindQueue:  {},
	SavedViewKindNext:   {},
}

// IsValid reports whether the saved view kind is supported.
func (k SavedViewKind) IsValid() bool {
	_, ok := validSavedViewKinds[k]
	return ok
}

// SavedBoardConfig captures presentation tweaks for board-backed saved views.
type SavedBoardConfig struct {
	Columns []Status `json:"columns,omitempty" toml:"columns"`
}

// Validate reports invalid board column values.
func (c SavedBoardConfig) Validate() error {
	for _, column := range c.Columns {
		if !column.IsValid() {
			return fmt.Errorf("invalid saved view board column: %s", column)
		}
	}
	return nil
}

// SavedQueueConfig captures presentation tweaks for queue-backed saved views.
type SavedQueueConfig struct {
	Categories []string `json:"categories,omitempty" toml:"categories"`
}

// Validate rejects blank category names.
func (c SavedQueueConfig) Validate() error {
	for _, category := range c.Categories {
		if strings.TrimSpace(category) == "" {
			return fmt.Errorf("saved view queue categories cannot be blank")
		}
	}
	return nil
}

// SavedView stores a named reusable read configuration.
type SavedView struct {
	Name     string           `json:"name" toml:"name"`
	Title    string           `json:"title,omitempty" toml:"title"`
	Kind     SavedViewKind    `json:"kind" toml:"kind"`
	Query    string           `json:"query,omitempty" toml:"query"`
	Project  string           `json:"project,omitempty" toml:"project"`
	Assignee Actor            `json:"assignee,omitempty" toml:"assignee"`
	Type     TicketType       `json:"type,omitempty" toml:"type"`
	Actor    Actor            `json:"actor,omitempty" toml:"actor"`
	Board    SavedBoardConfig `json:"board,omitempty" toml:"board"`
	Queue    SavedQueueConfig `json:"queue,omitempty" toml:"queue"`
}

// Validate checks the saved view payload before it is persisted or executed.
func (v SavedView) Validate() error {
	if strings.TrimSpace(v.Name) == "" {
		return fmt.Errorf("saved view name is required")
	}
	if !v.Kind.IsValid() {
		return fmt.Errorf("invalid saved view kind: %s", v.Kind)
	}
	if v.Assignee != "" && !v.Assignee.IsValid() {
		return fmt.Errorf("invalid saved view assignee: %s", v.Assignee)
	}
	if v.Actor != "" && !v.Actor.IsValid() {
		return fmt.Errorf("invalid saved view actor: %s", v.Actor)
	}
	if v.Type != "" && !v.Type.IsValid() {
		return fmt.Errorf("invalid saved view type: %s", v.Type)
	}
	if err := v.Board.Validate(); err != nil {
		return err
	}
	if err := v.Queue.Validate(); err != nil {
		return err
	}
	switch v.Kind {
	case SavedViewKindSearch:
		if strings.TrimSpace(v.Query) == "" {
			return fmt.Errorf("search saved view requires query")
		}
		if _, err := ParseSearchQuery(v.Query); err != nil {
			return fmt.Errorf("invalid saved view query: %w", err)
		}
	case SavedViewKindBoard:
		if strings.TrimSpace(v.Query) != "" {
			return fmt.Errorf("board saved view does not support raw query")
		}
	case SavedViewKindQueue, SavedViewKindNext:
		if strings.TrimSpace(v.Query) != "" {
			return fmt.Errorf("%s saved view does not support raw query", v.Kind)
		}
	}
	return nil
}
