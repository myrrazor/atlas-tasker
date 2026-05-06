# Governance Policies

Governance extends the existing permission profile, collaborator, and membership model. It is not a second ACL.

## Precedence

Default precedence is:

1. explicit deny
2. protected-action rule
3. ticket/run/gate override
4. project policy
5. workspace default
6. explicit allow

Deny wins unless an allowed owner override policy applies. Owner override is itself a protected action and must be evented, reasoned, and signed when policy requires signatures.

## Quorum And Separation

Quorum evaluates root collaborator identity, not raw actor strings. Protected actions evaluate quorum at action time. Suspended or removed collaborators' historical approvals remain visible but do not satisfy active quorum unless a policy explicitly uses snapshot-at-approval semantics.

Separation-of-duties can block actors who implemented, dispatched, created a change, or approved related work from satisfying later protected actions.

## Protected Actions

Protected actions include sync/bundle/structured import apply, gate approve/waive, ticket/run complete, change merge, export create, archive apply/restore, backup restore, trust/revoke key, redaction override, and owner override.

## PR-704 Implementation Notes

PR-704 makes governance executable for the first protected write paths. Governance packs are stored under `.tracker/governance/packs/`; applied policies are stored under `.tracker/governance/policies/`. TOML files use the same snake_case field names as JSON output, and `tracker governance validate` exits non-zero when any pack or policy is invalid.

Commands:

- `tracker governance pack create <NAME>`
- `tracker governance pack apply <PACK-ID>`
- `tracker governance validate`
- `tracker governance explain <TARGET>`
- `tracker governance simulate <ACTION>`

The evaluator runs after the existing permission/collaborator checks and before live provider or durable filesystem side effects. Sync export/import governance runs before migration scaffolding writes, so denied operations do not stamp migration state. Remote sync pulls fetch into staging first, including Git fetch caches, then promote the fetched publication into the durable mirror only after `sync_import_apply` passes. Current protected hooks cover ticket completion, run completion, gate approval/waiver, change merge, structured import apply, sync/bundle import apply paths, sync push export creation, project-scoped archive apply/restore, and key trust/revoke operations.

Gate rejection is not governed by `gate_approve`. Atlas treats rejection as a safe negative decision in PR-704; a future `gate_reject` protected action can be added if teams need rejection-specific policy.

Trusted-signature requirements are deliberately not owner-overridable. PR-704 only accepts them for artifact import actions that already carry verifiable signature evidence: `bundle_import_apply` and `sync_import_apply`. Duplicate envelopes from the same trusted signer count once. A quorum rule with `require_trusted_signatures = true` counts those distinct trusted signer identities as its quorum evidence rather than looking for gate approvals. Ticket/run/change/gate signature policies wait for the signed approval and audit packet work in PR-706.

Quorum and separation failures can be owner-overridden only when every failed matching policy has an applicable override rule, the actor is `human:owner`, and the reason/signature requirements on each override rule are satisfied. The override then evaluates matching `owner_override` policies before it is accepted. Atlas records `governance.override.recorded` only after the protected mutation succeeds.

`sync_import_apply` and `bundle_import_apply` are intentionally separate. A remote `sync pull` must pass `sync_import_apply`; direct local bundle imports must pass `bundle_import_apply`. That lets teams protect live remote pulls differently from manual bundle files without accidental double enforcement.

`governance explain` and `governance simulate` accept `--reason`, approval actors, and trusted-signature counts so operators can model reason-required owner overrides before running a mutation.

`governance validate` is the recovery command for hand-edited policy files. It returns a structured validation report even when a policy or pack cannot be decoded into a valid contract.

Before PR-705 lands inherited classification labels, classification-scoped governance is intentionally narrow: legacy `protected` or `sensitive` tickets only match `classification:restricted`. Other classification scope ids do not match those legacy flags.

Applying a reusable pack to multiple scopes writes scope-bound policy ids, so `project:APP` and `project:WEB` applications of the same pack can remain active at the same time.
