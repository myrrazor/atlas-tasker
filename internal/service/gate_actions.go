package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func (s *ActionService) ApproveGate(ctx context.Context, gateID string, actor contracts.Actor, reason string) (contracts.GateSnapshot, error) {
	return s.decideGate(ctx, gateID, actor, reason, contracts.GateStateApproved, contracts.EventGateApproved)
}

func (s *ActionService) RejectGate(ctx context.Context, gateID string, actor contracts.Actor, reason string) (contracts.GateSnapshot, error) {
	return s.decideGate(ctx, gateID, actor, reason, contracts.GateStateRejected, contracts.EventGateRejected)
}

func (s *ActionService) WaiveGate(ctx context.Context, gateID string, actor contracts.Actor, reason string) (contracts.GateSnapshot, error) {
	return s.decideGate(ctx, gateID, actor, reason, contracts.GateStateWaived, contracts.EventGateWaived)
}

func (s *ActionService) decideGate(ctx context.Context, gateID string, actor contracts.Actor, reason string, nextState contracts.GateState, eventType contracts.EventType) (contracts.GateSnapshot, error) {
	return withWriteLock(ctx, s.LockManager, "decide gate", func(ctx context.Context) (contracts.GateSnapshot, error) {
		if !actor.IsValid() {
			return contracts.GateSnapshot{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		gate, err := s.Gates.LoadGate(ctx, gateID)
		if err != nil {
			return contracts.GateSnapshot{}, err
		}
		if gate.State != contracts.GateStateOpen {
			return contracts.GateSnapshot{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("gate %s is already %s", gate.GateID, gate.State))
		}
		ticket, err := s.Tickets.GetTicket(ctx, gate.TicketID)
		if err != nil {
			return contracts.GateSnapshot{}, err
		}
		var runContext *contracts.RunSnapshot
		if gate.RunID != "" {
			loadedRun, err := s.Runs.LoadRun(ctx, gate.RunID)
			if err == nil {
				runContext = &loadedRun
			}
		}
		relevantAgent := (*contracts.AgentProfile)(nil)
		if runContext != nil {
			loadedAgent, err := s.Agents.LoadAgent(ctx, runContext.AgentID)
			if err == nil {
				relevantAgent = &loadedAgent
			}
		}
		if _, err := s.requirePermission(ctx, permissionEvalInput{
			Action:            contracts.PermissionActionGateApprove,
			Actor:             actor,
			Ticket:            ticket,
			Run:               runContext,
			Gate:              &gate,
			ActorAgent:        relevantAgent,
			Runbook:           permissionRunbook(ticket, runContext),
			ChangedFilesKnown: false,
		}); err != nil {
			return contracts.GateSnapshot{}, err
		}
		if err := s.authorizeGateDecision(ctx, actor, ticket, gate); err != nil {
			return contracts.GateSnapshot{}, err
		}
		protectedAction := contracts.ProtectedAction("")
		switch eventType {
		case contracts.EventGateApproved:
			protectedAction = contracts.ProtectedActionGateApprove
		case contracts.EventGateWaived:
			protectedAction = contracts.ProtectedActionGateWaive
		}
		governanceInput := GovernanceEvaluationInput{
			Action:   protectedAction,
			Target:   "gate:" + gate.GateID,
			Actor:    actor,
			Reason:   reason,
			TicketID: ticket.ID,
			RunID:    gate.RunID,
			GateID:   gate.GateID,
		}
		governanceExplanation := contracts.GovernanceExplanation{}
		if governanceInput.Action != "" {
			governanceExplanation, err = s.requireGovernance(ctx, governanceInput)
			if err != nil {
				return contracts.GateSnapshot{}, err
			}
		}
		gate.State = nextState
		gate.DecidedBy = actor
		gate.DecisionReason = strings.TrimSpace(reason)
		gate.DecidedAt = s.now()
		ticket.OpenGateIDs = removeString(ticket.OpenGateIDs, gate.GateID)

		var run contracts.RunSnapshot
		hasRun := false
		if gate.RunID != "" {
			run, err = s.Runs.LoadRun(ctx, gate.RunID)
			if err != nil {
				return contracts.GateSnapshot{}, err
			}
			remaining, err := s.Gates.ListGates(ctx, gate.TicketID)
			if err != nil {
				return contracts.GateSnapshot{}, err
			}
			openForRun := 0
			for _, existing := range remaining {
				if existing.GateID == gate.GateID || existing.RunID != gate.RunID || existing.State != contracts.GateStateOpen {
					continue
				}
				openForRun++
			}
			if nextState == contracts.GateStateRejected {
				if run.Status == contracts.RunStatusAwaitingReview || run.Status == contracts.RunStatusAwaitingOwner || run.Status == contracts.RunStatusHandoffReady {
					run.Status = contracts.RunStatusActive
				}
			} else if openForRun == 0 {
				switch run.Status {
				case contracts.RunStatusAwaitingReview, contracts.RunStatusAwaitingOwner:
					run.Status = contracts.RunStatusHandoffReady
				}
			}
			hasRun = true
		}

		payload := runMutationPayload{Ticket: ticket, Gate: gate}
		if hasRun {
			payload.Run = run
		}
		event, err := s.newEvent(ctx, ticket.Project, s.now(), actor, reason, eventType, ticket.ID, payload)
		if err != nil {
			return contracts.GateSnapshot{}, err
		}
		mentions, err := s.extractMentions(ctx, event, "gate", gate.GateID, ticket.ID, gate.DecisionReason)
		if err != nil {
			return contracts.GateSnapshot{}, err
		}
		if err := s.commitMutation(ctx, "decide gate", "gate", event, func(ctx context.Context) error {
			if err := s.Gates.SaveGate(ctx, gate); err != nil {
				return err
			}
			if hasRun {
				if err := s.Runs.SaveRun(ctx, run); err != nil {
					return err
				}
			}
			if err := s.UpdateTicket(ctx, ticket); err != nil {
				return err
			}
			for _, mention := range mentions.Mentions {
				if err := s.Mentions.SaveMention(ctx, mention); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return contracts.GateSnapshot{}, err
		}
		if err := s.recordMentionEvents(ctx, ticket.Project, actor, reason, mentions.Mentions); err != nil {
			return contracts.GateSnapshot{}, err
		}
		if governanceInput.Action != "" {
			if err := s.recordGovernanceOverrideIfApplied(ctx, governanceInput, governanceExplanation); err != nil {
				return contracts.GateSnapshot{}, err
			}
		}
		return gate, nil
	})
}

func (s *ActionService) ensureGatesForHandoff(ctx context.Context, run contracts.RunSnapshot, ticket contracts.TicketSnapshot, actor contracts.Actor, nextActor string, nextGate contracts.GateKind) ([]contracts.GateSnapshot, contracts.TicketSnapshot, contracts.RunSnapshot, error) {
	kinds, stage, err := s.requiredGatesForRun(ctx, ticket, run)
	if err != nil {
		return nil, ticket, run, err
	}
	if nextGate != "" {
		kinds = appendGateKind(kinds, nextGate)
	}
	if len(kinds) == 0 {
		run.Status = contracts.RunStatusHandoffReady
		return nil, ticket, run, nil
	}
	existing, err := s.Gates.ListGates(ctx, ticket.ID)
	if err != nil {
		return nil, ticket, run, err
	}
	relevantAgent := (*contracts.AgentProfile)(nil)
	loadedAgent, agentErr := s.Agents.LoadAgent(ctx, run.AgentID)
	if agentErr == nil {
		relevantAgent = &loadedAgent
	}
	opened := make([]contracts.GateSnapshot, 0, len(kinds))
	for _, kind := range kinds {
		if _, err := s.requirePermission(ctx, permissionEvalInput{
			Action:            contracts.PermissionActionGateOpen,
			Actor:             actor,
			Ticket:            ticket,
			Run:               &run,
			ActorAgent:        relevantAgent,
			Runbook:           permissionRunbook(ticket, &run),
			ChangedFilesKnown: false,
		}); err != nil {
			return nil, ticket, run, err
		}
		gate, created, err := s.ensureGateOpenLocked(ticket, run, existing, stage, actor, kind, nextActor)
		if err != nil {
			return nil, ticket, run, err
		}
		if created {
			existing = append(existing, gate)
		}
		opened = append(opened, gate)
		ticket.OpenGateIDs = appendStringUnique(ticket.OpenGateIDs, gate.GateID)
	}
	if hasGateKind(opened, contracts.GateKindOwner) {
		run.Status = contracts.RunStatusAwaitingOwner
	} else if hasGateKind(opened, contracts.GateKindReview) {
		run.Status = contracts.RunStatusAwaitingReview
	} else {
		run.Status = contracts.RunStatusHandoffReady
	}
	return opened, ticket, run, nil
}

func (s *ActionService) ensureGateOpenLocked(ticket contracts.TicketSnapshot, run contracts.RunSnapshot, existing []contracts.GateSnapshot, stage contracts.RunbookStage, actor contracts.Actor, kind contracts.GateKind, nextActor string) (contracts.GateSnapshot, bool, error) {
	for _, gate := range existing {
		if gate.TicketID == ticket.ID && gate.RunID == run.RunID && gate.Kind == kind && gate.State == contracts.GateStateOpen {
			return gate, false, nil
		}
	}
	replacesGateID := ""
	for i := len(existing) - 1; i >= 0; i-- {
		gate := existing[i]
		if gate.TicketID == ticket.ID && gate.RunID == run.RunID && gate.Kind == kind && gate.State != contracts.GateStateOpen {
			replacesGateID = gate.GateID
			break
		}
	}
	requiredAgentID := strings.TrimSpace(nextActor)
	if requiredAgentID == "" {
		switch kind {
		case contracts.GateKindReview:
			requiredAgentID = strings.TrimSpace(string(ticket.Reviewer))
		case contracts.GateKindOwner, contracts.GateKindRelease:
			requiredAgentID = "human:owner"
		}
	}
	gate := contracts.GateSnapshot{
		GateID:               "gate_" + NewOpaqueID(),
		TicketID:             ticket.ID,
		RunID:                run.RunID,
		Kind:                 kind,
		State:                contracts.GateStateOpen,
		RequiredRole:         gateRequiredRole(kind),
		RequiredAgentID:      requiredAgentID,
		CreatedBy:            actor,
		EvidenceRequirements: append([]contracts.EvidenceType(nil), stage.RequiredEvidenceTypes...),
		RelatedRunIDs:        []string{run.RunID},
		ReplacesGateID:       replacesGateID,
		CreatedAt:            s.now(),
		SchemaVersion:        contracts.CurrentSchemaVersion,
	}
	return gate, true, gate.Validate()
}

func (s *ActionService) requiredGatesForRun(ctx context.Context, ticket contracts.TicketSnapshot, run contracts.RunSnapshot) ([]contracts.GateKind, contracts.RunbookStage, error) {
	runbookName := strings.TrimSpace(ticket.Runbook)
	if runbookName == "" {
		runbookName = defaultRunbookName(ticket.Type)
	}
	runbook, err := s.Runbooks.LoadRunbook(ctx, runbookName)
	if err != nil {
		return nil, contracts.RunbookStage{}, err
	}
	stageKey := strings.TrimSpace(run.BlueprintStage)
	if stageKey == "" {
		stageKey = runbook.DefaultInitialStage
	}
	for _, stage := range runbook.Stages {
		if stage.Key == stageKey {
			kinds := make([]contracts.GateKind, 0, len(stage.RequiredGates))
			for _, kind := range stage.RequiredGates {
				kinds = appendGateKind(kinds, kind)
			}
			return kinds, stage, nil
		}
	}
	return nil, contracts.RunbookStage{}, nil
}

func gateRequiredRole(kind contracts.GateKind) contracts.AgentRole {
	switch kind {
	case contracts.GateKindReview, contracts.GateKindDesign:
		return contracts.AgentRoleReviewer
	case contracts.GateKindOwner, contracts.GateKindRelease:
		return contracts.AgentRoleOwnerDelegate
	case contracts.GateKindQA:
		return contracts.AgentRoleQA
	default:
		return ""
	}
}

func appendGateKind(values []contracts.GateKind, kind contracts.GateKind) []contracts.GateKind {
	if !kind.IsValid() {
		return values
	}
	for _, value := range values {
		if value == kind {
			return values
		}
	}
	return append(values, kind)
}

func hasGateKind(gates []contracts.GateSnapshot, kind contracts.GateKind) bool {
	for _, gate := range gates {
		if gate.Kind == kind {
			return true
		}
	}
	return false
}

func appendStringUnique(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func removeString(values []string, target string) []string {
	if len(values) == 0 {
		return values
	}
	items := make([]string, 0, len(values))
	for _, value := range values {
		if value == target {
			continue
		}
		items = append(items, value)
	}
	return items
}
