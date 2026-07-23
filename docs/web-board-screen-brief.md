# Web board screen brief

Mode: audit → revamp → harden.

The local workspace owner arrives to scan ticket flow, spot stalled work, and move a ticket without losing context. The board favors simultaneous column visibility; the drawer carries complete ticket detail and edits.

## Hierarchy

1. Search, filters, and the visible New Ticket action.
2. Six workflow columns and their counts.
3. Ticket ID and title.
4. Optional mapped-agent color mark.
5. Delayed hover context, then the linked detail drawer.

The card itself stays neutral. Status belongs to the column, ownership color is scarce, and priority/labels/counts remain readable in the preview and drawer rather than competing on every face.

## Interaction coverage

| Interaction | Behavior |
|---|---|
| Hover | Wait 2 seconds, then show one viewport-clamped preview from card `data-*` values |
| Leave, scroll, drag, Escape | Cancel the timer and dismiss the preview |
| Keyboard | Cards remain ordinary focused links; the preview never enters the tab order |
| Open drawer | Explicit `?ticket=` selection slides in after two animation frames |
| Close drawer | Reverse the transform before following the close link |
| Drag | Animate position for 150ms; preserve optimistic commit, revert, and journal behavior |
| Reduced motion | Remove drawer, card, preview, and drag animation without hiding state |

## Responsive coverage

- 1280px: six compact columns fit beside the drawer when content allows.
- 768px: the board remains horizontally navigable and the drawer stacks below.
- 390px: one selected workflow column is shown; column navigation and New Ticket remain visible.

## Acceptance

- A mapped assignee such as `agent:claude` receives only the configured color mark.
- Unknown agents and unsupported color names render no card mark.
- The preview makes no request and uses `textContent` for user-controlled values.
- Assignee, reviewer, priority, labels, blockers, gates, and comments remain available in the preview and drawer.
- Drag success journals a web move; failure returns the card to its original column.
- CSP stays self-only with vendored assets and no new dependency.
