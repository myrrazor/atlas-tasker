# Web Board

Atlas Tasker includes an optional local browser board:

```bash
tracker web serve --open
```

The web UI runs from the current workspace and uses the same canonical services as the CLI and TUI. It is a browser view over local Markdown snapshots, append-only JSONL events, and the SQLite projection; it is not a hosted server or separate database.

The root page is a workspace welcome view with per-project active, backlog, done, and blocked counts plus the latest ticket changes. Project links open `/board?project=KEY`; `/board` remains the canonical Kanban route. The settings link shows `web.owner_name`, `actor.default`, and agent color preferences read-only.

The board supports:

- viewing workflow columns
- opening ticket detail
- creating and editing tickets
- adding comments
- moving tickets through workflow states
- request-review, approve, and complete actions where policy allows
- project, actor, saved-view, and search/filter URL state
- drag/drop where JavaScript is available, plus button-based fallbacks (keyboard: `n` new ticket, `/` search)
- creating projects from the welcome page through the same `ActionService` and browser security gates

Default serve behavior binds to `127.0.0.1` on a random port and opens a session URL when `--open` is used. The session token stays valid for the lifetime of that server process and is printed only by `tracker web serve`; it is never written to disk.

## Screenshots

Desktop:

![Atlas web board desktop](assets/web-board-desktop.png)

Mobile:

![Atlas web board mobile](assets/web-board-mobile.png)
