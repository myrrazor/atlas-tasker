package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type GHService struct {
	Root string
}

func (s GHService) Capability(ctx context.Context) (GitHubCapabilityView, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return GitHubCapabilityView{}, nil
	}
	view := GitHubCapabilityView{Installed: true}
	if _, err := s.ghOutput(ctx, "auth", "status"); err != nil {
		return view, nil
	}
	view.Authenticated = true
	repo, err := s.repoView(ctx)
	if err == nil {
		view.Repo = repo.NameWithOwner
	}
	return view, nil
}

func (s GHService) ContextForTicket(ctx context.Context, ticket contracts.TicketSnapshot, suggestedBranch string) (GitHubContextView, error) {
	capability, err := s.Capability(ctx)
	if err != nil {
		return GitHubContextView{}, err
	}
	view := GitHubContextView{
		Capability:     capability,
		SuggestedTitle: fmt.Sprintf("%s: %s", ticket.ID, strings.TrimSpace(ticket.Title)),
	}
	if !capability.Installed || !capability.Authenticated {
		return view, nil
	}
	pulls, err := s.PullRequests(ctx, ticket.ID, suggestedBranch)
	if err != nil {
		return GitHubContextView{}, err
	}
	view.PullRequests = pulls
	return view, nil
}

func (s GHService) PullRequests(ctx context.Context, ticketID string, branchHint string) ([]GitHubPRView, error) {
	if strings.TrimSpace(ticketID) == "" {
		return []GitHubPRView{}, nil
	}
	capability, err := s.Capability(ctx)
	if err != nil {
		return nil, err
	}
	if !capability.Installed || !capability.Authenticated {
		return []GitHubPRView{}, nil
	}
	pulls, err := s.prList(ctx, "--search", ticketID)
	if err != nil {
		if isBenignGHRepoError(err) {
			return []GitHubPRView{}, nil
		}
		return nil, err
	}
	if len(pulls) == 0 && strings.TrimSpace(branchHint) != "" {
		pulls, err = s.prList(ctx, "--head", strings.TrimSpace(branchHint))
		if err != nil {
			if isBenignGHRepoError(err) {
				return []GitHubPRView{}, nil
			}
			return nil, err
		}
		return pulls, nil
	}
	return pulls, nil
}

func (s GHService) CreatePullRequest(ctx context.Context, ticket contracts.TicketSnapshot, title string, body string, base string, draft bool) (GitHubPRView, error) {
	capability, err := s.Capability(ctx)
	if err != nil {
		return GitHubPRView{}, err
	}
	if !capability.Installed {
		return GitHubPRView{}, fmt.Errorf("gh CLI is not installed")
	}
	if !capability.Authenticated {
		return GitHubPRView{}, fmt.Errorf("gh CLI is not authenticated")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = fmt.Sprintf("%s: %s", ticket.ID, strings.TrimSpace(ticket.Title))
	}
	body = strings.TrimSpace(body)
	if body == "" {
		body = fmt.Sprintf("Atlas ticket: %s\n\n%s", ticket.ID, strings.TrimSpace(ticket.Description))
	}
	args := []string{"pr", "create", "--title", title, "--body", body}
	if strings.TrimSpace(base) != "" {
		args = append(args, "--base", strings.TrimSpace(base))
	}
	if draft {
		args = append(args, "--draft")
	}
	out, err := s.ghOutput(ctx, args...)
	if err != nil {
		return GitHubPRView{}, err
	}
	ref := strings.TrimSpace(out)
	if ref == "" {
		return GitHubPRView{}, fmt.Errorf("gh pr create returned no URL")
	}
	return s.pullRequestView(ctx, ref)
}

func (s GHService) ImportReferenceURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("GitHub URL is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid GitHub URL: %w", err)
	}
	if !strings.EqualFold(parsed.Host, "github.com") {
		return "", fmt.Errorf("unsupported host %q", parsed.Host)
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 4 {
		return "", fmt.Errorf("GitHub URL must reference an issue or pull request")
	}
	switch parts[2] {
	case "pull", "issues":
		return raw, nil
	default:
		return "", fmt.Errorf("GitHub URL must reference an issue or pull request")
	}
}

func (s GHService) prList(ctx context.Context, extraArgs ...string) ([]GitHubPRView, error) {
	args := []string{"pr", "list", "--state", "all", "--json", "number,title,url,state,isDraft,headRefName,baseRefName"}
	args = append(args, extraArgs...)
	out, err := s.ghOutput(ctx, args...)
	if err != nil {
		return nil, err
	}
	type ghPR struct {
		BaseRefName string `json:"baseRefName"`
		HeadRefName string `json:"headRefName"`
		IsDraft     bool   `json:"isDraft"`
		Number      int    `json:"number"`
		State       string `json:"state"`
		Title       string `json:"title"`
		URL         string `json:"url"`
	}
	var payload []ghPR
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		return nil, fmt.Errorf("decode gh pr list: %w", err)
	}
	views := make([]GitHubPRView, 0, len(payload))
	for _, pr := range payload {
		views = append(views, GitHubPRView{
			Number:  pr.Number,
			Title:   pr.Title,
			URL:     pr.URL,
			State:   strings.ToLower(pr.State),
			Draft:   pr.IsDraft,
			HeadRef: pr.HeadRefName,
			BaseRef: pr.BaseRefName,
		})
	}
	return views, nil
}

func (s GHService) pullRequestView(ctx context.Context, ref string) (GitHubPRView, error) {
	out, err := s.ghOutput(ctx, "pr", "view", ref, "--json", "number,title,url,state,isDraft,headRefName,baseRefName")
	if err != nil {
		return GitHubPRView{}, err
	}
	type ghPR struct {
		BaseRefName string `json:"baseRefName"`
		HeadRefName string `json:"headRefName"`
		IsDraft     bool   `json:"isDraft"`
		Number      int    `json:"number"`
		State       string `json:"state"`
		Title       string `json:"title"`
		URL         string `json:"url"`
	}
	var payload ghPR
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		return GitHubPRView{}, fmt.Errorf("decode gh pr view: %w", err)
	}
	return GitHubPRView{
		Number:  payload.Number,
		Title:   payload.Title,
		URL:     payload.URL,
		State:   strings.ToLower(payload.State),
		Draft:   payload.IsDraft,
		HeadRef: payload.HeadRefName,
		BaseRef: payload.BaseRefName,
	}, nil
}

func (s GHService) repoView(ctx context.Context) (struct {
	NameWithOwner string `json:"nameWithOwner"`
	URL           string `json:"url"`
}, error) {
	out, err := s.ghOutput(ctx, "repo", "view", "--json", "nameWithOwner,url")
	if err != nil {
		return struct {
			NameWithOwner string `json:"nameWithOwner"`
			URL           string `json:"url"`
		}{}, err
	}
	var payload struct {
		NameWithOwner string `json:"nameWithOwner"`
		URL           string `json:"url"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		return payload, fmt.Errorf("decode gh repo view: %w", err)
	}
	return payload, nil
}

func (s GHService) ghOutput(ctx context.Context, args ...string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
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
		return "", fmt.Errorf("gh %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}

func isBenignGHRepoError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"not a git repository",
		"no git remotes found",
		"failed to run git",
		"could not determine base repo",
		"unable to determine default branch",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}
