package service

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type PermissionBindingLayer string

const (
	PermissionLayerWorkspace PermissionBindingLayer = "workspace_default"
	PermissionLayerProject   PermissionBindingLayer = "project_default"
	PermissionLayerAgent     PermissionBindingLayer = "agent_binding"
	PermissionLayerRunbook   PermissionBindingLayer = "runbook_binding"
	PermissionLayerTicket    PermissionBindingLayer = "ticket_overlay"
)

type PermissionProfileMatch struct {
	Layer     PermissionBindingLayer      `json:"layer"`
	ProfileID string                      `json:"profile_id"`
	Profile   contracts.PermissionProfile `json:"profile"`
}

type PermissionDecisionView struct {
	Action                contracts.PermissionAction `json:"action"`
	Allowed               bool                       `json:"allowed"`
	RequiresOwnerOverride bool                       `json:"requires_owner_override,omitempty"`
	OverrideApplied       bool                       `json:"override_applied,omitempty"`
	ReasonCodes           []string                   `json:"reason_codes,omitempty"`
	Profiles              []PermissionProfileMatch   `json:"profiles,omitempty"`
	ChangedFilesKnown     bool                       `json:"changed_files_known"`
	ChangedFiles          []string                   `json:"changed_files,omitempty"`
}

type PermissionsView struct {
	Target      string                   `json:"target"`
	Actor       contracts.Actor          `json:"actor"`
	TicketID    string                   `json:"ticket_id,omitempty"`
	RunID       string                   `json:"run_id,omitempty"`
	ChangeID    string                   `json:"change_id,omitempty"`
	GateID      string                   `json:"gate_id,omitempty"`
	Project     string                   `json:"project,omitempty"`
	Protected   bool                     `json:"protected,omitempty"`
	Sensitive   bool                     `json:"sensitive,omitempty"`
	Decisions   []PermissionDecisionView `json:"decisions"`
	GeneratedAt time.Time                `json:"generated_at"`
}

type permissionEvalInput struct {
	Action            contracts.PermissionAction
	Actor             contracts.Actor
	Ticket            contracts.TicketSnapshot
	Run               *contracts.RunSnapshot
	Change            *contracts.ChangeRef
	Gate              *contracts.GateSnapshot
	ActorAgent        *contracts.AgentProfile
	Runbook           string
	ChangedFiles      []string
	ChangedFilesKnown bool
}

type permissionEvaluator struct {
	root     string
	projects contracts.ProjectStore
	tickets  contracts.TicketStore
	profiles contracts.PermissionProfileStore
	agents   contracts.AgentStore
	runbooks contracts.RunbookStore
	clock    func() time.Time
}

func (s *QueryService) PermissionsView(ctx context.Context, target string, actor contracts.Actor, action contracts.PermissionAction) (PermissionsView, error) {
	ctx = contextWithDefaultReplayMode(ctx)
	ticket, run, change, gate, err := s.resolvePermissionTarget(ctx, target)
	if err != nil {
		return PermissionsView{}, err
	}
	if actor == "" {
		resolved, resolveErr := s.ResolveActor(ctx, "")
		if resolveErr == nil {
			actor = resolved
		}
	}
	actorAgent, _ := actorAgentProfile(ctx, s.Agents, actor)
	runbook := permissionRunbook(ticket, run)
	view := PermissionsView{
		Target:      strings.TrimSpace(target),
		Actor:       actor,
		TicketID:    ticket.ID,
		Project:     ticket.Project,
		Protected:   ticket.Protected,
		Sensitive:   ticket.Sensitive,
		GeneratedAt: s.now(),
	}
	if run != nil {
		view.RunID = run.RunID
	}
	if change != nil {
		view.ChangeID = change.ChangeID
	}
	if gate != nil {
		view.GateID = gate.GateID
	}
	evaluator := permissionEvaluator{root: s.Root, projects: s.Projects, tickets: s.Tickets, profiles: s.PermissionProfiles, agents: s.Agents, runbooks: s.Runbooks, clock: s.Clock}
	actions := []contracts.PermissionAction{
		contracts.PermissionActionDispatch,
		contracts.PermissionActionRunLaunch,
		contracts.PermissionActionChangeCreate,
		contracts.PermissionActionChangeMerge,
		contracts.PermissionActionGateOpen,
		contracts.PermissionActionGateApprove,
		contracts.PermissionActionRunComplete,
		contracts.PermissionActionTicketComplete,
	}
	if action != "" {
		actions = []contracts.PermissionAction{action}
	}
	changedFiles, changedKnown := s.permissionChangedFilesForView(ctx, ticket, run, change)
	for _, candidate := range actions {
		decision, err := evaluator.evaluate(ctx, permissionEvalInput{
			Action:            candidate,
			Actor:             actor,
			Ticket:            ticket,
			Run:               run,
			Change:            change,
			Gate:              gate,
			ActorAgent:        actorAgent,
			Runbook:           runbook,
			ChangedFiles:      changedFiles,
			ChangedFilesKnown: changedKnown,
		})
		if err != nil {
			return PermissionsView{}, err
		}
		view.Decisions = append(view.Decisions, decision)
	}
	return view, nil
}

func (s *ActionService) requirePermission(ctx context.Context, input permissionEvalInput) (PermissionDecisionView, error) {
	if !input.Actor.IsValid() {
		return PermissionDecisionView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", input.Actor))
	}
	evaluator := permissionEvaluator{root: s.Root, projects: s.Projects, tickets: s.Tickets, profiles: s.PermissionProfiles, agents: s.Agents, runbooks: s.Runbooks, clock: s.Clock}
	decision, err := evaluator.evaluate(ctx, input)
	if err != nil {
		return PermissionDecisionView{}, err
	}
	if decision.RequiresOwnerOverride && decision.OverrideApplied {
		if err := s.recordPermissionOverride(ctx, input, decision); err != nil {
			return PermissionDecisionView{}, err
		}
	}
	if !decision.Allowed {
		message := fmt.Sprintf("%s cannot %s on %s", input.Actor, input.Action, input.Ticket.ID)
		if len(decision.ReasonCodes) > 0 {
			message += fmt.Sprintf(" (%s)", strings.Join(decision.ReasonCodes, ", "))
		}
		return decision, apperr.New(apperr.CodePermissionDenied, message)
	}
	return decision, nil
}

func (s *ActionService) recordPermissionOverride(ctx context.Context, input permissionEvalInput, decision PermissionDecisionView) error {
	payload := map[string]any{
		"ticket":       input.Ticket,
		"action":       input.Action,
		"reason_codes": decision.ReasonCodes,
	}
	if input.Run != nil {
		payload["run"] = *input.Run
	}
	if input.Change != nil {
		payload["change"] = *input.Change
	}
	if input.Gate != nil {
		payload["gate"] = *input.Gate
	}
	event, err := s.newEvent(ctx, input.Ticket.Project, s.now(), input.Actor, "owner override applied", contracts.EventPermissionProfileOverrideApplied, input.Ticket.ID, payload)
	if err != nil {
		return err
	}
	return s.commitMutation(ctx, "record permission override", "event_only", event, nil)
}

func (e permissionEvaluator) evaluate(ctx context.Context, input permissionEvalInput) (PermissionDecisionView, error) {
	decision := PermissionDecisionView{Action: input.Action, Allowed: true, ChangedFilesKnown: input.ChangedFilesKnown, ChangedFiles: append([]string{}, input.ChangedFiles...)}
	if input.Action == "" {
		return decision, apperr.New(apperr.CodeInvalidInput, "permission action is required")
	}
	profiles, err := e.applicableProfiles(ctx, input.Ticket, input.ActorAgent, input.Runbook)
	if err != nil {
		return PermissionDecisionView{}, err
	}
	decision.Profiles = profiles
	if len(profiles) == 0 {
		return decision, nil
	}

	reasonCodes := make([]string, 0)
	allowPatterns := make([]string, 0)
	forbiddenPatterns := make([]string, 0)
	hasActionAllowList := false
	actionAllowed := false
	for _, match := range profiles {
		profile := match.Profile
		if len(profile.AllowedProjects) > 0 && !permissionStringSliceContains(profile.AllowedProjects, input.Ticket.Project) {
			reasonCodes = append(reasonCodes, "permission_project_denied")
		}
		if len(profile.AllowedTicketTypes) > 0 && !containsTicketType(profile.AllowedTicketTypes, input.Ticket.Type) {
			reasonCodes = append(reasonCodes, "permission_ticket_type_denied")
		}
		if len(profile.AllowedRunbooks) > 0 && !permissionStringSliceContains(profile.AllowedRunbooks, input.Runbook) {
			reasonCodes = append(reasonCodes, "permission_runbook_denied")
		}
		if len(profile.AllowedCapabilities) > 0 {
			if input.ActorAgent == nil {
				reasonCodes = append(reasonCodes, "permission_capability_denied")
			} else if missing := missingCapabilities(input.ActorAgent.Capabilities, profile.AllowedCapabilities); len(missing) > 0 {
				reasonCodes = append(reasonCodes, "permission_capability_denied")
			}
		}
		if profile.RequiresOwnerForSensitiveOps && (input.Ticket.Protected || input.Ticket.Sensitive) {
			decision.RequiresOwnerOverride = true
			if input.Actor != contracts.Actor("human:owner") {
				reasonCodes = append(reasonCodes, "owner_override_required")
			} else {
				decision.OverrideApplied = true
			}
		}
		if len(profile.AllowActions) > 0 {
			hasActionAllowList = true
			if permissionActionContains(profile.AllowActions, input.Action) {
				actionAllowed = true
			}
		}
		if permissionActionContains(profile.DenyActions, input.Action) {
			reasonCodes = append(reasonCodes, "permission_action_denied")
		}
		allowPatterns = append(allowPatterns, profile.AllowedPaths...)
		forbiddenPatterns = append(forbiddenPatterns, profile.ForbiddenPaths...)
	}
	if hasActionAllowList && !actionAllowed {
		reasonCodes = append(reasonCodes, "permission_action_not_allowed")
	}
	decision.ReasonCodes = dedupeStrings(reasonCodes)
	if len(allowPatterns) > 0 || len(forbiddenPatterns) > 0 {
		pathReasonCodes := evaluatePathRestrictions(allowPatterns, forbiddenPatterns, input.ChangedFiles, input.ChangedFilesKnown)
		decision.ReasonCodes = dedupeStrings(append(decision.ReasonCodes, pathReasonCodes...))
	}
	decision.Allowed = len(decision.ReasonCodes) == 0
	return decision, nil
}

func (e permissionEvaluator) applicableProfiles(ctx context.Context, ticket contracts.TicketSnapshot, actorAgent *contracts.AgentProfile, runbook string) ([]PermissionProfileMatch, error) {
	profiles, err := e.profiles.ListPermissionProfiles(ctx)
	if err != nil {
		return nil, err
	}
	project, err := e.projects.GetProject(ctx, ticket.Project)
	if err != nil {
		project = contracts.Project{Key: ticket.Project}
	}
	projectDefaultIDs := make(map[string]struct{}, len(project.Defaults.PermissionProfiles))
	for _, id := range project.Defaults.PermissionProfiles {
		projectDefaultIDs[strings.TrimSpace(id)] = struct{}{}
	}
	ticketIDs := make(map[string]struct{}, len(ticket.PermissionProfiles))
	for _, id := range ticket.PermissionProfiles {
		ticketIDs[strings.TrimSpace(id)] = struct{}{}
	}
	result := make([]PermissionProfileMatch, 0)
	appendMatches := func(layer PermissionBindingLayer, filter func(contracts.PermissionProfile) bool) {
		matched := make([]PermissionProfileMatch, 0)
		for _, profile := range profiles {
			if !filter(profile) {
				continue
			}
			matched = append(matched, PermissionProfileMatch{Layer: layer, ProfileID: profile.ProfileID, Profile: profile})
		}
		sort.Slice(matched, func(i, j int) bool {
			if matched[i].Profile.Priority != matched[j].Profile.Priority {
				return matched[i].Profile.Priority > matched[j].Profile.Priority
			}
			return matched[i].ProfileID < matched[j].ProfileID
		})
		result = append(result, matched...)
	}
	appendMatches(PermissionLayerWorkspace, func(profile contracts.PermissionProfile) bool {
		return profile.WorkspaceDefault
	})
	appendMatches(PermissionLayerProject, func(profile contracts.PermissionProfile) bool {
		if _, ok := projectDefaultIDs[profile.ProfileID]; ok {
			return true
		}
		return permissionStringSliceContains(profile.Projects, ticket.Project)
	})
	appendMatches(PermissionLayerAgent, func(profile contracts.PermissionProfile) bool {
		return actorAgent != nil && permissionStringSliceContains(profile.Agents, actorAgent.AgentID)
	})
	appendMatches(PermissionLayerRunbook, func(profile contracts.PermissionProfile) bool {
		return strings.TrimSpace(runbook) != "" && permissionStringSliceContains(profile.Runbooks, runbook)
	})
	appendMatches(PermissionLayerTicket, func(profile contracts.PermissionProfile) bool {
		_, ok := ticketIDs[profile.ProfileID]
		return ok
	})
	return result, nil
}

func (s *QueryService) resolvePermissionTarget(ctx context.Context, target string) (contracts.TicketSnapshot, *contracts.RunSnapshot, *contracts.ChangeRef, *contracts.GateSnapshot, error) {
	raw := strings.TrimSpace(target)
	if raw == "" {
		return contracts.TicketSnapshot{}, nil, nil, nil, apperr.New(apperr.CodeInvalidInput, "permission target is required")
	}
	parts := strings.SplitN(raw, ":", 2)
	kind := "ticket"
	id := raw
	if len(parts) == 2 {
		kind = strings.TrimSpace(parts[0])
		id = strings.TrimSpace(parts[1])
	}
	switch kind {
	case "ticket":
		ticket, err := s.Tickets.GetTicket(ctx, id)
		return ticket, nil, nil, nil, err
	case "run":
		run, err := s.Runs.LoadRun(ctx, id)
		if err != nil {
			return contracts.TicketSnapshot{}, nil, nil, nil, err
		}
		ticket, err := s.Tickets.GetTicket(ctx, run.TicketID)
		if err != nil {
			return contracts.TicketSnapshot{}, nil, nil, nil, err
		}
		return ticket, &run, nil, nil, nil
	case "change":
		change, err := s.Changes.LoadChange(ctx, id)
		if err != nil {
			return contracts.TicketSnapshot{}, nil, nil, nil, err
		}
		ticket, err := s.Tickets.GetTicket(ctx, change.TicketID)
		if err != nil {
			return contracts.TicketSnapshot{}, nil, nil, nil, err
		}
		var run *contracts.RunSnapshot
		if strings.TrimSpace(change.RunID) != "" {
			loadedRun, err := s.Runs.LoadRun(ctx, change.RunID)
			if err == nil {
				run = &loadedRun
			}
		}
		return ticket, run, &change, nil, nil
	case "gate":
		gate, err := s.Gates.LoadGate(ctx, id)
		if err != nil {
			return contracts.TicketSnapshot{}, nil, nil, nil, err
		}
		ticket, err := s.Tickets.GetTicket(ctx, gate.TicketID)
		if err != nil {
			return contracts.TicketSnapshot{}, nil, nil, nil, err
		}
		var run *contracts.RunSnapshot
		if strings.TrimSpace(gate.RunID) != "" {
			loadedRun, err := s.Runs.LoadRun(ctx, gate.RunID)
			if err == nil {
				run = &loadedRun
			}
		}
		return ticket, run, nil, &gate, nil
	default:
		return contracts.TicketSnapshot{}, nil, nil, nil, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("unsupported permission target: %s", target))
	}
}

func (s *QueryService) permissionChangedFilesForView(ctx context.Context, ticket contracts.TicketSnapshot, run *contracts.RunSnapshot, change *contracts.ChangeRef) ([]string, bool) {
	if change != nil {
		files, known := permissionChangedFilesForChange(ctx, s.Runs, s.Root, *change)
		return files, known
	}
	if run != nil {
		files, known := permissionChangedFilesForRun(ctx, s.Runs, s.Root, *run)
		return files, known
	}
	return permissionChangedFilesForTicket(ctx, s.Runs, s.Changes, s.Root, ticket)
}

func actorAgentProfile(ctx context.Context, store contracts.AgentStore, actor contracts.Actor) (*contracts.AgentProfile, error) {
	if !strings.HasPrefix(string(actor), "agent:") {
		return nil, nil
	}
	agentID := strings.TrimSpace(strings.TrimPrefix(string(actor), "agent:"))
	if agentID == "" {
		return nil, nil
	}
	profile, err := store.LoadAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

func maybeAgentProfile(profile contracts.AgentProfile) *contracts.AgentProfile {
	if strings.TrimSpace(profile.AgentID) == "" {
		return nil
	}
	return &profile
}

func permissionRunbook(ticket contracts.TicketSnapshot, run *contracts.RunSnapshot) string {
	if run != nil && strings.TrimSpace(run.BlueprintStage) != "" {
		// The runbook name still lives on the ticket; the stage just confirms a runbook is active.
	}
	if strings.TrimSpace(ticket.Runbook) != "" {
		return strings.TrimSpace(ticket.Runbook)
	}
	return defaultRunbookName(ticket.Type)
}

func permissionChangedFilesForRun(ctx context.Context, runs contracts.RunStore, root string, run contracts.RunSnapshot) ([]string, bool) {
	change := contracts.ChangeRef{RunID: run.RunID, BranchName: run.BranchName}
	scm, branch, currentBranch, err := changeSCMTarget(ctx, runs, root, change)
	if err != nil {
		return nil, false
	}
	files, err := observedChangedFiles(ctx, scm, change, branch, currentBranch)
	if err != nil {
		return nil, false
	}
	return normalizeChangedFiles(files), true
}

func permissionChangedFilesForChange(ctx context.Context, runs contracts.RunStore, root string, change contracts.ChangeRef) ([]string, bool) {
	scm, branch, currentBranch, err := changeSCMTarget(ctx, runs, root, change)
	if err != nil {
		return nil, false
	}
	files, err := observedChangedFiles(ctx, scm, change, branch, currentBranch)
	if err != nil {
		return nil, false
	}
	return normalizeChangedFiles(files), true
}

func permissionChangedFilesForTicket(ctx context.Context, runs contracts.RunStore, changes contracts.ChangeStore, root string, ticket contracts.TicketSnapshot) ([]string, bool) {
	if len(ticket.ChangeIDs) > 0 {
		files := make([]string, 0)
		for _, changeID := range ticket.ChangeIDs {
			change, err := changes.LoadChange(ctx, changeID)
			if err != nil {
				return nil, false
			}
			changed, known := permissionChangedFilesForChange(ctx, runs, root, change)
			if !known {
				return nil, false
			}
			files = append(files, changed...)
		}
		return normalizeChangedFiles(files), true
	}
	if strings.TrimSpace(ticket.LatestRunID) != "" {
		run, err := runs.LoadRun(ctx, ticket.LatestRunID)
		if err != nil {
			return nil, false
		}
		return permissionChangedFilesForRun(ctx, runs, root, run)
	}
	return nil, false
}

func normalizeChangedFiles(files []string) []string {
	cleaned := make([]string, 0, len(files))
	seen := map[string]struct{}{}
	for _, file := range files {
		normalized := normalizeRepoPath(file)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		cleaned = append(cleaned, normalized)
	}
	sort.Strings(cleaned)
	return cleaned
}

func evaluatePathRestrictions(allowPatterns []string, forbiddenPatterns []string, changedFiles []string, changedFilesKnown bool) []string {
	allowPatterns = normalizePatterns(allowPatterns)
	forbiddenPatterns = normalizePatterns(forbiddenPatterns)
	if len(allowPatterns) == 0 && len(forbiddenPatterns) == 0 {
		return nil
	}
	if !changedFilesKnown {
		return []string{"unverifiable_path_scope"}
	}
	reasons := make([]string, 0)
	for _, file := range normalizeChangedFiles(changedFiles) {
		if matchesAnyPattern(file, forbiddenPatterns) {
			reasons = append(reasons, "permission_forbidden_path")
		}
		if len(allowPatterns) > 0 && !matchesAnyPattern(file, allowPatterns) {
			reasons = append(reasons, "permission_path_not_allowed")
		}
	}
	return dedupeStrings(reasons)
}

func normalizePatterns(values []string) []string {
	patterns := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		normalized := normalizeRepoPath(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		patterns = append(patterns, normalized)
	}
	sort.Strings(patterns)
	return patterns
}

func normalizeRepoPath(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "./")
	value = strings.TrimPrefix(value, "/")
	value = strings.ReplaceAll(value, "\\", "/")
	value = path.Clean(value)
	if value == "." {
		return ""
	}
	return value
}

func matchesAnyPattern(file string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchesRepoPattern(file, pattern) {
			return true
		}
	}
	return false
}

func matchesRepoPattern(file string, pattern string) bool {
	file = normalizeRepoPath(file)
	pattern = normalizeRepoPath(pattern)
	if file == "" || pattern == "" {
		return false
	}
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return file == prefix || strings.HasPrefix(file, prefix+"/")
	}
	if matched, err := path.Match(pattern, file); err == nil && matched {
		return true
	}
	return file == pattern
}

func permissionActionContains(values []contracts.PermissionAction, target contracts.PermissionAction) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func permissionStringSliceContains(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}
