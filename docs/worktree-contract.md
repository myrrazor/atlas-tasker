# Worktree Contract

Config:
- `execution.worktrees.enabled`
- `execution.worktrees.root`
- `execution.worktrees.default_mode`
- `execution.worktrees.auto_prune`
- `execution.worktrees.require_clean_main`

Default path:
- `<repo-parent>/.atlas-tasker-worktrees/<repo-name>/<ticket-id>-<run-id>`

Rules:
- worktree creation happens only after run snapshot persistence
- cleanup never runs during replay or repair
- dirty worktrees require `cleanup --force`
- manual on-disk deletion is drift, not implicit success
- prune only touches orphaned or cleaned-up runs
- repair reports drift and re-syncs metadata only
