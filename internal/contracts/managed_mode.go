package contracts

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ManagedModeFormat is the versioned format tag of the workspace-shared
// managed project-mode policy (plan AT114-004). The policy is
// repository-carried: it lives at .tracker/managed-mode.json, contains no
// machine paths, credentials, or actor identities, and is read by the
// atlas-worker skill and the MCP context/status tools.
const ManagedModeFormat = "atlas_managed_mode_v1"

// ManagedModeFileName is the file name under the .tracker directory.
const ManagedModeFileName = "managed-mode.json"

// ManagedMode selects how much of Atlas an agent is expected to use.
type ManagedMode string

const (
	// ManagedModeGuidance installs skills and instructions only; the agent
	// drives Atlas through the CLI and no MCP server is registered by setup.
	ManagedModeGuidance ManagedMode = "guidance"
	// ManagedModeManaged is the recommended default: skills, MCP, status, and
	// the ordinary ticket workflow.
	ManagedModeManaged ManagedMode = "managed"
	// ManagedModeDelivery additionally allows separately enabled advanced
	// operations. Setup never selects it; it requires an explicit power-user
	// action and a separately registered delivery-profile server.
	ManagedModeDelivery ManagedMode = "delivery"
	// ManagedModeDisabled turns automatic tracking off. Skills may remain
	// installed; agents create, claim, and move nothing on their own.
	ManagedModeDisabled ManagedMode = "disabled"
)

var validManagedModes = map[ManagedMode]struct{}{
	ManagedModeGuidance: {}, ManagedModeManaged: {}, ManagedModeDelivery: {}, ManagedModeDisabled: {},
}

func (m ManagedMode) IsValid() bool {
	_, ok := validManagedModes[m]
	return ok
}

// UsesMCP reports whether setup registers the Atlas MCP server for this mode.
func (m ManagedMode) UsesMCP() bool {
	return m == ManagedModeManaged || m == ManagedModeDelivery
}

// AllowsDelivery reports whether advanced (delivery-profile) operations are
// part of the agent's expected behavior. Only the delivery mode says yes, and
// selecting it is never a setup default.
func (m ManagedMode) AllowsDelivery() bool { return m == ManagedModeDelivery }

// TracksWork reports whether agents are expected to track work at all.
func (m ManagedMode) TracksWork() bool { return m.IsValid() && m != ManagedModeDisabled }

// RequiresExplicitEnable is true for modes tracker setup must never choose on
// the user's behalf.
func (m ManagedMode) RequiresExplicitEnable() bool { return m == ManagedModeDelivery }

// CapturePolicy says when an agent may create or attach a ticket.
type CapturePolicy string

const (
	CapturePolicyNever        CapturePolicy = "never"
	CapturePolicyAsk          CapturePolicy = "ask"
	CapturePolicyMaterialWork CapturePolicy = "material_work"
)

var validCapturePolicies = map[CapturePolicy]struct{}{
	CapturePolicyNever: {}, CapturePolicyAsk: {}, CapturePolicyMaterialWork: {},
}

func (p CapturePolicy) IsValid() bool {
	_, ok := validCapturePolicies[p]
	return ok
}

// ProgressPolicy says how often an agent records progress on a ticket.
type ProgressPolicy string

const (
	ProgressPolicyNone            ProgressPolicy = "none"
	ProgressPolicyMilestones      ProgressPolicy = "milestones"
	ProgressPolicyEveryCheckpoint ProgressPolicy = "every_checkpoint"
)

var validProgressPolicies = map[ProgressPolicy]struct{}{
	ProgressPolicyNone: {}, ProgressPolicyMilestones: {}, ProgressPolicyEveryCheckpoint: {},
}

func (p ProgressPolicy) IsValid() bool {
	_, ok := validProgressPolicies[p]
	return ok
}

// StatusPolicy says where an agent's status answers come from.
type StatusPolicy string

// StatusPolicyAtlasRequired is the only v1 value: every status response is
// preceded by a fresh Atlas read (atlas.status or the CLI); an agent never
// answers a status question from memory, and reports a failed read instead of
// guessing. The plan spells the same requirement "query_atlas" in one place;
// that spelling is rejected so the file has one vocabulary.
const StatusPolicyAtlasRequired StatusPolicy = "atlas_required"

func (p StatusPolicy) IsValid() bool { return p == StatusPolicyAtlasRequired }

// ManagedCompletionPolicy says how an agent completes work.
type ManagedCompletionPolicy string

// ManagedCompletionFollowWorkspace is the only v1 value: completion, review,
// and approval follow the workspace and project completion mode exactly. The
// managed-mode policy has no field that can relax them.
const ManagedCompletionFollowWorkspace ManagedCompletionPolicy = "follow_workspace"

func (p ManagedCompletionPolicy) IsValid() bool { return p == ManagedCompletionFollowWorkspace }

// ManagedModePolicy is the persisted atlas_managed_mode_v1 document.
type ManagedModePolicy struct {
	Format           string                  `json:"format"`
	Mode             ManagedMode             `json:"mode"`
	CapturePolicy    CapturePolicy           `json:"capture_policy"`
	ProgressPolicy   ProgressPolicy          `json:"progress_policy"`
	StatusPolicy     StatusPolicy            `json:"status_policy"`
	CompletionPolicy ManagedCompletionPolicy `json:"completion_policy"`
	MCPPreferred     bool                    `json:"mcp_preferred"`
}

// DefaultManagedModePolicy is the recommended policy tracker setup proposes.
func DefaultManagedModePolicy() ManagedModePolicy {
	return ManagedModePolicyForMode(ManagedModeManaged)
}

// ManagedModePolicyForMode returns the recommended policy for one mode. The
// returned value always validates.
func ManagedModePolicyForMode(mode ManagedMode) ManagedModePolicy {
	policy := ManagedModePolicy{
		Format:           ManagedModeFormat,
		Mode:             mode,
		CapturePolicy:    CapturePolicyMaterialWork,
		ProgressPolicy:   ProgressPolicyMilestones,
		StatusPolicy:     StatusPolicyAtlasRequired,
		CompletionPolicy: ManagedCompletionFollowWorkspace,
		MCPPreferred:     mode.UsesMCP(),
	}
	if mode == ManagedModeDisabled {
		policy.CapturePolicy = CapturePolicyNever
		policy.ProgressPolicy = ProgressPolicyNone
	}
	return policy
}

// Validate enforces the vocabulary and the cross-field rules:
//   - disabled mode cannot capture tickets or record progress;
//   - guidance mode is skills-only, so it cannot prefer MCP.
//
// Nothing here can waive dependencies, governance, review, or approval; those
// stay with the existing workspace policies (see EffectiveCompletionMode).
func (p ManagedModePolicy) Validate() error {
	if p.Format != ManagedModeFormat {
		return fmt.Errorf("managed mode policy format must be %q, got %q", ManagedModeFormat, p.Format)
	}
	if !p.Mode.IsValid() {
		return fmt.Errorf("invalid managed mode %q", p.Mode)
	}
	if !p.CapturePolicy.IsValid() {
		return fmt.Errorf("invalid capture policy %q", p.CapturePolicy)
	}
	if !p.ProgressPolicy.IsValid() {
		return fmt.Errorf("invalid progress policy %q", p.ProgressPolicy)
	}
	if !p.StatusPolicy.IsValid() {
		return fmt.Errorf("invalid status policy %q (only %q is supported)", p.StatusPolicy, StatusPolicyAtlasRequired)
	}
	if !p.CompletionPolicy.IsValid() {
		return fmt.Errorf("invalid completion policy %q (only %q is supported)", p.CompletionPolicy, ManagedCompletionFollowWorkspace)
	}
	if p.Mode == ManagedModeDisabled {
		if p.CapturePolicy != CapturePolicyNever {
			return fmt.Errorf("disabled mode requires capture policy %q", CapturePolicyNever)
		}
		if p.ProgressPolicy != ProgressPolicyNone {
			return fmt.Errorf("disabled mode requires progress policy %q", ProgressPolicyNone)
		}
	}
	if p.Mode == ManagedModeGuidance && p.MCPPreferred {
		return fmt.Errorf("guidance mode is skills-only and cannot prefer MCP")
	}
	return nil
}

// Encode renders the canonical on-disk form: two-space indented JSON with a
// trailing newline, fields in declaration order.
func (p ManagedModePolicy) Encode() ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// ParseManagedModePolicy decodes and validates one policy document. Unknown
// fields are an error so that no waiver-style field can be smuggled in.
func ParseManagedModePolicy(data []byte) (ManagedModePolicy, error) {
	var policy ManagedModePolicy
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&policy); err != nil {
		return ManagedModePolicy{}, fmt.Errorf("parse managed mode policy: %w", err)
	}
	if decoder.More() {
		return ManagedModePolicy{}, fmt.Errorf("parse managed mode policy: trailing content after document")
	}
	if err := policy.Validate(); err != nil {
		return ManagedModePolicy{}, err
	}
	return policy, nil
}

// EffectiveCompletionMode returns the completion mode an agent must follow.
// The managed-mode policy never overrides the workspace or project mode; an
// invalid workspace mode is an error rather than a fallback to open.
func (p ManagedModePolicy) EffectiveCompletionMode(workspace CompletionMode) (CompletionMode, error) {
	if !workspace.IsValid() {
		return "", fmt.Errorf("invalid workspace completion mode %q", workspace)
	}
	return workspace, nil
}

// WorkIntent classifies what a user asked an agent to do. Only material work
// can ever produce a ticket.
type WorkIntent string

const (
	// WorkIntentMaterialWork is work expected to modify the repository, create
	// an implementation artifact, run a substantive review, or produce a
	// durable project deliverable.
	WorkIntentMaterialWork WorkIntent = "material_work"
	// WorkIntentStatusQuery is "what is the status", "what is blocked", "show
	// me the board".
	WorkIntentStatusQuery WorkIntent = "status_query"
	// WorkIntentReadOnlyExplanation is explaining code or state without
	// changing anything.
	WorkIntentReadOnlyExplanation WorkIntent = "read_only_explanation"
	// WorkIntentCasualDiscussion is conversation with no project change.
	WorkIntentCasualDiscussion WorkIntent = "casual_discussion"
	// WorkIntentSimpleQuestion is a question whose answer changes nothing.
	WorkIntentSimpleQuestion WorkIntent = "simple_question"
	// WorkIntentTrackingExcluded is any work the user explicitly asked not to
	// track. It wins over every policy.
	WorkIntentTrackingExcluded WorkIntent = "tracking_excluded"
)

var validWorkIntents = map[WorkIntent]struct{}{
	WorkIntentMaterialWork: {}, WorkIntentStatusQuery: {}, WorkIntentReadOnlyExplanation: {},
	WorkIntentCasualDiscussion: {}, WorkIntentSimpleQuestion: {}, WorkIntentTrackingExcluded: {},
}

func (i WorkIntent) IsValid() bool {
	_, ok := validWorkIntents[i]
	return ok
}

// Trackable reports whether this intent may ever be attached to a ticket.
func (i WorkIntent) Trackable() bool { return i == WorkIntentMaterialWork }

// CaptureDecision is what the skill does about a ticket before starting work.
type CaptureDecision string

const (
	// CaptureNoTicket means do not create or attach a ticket.
	CaptureNoTicket CaptureDecision = "no_ticket"
	// CaptureAskUser means ask before creating; an existing ticket named by
	// the user may still be used.
	CaptureAskUser CaptureDecision = "ask_user"
	// CaptureAttachOrCreate means search for an existing ticket first and
	// create one only when none matches and policy allows it.
	CaptureAttachOrCreate CaptureDecision = "attach_or_create"
)

// CaptureDecision applies the policy to a classified intent. Non-material
// intents, explicit tracking exclusion, and disabled mode always yield
// no_ticket regardless of the capture policy.
func (p ManagedModePolicy) CaptureDecision(intent WorkIntent) (CaptureDecision, error) {
	if !intent.IsValid() {
		return "", fmt.Errorf("invalid work intent %q", intent)
	}
	if err := p.Validate(); err != nil {
		return "", err
	}
	if !intent.Trackable() || !p.Mode.TracksWork() {
		return CaptureNoTicket, nil
	}
	switch p.CapturePolicy {
	case CapturePolicyAsk:
		return CaptureAskUser, nil
	case CapturePolicyMaterialWork:
		return CaptureAttachOrCreate, nil
	default:
		return CaptureNoTicket, nil
	}
}

// ProgressEvent is a moment at which an agent could record progress.
type ProgressEvent string

const (
	// ProgressEventMilestone is a meaningful step: tests passing, a review
	// requested, a deliverable produced.
	ProgressEventMilestone ProgressEvent = "milestone"
	// ProgressEventCheckpoint is any smaller unit of work the agent finished.
	ProgressEventCheckpoint ProgressEvent = "checkpoint"
)

func (e ProgressEvent) IsValid() bool {
	return e == ProgressEventMilestone || e == ProgressEventCheckpoint
}

// RecordsProgress reports whether the policy asks for a progress comment at
// this event.
func (p ManagedModePolicy) RecordsProgress(event ProgressEvent) (bool, error) {
	if !event.IsValid() {
		return false, fmt.Errorf("invalid progress event %q", event)
	}
	if err := p.Validate(); err != nil {
		return false, err
	}
	if !p.Mode.TracksWork() {
		return false, nil
	}
	switch p.ProgressPolicy {
	case ProgressPolicyEveryCheckpoint:
		return true, nil
	case ProgressPolicyMilestones:
		return event == ProgressEventMilestone, nil
	default:
		return false, nil
	}
}
