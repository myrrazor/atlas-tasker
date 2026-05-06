# Signature Format

Every signed Atlas artifact uses a `SignatureEnvelope` over `atlas-c14n-v1` canonical bytes. Embedded `signature_envelopes` are verification metadata and are excluded from canonical signed bytes, so adding or refreshing signatures does not change the payload hash being verified.

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
