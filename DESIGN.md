# Atlas Tasker web product design

## Thesis

Atlas uses a deep-navy operator console built on the atlas.pen tokens: dense enough for daily scanning, quiet enough that blocked work, due work, and recent changes carry the signal. The signature elements come from the product itself: a borderless project ledger, a compact Kanban board, the terminal wordmark, and an auditable time rail for scheduled tickets.

The product uses Geist for text and Geist Mono for ticket keys, counts, times, and uppercase kickers, navy surfaces, a single `#68a9ff` accent, semantic workflow hues on compact indicators only (column accent bars and the status, priority, and schedule-state pills), and inline Phosphor icons. Cards carry no colored borders. Elevation is reserved for real layers such as the project dialog, ticket drawer, hover preview, and temporary menus, plus the brief lift a card gets on hover.

Behavior should feel calm, exact, and accountable. Avoid marketing-style heroes, lifestyle calendars, glass dashboards, decorative metrics, AI glow, and pills that do not encode real state.

## Welcome and board

The welcome view uses a project ledger plus recent activity. It answers “where does attention go?” with real project counts and the append-only event feed. `[P]` Alignment, proximity, and weak dividers do the grouping before borders or shadows.

The Kanban board uses narrow, quiet cards so all six workflow columns fit more often. A card face shows a monospace ticket ID and title; when the assignee is an agent with a supported configured color, a small rectangular mark appears in the top-right. The mark never colors the card or replaces ownership text. `[S]`

Secondary metadata lives in the ticket drawer and in the delayed hover preview. The preview is non-interactive, stays out of the tab order, clamps to the viewport, and dismisses on leave, drag, scroll, resize, or Escape.

Drawer, card, drag, and live-sync motion exists to preserve spatial continuity. It is short, transform/opacity based, interruptible where applicable, and removed under `prefers-reduced-motion`. `[P][S]`

## Schedule

### Direction

Chosen: operator timeline. The supplied [Sparkpixel schedule reference](https://x.com/sparkpxldesign/status/2082667080311296177) contributes the compact week strip and vertical time rail; Atlas supplies the dark control-room language, ticket truth, runner identity, execution state, and completion ledger.

Rejected:

- Month grid: compresses exact execution state and makes one-time ticket work read like all-day events.
- Due buckets only: efficient for triage, but they discard the owner-requested time model.
- Lifestyle planner treatment: avatars, mobile-device framing, and decorative calendar chrome do not fit a local developer tool.

### Hierarchy and layout

1. Selected date and overdue or failed work.
2. Ticket ID/title, exact local time, runner kind, and honest execution state.
3. Schedule, clear, or run-due action.
4. Completion history derived from workflow events.

Expanded layouts place the time rail beside a 340px completion ledger. Medium and compact layouts stack them in DOM order. The week strip is the only intentional horizontal scroller; long titles, actors, and reasons wrap instead of truncating critical state.

### Components and states

- Native links, forms, `details`, selects, and `datetime-local` inputs preserve keyboard behavior without a new client dependency.
- Human and agent runners are named in text; color and initials are redundant cues.
- `Agent ready` means notify mode created a pending wakeup. `Agent launched` appears only after command mode starts its configured process.
- Empty, error, read-only, failed, success, long-content, and no-history states use the same hierarchy as populated schedules.
- Clearing names its exact scope: it removes the schedule, not the ticket or assignee.

## System

- Background `#0b0f14`; surfaces `#111821`, `#18222d`, and `#17283c`; lines `#27323e` and `#33404e`; text `#eef4fa`; muted text `#a8b5c3` and `#8b98a6`; accent `#68a9ff`.
- Status hues: backlog `#929ba6`, ready `#c9d45d`, in progress `#43cf7c`, in review `#a78bfa`, blocked `#ff7169`, done `#54c6b4`. They color the column accent bar and the status and schedule-state pills; priority pills get the same tinted treatment in red, `#e6b85c`, the accent, and gray. Semantic color never lands on a card border or a large surface.
- Geist and Geist Mono are vendored as variable fonts (weight axis 100 to 900). Changing counts and schedule times use tabular numerals; schedule times are 24h in the mono face.
- Radius scale: 12px panels (week strip, timeline, ledger), 10px trays (columns, hover preview), 8px plates and controls (cards, inputs, buttons), 999px pills.
- The web topbar carries the ASCII wordmark at fixed dimensions to avoid layout shift; the terminal wordmark is the README and site mark.
- Known agent colors are mapped server-side; unknown names remain uncolored. Color never replaces an owner or state label. `[S]`
- Board, filter, tab, and drawer controls grow to a 44px minimum height at phone widths (760px and below). Visible `:focus-visible` treatment remains on every control. `[S]`
- Schedule and board content reflow without two-dimensional page scrolling at 320px, apart from the explicitly scrollable week strip. `[S]`

## Performance and implementation

- Server-render final welcome, board, and schedule state; no loading-shell dependency.
- Keep fonts, scripts, styles, and logo local. Schedule adds no runtime package, remote font, or client framework.
- Load SortableJS only on the board. Schedule navigation and mutations work as ordinary links and form posts.
- Recent activity keeps its existing bounded feed. Schedule history is projected from canonical workflow events rather than copied into a calendar store.

## Open hypotheses

- `[H]` A project ledger plus recent changes is enough context for the first browser screen. Validate by watching whether owners still jump straight to `/board`.
- `[H]` A seven-day strip plus one-day rail is faster to scan than a month grid for exact agent runs. Validate with dense and empty-day task walkthroughs.
- `[H]` Completion history belongs beside the rail on wide screens and below it on narrow screens. Validate with responsive use and focus-order checks.
- `[H]` The two-second hover delay exposes card context without making ordinary pointer travel noisy. Revisit if owners repeatedly open the drawer for secondary metadata.
