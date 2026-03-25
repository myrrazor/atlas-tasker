# Atlas Tasker v1.3 Command Reference

## Top-Level

- `tracker init`
- `tracker help`
- `tracker doctor [--repair]`
- `tracker reindex`
- `tracker inspect <ID> [--actor <ACTOR>]`
- `tracker automation list`
- `tracker automation view <NAME>`
- `tracker automation create <NAME> [flags]`
- `tracker automation edit <NAME> [flags]`
- `tracker automation delete <NAME>`
- `tracker automation dry-run <NAME> [--ticket <ID>] [--event-type <TYPE>] [--actor <ACTOR>]`
- `tracker automation explain <NAME> [--ticket <ID>] [--event-type <TYPE>] [--actor <ACTOR>]`
- `tracker notify send --event-type <TYPE> [--ticket <ID>] [--project <KEY>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker notify log [--limit <N>]`
- `tracker notify dead-letter [--limit <N>]`
- `tracker git status`
- `tracker git branch-name <ID>`
- `tracker git refs <ID>`
- `tracker git commit <ID> --message <TEXT>`
- `tracker gh status`
- `tracker gh prs <ID>`
- `tracker gh create-pr <ID> [--title <TEXT>] [--body <TEXT>] [--base <BRANCH>] [--draft] [--actor <ACTOR>]`
- `tracker gh import-url <ID> --url <URL> [--actor <ACTOR>]`
- `tracker views list`
- `tracker views view <NAME>`
- `tracker views save <NAME> --kind <board|search|queue|next> [flags]`
- `tracker views delete <NAME>`
- `tracker views run <NAME> [--actor <ACTOR>]`
- `tracker watch list [--actor <ACTOR>]`
- `tracker watch ticket <ID> [--actor <ACTOR>] [--event <TYPE>]`
- `tracker watch project <KEY> [--actor <ACTOR>] [--event <TYPE>]`
- `tracker watch view <NAME> [--actor <ACTOR>] [--event <TYPE>]`
- `tracker unwatch ticket <ID> [--actor <ACTOR>]`
- `tracker unwatch project <KEY> [--actor <ACTOR>]`
- `tracker unwatch view <NAME> [--actor <ACTOR>]`
- `tracker bulk move <STATUS> [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>]`
- `tracker bulk assign <ACTOR> [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>]`
- `tracker bulk request-review [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>]`
- `tracker bulk complete [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>]`
- `tracker bulk claim [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>]`
- `tracker bulk release [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>]`
- `tracker templates list`
- `tracker templates view <NAME>`
- `tracker integrations install codex [--force]`
- `tracker integrations install claude [--force]`
- `tracker tui [--actor <ACTOR>]`
- `tracker config get [KEY]`
- `tracker config set <KEY> <VALUE>`

## Project

- `tracker project create <KEY> <NAME>`
- `tracker project list`
- `tracker project view <KEY>`
- `tracker project policy get <KEY>`
- `tracker project policy set <KEY> [flags]`

## Ticket CRUD

- `tracker ticket create --project <KEY> --title <TEXT> [--type <epic|task|bug|subtask>] [--template <NAME>] [flags]`
- `tracker ticket view <ID>`
- `tracker ticket edit <ID> [flags]`
- `tracker ticket delete <ID>`
- `tracker ticket list [--project <KEY>] [--status <STATUS>] [--assignee <ACTOR>] [--type <TYPE>]`

## Ticket Mutation

- `tracker ticket move <ID> <STATUS>`
- `tracker ticket assign <ID> <ACTOR>`
- `tracker ticket priority <ID> <PRIORITY>`
- `tracker ticket label add <ID> <LABEL>`
- `tracker ticket label remove <ID> <LABEL>`
- `tracker ticket claim <ID> [--actor <ACTOR>]`
- `tracker ticket release <ID> [--actor <ACTOR>]`
- `tracker ticket heartbeat <ID> [--actor <ACTOR>]`
- `tracker ticket request-review <ID> [--actor <ACTOR>]`
- `tracker ticket approve <ID> [--actor <ACTOR>]`
- `tracker ticket reject <ID> --reason <TEXT> [--actor <ACTOR>]`
- `tracker ticket complete <ID> [--actor <ACTOR>]`
- `tracker ticket policy get <ID>`
- `tracker ticket policy set <ID> [flags]`

## Relationships

- `tracker ticket link <ID> --blocks <OTHER_ID>`
- `tracker ticket link <ID> --blocked-by <OTHER_ID>`
- `tracker ticket link <ID> --parent <PARENT_ID>`
- `tracker ticket unlink <ID> <OTHER_ID>`

## Comments and History

- `tracker ticket comment <ID> --body <TEXT>`
- `tracker ticket history <ID>`

## Views

- `tracker board [--view <NAME>]`
- `tracker backlog`
- `tracker next [--actor <ACTOR>] [--view <NAME>]`
- `tracker blocked`
- `tracker queue [--actor <ACTOR>] [--view <NAME>]`
- `tracker review-queue [--actor <ACTOR>]`
- `tracker owner-queue`
- `tracker who`
- `tracker search <QUERY>`
- `tracker search --view <NAME>`
- `tracker render <ID>`

## Saved Views

- `tracker views list`
- `tracker views view <NAME>`
- `tracker views save <NAME> --kind <board|search|queue|next> [--title <TEXT>] [--project <KEY>] [--assignee <ACTOR>] [--type <TYPE>] [--actor <ACTOR>] [--query <QUERY>] [--column <STATUS>] [--queue-category <CATEGORY>]`
- `tracker views delete <NAME>`
- `tracker views run <NAME> [--actor <ACTOR>]`

## Watchers

- `tracker watch list [--actor <ACTOR>]`
- `tracker watch ticket <ID> [--actor <ACTOR>] [--event <TYPE>]`
- `tracker watch project <KEY> [--actor <ACTOR>] [--event <TYPE>]`
- `tracker watch view <NAME> [--actor <ACTOR>] [--event <TYPE>]`
- `tracker unwatch ticket <ID> [--actor <ACTOR>]`
- `tracker unwatch project <KEY> [--actor <ACTOR>]`
- `tracker unwatch view <NAME> [--actor <ACTOR>]`

## Bulk Operations

- `tracker bulk move <STATUS> [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker bulk assign <ACTOR> [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker bulk request-review [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker bulk complete [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker bulk claim [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker bulk release [--ticket <ID>]... [--view <NAME>] [--dry-run|--yes] [--actor <ACTOR>] [--reason <TEXT>]`

Rules:

- `--dry-run` previews the batch without mutating anything
- live bulk mutations require `--yes`
- `--ticket` may be repeated
- `--view` expands any saved board/search/queue/next view into ticket IDs
- duplicate ticket IDs are deduplicated before the batch runs
- every committed per-ticket event carries the same `metadata.batch_id`

## Maintenance

- `tracker sweep`
- `tracker doctor --repair`
- `tracker inspect <ID>`

## Notify

- `tracker notify send --event-type <TYPE> [--ticket <ID>] [--project <KEY>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker notify log [--limit <N>]`
- `tracker notify dead-letter [--limit <N>]`

## Git

- `tracker git status`
- `tracker git branch-name <ID>`
- `tracker git refs <ID>`
- `tracker git commit <ID> --message <TEXT>`

## GitHub

- `tracker gh status`
- `tracker gh prs <ID>`
- `tracker gh create-pr <ID> [--title <TEXT>] [--body <TEXT>] [--base <BRANCH>] [--draft] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker gh import-url <ID> --url <URL> [--actor <ACTOR>] [--reason <TEXT>]`

Notes:

- `tracker gh status` is capability-only and degrades cleanly when `gh` is missing or unauthenticated
- `tracker gh prs` is best-effort and returns no PRs when GitHub context is unavailable for the current workspace
- `tracker gh create-pr` requires a local git repo and a working `gh` session, then writes the created PR URL back into ticket history as a comment
- `tracker gh import-url` accepts GitHub issue and pull request URLs only, and records the reference in ticket history as a comment

## Automation

- `tracker automation list`
- `tracker automation view <NAME>`
- `tracker automation create <NAME> --on <EVENT_TYPE> [--on <EVENT_TYPE>] --action <ACTION> [--action <ACTION>] [flags]`
- `tracker automation edit <NAME> --on <EVENT_TYPE> [--on <EVENT_TYPE>] --action <ACTION> [--action <ACTION>] [flags]`
- `tracker automation delete <NAME>`
- `tracker automation dry-run <NAME> --event-type <EVENT_TYPE> [--ticket <ID>] [--actor <ACTOR>]`
- `tracker automation explain <NAME> --event-type <EVENT_TYPE> [--ticket <ID>] [--actor <ACTOR>]`

Supported automation actions:

- `comment:<TEXT>`
- `move:<STATUS>`
- `request_review`
- `notify:<TEXT>`

## Shell Mode

- `tracker shell`

Slash command examples:

- `/project create APP "App Project"`
- `/ticket create --project APP --title "Task" --type task`
- `/ticket move APP-1 in_review --actor agent:builder-1`
- `/ticket history APP-1`
- `/board`

## TUI Shortcuts

Once `tracker tui` is running:

- `/` opens the slash command palette
- `b` previews a bulk action against the current ticket list
- `y` applies the last bulk preview
- `n` opens the create-ticket form
- `e` edits the selected ticket
- `m` opens the move prompt
- `s` opens the assign prompt
- `l` opens the link prompt
- `u` opens the unlink prompt
- `c` toggles claim/release for the selected ticket
- `o` opens the comment prompt
- `p` requests review for the selected ticket
- `v` approves the selected ticket
- `x` opens the reject prompt
- `d` completes the selected ticket
- `tab` / `shift+tab` switch tabs
- `j` / `k` or arrow keys move the list cursor
- `enter` opens detail or submits the active dialog
- `esc` cancels the active dialog

TUI tabs:

- `Board`
- `Queues`
- `Detail`
- `Search`
- `Review`
- `Owner`
- `Inbox`
- `Views`
- `Ops`

## Common Flags

Read commands:

- `--pretty`
- `--md`
- `--json`

Mutating commands:

- `--actor <ACTOR>`
- `--reason <TEXT>`

Useful config keys:

- `workflow.completion_mode`
- `actor.default`
- `notifications.terminal`
- `notifications.file_enabled`
- `notifications.file_path`
- `notifications.webhook_url`
- `notifications.webhook_timeout_seconds`
- `notifications.webhook_retries`
- `notifications.delivery_log_path`
- `notifications.dead_letter_path`
