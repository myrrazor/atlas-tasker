# Atlas Tasker Operator Manual

Atlas Tasker is local-first. The normal operator loop is:

1. `tracker init`
2. create a project and tickets
3. claim work, move it, review it, and complete it
4. use `queue`, `next`, `watch`, `automation`, and `tui` to stay on top of the workspace

## Common flows

Create and claim work:

```bash
tracker project create APP "App Project"
tracker ticket create --project APP --title "Fix auth race" --type task --actor human:owner
tracker ticket move APP-1 ready --actor human:owner
tracker ticket claim APP-1 --actor agent:builder-1
```

Review flow:

```bash
tracker ticket request-review APP-1 --actor agent:builder-1
tracker ticket approve APP-1 --actor agent:reviewer-1
tracker ticket complete APP-1 --actor human:owner
```

Saved views and watchers:

```bash
tracker views save my-queue --kind queue --actor agent:builder-1 --queue-category ready_for_me
tracker watch ticket APP-1 --actor human:owner
tracker watch view my-queue --actor agent:builder-1 --event ticket.review_requested
```

Bulk work:

```bash
tracker bulk move in_progress --view my-queue --dry-run --actor agent:builder-1
tracker bulk move in_progress --view my-queue --yes --actor agent:builder-1
```

## TUI

Launch:

```bash
tracker tui --actor agent:builder-1
```

Useful tabs:
- `Queues`: current operator work
- `Inbox`: recent notification deliveries and dead letters
- `Views`: saved views you can load directly into the active read tabs
- `Ops`: automation summary plus bulk preview/apply state

Useful keys:
- `/` command palette
- `b` bulk preview for the current ticket list
- `y` apply the last bulk preview
- `c` claim/release selected ticket
- `p` request review
- `v` approve
- `d` complete

## Recovery

If the projection is stale or corrupted:

```bash
tracker doctor --repair
tracker reindex
```

If a ticket looks wrong, inspect it directly:

```bash
tracker inspect APP-1 --actor human:owner --json
tracker ticket history APP-1 --json
```
