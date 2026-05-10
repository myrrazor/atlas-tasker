package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

func ToolSpecs() []ToolSpec {
	readProfiles := []ToolProfile{ProfileRead, ProfileWorkflow, ProfileDelivery, ProfileAdmin}
	workflowProfiles := []ToolProfile{ProfileWorkflow, ProfileDelivery, ProfileAdmin}
	deliveryProfiles := []ToolProfile{ProfileDelivery, ProfileAdmin}
	adminProfiles := []ToolProfile{ProfileAdmin}
	deliveryHighProfiles := []ToolProfile{ProfileDelivery, ProfileAdmin}

	return []ToolSpec{
		readTool("atlas.queue", "Read the actor queue.", readProfiles, objectSchema(nil, mergeProps(commonReadProps(), map[string]any{"actor": stringProp("Optional actor filter.")})), "QueryService.Queue", queueTool),
		readTool("atlas.next", "Read the next recommended ticket for an actor.", readProfiles, objectSchema(nil, mergeProps(commonReadProps(), map[string]any{"actor": stringProp("Optional actor filter.")})), "QueryService.Next", nextTool),
		readTool("atlas.agent.available", "Read tickets the selected agent can act on now.", readProfiles, objectSchema(nil, mergeProps(commonReadProps(), map[string]any{"actor": stringProp("Optional actor such as agent:builder-1."), "agent_id": stringProp("Optional agent ID; maps to actor agent:<id>.")})), "QueryService.AgentAvailable", agentAvailableTool),
		readTool("atlas.agent.pending", "Read tickets the selected agent is waiting on.", readProfiles, objectSchema(nil, mergeProps(commonReadProps(), map[string]any{"actor": stringProp("Optional actor such as agent:builder-1."), "agent_id": stringProp("Optional agent ID; maps to actor agent:<id>.")})), "QueryService.AgentPending", agentPendingTool),
		readTool("atlas.search", "Search tickets with Atlas query syntax.", readProfiles, objectSchema([]string{"query"}, mergeProps(commonReadProps(), map[string]any{"query": stringProp("Atlas ticket search query.")})), "QueryService.Search", searchTool),
		readTool("atlas.board", "Read the board grouped by status.", readProfiles, objectSchema(nil, mergeProps(groupedReadProps("cursor_by_status", "Optional per-status cursors keyed by Atlas status."), map[string]any{"project": stringProp("Optional project key."), "assignee": stringProp("Optional assignee actor."), "type": stringProp("Optional ticket type.")})), "QueryService.Board", boardTool),
		readTool("atlas.ticket.view", "Read one ticket detail view.", readProfiles, objectSchema([]string{"ticket_id"}, map[string]any{"ticket_id": stringProp("Ticket ID.")}), "QueryService.TicketDetail", ticketViewTool),
		readTool("atlas.ticket.history", "Read ticket event history.", readProfiles, objectSchema([]string{"ticket_id"}, mergeProps(commonReadProps(), map[string]any{"ticket_id": stringProp("Ticket ID.")})), "QueryService.History", ticketHistoryTool),
		readTool("atlas.ticket.inspect", "Inspect a ticket, policy, links, and git context.", readProfiles, objectSchema([]string{"ticket_id"}, map[string]any{"ticket_id": stringProp("Ticket ID."), "actor": stringProp("Optional actor for policy context.")}), "QueryService.InspectTicket", ticketInspectTool),
		readTool("atlas.dashboard", "Read the delivery dashboard summary.", readProfiles, objectSchema(nil, mergeProps(groupedReadProps("cursor_by_section", "Optional per-dashboard-section cursors keyed by section name."), map[string]any{"collaborator": stringProp("Optional collaborator filter.")})), "QueryService.Dashboard", dashboardTool),
		readTool("atlas.timeline", "Read a ticket timeline.", readProfiles, objectSchema([]string{"ticket_id"}, mergeProps(commonReadProps(), map[string]any{"ticket_id": stringProp("Ticket ID."), "collaborator": stringProp("Optional collaborator filter.")})), "QueryService.Timeline", timelineTool),
		readTool("atlas.run.view", "Read one run detail view.", readProfiles, objectSchema([]string{"run_id"}, map[string]any{"run_id": stringProp("Run ID.")}), "QueryService.RunDetail", runViewTool),
		readTool("atlas.evidence.list", "List evidence for a run.", readProfiles, objectSchema([]string{"run_id"}, mergeProps(commonReadProps(), map[string]any{"run_id": stringProp("Run ID.")})), "QueryService.EvidenceList", evidenceListTool),
		readTool("atlas.evidence.view", "Read one evidence record.", readProfiles, objectSchema([]string{"evidence_id"}, map[string]any{"evidence_id": stringProp("Evidence ID.")}), "QueryService.EvidenceDetail", evidenceViewTool),
		readTool("atlas.handoff.view", "Read one handoff packet.", readProfiles, objectSchema([]string{"handoff_id"}, map[string]any{"handoff_id": stringProp("Handoff ID.")}), "QueryService.HandoffDetail", handoffViewTool),
		readTool("atlas.approvals", "List open approval work.", readProfiles, objectSchema(nil, mergeProps(commonReadProps(), map[string]any{"collaborator": stringProp("Optional collaborator filter.")})), "QueryService.Approvals", approvalsTool),
		readTool("atlas.inbox", "List human inbox items.", readProfiles, objectSchema(nil, mergeProps(commonReadProps(), map[string]any{"collaborator": stringProp("Optional collaborator filter.")})), "QueryService.Inbox", inboxTool),
		readTool("atlas.change.status", "Read observed local/provider status for one change.", readProfiles, objectSchema([]string{"change_id"}, map[string]any{"change_id": stringProp("Change ID.")}), "QueryService.ChangeStatus", changeStatusTool),
		readTool("atlas.checks.list", "List checks by scope.", readProfiles, objectSchema([]string{"scope", "id"}, mergeProps(commonReadProps(), map[string]any{"scope": stringProp("Scope: run, change, or ticket."), "id": stringProp("Scope ID.")})), "QueryService.ListChecks", checksListTool),
		readTool("atlas.sync.status", "Read sync remote health and latest job state.", readProfiles, objectSchema(nil, mergeProps(commonReadProps(), map[string]any{"remote_id": stringProp("Optional remote ID.")})), "QueryService.SyncStatus", syncStatusTool),
		readTool("atlas.conflict.list", "List sync conflicts.", readProfiles, objectSchema(nil, mergeProps(commonReadProps(), nil)), "QueryService.ListConflicts", conflictListTool),
		readTool("atlas.conflict.view", "Read one sync conflict.", readProfiles, objectSchema([]string{"conflict_id"}, map[string]any{"conflict_id": stringProp("Conflict ID.")}), "QueryService.ConflictDetail", conflictViewTool),
		readTool("atlas.archive.plan", "Preview archive candidates without mutation.", readProfiles, objectSchema([]string{"target"}, map[string]any{"target": stringProp("Retention target."), "project": stringProp("Optional project key.")}), "QueryService.ArchivePlan", archivePlanTool),
		readTool("atlas.dispatch.suggest", "Preview deterministic dispatch suggestions for a ticket.", readProfiles, objectSchema([]string{"ticket_id"}, map[string]any{"ticket_id": stringProp("Ticket ID.")}), "QueryService.DispatchSuggest", dispatchSuggestTool),
		readTool("atlas.dispatch.plan", "Alias for dispatch suggestion used as a dry-run plan.", readProfiles, objectSchema([]string{"ticket_id"}, map[string]any{"ticket_id": stringProp("Ticket ID.")}), "QueryService.DispatchSuggest", dispatchSuggestTool),
		readTool("atlas.change.merge_plan", "Preview readiness and provider state before merging a change.", readProfiles, objectSchema([]string{"change_id"}, map[string]any{"change_id": stringProp("Change ID.")}), "QueryService.ChangeStatus", changeMergePlanTool),
		readTool("atlas.sync.pull_plan", "Preview remote sync state before a pull.", readProfiles, objectSchema(nil, map[string]any{"remote_id": stringProp("Optional remote ID.")}), "QueryService.SyncStatus", syncPullPlanTool),
		readTool("atlas.bundle.import_plan", "Read bundle/import target details before importing a sync bundle.", readProfiles, objectSchema([]string{"bundle_ref"}, map[string]any{"bundle_ref": stringProp("Bundle ID or path.")}), "QueryService.BundleDetail", bundleImportPlanTool),
		readTool("atlas.archive.apply_plan", "Preview archive candidates before applying an archive operation.", readProfiles, objectSchema([]string{"target"}, map[string]any{"target": stringProp("Retention target."), "project": stringProp("Optional project key.")}), "QueryService.ArchivePlan", archivePlanTool),
		readTool("atlas.import.apply_plan", "Read an import preview job before applying it.", readProfiles, objectSchema([]string{"job_id"}, map[string]any{"job_id": stringProp("Import job ID.")}), "QueryService.ImportJobDetail", importApplyPlanTool),
		readTool("atlas.compact_plan", "Preview compactable local-only runtime files.", readProfiles, objectSchema(nil, map[string]any{}), "QueryService.CompactPlan", compactPlanTool),
		readTool("atlas.worktree.cleanup_plan", "Preview worktree/runtime state before cleanup.", readProfiles, objectSchema([]string{"run_id"}, map[string]any{"run_id": stringProp("Run ID.")}), "QueryService.WorktreeDetail", worktreeCleanupPlanTool),

		writeTool("atlas.ticket.comment", ClassWorkflow, workflowProfiles, false, "Comment on a ticket.", objectSchema([]string{"ticket_id", "body", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"ticket_id": stringProp("Ticket ID."), "body": stringProp("Comment body.")})), "ActionService.CommentTicket", "ticket_id", ticketCommentTool),
		writeTool("atlas.ticket.claim", ClassWorkflow, workflowProfiles, false, "Claim a ticket lease.", objectSchema([]string{"ticket_id", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"ticket_id": stringProp("Ticket ID.")})), "ActionService.ClaimTicket", "ticket_id", ticketClaimTool),
		writeTool("atlas.ticket.release", ClassWorkflow, workflowProfiles, false, "Release a ticket lease.", objectSchema([]string{"ticket_id", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"ticket_id": stringProp("Ticket ID.")})), "ActionService.ReleaseTicket", "ticket_id", ticketReleaseTool),
		writeTool("atlas.ticket.move", ClassWorkflow, workflowProfiles, false, "Move a ticket among non-terminal workflow statuses.", objectSchema([]string{"ticket_id", "status", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"ticket_id": stringProp("Ticket ID."), "status": stringProp("Target status."), "override_deps": boolProp("Owner-only dependency override.")})), "ActionService.MoveTicket", "ticket_id", ticketMoveTool),
		writeTool("atlas.ticket.request_review", ClassWorkflow, workflowProfiles, false, "Request review for a ticket.", objectSchema([]string{"ticket_id", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"ticket_id": stringProp("Ticket ID."), "override_deps": boolProp("Owner-only dependency override.")})), "ActionService.RequestReview", "ticket_id", ticketRequestReviewTool),
		writeTool("atlas.gate.approve", ClassWorkflow, workflowProfiles, false, "Approve a normal approval gate.", objectSchema([]string{"gate_id", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"gate_id": stringProp("Gate ID.")})), "ActionService.ApproveGate", "gate_id", gateApproveTool),
		writeTool("atlas.gate.reject", ClassWorkflow, workflowProfiles, false, "Reject a normal approval gate.", objectSchema([]string{"gate_id", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"gate_id": stringProp("Gate ID.")})), "ActionService.RejectGate", "gate_id", gateRejectTool),
		writeTool("atlas.run.checkpoint", ClassWorkflow, workflowProfiles, false, "Add a checkpoint evidence item to a run.", objectSchema([]string{"run_id", "title", "body", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"run_id": stringProp("Run ID."), "title": stringProp("Checkpoint title."), "body": stringProp("Checkpoint body.")})), "ActionService.CheckpointRun", "run_id", runCheckpointTool),
		writeTool("atlas.evidence.add", ClassWorkflow, workflowProfiles, false, "Add evidence metadata to a run.", objectSchema([]string{"run_id", "type", "title", "body", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"run_id": stringProp("Run ID."), "type": stringProp("Evidence type."), "title": stringProp("Evidence title."), "body": stringProp("Evidence body."), "artifact_source": stringProp("Optional artifact path or source."), "supersedes_evidence_id": stringProp("Optional evidence ID this supersedes.")})), "ActionService.AddEvidence", "run_id", evidenceAddTool),
		writeTool("atlas.handoff.create", ClassWorkflow, workflowProfiles, false, "Create a handoff packet for a run.", objectSchema([]string{"run_id", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"run_id": stringProp("Run ID."), "open_questions": stringArrayProp("Open questions."), "risks": stringArrayProp("Risks."), "next_actor": stringProp("Optional next actor."), "next_gate": stringProp("Optional next gate kind."), "next_status": stringProp("Optional next ticket status.")})), "ActionService.CreateHandoff", "run_id", handoffCreateTool),
		importPreviewSpec(workflowProfiles),
		writeTool("atlas.dispatch.run", ClassDelivery, deliveryProfiles, false, "Dispatch a ticket to an eligible agent when normal Atlas policy allows it.", objectSchema([]string{"ticket_id", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"ticket_id": stringProp("Ticket ID."), "agent_id": stringProp("Optional agent ID. If absent Atlas auto-routes only when exactly one agent is eligible.")})), "ActionService.DispatchRun/AutoDispatchRun", "ticket_id", dispatchRunTool),
		writeTool("atlas.change.create", ClassDelivery, deliveryProfiles, false, "Create or refresh the change tied to a run.", objectSchema([]string{"run_id", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"run_id": stringProp("Run ID.")})), "ActionService.CreateChange", "run_id", changeCreateTool),
		writeTool("atlas.change.sync", ClassDelivery, deliveryProfiles, false, "Sync provider-backed change status into Atlas.", objectSchema([]string{"change_id", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"change_id": stringProp("Change ID.")})), "ActionService.SyncChange", "change_id", changeSyncTool),
		writeTool("atlas.checks.sync", ClassDelivery, deliveryProfiles, false, "Sync provider checks for a change into Atlas.", objectSchema([]string{"change_id", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"change_id": stringProp("Change ID.")})), "ActionService.SyncChangeChecks", "change_id", checksSyncTool),

		highImpactTool("atlas.change.review_request", deliveryHighProfiles, "Request provider-side review for a change.", objectSchema([]string{"change_id", "actor", "reason", "operation_approval_id", "confirm_text"}, mergeProps(highImpactProps("Change ID."), map[string]any{"change_id": stringProp("Change ID.")})), "ActionService.RequestChangeReview", "change_id", changeReviewRequestTool),
		highImpactTool("atlas.change.merge", deliveryHighProfiles, "Merge a provider-backed change.", objectSchema([]string{"change_id", "actor", "reason", "operation_approval_id", "confirm_text"}, mergeProps(highImpactProps("Change ID."), map[string]any{"change_id": stringProp("Change ID.")})), "ActionService.MergeChange", "change_id", changeMergeTool),
		highImpactTool("atlas.gate.waive", adminProfiles, "Waive an approval gate.", objectSchema([]string{"gate_id", "actor", "reason", "operation_approval_id", "confirm_text"}, mergeProps(highImpactProps("Gate ID."), map[string]any{"gate_id": stringProp("Gate ID.")})), "ActionService.WaiveGate", "gate_id", gateWaiveTool),
		highImpactTool("atlas.ticket.complete", adminProfiles, "Complete a ticket, including protected workflows.", objectSchema([]string{"ticket_id", "actor", "reason", "operation_approval_id", "confirm_text"}, mergeProps(highImpactProps("Ticket ID."), map[string]any{"ticket_id": stringProp("Ticket ID."), "override_deps": boolProp("Owner-only dependency override.")})), "ActionService.CompleteTicket", "ticket_id", ticketCompleteTool),
		highImpactTool("atlas.sync.pull", adminProfiles, "Pull remote state into this workspace.", objectSchema([]string{"remote_id", "actor", "reason", "operation_approval_id", "confirm_text"}, mergeProps(highImpactProps("Remote ID."), map[string]any{"remote_id": stringProp("Remote ID."), "source_workspace_id": stringProp("Source workspace ID when needed.")})), "ActionService.SyncPull", "remote_id", syncPullTool),
		highImpactTool("atlas.sync.push", adminProfiles, "Publish local state to a remote.", objectSchema([]string{"remote_id", "actor", "reason", "operation_approval_id", "confirm_text"}, mergeProps(highImpactProps("Remote ID."), map[string]any{"remote_id": stringProp("Remote ID.")})), "ActionService.SyncPush", "remote_id", syncPushTool),
		highImpactTool("atlas.bundle.import", adminProfiles, "Import a sync bundle into this workspace.", objectSchema([]string{"bundle_ref", "actor", "reason", "operation_approval_id", "confirm_text"}, mergeProps(highImpactProps("Bundle reference."), map[string]any{"bundle_ref": stringProp("Bundle ID or path.")})), "ActionService.ImportSyncBundle", "bundle_ref", bundleImportTool),
		highImpactTool("atlas.import.apply", adminProfiles, "Apply a previewed import job.", objectSchema([]string{"job_id", "actor", "reason", "operation_approval_id", "confirm_text"}, mergeProps(highImpactProps("Import job ID."), map[string]any{"job_id": stringProp("Import job ID.")})), "ActionService.ApplyImport", "job_id", importApplyTool),
		highImpactTool("atlas.archive.apply", adminProfiles, "Archive eligible retained artifacts.", objectSchema([]string{"target", "actor", "reason", "operation_approval_id", "confirm_text"}, mergeProps(highImpactProps("Retention target."), map[string]any{"target": stringProp("Retention target."), "project": stringProp("Optional project key.")})), "ActionService.ApplyArchive", "target", archiveApplyTool),
		highImpactTool("atlas.archive.restore", adminProfiles, "Restore one archive record.", objectSchema([]string{"archive_id", "actor", "reason", "operation_approval_id", "confirm_text"}, mergeProps(highImpactProps("Archive ID."), map[string]any{"archive_id": stringProp("Archive ID.")})), "ActionService.RestoreArchive", "archive_id", archiveRestoreTool),
		highImpactTool("atlas.compact", adminProfiles, "Remove compactable local-only runtime files.", objectSchema([]string{"target", "actor", "reason", "operation_approval_id", "confirm_text"}, mergeProps(highImpactProps("workspace"), map[string]any{"target": stringProp("Must be workspace.")})), "ActionService.CompactWorkspace", "target", compactTool),
		highImpactTool("atlas.worktree.cleanup", adminProfiles, "Remove worktree/runtime artifacts for a finished run.", objectSchema([]string{"run_id", "actor", "reason", "operation_approval_id", "confirm_text"}, mergeProps(highImpactProps("Run ID."), map[string]any{"run_id": stringProp("Run ID."), "force": boolProp("Force cleanup.")})), "ActionService.CleanupRun", "run_id", worktreeCleanupTool),
	}
}

func ToolSpecByName(name string) (ToolSpec, bool) {
	for _, spec := range ToolSpecs() {
		if spec.Name == name {
			return spec, true
		}
	}
	return ToolSpec{}, false
}

func readTool(name string, description string, profiles []ToolProfile, schema map[string]any, underlying string, handler ToolHandler) ToolSpec {
	return ToolSpec{Name: name, Title: name, Description: description, Class: ClassRead, Profiles: profiles, ApprovalMechanism: ApprovalNone, Underlying: underlying, InputSchema: schema, Handler: handler}
}

func writeTool(name string, class ToolClass, profiles []ToolProfile, destructive bool, description string, schema map[string]any, underlying string, targetArg string, handler ToolHandler) ToolSpec {
	return ToolSpec{Name: name, Title: name, Description: description, Class: class, Profiles: profiles, RequiresActor: true, RequiresReason: true, ApprovalMechanism: ApprovalNone, Destructive: destructive, ProviderSideEffect: class == ClassDelivery, TargetArg: targetArg, Underlying: underlying, InputSchema: schema, Handler: handler}
}

func importPreviewSpec(profiles []ToolProfile) ToolSpec {
	spec := writeTool("atlas.import.preview", ClassWorkflow, profiles, false, "Create an import preview job for a local source.", objectSchema([]string{"source_path", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"source_path": stringProp("Import source path.")})), "ActionService.PreviewImport", "source_path", importPreviewTool)
	spec.ProviderSideEffect = true
	return spec
}

func highImpactTool(name string, profiles []ToolProfile, description string, schema map[string]any, underlying string, targetArg string, handler ToolHandler) ToolSpec {
	return ToolSpec{Name: name, Title: name, Description: description, Class: ClassHighImpact, Profiles: profiles, RequiresActor: true, RequiresReason: true, RequiresApproval: true, ApprovalMechanism: ApprovalOperation, ProviderSideEffect: true, HighImpact: true, Destructive: true, TargetArg: targetArg, Underlying: underlying, InputSchema: schema, Handler: handler}
}

func mergeProps(maps ...map[string]any) map[string]any {
	merged := map[string]any{}
	for _, item := range maps {
		for key, value := range item {
			merged[key] = value
		}
	}
	return merged
}

func stringArrayProp(description string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": description}
}

func queueTool(tc ToolContext, args map[string]any) (any, error) {
	actor := contracts.Actor(stringArg(args, "actor"))
	return tc.Server.Workspace.Queries.Queue(tc.Context, actor)
}

func nextTool(tc ToolContext, args map[string]any) (any, error) {
	actor := contracts.Actor(stringArg(args, "actor"))
	return tc.Server.Workspace.Queries.Next(tc.Context, actor)
}

func agentAvailableTool(tc ToolContext, args map[string]any) (any, error) {
	view, err := tc.Server.Workspace.Queries.AgentAvailable(tc.Context, agentActorArg(args))
	if err != nil {
		return nil, err
	}
	page := paginateSlice(view.Available, args, tc.Server.Options.MaxItems, tc.Server.Options.MaxItems)
	view.Available = page.Items.([]service.AgentWorkEntry)
	return map[string]any{"agent_work": view, "total": page.Total, "next_cursor": page.NextCursor}, nil
}

func agentPendingTool(tc ToolContext, args map[string]any) (any, error) {
	view, err := tc.Server.Workspace.Queries.AgentPending(tc.Context, agentActorArg(args))
	if err != nil {
		return nil, err
	}
	page := paginateSlice(view.Pending, args, tc.Server.Options.MaxItems, tc.Server.Options.MaxItems)
	view.Pending = page.Items.([]service.AgentWorkEntry)
	return map[string]any{"agent_work": view, "total": page.Total, "next_cursor": page.NextCursor}, nil
}

func agentActorArg(args map[string]any) contracts.Actor {
	if actor := strings.TrimSpace(stringArg(args, "actor")); actor != "" {
		return contracts.Actor(actor)
	}
	if agentID := strings.TrimSpace(stringArg(args, "agent_id")); agentID != "" {
		if strings.Contains(agentID, ":") {
			return contracts.Actor(agentID)
		}
		return contracts.Actor("agent:" + agentID)
	}
	return ""
}

func searchTool(tc ToolContext, args map[string]any) (any, error) {
	query, err := contracts.ParseSearchQuery(stringArg(args, "query"))
	if err != nil {
		return nil, err
	}
	items, err := tc.Server.Workspace.Queries.Search(tc.Context, query)
	if err != nil {
		return nil, err
	}
	return paginateSlice(items, args, tc.Server.Options.MaxItems, tc.Server.Options.MaxItems), nil
}

func boardTool(tc ToolContext, args map[string]any) (any, error) {
	view, err := tc.Server.Workspace.Queries.Board(tc.Context, contracts.BoardQueryOptions{
		Project:  stringArg(args, "project"),
		Assignee: contracts.Actor(stringArg(args, "assignee")),
		Type:     contracts.TicketType(stringArg(args, "type")),
	})
	if err != nil {
		return nil, err
	}
	return paginateBoard(view, args, tc.Server.Options.MaxItems), nil
}

func ticketViewTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Queries.TicketDetail(tc.Context, stringArg(args, "ticket_id"))
}

func ticketHistoryTool(tc ToolContext, args map[string]any) (any, error) {
	view, err := tc.Server.Workspace.Queries.History(tc.Context, stringArg(args, "ticket_id"))
	if err != nil {
		return nil, err
	}
	page := paginateSlice(view.Events, args, tc.Server.Options.MaxItems, tc.Server.Options.MaxItems)
	view.Events = page.Items.([]contracts.Event)
	return map[string]any{"history": view, "total": page.Total, "next_cursor": page.NextCursor}, nil
}

func ticketInspectTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Queries.InspectTicket(tc.Context, stringArg(args, "ticket_id"), contracts.Actor(stringArg(args, "actor")))
}

func dashboardTool(tc ToolContext, args map[string]any) (any, error) {
	view, err := tc.Server.Workspace.Queries.Dashboard(tc.Context, stringArg(args, "collaborator"))
	if err != nil {
		return nil, err
	}
	return paginateDashboard(view, args, tc.Server.Options.MaxItems), nil
}

func timelineTool(tc ToolContext, args map[string]any) (any, error) {
	view, err := tc.Server.Workspace.Queries.Timeline(tc.Context, stringArg(args, "ticket_id"), stringArg(args, "collaborator"))
	if err != nil {
		return nil, err
	}
	page := paginateSlice(view.Entries, args, tc.Server.Options.MaxItems, tc.Server.Options.MaxItems)
	view.Entries = page.Items.([]service.TimelineEntry)
	return map[string]any{"timeline": view, "total": page.Total, "next_cursor": page.NextCursor}, nil
}

func runViewTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Queries.RunDetail(tc.Context, stringArg(args, "run_id"))
}

func evidenceListTool(tc ToolContext, args map[string]any) (any, error) {
	items, err := tc.Server.Workspace.Queries.EvidenceList(tc.Context, stringArg(args, "run_id"))
	if err != nil {
		return nil, err
	}
	return paginateSlice(items, args, tc.Server.Options.MaxItems, tc.Server.Options.MaxItems), nil
}

func evidenceViewTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Queries.EvidenceDetail(tc.Context, stringArg(args, "evidence_id"))
}

func handoffViewTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Queries.HandoffDetail(tc.Context, stringArg(args, "handoff_id"))
}

func approvalsTool(tc ToolContext, args map[string]any) (any, error) {
	items, err := tc.Server.Workspace.Queries.Approvals(tc.Context, stringArg(args, "collaborator"))
	if err != nil {
		return nil, err
	}
	return paginateSlice(items, args, tc.Server.Options.MaxItems, tc.Server.Options.MaxItems), nil
}

func inboxTool(tc ToolContext, args map[string]any) (any, error) {
	items, err := tc.Server.Workspace.Queries.Inbox(tc.Context, stringArg(args, "collaborator"))
	if err != nil {
		return nil, err
	}
	return paginateSlice(items, args, tc.Server.Options.MaxItems, tc.Server.Options.MaxItems), nil
}

func changeStatusTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Queries.ChangeStatus(tc.Context, stringArg(args, "change_id"))
}

func checksListTool(tc ToolContext, args map[string]any) (any, error) {
	items, err := tc.Server.Workspace.Queries.ListChecks(tc.Context, contracts.CheckScope(stringArg(args, "scope")), stringArg(args, "id"))
	if err != nil {
		return nil, err
	}
	return paginateSlice(items, args, tc.Server.Options.MaxItems, tc.Server.Options.MaxItems), nil
}

func syncStatusTool(tc ToolContext, args map[string]any) (any, error) {
	view, err := tc.Server.Workspace.Queries.SyncStatus(tc.Context, stringArg(args, "remote_id"))
	if err != nil {
		return nil, err
	}
	page := paginateSlice(view.Remotes, args, tc.Server.Options.MaxItems, tc.Server.Options.MaxItems)
	view.Remotes = page.Items.([]service.SyncStatusRemoteView)
	return map[string]any{"sync_status": view, "total_remotes": page.Total, "next_cursor": page.NextCursor}, nil
}

func conflictListTool(tc ToolContext, args map[string]any) (any, error) {
	items, err := tc.Server.Workspace.Queries.ListConflicts(tc.Context)
	if err != nil {
		return nil, err
	}
	return paginateSlice(items, args, tc.Server.Options.MaxItems, tc.Server.Options.MaxItems), nil
}

func conflictViewTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Queries.ConflictDetail(tc.Context, stringArg(args, "conflict_id"))
}

func archivePlanTool(tc ToolContext, args map[string]any) (any, error) {
	target := contracts.RetentionTarget(stringArg(args, "target"))
	if !target.IsValid() {
		return nil, apperr.New(apperr.CodeInvalidInput, "valid archive target is required")
	}
	return tc.Server.Workspace.Queries.ArchivePlan(tc.Context, target, stringArg(args, "project"))
}

func dispatchSuggestTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Queries.DispatchSuggest(tc.Context, stringArg(args, "ticket_id"))
}

func changeMergePlanTool(tc ToolContext, args map[string]any) (any, error) {
	status, err := tc.Server.Workspace.Queries.ChangeStatus(tc.Context, stringArg(args, "change_id"))
	if err != nil {
		return nil, err
	}
	return map[string]any{"change_status": status, "execution_tool": "atlas.change.merge", "requires_operation_approval": true}, nil
}

func syncPullPlanTool(tc ToolContext, args map[string]any) (any, error) {
	status, err := tc.Server.Workspace.Queries.SyncStatus(tc.Context, stringArg(args, "remote_id"))
	if err != nil {
		return nil, err
	}
	return map[string]any{"sync_status": status, "execution_tool": "atlas.sync.pull", "requires_operation_approval": true}, nil
}

func bundleImportPlanTool(tc ToolContext, args map[string]any) (any, error) {
	detail, err := tc.Server.Workspace.Queries.BundleDetail(tc.Context, stringArg(args, "bundle_ref"))
	if err != nil {
		return nil, err
	}
	return map[string]any{"bundle": detail, "execution_tool": "atlas.bundle.import", "requires_operation_approval": true}, nil
}

func importApplyPlanTool(tc ToolContext, args map[string]any) (any, error) {
	detail, err := tc.Server.Workspace.Queries.ImportJobDetail(tc.Context, stringArg(args, "job_id"))
	if err != nil {
		return nil, err
	}
	return map[string]any{"import_job": detail, "execution_tool": "atlas.import.apply", "requires_operation_approval": true}, nil
}

func compactPlanTool(tc ToolContext, _ map[string]any) (any, error) {
	return tc.Server.Workspace.Queries.CompactPlan(tc.Context)
}

func worktreeCleanupPlanTool(tc ToolContext, args map[string]any) (any, error) {
	detail, err := tc.Server.Workspace.Queries.WorktreeDetail(tc.Context, stringArg(args, "run_id"))
	if err != nil {
		return nil, err
	}
	return map[string]any{"worktree": detail, "execution_tool": "atlas.worktree.cleanup", "requires_operation_approval": true}, nil
}

func ticketCommentTool(tc ToolContext, args map[string]any) (any, error) {
	if err := tc.Server.Workspace.Actions.CommentTicket(tc.Context, stringArg(args, "ticket_id"), stringArg(args, "body"), contracts.Actor(tc.Actor), tc.Reason); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func ticketClaimTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.ClaimTicket(tc.Context, stringArg(args, "ticket_id"), contracts.Actor(tc.Actor), tc.Reason)
}

func ticketReleaseTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.ReleaseTicket(tc.Context, stringArg(args, "ticket_id"), contracts.Actor(tc.Actor), tc.Reason)
}

func ticketMoveTool(tc ToolContext, args map[string]any) (any, error) {
	status := contracts.Status(stringArg(args, "status"))
	if status == contracts.StatusDone || status == contracts.StatusCanceled {
		return nil, apperr.New(apperr.CodePermissionDenied, "terminal ticket moves require atlas.ticket.complete or admin-only flows")
	}
	ctx, err := mcpContextWithDependencyOverride(tc, args)
	if err != nil {
		return nil, err
	}
	return tc.Server.Workspace.Actions.MoveTicket(ctx, stringArg(args, "ticket_id"), status, contracts.Actor(tc.Actor), tc.Reason)
}

func ticketRequestReviewTool(tc ToolContext, args map[string]any) (any, error) {
	ctx, err := mcpContextWithDependencyOverride(tc, args)
	if err != nil {
		return nil, err
	}
	return tc.Server.Workspace.Actions.RequestReview(ctx, stringArg(args, "ticket_id"), contracts.Actor(tc.Actor), tc.Reason)
}

func gateApproveTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.ApproveGate(tc.Context, stringArg(args, "gate_id"), contracts.Actor(tc.Actor), tc.Reason)
}

func gateRejectTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.RejectGate(tc.Context, stringArg(args, "gate_id"), contracts.Actor(tc.Actor), tc.Reason)
}

func runCheckpointTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.CheckpointRun(tc.Context, stringArg(args, "run_id"), contracts.Actor(tc.Actor), tc.Reason, stringArg(args, "title"), stringArg(args, "body"))
}

func evidenceAddTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.AddEvidence(tc.Context, stringArg(args, "run_id"), contracts.EvidenceType(stringArg(args, "type")), stringArg(args, "title"), stringArg(args, "body"), stringArg(args, "artifact_source"), stringArg(args, "supersedes_evidence_id"), contracts.Actor(tc.Actor), tc.Reason, contracts.EventRunEvidenceAdded)
}

func handoffCreateTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.CreateHandoff(tc.Context, stringArg(args, "run_id"), contracts.Actor(tc.Actor), tc.Reason, stringSliceArg(args, "open_questions"), stringSliceArg(args, "risks"), stringArg(args, "next_actor"), contracts.GateKind(stringArg(args, "next_gate")), contracts.Status(stringArg(args, "next_status")))
}

func importPreviewTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.PreviewImport(tc.Context, stringArg(args, "source_path"), contracts.Actor(tc.Actor), tc.Reason)
}

func dispatchRunTool(tc ToolContext, args map[string]any) (any, error) {
	agentID := stringArg(args, "agent_id")
	if agentID == "" {
		return tc.Server.Workspace.Actions.AutoDispatchRun(tc.Context, stringArg(args, "ticket_id"), contracts.Actor(tc.Actor), tc.Reason)
	}
	return tc.Server.Workspace.Actions.DispatchRun(tc.Context, stringArg(args, "ticket_id"), agentID, contracts.RunKindWork, contracts.Actor(tc.Actor), tc.Reason)
}

func changeCreateTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.CreateChange(tc.Context, stringArg(args, "run_id"), contracts.Actor(tc.Actor), tc.Reason)
}

func changeSyncTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.SyncChange(tc.Context, stringArg(args, "change_id"), contracts.Actor(tc.Actor), tc.Reason)
}

func checksSyncTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.SyncChangeChecks(tc.Context, stringArg(args, "change_id"), contracts.Actor(tc.Actor), tc.Reason)
}

func changeReviewRequestTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.RequestChangeReview(tc.Context, stringArg(args, "change_id"), contracts.Actor(tc.Actor), tc.Reason)
}

func changeMergeTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.MergeChange(tc.Context, stringArg(args, "change_id"), contracts.Actor(tc.Actor), tc.Reason)
}

func gateWaiveTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.WaiveGate(tc.Context, stringArg(args, "gate_id"), contracts.Actor(tc.Actor), tc.Reason)
}

func ticketCompleteTool(tc ToolContext, args map[string]any) (any, error) {
	ctx, err := mcpContextWithDependencyOverride(tc, args)
	if err != nil {
		return nil, err
	}
	return tc.Server.Workspace.Actions.CompleteTicket(ctx, stringArg(args, "ticket_id"), contracts.Actor(tc.Actor), tc.Reason)
}

func mcpContextWithDependencyOverride(tc ToolContext, args map[string]any) (context.Context, error) {
	if !boolArg(args, "override_deps") {
		return tc.Context, nil
	}
	actor := contracts.Actor(tc.Actor)
	if actor != contracts.Actor("human:owner") {
		return nil, apperr.New(apperr.CodePermissionDenied, "dependency_override_requires_owner")
	}
	if strings.TrimSpace(tc.Reason) == "" {
		return nil, apperr.New(apperr.CodeInvalidInput, "dependency_override_requires_reason")
	}
	return service.WithDependencyOverride(tc.Context, actor, tc.Reason), nil
}

func syncPullTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.SyncPull(tc.Context, stringArg(args, "remote_id"), stringArg(args, "source_workspace_id"), contracts.Actor(tc.Actor), tc.Reason)
}

func syncPushTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.SyncPush(tc.Context, stringArg(args, "remote_id"), contracts.Actor(tc.Actor), tc.Reason)
}

func bundleImportTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.ImportSyncBundle(tc.Context, stringArg(args, "bundle_ref"), contracts.Actor(tc.Actor), tc.Reason)
}

func importApplyTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.ApplyImport(tc.Context, stringArg(args, "job_id"), contracts.Actor(tc.Actor), tc.Reason)
}

func archiveApplyTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.ApplyArchive(tc.Context, contracts.RetentionTarget(stringArg(args, "target")), stringArg(args, "project"), true, contracts.Actor(tc.Actor), tc.Reason)
}

func archiveRestoreTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.RestoreArchive(tc.Context, stringArg(args, "archive_id"), contracts.Actor(tc.Actor), tc.Reason)
}

func compactTool(tc ToolContext, args map[string]any) (any, error) {
	if target := strings.TrimSpace(stringArg(args, "target")); target != "" && target != "workspace" {
		return nil, apperr.New(apperr.CodeInvalidInput, "compact target must be workspace")
	}
	return tc.Server.Workspace.Actions.CompactWorkspace(tc.Context, true, contracts.Actor(tc.Actor), tc.Reason)
}

func worktreeCleanupTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.CleanupRun(tc.Context, stringArg(args, "run_id"), boolArg(args, "force"), contracts.Actor(tc.Actor), tc.Reason)
}

func stringSliceArg(args map[string]any, key string) []string {
	raw, ok := args[key]
	if !ok || raw == nil {
		return nil
	}
	switch value := raw.(type) {
	case []string:
		return append([]string(nil), value...)
	case []any:
		items := make([]string, 0, len(value))
		for _, item := range value {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" {
				items = append(items, text)
			}
		}
		return items
	default:
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" {
			return nil
		}
		return []string{text}
	}
}

func paginateBoard(view service.BoardViewModel, args map[string]any, maxItems int) map[string]any {
	total := 0
	cursors := stringMapArg(args, "cursor_by_status")
	pagesByStatus := map[string]map[string]any{}
	nextByStatus := map[string]string{}
	for status, tickets := range view.Board.Columns {
		page := paginateSliceWithCursor(tickets, cursors[string(status)], args, maxItems, maxItems)
		view.Board.Columns[status] = page.Items.([]contracts.TicketSnapshot)
		total += page.Total
		pagesByStatus[string(status)] = map[string]any{"total": page.Total, "next_cursor": page.NextCursor}
		if page.NextCursor != "" {
			nextByStatus[string(status)] = page.NextCursor
		}
	}
	return map[string]any{"board": view, "total": total, "next_cursor_by_status": nextByStatus, "pages_by_status": pagesByStatus}
}

func paginateDashboard(view service.DashboardSummaryView, args map[string]any, maxItems int) map[string]any {
	cursors := stringMapArg(args, "cursor_by_section")
	pages := map[string]map[string]any{}
	pageStrings := func(name string, items []string) []string {
		page := paginateSliceWithCursor(items, cursors[name], args, maxItems, maxItems)
		pages[name] = map[string]any{"total": page.Total, "next_cursor": page.NextCursor}
		return page.Items.([]string)
	}

	view.StaleWorktrees = pageStrings("stale_worktrees", view.StaleWorktrees)
	view.RetentionTargets = pageStrings("retention_targets", view.RetentionTargets)
	view.FailedSyncJobs = pageStrings("failed_sync_jobs", view.FailedSyncJobs)
	view.ProviderMappingWarnings = pageStrings("provider_mapping_warnings", view.ProviderMappingWarnings)

	workload := paginateSliceWithCursor(view.CollaboratorWorkload, cursors["collaborator_workload"], args, maxItems, maxItems)
	view.CollaboratorWorkload = workload.Items.([]service.CollaboratorWorkloadView)
	pages["collaborator_workload"] = map[string]any{"total": workload.Total, "next_cursor": workload.NextCursor}

	mentions := paginateSliceWithCursor(view.MentionQueue, cursors["mention_queue"], args, maxItems, maxItems)
	view.MentionQueue = mentions.Items.([]service.MentionQueueEntry)
	pages["mention_queue"] = map[string]any{"total": mentions.Total, "next_cursor": mentions.NextCursor}

	conflicts := paginateSliceWithCursor(view.ConflictQueue, cursors["conflict_queue"], args, maxItems, maxItems)
	view.ConflictQueue = conflicts.Items.([]service.ConflictQueueEntry)
	pages["conflict_queue"] = map[string]any{"total": conflicts.Total, "next_cursor": conflicts.NextCursor}

	remotes := paginateSliceWithCursor(view.RemoteHealth, cursors["remote_health"], args, maxItems, maxItems)
	view.RemoteHealth = remotes.Items.([]service.RemoteHealthView)
	pages["remote_health"] = map[string]any{"total": remotes.Total, "next_cursor": remotes.NextCursor}

	return map[string]any{"dashboard": view, "pages": pages}
}
