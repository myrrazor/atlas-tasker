package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/domain"
	"github.com/myrrazor/atlas-tasker/internal/render"
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
		readTool("atlas.agent.list", "List agent profiles.", readProfiles, objectSchema(nil, mergeProps(commonReadProps(), nil)), "QueryService.ListAgents", agentListTool),
		readTool("atlas.agent.view", "Read one agent profile.", readProfiles, objectSchema([]string{"agent_id"}, map[string]any{"agent_id": stringProp("Agent ID.")}), "QueryService.AgentDetail", agentViewTool),
		readTool("atlas.agent.wakeup.list", "List agent wake-up records.", readProfiles, objectSchema(nil, mergeProps(commonReadProps(), map[string]any{"agent_id": stringProp("Optional agent ID filter.")})), "QueryService.AgentWakeups", agentWakeupListTool),
		readTool("atlas.agent.wakeup.view", "Read one agent wake-up record.", readProfiles, objectSchema([]string{"wakeup_id"}, map[string]any{"wakeup_id": stringProp("Wake-up ID.")}), "QueryService.AgentWakeup", agentWakeupViewTool),
		readTool("atlas.team.list", "List ready-made agent team presets.", readProfiles, objectSchema(nil, map[string]any{"provider": stringProp("Optional provider: claude, codex, or mixed.")}), "TeamPresets", teamListTool),
		readTool("atlas.team.show", "Show one agent team preset.", readProfiles, objectSchema([]string{"preset"}, map[string]any{"preset": stringProp("Preset name: solo, pair, swarm, or crossfire."), "provider": stringProp("Optional provider: claude, codex, or mixed.")}), "TeamPresetByName", teamShowTool),
		readTool("atlas.goal.brief", "Read a pasteable goal brief for a ticket or run.", readProfiles, objectSchema([]string{"target"}, map[string]any{"target": stringProp("Ticket ID or run ID.")}), "ActionService.GoalBrief", goalBriefTool),
		readTool("atlas.search", "Search tickets with Atlas query syntax.", readProfiles, objectSchema([]string{"query"}, mergeProps(commonReadProps(), map[string]any{"query": stringProp("Atlas ticket search query.")})), "QueryService.Search", searchTool),
		readTool("atlas.context", "Read workspace identity, managed-mode policy, assigned work, and backup health.", readProfiles, objectSchema(nil, mergeProps(commonReadProps(), map[string]any{"project": stringProp("Optional project key."), "actor": stringProp("Optional Atlas actor for this integration.")})), "QueryService.ManagedModeView", contextTool),
		readTool("atlas.status", "Read a fresh workspace, project, ticket, agent, or run status with compact Markdown.", readProfiles, objectSchema(nil, mergeProps(commonReadProps(), map[string]any{"scope": stringProp("Optional scope: workspace, project, ticket, agent, or run."), "project": stringProp("Optional project key."), "ticket_id": stringProp("Optional ticket ID for ticket scope."), "agent_id": stringProp("Optional agent ID for agent scope."), "run_id": stringProp("Optional run ID for run scope."), "actor": stringProp("Optional Atlas actor.")})), "QueryService.Board", statusTool),
		readTool("atlas.backup.status", "Read automatic backup health without target URLs, credentials, or mutation.", readProfiles, objectSchema(nil, map[string]any{}), "QueryService.AutoBackupStatus", backupStatusTool),
		readTool("atlas.board", "Read the board grouped by status.", readProfiles, objectSchema(nil, mergeProps(groupedReadProps("cursor_by_status", "Optional per-status cursors keyed by Atlas status."), map[string]any{"project": stringProp("Optional project key."), "assignee": stringProp("Optional assignee actor."), "type": stringProp("Optional ticket type.")})), "QueryService.Board", boardTool),
		readTool("atlas.ticket.view", "Read one ticket detail view.", readProfiles, objectSchema([]string{"ticket_id"}, map[string]any{"ticket_id": stringProp("Ticket ID.")}), "QueryService.TicketDetail", ticketViewTool),
		readTool("atlas.ticket.history", "Read ticket event history.", readProfiles, objectSchema([]string{"ticket_id"}, mergeProps(commonReadProps(), map[string]any{"ticket_id": stringProp("Ticket ID.")})), "QueryService.History", ticketHistoryTool),
		readTool("atlas.ticket.inspect", "Inspect a ticket, policy, links, and git context.", readProfiles, objectSchema([]string{"ticket_id"}, map[string]any{"ticket_id": stringProp("Ticket ID."), "actor": stringProp("Optional actor for policy context.")}), "QueryService.InspectTicket", ticketInspectTool),
		readTool("atlas.schedule.list", "Read one-time ticket schedules.", readProfiles, objectSchema(nil, mergeProps(commonReadProps(), map[string]any{"project": stringProp("Optional project key."), "from": stringProp("Optional inclusive RFC3339 start."), "to": stringProp("Optional exclusive RFC3339 end.")})), "QueryService.Schedule", scheduleListTool),
		readTool("atlas.schedule.history", "Read ticket completion history for a time range.", readProfiles, objectSchema(nil, mergeProps(commonReadProps(), map[string]any{"project": stringProp("Optional project key."), "from": stringProp("Optional inclusive RFC3339 start."), "to": stringProp("Optional exclusive RFC3339 end.")})), "QueryService.CompletionHistory", scheduleHistoryTool),
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

		projectCreateSpec(workflowProfiles),
		writeTool("atlas.ticket.comment", ClassWorkflow, workflowProfiles, false, "Comment on a ticket.", objectSchema([]string{"ticket_id", "body", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"ticket_id": stringProp("Ticket ID."), "body": stringProp("Comment body.")})), "ActionService.CommentTicket", "ticket_id", ticketCommentTool),
		writeTool("atlas.ticket.claim", ClassWorkflow, workflowProfiles, false, "Claim a ticket lease.", objectSchema([]string{"ticket_id", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"ticket_id": stringProp("Ticket ID.")})), "ActionService.ClaimTicket", "ticket_id", ticketClaimTool),
		writeTool("atlas.ticket.release", ClassWorkflow, workflowProfiles, false, "Release a ticket lease.", objectSchema([]string{"ticket_id", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"ticket_id": stringProp("Ticket ID.")})), "ActionService.ReleaseTicket", "ticket_id", ticketReleaseTool),
		writeTool("atlas.ticket.heartbeat", ClassWorkflow, workflowProfiles, false, "Extend an active ticket lease held by the actor.", objectSchema([]string{"ticket_id", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"ticket_id": stringProp("Ticket ID.")})), "ActionService.HeartbeatTicket", "ticket_id", ticketHeartbeatTool),
		writeTool("atlas.ticket.move", ClassWorkflow, workflowProfiles, false, "Move a ticket among non-terminal workflow statuses.", objectSchema([]string{"ticket_id", "status", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"ticket_id": stringProp("Ticket ID."), "status": stringProp("Target status."), "override_deps": boolProp("Owner-only dependency override.")})), "ActionService.MoveTicket", "ticket_id", ticketMoveTool),
		writeTool("atlas.ticket.create", ClassWorkflow, workflowProfiles, false, "Create a ticket.", objectSchema([]string{"project", "title", "type", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"project": stringProp("Project key."), "title": stringProp("Ticket title."), "type": stringProp("Ticket type: epic, task, bug, or subtask."), "status": stringProp("Optional initial status."), "priority": stringProp("Optional priority."), "parent": stringProp("Optional parent ticket ID."), "labels": stringArrayProp("Optional labels."), "assignee": stringProp("Optional assignee actor."), "reviewer": stringProp("Optional reviewer actor."), "description": stringProp("Optional description."), "acceptance": stringArrayProp("Optional acceptance criteria."), "template": stringProp("Optional template name."), "protected": boolProp("Mark the ticket as protected."), "sensitive": boolProp("Mark the ticket as sensitive.")})), "ActionService.CreateTrackedTicket", "project", ticketCreateTool),
		writeTool("atlas.ticket.edit", ClassWorkflow, workflowProfiles, false, "Edit ordinary ticket fields in one tracked mutation.", objectSchema([]string{"ticket_id", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"ticket_id": stringProp("Ticket ID."), "title": stringProp("New ticket title."), "description": stringProp("New description; an empty string clears it."), "acceptance": stringArrayProp("New acceptance criteria; an empty array clears them."), "priority": stringProp("New priority: low, medium, high, or critical."), "labels": stringArrayProp("Replacement labels; an empty array clears them."), "assignee": stringProp("New assignee actor; an empty string clears it."), "reviewer": stringProp("New reviewer actor; an empty string clears it.")})), "ActionService.MutateTrackedTicket", "ticket_id", ticketEditTool),
		writeTool("atlas.ticket.assign", ClassWorkflow, workflowProfiles, false, "Set a ticket assignee.", objectSchema([]string{"ticket_id", "assignee", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"ticket_id": stringProp("Ticket ID."), "assignee": stringProp("Assignee actor.")})), "ActionService.AssignTicket", "ticket_id", ticketAssignTool),
		writeTool("atlas.ticket.priority", ClassWorkflow, workflowProfiles, false, "Set a ticket priority.", objectSchema([]string{"ticket_id", "priority", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"ticket_id": stringProp("Ticket ID."), "priority": stringProp("Priority: low, medium, high, or critical.")})), "ActionService.MutateTrackedTicket", "ticket_id", ticketPriorityTool),
		writeTool("atlas.ticket.label.add", ClassWorkflow, workflowProfiles, false, "Add a ticket label without creating a duplicate.", objectSchema([]string{"ticket_id", "label", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"ticket_id": stringProp("Ticket ID."), "label": stringProp("Label to add.")})), "ActionService.MutateTrackedTicket", "ticket_id", ticketLabelAddTool),
		writeTool("atlas.ticket.label.remove", ClassWorkflow, workflowProfiles, false, "Remove every exact match for a ticket label.", objectSchema([]string{"ticket_id", "label", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"ticket_id": stringProp("Ticket ID."), "label": stringProp("Label to remove.")})), "ActionService.MutateTrackedTicket", "ticket_id", ticketLabelRemoveTool),
		writeTool("atlas.ticket.link", ClassWorkflow, workflowProfiles, false, "Link two tickets.", objectSchema([]string{"ticket_id", "other_id", "kind", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"ticket_id": stringProp("Ticket ID."), "other_id": stringProp("Other ticket ID."), "kind": stringProp("Relationship: blocks, blocked_by, or parent.")})), "ActionService.LinkTickets", "ticket_id", ticketLinkTool),
		writeTool("atlas.ticket.unlink", ClassWorkflow, workflowProfiles, false, "Remove a ticket relationship.", objectSchema([]string{"ticket_id", "other_id", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"ticket_id": stringProp("Ticket ID."), "other_id": stringProp("Other ticket ID.")})), "ActionService.UnlinkTickets", "ticket_id", ticketUnlinkTool),
		writeTool("atlas.ticket.approve", ClassWorkflow, workflowProfiles, false, "Approve a ticket in review.", objectSchema([]string{"ticket_id", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"ticket_id": stringProp("Ticket ID."), "override_deps": boolProp("Owner-only dependency override.")})), "ActionService.ApproveTicket", "ticket_id", ticketApproveTool),
		writeTool("atlas.ticket.reject", ClassWorkflow, workflowProfiles, false, "Reject a ticket in review.", objectSchema([]string{"ticket_id", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"ticket_id": stringProp("Ticket ID.")})), "ActionService.RejectTicket", "ticket_id", ticketRejectTool),
		writeTool("atlas.ticket.complete", ClassWorkflow, workflowProfiles, false, "Complete an approved ticket, or close in-progress work when completion_mode is open.", objectSchema([]string{"ticket_id", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"ticket_id": stringProp("Ticket ID."), "override_deps": boolProp("Owner-only dependency override.")})), "ActionService.CompleteTicket", "ticket_id", ticketCompleteTool),
		writeTool("atlas.agent.create", ClassWorkflow, workflowProfiles, false, "Create an agent profile.", objectSchema([]string{"agent_id", "name", "provider", "actor", "reason"}, mergeProps(actorReasonProps(), agentProfileProps(true))), "ActionService.SaveAgentProfile", "agent_id", agentCreateTool),
		writeTool("atlas.agent.edit", ClassWorkflow, workflowProfiles, false, "Edit an agent profile.", objectSchema([]string{"agent_id", "actor", "reason"}, mergeProps(actorReasonProps(), agentProfileProps(false))), "ActionService.SaveAgentProfile", "agent_id", agentEditTool),
		writeTool("atlas.agent.enable", ClassWorkflow, workflowProfiles, false, "Enable an agent profile.", objectSchema([]string{"agent_id", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"agent_id": stringProp("Agent ID.")})), "ActionService.SetAgentEnabled", "agent_id", agentEnableTool),
		writeTool("atlas.agent.disable", ClassWorkflow, workflowProfiles, false, "Disable an agent profile.", objectSchema([]string{"agent_id", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"agent_id": stringProp("Agent ID.")})), "ActionService.SetAgentEnabled", "agent_id", agentDisableTool),
		writeTool("atlas.agent.wakeup.ack", ClassWorkflow, workflowProfiles, false, "Acknowledge an agent wake-up.", objectSchema([]string{"wakeup_id", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"wakeup_id": stringProp("Wake-up ID.")})), "ActionService.AckAgentWakeup", "wakeup_id", agentWakeupAckTool),
		writeTool("atlas.team.apply", ClassWorkflow, workflowProfiles, false, "Apply a ready-made agent team preset.", objectSchema([]string{"preset", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"preset": stringProp("Preset name: solo, pair, swarm, or crossfire."), "provider": stringProp("Optional provider: claude, codex, or mixed."), "dry_run": boolProp("Preview without creating profiles.")})), "ActionService.ApplyTeamPreset", "preset", teamApplyTool),
		writeTool("atlas.schedule.set", ClassWorkflow, workflowProfiles, false, "Set or replace a one-time ticket schedule.", objectSchema([]string{"ticket_id", "at", "runner", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"ticket_id": stringProp("Ticket ID."), "at": stringProp("RFC3339 schedule instant."), "runner": stringProp("Human or agent actor that will own the ticket.")})), "ActionService.SetTicketSchedule", "ticket_id", scheduleSetTool),
		writeTool("atlas.schedule.clear", ClassWorkflow, workflowProfiles, false, "Remove a ticket schedule without changing its assignee.", objectSchema([]string{"ticket_id", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"ticket_id": stringProp("Ticket ID.")})), "ActionService.ClearTicketSchedule", "ticket_id", scheduleClearTool),
		writeTool("atlas.ticket.request_review", ClassWorkflow, workflowProfiles, false, "Request review for a ticket.", objectSchema([]string{"ticket_id", "actor", "reason"}, mergeProps(actorReasonProps(), map[string]any{"ticket_id": stringProp("Ticket ID."), "reviewer": stringProp("Optional reviewer actor."), "override_deps": boolProp("Owner-only dependency override.")})), "ActionService.RequestReviewWithReviewer", "ticket_id", ticketRequestReviewTool),
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

func projectCreateSpec(profiles []ToolProfile) ToolSpec {
	return ToolSpec{
		Name:              "atlas.project.create",
		Title:             "atlas.project.create",
		Description:       "Create a project container in the pinned Atlas workspace.",
		Class:             ClassWorkflow,
		Profiles:          profiles,
		ApprovalMechanism: ApprovalNone,
		TargetArg:         "key",
		Underlying:        "ActionService.CreateProject",
		InputSchema: objectSchema([]string{"key", "name"}, map[string]any{
			"key":  stringProp("Project key."),
			"name": stringProp("Project name."),
		}),
		Handler: projectCreateTool,
	}
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

func agentListTool(tc ToolContext, args map[string]any) (any, error) {
	items, err := tc.Server.Workspace.Queries.ListAgents(tc.Context)
	if err != nil {
		return nil, err
	}
	return paginateSlice(items, args, tc.Server.Options.MaxItems, tc.Server.Options.MaxItems), nil
}

func agentViewTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Queries.AgentDetail(tc.Context, stringArg(args, "agent_id"))
}

func agentWakeupListTool(tc ToolContext, args map[string]any) (any, error) {
	items, err := tc.Server.Workspace.Queries.AgentWakeups(tc.Context, stringArg(args, "agent_id"))
	if err != nil {
		return nil, err
	}
	return paginateSlice(items, args, tc.Server.Options.MaxItems, tc.Server.Options.MaxItems), nil
}

func agentWakeupViewTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Queries.AgentWakeup(tc.Context, stringArg(args, "wakeup_id"))
}

func teamListTool(_ ToolContext, args map[string]any) (any, error) {
	return service.TeamPresets(stringArg(args, "provider"))
}

func teamShowTool(_ ToolContext, args map[string]any) (any, error) {
	return service.TeamPresetByName(stringArg(args, "preset"), stringArg(args, "provider"))
}

func goalBriefTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.GoalBrief(tc.Context, stringArg(args, "target"))
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
	paged := paginateBoard(view, args, tc.Server.Options.MaxItems)
	cursors, _ := paged["next_cursor_by_status"].(map[string]string)
	board := render.NewCompactBoard(stringArg(args, "project"), view.Board.Columns, tc.Server.Options.MaxItems, cursors)
	paged["markdown"] = render.CompactBoardMarkdown(board)
	paged["mcp_app"] = newBoardApp(board)
	paged["board_url"] = board.BoardURL
	return paged, nil
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

func scheduleListTool(tc ToolContext, args map[string]any) (any, error) {
	query, err := scheduleQueryArgs(args)
	if err != nil {
		return nil, err
	}
	view, err := tc.Server.Workspace.Queries.Schedule(tc.Context, query)
	if err != nil {
		return nil, err
	}
	page := paginateSlice(view.Entries, args, tc.Server.Options.MaxItems, tc.Server.Options.MaxItems)
	return map[string]any{"schedule": page.Items, "generated_at": view.GeneratedAt, "total": page.Total, "next_cursor": page.NextCursor}, nil
}

func scheduleHistoryTool(tc ToolContext, args map[string]any) (any, error) {
	query, err := scheduleQueryArgs(args)
	if err != nil {
		return nil, err
	}
	items, err := tc.Server.Workspace.Queries.CompletionHistory(tc.Context, query)
	if err != nil {
		return nil, err
	}
	return paginateSlice(items, args, tc.Server.Options.MaxItems, tc.Server.Options.MaxItems), nil
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

func projectCreateTool(tc ToolContext, args map[string]any) (any, error) {
	now := time.Now().UTC()
	if tc.Server.Workspace.Actions.Clock != nil {
		now = tc.Server.Workspace.Actions.Clock().UTC()
	}
	project := contracts.Project{
		Key:           stringArg(args, "key"),
		Name:          stringArg(args, "name"),
		CreatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := tc.Server.Workspace.Actions.CreateProject(tc.Context, project); err != nil {
		return nil, err
	}
	return tc.Server.Workspace.Actions.Projects.GetProject(tc.Context, project.Key)
}

func ticketCommentTool(tc ToolContext, args map[string]any) (any, error) {
	body := stringArg(args, "body")
	if err := tc.Server.Workspace.Actions.CommentTicket(tc.Context, stringArg(args, "ticket_id"), body, contracts.Actor(tc.Actor), tc.Reason); err != nil {
		return nil, err
	}
	out := map[string]any{"ok": true}
	if findings := service.SecretLikeFindings(body); len(findings) > 0 {
		out["warnings"] = []string{"secret_like_content:" + strings.Join(findings, ",")}
	}
	return out, nil
}

func ticketClaimTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.ClaimTicket(tc.Context, stringArg(args, "ticket_id"), contracts.Actor(tc.Actor), tc.Reason)
}

func ticketReleaseTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.ReleaseTicket(tc.Context, stringArg(args, "ticket_id"), contracts.Actor(tc.Actor), tc.Reason)
}

func ticketHeartbeatTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.HeartbeatTicket(tc.Context, stringArg(args, "ticket_id"), contracts.Actor(tc.Actor), tc.Reason)
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

func ticketCreateTool(tc ToolContext, args map[string]any) (any, error) {
	now := time.Now().UTC()
	if tc.Server.Workspace.Actions.Clock != nil {
		now = tc.Server.Workspace.Actions.Clock().UTC()
	}
	status := contracts.StatusBacklog
	if raw := stringArg(args, "status"); raw != "" {
		status = contracts.Status(raw)
	}
	priority := contracts.PriorityMedium
	if raw := stringArg(args, "priority"); raw != "" {
		priority = contracts.Priority(raw)
	}
	ticket := contracts.TicketSnapshot{
		Project:            stringArg(args, "project"),
		Title:              stringArg(args, "title"),
		Type:               contracts.TicketType(stringArg(args, "type")),
		Status:             status,
		Priority:           priority,
		Parent:             stringArg(args, "parent"),
		Labels:             stringSliceArg(args, "labels"),
		Assignee:           contracts.Actor(stringArg(args, "assignee")),
		Reviewer:           contracts.Actor(stringArg(args, "reviewer")),
		CreatedAt:          now,
		UpdatedAt:          now,
		SchemaVersion:      contracts.CurrentSchemaVersion,
		Summary:            stringArg(args, "title"),
		Description:        stringArg(args, "description"),
		AcceptanceCriteria: stringSliceArg(args, "acceptance"),
		Template:           stringArg(args, "template"),
		Protected:          boolArg(args, "protected"),
		Sensitive:          boolArg(args, "sensitive"),
	}
	created, err := tc.Server.Workspace.Actions.CreateTrackedTicket(tc.Context, ticket, contracts.Actor(tc.Actor), tc.Reason)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"ticket": created}
	findings := service.SecretLikeFindings(strings.Join([]string{
		ticket.Title,
		ticket.Description,
		strings.Join(ticket.AcceptanceCriteria, "\n"),
	}, "\n"))
	if len(findings) > 0 {
		out["warnings"] = []string{"secret_like_content:" + strings.Join(findings, ",")}
	}
	return out, nil
}

func ticketAssignTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.AssignTicket(tc.Context, stringArg(args, "ticket_id"), contracts.Actor(stringArg(args, "assignee")), contracts.Actor(tc.Actor), tc.Reason)
}

func ticketPriorityTool(tc ToolContext, args map[string]any) (any, error) {
	priority := contracts.Priority(stringArg(args, "priority"))
	if !priority.IsValid() {
		return nil, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid priority: %s", priority))
	}
	return tc.Server.Workspace.Actions.MutateTrackedTicket(tc.Context, stringArg(args, "ticket_id"), contracts.Actor(tc.Actor), tc.Reason, "set ticket priority", func(ticket *contracts.TicketSnapshot) error {
		ticket.Priority = priority
		return nil
	})
}

func ticketLabelAddTool(tc ToolContext, args map[string]any) (any, error) {
	label := stringArg(args, "label")
	return tc.Server.Workspace.Actions.MutateTrackedTicket(tc.Context, stringArg(args, "ticket_id"), contracts.Actor(tc.Actor), tc.Reason, "add ticket label", func(ticket *contracts.TicketSnapshot) error {
		for _, existing := range ticket.Labels {
			if existing == label {
				return nil
			}
		}
		ticket.Labels = append(ticket.Labels, label)
		return nil
	})
}

func ticketLabelRemoveTool(tc ToolContext, args map[string]any) (any, error) {
	label := stringArg(args, "label")
	return tc.Server.Workspace.Actions.MutateTrackedTicket(tc.Context, stringArg(args, "ticket_id"), contracts.Actor(tc.Actor), tc.Reason, "remove ticket label", func(ticket *contracts.TicketSnapshot) error {
		labels := make([]string, 0, len(ticket.Labels))
		for _, existing := range ticket.Labels {
			if existing != label {
				labels = append(labels, existing)
			}
		}
		ticket.Labels = labels
		return nil
	})
}

func ticketEditTool(tc ToolContext, args map[string]any) (any, error) {
	for _, key := range []string{"title", "description", "acceptance", "priority", "labels", "assignee", "reviewer"} {
		if raw, ok := args[key]; ok && raw == nil {
			return nil, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("%s cannot be null", key))
		}
	}

	var title *string
	if _, ok := args["title"]; ok {
		value := render.SanitizeDisplayLine(stringArg(args, "title"))
		if value == "" {
			return nil, apperr.New(apperr.CodeInvalidInput, "title cannot be blank")
		}
		title = &value
	}
	var description *string
	if raw, ok := args["description"]; ok {
		value := raw.(string)
		description = &value
	}
	var acceptance []string
	_, acceptanceSet := args["acceptance"]
	if acceptanceSet {
		acceptance = rawStringSliceArg(args, "acceptance")
	}
	var priority *contracts.Priority
	if _, ok := args["priority"]; ok {
		value := contracts.Priority(stringArg(args, "priority"))
		if !value.IsValid() {
			return nil, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid priority: %s", value))
		}
		priority = &value
	}
	var labels []string
	_, labelsSet := args["labels"]
	if labelsSet {
		labels = stringSliceArg(args, "labels")
	}
	var assignee *contracts.Actor
	if _, ok := args["assignee"]; ok {
		value := contracts.Actor(stringArg(args, "assignee"))
		if value != "" && !value.IsValid() {
			return nil, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid assignee actor: %s", value))
		}
		assignee = &value
	}
	var reviewer *contracts.Actor
	if _, ok := args["reviewer"]; ok {
		value := contracts.Actor(stringArg(args, "reviewer"))
		if value != "" && !value.IsValid() {
			return nil, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid reviewer actor: %s", value))
		}
		reviewer = &value
	}

	return tc.Server.Workspace.Actions.MutateTrackedTicket(tc.Context, stringArg(args, "ticket_id"), contracts.Actor(tc.Actor), tc.Reason, "edit ticket", func(ticket *contracts.TicketSnapshot) error {
		if title != nil {
			ticket.Title = *title
			ticket.Summary = *title
		}
		if description != nil {
			ticket.Description = *description
		}
		if acceptanceSet {
			ticket.AcceptanceCriteria = acceptance
		}
		if priority != nil {
			ticket.Priority = *priority
		}
		if labelsSet {
			ticket.Labels = labels
		}
		if assignee != nil {
			ticket.Assignee = *assignee
		}
		if reviewer != nil {
			ticket.Reviewer = *reviewer
		}
		return nil
	})
}

func ticketLinkTool(tc ToolContext, args map[string]any) (any, error) {
	kind := domain.LinkKind(stringArg(args, "kind"))
	switch kind {
	case domain.LinkBlocks, domain.LinkBlockedBy, domain.LinkParent:
	default:
		return nil, apperr.New(apperr.CodeInvalidInput, "kind must be blocks, blocked_by, or parent")
	}
	return tc.Server.Workspace.Actions.LinkTickets(tc.Context, stringArg(args, "ticket_id"), stringArg(args, "other_id"), kind, contracts.Actor(tc.Actor), tc.Reason)
}

func ticketUnlinkTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.UnlinkTickets(tc.Context, stringArg(args, "ticket_id"), stringArg(args, "other_id"), contracts.Actor(tc.Actor), tc.Reason)
}

func ticketApproveTool(tc ToolContext, args map[string]any) (any, error) {
	ctx, err := mcpContextWithDependencyOverride(tc, args)
	if err != nil {
		return nil, err
	}
	return tc.Server.Workspace.Actions.ApproveTicket(ctx, stringArg(args, "ticket_id"), contracts.Actor(tc.Actor), tc.Reason)
}

func ticketRejectTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.RejectTicket(tc.Context, stringArg(args, "ticket_id"), contracts.Actor(tc.Actor), tc.Reason)
}

func agentCreateTool(tc ToolContext, args map[string]any) (any, error) {
	profile, err := agentProfileFromArgs(args, contracts.AgentProfile{Enabled: true}, true)
	if err != nil {
		return nil, err
	}
	return tc.Server.Workspace.Actions.SaveAgentProfile(tc.Context, profile, contracts.Actor(tc.Actor), tc.Reason)
}

func agentEditTool(tc ToolContext, args map[string]any) (any, error) {
	existing, err := tc.Server.Workspace.Queries.Agents.LoadAgent(tc.Context, stringArg(args, "agent_id"))
	if err != nil {
		return nil, err
	}
	profile, err := agentProfileFromArgs(args, existing, false)
	if err != nil {
		return nil, err
	}
	return tc.Server.Workspace.Actions.SaveAgentProfile(tc.Context, profile, contracts.Actor(tc.Actor), tc.Reason)
}

func agentEnableTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.SetAgentEnabled(tc.Context, stringArg(args, "agent_id"), true, contracts.Actor(tc.Actor), tc.Reason)
}

func agentDisableTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.SetAgentEnabled(tc.Context, stringArg(args, "agent_id"), false, contracts.Actor(tc.Actor), tc.Reason)
}

func agentWakeupAckTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.AckAgentWakeup(tc.Context, stringArg(args, "wakeup_id"), contracts.Actor(tc.Actor), tc.Reason)
}

func teamApplyTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.ApplyTeamPreset(tc.Context, stringArg(args, "preset"), stringArg(args, "provider"), boolArg(args, "dry_run"), contracts.Actor(tc.Actor), tc.Reason)
}

func agentProfileFromArgs(args map[string]any, profile contracts.AgentProfile, create bool) (contracts.AgentProfile, error) {
	if id := stringArg(args, "agent_id"); id != "" {
		profile.AgentID = id
	}
	if name := stringArg(args, "name"); name != "" || create {
		if name != "" {
			profile.DisplayName = name
		}
	}
	if provider := stringArg(args, "provider"); provider != "" {
		profile.Provider = contracts.AgentProvider(provider)
	}
	if caps := stringSliceArg(args, "capability"); len(caps) > 0 {
		profile.Capabilities = caps
	}
	if types := stringSliceArg(args, "ticket_type"); len(types) > 0 {
		profile.AllowedTicketTypes = make([]contracts.TicketType, 0, len(types))
		for _, item := range types {
			profile.AllowedTicketTypes = append(profile.AllowedTicketTypes, contracts.TicketType(item))
		}
	}
	if roles := stringSliceArg(args, "role"); len(roles) > 0 {
		profile.PreferredRoles = make([]contracts.AgentRole, 0, len(roles))
		for _, item := range roles {
			profile.PreferredRoles = append(profile.PreferredRoles, contracts.AgentRole(item))
		}
	}
	if v := stringArg(args, "default_runbook"); v != "" {
		profile.DefaultRunbook = v
	}
	if _, ok := args["max_active_runs"]; ok {
		profile.MaxActiveRuns = intArg(args, "max_active_runs", profile.MaxActiveRuns)
	}
	if _, ok := args["routing_weight"]; ok {
		profile.RoutingWeight = intArg(args, "routing_weight", profile.RoutingWeight)
	}
	if v := stringArg(args, "instruction_profile"); v != "" {
		profile.InstructionProfile = v
	}
	if v := stringArg(args, "launch_target"); v != "" {
		profile.LaunchTarget = v
	}
	if v := stringArg(args, "integration_template"); v != "" {
		profile.IntegrationTemplate = v
	}
	if v := stringArg(args, "notes"); v != "" {
		profile.Notes = v
	}
	if _, ok := args["enabled"]; ok {
		profile.Enabled = boolArg(args, "enabled")
	}
	return profile, nil
}

func scheduleSetTool(tc ToolContext, args map[string]any) (any, error) {
	at, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(stringArg(args, "at")))
	if err != nil {
		return nil, apperr.New(apperr.CodeInvalidInput, "at must be an RFC3339 instant with a timezone")
	}
	return tc.Server.Workspace.Actions.SetTicketSchedule(tc.Context, stringArg(args, "ticket_id"), at, contracts.Actor(stringArg(args, "runner")), contracts.Actor(tc.Actor), tc.Reason)
}

func scheduleClearTool(tc ToolContext, args map[string]any) (any, error) {
	return tc.Server.Workspace.Actions.ClearTicketSchedule(tc.Context, stringArg(args, "ticket_id"), contracts.Actor(tc.Actor), tc.Reason)
}

func ticketRequestReviewTool(tc ToolContext, args map[string]any) (any, error) {
	ctx, err := mcpContextWithDependencyOverride(tc, args)
	if err != nil {
		return nil, err
	}
	return tc.Server.Workspace.Actions.RequestReviewWithReviewer(ctx, stringArg(args, "ticket_id"), contracts.Actor(stringArg(args, "reviewer")), contracts.Actor(tc.Actor), tc.Reason)
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

func rawStringSliceArg(args map[string]any, key string) []string {
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
			items = append(items, item.(string))
		}
		return items
	default:
		return nil
	}
}

func scheduleQueryArgs(args map[string]any) (service.ScheduleQuery, error) {
	query := service.ScheduleQuery{Project: strings.TrimSpace(stringArg(args, "project"))}
	for key, target := range map[string]*time.Time{"from": &query.From, "to": &query.To} {
		raw := strings.TrimSpace(stringArg(args, key))
		if raw == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return service.ScheduleQuery{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("%s must be an RFC3339 instant with a timezone", key))
		}
		*target = parsed.UTC()
	}
	return query, nil
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
