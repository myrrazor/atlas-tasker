# Web Board User Guide

Start Home from anywhere:

```bash
tracker
```

Single-workspace board from an initialized workspace:

```bash
tracker web serve --open
```

Useful options:

```bash
tracker web serve --project APP --actor human:owner --open
tracker web serve --read-only --no-browser
tracker web status
tracker web open
```

`--open` lands on `/board`. The server root `/` is a welcome overview with
per-project rollups and the latest ticket changes, `/schedule` is the week
timeline for one-time reminders and agent wakeups (the ticket drawer on the
board can set or clear a schedule too — see [scheduling](scheduling.md)), and
`/settings` shows web preferences read-only.

v1.16 Home create/attach and Kanban archive: [browser management](v1.16-browser-management.md).
The current browser stills show the v1.16 implementation in dark mode with synthetic tickets. The recorded Grok and shared-agent demos retain their original v1.15 provenance.

Set the welcome display name with `tracker config set web.owner_name "User"`.
Set the workspace language with `tracker config set web.lang ja`; supported
values are `en`, `es`, `id`, `zh`, `ja`, and `ko`. Welcome, settings, and board
chrome use that preference. Schedule remains English-only, and ticket content
stays as written.

## Select a directory and create a project

On Home, **Initialize board** creates a workspace. Pick the Atlas boards
location (default) or another authorized root, then a relative folder. Optional
project key and name; blank derives a key from the folder when default projects
are on.

To attach an existing Atlas workspace, **Find existing** and choose it under an
authorized root, or preview an existing absolute directory, confirm the exact
path and purpose, then attach. Preview does not write `.tracker`.

Home init does not register coding agents. Run `tracker init` in the project
for MCP/skills, then restart the client.

From a workspace overview you can still create additional projects through the
same `ActionService` and CSRF/session/read-only gates as the CLI.

## Daily ticket work

The board mirrors Atlas workflow rules. If a dependency, reviewer gate, owner
gate, or read-only mode blocks an action, the page shows the service error
instead of bypassing policy. Dropping a card on the column it is already in is
a no-op on the web board, and the CLI/MCP/bulk paths match that behavior.

Drag cards between columns when JavaScript is enabled. The same moves are
available through card and detail actions for keyboard and non-drag users.

In the ticket drawer you can:

- Create and edit title, description, acceptance, labels, priority
- Assign and set a reviewer (`human:` / `agent:` ids; registered agents are
  offered in a datalist)
- Claim and release
- Comment
- Link or unlink blocks / blocked-by / parent
- Request review, approve, complete (policy unchanged)
- Cancel (status `canceled`, stays in Canceled — not archive)
- **Delete ticket** (same as `tracker ticket delete`; `ticket archive` is an
  alias). Confirm names the id and title. Markdown, events, and history stay.
  There is no restore and no purge.

Show archived tickets with Filters → Archived (`?archived=1`).

Assignee and reviewer fields keep typed values on error. Stale revision edits
conflict and keep the form. Read-only disables the buttons and leaves the board
visible.

`tracker web open` reuses the last recorded server and fails fast when that
server is no longer running. Opening the board in a browser that has no session
cookie for it requires the session URL printed by `tracker web serve`.
