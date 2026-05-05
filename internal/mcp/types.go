package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

const FormatVersion = "v1"

type ToolProfile string

const (
	ProfileRead     ToolProfile = "read"
	ProfileWorkflow ToolProfile = "workflow"
	ProfileDelivery ToolProfile = "delivery"
	ProfileAdmin    ToolProfile = "admin"
)

var toolProfileOrder = map[ToolProfile]int{
	ProfileRead:     0,
	ProfileWorkflow: 1,
	ProfileDelivery: 2,
	ProfileAdmin:    3,
}

func ParseToolProfile(raw string) (ToolProfile, error) {
	profile := ToolProfile(strings.ToLower(strings.TrimSpace(raw)))
	if profile == "" {
		return ProfileRead, nil
	}
	if _, ok := toolProfileOrder[profile]; !ok {
		return "", fmt.Errorf("invalid MCP tool profile %q", raw)
	}
	return profile, nil
}

func ToolProfiles() []ToolProfile {
	return []ToolProfile{ProfileRead, ProfileWorkflow, ProfileDelivery, ProfileAdmin}
}

type ToolClass string

const (
	ClassRead       ToolClass = "read"
	ClassWorkflow   ToolClass = "workflow"
	ClassDelivery   ToolClass = "delivery"
	ClassAdmin      ToolClass = "admin"
	ClassHighImpact ToolClass = "high_impact"
)

type ApprovalMechanism string

const (
	ApprovalNone      ApprovalMechanism = "none"
	ApprovalOperation ApprovalMechanism = "operation_approval"
)

type Options struct {
	Profile               ToolProfile
	ReadOnly              bool
	AllowHighImpactTools  bool
	MaxResultBytes        int
	MaxItems              int
	MaxTextTokensEstimate int
	IncludeLocalOnlyPaths bool
	Now                   func() time.Time
}

func (o Options) Normalized() Options {
	profile := o.Profile
	if profile == "" {
		profile = ProfileRead
	}
	if o.ReadOnly {
		profile = ProfileRead
	}
	if o.MaxResultBytes <= 0 {
		o.MaxResultBytes = 128 * 1024
	}
	if o.MaxItems <= 0 {
		o.MaxItems = 50
	}
	if o.MaxTextTokensEstimate <= 0 {
		o.MaxTextTokensEstimate = 4000
	}
	if o.Now == nil {
		o.Now = func() time.Time { return time.Now().UTC() }
	}
	o.Profile = profile
	return o
}

type ToolSpec struct {
	Name               string            `json:"name"`
	Title              string            `json:"title,omitempty"`
	Description        string            `json:"description"`
	Class              ToolClass         `json:"class"`
	Profiles           []ToolProfile     `json:"profiles"`
	RequiresActor      bool              `json:"requires_actor"`
	RequiresReason     bool              `json:"requires_reason"`
	RequiresApproval   bool              `json:"requires_approval"`
	ApprovalMechanism  ApprovalMechanism `json:"approval_mechanism"`
	ProviderSideEffect bool              `json:"provider_live_side_effect"`
	HighImpact         bool              `json:"high_impact"`
	Destructive        bool              `json:"destructive"`
	TargetArg          string            `json:"target_arg,omitempty"`
	Underlying         string            `json:"underlying_atlas_action"`
	InputSchema        map[string]any    `json:"input_schema"`
	Handler            ToolHandler       `json:"-"`
}

type ToolHandler func(ctx ToolContext, args map[string]any) (any, error)

type ToolContext struct {
	Context context.Context
	Server  *Server
	Spec    ToolSpec
	Actor   string
	Reason  string
	Target  string
}

type ToolInfo struct {
	Name               string            `json:"name"`
	Class              ToolClass         `json:"class"`
	Enabled            bool              `json:"enabled"`
	DisabledReason     string            `json:"disabled_reason,omitempty"`
	Profiles           []ToolProfile     `json:"profiles"`
	RequiresActor      bool              `json:"requires_actor"`
	RequiresReason     bool              `json:"requires_reason"`
	HighImpact         bool              `json:"high_impact"`
	RequiresApproval   bool              `json:"requires_approval_token_or_gate"`
	ApprovalMechanism  ApprovalMechanism `json:"approval_mechanism"`
	ProviderSideEffect bool              `json:"provider_live_side_effect"`
	Underlying         string            `json:"underlying_atlas_action"`
	SchemaHash         string            `json:"json_schema_hash"`
}

func (s ToolSpec) Enabled(opts Options) (bool, string) {
	opts = opts.Normalized()
	if opts.ReadOnly && s.Class != ClassRead {
		return false, "read_only_mode"
	}
	if !slices.Contains(s.Profiles, opts.Profile) {
		return false, "profile_not_selected"
	}
	if s.HighImpact && !opts.AllowHighImpactTools {
		return false, "high_impact_tools_disabled"
	}
	return true, ""
}

func (s ToolSpec) Info(opts Options) ToolInfo {
	enabled, reason := s.Enabled(opts)
	return ToolInfo{
		Name:               s.Name,
		Class:              s.Class,
		Enabled:            enabled,
		DisabledReason:     reason,
		Profiles:           append([]ToolProfile(nil), s.Profiles...),
		RequiresActor:      s.RequiresActor,
		RequiresReason:     s.RequiresReason,
		HighImpact:         s.HighImpact,
		RequiresApproval:   s.RequiresApproval,
		ApprovalMechanism:  s.ApprovalMechanism,
		ProviderSideEffect: s.ProviderSideEffect,
		Underlying:         s.Underlying,
		SchemaHash:         schemaHash(s.InputSchema),
	}
}

func schemaHash(schema map[string]any) string {
	raw, err := json.Marshal(schema)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func toolResult(kind string, generatedAt time.Time, payload any) map[string]any {
	return map[string]any{
		"format_version": FormatVersion,
		"kind":           kind,
		"generated_at":   generatedAt.UTC(),
		"payload":        payload,
	}
}
