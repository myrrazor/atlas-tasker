# Command Reference

The full command inventory lives in [../command-reference.md](../command-reference.md). This page is the short map for first-time readers.

Core read commands:

- `tracker queue --actor <ACTOR>`
- `tracker next --actor <ACTOR>`
- `tracker board`
- `tracker dashboard`
- `tracker inspect <TICKET-ID> --actor <ACTOR>`
- `tracker ticket history <TICKET-ID> --json`
- `tracker search 'project=AUTH text~logout flow' --json`

Core write commands:

- `tracker project create <KEY> <NAME>`
- `tracker ticket create --project <KEY> --title <TITLE> --type task --actor <ACTOR> --reason <TEXT>`
- `tracker ticket move <TICKET-ID> <STATUS> --actor <ACTOR> --reason <TEXT>`
- `tracker ticket comment <TICKET-ID> --body <TEXT> --actor <ACTOR> --reason <TEXT>`

Agent workflow commands:

- `tracker agent create <AGENT-ID> --name <NAME> --provider <PROVIDER> --capability <CAPABILITY> --actor <ACTOR> --reason <TEXT>`
- `tracker agent available <AGENT-ID> --json`
- `tracker agent pending <AGENT-ID> --json`
- `tracker run dispatch <TICKET-ID> --agent <AGENT-ID> --actor <ACTOR> --reason <TEXT>`
- `tracker run checkpoint <RUN-ID> --title <TITLE> --body <TEXT> --actor <ACTOR> --reason <TEXT>`
- `tracker run evidence add <RUN-ID> --type note --title <TITLE> --body <TEXT> --actor <ACTOR> --reason <TEXT>`
- `tracker run handoff <RUN-ID> --next-actor <ACTOR> --next-gate review --actor <ACTOR> --reason <TEXT>`

GitHub commands (need the `gh` CLI installed and authenticated; `tracker gh status` tells you whether it is):

- `tracker gh status --json`
- `tracker gh prs <TICKET-ID> --json`
- `tracker gh view <PR-NUMBER|URL> --json`
- `tracker gh checks <TICKET-ID|PR-REF> --json`
- `tracker gh create-pr <TICKET-ID> [--title <TITLE>] [--body <TEXT>] [--base <BRANCH>] [--draft] --actor <ACTOR> --reason <TEXT>`
- `tracker gh request-review <TICKET-ID|CHANGE-ID> --actor <ACTOR> --reason <TEXT>`
- `tracker gh import-url <TICKET-ID> --url <PR-URL> --actor <ACTOR> --reason <TEXT>` (alias of `tracker change import-url`)

For discovery, the CLI is still the source of truth:

```bash
tracker --help
tracker ticket --help
tracker run --help
tracker goal --help
tracker mcp --help
tracker version --json
```
