# Known Limitations

Atlas uses GitHub Issues as the source of truth for public follow-up work. This page points to known v1.10 wake-up design items without duplicating the issue bodies.

## v1.10 Wake-Up Follow-Ups

- [#100 Wakeups do not fire for naturally blocked backlog successors](https://github.com/myrrazor/atlas-tasker/issues/100) — the wake-up itself has fired for `backlog`/`blocked` dependents since v1.9.0-rc1. What was still missing, and is fixed in the next release (see CHANGELOG Unreleased), is that the woken agent's own queries never showed the ticket: an agent-assigned `backlog` dependent is now promoted to `ready` by `agent:atlas` when its last blocker completes, and `queue`/`next` list unblocked backlog under `unblocked_for_me`. What stays deliberate: human-assigned and unassigned dependents remain in `backlog` and surface under `unblocked_for_me` instead, and a ticket set to `blocked` by hand is woken but never moved.
- [#101 Add reviewer wakeups when review work becomes available](https://github.com/myrrazor/atlas-tasker/issues/101)
- [#102 Harden agent auto command validation for interpreter-style launchers](https://github.com/myrrazor/atlas-tasker/issues/102)

Security-sensitive wake-up authorization and integrity findings are tracked privately first. They should be disclosed publicly only after fixes land.
