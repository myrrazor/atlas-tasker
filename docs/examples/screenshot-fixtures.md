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
at 1440 × 900, then `/board` (without an open ticket) and the schedule at
390 × 844. Set the phone viewport before navigating so filters start collapsed.
Wait for fonts and drawer transitions to settle before capturing. Save captures to
`docs/assets/web-*.png`; encode the desktop board as `site/assets/web-board.webp`.
Use actual rendered UI and inspect every capture for private names, paths, and tokens.

The README and website wordmark copies come from
`internal/web/static/brand/atlas-tasker-ascii.svg`. Render
`assets/brand/social-card.html` at 1200 × 630 to reproduce `site/og.png`.

## Current dark mode

Every product screenshot in README and the marketing site must be a genuine
capture of the current dark-mode UI. Recapture rather than restyle an old
light-mode or stale-chrome frame. Do not author simulated chat or board pixels
and present them as output.

## Multi-agent shared board

Seed a board where Grok Build, Cursor, and Grok Bot each own tickets:

```bash
TRACKER_BIN=/absolute/path/to/tracker sh examples/create-multi-agent-demo.sh /tmp/atlas-multi-agent
cd /tmp/atlas-multi-agent
/absolute/path/to/tracker web serve --no-browser
```

Capture `/board` at 1440 × 900 as `docs/assets/multi-agent-board-desktop.png` and encode
`site/assets/multi-agent-board.webp`. Capture `tracker board` as
`docs/assets/multi-agent-board.png`. Keep session URLs and tokens out of public files.

## Grok Build status

Capture a real Grok Build session after `tracker init` on a v1.15 source build,
with the Atlas MCP entry loaded, asking a normal ticket-status question.
Use synthetic sample tickets only (the web demo fixture is fine). Save:

1. `docs/assets/grok-status.png` for README
2. `site/assets/grok-status.webp` encoded from that capture for the website

Use the capture’s actual dimensions in HTML image attributes. Crop session IDs,
credentials, personal identity, and local filesystem paths before commit. Public files may identify `myrrazor` and the product; they must not
identify a person or a private path.

Caption both uses as a real Grok Build session, synthetic sample tickets, and a
v1.15 source build. Grok has its own native Markdown display. Do not caption it
as Atlas's terminal TUI or a rich browser board.

Source/build evidence for the capture stays outside the repository.

## Grok Build walkthrough video

`site/assets/grok-flow.mp4` records the actual terminal canvas while Grok reads
the Example App board through Atlas MCP, creates the requested high-priority task,
and reads the board again. The original eight synthetic tickets become nine;
APP-9 is the new backlog task. The [transcript](grok-video-transcript.md) contains
the actual prompts, responses, and tool names.

The 1260×720 H.264 clip is silent, has no generated UI pixels, and keeps events
in their original order. Local paths and client chrome are cropped out; pauses
are shortened and disclosed beside the player. The poster is an actual video
frame. Preserve the private original capture and editing evidence outside the
repository. Verify media decoding, responsive sizing, keyboard playback, seeking,
and the transcript link before replacing the public assets.
