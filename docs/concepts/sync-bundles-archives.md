# Sync, Bundles, And Archives

Atlas is local-first. Sync and bundle flows move Atlas-owned state between workspaces without turning Atlas into a hosted service.

- Export bundles are portable snapshots for sharing or backup-like handoff.
- Sync publications are workspace-to-workspace exchange records.
- Archives retain older Atlas artifacts according to retention plans.

Start with read-only or plan commands:

```bash
tracker sync status
tracker bundle verify <PATH>
tracker archive plan
```

High-impact writes such as import apply, sync pull/push, archive apply/restore, and compact/cleanup flows should be treated as protected operations. In MCP mode they belong behind strict profiles and approval mechanisms, not default read access.
