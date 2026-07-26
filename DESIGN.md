# Web product design

## Thesis

Atlas uses a restrained charcoal operator console: dense enough for daily scanning, quiet enough that blocked work and recent changes carry the signal. The signature element is a borderless project ledger with soft row separators, not a collection of dashboard cards.

The product keeps the existing Geist type, charcoal tokens, blue action accent, semantic workflow colors, and inline Phosphor icons. Elevation is reserved for the project-creation dialog.

## Direction

Chosen: ledger plus activity rail. It answers “where does attention go?” with real project counts and an append-only change feed. `[P]` Alignment, proximity, and weak dividers do the grouping before borders or shadows.

Rejected:

- Stat-card dashboard: equal-weight tiles would fragment one comparison task and repeat generic dashboard structure.
- Marketing-style welcome hero: large empty space and product claims would slow a high-frequency operational screen.

## Board

The Kanban board uses narrow, quiet cards so all six workflow columns fit more often. A card face shows a monospace ticket ID and title; when the assignee is an agent with a supported configured color, a small rectangular mark appears in the top-right. The mark never colors the card and never replaces ownership text in the preview or drawer. `[S]`

Secondary metadata lives in the ticket drawer and in one reusable hover preview populated from escaped `data-*` values. The two-second delay keeps normal pointer travel calm. The preview is non-interactive, does not enter the tab order, clamps to the viewport, and disappears on leave, drag, scroll, or Escape.

Drawer motion preserves spatial continuity for an explicitly selected ticket: two animation frames establish the off-screen state, then a 240ms transform with slight overshoot brings the drawer into place; close uses a clean 160ms ease-out. Card hover/focus lifts two pixels on the same restrained spring curve, while press compresses for 90ms before springing back. Drag reordering uses a 150ms positional animation with restrained chosen and ghost states, followed by a short scale settle. Server sync records ticket positions by ID before replacing the grid, then uses a 180ms native FLIP transform to carry moved cards to their new positions; changed column counts receive one 300ms pulse. These effects are causal, short, dependency-free, transform/opacity-only, and disabled by `prefers-reduced-motion`. `[P]` `[S]`

## System

- Background `#151517`; primary surface `#1b1b1e`; text `#ececee`; muted text `#9d9ea6`; action `#5b8def`.
- Geist 400/500/600 is vendored. Changing counts use tabular numerals.
- Controls use the existing 10px radius. The dialog uses the 14px overlay radius and named modal shadow.
- Known agent colors are mapped server-side to `chip--blue` and `chip--orange`; unknown names remain uncolored. Color never replaces the agent label. `[S]`
- Table rows have soft bottom dividers only. There are no vertical rules or cell boxes.
- Compact layouts stack the activity rail below the project ledger; fixed table columns wrap names and status segments so blockers stay visible.
- Native `<dialog>` supplies modal focus/escape behavior, with an ordinary linked fallback when JavaScript is unavailable. Visible focus remains on every control. `[S]`
- Motion is limited to existing control feedback and disappears under `prefers-reduced-motion`. `[S]`

## States

- Empty: explains both browser and CLI project creation.
- Error: renders the exact service/validation message with the original form values.
- Read-only: hides project creation and keeps the workspace fully inspectable.
- Success: returns to the ledger with a persistent inline confirmation.
- Long content: project keys stay secondary; status links wrap without changing meaning.

## Performance

The welcome page adds no dependency or external request. SortableJS is no longer loaded outside the board. Recent activity is capped at 20 events; the service documents the current per-project JSONL scan tradeoff.
