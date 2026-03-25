package service

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
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

func (s SCMService) ContextForTicket(ctx context.Context, ticket contracts.TicketSnapshot) (GitContextView, error) {
	repo, err := s.RepoStatus(ctx)
	if err != nil {
		return GitContextView{}, err
	}
	if !repo.Present {
		ghView, err := GHService{Root: s.Root}.ContextForTicket(ctx, ticket, "")
		if err != nil {
			return GitContextView{}, err
		}
		return GitContextView{GitHub: ghView}, nil
	}
	suggested := s.SuggestedBranch(ticket)
	refs, err := s.TicketRefs(ctx, ticket.ID)
	if err != nil {
		return GitContextView{}, err
	}
	ghView, err := GHService{Root: s.Root}.ContextForTicket(ctx, ticket, suggested)
	if err != nil {
		return GitContextView{}, err
	}
	branch := strings.ToLower(repo.Branch)
	id := strings.ToLower(ticket.ID)
	return GitContextView{
		Repo:                 repo,
		SuggestedBranch:      suggested,
		CurrentBranchMatches: branch != "" && strings.Contains(branch, id),
		Refs:                 refs,
		GitHub:               ghView,
	}, nil
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
