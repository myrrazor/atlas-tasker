# Atlas Tasker product context

Atlas Tasker is a local-first, terminal-first issue tracker for people coordinating human and coding-agent work. The browser is a local operator console over the same Markdown snapshots, JSONL events, SQLite projection, and Go services used by the CLI, TUI, shell, and MCP surfaces.

## Users and jobs

The primary user is a technical owner working beside terminals and coding tools throughout the day. They need exact state, clear ownership, visible failures, and no hidden cloud dependency.

The browser has three related arrival questions:

- **Overview:** Which projects are moving, blocked, or recently changed?
- **Board:** Which tickets need attention, and where are they in the workflow?
- **Schedule:** What is due on the selected day, who or what runs it, and what completed this week?

Projects are ticket namespaces such as `APP` or `OPS`. A board is a filtered status view, not a separate persisted object. A schedule is a one-time due instruction on the canonical ticket, not a copied calendar event or recurring cron record.

## Schedule journey

1. Open Schedule and choose a day in the seven-day strip.
2. Scan exact local times, ticket state, and human or agent runner identity.
3. Set, replace, or clear a one-time schedule.
4. Run due schedules manually when needed and inspect the durable reminder, wakeup, failure, or completion history.

Human schedules notify the configured sinks. Agent schedules create a wakeup in notify mode or launch the configured process in command mode. The UI must never describe a pending wakeup as a launched agent.

Completion history comes from existing ticket workflow events. Every schedule mutation is also an append-only ticket event, and the SQLite data remains a rebuildable projection.

## Recovery and trust

- **Empty:** explain the clear day and keep scheduling available.
- **Error:** retain the submitted ticket, time, runner, and reason with the exact failure.
- **Read-only or unauthorized:** keep schedules and history visible, disable mutation controls, and state why a rejected write did not land.
- **Missing profile:** show the actor ID as unavailable; never invent a provider, model, or display name.
- **Clear:** remove only the schedule and preserve ticket ownership and history.
- **Offline:** the local server is the product boundary; an unreachable server uses the browser’s normal connection failure.

Exact instants are stored in UTC. The web form and rail use the server’s named local timezone and show that timezone in the interface.

## Behavior and constraints

- Reads go through `QueryService`; writes go through `ActionService`.
- Browser mutations keep the local session, origin, CSRF, policy, and read-only gates.
- The UI is server-rendered Go with vendored fonts and local assets. CSP forbids external scripts, styles, and images.
- The welcome page, settings, and board chrome retain the existing language catalogs. Ticket content is never translated.
- Schedule navigation is deep-linkable with `/schedule?date=YYYY-MM-DD&project=KEY`.
- Atlas does not run a hidden scheduler daemon. Unattended ticking remains the owner’s explicit cron, launchd, or other trusted runner setup.
- One-time scheduling, execution state, and completion history are in scope. Recurrence is not implied.

## Voice and anti-references

Use calm, exact Atlas terms: due, overdue, scheduled, human, agent, ready, launched, completed, failed. Avoid “autonomous” when only notify mode is configured, vague automation claims, lifestyle-planner styling, decorative avatars, generic month grids, and terminal cosplay disconnected from real product behavior.

## Open hypotheses

- `[H]` The project ledger and event rail provide enough orientation before users enter the board.
- `[H]` The JSONL scan remains acceptable for the bounded local recent-activity feed.
- `[H]` The week strip and daily rail fit exact one-time agent work better than a month calendar.
- `[H]` Completion history is most useful beside upcoming work on wide screens and below it on compact screens.
