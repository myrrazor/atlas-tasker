package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/storage"
)

type RuntimeState struct {
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	URL       string    `json:"url"`
	PID       int       `json:"pid"`
	Project   string    `json:"project,omitempty"`
	Actor     string    `json:"actor,omitempty"`
	ReadOnly  bool      `json:"read_only"`
	StartedAt time.Time `json:"started_at"`
}

func RuntimeStatePath(root string) string {
	return filepath.Join(storage.TrackerDir(root), "runtime", "web", "server.json")
}

func WriteRuntimeState(root string, state RuntimeState) error {
	path := RuntimeStatePath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}

func ClearRuntimeState(root string) error {
	err := os.Remove(RuntimeStatePath(root))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func ReadRuntimeState(root string) (RuntimeState, error) {
	raw, err := os.ReadFile(RuntimeStatePath(root))
	if err != nil {
		return RuntimeState{}, err
	}
	var state RuntimeState
	if err := json.Unmarshal(raw, &state); err != nil {
		return RuntimeState{}, err
	}
	return state, nil
}
