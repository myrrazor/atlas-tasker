package setup

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	sqlitestore "github.com/myrrazor/atlas-tasker/internal/storage/sqlite"
)

func suggestTeamPreset(selected int) string {
	switch {
	case selected <= 1:
		return "solo"
	case selected == 2:
		return "pair"
	case selected <= 4:
		return "swarm"
	default:
		return "crossfire"
	}
}

func actorHintFor(target integrations.Target, team string, selected []integrations.Target) contracts.Actor {
	if strings.TrimSpace(team) == "" {
		return contracts.Actor("agent:" + string(target))
	}
	preset, err := service.TeamPresetByName(strings.TrimSpace(team), teamProvider(selected))
	if err != nil || len(preset.Agents) == 0 {
		return contracts.Actor("agent:" + string(target))
	}
	for i, candidate := range selected {
		if candidate == target {
			if i < len(preset.Agents) {
				return contracts.Actor("agent:" + preset.Agents[i].AgentID)
			}
			return contracts.Actor("agent:" + preset.Agents[len(preset.Agents)-1].AgentID)
		}
	}
	return contracts.Actor("agent:" + string(target))
}

func teamProvider(selected []integrations.Target) string {
	hasClaude, hasCodex := false, false
	for _, target := range selected {
		switch target {
		case integrations.TargetClaude:
			hasClaude = true
		case integrations.TargetCodex:
			hasCodex = true
		}
	}
	if hasClaude && hasCodex {
		return "mixed"
	}
	if hasCodex {
		return "codex"
	}
	if hasClaude {
		return "claude"
	}
	return ""
}

func (e *Engine) applyTeam(ctx context.Context, plan *TeamPlan) error {
	if plan == nil || !plan.Writes || strings.TrimSpace(plan.Requested) == "" {
		return nil
	}
	actions, closer, err := openSetupActions(e.WorkspaceRoot)
	if err != nil {
		return err
	}
	defer closer()
	_, err = actions.ApplyTeamPreset(ctx, plan.Requested, plan.Provider, false, contracts.Actor("human:owner"), "setup team policy")
	return err
}

func openSetupActions(root string) (*service.ActionService, func(), error) {
	ticketStore := mdstore.TicketStore{RootDir: root}
	eventLog := &eventstore.Log{RootDir: root}
	indexPath := filepath.Join(storage.TrackerDir(root), "index.sqlite")
	projection, err := sqlitestore.Open(indexPath, ticketStore, eventLog)
	if err != nil {
		return nil, nil, err
	}
	projection.Root = root
	locks := service.FileLockManager{Root: root}
	projectStore := mdstore.ProjectStore{RootDir: root}
	actions := service.NewActionService(root, projectStore, ticketStore, eventLog, projection, nil, locks, nil, nil)
	return actions, func() { _ = projection.Close() }, nil
}

func validateTeamName(name string) error {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	if _, err := service.TeamPresetByName(name, ""); err != nil {
		return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("unknown team preset %q; use solo, pair, swarm, or crossfire", name))
	}
	return nil
}
