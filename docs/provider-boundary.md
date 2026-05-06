# Provider Boundary

## Canonical truth
Atlas snapshots and events remain canonical. Provider data is enrichment only.

## Explicit live operations only
Provider reads and writes happen only in explicit live commands.

## Never allowed during replay-oriented flows
These flows must never call external providers or recreate live side effects:
- replay
- reindex
- repair
- archive plan
- import preview
- mixed-fixture upgrade paths

## Drift handling
If provider state disagrees with Atlas-local state in a way that cannot be safely reconciled forward, Atlas records drift and requires explicit reconcile.
