package service

import (
	"context"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestV17GoalManifestSignatureDetectsSourceTamper(t *testing.T) {
	ctx, actions, ticket := newGovernanceHarness(t)
	key, err := actions.GenerateKey(ctx, KeyGenerateOptions{Scope: contracts.KeyScopeCollaborator, OwnerID: "owner"}, contracts.Actor("human:owner"), "goal signer")
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if _, err := actions.BindTrust(ctx, "owner", key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "trust goal signer"); err != nil {
		t.Fatalf("bind trust: %v", err)
	}
	manifest, err := actions.CreateGoalManifest(ctx, ticket.ID, contracts.Actor("human:owner"), "create goal manifest")
	if err != nil {
		t.Fatalf("goal manifest: %v", err)
	}
	if _, err := actions.SignGoalManifest(ctx, manifest.Manifest.ManifestID, key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "sign goal"); err != nil {
		t.Fatalf("sign goal: %v", err)
	}
	stored, err := actions.GoalManifests.LoadGoalManifest(context.Background(), manifest.Manifest.ManifestID)
	if err != nil {
		t.Fatalf("load signed goal: %v", err)
	}
	stored.SourceHash = "tampered-source-hash"
	if err := actions.GoalManifests.SaveGoalManifest(context.Background(), stored); err != nil {
		t.Fatalf("save tampered goal: %v", err)
	}
	verified, err := actions.VerifyGoalManifest(context.Background(), manifest.Manifest.ManifestID)
	if err != nil {
		t.Fatalf("verify tampered goal: %v", err)
	}
	if verified.Signature.State == contracts.VerificationTrustedValid {
		t.Fatalf("tampered goal manifest should not verify trusted: %#v", verified.Signature)
	}
	if verified.Signature.State != contracts.VerificationPayloadHashMismatch {
		t.Fatalf("expected payload hash mismatch, got %#v", verified.Signature)
	}
}
