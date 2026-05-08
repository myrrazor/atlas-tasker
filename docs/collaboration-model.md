# Collaboration Model

v1.6 extends Atlas from single-workspace local orchestration into deterministic multi-workspace collaboration.

Core rules:
- collaborator identity is explicit and stable
- collaborator IDs must match `^[A-Za-z][A-Za-z0-9_-]{0,63}$`; path-like IDs are rejected
- membership feeds the existing permission-profile system; it does not replace it
- mentions are canonical immutable records extracted from supported write surfaces
- inbox items remain derived, not stored
- removed collaborators and unbound memberships are tombstoned, not deleted
- collaborator trust is Atlas-local trust, not cryptographic authorship

Canonical mention syntax is only `@<collaborator_id>`.
Provider handles are not canonical mention syntax in v1.6.

For reviewer quorum workflows, add collaborators, map their Atlas actors, bind them to the project with `--role reviewer`, then approve the generated review gate with `tracker gate approve <GATE-ID> --actor <ACTOR> --reason <TEXT>`. `tracker ticket approve` remains a single assigned-reviewer convenience command rather than a multi-reviewer quorum collector.

For "review ticket of another ticket" work, keep the review-task assignee and reviewer distinct. The simple pattern is:

```bash
tracker ticket request-review REVIEW-1 --reviewer human:owner --actor agent:reviewer-1 --reason "meta-review ready"
tracker ticket approve REVIEW-1 --actor human:owner --reason "owner accepted review"
```

If the meta-review is intentionally owner-gated or open-completion work, set that completion policy explicitly. Do not make the review-task assignee approve their own ticket; Atlas rejects assignee/reviewer self-approval.
