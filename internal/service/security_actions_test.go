package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	eventstore "github.com/myrrazor/atlas-tasker/internal/storage/events"
)

func newSecurityTestActions(t *testing.T) (context.Context, string, *ActionService) {
	t.Helper()
	root := t.TempDir()
	clock := time.Date(2026, 5, 6, 14, 0, 0, 0, time.UTC)
	actions := NewActionService(
		root,
		nil,
		nil,
		&eventstore.Log{RootDir: root},
		nil,
		func() time.Time { return clock },
		FileLockManager{Root: root},
		nil,
		nil,
	)
	return context.Background(), root, actions
}

func TestSecurityKeysGenerateTrustAndStayLocal(t *testing.T) {
	ctx, root, actions := newSecurityTestActions(t)
	key, err := actions.GenerateKey(ctx, KeyGenerateOptions{Scope: contracts.KeyScopeCollaborator, OwnerID: "alice"}, contracts.Actor("human:owner"), "create alice key")
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if key.PublicKey.OwnerKind != contracts.PublicKeyOwnerCollaborator || key.PublicKey.OwnerID != "alice" {
		t.Fatalf("unexpected key owner: %#v", key.PublicKey)
	}
	adminKey, err := actions.GenerateKey(ctx, KeyGenerateOptions{Scope: contracts.KeyScopeAdmin}, contracts.Actor("human:owner"), "create admin key")
	if err != nil {
		t.Fatalf("generate admin key: %v", err)
	}
	if adminKey.PublicKey.OwnerKind != contracts.PublicKeyOwnerAdmin || adminKey.PublicKey.OwnerID != "admin" {
		t.Fatalf("admin scope should preserve admin owner boundary: %#v", adminKey.PublicKey)
	}
	if !key.CanSign || !key.PrivateKeyHealth.Present || !key.PrivateKeyHealth.PermissionsOK {
		t.Fatalf("generated key should be signable: %#v", key.PrivateKeyHealth)
	}
	if key.PublicKey.PublicKeyMaterial == "" || strings.Contains(key.PublicKey.PublicKeyMaterial, "PRIVATE") {
		t.Fatalf("public key output looks wrong: %#v", key.PublicKey)
	}
	if got := strings.TrimPrefix(key.PublicKey.PublicKeyID, "key-"); len(got) != 32 {
		t.Fatalf("public key ids should use 128-bit fingerprint prefixes, got %s", key.PublicKey.PublicKeyID)
	}
	privateInfo, err := os.Stat(storage.PrivateKeyFile(root, key.PublicKey.PublicKeyID))
	if err != nil {
		t.Fatalf("stat private key: %v", err)
	}
	if runtime.GOOS != "windows" && privateInfo.Mode().Perm() != 0o600 {
		t.Fatalf("private key mode = %04o, want 0600", privateInfo.Mode().Perm())
	}

	binding, err := actions.BindTrust(ctx, "alice", key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "trust alice key")
	if err != nil {
		t.Fatalf("bind trust: %v", err)
	}
	if _, err := actions.BindTrust(ctx, "bob", key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "wrong collaborator"); err == nil || !strings.Contains(err.Error(), "owner does not match") {
		t.Fatalf("trust binding should reject mismatched collaborator owner, got %v", err)
	}
	if !binding.LocalOnly || binding.TrustLevel != contracts.TrustLevelTrusted {
		t.Fatalf("trust binding should be local trusted material: %#v", binding)
	}
	status, err := actions.TrustStatus(ctx)
	if err != nil {
		t.Fatalf("trust status: %v", err)
	}
	if status.PublicKeys != 2 || status.LocalPrivateKeys != 2 || status.TrustedBindings != 1 || status.ImportedUntrusted != 0 {
		t.Fatalf("unexpected trust status: %#v", status)
	}
	explain, err := actions.ExplainTrust(ctx, key.PublicKey.PublicKeyID)
	if err != nil {
		t.Fatalf("explain trust: %v", err)
	}
	if len(explain.TrustedScopes) == 0 {
		t.Fatalf("expected trusted scopes in explanation: %#v", explain)
	}
	if _, err := actions.RevokeTrustForKey(ctx, key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "local distrust ceremony"); err != nil {
		t.Fatalf("revoke trust: %v", err)
	}
	events, err := actions.Events.StreamEvents(ctx, workspaceProjectKey, 0)
	if err != nil {
		t.Fatalf("stream events: %v", err)
	}
	for _, event := range events {
		if event.Type == contracts.EventTrustBound || event.Type == contracts.EventTrustRevoked {
			t.Fatalf("local trust ceremonies must not enter syncable event history: %#v", event)
		}
	}
}

func TestBindTrustRejectsInactiveOrExpiredKeys(t *testing.T) {
	ctx, _, actions := newSecurityTestActions(t)
	rotated, err := actions.GenerateKey(ctx, KeyGenerateOptions{Scope: contracts.KeyScopeCollaborator, OwnerID: "rotated"}, contracts.Actor("human:owner"), "create rotated key")
	if err != nil {
		t.Fatalf("generate rotated key: %v", err)
	}
	if _, err := actions.RotateKey(ctx, rotated.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "rotate before trust"); err != nil {
		t.Fatalf("rotate key: %v", err)
	}
	if _, err := actions.BindTrust(ctx, "rotated", rotated.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "trust rotated key"); err == nil || !strings.Contains(err.Error(), "state rotated") {
		t.Fatalf("rotated key should not be trust-bound, got %v", err)
	}

	revoked, err := actions.GenerateKey(ctx, KeyGenerateOptions{Scope: contracts.KeyScopeCollaborator, OwnerID: "revoked"}, contracts.Actor("human:owner"), "create revoked key")
	if err != nil {
		t.Fatalf("generate revoked key: %v", err)
	}
	if _, err := actions.RevokeKey(ctx, revoked.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "revoke before trust"); err != nil {
		t.Fatalf("revoke key: %v", err)
	}
	if _, err := actions.BindTrust(ctx, "revoked", revoked.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "trust revoked key"); err == nil || !strings.Contains(err.Error(), "state revoked") {
		t.Fatalf("revoked key should not be trust-bound, got %v", err)
	}

	expired, err := actions.GenerateKey(ctx, KeyGenerateOptions{Scope: contracts.KeyScopeCollaborator, OwnerID: "expired"}, contracts.Actor("human:owner"), "create expired key")
	if err != nil {
		t.Fatalf("generate expired key: %v", err)
	}
	expiredRecord := expired.PublicKey
	expiredRecord.ExpiresAt = actions.now().Add(-time.Second)
	if err := actions.SecurityKeys.SavePublicKey(ctx, expiredRecord); err != nil {
		t.Fatalf("save expired key: %v", err)
	}
	if _, err := actions.BindTrust(ctx, "expired", expired.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "trust expired key"); err == nil || !strings.Contains(err.Error(), "expired key") {
		t.Fatalf("expired key should not be trust-bound, got %v", err)
	}
}

func TestSecuritySignVerifyLifecycleStates(t *testing.T) {
	ctx, _, actions := newSecurityTestActions(t)
	key, err := actions.GenerateKey(ctx, KeyGenerateOptions{Scope: contracts.KeyScopeCollaborator, OwnerID: "alice"}, contracts.Actor("human:owner"), "create alice key")
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	payload := map[string]any{
		"kind":  "bundle",
		"title": "hello\r\nworld",
		"items": []string{"one", "two"},
	}
	sig, err := actions.SignPayload(ctx, SignatureRequest{
		ArtifactKind: contracts.ArtifactKindBundle,
		ArtifactUID:  "bundle-1",
		PublicKeyID:  key.PublicKey.PublicKeyID,
		Payload:      payload,
	})
	if err != nil {
		t.Fatalf("sign payload: %v", err)
	}
	verify, err := actions.VerifyPayloadSignature(ctx, payload, &sig)
	if err != nil {
		t.Fatalf("verify untrusted signature: %v", err)
	}
	if verify.State != contracts.VerificationValidUntrusted {
		t.Fatalf("state before trust = %s, want %s", verify.State, contracts.VerificationValidUntrusted)
	}
	if _, err := actions.BindTrust(ctx, "alice", key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "trust alice key"); err != nil {
		t.Fatalf("bind trust: %v", err)
	}
	verify, err = actions.VerifyPayloadSignature(ctx, payload, &sig)
	if err != nil {
		t.Fatalf("verify trusted signature: %v", err)
	}
	if verify.State != contracts.VerificationTrustedValid || verify.TrustedOwnerID != "alice" {
		t.Fatalf("trusted verification failed: %#v", verify)
	}
	tamperedEnvelope := sig
	tamperedEnvelope.PublicKeyFingerprint = "ed25519:" + strings.Repeat("0", 64)
	verify, err = actions.VerifyPayloadSignature(ctx, payload, &tamperedEnvelope)
	if err != nil {
		t.Fatalf("verify tampered envelope metadata: %v", err)
	}
	if verify.State != contracts.VerificationMalformedSignature || !strings.Contains(strings.Join(verify.ReasonCodes, ","), "signature_key_metadata_mismatch") {
		t.Fatalf("tampered envelope metadata should be rejected: %#v", verify)
	}
	tamperedEnvelope = sig
	tamperedEnvelope.PublicKeyID = "key-unknown"
	tamperedEnvelope.Signature = "not-base64"
	verify, err = actions.VerifyPayloadSignature(ctx, payload, &tamperedEnvelope)
	if err != nil {
		t.Fatalf("verify malformed unknown-key envelope: %v", err)
	}
	if verify.State != contracts.VerificationMalformedSignature || !strings.Contains(strings.Join(verify.ReasonCodes, ","), "signature_malformed") {
		t.Fatalf("malformed signature bytes must not be reported as valid_unknown_key: %#v", verify)
	}
	tamperedEnvelope = sig
	tamperedEnvelope.ArtifactUID = "bundle-2"
	verify, err = actions.VerifyPayloadSignature(ctx, payload, &tamperedEnvelope)
	if err != nil {
		t.Fatalf("verify tampered artifact uid: %v", err)
	}
	if verify.State != contracts.VerificationInvalidSignature {
		t.Fatalf("artifact uid replay should fail signature verification: %#v", verify)
	}
	tamperedEnvelope = sig
	tamperedEnvelope.ArtifactKind = contracts.ArtifactKindBackupSnapshot
	verify, err = actions.VerifyPayloadSignature(ctx, payload, &tamperedEnvelope)
	if err != nil {
		t.Fatalf("verify tampered artifact kind: %v", err)
	}
	if verify.State != contracts.VerificationInvalidSignature {
		t.Fatalf("artifact kind replay should fail signature verification: %#v", verify)
	}

	tampered := map[string]any{"kind": "bundle", "title": "changed", "items": []string{"one", "two"}}
	verify, err = actions.VerifyPayloadSignature(ctx, tampered, &sig)
	if err != nil {
		t.Fatalf("verify tampered payload: %v", err)
	}
	if verify.State != contracts.VerificationPayloadHashMismatch {
		t.Fatalf("tamper state = %s, want %s", verify.State, contracts.VerificationPayloadHashMismatch)
	}

	if _, err := actions.RotateKey(ctx, key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "rotate alice key"); err != nil {
		t.Fatalf("rotate key: %v", err)
	}
	if _, err := actions.SignPayload(ctx, SignatureRequest{ArtifactKind: contracts.ArtifactKindBundle, ArtifactUID: "bundle-2", PublicKeyID: key.PublicKey.PublicKeyID, Payload: payload}); err == nil {
		t.Fatalf("rotated key should not sign new payloads")
	}
	verify, err = actions.VerifyPayloadSignature(ctx, payload, &sig)
	if err != nil {
		t.Fatalf("verify rotated key signature: %v", err)
	}
	if verify.State != contracts.VerificationValidRotatedKey {
		t.Fatalf("rotated state = %s, want %s", verify.State, contracts.VerificationValidRotatedKey)
	}

	if _, err := actions.RevokeKey(ctx, key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "compromised key"); err != nil {
		t.Fatalf("revoke key: %v", err)
	}
	verify, err = actions.VerifyPayloadSignature(ctx, payload, &sig)
	if err != nil {
		t.Fatalf("verify revoked key signature: %v", err)
	}
	if verify.State != contracts.VerificationValidRevokedKey {
		t.Fatalf("revoked state = %s, want %s", verify.State, contracts.VerificationValidRevokedKey)
	}
}

func TestSecuritySyncedRevocationAffectsVerification(t *testing.T) {
	ctx, _, actions := newSecurityTestActions(t)
	key, err := actions.GenerateKey(ctx, KeyGenerateOptions{Scope: contracts.KeyScopeCollaborator, OwnerID: "alice"}, contracts.Actor("human:owner"), "create alice key")
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	payload := map[string]string{"bundle": "revoked"}
	sig, err := actions.SignPayload(ctx, SignatureRequest{ArtifactKind: contracts.ArtifactKindBundle, ArtifactUID: "bundle-revoked", PublicKeyID: key.PublicKey.PublicKeyID, Payload: payload})
	if err != nil {
		t.Fatalf("sign payload: %v", err)
	}
	if _, err := actions.BindTrust(ctx, "alice", key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "trust alice key"); err != nil {
		t.Fatalf("bind trust: %v", err)
	}
	revocation := contracts.RevocationRecord{
		RevocationID:  "revocation-synced",
		PublicKeyID:   key.PublicKey.PublicKeyID,
		Fingerprint:   key.PublicKey.Fingerprint,
		RevokedBy:     contracts.Actor("human:owner"),
		RevokedAt:     time.Date(2026, 5, 6, 15, 0, 0, 0, time.UTC),
		Reason:        "synced revocation",
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := actions.SecurityKeys.SaveRevocation(ctx, revocation); err != nil {
		t.Fatalf("save synced revocation: %v", err)
	}
	verify, err := actions.VerifyPayloadSignature(ctx, payload, &sig)
	if err != nil {
		t.Fatalf("verify signature with synced revocation: %v", err)
	}
	if verify.State != contracts.VerificationValidRevokedKey {
		t.Fatalf("synced revocation state = %s, want %s", verify.State, contracts.VerificationValidRevokedKey)
	}
}

func TestSecurityExpiredKeyBlocksSigningAndVerifiesHistorically(t *testing.T) {
	ctx, _, actions := newSecurityTestActions(t)
	key, err := actions.GenerateKey(ctx, KeyGenerateOptions{}, contracts.Actor("human:owner"), "create workspace key")
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	payload := map[string]string{"bundle": "old"}
	sig, err := actions.SignPayload(ctx, SignatureRequest{ArtifactKind: contracts.ArtifactKindBundle, ArtifactUID: "bundle-before-expiry", PublicKeyID: key.PublicKey.PublicKeyID, Payload: payload})
	if err != nil {
		t.Fatalf("sign before expiry: %v", err)
	}
	expired := key.PublicKey
	expired.ExpiresAt = actions.now().Add(-time.Second)
	if err := actions.SecurityKeys.SavePublicKey(ctx, expired); err != nil {
		t.Fatalf("save expired public key: %v", err)
	}
	if _, err := actions.SignPayload(ctx, SignatureRequest{ArtifactKind: contracts.ArtifactKindBundle, ArtifactUID: "bundle-after-expiry", PublicKeyID: key.PublicKey.PublicKeyID, Payload: payload}); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired key should block new signatures, got %v", err)
	}
	verify, err := actions.VerifyPayloadSignature(ctx, payload, &sig)
	if err != nil {
		t.Fatalf("verify expired historical signature: %v", err)
	}
	if verify.State != contracts.VerificationValidExpiredKey {
		t.Fatalf("expired key state = %s, want %s", verify.State, contracts.VerificationValidExpiredKey)
	}
}

func TestSecurityImportedPublicKeyIsUntrustedAndCannotSign(t *testing.T) {
	ctx, _, signer := newSecurityTestActions(t)
	signingKey, err := signer.GenerateKey(ctx, KeyGenerateOptions{Scope: contracts.KeyScopeCollaborator, OwnerID: "bob"}, contracts.Actor("human:owner"), "create bob key")
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}
	payload := map[string]string{"bundle": "shared"}
	sig, err := signer.SignPayload(ctx, SignatureRequest{ArtifactKind: contracts.ArtifactKindBundle, ArtifactUID: "bundle-bob", PublicKeyID: signingKey.PublicKey.PublicKeyID, Payload: payload})
	if err != nil {
		t.Fatalf("sign with source workspace: %v", err)
	}
	publicRaw, err := json.MarshalIndent(signingKey.PublicKey, "", "  ")
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	importPath := filepath.Join(t.TempDir(), "bob-public.json")
	if err := os.WriteFile(importPath, append(publicRaw, '\n'), 0o644); err != nil {
		t.Fatalf("write import public key: %v", err)
	}

	ctx2, _, verifier := newSecurityTestActions(t)
	imported, err := verifier.ImportPublicKey(ctx2, importPath, contracts.Actor("human:owner"), "import bob public key")
	if err != nil {
		t.Fatalf("import public key: %v", err)
	}
	if imported.PublicKey.Status != contracts.KeyStateImported || imported.PublicKey.Source != contracts.PublicKeySourceManualImport || imported.CanSign {
		t.Fatalf("imported public key should be untrusted non-signing material: %#v", imported)
	}
	verify, err := verifier.VerifyPayloadSignature(ctx2, payload, &sig)
	if err != nil {
		t.Fatalf("verify imported public key: %v", err)
	}
	if verify.State != contracts.VerificationValidUntrusted {
		t.Fatalf("imported key state = %s, want %s", verify.State, contracts.VerificationValidUntrusted)
	}
	if _, err := verifier.BindTrust(ctx2, "bob", imported.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "trust bob key"); err != nil {
		t.Fatalf("bind imported key: %v", err)
	}
	verify, err = verifier.VerifyPayloadSignature(ctx2, payload, &sig)
	if err != nil {
		t.Fatalf("verify trusted imported key: %v", err)
	}
	if verify.State != contracts.VerificationTrustedValid {
		t.Fatalf("trusted imported key state = %s, want %s", verify.State, contracts.VerificationTrustedValid)
	}
	status, err := verifier.TrustStatus(ctx2)
	if err != nil {
		t.Fatalf("trust status after imported trust: %v", err)
	}
	if status.ImportedUntrusted != 0 || status.TrustedBindings != 1 {
		t.Fatalf("trusted imported key should not count as imported_untrusted: %#v", status)
	}
	if _, err := verifier.SignPayload(ctx2, SignatureRequest{ArtifactKind: contracts.ArtifactKindBundle, ArtifactUID: "bundle-bob-2", PublicKeyID: imported.PublicKey.PublicKeyID, Payload: payload}); err == nil {
		t.Fatalf("imported public key should not sign without local private material")
	}

	localImportPath := filepath.Join(t.TempDir(), "same-local-key.json")
	localRaw, err := json.MarshalIndent(imported.PublicKey, "", "  ")
	if err != nil {
		t.Fatalf("marshal local import fixture: %v", err)
	}
	if err := os.WriteFile(localImportPath, append(localRaw, '\n'), 0o644); err != nil {
		t.Fatalf("write local import fixture: %v", err)
	}
	if _, err := verifier.ImportPublicKey(ctx2, localImportPath, contracts.Actor("human:owner"), "reimport bob key"); err != nil {
		t.Fatalf("reimporting an imported public key should be idempotent enough for sync/import flows: %v", err)
	}
	localKey, err := verifier.GenerateKey(ctx2, KeyGenerateOptions{}, contracts.Actor("human:owner"), "create local key")
	if err != nil {
		t.Fatalf("generate local key: %v", err)
	}
	localRaw, err = json.MarshalIndent(localKey.PublicKey, "", "  ")
	if err != nil {
		t.Fatalf("marshal local key: %v", err)
	}
	if err := os.WriteFile(localImportPath, append(localRaw, '\n'), 0o644); err != nil {
		t.Fatalf("write local overwrite fixture: %v", err)
	}
	if _, err := verifier.ImportPublicKey(ctx2, localImportPath, contracts.Actor("human:owner"), "overwrite local key"); err == nil {
		t.Fatalf("import should not overwrite local private-backed key records")
	}

	ctx3, _, attacker := newSecurityTestActions(t)
	attackerKey, err := attacker.GenerateKey(ctx3, KeyGenerateOptions{}, contracts.Actor("human:owner"), "create attacker key")
	if err != nil {
		t.Fatalf("generate attacker key: %v", err)
	}
	attackerRecord := attackerKey.PublicKey
	attackerRecord.PublicKeyID = signingKey.PublicKey.PublicKeyID
	attackerRecord.Fingerprint = signingKey.PublicKey.Fingerprint
	tamperedRaw, err := json.MarshalIndent(attackerRecord, "", "  ")
	if err != nil {
		t.Fatalf("marshal tampered public key: %v", err)
	}
	tamperedPath := filepath.Join(t.TempDir(), "tampered-public.json")
	if err := os.WriteFile(tamperedPath, append(tamperedRaw, '\n'), 0o644); err != nil {
		t.Fatalf("write tampered public key: %v", err)
	}
	if _, err := verifier.ImportPublicKey(ctx2, tamperedPath, contracts.Actor("human:owner"), "import tampered key"); err == nil || !strings.Contains(err.Error(), "fingerprint does not match") {
		t.Fatalf("public import should reject claimed identity that does not match key bytes, got %v", err)
	}
}

func TestSecurityPrivateKeyPermissionsBlockSigning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows permission semantics are surfaced separately")
	}
	ctx, root, actions := newSecurityTestActions(t)
	key, err := actions.GenerateKey(ctx, KeyGenerateOptions{}, contracts.Actor("human:owner"), "create workspace key")
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	privatePath := storage.PrivateKeyFile(root, key.PublicKey.PublicKeyID)
	if err := os.Chmod(privatePath, 0o644); err != nil {
		t.Fatalf("chmod private key: %v", err)
	}
	detail, err := actions.KeyDetail(ctx, key.PublicKey.PublicKeyID)
	if err != nil {
		t.Fatalf("key detail: %v", err)
	}
	if detail.CanSign || !strings.Contains(strings.Join(detail.ReasonCodes, ","), "private_key_permissions_too_broad") {
		t.Fatalf("unsafe private key should not be signable: %#v", detail)
	}
	if _, err := actions.SignPayload(ctx, SignatureRequest{ArtifactKind: contracts.ArtifactKindBundle, ArtifactUID: "bundle-unsafe", PublicKeyID: key.PublicKey.PublicKeyID, Payload: map[string]string{"x": "y"}}); err == nil {
		t.Fatalf("unsafe private key permissions should block signing")
	}
	if err := os.Chmod(privatePath, 0o400); err != nil {
		t.Fatalf("chmod private key strict: %v", err)
	}
	detail, err = actions.KeyDetail(ctx, key.PublicKey.PublicKeyID)
	if err != nil {
		t.Fatalf("key detail after strict chmod: %v", err)
	}
	if !detail.CanSign || !detail.PrivateKeyHealth.PermissionsOK {
		t.Fatalf("0400 private key should be accepted as strict owner-only material: %#v", detail)
	}
	if _, err := actions.SignPayload(ctx, SignatureRequest{ArtifactKind: contracts.ArtifactKindBundle, ArtifactUID: "bundle-strict", PublicKeyID: key.PublicKey.PublicKeyID, Payload: map[string]string{"x": "y"}}); err != nil {
		t.Fatalf("strict private key mode should sign: %v", err)
	}
}

func TestSecurityMissingPrivateKeyReasonIsActionable(t *testing.T) {
	ctx, root, actions := newSecurityTestActions(t)
	key, err := actions.GenerateKey(ctx, KeyGenerateOptions{}, contracts.Actor("human:owner"), "create workspace key")
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	privatePath := storage.PrivateKeyFile(root, key.PublicKey.PublicKeyID)
	if err := os.Remove(privatePath); err != nil {
		t.Fatalf("remove private key fixture: %v", err)
	}
	detail, err := actions.KeyDetail(ctx, key.PublicKey.PublicKeyID)
	if err != nil {
		t.Fatalf("key detail: %v", err)
	}
	if detail.CanSign || !strings.Contains(strings.Join(detail.ReasonCodes, ","), "private_key_missing") {
		t.Fatalf("missing private key should report missing material: %#v", detail)
	}
}

func TestPrivateKeyPermissionEvaluationSurfacesUnsupportedPlatforms(t *testing.T) {
	ok, unsupported, warnings := evaluatePrivateKeyMode("windows", 0o666)
	if !ok || !unsupported || !strings.Contains(strings.Join(warnings, ","), "private_key_permissions_unverified") {
		t.Fatalf("windows-like private-key mode should be warning-only: ok=%v unsupported=%v warnings=%v", ok, unsupported, warnings)
	}
	ok, unsupported, warnings = evaluatePrivateKeyMode("linux", 0o400)
	if !ok || unsupported || len(warnings) != 0 {
		t.Fatalf("strict owner-readable POSIX key mode should pass: ok=%v unsupported=%v warnings=%v", ok, unsupported, warnings)
	}
	ok, unsupported, warnings = evaluatePrivateKeyMode("linux", 0o644)
	if ok || unsupported || !strings.Contains(strings.Join(warnings, ","), "private_key_permissions_too_broad") {
		t.Fatalf("group/other POSIX key mode should fail: ok=%v unsupported=%v warnings=%v", ok, unsupported, warnings)
	}
}

func TestSecurityPrivateKeyMustMatchPublicRecord(t *testing.T) {
	ctx, _, actions := newSecurityTestActions(t)
	key, err := actions.GenerateKey(ctx, KeyGenerateOptions{}, contracts.Actor("human:owner"), "create workspace key")
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	privateRecord, err := actions.SecurityKeys.LoadPrivateKey(ctx, key.PublicKey.PublicKeyID)
	if err != nil {
		t.Fatalf("load private key: %v", err)
	}
	privateRecord.Fingerprint = "sha256:not-this-key"
	if err := actions.SecurityKeys.SavePrivateKey(ctx, privateRecord); err != nil {
		t.Fatalf("save mismatched private key fixture: %v", err)
	}
	detail, err := actions.KeyDetail(ctx, key.PublicKey.PublicKeyID)
	if err != nil {
		t.Fatalf("key detail with mismatched private key: %v", err)
	}
	if detail.CanSign || !strings.Contains(strings.Join(detail.ReasonCodes, ","), "private_key_metadata_mismatch") {
		t.Fatalf("key detail should not report mismatched private key as signable: %#v", detail)
	}
	_, err = actions.SignPayload(ctx, SignatureRequest{
		ArtifactKind: contracts.ArtifactKindBundle,
		ArtifactUID:  "bundle-mismatch",
		PublicKeyID:  key.PublicKey.PublicKeyID,
		Payload:      map[string]string{"x": "y"},
	})
	if err == nil || !strings.Contains(err.Error(), "metadata does not match") {
		t.Fatalf("expected private/public mismatch error, got %v", err)
	}
}
