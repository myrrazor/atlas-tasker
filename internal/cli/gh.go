package cli

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/spf13/cobra"
)

func newGHCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "gh", Short: "GitHub pull request workflow backed by the gh CLI"}
	status := &cobra.Command{Use: "status", Short: "Show gh CLI capability for this workspace", RunE: runGHStatus}
	prs := &cobra.Command{Use: "prs <TICKET-ID>", Args: cobra.ExactArgs(1), Short: "List GitHub pull requests matching a ticket", RunE: runGHPRs}
	view := &cobra.Command{Use: "view <PR-NUMBER|URL>", Args: cobra.ExactArgs(1), Short: "Show one GitHub pull request", RunE: runGHView}
	checks := &cobra.Command{Use: "checks <TICKET-ID|PR-REF>", Args: cobra.ExactArgs(1), Short: "Show GitHub checks for a ticket's pull request or a direct PR ref", RunE: runGHChecks}
	createPR := &cobra.Command{Use: "create-pr <TICKET-ID>", Args: cobra.ExactArgs(1), Short: "Create a GitHub pull request and link it as a change", RunE: runGHCreatePR}
	createPR.Flags().String("title", "", "Pull request title (defaults to \"<TICKET-ID>: <title>\")")
	createPR.Flags().String("body", "", "Pull request body")
	createPR.Flags().String("base", "", "Base branch (defaults to the repository default)")
	createPR.Flags().Bool("draft", false, "Open the pull request as a draft")
	requestReview := &cobra.Command{Use: "request-review <TICKET-ID|CHANGE-ID>", Args: cobra.ExactArgs(1), Short: "Request provider-side review for a ticket's linked change", RunE: runGHRequestReview}
	importURL := &cobra.Command{Use: "import-url <TICKET-ID>", Args: cobra.ExactArgs(1), Short: "Import a GitHub pull request URL as a linked change (alias of `tracker change import-url`)", RunE: runChangeImportURL}
	importURL.Flags().String("url", "", "GitHub pull request URL")
	_ = importURL.MarkFlagRequired("url")
	for _, sub := range []*cobra.Command{status, prs, view, checks} {
		addReadOutputFlags(sub, &outputFlags{})
	}
	for _, sub := range []*cobra.Command{createPR, requestReview, importURL} {
		addMutationFlags(sub, &mutationFlags{Actor: "human:owner"})
		addReadOutputFlags(sub, &outputFlags{})
	}
	cmd.AddCommand(status, prs, view, checks, createPR, requestReview, importURL)
	return cmd
}

var ghTicketRefPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*-[0-9]+$`)

func runGHStatus(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	capability, err := workspace.queries.GitHubCapability(commandContext(cmd))
	if err != nil {
		return err
	}
	pretty := formatGHCapability(capability)
	md := fmt.Sprintf("## GitHub Status\n\n- Installed: %t\n- Authenticated: %t\n- Repo: %s\n", capability.Installed, capability.Authenticated, capability.Repo)
	data := map[string]any{"kind": "gh_status", "generated_at": time.Now().UTC(), "payload": capability}
	return writeCommandOutput(cmd, data, md, pretty)
}

func runGHPRs(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	view, err := workspace.queries.GitHubTicketContext(commandContext(cmd), args[0])
	if err != nil {
		return err
	}
	pretty := formatGHTicketContext(view)
	data := map[string]any{"kind": "gh_pr_list", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runGHChecks(cmd *cobra.Command, args []string) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	ref := strings.TrimSpace(args[0])
	if ghTicketRefPattern.MatchString(ref) {
		view, err := workspace.queries.GitHubTicketContext(ctx, ref)
		if err != nil {
			return err
		}
		if !view.GitHub.Capability.Installed {
			return ghSetupError(fmt.Errorf("gh CLI is not installed"))
		}
		if !view.GitHub.Capability.Authenticated {
			return ghSetupError(fmt.Errorf("gh CLI is not authenticated"))
		}
		if len(view.GitHub.PullRequests) == 0 {
			return apperr.New(apperr.CodeNotFound, fmt.Sprintf("no pull request found for %s; run 'tracker gh create-pr' or 'tracker gh import-url' first", ref))
		}
		ref = view.GitHub.PullRequests[0].URL
	}
	checks, err := workspace.queries.GitHubPullRequestChecks(ctx, ref)
	if err != nil {
		return ghSetupError(err)
	}
	pretty := formatGHChecks(checks)
	data := map[string]any{"kind": "gh_check_list", "generated_at": time.Now().UTC(), "items": checks}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runGHView(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	pr, err := workspace.queries.GitHubPullRequest(commandContext(cmd), args[0])
	if err != nil {
		return ghSetupError(err)
	}
	pretty := formatGHPullRequest(pr)
	data := map[string]any{"kind": "gh_pr", "generated_at": time.Now().UTC(), "payload": pr}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runGHCreatePR(cmd *cobra.Command, args []string) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	title, _ := cmd.Flags().GetString("title")
	body, _ := cmd.Flags().GetString("body")
	base, _ := cmd.Flags().GetString("base")
	draft, _ := cmd.Flags().GetBool("draft")
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	opts := service.CreatePROptions{Title: title, Body: body, Base: base, Draft: draft}
	result, err := workspace.actions.CreatePullRequestForTicket(ctx, args[0], opts, normalizeActor(actorRaw), reason)
	if err != nil {
		return ghSetupError(err)
	}
	pretty := formatChangeCreateResult(result)
	data := map[string]any{"kind": "change_create_result", "generated_at": result.GeneratedAt, "reason_codes": result.ReasonCodes, "payload": result}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runGHRequestReview(cmd *cobra.Command, args []string) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	ref := strings.TrimSpace(args[0])
	changeID := ref
	if ghTicketRefPattern.MatchString(ref) {
		changes, err := workspace.queries.ListChanges(ctx, ref)
		if err != nil {
			return err
		}
		changeID = ""
		for _, change := range changes {
			if change.Provider == contracts.ChangeProviderGitHub {
				changeID = change.ChangeID
				break
			}
		}
		if changeID == "" {
			return apperr.New(apperr.CodeNotFound, fmt.Sprintf("no linked change for %s; run 'tracker gh create-pr' or 'tracker gh import-url' first", ref))
		}
	}
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	view, err := workspace.actions.RequestChangeReview(ctx, changeID, normalizeActor(actorRaw), reason)
	if err != nil {
		return ghSetupError(err)
	}
	pretty := formatChangeStatus(view)
	data := map[string]any{"kind": "change_status", "generated_at": view.GeneratedAt, "reason_codes": view.ReasonCodes, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

// every gh subcommand except `status` needs a working binary; turn the provider
// errors into a fixable setup message instead of a generic conflict
func ghSetupError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "not installed"), strings.Contains(msg, "provider_unavailable"):
		return apperr.New(apperr.CodeRepairNeeded, "gh CLI is not installed; install GitHub CLI and run 'gh auth login'")
	case strings.Contains(msg, "not authenticated"), strings.Contains(msg, "provider_unauthenticated"):
		return apperr.New(apperr.CodeRepairNeeded, "gh CLI is not authenticated; run 'gh auth login'")
	}
	return err
}

func formatGHCapability(capability service.GitHubCapabilityView) string {
	switch {
	case !capability.Installed:
		return "gh not installed; install GitHub CLI and run 'gh auth login'"
	case !capability.Authenticated:
		return "gh installed but not authenticated; run 'gh auth login'"
	case capability.Repo != "":
		return "gh ready repo=" + capability.Repo
	default:
		return "gh ready (no repository resolved)"
	}
}

func formatGHTicketContext(view service.GitHubTicketContextView) string {
	lines := []string{
		fmt.Sprintf("github prs for %s", view.TicketID),
		formatGHCapability(view.GitHub.Capability),
	}
	if view.SuggestedBranch != "" {
		lines = append(lines, "suggested_branch="+view.SuggestedBranch)
	}
	if len(view.GitHub.PullRequests) == 0 {
		lines = append(lines, "no pull requests")
		return strings.Join(lines, "\n")
	}
	lines = append(lines, "prs:")
	for _, pr := range view.GitHub.PullRequests {
		lines = append(lines, fmt.Sprintf("- #%d [%s] draft=%t review=%s %s", pr.Number, pr.State, pr.Draft, pr.ReviewDecision, pr.Title))
		if pr.URL != "" {
			lines = append(lines, "  "+pr.URL)
		}
	}
	return strings.Join(lines, "\n")
}

func formatGHChecks(checks []service.GitHubCheckView) string {
	if len(checks) == 0 {
		return "no checks reported"
	}
	lines := []string{"checks:"}
	for _, check := range checks {
		name := check.Name
		if check.Workflow != "" {
			name = check.Workflow + " / " + name
		}
		line := fmt.Sprintf("- %s [%s] state=%s", name, check.Bucket, check.State)
		if check.Link != "" {
			line += " " + check.Link
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func formatGHPullRequest(pr service.GitHubPRView) string {
	lines := []string{
		fmt.Sprintf("pr #%d [%s] %s", pr.Number, pr.State, pr.Title),
		fmt.Sprintf("draft=%t review=%s base=%s head=%s", pr.Draft, pr.ReviewDecision, pr.BaseRef, pr.HeadRef),
	}
	if pr.URL != "" {
		lines = append(lines, "url="+pr.URL)
	}
	if !pr.MergedAt.IsZero() {
		lines = append(lines, "merged_at="+pr.MergedAt.Format(timeRFC3339))
	}
	return strings.Join(lines, "\n")
}
