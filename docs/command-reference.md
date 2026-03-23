# Atlas Tasker v1.2 Command Reference

## Top-Level

- `tracker init`
- `tracker help`
- `tracker doctor [--repair]`
- `tracker reindex`
- `tracker inspect <ID> [--actor <ACTOR>]`
- `tracker templates list`
- `tracker templates view <NAME>`
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

## Shell Mode

- `tracker shell`

Slash command examples:

- `/project create APP "App Project"`
- `/ticket create --project APP --title "Task" --type task`
- `/ticket move APP-1 in_review --actor agent:builder-1`
- `/ticket history APP-1`
- `/board`

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
