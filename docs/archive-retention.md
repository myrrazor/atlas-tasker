# Archive and Retention

## Archive vs compact
- archive: move cold Atlas-owned data to a retained archive structure
- compact: reduce storage footprint of derived or safely archived material

## Archiveable targets
- runtime bundles
- copied evidence artifacts
- handoff exports
- logs
- export bundles
- archive bundles
- derived verification scratch

## Compactable targets
- derived caches
- generated launch files
- archived copied artifacts when policy allows
- archived scratch and projection artifacts

## Never deleted or compacted in v1.5
- canonical event history
- canonical ticket, project, run, gate, handoff, evidence, change, and check snapshots
- import and export job records

## Restore rules
- Atlas-owned copy-back only
- no worktree recreation
- no runtime side-effect recreation
- no provider-state recreation
- conflicts block by default

## Precedence and audit
- workspace policy is the default
- project policy may override by target
- item-level preservation markers can only preserve more, not less
- `archive plan` is always dry-run and exact
- `archive apply` is always auditable and journaling-aware

## Git hygiene
- keep archive metadata markdown under `.tracker/archives/*.md` if you want archive state reviewable in Git
- ignore archive payload directories under `.tracker/archives/<ARCHIVE-ID>/`
- keep export bundle metadata markdown under `.tracker/exports/*.md` if you want export history reviewable in Git
- ignore generated export artifacts, manifests, and checksum sidecars under `.tracker/exports/`
