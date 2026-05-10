# Atlas Trust Model

Atlas v1.7 separates authenticity from authority.

Authenticity answers: did this artifact verify against a known public key? Authority answers: was the actor allowed to perform the protected action under Atlas governance, collaborator membership, permission profiles, gates, quorum, and separation-of-duties?

## Trust Records

- Public key records are syncable claims. They do not create trust.
- Revocation records are syncable because teams need to converge on compromised or retired keys.
- Trust bindings are local by default. Importing someone else's trust decision must be an explicit future flow, not a side effect of sync.
- Trust binds to both the collaborator/workspace owner and key fingerprint. A display name is never authority.

## Key States

Frozen key states are `generated`, `active`, `rotated`, `revoked`, `expired`, `lost`, `imported`, and `disabled`.

Revoked keys do not become active again. Rotated and expired keys can verify old signatures with warning states. Protected actions evaluate trust at action time, so a previously valid approval may stop satisfying current quorum after revocation, suspension, or policy changes.

## Security Claim

Atlas can claim signed artifacts verify against trusted local keys. Atlas does not claim SaaS-grade identity proof or protection from a malicious local user with filesystem access.
