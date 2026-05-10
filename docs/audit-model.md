# Audit Model

Audit reports are derived snapshot artifacts. The event log remains canonical truth.

An audit report includes scope, event range, generator, generated timestamp, policy snapshot hash, trust snapshot hash, included artifact hashes, findings, redaction state, and optional signatures.

Workspace-scoped reports use `scope_kind: workspace` and omit `scope_id`. Project, ticket, run, change, release, and incident reports must include the concrete `scope_id` they snapshot.

Verification checks the audit packet snapshot, not current live workspace meaning. Later policy, trust, or classification changes may affect current governance decisions without rewriting an old audit packet.

Audit packets can be exported and signed. Tampering with included hashes, signatures, or packet metadata must produce a failed verification state.

PR-706 ships the first concrete audit surface:

- `tracker audit report --scope <SCOPE>` writes `.tracker/audit/reports/<id>.json` and records `audit.report.created`.
- `tracker audit list` and `tracker audit view <REPORT-ID>` read local report snapshots.
- `tracker audit export <REPORT-ID>` writes `.tracker/audit/packets/<id>.json` and records `audit.report.exported`.
- `tracker audit verify <REPORT-ID|PATH>` and `tracker verify audit[-packet] <REF>` are side-effect free.
- `tracker sign audit <REPORT-ID>` and `tracker sign audit-packet <PACKET-ID>` sign canonical audit payloads.

Audit report scope strings are `workspace`, `project:<KEY>`, `ticket:<ID>`, `run:<ID>`, `change:<ID>`, `release:<ID>`, or `incident:<ID>`. Ticket/run/change scopes resolve their project before event filtering where Atlas has enough local state.

Scoped reports hash related Atlas-owned artifacts, not only files whose path contains the top-level scope ID. For example, a ticket report includes the ticket file plus known run, gate, handoff, evidence, and change records for that ticket. Packet export events are recorded in the scoped project event stream when Atlas can resolve one, so later scoped reports can audit their own export history.

Audit packets bind `packet_hash` to the canonical report payload. Verification recomputes that hash and returns `packet_hash_mismatch` if a copied packet was edited. Exported packets intentionally omit nested report signatures; packet signatures authenticate the packet, while report signatures remain on stored reports. Signature verification still reports its normal state (`missing_signature`, `trusted_valid`, `payload_hash_mismatch`, and so on) independently of packet integrity.

`tracker audit explain-policy` accepts `event_uid`, not a bare numeric `event_id`. Numeric event IDs are allocated per project stream, so treating them as workspace-global would be misleading in multi-project workspaces.

PR-706 also enables the shared signature model for approval, handoff, and evidence artifacts through:

- `tracker sign approval <GATE-ID>` / `tracker verify approval <GATE-ID>`
- `tracker sign handoff <HANDOFF-ID>` / `tracker verify handoff <HANDOFF-ID>`
- `tracker sign evidence <EVIDENCE-ID>` / `tracker verify evidence <EVIDENCE-ID>`

Those signatures are stored in `.tracker/security/signatures/` and do not rewrite the source gate, handoff, or evidence document. Governance enforcement can consume the shared trust model, but PR-706 does not make every approval/handoff/evidence workflow require signatures by default.
