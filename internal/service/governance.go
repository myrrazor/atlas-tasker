package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type GovernancePackCreateOptions struct {
	Name                    string
	PolicyID                string
	Scope                   string
	ProtectedActions        []contracts.ProtectedAction
	RequiredSignatures      int
	QuorumCount             int
	QuorumRoles             []contracts.MembershipRole
	SeparationEventTypes    []contracts.EventType
	AllowOwnerOverride      bool
	RequireOverrideReason   bool
	RequireTrustedSignature bool
}

type GovernanceEvaluationInput struct {
	Action                contracts.ProtectedAction
	Target                string
	Actor                 contracts.Actor
	TicketID              string
	RunID                 string
	ChangeID              string
	GateID                string
	ApprovalActors        []contracts.Actor
	TrustedSignatureCount int
	Reason                string
}

type GovernancePackDetailView struct {
	Kind        string               `json:"kind"`
	GeneratedAt time.Time            `json:"generated_at"`
	Pack        contracts.PolicyPack `json:"pack"`
}

type GovernancePackListView struct {
	Kind        string                 `json:"kind"`
	GeneratedAt time.Time              `json:"generated_at"`
	Items       []contracts.PolicyPack `json:"items"`
}

type GovernanceValidationView struct {
	Kind        string    `json:"kind"`
	GeneratedAt time.Time `json:"generated_at"`
	Valid       bool      `json:"valid"`
	Errors      []string  `json:"errors,omitempty"`
	Policies    int       `json:"policies"`
	Packs       int       `json:"packs"`
}

type governanceTargetContext struct {
	Target   string
	Ticket   *contracts.TicketSnapshot
	Run      *contracts.RunSnapshot
	Change   *contracts.ChangeRef
	Gate     *contracts.GateSnapshot
	Project  string
	Runbook  string
	ScopeIDs map[string]string
}

func (s *ActionService) CreateGovernancePack(ctx context.Context, opts GovernancePackCreateOptions, actor contracts.Actor, reason string) (GovernancePackDetailView, error) {
	return withWriteLock(ctx, s.LockManager, "create governance pack", func(ctx context.Context) (GovernancePackDetailView, error) {
		if !actor.IsValid() {
			return GovernancePackDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if strings.TrimSpace(reason) == "" {
			return GovernancePackDetailView{}, apperr.New(apperr.CodeInvalidInput, "reason is required")
		}
		scopeKind, scopeID, err := parseGovernanceScope(opts.Scope)
		if err != nil {
			return GovernancePackDetailView{}, err
		}
		now := s.now()
		name := strings.TrimSpace(opts.Name)
		policyID := strings.TrimSpace(opts.PolicyID)
		if policyID == "" {
			policyID = name
		}
		actions := opts.ProtectedActions
		if len(actions) == 0 {
			actions = []contracts.ProtectedAction{contracts.ProtectedActionTicketComplete}
		}
		policy := contracts.GovernancePolicy{
			PolicyID:           policyID,
			Name:               name,
			ScopeKind:          scopeKind,
			ScopeID:            scopeID,
			ProtectedActions:   actions,
			RequiredSignatures: opts.RequiredSignatures,
			CreatedAt:          now,
			UpdatedAt:          now,
			SchemaVersion:      contracts.CurrentSchemaVersion,
		}
		for _, action := range actions {
			if opts.QuorumCount > 0 {
				policy.QuorumRules = append(policy.QuorumRules, contracts.QuorumRule{
					RuleID:                       sanitizeGovernanceID(fmt.Sprintf("%s-%s-quorum", policyID, action), "quorum"),
					ActionKind:                   action,
					RequiredCount:                opts.QuorumCount,
					AllowedRoles:                 append([]contracts.MembershipRole{}, opts.QuorumRoles...),
					RequireDistinctCollaborators: true,
					RequireTrustedSignatures:     opts.RequireTrustedSignature,
				})
			}
			if len(opts.SeparationEventTypes) > 0 {
				policy.SeparationOfDutiesRules = append(policy.SeparationOfDutiesRules, contracts.SeparationOfDutiesRule{
					RuleID:                      sanitizeGovernanceID(fmt.Sprintf("%s-%s-separation", policyID, action), "separation"),
					ActionKind:                  action,
					ForbiddenActorRelationships: []string{"same_collaborator", "same_actor"},
					LookbackEventTypes:          append([]contracts.EventType{}, opts.SeparationEventTypes...),
					LookbackScope:               "ticket",
				})
			}
			if opts.AllowOwnerOverride {
				policy.OverrideRules = append(policy.OverrideRules, contracts.OverrideRule{
					RuleID:                  sanitizeGovernanceID(fmt.Sprintf("%s-%s-owner-override", policyID, action), "owner-override"),
					ActionKind:              action,
					Allowed:                 true,
					RequireReason:           opts.RequireOverrideReason,
					RequireTrustedSignature: opts.RequireTrustedSignature,
				})
			}
		}
		policy = normalizeGovernancePolicy(policy)
		pack := normalizeGovernancePack(contracts.PolicyPack{
			PackID:        name,
			Name:          name,
			Policies:      []contracts.GovernancePolicy{policy},
			CreatedAt:     now,
			UpdatedAt:     now,
			SchemaVersion: contracts.CurrentSchemaVersion,
		})
		event, err := s.newEvent(ctx, workspaceEventProject, now, actor, reason, contracts.EventGovernancePackCreated, "", pack)
		if err != nil {
			return GovernancePackDetailView{}, err
		}
		if err := s.commitMutation(ctx, "create governance pack", "governance_pack", event, func(ctx context.Context) error {
			return s.GovernancePacks.SaveGovernancePack(ctx, pack)
		}); err != nil {
			return GovernancePackDetailView{}, err
		}
		return GovernancePackDetailView{Kind: "governance_pack_detail", GeneratedAt: s.now(), Pack: pack}, nil
	})
}

func (s *ActionService) ApplyGovernancePack(ctx context.Context, packID string, scope string, actor contracts.Actor, reason string) (GovernancePackDetailView, error) {
	return withWriteLock(ctx, s.LockManager, "apply governance pack", func(ctx context.Context) (GovernancePackDetailView, error) {
		if !actor.IsValid() {
			return GovernancePackDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if strings.TrimSpace(reason) == "" {
			return GovernancePackDetailView{}, apperr.New(apperr.CodeInvalidInput, "reason is required")
		}
		pack, err := s.GovernancePacks.LoadGovernancePack(ctx, packID)
		if err != nil {
			return GovernancePackDetailView{}, err
		}
		scopeKind, scopeID, hasScope, err := parseOptionalGovernanceScope(scope)
		if err != nil {
			return GovernancePackDetailView{}, err
		}
		now := s.now()
		applied := pack
		applied.UpdatedAt = now
		for i := range applied.Policies {
			applied.Policies[i].UpdatedAt = now
			if hasScope {
				applied.Policies[i].PolicyID = scopedGovernancePolicyID(applied.Policies[i].PolicyID, scopeKind, scopeID)
				applied.Policies[i].ScopeKind = scopeKind
				applied.Policies[i].ScopeID = scopeID
			}
			applied.Policies[i] = normalizeGovernancePolicy(applied.Policies[i])
		}
		event, err := s.newEvent(ctx, workspaceEventProject, now, actor, reason, contracts.EventGovernancePackApplied, "", applied)
		if err != nil {
			return GovernancePackDetailView{}, err
		}
		if err := s.commitMutation(ctx, "apply governance pack", "governance_policy", event, func(ctx context.Context) error {
			for _, policy := range applied.Policies {
				if err := s.GovernancePolicies.SaveGovernancePolicy(ctx, policy); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return GovernancePackDetailView{}, err
		}
		return GovernancePackDetailView{Kind: "governance_pack_detail", GeneratedAt: s.now(), Pack: applied}, nil
	})
}

func (s *ActionService) ListGovernancePacks(ctx context.Context) (GovernancePackListView, error) {
	packs, err := s.GovernancePacks.ListGovernancePacks(ctx)
	if err != nil {
		return GovernancePackListView{}, err
	}
	return GovernancePackListView{Kind: "governance_pack_list", GeneratedAt: s.now(), Items: packs}, nil
}

func (s *ActionService) GovernancePackDetail(ctx context.Context, packID string) (GovernancePackDetailView, error) {
	pack, err := s.GovernancePacks.LoadGovernancePack(ctx, packID)
	if err != nil {
		return GovernancePackDetailView{}, err
	}
	return GovernancePackDetailView{Kind: "governance_pack_detail", GeneratedAt: s.now(), Pack: pack}, nil
}

func (s *ActionService) ValidateGovernance(ctx context.Context) (GovernanceValidationView, error) {
	view := GovernanceValidationView{Kind: "governance_validation_result", GeneratedAt: s.now(), Valid: true}
	policies, err := s.GovernancePolicies.ListGovernancePolicies(ctx)
	if err != nil {
		view.Valid = false
		view.Errors = append(view.Errors, fmt.Sprintf("policy store: %v", err))
	}
	packs, err := s.GovernancePacks.ListGovernancePacks(ctx)
	if err != nil {
		view.Valid = false
		view.Errors = append(view.Errors, fmt.Sprintf("pack store: %v", err))
	}
	view.Policies = len(policies)
	view.Packs = len(packs)
	for _, policy := range policies {
		if err := policy.Validate(); err != nil {
			view.Valid = false
			view.Errors = append(view.Errors, fmt.Sprintf("policy %s: %v", policy.PolicyID, err))
			continue
		}
		if err := validateGovernancePolicyRuntime(policy); err != nil {
			view.Valid = false
			view.Errors = append(view.Errors, fmt.Sprintf("policy %s: %v", policy.PolicyID, err))
		}
	}
	for _, pack := range packs {
		if err := pack.Validate(); err != nil {
			view.Valid = false
			view.Errors = append(view.Errors, fmt.Sprintf("pack %s: %v", pack.PackID, err))
			continue
		}
		for _, policy := range pack.Policies {
			if err := validateGovernancePolicyRuntime(policy); err != nil {
				view.Valid = false
				view.Errors = append(view.Errors, fmt.Sprintf("pack %s policy %s: %v", pack.PackID, policy.PolicyID, err))
			}
		}
	}
	sort.Strings(view.Errors)
	return view, nil
}

func (s *ActionService) ExplainGovernance(ctx context.Context, input GovernanceEvaluationInput) (contracts.GovernanceExplanation, error) {
	explanation, _, err := s.evaluateGovernance(ctx, input)
	return explanation, err
}

func (s *ActionService) SimulateGovernance(ctx context.Context, input GovernanceEvaluationInput) (contracts.GovernanceSimulationResult, error) {
	explanation, err := s.ExplainGovernance(ctx, input)
	if err != nil {
		return contracts.GovernanceSimulationResult{}, err
	}
	return contracts.GovernanceSimulationResult{Explanation: explanation, DryRun: true}, nil
}

func (s *ActionService) requireGovernance(ctx context.Context, input GovernanceEvaluationInput) (contracts.GovernanceExplanation, error) {
	explanation, _, err := s.evaluateGovernance(ctx, input)
	if err != nil {
		return explanation, err
	}
	if !explanation.Allowed {
		return explanation, apperr.New(apperr.CodePermissionDenied, fmt.Sprintf("%s cannot %s on %s (%s)", input.Actor, input.Action, explanation.Target, strings.Join(explanation.ReasonCodes, ", ")))
	}
	return explanation, nil
}

func (s *ActionService) evaluateGovernance(ctx context.Context, input GovernanceEvaluationInput) (contracts.GovernanceExplanation, bool, error) {
	if !input.Action.IsValid() {
		return contracts.GovernanceExplanation{}, false, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid protected action: %s", input.Action))
	}
	if input.Actor != "" && !input.Actor.IsValid() {
		return contracts.GovernanceExplanation{}, false, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", input.Actor))
	}
	target, err := s.resolveGovernanceTarget(ctx, input)
	if err != nil {
		return contracts.GovernanceExplanation{}, false, err
	}
	policies, err := s.GovernancePolicies.ListGovernancePolicies(ctx)
	if err != nil {
		return contracts.GovernanceExplanation{}, false, err
	}
	matched := make([]contracts.GovernancePolicy, 0)
	for _, policy := range policies {
		if governancePolicyMatches(policy, input.Action, target) {
			matched = append(matched, policy)
		}
	}
	explanation := contracts.GovernanceExplanation{
		Target:        target.Target,
		Action:        input.Action,
		Actor:         input.Actor,
		Allowed:       true,
		GeneratedAt:   s.now(),
		SchemaVersion: contracts.CurrentSchemaVersion,
		Inputs: map[string]string{
			"project":                  target.Project,
			"runbook":                  target.Runbook,
			"trusted_signature_count":  fmt.Sprintf("%d", input.TrustedSignatureCount),
			"explicit_approval_actors": fmt.Sprintf("%d", len(input.ApprovalActors)),
		},
	}
	if target.Ticket != nil {
		explanation.Inputs["ticket_id"] = target.Ticket.ID
	}
	if target.Run != nil {
		explanation.Inputs["run_id"] = target.Run.RunID
	}
	if target.Change != nil {
		explanation.Inputs["change_id"] = target.Change.ChangeID
	}
	if target.Gate != nil {
		explanation.Inputs["gate_id"] = target.Gate.GateID
	}
	if len(matched) == 0 {
		explanation.ReasonCodes = []string{"no_matching_governance_policy"}
		return explanation, false, nil
	}
	policyReasons := map[string][]string{}
	for _, policy := range matched {
		explanation.MatchedPolicies = append(explanation.MatchedPolicies, policy.PolicyID)
		reasons := s.evaluateGovernancePolicy(ctx, policy, input, target)
		if len(reasons) > 0 {
			policyReasons[policy.PolicyID] = reasons
		}
		explanation.ReasonCodes = append(explanation.ReasonCodes, reasons...)
	}
	explanation.MatchedPolicies = dedupeStrings(explanation.MatchedPolicies)
	explanation.ReasonCodes = dedupeStrings(explanation.ReasonCodes)
	overrideApplied := false
	if len(explanation.ReasonCodes) > 0 {
		if canApply, extraReasons := s.canApplyGovernanceOwnerOverride(matched, input, policyReasons); canApply {
			overrideReasons := s.evaluateOwnerOverrideGovernance(ctx, policies, input, target)
			if len(overrideReasons) > 0 {
				explanation.ReasonCodes = dedupeStrings(append(explanation.ReasonCodes, overrideReasons...))
				explanation.Allowed = false
			} else {
				explanation.Inputs["override_applied"] = "true"
				explanation.ReasonCodes = append(explanation.ReasonCodes, "owner_override_applied")
				overrideApplied = true
			}
		} else {
			explanation.ReasonCodes = dedupeStrings(append(explanation.ReasonCodes, extraReasons...))
			explanation.Allowed = false
		}
	}
	return explanation, overrideApplied, nil
}

func (s *ActionService) evaluateGovernancePolicy(ctx context.Context, policy contracts.GovernancePolicy, input GovernanceEvaluationInput, target governanceTargetContext) []string {
	reasons := make([]string, 0)
	if policy.RequiredSignatures > 0 && input.TrustedSignatureCount < policy.RequiredSignatures {
		reasons = append(reasons, "trusted_signature_required")
	}
	for _, rule := range policy.QuorumRules {
		if rule.ActionKind != input.Action {
			continue
		}
		count, details := s.countGovernanceApprovals(ctx, rule, input, target)
		if rule.RequireTrustedSignatures {
			count = input.TrustedSignatureCount
			details = nil
		}
		if count < rule.RequiredCount {
			reasons = append(reasons, fmt.Sprintf("quorum_unsatisfied:%s:%d/%d", rule.RuleID, count, rule.RequiredCount))
		}
		reasons = append(reasons, details...)
		if rule.RequireTrustedSignatures && input.TrustedSignatureCount < rule.RequiredCount {
			reasons = append(reasons, "trusted_signature_required")
		}
	}
	for _, rule := range policy.SeparationOfDutiesRules {
		if rule.ActionKind != input.Action {
			continue
		}
		if violated, reason := s.separationViolated(ctx, rule, input, target); violated {
			reasons = append(reasons, reason)
		}
	}
	return dedupeStrings(reasons)
}

func (s *ActionService) evaluateOwnerOverrideGovernance(ctx context.Context, policies []contracts.GovernancePolicy, input GovernanceEvaluationInput, target governanceTargetContext) []string {
	overrideInput := input
	overrideInput.Action = contracts.ProtectedActionOwnerOverride
	reasons := make([]string, 0)
	for _, policy := range policies {
		if !governancePolicyMatches(policy, contracts.ProtectedActionOwnerOverride, target) {
			continue
		}
		policyReasons := s.evaluateGovernancePolicy(ctx, policy, overrideInput, target)
		if len(policyReasons) == 0 {
			continue
		}
		reasons = append(reasons, "owner_override_policy_unsatisfied:"+policy.PolicyID)
		reasons = append(reasons, policyReasons...)
	}
	return dedupeStrings(reasons)
}

func (s *ActionService) canApplyGovernanceOwnerOverride(policies []contracts.GovernancePolicy, input GovernanceEvaluationInput, policyReasons map[string][]string) (bool, []string) {
	if input.Action == contracts.ProtectedActionOwnerOverride {
		return false, []string{"owner_override_cannot_override_owner_override"}
	}
	if input.Actor != contracts.Actor("human:owner") {
		return false, nil
	}
	extras := make([]string, 0)
	failedPolicies := 0
	for _, policy := range policies {
		reasons := policyReasons[policy.PolicyID]
		if len(reasons) == 0 {
			continue
		}
		failedPolicies++
		if ok, reasons := governancePolicyAllowsOwnerOverride(policy, input, reasons); !ok {
			extras = append(extras, reasons...)
		}
	}
	if failedPolicies == 0 {
		return false, nil
	}
	if len(extras) > 0 {
		return false, dedupeStrings(extras)
	}
	return true, nil
}

func governancePolicyAllowsOwnerOverride(policy contracts.GovernancePolicy, input GovernanceEvaluationInput, reasons []string) (bool, []string) {
	for _, reason := range reasons {
		if strings.HasPrefix(reason, "trusted_signature_required") {
			return false, []string{"owner_override_cannot_bypass_signature", "owner_override_cannot_bypass_signature:" + policy.PolicyID}
		}
	}
	for _, rule := range policy.OverrideRules {
		if rule.ActionKind != input.Action || !rule.Allowed {
			continue
		}
		if rule.RequireReason && strings.TrimSpace(input.Reason) == "" {
			return false, []string{"owner_override_reason_required", "owner_override_reason_required:" + policy.PolicyID}
		}
		if rule.RequireTrustedSignature && input.TrustedSignatureCount == 0 {
			return false, []string{"owner_override_signature_required", "owner_override_signature_required:" + policy.PolicyID}
		}
		return true, nil
	}
	return false, []string{"owner_override_not_allowed", "owner_override_not_allowed:" + policy.PolicyID}
}

func (s *ActionService) recordGovernanceOverride(ctx context.Context, input GovernanceEvaluationInput, explanation contracts.GovernanceExplanation) error {
	event, err := s.newEvent(ctx, governanceEventProject(explanation), s.now(), input.Actor, input.Reason, contracts.EventGovernanceOverrideRecorded, governanceEventTicket(explanation), map[string]any{
		"action":      input.Action,
		"target":      explanation.Target,
		"explanation": explanation,
	})
	if err != nil {
		return err
	}
	return s.commitMutation(ctx, "record governance override", "event_only", event, nil)
}

func (s *ActionService) recordGovernanceOverrideIfApplied(ctx context.Context, input GovernanceEvaluationInput, explanation contracts.GovernanceExplanation) error {
	if explanation.Inputs == nil || explanation.Inputs["override_applied"] != "true" {
		return nil
	}
	return s.recordGovernanceOverride(ctx, input, explanation)
}

func (s *ActionService) resolveGovernanceTarget(ctx context.Context, input GovernanceEvaluationInput) (governanceTargetContext, error) {
	target := strings.TrimSpace(input.Target)
	if target == "" {
		switch {
		case strings.TrimSpace(input.GateID) != "":
			target = "gate:" + strings.TrimSpace(input.GateID)
		case strings.TrimSpace(input.ChangeID) != "":
			target = "change:" + strings.TrimSpace(input.ChangeID)
		case strings.TrimSpace(input.RunID) != "":
			target = "run:" + strings.TrimSpace(input.RunID)
		case strings.TrimSpace(input.TicketID) != "":
			target = "ticket:" + strings.TrimSpace(input.TicketID)
		default:
			target = "workspace"
		}
	}
	out := governanceTargetContext{Target: target, ScopeIDs: map[string]string{}}
	if target == "workspace" {
		out.Project = workspaceEventProject
		level, _, _, err := s.effectiveClassification(ctx, contracts.ClassifiedEntityWorkspace, "workspace")
		if err != nil {
			return out, err
		}
		out.ScopeIDs["classification"] = string(level)
		return out, nil
	}
	if strings.HasPrefix(target, "project:") {
		projectID := strings.TrimSpace(strings.TrimPrefix(target, "project:"))
		if projectID == "" {
			return out, apperr.New(apperr.CodeInvalidInput, "project governance target requires an id")
		}
		out.Project = projectID
		out.ScopeIDs["project"] = projectID
		level, _, _, err := s.effectiveClassification(ctx, contracts.ClassifiedEntityProject, projectID)
		if err != nil {
			return out, err
		}
		out.ScopeIDs["classification"] = string(level)
		return out, nil
	}
	queries := NewQueryService(s.Root, s.Projects, s.Tickets, s.Events, s.Projection, s.Clock)
	ticket, run, change, gate, err := queries.resolvePermissionTarget(ctx, target)
	if err != nil {
		return out, err
	}
	out.Ticket = &ticket
	out.Project = ticket.Project
	out.Runbook = permissionRunbook(ticket, run)
	out.ScopeIDs["ticket"] = ticket.ID
	classificationKind := contracts.ClassifiedEntityTicket
	classificationID := ticket.ID
	targetKind, _, _ := strings.Cut(target, ":")
	if run != nil && (targetKind == "run" || targetKind == "change" || targetKind == "gate") {
		classificationKind = contracts.ClassifiedEntityRun
		classificationID = run.RunID
	}
	level, _, _, err := s.effectiveClassification(ctx, classificationKind, classificationID)
	if err != nil {
		return out, err
	}
	out.ScopeIDs["classification"] = string(level)
	if run != nil {
		out.Run = run
		out.ScopeIDs["run"] = run.RunID
	}
	if change != nil {
		out.Change = change
		out.ScopeIDs["change"] = change.ChangeID
	}
	if gate != nil {
		out.Gate = gate
		out.ScopeIDs["gate"] = gate.GateID
	}
	return out, nil
}

func governancePolicyMatches(policy contracts.GovernancePolicy, action contracts.ProtectedAction, target governanceTargetContext) bool {
	if len(policy.ProtectedActions) > 0 && !protectedActionContains(policy.ProtectedActions, action) {
		return false
	}
	switch policy.ScopeKind {
	case contracts.PolicyScopeWorkspace:
		return true
	case contracts.PolicyScopeProject:
		return target.Project != "" && target.Project == policy.ScopeID
	case contracts.PolicyScopeRunbook:
		return target.Runbook != "" && target.Runbook == policy.ScopeID
	case contracts.PolicyScopeTicketType:
		return target.Ticket != nil && string(target.Ticket.Type) == policy.ScopeID
	case contracts.PolicyScopeClassification:
		return contracts.ClassificationLevel(target.ScopeIDs["classification"]) == contracts.ClassificationLevel(policy.ScopeID)
	default:
		return false
	}
}

func (s *ActionService) countGovernanceApprovals(ctx context.Context, rule contracts.QuorumRule, input GovernanceEvaluationInput, target governanceTargetContext) (int, []string) {
	candidates := append([]contracts.Actor{}, input.ApprovalActors...)
	if target.Ticket != nil {
		gates, err := s.Gates.ListGates(ctx, target.Ticket.ID)
		if err == nil {
			for _, gate := range gates {
				if gate.State == contracts.GateStateApproved && gate.DecidedBy.IsValid() {
					candidates = append(candidates, gate.DecidedBy)
				}
			}
		}
	}
	actorRoot, _ := s.actorGovernanceIdentity(ctx, input.Actor, target.Project)
	seenRoots := map[string]struct{}{}
	count := 0
	reasons := make([]string, 0)
	for _, candidate := range candidates {
		identity, roles := s.actorGovernanceIdentity(ctx, candidate, target.Project)
		if identity.RootID == "" {
			continue
		}
		if identity.Status == contracts.CollaboratorStatusSuspended || identity.Status == contracts.CollaboratorStatusRemoved {
			reasons = append(reasons, "quorum_approval_inactive:"+identity.RootID)
			continue
		}
		if len(rule.AllowedCollaborators) > 0 && !permissionStringSliceContains(rule.AllowedCollaborators, strings.TrimPrefix(identity.RootID, "collaborator:")) {
			continue
		}
		if len(rule.AllowedRoles) > 0 && !membershipRolesIntersect(rule.AllowedRoles, roles) {
			continue
		}
		if rule.RequireDistinctCollaborators {
			if _, ok := seenRoots[identity.RootID]; ok {
				continue
			}
			seenRoots[identity.RootID] = struct{}{}
		}
		if len(rule.DisallowActorFromPriorRoles) > 0 && actorRoot.RootID != "" && actorRoot.RootID == identity.RootID {
			reasons = append(reasons, "quorum_actor_cannot_self_approve")
			continue
		}
		count++
	}
	return count, dedupeStrings(reasons)
}

func (s *ActionService) separationViolated(ctx context.Context, rule contracts.SeparationOfDutiesRule, input GovernanceEvaluationInput, target governanceTargetContext) (bool, string) {
	if target.Project == "" || len(rule.LookbackEventTypes) == 0 {
		return false, ""
	}
	events, err := s.Events.StreamEvents(ctx, target.Project, 0)
	if err != nil {
		return false, ""
	}
	current, _ := s.actorGovernanceIdentity(ctx, input.Actor, target.Project)
	for _, event := range events {
		if !eventTypeContains(rule.LookbackEventTypes, event.Type) {
			continue
		}
		if !eventInGovernanceScope(event, rule.LookbackScope, target) {
			continue
		}
		prior, _ := s.actorGovernanceIdentity(ctx, event.Actor, target.Project)
		if sameGovernanceActor(rule.ForbiddenActorRelationships, input.Actor, current.RootID, event.Actor, prior.RootID) {
			return true, "separation_of_duties_violation:" + rule.RuleID
		}
	}
	return false, ""
}

type governanceActorIdentity struct {
	RootID string
	Status contracts.CollaboratorStatus
}

func (s *ActionService) actorGovernanceIdentity(ctx context.Context, actor contracts.Actor, project string) (governanceActorIdentity, []contracts.MembershipRole) {
	if actor == "" {
		return governanceActorIdentity{}, nil
	}
	if actor == contracts.Actor("human:owner") {
		return governanceActorIdentity{RootID: "human:owner", Status: contracts.CollaboratorStatusActive}, []contracts.MembershipRole{contracts.MembershipRoleOwner}
	}
	collaborators, err := s.Collaborators.ListCollaborators(ctx)
	if err != nil {
		return governanceActorIdentity{RootID: string(actor), Status: contracts.CollaboratorStatusActive}, nil
	}
	for _, collaborator := range collaborators {
		for _, mapped := range collaborator.AtlasActors {
			if mapped != actor {
				continue
			}
			memberships, err := s.Memberships.ListMemberships(ctx, collaborator.CollaboratorID)
			if err != nil {
				return governanceActorIdentity{RootID: "collaborator:" + collaborator.CollaboratorID, Status: collaborator.Status}, nil
			}
			roles := make([]contracts.MembershipRole, 0)
			for _, membership := range activeMembershipsForProject(memberships, project) {
				roles = append(roles, membership.Role)
			}
			return governanceActorIdentity{RootID: "collaborator:" + collaborator.CollaboratorID, Status: collaborator.Status}, uniqueMembershipRoles(roles)
		}
	}
	return governanceActorIdentity{RootID: string(actor), Status: contracts.CollaboratorStatusActive}, nil
}

func parseGovernanceScope(raw string) (contracts.PolicyScopeKind, string, error) {
	kind, id, hasScope, err := parseOptionalGovernanceScope(raw)
	if err != nil {
		return "", "", err
	}
	if !hasScope {
		return contracts.PolicyScopeWorkspace, "", nil
	}
	return kind, id, nil
}

func parseOptionalGovernanceScope(raw string) (contracts.PolicyScopeKind, string, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "workspace" {
		return contracts.PolicyScopeWorkspace, "", raw != "", nil
	}
	parts := strings.SplitN(raw, ":", 2)
	kind := contracts.PolicyScopeKind(strings.TrimSpace(parts[0]))
	id := ""
	if len(parts) == 2 {
		id = strings.TrimSpace(parts[1])
	}
	if !kind.IsValid() {
		return "", "", false, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid governance scope: %s", raw))
	}
	if kind == contracts.PolicyScopeWorkspace {
		return kind, "", true, nil
	}
	if id == "" {
		return "", "", false, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("%s scope requires an id", kind))
	}
	return kind, id, true, nil
}

func protectedActionContains(values []contracts.ProtectedAction, value contracts.ProtectedAction) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func eventTypeContains(values []contracts.EventType, value contracts.EventType) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func membershipRolesIntersect(allowed []contracts.MembershipRole, actual []contracts.MembershipRole) bool {
	for _, left := range allowed {
		for _, right := range actual {
			if left == right {
				return true
			}
		}
	}
	return false
}

func uniqueMembershipRoles(values []contracts.MembershipRole) []contracts.MembershipRole {
	seen := map[contracts.MembershipRole]struct{}{}
	out := make([]contracts.MembershipRole, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func sameGovernanceActor(relationships []string, current contracts.Actor, currentRoot string, prior contracts.Actor, priorRoot string) bool {
	if len(relationships) == 0 {
		return currentRoot != "" && currentRoot == priorRoot
	}
	for _, relationship := range relationships {
		switch strings.TrimSpace(relationship) {
		case "same_actor":
			if current == prior {
				return true
			}
		case "same_collaborator", "same_root", "implemented", "dispatched", "created", "approved":
			if currentRoot != "" && currentRoot == priorRoot {
				return true
			}
		}
	}
	return false
}

func eventInGovernanceScope(event contracts.Event, scope string, target governanceTargetContext) bool {
	switch scope {
	case "ticket", "":
		return target.Ticket != nil && event.TicketID == target.Ticket.ID
	case "project":
		return target.Project != "" && event.Project == target.Project
	case "run":
		return target.Run != nil && eventPayloadContains(event.Payload, target.Run.RunID)
	case "change":
		return target.Change != nil && eventPayloadContains(event.Payload, target.Change.ChangeID)
	default:
		return false
	}
}

func eventPayloadContains(payload any, needle string) bool {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return false
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return false
	}
	return strings.Contains(string(raw), needle)
}

func governanceEventProject(explanation contracts.GovernanceExplanation) string {
	if project := strings.TrimSpace(explanation.Inputs["project"]); project != "" {
		return project
	}
	return workspaceEventProject
}

func governanceEventTicket(explanation contracts.GovernanceExplanation) string {
	if ticketID := strings.TrimSpace(explanation.Inputs["ticket_id"]); ticketID != "" {
		return ticketID
	}
	target := strings.TrimSpace(explanation.Target)
	if strings.HasPrefix(target, "ticket:") {
		return strings.TrimPrefix(target, "ticket:")
	}
	return ""
}
