package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/spf13/cobra"
)

type bulkDispatchEntry struct {
	TicketID     string   `json:"ticket_id"`
	OK           bool     `json:"ok"`
	AgentID      string   `json:"agent_id,omitempty"`
	RunID        string   `json:"run_id,omitempty"`
	Runbook      string   `json:"runbook,omitempty"`
	Stage        string   `json:"stage,omitempty"`
	ReasonCodes  []string `json:"reason_codes,omitempty"`
	Error        string   `json:"error,omitempty"`
	PreviewOnly  bool     `json:"preview_only,omitempty"`
	WorktreePath string   `json:"worktree_path,omitempty"`
}

func newDispatchCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "dispatch", Short: "Suggest and execute orchestration dispatch"}
	suggest := &cobra.Command{Use: "suggest <TICKET-ID>", Args: cobra.ExactArgs(1), Short: "Suggest eligible agents for a ticket", RunE: runDispatchSuggest}
	addReadOutputFlags(suggest, &outputFlags{})
	queue := &cobra.Command{Use: "queue", Short: "List tickets with dispatch suggestions", RunE: runDispatchQueue}
	addReadOutputFlags(queue, &outputFlags{})
	run := &cobra.Command{Use: "run <TICKET-ID>", Args: cobra.ExactArgs(1), Short: "Dispatch a run manually or auto-route when exactly one agent is eligible", RunE: runDispatchRun}
	run.Flags().String("agent", "", "Dispatch to a specific agent id")
	addMutationFlags(run, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(run, &outputFlags{})
	bulk := &cobra.Command{Use: "bulk", Short: "Dispatch a saved view or ticket set in order", RunE: runDispatchBulk}
	bulk.Flags().StringArray("ticket", nil, "Target ticket id (repeatable)")
	bulk.Flags().String("view", "", "Saved view used to expand ticket targets")
	bulk.Flags().String("agent", "", "Dispatch every ticket to this agent instead of auto-routing")
	bulk.Flags().Bool("dry-run", false, "Preview dispatch decisions without mutating")
	bulk.Flags().Bool("yes", false, "Apply the dispatch")
	addMutationFlags(bulk, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(bulk, &outputFlags{})
	cmd.AddCommand(suggest, queue, run, bulk)
	return cmd
}

func runDispatchSuggest(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	suggestion, err := workspace.queries.DispatchSuggest(commandContext(cmd), args[0])
	if err != nil {
		return err
	}
	pretty := formatDispatchSuggestion(suggestion)
	data := map[string]any{"kind": "dispatch_suggestion", "generated_at": suggestion.GeneratedAt, "payload": suggestion}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runDispatchQueue(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	queue, err := workspace.queries.DispatchQueue(commandContext(cmd))
	if err != nil {
		return err
	}
	pretty := formatDispatchQueue(queue)
	data := map[string]any{"kind": "dispatch_queue", "generated_at": queue.GeneratedAt, "payload": queue}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runDispatchRun(cmd *cobra.Command, args []string) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	agentID, _ := cmd.Flags().GetString("agent")
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	actor := normalizeActor(actorRaw)
	var result service.DispatchResult
	if strings.TrimSpace(agentID) != "" {
		result, err = workspace.actions.DispatchRun(ctx, args[0], agentID, contracts.RunKindWork, actor, reason)
	} else {
		result, err = workspace.actions.AutoDispatchRun(ctx, args[0], actor, reason)
	}
	if err != nil {
		return err
	}
	pretty := fmt.Sprintf("dispatched %s to %s", result.TicketID, result.AgentID)
	data := map[string]any{"kind": "run_dispatch_result", "generated_at": result.GeneratedAt, "payload": result}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runDispatchBulk(cmd *cobra.Command, _ []string) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	confirm, _ := cmd.Flags().GetBool("yes")
	if !dryRun && !confirm {
		return apperr.New(apperr.CodeInvalidInput, "dispatch bulk requires --dry-run or --yes")
	}
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	agentID, _ := cmd.Flags().GetString("agent")
	ticketIDs, source, err := bulkTargetTicketIDs(ctx, workspace, cmd)
	if err != nil {
		return err
	}
	entries := make([]bulkDispatchEntry, 0, len(ticketIDs))
	for _, ticketID := range ticketIDs {
		entry := bulkDispatchEntry{TicketID: ticketID, PreviewOnly: dryRun}
		var result service.DispatchResult
		if strings.TrimSpace(agentID) != "" {
			if dryRun {
				suggestion, suggestErr := workspace.queries.DispatchSuggest(ctx, ticketID)
				if suggestErr != nil {
					entry.Error = suggestErr.Error()
					entries = append(entries, entry)
					continue
				}
				matched := false
				for _, suggestionEntry := range suggestion.Suggestions {
					if suggestionEntry.Agent.AgentID != agentID {
						continue
					}
					matched = true
					entry.AgentID = agentID
					entry.Runbook = suggestionEntry.Runbook
					entry.Stage = suggestionEntry.Stage
					entry.ReasonCodes = append([]string{}, suggestionEntry.ReasonCodes...)
					entry.OK = suggestionEntry.Eligible
					break
				}
				if !matched {
					entry.Error = fmt.Sprintf("agent %s not found", agentID)
				}
			} else {
				result, err = workspace.actions.DispatchRun(ctx, ticketID, agentID, contracts.RunKindWork, normalizeActor(actorRaw), reason)
			}
		} else if dryRun {
			suggestion, suggestErr := workspace.queries.DispatchSuggest(ctx, ticketID)
			if suggestErr != nil {
				entry.Error = suggestErr.Error()
				entries = append(entries, entry)
				continue
			}
			entry.ReasonCodes = dispatchReasonCodes(suggestion)
			entry.AgentID = suggestion.AutoRouteAgentID
			entry.OK = suggestion.AutoRouteAgentID != ""
		} else {
			result, err = workspace.actions.AutoDispatchRun(ctx, ticketID, normalizeActor(actorRaw), reason)
		}
		if !dryRun {
			if err != nil {
				entry.Error = err.Error()
				if appErr, ok := err.(*apperr.Error); ok {
					_ = appErr
				}
				entries = append(entries, entry)
				continue
			}
			entry.OK = true
			entry.AgentID = result.AgentID
			entry.RunID = result.RunID
			entry.Runbook = result.Runbook
			entry.Stage = result.Stage
			entry.WorktreePath = result.WorktreePath
		}
		entries = append(entries, entry)
	}
	payload := map[string]any{"source": source, "dry_run": dryRun, "items": entries}
	pretty := formatBulkDispatch(source, dryRun, entries)
	data := map[string]any{"kind": "bulk_dispatch_result", "generated_at": time.Now().UTC(), "payload": payload}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func formatDispatchSuggestion(suggestion service.DispatchSuggestion) string {
	lines := []string{fmt.Sprintf("dispatch suggestion for %s", suggestion.TicketID)}
	if suggestion.AutoRouteAgentID != "" {
		lines = append(lines, fmt.Sprintf("auto-route=%s", suggestion.AutoRouteAgentID))
	}
	for _, entry := range suggestion.Suggestions {
		line := fmt.Sprintf("- %s eligible=%t active_runs=%d", entry.Agent.AgentID, entry.Eligible, entry.ActiveRuns)
		if entry.Runbook != "" {
			line += fmt.Sprintf(" runbook=%s", entry.Runbook)
		}
		if len(entry.ReasonCodes) > 0 {
			line += fmt.Sprintf(" reasons=%s", strings.Join(entry.ReasonCodes, ","))
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func formatDispatchQueue(queue service.DispatchQueueView) string {
	if len(queue.Entries) == 0 {
		return "dispatch queue is empty"
	}
	lines := []string{"dispatch queue:"}
	for _, entry := range queue.Entries {
		line := fmt.Sprintf("- %s [%s] %s", entry.Ticket.ID, entry.Ticket.Priority, entry.Ticket.Title)
		if entry.Suggestion.AutoRouteAgentID != "" {
			line += fmt.Sprintf(" -> %s", entry.Suggestion.AutoRouteAgentID)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func formatBulkDispatch(source string, dryRun bool, entries []bulkDispatchEntry) string {
	lines := []string{fmt.Sprintf("bulk dispatch source=%s dry_run=%t", source, dryRun)}
	for _, entry := range entries {
		line := fmt.Sprintf("- %s", entry.TicketID)
		if entry.OK {
			line += " ok"
		} else {
			line += " failed"
		}
		if entry.AgentID != "" {
			line += fmt.Sprintf(" agent=%s", entry.AgentID)
		}
		if entry.RunID != "" {
			line += fmt.Sprintf(" run=%s", entry.RunID)
		}
		if len(entry.ReasonCodes) > 0 {
			line += fmt.Sprintf(" reasons=%s", strings.Join(entry.ReasonCodes, ","))
		}
		if entry.Error != "" {
			line += fmt.Sprintf(" error=%s", entry.Error)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func dispatchReasonCodes(suggestion service.DispatchSuggestion) []string {
	codes := make([]string, 0)
	for _, entry := range suggestion.Suggestions {
		for _, code := range entry.ReasonCodes {
			found := false
			for _, existing := range codes {
				if existing == code {
					found = true
					break
				}
			}
			if !found {
				codes = append(codes, code)
			}
		}
	}
	return codes
}
