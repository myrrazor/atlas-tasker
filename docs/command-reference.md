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

- `tracker board`
- `tracker backlog`
- `tracker next [--actor <ACTOR>]`
- `tracker blocked`
- `tracker queue [--actor <ACTOR>]`
- `tracker review-queue [--actor <ACTOR>]`
- `tracker owner-queue`
- `tracker who`
- `tracker search <QUERY>`
- `tracker render <ID>`

## Maintenance

- `tracker sweep`
- `tracker doctor --repair`
- `tracker inspect <ID>`

## Notify

- `tracker notify send --event-type <TYPE> [--ticket <ID>] [--project <KEY>] [--actor <ACTOR>] [--reason <TEXT>]`
- `tracker notify log [--limit <N>]`
- `tracker notify dead-letter [--limit <N>]`

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
