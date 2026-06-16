# Web Board Dev Notes

The web board is server-rendered Go.

- Package: `internal/web`
- Templates: `internal/web/templates`
- Static assets: `internal/web/static`
- Browser drag/drop: vendored `sortablejs@1.15.7` MIT under `internal/web/static/vendor/`

Do not add CDN references. Do not mutate storage directly from handlers. Route reads through `QueryService` and writes through `ActionService`.

For UI fidelity, compare the implementation against the approved concept images:

- Desktop: `/Users/masterhit/.codex/generated_images/019eb2ed-69f2-77a1-97cd-8c098e7471c7/ig_04d8144660305d6b016a31d79c126881968b011829d0582317.png`
- Mobile: `/Users/masterhit/.codex/generated_images/019eb2ed-69f2-77a1-97cd-8c098e7471c7/ig_04d8144660305d6b016a31d80d6d9c8196a1fcae0bc9e14550.png`

