package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/spf13/cobra"
)

func newGateCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "gate", Short: "Inspect and resolve approval gates"}
	list := &cobra.Command{Use: "list", Short: "List gates", RunE: runGateList}
	list.Flags().String("ticket", "", "Filter by ticket id")
	list.Flags().String("run", "", "Filter by run id")
	list.Flags().String("state", "", "Filter by gate state")
	addReadOutputFlags(list, &outputFlags{})

	view := &cobra.Command{Use: "view <GATE-ID>", Args: cobra.ExactArgs(1), Short: "Show one gate", RunE: runGateView}
	addReadOutputFlags(view, &outputFlags{})

	approve := &cobra.Command{Use: "approve <GATE-ID>", Args: cobra.ExactArgs(1), Short: "Approve a gate", RunE: runGateApprove}
	addMutationFlags(approve, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(approve, &outputFlags{})

	reject := &cobra.Command{Use: "reject <GATE-ID>", Args: cobra.ExactArgs(1), Short: "Reject a gate", RunE: runGateReject}
	addMutationFlags(reject, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(reject, &outputFlags{})

	waive := &cobra.Command{Use: "waive <GATE-ID>", Args: cobra.ExactArgs(1), Short: "Waive a gate", RunE: runGateWaive}
	addMutationFlags(waive, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(waive, &outputFlags{})

	cmd.AddCommand(list, view, approve, reject, waive)
	return cmd
}

func newApprovalsCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "approvals", Short: "List open approval work", RunE: runApprovals}
	cmd.Flags().String("collaborator", "", "Filter approval work for one collaborator")
	addReadOutputFlags(cmd, &outputFlags{})
	return cmd
}

func newInboxCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "inbox", Short: "List derived operator inbox items", RunE: runInbox}
	cmd.Flags().String("collaborator", "", "Filter inbox items for one collaborator")
	addReadOutputFlags(cmd, &outputFlags{})
	view := &cobra.Command{Use: "view <ITEM-ID>", Args: cobra.ExactArgs(1), Short: "Show one inbox item", RunE: runInboxView}
	addReadOutputFlags(view, &outputFlags{})
	cmd.AddCommand(view)
	return cmd
}

func runGateList(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	ticketID, _ := cmd.Flags().GetString("ticket")
	runID, _ := cmd.Flags().GetString("run")
	stateRaw, _ := cmd.Flags().GetString("state")
	items, err := workspace.queries.GateList(commandContext(cmd), ticketID, runID, contracts.GateState(strings.TrimSpace(stateRaw)))
	if err != nil {
		return err
	}
	pretty := formatGateList(items)
	data := map[string]any{"kind": "gate_list", "generated_at": time.Now().UTC(), "items": items}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runGateView(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	gate, err := workspace.queries.GateDetail(commandContext(cmd), args[0])
	if err != nil {
		return err
	}
	pretty := service.RenderGateMarkdown(gate)
	data := map[string]any{"kind": "gate_detail", "generated_at": gate.CreatedAt, "payload": gate}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runGateApprove(cmd *cobra.Command, args []string) error {
	return mutateGate(cmd, args[0], func(workspace *workspace, actor contracts.Actor, reason string) (contracts.GateSnapshot, error) {
		return workspace.actions.ApproveGate(commandContext(cmd), args[0], actor, reason)
	})
}

func runGateReject(cmd *cobra.Command, args []string) error {
	return mutateGate(cmd, args[0], func(workspace *workspace, actor contracts.Actor, reason string) (contracts.GateSnapshot, error) {
		return workspace.actions.RejectGate(commandContext(cmd), args[0], actor, reason)
	})
}

func runGateWaive(cmd *cobra.Command, args []string) error {
	return mutateGate(cmd, args[0], func(workspace *workspace, actor contracts.Actor, reason string) (contracts.GateSnapshot, error) {
		return workspace.actions.WaiveGate(commandContext(cmd), args[0], actor, reason)
	})
}

func mutateGate(cmd *cobra.Command, gateID string, fn func(*workspace, contracts.Actor, string) (contracts.GateSnapshot, error)) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	gate, err := fn(workspace, normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	pretty := service.RenderGateMarkdown(gate)
	data := map[string]any{"kind": "gate_detail", "generated_at": gate.CreatedAt, "payload": gate}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runApprovals(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	collaboratorID, _ := cmd.Flags().GetString("collaborator")
	items, err := workspace.queries.Approvals(commandContext(cmd), collaboratorID)
	if err != nil {
		return err
	}
	pretty := formatApprovals(items)
	data := map[string]any{"kind": "approvals_list", "generated_at": time.Now().UTC(), "items": items}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runInbox(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	collaboratorID, _ := cmd.Flags().GetString("collaborator")
	items, err := workspace.queries.Inbox(commandContext(cmd), collaboratorID)
	if err != nil {
		return err
	}
	pretty := formatInbox(items)
	data := map[string]any{"kind": "inbox_list", "generated_at": time.Now().UTC(), "items": items}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runInboxView(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	detail, err := workspace.queries.InboxDetail(commandContext(cmd), args[0])
	if err != nil {
		return err
	}
	pretty := formatInboxDetail(detail)
	data := map[string]any{"kind": "inbox_detail", "generated_at": detail.Generated, "payload": detail}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func formatGateList(items []contracts.GateSnapshot) string {
	if len(items) == 0 {
		return "no gates"
	}
	lines := []string{"gates:"}
	for _, gate := range items {
		lines = append(lines, fmt.Sprintf("- %s [%s/%s] ticket=%s run=%s", gate.GateID, gate.Kind, gate.State, gate.TicketID, gate.RunID))
	}
	return strings.Join(lines, "\n")
}

func formatApprovals(items []service.ApprovalItemView) string {
	if len(items) == 0 {
		return "no approvals"
	}
	lines := []string{"approvals:"}
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("- %s ticket=%s %s", item.Gate.GateID, item.Ticket.ID, item.Summary))
	}
	return strings.Join(lines, "\n")
}

func formatInbox(items []service.InboxItemView) string {
	if len(items) == 0 {
		return "inbox is empty"
	}
	lines := []string{"inbox:"}
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("- %s [%s] %s", item.ID, item.Kind, item.Summary))
	}
	return strings.Join(lines, "\n")
}

func formatInboxDetail(detail service.InboxDetailView) string {
	lines := []string{
		fmt.Sprintf("inbox %s", detail.Item.ID),
		fmt.Sprintf("kind=%s state=%s ticket=%s", detail.Item.Kind, detail.Item.State, detail.Item.TicketID),
		detail.Item.Summary,
	}
	if detail.Item.GateID != "" {
		lines = append(lines, "", service.RenderGateMarkdown(detail.Gate))
	}
	if detail.Item.HandoffID != "" {
		lines = append(lines, "", service.RenderHandoffMarkdown(detail.Handoff))
	}
	if detail.Item.MentionUID != "" {
		lines = append(lines, "", fmt.Sprintf("mention=@%s via %s %s", detail.Mention.CollaboratorID, detail.Mention.SourceKind, detail.Mention.SourceID))
	}
	return strings.Join(lines, "\n")
}
