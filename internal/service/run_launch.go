package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	type fileOp struct {
		path     string
		body     []byte
		existed  bool
		previous []byte
		tempPath string
	}

	created := make([]string, 0, len(files))
	updated := make([]string, 0, len(files))
	ops := make([]fileOp, 0, len(files))
	for path, body := range files {
		current, err := os.ReadFile(path)
		if err == nil {
			if !refresh {
				continue
			}
			if string(current) == body {
				continue
			}
			ops = append(ops, fileOp{path: path, body: []byte(body), existed: true, previous: current})
			continue
		}
		if !os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("read runtime artifact %s: %w", filepath.Base(path), err)
		}
		ops = append(ops, fileOp{path: path, body: []byte(body)})
	}
	sort.Slice(ops, func(i, j int) bool { return ops[i].path < ops[j].path })
	for i := range ops {
		temp, err := os.CreateTemp(filepath.Dir(ops[i].path), "."+filepath.Base(ops[i].path)+".tmp-*")
		if err != nil {
			return nil, nil, fmt.Errorf("create runtime temp file %s: %w", filepath.Base(ops[i].path), err)
		}
		if _, err := temp.Write(ops[i].body); err != nil {
			_ = temp.Close()
			_ = os.Remove(temp.Name())
			return nil, nil, fmt.Errorf("write runtime temp file %s: %w", filepath.Base(ops[i].path), err)
		}
		if err := temp.Close(); err != nil {
			_ = os.Remove(temp.Name())
			return nil, nil, fmt.Errorf("close runtime temp file %s: %w", filepath.Base(ops[i].path), err)
		}
		ops[i].tempPath = temp.Name()
	}
	applied := make([]fileOp, 0, len(ops))
	cleanupTemps := func(items []fileOp) {
		for _, item := range items {
			if strings.TrimSpace(item.tempPath) == "" {
				continue
			}
			_ = os.Remove(item.tempPath)
		}
	}
	rollback := func() {
		for i := len(applied) - 1; i >= 0; i-- {
			item := applied[i]
			if item.existed {
				_ = writeFileAtomically(item.path, item.previous)
				continue
			}
			_ = os.Remove(item.path)
		}
	}
	for _, op := range ops {
		if testRuntimeArtifactWriteHook != nil {
			if err := testRuntimeArtifactWriteHook(op.path); err != nil {
				cleanupTemps(ops)
				rollback()
				return nil, nil, err
			}
		}
		if err := os.Rename(op.tempPath, op.path); err != nil {
			cleanupTemps(ops)
			rollback()
			return nil, nil, fmt.Errorf("write runtime artifact %s: %w", filepath.Base(op.path), err)
		}
		applied = append(applied, op)
		if op.existed {
			updated = append(updated, op.path)
			continue
		}
		created = append(created, op.path)
	}
	sort.Strings(created)
	sort.Strings(updated)
	return created, updated, nil
}

func writeFileAtomically(path string, body []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".restore-*")
	if err != nil {
		return err
	}
	if _, err := temp.Write(body); err != nil {
		_ = temp.Close()
		_ = os.Remove(temp.Name())
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(temp.Name())
		return err
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		_ = os.Remove(temp.Name())
		return err
	}
	return nil
}
