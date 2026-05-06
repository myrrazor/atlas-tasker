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

Protected actions include sync/bundle import apply, gate approve/waive, ticket/run complete, change merge, export create, archive/backup restore, trust/revoke key, redaction override, and owner override.
