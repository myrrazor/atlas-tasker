package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type WorktreeManager struct {
	Root string
}

func (m WorktreeManager) Prepare(ctx context.Context, project contracts.Project, ticket contracts.TicketSnapshot, run contracts.RunSnapshot) (contracts.RunSnapshot, error) {
	scm := SCMService{Root: m.Root}
	cfg := effectiveWorktreeConfig(project.Defaults.Worktrees)
	if !cfg.Enabled || cfg.DefaultMode == contracts.WorktreeModeDisabled {
		return run, nil
	}
	repo, err := scm.RepoStatus(ctx)
	if err != nil {
		return contracts.RunSnapshot{}, err
	}
	if !repo.Present {
		return run, nil
	}
	if cfg.RequireCleanMain || project.Defaults.ExecutionSafety.RequireCleanRepo {
		dirty, err := worktreeDirtySourceRepo(ctx, scm)
		if err != nil {
			return contracts.RunSnapshot{}, err
		}
		if dirty {
			return contracts.RunSnapshot{}, apperr.New(apperr.CodeConflict, "source repo is dirty; clean it before dispatch")
		}
	}
	if strings.TrimSpace(run.BranchName) == "" {
		run.BranchName = runBranchName(ticket.ID, run.RunID)
	}
	run.WorktreePath = filepath.Join(m.worktreeBase(repo.Root, cfg), strings.ToLower(ticket.ID)+"-"+run.RunID)
	return run, nil
}

func (m WorktreeManager) Create(ctx context.Context, run contracts.RunSnapshot) error {
	scm := SCMService{Root: m.Root}
	if strings.TrimSpace(run.WorktreePath) == "" {
		return nil
	}
	repo, err := scm.RepoStatus(ctx)
	if err != nil {
		return err
	}
	if !repo.Present {
		return nil
	}
	if info, err := os.Stat(run.WorktreePath); err == nil {
		if info.IsDir() {
			if _, statErr := os.Stat(filepath.Join(run.WorktreePath, ".git")); statErr == nil {
				return nil
			}
		}
		return apperr.New(apperr.CodeConflict, fmt.Sprintf("worktree path already exists: %s", run.WorktreePath))
	}
	if err := os.MkdirAll(filepath.Dir(run.WorktreePath), 0o755); err != nil {
		return fmt.Errorf("create worktree base: %w", err)
	}
	args := []string{"worktree", "add"}
	if strings.TrimSpace(run.BranchName) != "" {
		args = append(args, "-b", run.BranchName)
	}
	args = append(args, run.WorktreePath, "HEAD")
	if _, err := scm.gitOutput(ctx, args...); err != nil {
		return err
	}
	return nil
}

func (m WorktreeManager) Remove(ctx context.Context, run contracts.RunSnapshot, force bool) error {
	scm := SCMService{Root: m.Root}
	if strings.TrimSpace(run.WorktreePath) == "" {
		return nil
	}
	if _, err := os.Stat(run.WorktreePath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat worktree path: %w", err)
	}
	status, err := m.Inspect(ctx, run)
	if err != nil {
		return err
	}
	if status.Dirty && !force {
		return apperr.New(apperr.CodeConflict, fmt.Sprintf("worktree %s has uncommitted changes; rerun cleanup with --force", run.RunID))
	}
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, run.WorktreePath)
	if _, err := scm.gitOutput(ctx, args...); err != nil {
		if _, statErr := os.Stat(run.WorktreePath); os.IsNotExist(statErr) {
			return nil
		}
		return err
	}
	_, _ = scm.gitOutput(ctx, "worktree", "prune")
	return nil
}

func (m WorktreeManager) Repair(ctx context.Context, runs []contracts.RunSnapshot) ([]WorktreeStatusView, error) {
	scm := SCMService{Root: m.Root}
	repo, err := scm.RepoStatus(ctx)
	if err != nil {
		return nil, err
	}
	if repo.Present {
		paths := make([]string, 0, len(runs))
		for _, run := range runs {
			if strings.TrimSpace(run.WorktreePath) == "" {
				continue
			}
			if _, err := os.Stat(run.WorktreePath); err == nil {
				paths = append(paths, run.WorktreePath)
			}
		}
		if len(paths) > 0 {
			args := append([]string{"worktree", "repair"}, paths...)
			if _, err := scm.gitOutput(ctx, args...); err != nil {
				return nil, err
			}
		}
	}
	return m.List(ctx, runs)
}

func (m WorktreeManager) Prune(ctx context.Context, runs []contracts.RunSnapshot) ([]WorktreeStatusView, error) {
	scm := SCMService{Root: m.Root}
	repo, err := scm.RepoStatus(ctx)
	if err != nil {
		return nil, err
	}
	if repo.Present {
		if _, err := scm.gitOutput(ctx, "worktree", "prune"); err != nil {
			return nil, err
		}
	}
	return m.List(ctx, runs)
}

func (m WorktreeManager) List(ctx context.Context, runs []contracts.RunSnapshot) ([]WorktreeStatusView, error) {
	items := make([]WorktreeStatusView, 0, len(runs))
	for _, run := range runs {
		status, err := m.Inspect(ctx, run)
		if err != nil {
			return nil, err
		}
		items = append(items, status)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].TicketID == items[j].TicketID {
			return items[i].RunID < items[j].RunID
		}
		return items[i].TicketID < items[j].TicketID
	})
	return items, nil
}

func (m WorktreeManager) Inspect(ctx context.Context, run contracts.RunSnapshot) (WorktreeStatusView, error) {
	scm := SCMService{Root: m.Root}
	view := WorktreeStatusView{
		RunID:         run.RunID,
		TicketID:      run.TicketID,
		Path:          run.WorktreePath,
		BranchName:    run.BranchName,
		LastCheckedAt: time.Now().UTC(),
	}
	if strings.TrimSpace(run.WorktreePath) == "" {
		return view, nil
	}
	if _, err := os.Stat(run.WorktreePath); err != nil {
		if os.IsNotExist(err) {
			return view, nil
		}
		return WorktreeStatusView{}, fmt.Errorf("stat worktree path: %w", err)
	}
	view.Present = true
	if status, err := scm.gitOutput(ctx, "-C", run.WorktreePath, "status", "--porcelain"); err == nil {
		view.Dirty = strings.TrimSpace(status) != ""
	}
	if strings.TrimSpace(view.BranchName) == "" {
		if branch, err := scm.gitOutput(ctx, "-C", run.WorktreePath, "branch", "--show-current"); err == nil {
			view.BranchName = strings.TrimSpace(branch)
		}
	}
	return view, nil
}

func effectiveWorktreeConfig(cfg contracts.WorktreeConfig) contracts.WorktreeConfig {
	if cfg == (contracts.WorktreeConfig{}) {
		return contracts.WorktreeConfig{Enabled: true, DefaultMode: contracts.WorktreeModePerRun, RequireCleanMain: true}
	}
	if cfg.DefaultMode == "" {
		cfg.DefaultMode = contracts.WorktreeModePerRun
	}
	if !cfg.Enabled && cfg.DefaultMode == contracts.WorktreeModeDisabled {
		return cfg
	}
	if !cfg.Enabled {
		cfg.Enabled = true
	}
	if !cfg.RequireCleanMain {
		cfg.RequireCleanMain = true
	}
	return cfg
}

func (m WorktreeManager) worktreeBase(repoRoot string, cfg contracts.WorktreeConfig) string {
	if strings.TrimSpace(cfg.Root) != "" {
		if filepath.IsAbs(cfg.Root) {
			return cfg.Root
		}
		return filepath.Clean(filepath.Join(m.Root, cfg.Root))
	}
	return filepath.Join(filepath.Dir(repoRoot), ".atlas-tasker-worktrees", filepath.Base(repoRoot))
}

func runBranchName(ticketID string, runID string) string {
	return fmt.Sprintf("run/%s-%s", strings.ToLower(strings.TrimSpace(ticketID)), strings.TrimSpace(runID))
}

func worktreeDirtySourceRepo(ctx context.Context, scm SCMService) (bool, error) {
	output, err := scm.gitOutput(ctx, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		path := strings.TrimSpace(line)
		if path == "" {
			continue
		}
		if len(path) > 3 {
			path = strings.TrimSpace(path[3:])
		}
		if strings.Contains(path, " -> ") {
			parts := strings.SplitN(path, " -> ", 2)
			path = strings.TrimSpace(parts[1])
		}
		if !isAtlasWorkspacePath(path) {
			return true, nil
		}
	}
	return false, nil
}

func isAtlasWorkspacePath(path string) bool {
	clean := filepath.ToSlash(strings.TrimSpace(path))
	if clean == "" {
		return false
	}
	if clean == ".tracker" || strings.HasPrefix(clean, ".tracker/") {
		return true
	}
	if strings.HasPrefix(clean, "projects/") && strings.HasSuffix(clean, "/project.md") {
		return true
	}
	if strings.HasPrefix(clean, "projects/") && strings.Contains(clean, "/tickets/") {
		return true
	}
	return false
}
