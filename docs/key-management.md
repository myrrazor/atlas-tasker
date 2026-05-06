# Key Management

Atlas v1.7 uses Go standard-library Ed25519 for signing. Private key bytes are local-only and must never sync, export, bundle, log, appear in JSON errors, or appear in `TEST_STDOUT.log`.

## Storage

- public keys: `.tracker/security/keys/public/*.md`
- private keys: `.tracker/security/keys/private/*`
- revocations: `.tracker/security/revocations/*.md`
- signatures: `.tracker/security/signatures/*.json`
- local trust bindings: `.tracker/security/trust/*`

Private key files default to mode `0600` on Unix. Atlas should warn or fail when permissions are broader, depending on command risk. Windows or unsupported permission semantics should report an explicit warning instead of pretending the check succeeded.

## Lifecycle

Keys move through `generated`, `active`, `rotated`, `revoked`, `expired`, `lost`, `imported`, and `disabled`. Revocation is terminal. Rotation keeps historical verification material and verifies old signatures with `valid_rotated_key`. Expiration blocks new authority but can verify old signatures with `valid_expired_key`.

## Export Policy

`tracker key export-private` is not part of v1.7. Public key export is allowed. Private key backup remains an operator responsibility unless a later encrypted/manual export flow is approved.
