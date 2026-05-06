# Collaboration Model

v1.6 extends Atlas from single-workspace local orchestration into deterministic multi-workspace collaboration.

Core rules:
- collaborator identity is explicit and stable
- membership feeds the existing permission-profile system; it does not replace it
- mentions are canonical immutable records extracted from supported write surfaces
- inbox items remain derived, not stored
- removed collaborators and unbound memberships are tombstoned, not deleted
- collaborator trust is Atlas-local trust, not cryptographic authorship

Canonical mention syntax is only `@<collaborator_id>`.
Provider handles are not canonical mention syntax in v1.6.
