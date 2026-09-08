# Web Board Dev Notes

The web board is server-rendered Go.

- Package: `internal/web`
- Templates: `internal/web/templates`
- Static assets: `internal/web/static`
- Welcome route: `/` (project rollups and recent activity)
- Canonical board route: `/board`
- Schedule route: `/schedule` (week strip, hourly timeline, completion history)
- Read-only settings route: `/settings`
- Browser drag/drop: vendored `sortablejs@1.15.7` MIT under `internal/web/static/vendor/`

Do not add CDN references. Do not mutate storage directly from handlers. Route reads through `QueryService` and writes through `ActionService`.

For UI fidelity, compare the implementation against the committed reference screenshots in `docs/assets/web-board-desktop.png` and `docs/assets/web-board-mobile.png`.
The welcome-page intent and responsive/state coverage are recorded in `docs/web-welcome-screen-brief.md`.
The minimal-card interaction contract is recorded in `docs/web-board-screen-brief.md`.
The schedule screen brief is recorded in `docs/design/schedule-screen.md`.
