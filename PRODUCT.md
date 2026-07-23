# Product context

Atlas Tasker is a local-first, terminal-first issue tracker for people coordinating coding agents. The browser surface is a local operator console over the same Markdown snapshots, JSONL events, SQLite projection, and Go services used by the CLI and TUI.

## User and job

- Primary user: a technical owner checking a workspace throughout the day with keyboard or pointer.
- Arrival question: which projects are moving, which are blocked, and what changed most recently?
- Success: the owner can spot a project that needs attention and open its filtered board in one click.
- Failure cost: misleading counts or stale-looking activity sends the owner to the wrong work.

Projects are ticket namespaces such as `APP` or `OPS`. A board is a filtered status view, not a separate persisted object. Recent changes are append-only ticket events.

## Behavior and constraints

- Reads go through `QueryService`; writes go through `ActionService`.
- Browser mutations keep the local session, origin, CSRF, and read-only gates.
- The UI is server-rendered Go with vendored Geist fonts and Phosphor icons. CSP forbids external assets and inline styles/scripts.
- The welcome page supports empty, error, read-only, success, long-name, and compact-width states.
- Project/event data is local but may still contain sensitive work details.
- Activity timestamps use the browser host's local timezone.

## Voice and anti-references

The product should feel exact, calm, and candid. Use Atlas terms and real counts. Avoid marketing language, walls of stat cards, boxed-in tables, decorative charts, and terminal cosplay in the browser.

## Open hypotheses

- `[H]` A project ledger plus recent changes is enough context for the first browser screen. Validate by watching whether owners still jump straight to `/board`.
- `[H]` The JSONL full scan remains acceptable for a 20-item local feed. Revisit when project/event volume makes root-page latency noticeable.
