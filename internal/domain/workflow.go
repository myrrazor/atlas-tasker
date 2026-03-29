package domain

import (
	"fmt"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

var allowedTransitions = map[contracts.Status]map[contracts.Status]struct{}{
	contracts.StatusBacklog: {
		contracts.StatusReady: {}, contracts.StatusCanceled: {},
	},
	contracts.StatusReady: {
		contracts.StatusInProgress: {}, contracts.StatusBlocked: {}, contracts.StatusCanceled: {},
	},
	contracts.StatusInProgress: {
		contracts.StatusInReview: {}, contracts.StatusBlocked: {}, contracts.StatusReady: {}, contracts.StatusCanceled: {},
	},
	contracts.StatusBlocked: {
		contracts.StatusReady: {}, contracts.StatusInProgress: {}, contracts.StatusCanceled: {},
	},
	contracts.StatusInReview: {
		contracts.StatusDone: {}, contracts.StatusInProgress: {}, contracts.StatusBlocked: {},
	},
	contracts.StatusDone:     {},
	contracts.StatusCanceled: {},
}

// CanTransition returns true when the status edge is allowed.
func CanTransition(from contracts.Status, to contracts.Status) bool {
	next, ok := allowedTransitions[from]
	if !ok {
		return false
	}
	_, ok = next[to]
	return ok
}

// ValidateTransition checks status validity and transition edge constraints.
func ValidateTransition(from contracts.Status, to contracts.Status) error {
	if !from.IsValid() {
		return fmt.Errorf("invalid source status: %s", from)
	}
	if !to.IsValid() {
		return fmt.Errorf("invalid target status: %s", to)
	}
	if !CanTransition(from, to) {
		return fmt.Errorf("forbidden transition: %s -> %s", from, to)
	}
	return nil
}

// CheckCompletionPermission enforces completion_mode when moving in_review to done.
func CheckCompletionPermission(mode contracts.CompletionMode, actor contracts.Actor, reviewer contracts.Actor) error {
	if !mode.IsValid() {
		return fmt.Errorf("invalid completion mode: %s", mode)
	}
	if !actor.IsValid() {
		return fmt.Errorf("invalid actor: %s", actor)
	}

	switch mode {
	case contracts.CompletionModeOpen:
		return nil
	case contracts.CompletionModeOwnerGate:
		if actor != contracts.Actor("human:owner") {
			return fmt.Errorf("only human:owner can complete in owner_gate mode")
		}
		return nil
	case contracts.CompletionModeReviewGate:
		if actor == contracts.Actor("human:owner") {
			return nil
		}
		if reviewer != "" && actor == reviewer {
			return nil
		}
		return fmt.Errorf("only reviewer or human:owner can complete in review_gate mode")
	case contracts.CompletionModeDualGate:
		if actor != contracts.Actor("human:owner") {
			return fmt.Errorf("only human:owner can complete in dual_gate mode")
		}
		return nil
	default:
		return fmt.Errorf("invalid completion mode: %s", mode)
	}
}

// ValidateMove checks transition and completion permission requirements.
func ValidateMove(mode contracts.CompletionMode, from contracts.Status, to contracts.Status, actor contracts.Actor, reviewer contracts.Actor) error {
	if err := ValidateTransition(from, to); err != nil {
		return err
	}
	if from == contracts.StatusInReview && to == contracts.StatusDone {
		if err := CheckCompletionPermission(mode, actor, reviewer); err != nil {
			return err
		}
	}
	return nil
}
