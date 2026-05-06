# Key Management

Atlas v1.7 uses Go standard-library Ed25519 for signing. Private key bytes are local-only and must never sync, export, bundle, log, appear in JSON errors, or appear in `TEST_STDOUT.log`.

## Storage

- public keys: `.tracker/security/keys/public/*.md`
- private keys: `.tracker/security/keys/private/*`
- revocations: `.tracker/security/revocations/*.md`
- signatures: `.tracker/security/signatures/*.json`
- local trust bindings: `.tracker/security/trust/*`

Private key files default to mode `0600` on Unix. Atlas accepts stricter owner-only readable modes such as `0400`, and fails when POSIX group/other permission bits are present. Windows or unsupported permission semantics report `private_key_permissions_unverified` instead of pretending the check succeeded.

## Lifecycle

Keys move through `generated`, `active`, `rotated`, `revoked`, `expired`, `lost`, `imported`, and `disabled`. Revocation is terminal. Rotation keeps historical verification material and verifies old signatures with `valid_rotated_key`. Expiration blocks new authority even if a synced record still says `active`, but old signatures verify with `valid_expired_key`.

## Commands

PR-702 implements the local key and trust ceremony:

- `tracker key generate --scope <workspace|collaborator|admin|release> --actor human:owner --reason "create signing key"` creates an active Ed25519 signing key and writes private material only under `.tracker/security/keys/private/`.
- `tracker key list|view|verify` report public key records and private-key health without printing private bytes.
- `tracker key export-public <KEY-ID>` emits the public key record for another workspace or collaborator to inspect.
- `tracker key import-public <PATH> --actor ... --reason ...` imports a public key as `manual_import`/`imported`; it does not trust the key and cannot sign without local private material.
- `tracker key rotate <KEY-ID>` blocks future signing with that key while preserving old-signature verification.
- `tracker key revoke <KEY-ID>` is terminal and writes a syncable revocation record.
- `tracker trust bind-key <COLLABORATOR-ID> <KEY-ID> --actor ... --reason ...` creates a local-only trust binding after confirming the key record is owned by that collaborator.
- `tracker trust revoke-key <KEY-ID> --actor ... --reason ...` revokes local trust without deleting historical trust evidence.
- `tracker trust status|list|collaborator|explain` give operators a quick view of trusted, revoked, local-private, and imported-untrusted material.

Public key import refuses to overwrite an existing local private-backed key record. Reimporting already-imported public material is allowed so sync/import flows can converge without weakening local signing material.

Trust commands still require actor and reason, but they write local trust records only. They do not append syncable event-log entries because local trust decisions are not team claims.

## Export Policy

`tracker key export-private` is not part of v1.7. Public key export is allowed. Private key backup remains an operator responsibility unless a later encrypted/manual export flow is approved.
