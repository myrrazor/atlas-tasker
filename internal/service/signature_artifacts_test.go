package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestSignAndVerifyExportBundle(t *testing.T) {
	ctx := context.Background()
	root, actions, _, projectStore, _, _ := newImportExportHarness(t)
	seedSyncWorkspace(t, ctx, actions, projectStore)
	key := generateTrustedSigningKey(t, ctx, actions, "alice")

	bundle, err := actions.CreateExportBundle(ctx, "workspace", contracts.Actor("human:owner"), "create export")
	if err != nil {
		t.Fatalf("create export: %v", err)
	}
	unsigned, err := actions.VerifyExportBundleSignature(ctx, bundle.Bundle.BundleID)
	if err != nil {
		t.Fatalf("verify unsigned export: %v", err)
	}
	if unsigned.Signature.State != contracts.VerificationMissingSignature {
		t.Fatalf("unsigned export should be explicit missing_signature, got %#v", unsigned.Signature)
	}

	signed, err := actions.SignExportBundle(ctx, bundle.Bundle.BundleID, key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "sign export")
	if err != nil {
		t.Fatalf("sign export: %v", err)
	}
	if signed.Signature.ArtifactKind != contracts.ArtifactKindBundle || signed.Signature.ArtifactUID != bundle.Bundle.BundleID {
		t.Fatalf("signature should bind export bundle identity: %#v", signed.Signature)
	}
	if _, err := os.Stat(storage.SignatureFile(root, sanitizeSecurityID(signed.Signature.SignatureID))); err != nil {
		t.Fatalf("signature sidecar missing: %v", err)
	}
	if _, err := os.Stat(exportSignatureSidecarPath(bundle.Bundle.ArtifactPath)); err != nil {
		t.Fatalf("portable export signature sidecar missing: %v", err)
	}
	verified, err := actions.VerifyExportBundleSignature(ctx, bundle.Bundle.BundleID)
	if err != nil {
		t.Fatalf("verify signed export: %v", err)
	}
	if verified.Signature.State != contracts.VerificationTrustedValid {
		t.Fatalf("signed export should verify trusted, got %#v", verified.Signature)
	}
	verifiedByPath, err := actions.VerifyExportBundleSignature(ctx, bundle.Bundle.ArtifactPath)
	if err != nil {
		t.Fatalf("verify signed export by path: %v", err)
	}
	if verifiedByPath.Signature.State != contracts.VerificationTrustedValid {
		t.Fatalf("signed export path should hydrate signature metadata, got %#v", verifiedByPath.Signature)
	}
	loaded, err := actions.ExportBundles.LoadExportBundle(ctx, bundle.Bundle.BundleID)
	if err != nil {
		t.Fatalf("load signed export: %v", err)
	}
	if len(loaded.SignatureEnvelopes) != 1 || loaded.SignatureEnvelopes[0].SignatureID != signed.Signature.SignatureID {
		t.Fatalf("export metadata should carry signature: %#v", loaded.SignatureEnvelopes)
	}
	_, targetActions, _, _, _, _ := newImportExportHarness(t)
	if err := targetActions.SecurityKeys.SavePublicKey(ctx, key.PublicKey); err != nil {
		t.Fatalf("seed target public key: %v", err)
	}
	if _, err := targetActions.BindTrust(ctx, "alice", key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "trust copied signer"); err != nil {
		t.Fatalf("trust copied signer: %v", err)
	}
	copiedDir := t.TempDir()
	copiedArtifact := filepath.Join(copiedDir, filepath.Base(bundle.Bundle.ArtifactPath))
	for _, pair := range [][2]string{
		{bundle.Bundle.ArtifactPath, copiedArtifact},
		{bundle.Bundle.ManifestPath, strings.TrimSuffix(copiedArtifact, ".tar.gz") + ".manifest.json"},
		{bundle.Bundle.ChecksumPath, strings.TrimSuffix(copiedArtifact, ".tar.gz") + ".sha256"},
		{exportSignatureSidecarPath(bundle.Bundle.ArtifactPath), exportSignatureSidecarPath(copiedArtifact)},
	} {
		if err := copySyncFile(pair[0], pair[1]); err != nil {
			t.Fatalf("copy export sidecar %s -> %s: %v", pair[0], pair[1], err)
		}
	}
	copied, err := targetActions.VerifyExportBundleSignature(ctx, copiedArtifact)
	if err != nil {
		t.Fatalf("verify copied signed export: %v", err)
	}
	if copied.Signature.State != contracts.VerificationTrustedValid {
		t.Fatalf("copied export signature should verify trusted, got %#v", copied.Signature)
	}
	if err := os.Remove(strings.TrimSuffix(copiedArtifact, ".tar.gz") + ".manifest.json"); err != nil {
		t.Fatalf("remove copied manifest sidecar: %v", err)
	}
	if _, err := targetActions.VerifyExportBundleSignature(ctx, copiedArtifact); err == nil || !strings.Contains(err.Error(), "sidecar_manifest_missing") {
		t.Fatalf("expected missing manifest sidecar reason, got %v", err)
	}
	secondKey, err := actions.GenerateKey(ctx, KeyGenerateOptions{Scope: contracts.KeyScopeWorkspace}, contracts.Actor("human:owner"), "create second signing key")
	if err != nil {
		t.Fatalf("generate second key: %v", err)
	}
	secondSignature, err := actions.SignExportBundle(ctx, bundle.Bundle.BundleID, secondKey.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "co-sign export")
	if err != nil {
		t.Fatalf("co-sign export: %v", err)
	}
	loaded, err = actions.ExportBundles.LoadExportBundle(ctx, bundle.Bundle.BundleID)
	if err != nil {
		t.Fatalf("load co-signed export: %v", err)
	}
	if len(loaded.SignatureEnvelopes) != 2 {
		t.Fatalf("co-signed export should retain both envelopes: %#v", loaded.SignatureEnvelopes)
	}
	if loaded.SignatureEnvelopes[0].SignatureID == loaded.SignatureEnvelopes[1].SignatureID || secondSignature.Signature.SignatureID == signed.Signature.SignatureID {
		t.Fatalf("co-signatures must have signer-specific ids: %#v", loaded.SignatureEnvelopes)
	}
	coSigned, err := actions.VerifyExportBundleSignature(ctx, bundle.Bundle.BundleID)
	if err != nil {
		t.Fatalf("verify co-signed export: %v", err)
	}
	if coSigned.Signature.State != contracts.VerificationTrustedValid || coSigned.Signature.Signature == nil || coSigned.Signature.Signature.SignatureID != signed.Signature.SignatureID {
		t.Fatalf("co-signed export should prefer trusted signature over later untrusted signer, got %#v", coSigned.Signature)
	}
	pathSignDir := t.TempDir()
	pathSignedArtifact := filepath.Join(pathSignDir, filepath.Base(bundle.Bundle.ArtifactPath))
	for _, pair := range [][2]string{
		{bundle.Bundle.ArtifactPath, pathSignedArtifact},
		{bundle.Bundle.ManifestPath, strings.TrimSuffix(pathSignedArtifact, ".tar.gz") + ".manifest.json"},
		{bundle.Bundle.ChecksumPath, strings.TrimSuffix(pathSignedArtifact, ".tar.gz") + ".sha256"},
		{exportSignatureSidecarPath(bundle.Bundle.ArtifactPath), exportSignatureSidecarPath(pathSignedArtifact)},
	} {
		if err := copySyncFile(pair[0], pair[1]); err != nil {
			t.Fatalf("copy path-sign sidecar %s -> %s: %v", pair[0], pair[1], err)
		}
	}
	if _, err := actions.SignExportBundle(ctx, pathSignedArtifact, secondKey.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "sign copied export path"); err != nil {
		t.Fatalf("sign copied export path: %v", err)
	}
	if err := os.RemoveAll(pathSignDir); err != nil {
		t.Fatalf("remove copied export path: %v", err)
	}
	loaded, err = actions.ExportBundles.LoadExportBundle(ctx, bundle.Bundle.BundleID)
	if err != nil {
		t.Fatalf("load export after copied path sign: %v", err)
	}
	if loaded.ArtifactPath != bundle.Bundle.ArtifactPath || loaded.ManifestPath != bundle.Bundle.ManifestPath || loaded.ChecksumPath != bundle.Bundle.ChecksumPath {
		t.Fatalf("path-based signing should not rewrite stored bundle paths: %#v", loaded)
	}
	afterPathSign, err := actions.VerifyExportBundleSignature(ctx, bundle.Bundle.BundleID)
	if err != nil {
		t.Fatalf("verify export after copied path sign: %v", err)
	}
	if afterPathSign.Signature.State != contracts.VerificationTrustedValid {
		t.Fatalf("stored export should still verify after copied path sign, got %#v", afterPathSign.Signature)
	}
	if err := os.WriteFile(bundle.Bundle.ChecksumPath, []byte("bad-checksum  "+filepath.Base(bundle.Bundle.ArtifactPath)+"\n"), 0o644); err != nil {
		t.Fatalf("corrupt export checksum: %v", err)
	}
	corrupt, err := actions.VerifyExportBundleSignature(ctx, bundle.Bundle.BundleID)
	if err != nil {
		t.Fatalf("verify export with integrity failure should report, not error: %v", err)
	}
	corruptIntegrity, ok := corrupt.Integrity.(ExportVerifyView)
	if !ok {
		t.Fatalf("expected export integrity view, got %T", corrupt.Integrity)
	}
	if corruptIntegrity.Verified || len(corruptIntegrity.Errors) == 0 {
		t.Fatalf("corrupt checksum should be reported in integrity view: %#v", corruptIntegrity)
	}

	files, err := readBundleArchive(bundle.Bundle.ArtifactPath)
	if err != nil {
		t.Fatalf("read export artifact: %v", err)
	}
	for name, raw := range files {
		if strings.Contains(string(raw), "private_key_material") {
			t.Fatalf("export bundle leaked private key material in %s", name)
		}
	}
}

func TestSignExportBundleRejectsManifestIdentityMismatch(t *testing.T) {
	ctx := context.Background()
	_, actions, _, projectStore, _, _ := newImportExportHarness(t)
	seedSyncWorkspace(t, ctx, actions, projectStore)
	key := generateTrustedSigningKey(t, ctx, actions, "alice")

	first, err := actions.CreateExportBundle(ctx, "workspace", contracts.Actor("human:owner"), "create first export")
	if err != nil {
		t.Fatalf("create first export: %v", err)
	}
	second, err := actions.CreateExportBundle(ctx, "workspace", contracts.Actor("human:owner"), "create second export")
	if err != nil {
		t.Fatalf("create second export: %v", err)
	}
	for _, pair := range [][2]string{
		{second.Bundle.ArtifactPath, first.Bundle.ArtifactPath},
		{second.Bundle.ManifestPath, first.Bundle.ManifestPath},
		{second.Bundle.ChecksumPath, first.Bundle.ChecksumPath},
	} {
		if err := copySyncFile(pair[0], pair[1]); err != nil {
			t.Fatalf("replace export sidecar %s -> %s: %v", pair[0], pair[1], err)
		}
	}
	if _, err := actions.SignExportBundle(ctx, first.Bundle.BundleID, key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "sign stale export"); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("signing stale export identity should fail with mismatch, got %v", err)
	}
	if _, err := actions.VerifyExportBundleSignature(ctx, first.Bundle.BundleID); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("verifying stale export identity should fail with mismatch, got %v", err)
	}
}

func TestSignAndVerifySyncPublication(t *testing.T) {
	ctx := context.Background()
	_, actions, _, projectStore, _, _ := newImportExportHarness(t)
	seedSyncWorkspace(t, ctx, actions, projectStore)
	key := generateTrustedSigningKey(t, ctx, actions, "alice")

	bundle, err := actions.CreateSyncBundle(ctx, contracts.Actor("human:owner"), "create sync")
	if err != nil {
		t.Fatalf("create sync bundle: %v", err)
	}
	unsigned, err := actions.VerifySyncPublicationSignature(ctx, bundle.Job.BundleRef)
	if err != nil {
		t.Fatalf("verify unsigned sync publication: %v", err)
	}
	if unsigned.Signature.State != contracts.VerificationMissingSignature {
		t.Fatalf("unsigned sync publication should be explicit missing_signature, got %#v", unsigned.Signature)
	}
	signed, err := actions.SignSyncPublication(ctx, bundle.Job.BundleRef, key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "sign sync publication")
	if err != nil {
		t.Fatalf("sign sync publication: %v", err)
	}
	if signed.Signature.ArtifactKind != contracts.ArtifactKindSyncPublication || signed.Signature.ArtifactUID != bundle.Publication.BundleID {
		t.Fatalf("signature should bind sync publication identity: %#v", signed.Signature)
	}
	publication, err := readSyncPublication(strings.TrimSuffix(bundle.Job.BundleRef, ".tar.gz") + ".publication.json")
	if err != nil {
		t.Fatalf("read signed publication: %v", err)
	}
	if len(publication.SignatureEnvelopes) != 1 {
		t.Fatalf("publication should carry signature envelope: %#v", publication.SignatureEnvelopes)
	}
	verified, err := actions.VerifySyncPublicationSignature(ctx, bundle.Job.BundleRef)
	if err != nil {
		t.Fatalf("verify signed sync publication: %v", err)
	}
	if verified.Signature.State != contracts.VerificationTrustedValid {
		t.Fatalf("signed sync publication should verify trusted, got %#v", verified.Signature)
	}
	untrustedKey, err := actions.GenerateKey(ctx, KeyGenerateOptions{Scope: contracts.KeyScopeWorkspace}, contracts.Actor("human:owner"), "create untrusted sync signer")
	if err != nil {
		t.Fatalf("generate untrusted sync signer: %v", err)
	}
	if _, err := actions.SignSyncPublication(ctx, bundle.Job.BundleRef, untrustedKey.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "co-sign sync publication"); err != nil {
		t.Fatalf("co-sign sync publication: %v", err)
	}
	coSignedSync, err := actions.VerifySyncPublicationSignature(ctx, bundle.Job.BundleRef)
	if err != nil {
		t.Fatalf("verify co-signed sync publication: %v", err)
	}
	if coSignedSync.Signature.State != contracts.VerificationTrustedValid || coSignedSync.Signature.Signature == nil || coSignedSync.Signature.Signature.SignatureID != signed.Signature.SignatureID {
		t.Fatalf("co-signed sync publication should prefer trusted signature over later untrusted signer, got %#v", coSignedSync.Signature)
	}
	tamperedBundle, err := actions.CreateSyncBundle(ctx, contracts.Actor("human:owner"), "create replacement sync")
	if err != nil {
		t.Fatalf("create replacement sync bundle: %v", err)
	}
	copySyncBundleSidecars(t, tamperedBundle.Job.BundleRef, bundle.Job.BundleRef)
	tampered, err := actions.VerifySyncPublicationSignature(ctx, bundle.Job.BundleRef)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("verifying stale sync publication should fail with mismatch, got %v and %#v", err, tampered.Signature)
	}
	if _, err := actions.SignSyncPublication(ctx, bundle.Job.BundleRef, untrustedKey.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "sign stale sync publication"); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("signing stale sync publication should fail with mismatch, got %v", err)
	}

	files, err := readBundleArchive(filepath.Join(storage.SyncBundlesDir(actions.Root), publication.ArtifactName))
	if err != nil {
		t.Fatalf("read sync artifact: %v", err)
	}
	for name, raw := range files {
		if strings.Contains(string(raw), "private_key_material") {
			t.Fatalf("sync bundle leaked private key material in %s", name)
		}
	}
}

func TestVerifySyncPublicationRejectsMismatchedEnvelope(t *testing.T) {
	ctx := context.Background()
	_, actions, _, projectStore, _, _ := newImportExportHarness(t)
	seedSyncWorkspace(t, ctx, actions, projectStore)
	key := generateTrustedSigningKey(t, ctx, actions, "alice")

	bundle, err := actions.CreateSyncBundle(ctx, contracts.Actor("human:owner"), "create sync")
	if err != nil {
		t.Fatalf("create sync bundle: %v", err)
	}
	if _, err := actions.SignSyncPublication(ctx, bundle.Job.BundleRef, key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "sign sync publication"); err != nil {
		t.Fatalf("sign sync publication: %v", err)
	}
	publicationPath := strings.TrimSuffix(bundle.Job.BundleRef, ".tar.gz") + ".publication.json"
	publication, err := readSyncPublication(publicationPath)
	if err != nil {
		t.Fatalf("read sync publication: %v", err)
	}
	if len(publication.SignatureEnvelopes) != 1 {
		t.Fatalf("expected one signature envelope, got %#v", publication.SignatureEnvelopes)
	}
	publication.SignatureEnvelopes[0].ArtifactUID = "syncbundle_other"
	if err := writeSyncPublication(publicationPath, publication); err != nil {
		t.Fatalf("write mismatched publication: %v", err)
	}
	if _, err := actions.VerifySyncPublicationSignature(ctx, bundle.Job.BundleRef); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched sync signature envelope should fail, got %v", err)
	}
}

func TestSignRemoteStyleSyncPublicationWritesPublicationJSON(t *testing.T) {
	ctx := context.Background()
	_, actions, _, projectStore, _, _ := newImportExportHarness(t)
	seedSyncWorkspace(t, ctx, actions, projectStore)
	key := generateTrustedSigningKey(t, ctx, actions, "alice")

	bundle, err := actions.CreateSyncBundle(ctx, contracts.Actor("human:owner"), "create sync bundle")
	if err != nil {
		t.Fatalf("create sync bundle: %v", err)
	}
	remoteDir := t.TempDir()
	remoteArtifact := filepath.Join(remoteDir, bundle.Publication.ArtifactName)
	remoteManifest := filepath.Join(remoteDir, bundle.Publication.ManifestName)
	remoteChecksum := filepath.Join(remoteDir, bundle.Publication.ChecksumName)
	if err := copySyncFile(bundle.Job.BundleRef, remoteArtifact); err != nil {
		t.Fatalf("copy remote artifact: %v", err)
	}
	if err := copySyncFile(strings.TrimSuffix(bundle.Job.BundleRef, ".tar.gz")+".manifest.json", remoteManifest); err != nil {
		t.Fatalf("copy remote manifest: %v", err)
	}
	if err := copySyncFile(strings.TrimSuffix(bundle.Job.BundleRef, ".tar.gz")+".sha256", remoteChecksum); err != nil {
		t.Fatalf("copy remote checksum: %v", err)
	}
	if err := writeSyncPublication(filepath.Join(remoteDir, "publication.json"), bundle.Publication); err != nil {
		t.Fatalf("write remote publication: %v", err)
	}

	if _, err := actions.SignSyncPublication(ctx, remoteArtifact, key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "sign remote publication"); err != nil {
		t.Fatalf("sign remote-style publication: %v", err)
	}
	publication, err := readSyncPublication(filepath.Join(remoteDir, "publication.json"))
	if err != nil {
		t.Fatalf("read remote publication: %v", err)
	}
	if len(publication.SignatureEnvelopes) != 1 {
		t.Fatalf("canonical publication.json should carry signature: %#v", publication.SignatureEnvelopes)
	}
	if _, err := os.Stat(strings.TrimSuffix(remoteArtifact, ".tar.gz") + ".publication.json"); !os.IsNotExist(err) {
		t.Fatalf("signing remote-style publication should not create non-canonical sidecar, err=%v", err)
	}
	verified, err := actions.VerifySyncPublicationSignature(ctx, remoteArtifact)
	if err != nil {
		t.Fatalf("verify remote-style publication: %v", err)
	}
	if verified.Signature.State != contracts.VerificationTrustedValid {
		t.Fatalf("remote-style publication should verify trusted, got %#v", verified.Signature)
	}
}

func TestRemoteStyleSyncPublicationIgnoresStalePublicationJSON(t *testing.T) {
	ctx := context.Background()
	_, actions, _, projectStore, _, _ := newImportExportHarness(t)
	seedSyncWorkspace(t, ctx, actions, projectStore)
	key := generateTrustedSigningKey(t, ctx, actions, "alice")

	first, err := actions.CreateSyncBundle(ctx, contracts.Actor("human:owner"), "create first sync bundle")
	if err != nil {
		t.Fatalf("create first sync bundle: %v", err)
	}
	second, err := actions.CreateSyncBundle(ctx, contracts.Actor("human:owner"), "create second sync bundle")
	if err != nil {
		t.Fatalf("create second sync bundle: %v", err)
	}
	remoteDir := t.TempDir()
	firstArtifact := filepath.Join(remoteDir, first.Publication.ArtifactName)
	for _, pair := range [][2]string{
		{first.Job.BundleRef, firstArtifact},
		{strings.TrimSuffix(first.Job.BundleRef, ".tar.gz") + ".manifest.json", strings.TrimSuffix(firstArtifact, ".tar.gz") + ".manifest.json"},
		{strings.TrimSuffix(first.Job.BundleRef, ".tar.gz") + ".sha256", strings.TrimSuffix(firstArtifact, ".tar.gz") + ".sha256"},
	} {
		if err := copySyncFile(pair[0], pair[1]); err != nil {
			t.Fatalf("copy first sync sidecar %s -> %s: %v", pair[0], pair[1], err)
		}
	}
	if err := writeSyncPublication(filepath.Join(remoteDir, "publication.json"), second.Publication); err != nil {
		t.Fatalf("write stale remote publication: %v", err)
	}

	unsigned, err := actions.VerifySyncPublicationSignature(ctx, firstArtifact)
	if err != nil {
		t.Fatalf("verify first artifact with stale publication: %v", err)
	}
	if unsigned.Signature.State != contracts.VerificationMissingSignature || unsigned.Signature.ArtifactUID != first.Publication.BundleID {
		t.Fatalf("stale publication.json should not bind to first artifact, got %#v", unsigned.Signature)
	}
	if _, err := actions.SignSyncPublication(ctx, firstArtifact, key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "sign first remote artifact"); err != nil {
		t.Fatalf("sign first artifact with stale publication present: %v", err)
	}
	if _, err := os.Stat(strings.TrimSuffix(firstArtifact, ".tar.gz") + ".publication.json"); err != nil {
		t.Fatalf("signing first artifact should write artifact-specific publication: %v", err)
	}
	stale, err := readSyncPublication(filepath.Join(remoteDir, "publication.json"))
	if err != nil {
		t.Fatalf("read stale publication: %v", err)
	}
	if stale.BundleID != second.Publication.BundleID || len(stale.SignatureEnvelopes) != 0 {
		t.Fatalf("stale directory publication should stay untouched, got %#v", stale)
	}
}

func generateTrustedSigningKey(t *testing.T, ctx context.Context, actions *ActionService, collaboratorID string) KeyDetailView {
	t.Helper()
	key, err := actions.GenerateKey(ctx, KeyGenerateOptions{Scope: contracts.KeyScopeCollaborator, OwnerID: collaboratorID}, contracts.Actor("human:owner"), "create signing key")
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if _, err := actions.BindTrust(ctx, collaboratorID, key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "trust signing key"); err != nil {
		t.Fatalf("bind trust: %v", err)
	}
	return key
}

func copySyncBundleSidecars(t *testing.T, fromArtifact string, toArtifact string) {
	t.Helper()
	pairs := [][2]string{
		{fromArtifact, toArtifact},
		{strings.TrimSuffix(fromArtifact, ".tar.gz") + ".manifest.json", strings.TrimSuffix(toArtifact, ".tar.gz") + ".manifest.json"},
		{strings.TrimSuffix(fromArtifact, ".tar.gz") + ".sha256", strings.TrimSuffix(toArtifact, ".tar.gz") + ".sha256"},
	}
	for _, pair := range pairs {
		if err := copySyncFile(pair[0], pair[1]); err != nil {
			t.Fatalf("copy sync sidecar %s -> %s: %v", pair[0], pair[1], err)
		}
	}
}
