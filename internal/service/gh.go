package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type GHService struct {
	Root string
	Repo string
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
		if isBenignGHRepoError(err) {
			return view, nil
		}
		return GitHubContextView{}, err
	}
	view.PullRequests = pulls
	return view, nil
}

func (s GHService) PullRequests(ctx context.Context, ticketID string, branchHint string) ([]GitHubPRView, error) {
	if strings.TrimSpace(ticketID) == "" && strings.TrimSpace(branchHint) == "" {
		return []GitHubPRView{}, nil
	}
	capability, err := s.Capability(ctx)
	if err != nil {
		return nil, err
	}
	if !capability.Installed || !capability.Authenticated {
		return []GitHubPRView{}, nil
	}
	if ticketID != "" {
		pulls, err := s.prList(ctx, "--search", strings.TrimSpace(ticketID))
		if err == nil && len(pulls) > 0 {
			return pulls, nil
		}
		if err != nil && !isBenignGHRepoError(err) {
			return nil, err
		}
	}
	if branchHint != "" {
		pulls, err := s.prList(ctx, "--head", strings.TrimSpace(branchHint))
		if err != nil {
			if isBenignGHRepoError(err) {
				return []GitHubPRView{}, nil
			}
			return nil, err
		}
		return pulls, nil
	}
	return []GitHubPRView{}, nil
}

func (s GHService) PullRequestView(ctx context.Context, ref string) (GitHubPRView, error) {
	out, err := s.ghRepoOutput(ctx, "pr", "view", ref, "--json", "number,title,url,state,isDraft,headRefName,baseRefName,reviewDecision,mergeStateStatus,mergedAt")
	if err != nil {
		return GitHubPRView{}, err
	}
	return decodeGitHubPR(out)
}

func (s GHService) PullRequestChecks(ctx context.Context, ref string) ([]GitHubCheckView, error) {
	out, err := s.ghRepoOutput(ctx, "pr", "checks", ref, "--json", "bucket,completedAt,description,link,name,startedAt,state,workflow")
	if err != nil {
		return nil, err
	}
	type ghCheck struct {
		Bucket      string `json:"bucket"`
		CompletedAt string `json:"completedAt"`
		Description string `json:"description"`
		Link        string `json:"link"`
		Name        string `json:"name"`
		StartedAt   string `json:"startedAt"`
		State       string `json:"state"`
		Workflow    string `json:"workflow"`
	}
	var payload []ghCheck
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		return nil, fmt.Errorf("decode gh pr checks: %w", err)
	}
	checks := make([]GitHubCheckView, 0, len(payload))
	for _, item := range payload {
		view := GitHubCheckView{
			Bucket:      strings.ToLower(strings.TrimSpace(item.Bucket)),
			Description: strings.TrimSpace(item.Description),
			Link:        strings.TrimSpace(item.Link),
			Name:        strings.TrimSpace(item.Name),
			State:       strings.ToLower(strings.TrimSpace(item.State)),
			Workflow:    strings.TrimSpace(item.Workflow),
		}
		if started := strings.TrimSpace(item.StartedAt); started != "" {
			if parsed, err := time.Parse(time.RFC3339, started); err == nil {
				view.StartedAt = parsed
			}
		}
		if completed := strings.TrimSpace(item.CompletedAt); completed != "" {
			if parsed, err := time.Parse(time.RFC3339, completed); err == nil {
				view.CompletedAt = parsed
			}
		}
		checks = append(checks, view)
	}
	return checks, nil
}

func (s GHService) RequestPullRequestReview(ctx context.Context, ref string) (GitHubPRView, error) {
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
	pr, err := s.PullRequestView(ctx, ref)
	if err != nil {
		return GitHubPRView{}, err
	}
	if !pr.Draft {
		return pr, nil
	}
	if _, err := s.ghRepoOutput(ctx, "pr", "ready", ref); err != nil {
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "already ready for review") {
			return GitHubPRView{}, err
		}
	}
	return s.PullRequestView(ctx, ref)
}

func (s GHService) MergePullRequest(ctx context.Context, ref string) (GitHubPRView, error) {
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
	if _, err := s.ghRepoOutput(ctx, "pr", "merge", ref, "--merge", "--delete-branch=false"); err != nil {
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "already merged") {
			return GitHubPRView{}, err
		}
	}
	return s.PullRequestView(ctx, ref)
}

func (s GHService) ImportPullRequestURL(raw string) (string, int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", 0, fmt.Errorf("GitHub pull request URL is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", 0, fmt.Errorf("invalid GitHub URL: %w", err)
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Host))
	if host != "github.com" && host != "www.github.com" {
		return "", 0, fmt.Errorf("unsupported host %q", parsed.Host)
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 4 || parts[2] != "pull" {
		return "", 0, fmt.Errorf("GitHub URL must reference a pull request")
	}
	owner := strings.TrimSpace(parts[0])
	repo := strings.TrimSpace(parts[1])
	if owner == "" || repo == "" {
		return "", 0, fmt.Errorf("GitHub URL must include an owner and repository")
	}
	number, err := strconv.Atoi(parts[3])
	if err != nil || number <= 0 {
		return "", 0, fmt.Errorf("GitHub URL must include a valid pull request number")
	}
	return fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, repo, number), number, nil
}

func (s GHService) prList(ctx context.Context, extraArgs ...string) ([]GitHubPRView, error) {
	args := []string{"pr", "list", "--state", "all", "--json", "number,title,url,state,isDraft,headRefName,baseRefName,reviewDecision,mergeStateStatus,mergedAt"}
	args = append(args, extraArgs...)
	out, err := s.ghRepoOutput(ctx, args...)
	if err != nil {
		return nil, err
	}
	type ghPR struct {
		BaseRefName      string `json:"baseRefName"`
		HeadRefName      string `json:"headRefName"`
		IsDraft          bool   `json:"isDraft"`
		MergeStateStatus string `json:"mergeStateStatus"`
		MergedAt         string `json:"mergedAt"`
		Number           int    `json:"number"`
		ReviewDecision   string `json:"reviewDecision"`
		State            string `json:"state"`
		Title            string `json:"title"`
		URL              string `json:"url"`
	}
	var payload []ghPR
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		return nil, fmt.Errorf("decode gh pr list: %w", err)
	}
	views := make([]GitHubPRView, 0, len(payload))
	for _, pr := range payload {
		views = append(views, mapGitHubPR(pr.BaseRefName, pr.HeadRefName, pr.IsDraft, pr.MergeStateStatus, pr.MergedAt, pr.Number, pr.ReviewDecision, pr.State, pr.Title, pr.URL))
	}
	return views, nil
}

func decodeGitHubPR(raw string) (GitHubPRView, error) {
	type ghPR struct {
		BaseRefName      string `json:"baseRefName"`
		HeadRefName      string `json:"headRefName"`
		IsDraft          bool   `json:"isDraft"`
		MergeStateStatus string `json:"mergeStateStatus"`
		MergedAt         string `json:"mergedAt"`
		Number           int    `json:"number"`
		ReviewDecision   string `json:"reviewDecision"`
		State            string `json:"state"`
		Title            string `json:"title"`
		URL              string `json:"url"`
	}
	var payload ghPR
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return GitHubPRView{}, fmt.Errorf("decode gh pr view: %w", err)
	}
	return mapGitHubPR(payload.BaseRefName, payload.HeadRefName, payload.IsDraft, payload.MergeStateStatus, payload.MergedAt, payload.Number, payload.ReviewDecision, payload.State, payload.Title, payload.URL), nil
}

func mapGitHubPR(baseRef string, headRef string, draft bool, mergeState string, mergedAt string, number int, reviewDecision string, state string, title string, rawURL string) GitHubPRView {
	view := GitHubPRView{
		BaseRef:          strings.TrimSpace(baseRef),
		Draft:            draft,
		HeadRef:          strings.TrimSpace(headRef),
		MergeStateStatus: strings.ToLower(strings.TrimSpace(mergeState)),
		Number:           number,
		ReviewDecision:   strings.ToLower(strings.TrimSpace(reviewDecision)),
		State:            strings.ToLower(strings.TrimSpace(state)),
		Title:            strings.TrimSpace(title),
		URL:              strings.TrimSpace(rawURL),
	}
	if mergedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(mergedAt)); err == nil {
			view.MergedAt = parsed
		}
	}
	return view
}

func (s GHService) repoView(ctx context.Context) (struct {
	NameWithOwner string `json:"nameWithOwner"`
	URL           string `json:"url"`
}, error) {
	out, err := s.ghRepoOutput(ctx, "repo", "view", "--json", "nameWithOwner,url")
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

func (s GHService) ghRepoOutput(ctx context.Context, args ...string) (string, error) {
	if strings.TrimSpace(s.Repo) == "" {
		return s.ghOutput(ctx, args...)
	}
	repoArgs := append([]string{}, args...)
	repoArgs = append(repoArgs, "-R", strings.TrimSpace(s.Repo))
	return s.ghOutput(ctx, repoArgs...)
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
		"no pull requests match",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}
