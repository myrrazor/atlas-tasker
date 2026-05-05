# Event Semantics

## Source of truth
The event log is append-only audit history. It explains how markdown state changed, but the current canonical state is the combination of markdown snapshots plus the event stream.

## Required event properties today
- positive `event_id`
- UTC `timestamp`
- valid `actor`
- valid `type`
- non-empty `project`
- `schema_version`

## Planned metadata before automation
- `correlation_id`
- `causation_event_id`
- `mutation_id`
- `surface`
- `batch_id`
- `root_actor`
