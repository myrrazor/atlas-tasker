# Known Limitations

Atlas uses GitHub Issues as the source of truth for public follow-up work. This page points to known v1.10 wake-up design items without duplicating the issue bodies.

## v1.10 Wake-Up Follow-Ups

- [#100 Wakeups do not fire for naturally blocked backlog successors](https://github.com/myrrazor/atlas-tasker/issues/100) — shipped in v1.10.0: when the last blocker of an agent-assigned ticket reaches `done`, Atlas promotes that dependent from `backlog` to `ready` (actor `agent:atlas`) before waking the agent, and `queue`/`next` list unblocked backlog under `unblocked_for_me`. What stays deliberate: human-assigned and unassigned dependents remain in `backlog` and surface under `unblocked_for_me` instead, and a ticket set to `blocked` by hand is woken but never moved.
- [#101 Add reviewer wakeups when review work becomes available](https://github.com/myrrazor/atlas-tasker/issues/101)
- [#102 Harden agent auto command validation for interpreter-style launchers](https://github.com/myrrazor/atlas-tasker/issues/102)

Security-sensitive wake-up authorization and integrity findings are tracked privately first. They should be disclosed publicly only after fixes land.
