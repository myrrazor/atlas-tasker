# Web Board Dev Notes

The web board is server-rendered Go.

- Package: `internal/web`
- Templates: `internal/web/templates`
- Static assets: `internal/web/static`
- Browser drag/drop: vendored `sortablejs@1.15.7` MIT under `internal/web/static/vendor/`

Do not add CDN references. Do not mutate storage directly from handlers. Route reads through `QueryService` and writes through `ActionService`.

For UI fidelity, compare the implementation against the committed reference screenshots in `docs/assets/web-board-desktop.png` and `docs/assets/web-board-mobile.png`.

