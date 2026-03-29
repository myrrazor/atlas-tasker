package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	"github.com/pelletier/go-toml/v2"
)

const workspaceProjectKey = "_workspace"

type AgentStore struct {
	Root string
}

func (s AgentStore) SaveAgent(_ context.Context, profile contracts.AgentProfile) error {
	profile.AgentID = sanitizeAgentID(profile.AgentID)
	if err := profile.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.AgentsDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create agents dir: %w", err)
	}
	raw, err := toml.Marshal(profile)
	if err != nil {
		return fmt.Errorf("encode agent %s: %w", profile.AgentID, err)
	}
	if err := os.WriteFile(s.path(profile.AgentID), raw, 0o644); err != nil {
		return fmt.Errorf("write agent %s: %w", profile.AgentID, err)
	}
	return nil
}

func (s AgentStore) LoadAgent(_ context.Context, agentID string) (contracts.AgentProfile, error) {
	raw, err := os.ReadFile(s.path(agentID))
	if err != nil {
		return contracts.AgentProfile{}, fmt.Errorf("read agent %s: %w", agentID, err)
	}
	var profile contracts.AgentProfile
	if err := toml.Unmarshal(raw, &profile); err != nil {
		return contracts.AgentProfile{}, fmt.Errorf("parse agent %s: %w", agentID, err)
	}
	if strings.TrimSpace(profile.AgentID) == "" {
		profile.AgentID = sanitizeAgentID(agentID)
	}
	if err := profile.Validate(); err != nil {
		return contracts.AgentProfile{}, err
	}
	return profile, nil
}

func (s AgentStore) ListAgents(_ context.Context) ([]contracts.AgentProfile, error) {
	entries, err := os.ReadDir(storage.AgentsDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.AgentProfile{}, nil
		}
		return nil, fmt.Errorf("read agents dir: %w", err)
	}
	items := make([]contracts.AgentProfile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		profile, err := s.LoadAgent(context.Background(), strings.TrimSuffix(entry.Name(), ".toml"))
		if err != nil {
			return nil, err
		}
		items = append(items, profile)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].AgentID < items[j].AgentID
	})
	return items, nil
}

func (s AgentStore) DeleteAgent(_ context.Context, agentID string) error {
	if err := os.Remove(s.path(agentID)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete agent %s: %w", agentID, err)
	}
	return nil
}

func (s AgentStore) path(agentID string) string {
	return filepath.Join(storage.AgentsDir(s.Root), sanitizeAgentID(agentID)+".toml")
}

func sanitizeAgentID(agentID string) string {
	agentID = strings.TrimSpace(strings.ToLower(agentID))
	agentID = strings.ReplaceAll(agentID, " ", "-")
	agentID = strings.ReplaceAll(agentID, "/", "-")
	agentID = strings.ReplaceAll(agentID, "_", "-")
	if agentID == "" {
		return "agent"
	}
	return agentID
}
