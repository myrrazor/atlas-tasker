package contracts

import (
	"testing"
	"time"
)

func TestV17GovernanceContractsValidate(t *testing.T) {
	now := time.Now().UTC()
	pack := PolicyPack{
		PackID: "pack_security",
		Name:   "Security gates",
		Policies: []GovernancePolicy{{
			PolicyID:         "policy_restricted_merge",
			Name:             "Restricted merge policy",
			ScopeKind:        PolicyScopeProject,
			ScopeID:          "APP",
			ProtectedActions: []ProtectedAction{ProtectedActionChangeMerge},
			QuorumRules: []QuorumRule{{
				RuleID:                       "two_reviewers",
				ActionKind:                   ProtectedActionChangeMerge,
				RequiredCount:                2,
				AllowedRoles:                 []MembershipRole{MembershipRoleReviewer, MembershipRoleMaintainer},
				RequireDistinctCollaborators: true,
				RequireTrustedSignatures:     true,
			}},
			SeparationOfDutiesRules: []SeparationOfDutiesRule{{
				RuleID:                      "implementer_cannot_merge",
				ActionKind:                  ProtectedActionChangeMerge,
				ForbiddenActorRelationships: []string{"implemented"},
				LookbackEventTypes:          []EventType{EventRunCompleted},
				LookbackScope:               "ticket",
			}},
			OverrideRules: []OverrideRule{{
				RuleID:                  "owner_override_signed",
				ActionKind:              ProtectedActionOwnerOverride,
				Allowed:                 true,
				RequireReason:           true,
				RequireTrustedSignature: true,
			}},
			CreatedAt:     now,
			UpdatedAt:     now,
			SchemaVersion: CurrentSchemaVersion,
		}},
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: CurrentSchemaVersion,
	}
	if err := pack.Validate(); err != nil {
		t.Fatalf("valid governance pack rejected: %v", err)
	}
	pack.Policies[0].QuorumRules[0].RequiredCount = 0
	if err := pack.Validate(); err == nil {
		t.Fatalf("invalid quorum count should fail")
	}
	pack.Policies[0].QuorumRules[0].RequiredCount = 2
	pack.Policies[0].RequiredSignatures = -1
	if err := pack.Validate(); err == nil {
		t.Fatalf("negative required signatures should fail")
	}
	pack.Policies[0].RequiredSignatures = 0
	pack.Policies[0].ScopeID = ""
	if err := pack.Validate(); err == nil {
		t.Fatalf("project policy without scope id should fail")
	}
	pack.Policies[0].ScopeKind = PolicyScopeWorkspace
	pack.Policies[0].ScopeID = "APP"
	if err := pack.Validate(); err == nil {
		t.Fatalf("workspace policy with scope id should fail")
	}
	pack.Policies[0].ScopeID = ""
	if err := pack.Validate(); err != nil {
		t.Fatalf("valid workspace policy rejected: %v", err)
	}
}

func TestV17ClassificationAndRedactionContractsValidate(t *testing.T) {
	if HigherClassification(ClassificationInternal, ClassificationRestricted) != ClassificationRestricted {
		t.Fatalf("higher sensitivity should win")
	}
	if HigherClassification(ClassificationLevel("bogus"), ClassificationPublic) != ClassificationRestricted {
		t.Fatalf("unknown classification should fail closed to restricted")
	}
	now := time.Now().UTC()
	label := ClassificationLabel{
		ClassificationID: "class_APP",
		EntityKind:       ClassifiedEntityProject,
		EntityID:         "APP",
		Level:            ClassificationInternal,
		AppliedBy:        Actor("human:owner"),
		Reason:           "workspace default",
		CreatedAt:        now,
		UpdatedAt:        now,
		SchemaVersion:    CurrentSchemaVersion,
	}
	if err := label.Validate(); err != nil {
		t.Fatalf("valid classification label rejected: %v", err)
	}
	rule := RedactionRule{
		RuleID:    "rule_1",
		Target:    RedactionTargetGoal,
		FieldPath: "body",
		MinLevel:  ClassificationConfidential,
		Action:    RedactionReplaceWithMarker,
		Reason:    "agent-safe output",
	}
	if err := rule.Validate(); err == nil {
		t.Fatalf("marker redaction without marker should fail")
	}
	rule.Marker = "[redacted]"
	if err := rule.Validate(); err != nil {
		t.Fatalf("valid marker redaction rule rejected: %v", err)
	}
	preview := RedactionPreview{
		PreviewID:          "redact_1",
		Scope:              "project:APP",
		Target:             RedactionTargetExport,
		Actor:              Actor("human:owner"),
		PolicyVersionHash:  "policyhash",
		ClassificationHash: "classhash",
		SourceStateHash:    "sourcehash",
		CommandTarget:      "export",
		CreatedAt:          now,
		ExpiresAt:          now.Add(10 * time.Minute),
		Items: []RedactionResult{{
			EntityKind: ClassifiedEntityEvidence,
			EntityID:   "ev_1",
			FieldPath:  "body",
			Level:      ClassificationRestricted,
			Action:     RedactionOmit,
		}},
		SchemaVersion: CurrentSchemaVersion,
	}
	if err := preview.Validate(); err != nil {
		t.Fatalf("valid redaction preview rejected: %v", err)
	}
	preview.CreatedAt = time.Time{}
	if err := preview.Validate(); err == nil {
		t.Fatalf("preview without created_at should fail")
	}
	preview.CreatedAt = now
	preview.ExpiresAt = now
	if err := preview.Validate(); err == nil {
		t.Fatalf("preview without a positive ttl should fail")
	}
	preview.ExpiresAt = now.Add(10 * time.Minute)
	preview.CommandTarget = ""
	if err := preview.Validate(); err == nil {
		t.Fatalf("preview without command target should fail")
	}
}
