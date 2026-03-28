package service

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type SCMService struct {
	Root string
}

func (s SCMService) RepoStatus(ctx context.Context) (GitRepoView, error) {
	root, err := s.gitOutput(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not a git repository") {
			return GitRepoView{}, nil
		}
		return GitRepoView{}, err
	}
	branch, err := s.gitOutput(ctx, "branch", "--show-current")
	if err != nil {
		return GitRepoView{}, err
	}
	status, err := s.gitOutput(ctx, "status", "--porcelain")
	if err != nil {
		return GitRepoView{}, err
	}
	return GitRepoView{
		Present: true,
		Root:    strings.TrimSpace(root),
		Branch:  strings.TrimSpace(branch),
		Dirty:   strings.TrimSpace(status) != "",
	}, nil
}

func (s SCMService) SuggestedBranch(ticket contracts.TicketSnapshot) string {
	base := strings.ToLower(strings.TrimSpace(ticket.ID))
	title := slugify(ticket.Title)
	if title == "" {
		return "ticket/" + base
	}
	return fmt.Sprintf("ticket/%s-%s", base, title)
}

func (s SCMService) TicketRefs(ctx context.Context, ticketID string) ([]GitCommitView, error) {
	if strings.TrimSpace(ticketID) == "" {
		return []GitCommitView{}, nil
	}
	output, err := s.gitOutput(ctx, "log", "--date=iso-strict", "--pretty=format:%H%x1f%cI%x1f%s", "--all", "--grep", ticketID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not a git repository") {
			return []GitCommitView{}, nil
		}
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return []GitCommitView{}, nil
	}
	refs := make([]GitCommitView, 0, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, "\x1f")
		if len(parts) != 3 {
			continue
		}
		at, err := time.Parse(time.RFC3339, strings.TrimSpace(parts[1]))
		if err != nil {
			continue
		}
		refs = append(refs, GitCommitView{
			Hash:       strings.TrimSpace(parts[0]),
			AuthorDate: at,
			Subject:    strings.TrimSpace(parts[2]),
		})
	}
	return refs, nil
}

func (s SCMService) Commit(ctx context.Context, ticket contracts.TicketSnapshot, message string) (string, error) {
	repo, err := s.RepoStatus(ctx)
	if err != nil {
		return "", err
	}
	if !repo.Present {
		return "", fmt.Errorf("git repository not detected")
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return "", fmt.Errorf("commit message is required")
	}
	branch, err := s.currentBranch(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(branch) == "" {
		return "", fmt.Errorf("git commit requires a checked-out branch")
	}
	if err := s.ensureNoNestedRepoAmbiguity(repo.Root); err != nil {
		return "", err
	}
	if !strings.HasPrefix(message, ticket.ID+":") {
		message = fmt.Sprintf("%s: %s", ticket.ID, message)
	}
	staged, err := s.gitOutput(ctx, "diff", "--cached", "--name-only")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(staged) == "" {
		return "", fmt.Errorf("no staged changes found for git commit")
	}
	if _, err := s.gitOutput(ctx, "commit", "-m", message); err != nil {
		return "", err
	}
	head, err := s.gitOutput(ctx, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(head), nil
}

func (s SCMService) currentBranch(ctx context.Context) (string, error) {
	branch, err := s.gitOutput(ctx, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not a git repository") {
			return "", err
		}
		return "", nil
	}
	return strings.TrimSpace(branch), nil
}

func (s SCMService) ContextForTicket(ctx context.Context, ticket contracts.TicketSnapshot) (GitContextView, error) {
	repo, err := s.RepoStatus(ctx)
	if err != nil {
		return GitContextView{}, err
	}
	suggested := s.SuggestedBranch(ticket)
	view := GitContextView{SuggestedBranch: suggested, Repo: repo}
	if repo.Present {
		refs, err := s.TicketRefs(ctx, ticket.ID)
		if err != nil {
			return GitContextView{}, err
		}
		branch := strings.ToLower(repo.Branch)
		id := strings.ToLower(ticket.ID)
		view.CurrentBranchMatches = branch != "" && strings.Contains(branch, id)
		view.Refs = refs
	}
	return view, nil
}

func (s SCMService) gitOutput(ctx context.Context, args ...string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = s.Root
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}

func (s SCMService) ensureNoNestedRepoAmbiguity(repoRoot string) error {
	root := canonicalPath(filepath.Clean(s.Root))
	repoGitDir := filepath.Join(canonicalPath(filepath.Clean(repoRoot)), ".git")
	var nested string
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == filepath.Join(root, ".tracker") && entry.IsDir() {
			return filepath.SkipDir
		}
		if entry.Name() != ".git" {
			return nil
		}
		if filepath.Clean(path) == repoGitDir {
			return nil
		}
		nested = path
		return fs.SkipAll
	})
	if walkErr != nil && walkErr != fs.SkipAll {
		return fmt.Errorf("scan nested git repos: %w", walkErr)
	}
	if nested != "" {
		rel, err := filepath.Rel(root, nested)
		if err != nil {
			rel = nested
		}
		return fmt.Errorf("nested git repo detected at %s; commit from the repo containing the workspace root", rel)
	}
	return nil
}

func (s SCMService) BranchExists(ctx context.Context, branch string) (bool, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return false, nil
	}
	if _, err := s.gitOutput(ctx, "rev-parse", "--verify", "--quiet", branch); err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "not a git repository") {
			return false, err
		}
		if strings.Contains(msg, "unknown revision") || strings.Contains(msg, "needed a single revision") || strings.Contains(msg, "exit status 1") {
			return false, nil
		}
		return false, nil
	}
	return true, nil
}

func (s SCMService) ChangedFiles(ctx context.Context) ([]string, error) {
	output, err := s.gitOutput(ctx, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not a git repository") {
			return []string{}, nil
		}
		return nil, err
	}
	seen := map[string]struct{}{}
	files := make([]string, 0)
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
		if path == "" || isAtlasWorkspacePath(path) {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		files = append(files, filepath.ToSlash(path))
	}
	sort.Strings(files)
	return files, nil
}

func canonicalPath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}
	return filepath.Clean(resolved)
}

var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	raw = slugPattern.ReplaceAllString(raw, "-")
	raw = strings.Trim(raw, "-")
	if len(raw) > 48 {
		raw = strings.Trim(raw[:48], "-")
	}
	return raw
}
