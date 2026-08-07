# Screen Brief — Schedule

## Mode

shape + harden

## User and job

- **User:** Developer/owner coordinating human and agent tickets.
- **Arrival context:** Opens Schedule from the local Atlas top bar.
- **Question on arrival:** What is due today, who runs it, and what completed this week?
- **Primary action:** Add or replace a one-time ticket schedule.
- **Success:** Exact local time, runner kind, and derived state appear in the day rail.
- **Failure cost:** Missed work or falsely implied agent execution.
- **Frequency:** Daily.
- **Platform/input:** Desktop local browser first; keyboard, pointer, touch, and screen reader supported.

## Content

- **Real data:** `ScheduleView`, agent profile provider/name, wakeup state, ticket done events.
- **Longest labels:** Ticket title, actor ID, error message, completion reason.
- **Largest values:** No artificial item cap; the timeline scroll region and wrapping absorb density.
- **Sensitive content:** Workspace tickets; never copy session tokens into screenshots.
- **Freshness/units/timezone:** Request-time state; local day/time with visible IANA or local timezone name; UTC in persistence/API.
- **Sample content:** Only repository fixture/demo content, not presented as production activity.

## Decision sequence

1. Choose a day in the seven-day strip.
2. Read chronological ticket blocks and human/agent state.
3. Schedule, clear, tick, or follow a ticket back to its board detail.

## Hierarchy

1. **Must notice:** Selected date, overdue/failed work.
2. **Must understand:** Ticket title/time, runner kind, honest state.
3. **Must act:** Schedule ticket or run due schedules.
4. **May inspect:** Completed-this-week ledger.
5. **Advanced/rare:** Exact wakeup ID/error and rescheduling after failure.

## Layout by window

### Compact

- **Navigation:** Brand plus Board/Schedule; horizontal seven-day strip.
- **Primary content:** One-column day rail with narrow time gutter.
- **Secondary content:** Completion history below rail.
- **Actions:** Schedule form under a native `details`; tick button remains visible.
- **Overflow:** Week strip horizontal only; timeline scrolls vertically within its own region.

### Medium

- Form and timeline remain one column; completion history follows at full width.

### Expanded

- **Panes:** Flexible day rail and 340px completion ledger.
- **Persistent context:** Week strip and selected date remain above both panes.
- **Use of extra width:** Longer ticket titles and runner metadata, never gratuitous whitespace.

## State matrix

| State | What the user sees | Primary recovery/action | Focus/announcement |
|---|---|---|---|
| Loading | Final server-rendered page; no spinner shell | wait for navigation | browser standard |
| Empty | “Nothing scheduled for [day]” and available form | schedule a ticket | heading precedes form |
| Error | Red alert with exact failure; submitted fields retained | correct runner/time/reason | `role=alert` |
| Offline | Browser connection error | restart/open local server | browser standard |
| Permission denied | Read-only chip and disabled writes, or owner-only message | reopen with authorized actor | alert on failed submit |
| Partial/stale | Missing profile labeled by actor ID; no invented model | repair agent profile | text state |
| Success | Flash plus updated rail state | inspect card or continue | `role=status` |
| Destructive confirmation | Clear button says “Clear schedule”; ticket owner stays | reschedule if needed | native submit focus |
| Long/localized content | Wrapped titles/actors/reasons | scroll within region | no truncation of critical state |

## Interaction

- **Keyboard:** Tab through app navigation, dates, form, ticket blocks, history; Enter submits native forms.
- **Touch/pointer:** Date and action targets meet compact size; no drag required.
- **Undo/cancel/back:** Browser Back restores URL date; reschedule replaces; clear is explicit.
- **Save/progress:** Full-page POST/redirect with flash; failed form renders in place.
- **Motion purpose:** Existing hover/focus feedback only.
- **Reduced-motion behavior:** No transitions under the existing media query.

## Visual direction

- **Thesis:** Operator timeline, not lifestyle calendar.
- **Signature move:** Atlas status rail crossing chronological human reminders and agent wakeups.
- **Type:** Geist with tabular time numerals.
- **Color:** Existing neutral charcoal; red/yellow/teal/purple only for semantic state.
- **Surfaces:** One main rail boundary and one completion ledger; cards only for actual tickets.
- **Icons/imagery:** Text-first runner badges and small CSS marks; no avatars required.
- **Anti-references:** copied mobile shell, month grid, AI glow, card wall, decorative metrics.

## Evidence and hypotheses

| Decision | Tag | Rationale | Validation |
|---|---|---|---|
| Seven-day strip and vertical rail | [C][P] | Matches owner reference and exact one-time execution | desktop/mobile screenshots |
| Human/agent words on every block | [P][A] | Prevents model or color ambiguity | screen-reader/text inspection |
| Completion ledger beside rail | [P][H] | Owner requested history; keeps future and past adjacent | responsive QA |
| Existing CSS components, no new library | [S] | Current stack already meets controls/semantics | tests and bundle inspection |
| Visible timezone | [P][H] | Exact schedules are unsafe without local context | handler and template tests |

## Acceptance criteria

- [x] Core task complete
- [x] Real/representative content
- [x] Empty, error, failed, read-only, and success states
- [x] Compact/medium/expanded
- [x] Keyboard/touch
- [x] Labels/focus/contrast/targets
- [x] Text scaling/localization
- [x] Reduced motion
- [x] Performance checked
- [x] Screenshots captured and reviewed
