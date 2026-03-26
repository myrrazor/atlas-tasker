package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/spf13/cobra"
)

func newChangeCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "change", Short: "Inspect and link ticket changes"}
	list := &cobra.Command{Use: "list", Short: "List linked changes", RunE: runChangeList}
	list.Flags().String("ticket", "", "Filter by ticket id")
	addReadOutputFlags(list, &outputFlags{})
	view := &cobra.Command{Use: "view <CHANGE-ID>", Args: cobra.ExactArgs(1), Short: "Show one change", RunE: runChangeView}
	addReadOutputFlags(view, &outputFlags{})
	link := &cobra.Command{Use: "link <TICKET-ID>", Args: cobra.ExactArgs(1), Short: "Link or update a change for a ticket", RunE: runChangeLink}
	link.Flags().String("change-id", "", "Existing change id to update instead of creating one")
	link.Flags().String("provider", string(contracts.ChangeProviderLocal), "Change provider: local|github")
	link.Flags().String("status", string(contracts.ChangeStatusLocalOnly), "Change status")
	link.Flags().String("run", "", "Associated run id")
	link.Flags().String("branch", "", "Source branch name")
	link.Flags().String("base", "", "Base branch")
	link.Flags().String("head", "", "Provider head ref")
	link.Flags().String("url", "", "Provider URL")
	link.Flags().String("external-id", "", "Provider external id")
	link.Flags().String("checks-status", "", "Optional check aggregate status")
	link.Flags().StringArray("reviewer", nil, "Actor requested for review (repeatable)")
	link.Flags().String("review-summary", "", "Human summary for the linked change")
	addMutationFlags(link, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(link, &outputFlags{})
	unlink := &cobra.Command{Use: "unlink <TICKET-ID> <CHANGE-ID>", Args: cobra.ExactArgs(2), Short: "Remove a change link from a ticket", RunE: runChangeUnlink}
	addMutationFlags(unlink, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(unlink, &outputFlags{})
	cmd.AddCommand(list, view, link, unlink)
	return cmd
}

func newChecksCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "checks", Short: "Inspect and record checks"}
	list := &cobra.Command{Use: "list", Short: "List checks by scope", RunE: runChecksList}
	list.Flags().String("scope", "", "Scope: run|change|ticket")
	list.Flags().String("id", "", "Scope id")
	addReadOutputFlags(list, &outputFlags{})
	view := &cobra.Command{Use: "view <CHECK-ID>", Args: cobra.ExactArgs(1), Short: "Show one check", RunE: runChecksView}
	addReadOutputFlags(view, &outputFlags{})
	record := &cobra.Command{Use: "record", Short: "Record or update a local/manual check", RunE: runChecksRecord}
	record.Flags().String("check-id", "", "Existing check id to update instead of creating one")
	record.Flags().String("scope", "", "Scope: run|change|ticket")
	record.Flags().String("id", "", "Scope id")
	record.Flags().String("name", "", "Check name")
	record.Flags().String("source", string(contracts.CheckSourceManual), "Check source: local|provider|manual")
	record.Flags().String("provider", "", "Optional provider name")
	record.Flags().String("status", string(contracts.CheckStatusQueued), "Check status")
	record.Flags().String("conclusion", string(contracts.CheckConclusionUnknown), "Check conclusion")
	record.Flags().String("summary", "", "Check summary")
	record.Flags().String("url", "", "Check URL")
	record.Flags().String("external-id", "", "Provider external id")
	_ = record.MarkFlagRequired("scope")
	_ = record.MarkFlagRequired("id")
	_ = record.MarkFlagRequired("name")
	addMutationFlags(record, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(record, &outputFlags{})
	cmd.AddCommand(list, view, record)
	return cmd
}

func runChangeList(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	ticketID, _ := cmd.Flags().GetString("ticket")
	changes, err := workspace.queries.ListChanges(commandContext(cmd), strings.TrimSpace(ticketID))
	if err != nil {
		return err
	}
	pretty := formatChangeList(changes)
	data := map[string]any{"kind": "change_list", "generated_at": time.Now().UTC(), "items": changes}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runChangeView(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	view, err := workspace.queries.ChangeDetail(commandContext(cmd), args[0])
	if err != nil {
		return err
	}
	pretty := formatChangeDetail(view)
	data := map[string]any{"kind": "change_detail", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runChangeLink(cmd *cobra.Command, args []string) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	change, err := buildChangeFromFlags(cmd, args[0])
	if err != nil {
		return err
	}
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	saved, err := workspace.actions.LinkChange(ctx, args[0], change, normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	view, err := workspace.queries.ChangeDetail(ctx, saved.ChangeID)
	if err != nil {
		return err
	}
	pretty := formatChangeDetail(view)
	data := map[string]any{"kind": "change_detail", "generated_at": view.GeneratedAt, "payload": view}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runChangeUnlink(cmd *cobra.Command, args []string) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	ticket, change, err := workspace.actions.UnlinkChange(ctx, args[0], args[1], normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	pretty := fmt.Sprintf("unlinked %s from %s", change.ChangeID, ticket.ID)
	data := map[string]any{
		"kind":         "change_detail",
		"generated_at": time.Now().UTC(),
		"payload": map[string]any{
			"ticket": ticket,
			"change": change,
		},
	}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runChecksList(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	scopeRaw, _ := cmd.Flags().GetString("scope")
	scopeID, _ := cmd.Flags().GetString("id")
	checks, err := workspace.queries.ListChecks(commandContext(cmd), contracts.CheckScope(strings.TrimSpace(scopeRaw)), strings.TrimSpace(scopeID))
	if err != nil {
		return err
	}
	pretty := formatCheckList(checks)
	data := map[string]any{"kind": "check_list", "generated_at": time.Now().UTC(), "items": checks}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runChecksView(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	check, err := workspace.queries.CheckDetail(commandContext(cmd), args[0])
	if err != nil {
		return err
	}
	pretty := formatCheckDetail(check)
	data := map[string]any{"kind": "check_detail", "generated_at": check.UpdatedAt, "payload": check}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runChecksRecord(cmd *cobra.Command, _ []string) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	check, err := buildCheckFromFlags(cmd)
	if err != nil {
		return err
	}
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	saved, err := workspace.actions.RecordCheck(ctx, check, normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	pretty := formatCheckDetail(saved)
	data := map[string]any{"kind": "check_detail", "generated_at": saved.UpdatedAt, "payload": saved}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func buildChangeFromFlags(cmd *cobra.Command, ticketID string) (contracts.ChangeRef, error) {
	changeID, _ := cmd.Flags().GetString("change-id")
	providerRaw, _ := cmd.Flags().GetString("provider")
	statusRaw, _ := cmd.Flags().GetString("status")
	runID, _ := cmd.Flags().GetString("run")
	branch, _ := cmd.Flags().GetString("branch")
	base, _ := cmd.Flags().GetString("base")
	head, _ := cmd.Flags().GetString("head")
	url, _ := cmd.Flags().GetString("url")
	externalID, _ := cmd.Flags().GetString("external-id")
	checksStatusRaw, _ := cmd.Flags().GetString("checks-status")
	reviewersRaw, _ := cmd.Flags().GetStringArray("reviewer")
	reviewSummary, _ := cmd.Flags().GetString("review-summary")
	reviewers := make([]contracts.Actor, 0, len(reviewersRaw))
	for _, raw := range reviewersRaw {
		reviewer := contracts.Actor(strings.TrimSpace(raw))
		if reviewer != "" {
			reviewers = append(reviewers, reviewer)
		}
	}
	change := contracts.ChangeRef{
		ChangeID:            strings.TrimSpace(changeID),
		Provider:            contracts.ChangeProvider(strings.TrimSpace(providerRaw)),
		TicketID:            ticketID,
		RunID:               strings.TrimSpace(runID),
		BranchName:          strings.TrimSpace(branch),
		BaseBranch:          strings.TrimSpace(base),
		HeadRef:             strings.TrimSpace(head),
		URL:                 strings.TrimSpace(url),
		ExternalID:          strings.TrimSpace(externalID),
		Status:              contracts.ChangeStatus(strings.TrimSpace(statusRaw)),
		ChecksStatus:        contracts.CheckAggregateState(strings.TrimSpace(checksStatusRaw)),
		ReviewRequestedFrom: reviewers,
		ReviewSummary:       strings.TrimSpace(reviewSummary),
	}
	return change, nil
}

func buildCheckFromFlags(cmd *cobra.Command) (contracts.CheckResult, error) {
	checkID, _ := cmd.Flags().GetString("check-id")
	scopeRaw, _ := cmd.Flags().GetString("scope")
	scopeID, _ := cmd.Flags().GetString("id")
	name, _ := cmd.Flags().GetString("name")
	sourceRaw, _ := cmd.Flags().GetString("source")
	providerRaw, _ := cmd.Flags().GetString("provider")
	statusRaw, _ := cmd.Flags().GetString("status")
	conclusionRaw, _ := cmd.Flags().GetString("conclusion")
	summary, _ := cmd.Flags().GetString("summary")
	url, _ := cmd.Flags().GetString("url")
	externalID, _ := cmd.Flags().GetString("external-id")
	check := contracts.CheckResult{
		CheckID:    strings.TrimSpace(checkID),
		Source:     contracts.CheckSource(strings.TrimSpace(sourceRaw)),
		Provider:   contracts.ChangeProvider(strings.TrimSpace(providerRaw)),
		Scope:      contracts.CheckScope(strings.TrimSpace(scopeRaw)),
		ScopeID:    strings.TrimSpace(scopeID),
		Name:       strings.TrimSpace(name),
		Status:     contracts.CheckStatus(strings.TrimSpace(statusRaw)),
		Conclusion: contracts.CheckConclusion(strings.TrimSpace(conclusionRaw)),
		Summary:    strings.TrimSpace(summary),
		URL:        strings.TrimSpace(url),
		ExternalID: strings.TrimSpace(externalID),
	}
	return check, nil
}

func formatChangeList(changes []contracts.ChangeRef) string {
	if len(changes) == 0 {
		return "no changes"
	}
	lines := []string{"changes:"}
	for _, change := range changes {
		line := fmt.Sprintf("- %s [%s] ticket=%s provider=%s", change.ChangeID, change.Status, change.TicketID, change.Provider)
		if change.RunID != "" {
			line += " run=" + change.RunID
		}
		if change.BranchName != "" {
			line += " branch=" + change.BranchName
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func formatChangeDetail(view service.ChangeDetailView) string {
	lines := []string{
		fmt.Sprintf("change %s", view.Change.ChangeID),
		fmt.Sprintf("ticket=%s status=%s provider=%s", view.Change.TicketID, view.Change.Status, view.Change.Provider),
	}
	if view.Change.RunID != "" {
		lines = append(lines, "run="+view.Change.RunID)
	}
	if view.Change.BranchName != "" {
		lines = append(lines, "branch="+view.Change.BranchName)
	}
	if view.Change.URL != "" {
		lines = append(lines, "url="+view.Change.URL)
	}
	if view.Change.ReviewSummary != "" {
		lines = append(lines, "", view.Change.ReviewSummary)
	}
	if len(view.Checks) > 0 {
		lines = append(lines, "", fmt.Sprintf("checks=%d", len(view.Checks)))
		for _, check := range view.Checks {
			lines = append(lines, fmt.Sprintf("- %s [%s/%s] %s", check.CheckID, check.Status, check.Conclusion, check.Name))
		}
	}
	return strings.Join(lines, "\n")
}

func formatCheckList(checks []contracts.CheckResult) string {
	if len(checks) == 0 {
		return "no checks"
	}
	lines := []string{"checks:"}
	for _, check := range checks {
		lines = append(lines, fmt.Sprintf("- %s [%s/%s] %s scope=%s:%s", check.CheckID, check.Status, check.Conclusion, check.Name, check.Scope, check.ScopeID))
	}
	return strings.Join(lines, "\n")
}

func formatCheckDetail(check contracts.CheckResult) string {
	lines := []string{
		fmt.Sprintf("check %s", check.CheckID),
		fmt.Sprintf("scope=%s:%s", check.Scope, check.ScopeID),
		fmt.Sprintf("source=%s status=%s conclusion=%s", check.Source, check.Status, check.Conclusion),
		fmt.Sprintf("name=%s", check.Name),
	}
	if check.URL != "" {
		lines = append(lines, "url="+check.URL)
	}
	if check.Summary != "" {
		lines = append(lines, "", check.Summary)
	}
	return strings.Join(lines, "\n")
}
