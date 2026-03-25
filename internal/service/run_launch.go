package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func (s *QueryService) RunOpen(ctx context.Context, runID string) (RunLaunchManifestView, error) {
	detail, err := s.RunDetail(ctx, runID)
	if err != nil {
		return RunLaunchManifestView{}, err
	}
	return launchManifestView(s.Root, detail.Run, s.now()), nil
}

func (s *ActionService) LaunchRun(ctx context.Context, runID string, refresh bool) (RunLaunchManifestView, error) {
	return withWriteLock(ctx, s.LockManager, "launch run runtime", func(ctx context.Context) (RunLaunchManifestView, error) {
		query := NewQueryService(s.Root, s.Projects, s.Tickets, s.Events, s.Projection, s.Clock)
		detail, err := query.RunDetail(ctx, runID)
		if err != nil {
			return RunLaunchManifestView{}, err
		}
		view := launchManifestView(s.Root, detail.Run, s.now())
		agent, _ := s.Agents.LoadAgent(ctx, detail.Run.AgentID)
		manifest, err := integrations.BuildRunManifest(integrations.RunManifestInput{
			WorkspaceRoot:    s.Root,
			Run:              detail.Run,
			Ticket:           detail.Ticket,
			Agent:            agent,
			Gates:            detail.Gates,
			Evidence:         detail.Evidence,
			Handoffs:         detail.Handoffs,
			RuntimeDir:       view.RuntimeDir,
			BriefPath:        view.BriefPath,
			ContextPath:      view.ContextPath,
			CodexLaunchPath:  view.CodexLaunchPath,
			ClaudeLaunchPath: view.ClaudeLaunchPath,
			EvidenceDir:      view.EvidenceDir,
			GeneratedAt:      s.now(),
		})
		if err != nil {
			return RunLaunchManifestView{}, err
		}
		if err := os.MkdirAll(view.RuntimeDir, 0o755); err != nil {
			return RunLaunchManifestView{}, fmt.Errorf("create runtime dir: %w", err)
		}
		created, updated, err := writeRuntimeArtifacts(refresh, map[string]string{
			view.BriefPath:        manifest.BriefMarkdown,
			view.ContextPath:      string(manifest.ContextJSON),
			view.CodexLaunchPath:  manifest.CodexLaunch,
			view.ClaudeLaunchPath: manifest.ClaudeLaunch,
		})
		if err != nil {
			return RunLaunchManifestView{}, err
		}
		view.Created = created
		view.Updated = updated
		view.GeneratedAt = s.now()
		return view, nil
	})
}

func launchManifestView(root string, run contracts.RunSnapshot, generatedAt time.Time) RunLaunchManifestView {
	return RunLaunchManifestView{
		RunID:            run.RunID,
		TicketID:         run.TicketID,
		AgentID:          run.AgentID,
		RuntimeDir:       storage.RuntimeDir(root, run.RunID),
		WorktreePath:     run.WorktreePath,
		EvidenceDir:      storage.EvidenceDir(root, run.RunID),
		BriefPath:        storage.RuntimeBriefFile(root, run.RunID),
		ContextPath:      storage.RuntimeContextFile(root, run.RunID),
		CodexLaunchPath:  storage.RuntimeLaunchFile(root, run.RunID, "codex"),
		ClaudeLaunchPath: storage.RuntimeLaunchFile(root, run.RunID, "claude"),
		GeneratedAt:      generatedAt.UTC(),
	}
}

func writeRuntimeArtifacts(refresh bool, files map[string]string) ([]string, []string, error) {
	created := make([]string, 0, len(files))
	updated := make([]string, 0, len(files))
	for path, body := range files {
		current, err := os.ReadFile(path)
		if err == nil {
			if !refresh {
				continue
			}
			if string(current) == body {
				continue
			}
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				return nil, nil, fmt.Errorf("write runtime artifact %s: %w", filepath.Base(path), err)
			}
			updated = append(updated, path)
			continue
		}
		if !os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("read runtime artifact %s: %w", filepath.Base(path), err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return nil, nil, fmt.Errorf("write runtime artifact %s: %w", filepath.Base(path), err)
		}
		created = append(created, path)
	}
	sort.Strings(created)
	sort.Strings(updated)
	return created, updated, nil
}
