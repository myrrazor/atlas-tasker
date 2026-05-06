# Audit Model

Audit reports are derived snapshot artifacts. The event log remains canonical truth.

An audit report includes scope, event range, generator, generated timestamp, policy snapshot hash, trust snapshot hash, included artifact hashes, findings, redaction state, and optional signatures.

Workspace-scoped reports use `scope_kind: workspace` and omit `scope_id`. Project, ticket, run, change, release, and incident reports must include the concrete `scope_id` they snapshot.

Verification checks the audit packet snapshot, not current live workspace meaning. Later policy, trust, or classification changes may affect current governance decisions without rewriting an old audit packet.

Audit packets can be exported and signed. Tampering with included hashes, signatures, or packet metadata must produce a failed verification state.
