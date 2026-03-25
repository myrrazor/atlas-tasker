package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/spf13/cobra"
)

func newRunCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "run", Short: "Manage orchestration runs"}
	list := &cobra.Command{Use: "list", Short: "List runs", RunE: runRunList}
	list.Flags().String("ticket", "", "Filter by ticket id")
	list.Flags().String("agent", "", "Filter by agent id")
	list.Flags().String("status", "", "Filter by run status")
	addReadOutputFlags(list, &outputFlags{})

	view := &cobra.Command{Use: "view <RUN-ID>", Args: cobra.ExactArgs(1), Short: "Show one run", RunE: runRunView}
	addReadOutputFlags(view, &outputFlags{})

	dispatch := &cobra.Command{Use: "dispatch <TICKET-ID>", Args: cobra.ExactArgs(1), Short: "Dispatch a ticket to an agent", RunE: runRunDispatch}
	dispatch.Flags().String("agent", "", "Agent id to dispatch to")
	dispatch.Flags().String("kind", string(contracts.RunKindWork), "Run kind: work|review|qa|release")
	_ = dispatch.MarkFlagRequired("agent")
	addMutationFlags(dispatch, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(dispatch, &outputFlags{})

	start := &cobra.Command{Use: "start <RUN-ID>", Args: cobra.ExactArgs(1), Short: "Move a run to active", RunE: runRunStart}
	start.Flags().String("summary", "", "Optional run summary")
	addMutationFlags(start, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(start, &outputFlags{})

	attach := &cobra.Command{Use: "attach <RUN-ID>", Args: cobra.ExactArgs(1), Short: "Attach an external session to a run", RunE: runRunAttach}
	attach.Flags().String("provider", "", "Session provider: codex|claude|human|custom")
	attach.Flags().String("session-ref", "", "External session reference")
	attach.Flags().Bool("replace", false, "Replace an existing conflicting attachment")
	_ = attach.MarkFlagRequired("provider")
	_ = attach.MarkFlagRequired("session-ref")
	addMutationFlags(attach, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(attach, &outputFlags{})

	complete := &cobra.Command{Use: "complete <RUN-ID>", Args: cobra.ExactArgs(1), Short: "Complete a run", RunE: runRunComplete}
	complete.Flags().String("summary", "", "Completion summary")
	addMutationFlags(complete, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(complete, &outputFlags{})

	checkpoint := &cobra.Command{Use: "checkpoint <RUN-ID>", Args: cobra.ExactArgs(1), Short: "Record a checkpoint note for a run", RunE: runRunCheckpoint}
	checkpoint.Flags().String("title", "", "Checkpoint title")
	checkpoint.Flags().String("body", "", "Checkpoint body")
	addMutationFlags(checkpoint, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(checkpoint, &outputFlags{})

	evidence := &cobra.Command{Use: "evidence", Short: "Attach evidence to a run"}
	evidenceAdd := &cobra.Command{Use: "add <RUN-ID>", Args: cobra.ExactArgs(1), Short: "Add evidence to a run", RunE: runRunEvidenceAdd}
	evidenceAdd.Flags().String("type", "", "Evidence type")
	evidenceAdd.Flags().String("title", "", "Evidence title")
	evidenceAdd.Flags().String("body", "", "Evidence body")
	evidenceAdd.Flags().String("artifact", "", "Path to an artifact file to copy into the evidence bundle")
	evidenceAdd.Flags().String("supersedes", "", "Prior evidence id this entry supersedes")
	_ = evidenceAdd.MarkFlagRequired("type")
	addMutationFlags(evidenceAdd, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(evidenceAdd, &outputFlags{})
	evidence.AddCommand(evidenceAdd)

	handoff := &cobra.Command{Use: "handoff <RUN-ID>", Args: cobra.ExactArgs(1), Short: "Generate a handoff packet for a run", RunE: runRunHandoff}
	handoff.Flags().StringArray("open-question", nil, "Open question to include in the packet")
	handoff.Flags().StringArray("risk", nil, "Risk to include in the packet")
	handoff.Flags().String("next-actor", "", "Suggested next actor")
	handoff.Flags().String("next-gate", "", "Suggested next gate")
	handoff.Flags().String("next-status", "", "Suggested next ticket status")
	addMutationFlags(handoff, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(handoff, &outputFlags{})

	fail := &cobra.Command{Use: "fail <RUN-ID>", Args: cobra.ExactArgs(1), Short: "Fail a run", RunE: runRunFail}
	fail.Flags().String("summary", "", "Failure summary")
	addMutationFlags(fail, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(fail, &outputFlags{})

	abort := &cobra.Command{Use: "abort <RUN-ID>", Args: cobra.ExactArgs(1), Short: "Abort a run", RunE: runRunAbort}
	abort.Flags().String("summary", "", "Abort summary")
	addMutationFlags(abort, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(abort, &outputFlags{})

	cleanup := &cobra.Command{Use: "cleanup <RUN-ID>", Args: cobra.ExactArgs(1), Short: "Remove worktree/runtime artifacts for a finished run", RunE: runRunCleanup}
	cleanup.Flags().Bool("force", false, "Remove dirty worktrees too")
	addMutationFlags(cleanup, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(cleanup, &outputFlags{})

	cmd.AddCommand(list, view, dispatch, start, attach, checkpoint, evidence, handoff, complete, fail, abort, cleanup)
	return cmd
}

func newWorktreeCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "worktree", Short: "Inspect managed run worktrees"}
	list := &cobra.Command{Use: "list", Short: "List run worktree status", RunE: runWorktreeList}
	addReadOutputFlags(list, &outputFlags{})
	view := &cobra.Command{Use: "view <RUN-ID>", Args: cobra.ExactArgs(1), Short: "Inspect one run worktree", RunE: runWorktreeView}
	addReadOutputFlags(view, &outputFlags{})
	repair := &cobra.Command{Use: "repair", Short: "Repair git worktree admin files for known runs", RunE: runWorktreeRepair}
	addReadOutputFlags(repair, &outputFlags{})
	prune := &cobra.Command{Use: "prune", Short: "Prune stale git worktree records", RunE: runWorktreePrune}
	addReadOutputFlags(prune, &outputFlags{})
	cmd.AddCommand(list, view, repair, prune)
	return cmd
}

func runRunList(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	ticketID, _ := cmd.Flags().GetString("ticket")
	agentID, _ := cmd.Flags().GetString("agent")
	statusRaw, _ := cmd.Flags().GetString("status")
	runs, err := workspace.queries.ListRuns(commandContext(cmd), ticketID, agentID, contracts.RunStatus(strings.TrimSpace(statusRaw)))
	if err != nil {
		return err
	}
	pretty := formatRunList(runs)
	md := pretty
	data := map[string]any{"kind": "run_list", "generated_at": time.Now().UTC(), "items": runs}
	return writeCommandOutput(cmd, data, md, pretty)
}

func runRunView(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	detail, err := workspace.queries.RunDetail(commandContext(cmd), args[0])
	if err != nil {
		return err
	}
	pretty := formatRunDetail(detail)
	data := map[string]any{"kind": "run_detail", "generated_at": detail.GeneratedAt, "payload": detail}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runRunDispatch(cmd *cobra.Command, args []string) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	agentID, _ := cmd.Flags().GetString("agent")
	kindRaw, _ := cmd.Flags().GetString("kind")
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	result, err := workspace.actions.DispatchRun(ctx, args[0], agentID, contracts.RunKind(strings.TrimSpace(kindRaw)), normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	pretty := fmt.Sprintf("dispatched %s to %s as %s", result.RunID, result.AgentID, result.TicketID)
	if result.WorktreePath != "" {
		pretty += fmt.Sprintf("\nworktree=%s", result.WorktreePath)
	}
	data := map[string]any{"kind": "run_dispatch_result", "generated_at": result.GeneratedAt, "payload": result}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runRunStart(cmd *cobra.Command, args []string) error {
	return mutateRunDetail(cmd, args[0], func(ctx context.Context, workspace *workspace, actor contracts.Actor, reason string) (contracts.RunSnapshot, string, error) {
		summary, _ := cmd.Flags().GetString("summary")
		run, err := workspace.actions.StartRun(ctx, args[0], actor, reason, summary)
		return run, fmt.Sprintf("started %s", run.RunID), err
	})
}

func runRunAttach(cmd *cobra.Command, args []string) error {
	return mutateRun(cmd, args[0], "run_attach_result", func(ctx context.Context, workspace *workspace, actor contracts.Actor, reason string) (contracts.RunSnapshot, error) {
		providerRaw, _ := cmd.Flags().GetString("provider")
		sessionRef, _ := cmd.Flags().GetString("session-ref")
		replace, _ := cmd.Flags().GetBool("replace")
		return workspace.actions.AttachRun(ctx, args[0], contracts.AgentProvider(strings.TrimSpace(providerRaw)), sessionRef, replace, actor, reason)
	})
}

func runRunComplete(cmd *cobra.Command, args []string) error {
	return mutateRunDetail(cmd, args[0], func(ctx context.Context, workspace *workspace, actor contracts.Actor, reason string) (contracts.RunSnapshot, string, error) {
		summary, _ := cmd.Flags().GetString("summary")
		run, err := workspace.actions.CompleteRun(ctx, args[0], actor, reason, summary)
		return run, fmt.Sprintf("completed %s", run.RunID), err
	})
}

func runRunCheckpoint(cmd *cobra.Command, args []string) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	title, _ := cmd.Flags().GetString("title")
	body, _ := cmd.Flags().GetString("body")
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	item, err := workspace.actions.CheckpointRun(ctx, args[0], normalizeActor(actorRaw), reason, title, body)
	if err != nil {
		return err
	}
	pretty := formatEvidenceDetail(item)
	data := map[string]any{"kind": "evidence_detail", "generated_at": item.CreatedAt, "payload": item}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runRunEvidenceAdd(cmd *cobra.Command, args []string) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	typeRaw, _ := cmd.Flags().GetString("type")
	title, _ := cmd.Flags().GetString("title")
	body, _ := cmd.Flags().GetString("body")
	artifact, _ := cmd.Flags().GetString("artifact")
	supersedes, _ := cmd.Flags().GetString("supersedes")
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	item, err := workspace.actions.AddEvidence(ctx, args[0], contracts.EvidenceType(strings.TrimSpace(typeRaw)), title, body, artifact, supersedes, normalizeActor(actorRaw), reason, contracts.EventRunEvidenceAdded)
	if err != nil {
		return err
	}
	pretty := formatEvidenceDetail(item)
	data := map[string]any{"kind": "evidence_detail", "generated_at": item.CreatedAt, "payload": item}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runRunHandoff(cmd *cobra.Command, args []string) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	openQuestions, _ := cmd.Flags().GetStringArray("open-question")
	risks, _ := cmd.Flags().GetStringArray("risk")
	nextActor, _ := cmd.Flags().GetString("next-actor")
	nextGateRaw, _ := cmd.Flags().GetString("next-gate")
	nextStatusRaw, _ := cmd.Flags().GetString("next-status")
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	packet, err := workspace.actions.CreateHandoff(
		ctx,
		args[0],
		normalizeActor(actorRaw),
		reason,
		openQuestions,
		risks,
		nextActor,
		contracts.GateKind(strings.TrimSpace(nextGateRaw)),
		contracts.Status(strings.TrimSpace(nextStatusRaw)),
	)
	if err != nil {
		return err
	}
	pretty := service.RenderHandoffMarkdown(packet)
	data := map[string]any{"kind": "handoff_detail", "generated_at": packet.GeneratedAt, "payload": packet}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runRunFail(cmd *cobra.Command, args []string) error {
	return mutateRunDetail(cmd, args[0], func(ctx context.Context, workspace *workspace, actor contracts.Actor, reason string) (contracts.RunSnapshot, string, error) {
		summary, _ := cmd.Flags().GetString("summary")
		run, err := workspace.actions.FailRun(ctx, args[0], actor, reason, summary)
		return run, fmt.Sprintf("failed %s", run.RunID), err
	})
}

func runRunAbort(cmd *cobra.Command, args []string) error {
	return mutateRunDetail(cmd, args[0], func(ctx context.Context, workspace *workspace, actor contracts.Actor, reason string) (contracts.RunSnapshot, string, error) {
		summary, _ := cmd.Flags().GetString("summary")
		run, err := workspace.actions.AbortRun(ctx, args[0], actor, reason, summary)
		return run, fmt.Sprintf("aborted %s", run.RunID), err
	})
}

func runRunCleanup(cmd *cobra.Command, args []string) error {
	return mutateRunDetail(cmd, args[0], func(ctx context.Context, workspace *workspace, actor contracts.Actor, reason string) (contracts.RunSnapshot, string, error) {
		force, _ := cmd.Flags().GetBool("force")
		run, err := workspace.actions.CleanupRun(ctx, args[0], force, actor, reason)
		return run, fmt.Sprintf("cleaned up %s", run.RunID), err
	})
}

func runWorktreeList(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	items, err := workspace.queries.WorktreeList(commandContext(cmd))
	if err != nil {
		return err
	}
	pretty := formatWorktreeList(items)
	data := map[string]any{"kind": "worktree_list", "generated_at": time.Now().UTC(), "items": items}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runWorktreeView(cmd *cobra.Command, args []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	item, err := workspace.queries.WorktreeDetail(commandContext(cmd), args[0])
	if err != nil {
		return err
	}
	pretty := formatWorktreeDetail(item)
	data := map[string]any{"kind": "worktree_detail", "generated_at": time.Now().UTC(), "payload": item}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runWorktreeRepair(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	runs, err := workspace.queries.ListRuns(commandContext(cmd), "", "", "")
	if err != nil {
		return err
	}
	items, err := service.WorktreeManager{Root: workspace.root}.Repair(commandContext(cmd), runs)
	if err != nil {
		return err
	}
	pretty := formatWorktreeList(items)
	data := map[string]any{"kind": "worktree_repair_result", "generated_at": time.Now().UTC(), "payload": map[string]any{"action": "repair", "items": items}}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func runWorktreePrune(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	runs, err := workspace.queries.ListRuns(commandContext(cmd), "", "", "")
	if err != nil {
		return err
	}
	items, err := service.WorktreeManager{Root: workspace.root}.Prune(commandContext(cmd), runs)
	if err != nil {
		return err
	}
	pretty := formatWorktreeList(items)
	data := map[string]any{"kind": "worktree_repair_result", "generated_at": time.Now().UTC(), "payload": map[string]any{"action": "prune", "items": items}}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func mutateRun(cmd *cobra.Command, runID string, kind string, fn func(context.Context, *workspace, contracts.Actor, string) (contracts.RunSnapshot, error)) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	run, err := fn(ctx, workspace, normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	pretty := fmt.Sprintf("%s %s", run.Status, run.RunID)
	data := map[string]any{"kind": kind, "generated_at": time.Now().UTC(), "payload": run}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func mutateRunDetail(cmd *cobra.Command, runID string, fn func(context.Context, *workspace, contracts.Actor, string) (contracts.RunSnapshot, string, error)) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	run, pretty, err := fn(ctx, workspace, normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	if pretty == "" {
		pretty = fmt.Sprintf("%s %s", run.Status, runID)
	}
	data := map[string]any{"kind": "run_detail", "generated_at": time.Now().UTC(), "payload": run}
	return writeCommandOutput(cmd, data, pretty, pretty)
}

func formatRunList(runs []contracts.RunSnapshot) string {
	if len(runs) == 0 {
		return "no runs"
	}
	lines := make([]string, 0, len(runs)+1)
	lines = append(lines, "runs:")
	for _, run := range runs {
		line := fmt.Sprintf("- %s [%s] ticket=%s agent=%s", run.RunID, run.Status, run.TicketID, run.AgentID)
		if run.WorktreePath != "" {
			line += fmt.Sprintf(" worktree=%s", run.WorktreePath)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func formatRunDetail(detail service.RunDetailView) string {
	lines := []string{
		fmt.Sprintf("run %s", detail.Run.RunID),
		fmt.Sprintf("status=%s kind=%s", detail.Run.Status, detail.Run.Kind),
		fmt.Sprintf("ticket=%s project=%s", detail.Run.TicketID, detail.Run.Project),
		fmt.Sprintf("agent=%s provider=%s", detail.Run.AgentID, detail.Run.Provider),
	}
	if detail.Run.WorktreePath != "" {
		lines = append(lines, fmt.Sprintf("worktree=%s", detail.Run.WorktreePath))
	}
	if detail.Run.BranchName != "" {
		lines = append(lines, fmt.Sprintf("branch=%s", detail.Run.BranchName))
	}
	if detail.Run.Summary != "" {
		lines = append(lines, "", detail.Run.Summary)
	}
	if len(detail.Gates) > 0 {
		lines = append(lines, "", fmt.Sprintf("gates=%d", len(detail.Gates)))
		for _, gate := range detail.Gates {
			lines = append(lines, fmt.Sprintf("- %s [%s/%s]", gate.GateID, gate.Kind, gate.State))
		}
	}
	if len(detail.Evidence) > 0 {
		lines = append(lines, "", fmt.Sprintf("evidence=%d", len(detail.Evidence)))
		for _, item := range detail.Evidence {
			lines = append(lines, fmt.Sprintf("- %s [%s] %s", item.EvidenceID, item.Type, item.Title))
		}
	}
	if len(detail.Handoffs) > 0 {
		lines = append(lines, "", fmt.Sprintf("handoffs=%d", len(detail.Handoffs)))
		for _, item := range detail.Handoffs {
			lines = append(lines, fmt.Sprintf("- %s next_actor=%s next_gate=%s", item.HandoffID, item.SuggestedNextActor, item.SuggestedNextGate))
		}
	}
	return strings.Join(lines, "\n")
}

func formatEvidenceList(items []contracts.EvidenceItem) string {
	if len(items) == 0 {
		return "no evidence"
	}
	lines := make([]string, 0, len(items)+1)
	lines = append(lines, "evidence:")
	for _, item := range items {
		line := fmt.Sprintf("- %s [%s]", item.EvidenceID, item.Type)
		if item.Title != "" {
			line += " " + item.Title
		}
		if item.ArtifactPath != "" {
			line += " artifact=" + item.ArtifactPath
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func formatEvidenceDetail(item contracts.EvidenceItem) string {
	lines := []string{
		fmt.Sprintf("evidence %s", item.EvidenceID),
		fmt.Sprintf("run=%s ticket=%s type=%s", item.RunID, item.TicketID, item.Type),
		fmt.Sprintf("actor=%s created_at=%s", item.Actor, item.CreatedAt.UTC().Format(time.RFC3339)),
	}
	if item.Title != "" {
		lines = append(lines, "title="+item.Title)
	}
	if item.SupersedesEvidenceID != "" {
		lines = append(lines, "supersedes="+item.SupersedesEvidenceID)
	}
	if item.ArtifactPath != "" {
		lines = append(lines, "artifact="+item.ArtifactPath)
	}
	if item.Body != "" {
		lines = append(lines, "", item.Body)
	}
	return strings.Join(lines, "\n")
}

func formatWorktreeList(items []service.WorktreeStatusView) string {
	if len(items) == 0 {
		return "no managed worktrees"
	}
	lines := make([]string, 0, len(items)+1)
	lines = append(lines, "worktrees:")
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("- %s present=%t dirty=%t path=%s", item.RunID, item.Present, item.Dirty, item.Path))
	}
	return strings.Join(lines, "\n")
}

func formatWorktreeDetail(item service.WorktreeStatusView) string {
	return fmt.Sprintf("run=%s\npresent=%t\ndirty=%t\npath=%s\nbranch=%s", item.RunID, item.Present, item.Dirty, item.Path, item.BranchName)
}
