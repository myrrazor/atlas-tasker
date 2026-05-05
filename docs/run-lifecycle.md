# Run Lifecycle

Statuses:
- `planned`
- `dispatched`
- `attached`
- `active`
- `handoff_ready`
- `awaiting_review`
- `awaiting_owner`
- `completed`
- `failed`
- `aborted`
- `cleaned_up`

Transitions:
- `planned -> dispatched | aborted`
- `dispatched -> attached | active | failed | aborted`
- `attached -> active | failed | aborted`
- `active -> handoff_ready | awaiting_review | awaiting_owner | failed | aborted`
- `handoff_ready -> active | awaiting_review | awaiting_owner | completed | failed | aborted`
- `awaiting_review -> active | handoff_ready | awaiting_owner | completed | failed | aborted`
- `awaiting_owner -> active | handoff_ready | completed | failed | aborted`
- `completed -> cleaned_up`
- `failed -> cleaned_up`
- `aborted -> cleaned_up`

Rules:
- one active run per ticket by default
- parallel runs require `allow_parallel_runs=true`
- cleanup is only valid from completed, failed, or aborted runs
- `run open` is read-only and never creates or rewrites runtime files
- `run launch` writes run-scoped artifacts under `.tracker/runtime/<run-id>/` and is idempotent unless `--refresh` is used
- `run attach` records an external session binding and rejects conflicting double-attachments unless explicitly replaced
