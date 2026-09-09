# Screenshot Fixtures

These files are normalized so they are useful for README screenshots, terminal captures, and docs examples without leaking local paths.

Recommended panes:

1. `docs/examples/output/board.txt`
2. `docs/examples/output/ticket-inspect.md`
3. `docs/examples/output/dashboard.txt`
4. `docs/examples/output/goal-brief.md`

Use a terminal width around 96 columns for board/dashboard captures and 100-110 columns for the goal brief.

## Web screenshots

Create a fresh synthetic workspace for browser captures. This seeds `web.owner_name`
as `User`, uses demo projects and actors, and refuses a non-empty destination.
Schedules default to tomorrow in UTC so every new schedule is in the future;
use the schedule date picker for that day, or set `DEMO_DATE` to another future date:

```bash
TRACKER_BIN=/absolute/path/to/tracker sh examples/create-web-demo.sh /tmp/atlas-web-demo
cd /tmp/atlas-web-demo
/absolute/path/to/tracker web serve --no-browser
```

Open the authenticated loopback URL printed by the server. Keep that session URL
and token out of public files. Capture `/`, `/board?ticket=APP-4`, and `/schedule`
at 1440 × 900, then the board and schedule at 390 × 844. Save the captures to
`docs/assets/web-*.png`; encode the desktop board as `site/assets/web-board.webp`.
Use actual rendered UI and inspect every capture for private names, paths, and tokens.

The README and website wordmark copies come from
`internal/web/static/brand/atlas-tasker-ascii.svg`. Render
`assets/brand/social-card.html` at 1200 × 630 to reproduce `site/og.png`.
