# Scheduled Work

Atlas schedules a ticket once at an exact RFC3339 instant. The ticket's assignee is the runner, so the schedule cannot quietly name one owner while the workflow names another.

## Human reminders

```bash
tracker schedule set APP-12 \
  --at 2026-08-10T09:00:00-04:00 \
  --runner human:alex \
  --actor human:owner \
  --reason "Monday follow-up"
```

Before the due time, `tracker schedule list` reports the ticket as `scheduled`. Once it is past due but has not been ticked, it is `overdue`. A tick records `ticket.schedule_triggered`, which goes through the same terminal, file, webhook, and subscription notification paths as other Atlas alerts.

## Agent work

The runner may be an enabled agent profile:

```bash
tracker schedule set APP-12 \
  --at 2026-08-10T13:00:00Z \
  --runner agent:builder-1 \
  --actor human:owner \
  --reason "run release checks"
```

When due, Atlas creates the same inspectable wakeup used by dependency-driven agent work. The default `notify` mode leaves it pending for the agent to pick up. To launch an executable automatically, the owner must opt that agent into command mode:

```bash
tracker agent auto set builder-1 --mode command \
  --argv codex --argv "work ticket {ticket_id}" \
  --actor human:owner --reason "allow scheduled pickup"
```

Arguments are passed directly without a shell. Existing placeholders still work, plus `{scheduled_at}` for the ticket's UTC schedule time. Shell interpreters remain rejected. If launch fails, Atlas records a durable failed state and does not loop; reschedule the ticket after fixing the command.

Agent schedules are owner-only because they can start a local process. The agent profile determines the provider/model family and keeps the rest of Atlas's capability, policy, lease, and review behavior intact once the worker picks up the ticket.

## Ticking and history

Atlas deliberately does not run a hidden scheduler. Call this from the scheduler already trusted on the machine:

```bash
tracker schedule tick --actor human:owner --reason "scheduled tick"
```

`tick` handles every due, untriggered schedule and is safe to run again. `--now` exists for deterministic operations and tests. Use `tracker schedule clear` to remove a schedule or `schedule set` to replace it and reset its outcome.

```bash
tracker schedule list --project APP --from 2026-08-10T00:00:00-04:00 --to 2026-08-11T00:00:00-04:00
tracker schedule history --project APP --json
```

Completion history is derived from ticket transitions to `done` in the append-only event log. There is no parallel history database to drift from ticket truth.
