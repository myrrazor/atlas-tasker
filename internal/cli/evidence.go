package cli

import (
	"fmt"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/spf13/cobra"
)

func newEvidenceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "evidence", Short: "Inspect run evidence"}
	list := &cobra.Command{Use: "list <RUN-ID>", Args: cobra.ExactArgs(1), Short: "List evidence for a run", RunE: runEvidenceList}
	addReadOutputFlags(list, &outputFlags{})
	view := &cobra.Command{Use: "view <EVIDENCE-ID>", Args: cobra.ExactArgs(1), Short: "Show one evidence item", RunE: runEvidenceView}
	addReadOutputFlags(view, &outputFlags{})
	cmd.AddCommand(list, view)
	return cmd
}

func newHandoffCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "handoff", Short: "Inspect exported handoff packets"}
	view := &cobra.Command{Use: "view <HANDOFF-ID>", Args: cobra.ExactArgs(1), Short: "Show one handoff packet", RunE: runHandoffView}
	addReadOutputFlags(view, &outputFlags{})
	export := &cobra.Command{Use: "export <HANDOFF-ID>", Args: cobra.ExactArgs(1), Short: "Render one handoff packet as markdown", RunE: runHandoffExport}
	addReadOutputFlags(export, &outputFlags{})
	cmd.AddCommand(view, export)
	return cmd
}

func runEvidenceList(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	items, err := workspace.queries.EvidenceList(commandContext(cmd), args[0])
	if err != nil {
		return err
	}
	pretty := formatEvidenceList(items)
	data := map[string]any{"kind": "evidence_list", "generated_at": defaultNow(), "items": items}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runEvidenceView(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	item, err := workspace.queries.EvidenceDetail(commandContext(cmd), args[0])
	if err != nil {
		return err
	}
	pretty := formatEvidenceDetail(item)
	data := map[string]any{"kind": "evidence_detail", "generated_at": item.CreatedAt, "payload": item}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runHandoffView(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	view, err := workspace.queries.HandoffView(commandContext(cmd), args[0])
	if err != nil {
		return err
	}
	pretty := formatHandoffDetail(view)
	data := map[string]any{
		"kind":         "handoff_detail",
		"generated_at": view.GeneratedAt,
		"payload": map[string]any{
			"handoff_id":                   view.Handoff.HandoffID,
			"source_run_id":                view.Handoff.SourceRunID,
			"ticket_id":                    view.Handoff.TicketID,
			"actor":                        view.Handoff.Actor,
			"status_summary":               view.Handoff.StatusSummary,
			"changed_files":                view.Handoff.ChangedFiles,
			"commit_refs":                  view.Handoff.CommitRefs,
			"tests":                        view.Handoff.Tests,
			"evidence_links":               view.Handoff.EvidenceLinks,
			"open_questions":               view.Handoff.OpenQuestions,
			"risks":                        view.Handoff.Risks,
			"suggested_next_actor":         view.Handoff.SuggestedNextActor,
			"suggested_next_gate":          view.Handoff.SuggestedNextGate,
			"suggested_next_ticket_status": view.Handoff.SuggestedNextTicketStatus,
			"generated_at":                 view.Handoff.GeneratedAt,
			"schema_version":               view.Handoff.SchemaVersion,
			"changes":                      view.Changes,
			"checks":                       view.Checks,
		},
	}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runHandoffExport(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	packet, err := workspace.queries.HandoffDetail(commandContext(cmd), args[0])
	if err != nil {
		return err
	}
	markdown := service.RenderHandoffMarkdown(packet)
	pretty := markdown
	data := map[string]any{
		"kind":         "handoff_export",
		"generated_at": packet.GeneratedAt,
		"payload": map[string]any{
			"handoff":  packet,
			"markdown": markdown,
		},
	}
	return writeCommandOutput(cmd, data, markdown, pretty)
}

func formatHandoffShort(packetID string, runID string, ticketID string) string {
	return fmt.Sprintf("handoff %s run=%s ticket=%s", packetID, runID, ticketID)
}

func formatHandoffDetail(view service.HandoffContextView) string {
	lines := []string{service.RenderHandoffMarkdown(view.Handoff)}
	if len(view.Changes) > 0 {
		lines = append(lines, "", "## Changes", "")
		for _, change := range view.Changes {
			line := fmt.Sprintf("- %s [%s]", change.ChangeID, change.Status)
			if change.BranchName != "" {
				line += " branch=" + change.BranchName
			}
			lines = append(lines, line)
		}
	}
	if len(view.Checks) > 0 {
		lines = append(lines, "", "## Checks", "")
		for _, check := range view.Checks {
			lines = append(lines, fmt.Sprintf("- %s [%s/%s] %s", check.CheckID, check.Status, check.Conclusion, check.Name))
		}
	}
	return strings.Join(lines, "\n")
}
