package service

import (
	"context"
	"fmt"
	"time"
)

type GitHubTicketContextView struct {
	GitHub          GitHubContextView `json:"github"`
	Repo            string            `json:"repo,omitempty"`
	SuggestedBranch string            `json:"suggested_branch,omitempty"`
	TicketID        string            `json:"ticket_id"`
	GeneratedAt     time.Time         `json:"generated_at"`
}

func (s *QueryService) GitHubCapability(ctx context.Context) (GitHubCapabilityView, error) {
	defaults, err := resolveSCMDefaults(ctx, s.Root, s.Projects, "")
	if err != nil {
		return GitHubCapabilityView{}, err
	}
	return GHService{Root: s.Root, Repo: defaults.Repo}.Capability(ctx)
}

func (s *QueryService) GitHubTicketContext(ctx context.Context, ticketID string) (GitHubTicketContextView, error) {
	ticket, err := s.Tickets.GetTicket(ctx, ticketID)
	if err != nil {
		return GitHubTicketContextView{}, err
	}
	defaults, err := resolveSCMDefaults(ctx, s.Root, s.Projects, ticket.Project)
	if err != nil {
		return GitHubTicketContextView{}, err
	}
	branch := SCMService{Root: s.Root}.SuggestedBranch(ticket)
	github, err := (GHService{Root: s.Root, Repo: defaults.Repo}).ContextForTicket(ctx, ticket, branch)
	if err != nil {
		return GitHubTicketContextView{}, err
	}
	return GitHubTicketContextView{
		GitHub:          github,
		Repo:            firstNonEmpty(github.Capability.Repo, defaults.Repo),
		SuggestedBranch: branch,
		TicketID:        ticket.ID,
		GeneratedAt:     s.now(),
	}, nil
}

func (s *QueryService) GitHubPullRequest(ctx context.Context, ref string) (GitHubPRView, error) {
	gh, err := s.githubService(ctx)
	if err != nil {
		return GitHubPRView{}, err
	}
	return gh.PullRequestView(ctx, ref)
}

func (s *QueryService) GitHubPullRequestChecks(ctx context.Context, ref string) ([]GitHubCheckView, error) {
	gh, err := s.githubService(ctx)
	if err != nil {
		return nil, err
	}
	return gh.PullRequestChecks(ctx, ref)
}

func (s *QueryService) githubService(ctx context.Context) (GHService, error) {
	defaults, err := resolveSCMDefaults(ctx, s.Root, s.Projects, "")
	if err != nil {
		return GHService{}, err
	}
	gh := GHService{Root: s.Root, Repo: defaults.Repo}
	capability, err := gh.Capability(ctx)
	if err != nil {
		return GHService{}, err
	}
	if !capability.Installed {
		return GHService{}, fmt.Errorf("gh CLI is not installed")
	}
	if !capability.Authenticated {
		return GHService{}, fmt.Errorf("gh CLI is not authenticated")
	}
	return gh, nil
}
