# Signature Format

Every signed Atlas artifact uses a `SignatureEnvelope` over `atlas-c14n-v1` canonical bytes. The signed preimage includes the artifact kind, artifact UID, and canonical payload, so a signature cannot be replayed onto a different artifact or trust scope. Embedded `signature_envelopes` are verification metadata and are excluded from canonical payload bytes, so adding or refreshing signatures does not change the payload hash being verified.

## Envelope Fields

The envelope records artifact kind, artifact UID, canonicalization version, SHA-256 payload hash, signing time, signer kind/id, public key id/fingerprint, algorithm `ed25519`, signature bytes, schema version, and computed verification state.

Frozen artifact kinds are `bundle`, `sync_publication`, `approval`, `handoff`, `evidence_packet`, `audit_report`, `audit_packet`, `backup_snapshot`, and `goal_manifest`. Trust scopes map one-to-one for those signed families: `bundle`, `sync`, `approval`, `handoff`, `evidence`, `audit_report`, `audit_packet`, `backup`, and `goal`. `release` is a trust scope for release governance, not a v1.7 signed artifact kind.

Verification states are frozen:

- `trusted_valid`
- `valid_untrusted`
- `valid_unknown_key`
- `valid_revoked_key`
- `valid_expired_key`
- `valid_rotated_key`
- `invalid_signature`
- `missing_signature`
- `malformed_signature`
- `payload_hash_mismatch`
- `canonicalization_mismatch`
- `unsupported_signature_version`

## Verification Semantics

Verification is side-effect free by default. `tracker verify ...`, status, replay, reindex, repair, and doctor checks must not append events. A future recorded verification workflow must be explicit and non-default.

Signatures prove authenticity relative to a key. They do not prove that the signer had authority.

## PR-703 Signed Artifacts

PR-703 enables explicit signatures for export bundles and sync publications:

- `tracker sign bundle <BUNDLE-ID> [--signing-key <KEY-ID>]` signs the export bundle manifest/checksum payload after archive integrity passes.
- `tracker verify bundle <BUNDLE-ID|PATH>` verifies export bundle integrity and reports the signature state without appending events.
- `tracker sign sync-publication <BUNDLE-ID|PATH> [--signing-key <KEY-ID>]` signs the sync publication metadata after sync bundle integrity passes.
- `tracker verify sync-publication <BUNDLE-ID|PATH>` verifies sync bundle integrity and reports the publication signature state without appending events.

Export bundle signatures are persisted in the local security signature store and in an adjacent `<bundle>.signatures.json` sidecar. Path-based verification loads that sidecar and derives the signed bundle identity from the manifest, so a copied artifact set can verify without the source workspace's export metadata. Sync publication signatures live in the matching publication metadata; Atlas ignores a directory-level `publication.json` when it does not name the requested archive.

Unsigned artifacts remain readable and verify as `missing_signature` at the signature layer. Governance in PR-704 decides when that state is acceptable.

PR-706 extends the same envelope and trust-state machinery to approval gates, handoffs, evidence packets, audit reports, and audit packets:

- `approval`, `handoff`, and `evidence_packet` signatures are standalone records under `.tracker/security/signatures/`; the source gate/handoff/evidence document is not rewritten.
- `audit_report` signatures are embedded in the stored report and also persisted in the signature store.
- `audit_packet` signatures are embedded in the stored packet and also persisted in the signature store. Packets do not embed the source report's own signature envelopes.
- audit packet verification recomputes `packet_hash` from the canonical report payload before reporting signature state.

Approval/handoff/evidence signatures make those artifacts authenticatable, but PR-706 does not require every workflow to be signed by default. Governance policy remains the authority layer.
