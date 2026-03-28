# Change Lifecycle

## Canonical rule
Atlas owns workflow truth. Provider state only enriches the local change record.

## Persisted states
- `local_only`
- `draft`
- `open`
- `review_requested`
- `approved`
- `changes_requested`
- `merge_ready`
- `merged`
- `closed`
- `superseded`
- `external_drifted`

`unlinked` is a virtual readiness state, not a persisted `ChangeRef`.

## Allowed transitions
```text
local_only -> draft | open | closed | superseded | external_drifted
draft -> open | review_requested | closed | superseded | external_drifted
open -> review_requested | approved | changes_requested | merge_ready | closed | superseded | external_drifted
review_requested -> approved | changes_requested | merge_ready | closed | superseded | external_drifted
approved -> changes_requested | merge_ready | merged | closed | superseded | external_drifted
changes_requested -> draft | open | review_requested | closed | superseded | external_drifted
merge_ready -> merged | changes_requested | closed | superseded | external_drifted
external_drifted -> open | review_requested | approved | changes_requested | merge_ready | merged | closed | superseded
```

## Terminal states
- `merged`
- `closed`
- `superseded`

## Local vs provider-backed states
- local-only: `local_only`, `superseded`
- provider-synced: `draft`, `open`, `review_requested`, `approved`, `changes_requested`, `merge_ready`, `merged`, `closed`

## Provider boundary
- Provider reads happen only in explicit live status or sync commands.
- Provider writes happen only in explicit live commands.
- Replay, repair, reindex, archive planning, and import preview never change provider state.
- If provider state conflicts with an incompatible local state, Atlas marks the change `external_drifted` until explicit reconcile.
