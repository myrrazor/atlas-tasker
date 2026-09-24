# Display the board in chat

Atlas can present the same live ticket state as a terminal table, an MCP App widget, a self-contained HTML fragment, or Markdown. Pick the route the chat host renders. The ticket markdown and event log remain the source of truth.

## Meta Muse and HTML widgets

Meta Muse's chat widget can present HTML with `--hatch-widget-*` theme variables. Run `tracker board --style html` from the initialized workspace, then place its complete stdout directly in the widget or assistant message where Muse renders HTML. This built-in command replaces a separate `render_board.py`: it reads the live board and ticket detail in one process. Do not send a path to a temporary file: the board must survive transcript replay. The output has no external scripts, fonts, or stylesheets. Check the actual Muse conversation before claiming that its widget rendered the fragment.

With agent setup enabled, `tracker init` puts these instructions in the workspace `AGENTS.md` and an `atlas-worker` skill. When no detected client gets an `AGENTS.md`, it installs the generic guide. Ask Muse to read `AGENTS.md` once after setup if it does not automatically load workspace instructions; Atlas cannot force a separate chat host to discover a local file. Existing workspaces can refresh the managed guide with `tracker init`. The published one-line installer downloads the latest **release** binary, so this command requires a release that includes `--style html`; check `tracker board --help` after installing.

In a Git workspace, commit the generated agent instructions and skill before dispatching workers into Git worktrees, so those workers can read the same guidance.

## MCP Apps hosts

Call `atlas.board` with the workspace or project you want to show. The tool advertises `ui://atlas/board` as an MCP App resource. A compatible host displays a read-only board in the transcript; the same tool result includes Markdown for hosts without Apps support. Cards expand with native `<details>` to show description, acceptance criteria, relations, and review state. The widget has a restrictive CSP and no external fonts, scripts, or network calls. Check the actual session before claiming it displayed the widget.

## HTML command and fallback

From the initialized workspace root:

```bash
tracker board --style html
tracker board --style html --project APP
```

Use the command's entire stdout as one inline HTML fragment in the assistant message or widget. Do not wrap it in a code fence or reference a temporary file. CSS, status counts, priority cues, labels, blocker badges, and expandable detail live in the fragment. The fragment honors `--hatch-widget-surface`, `--hatch-widget-surface-muted`, `--hatch-widget-text`, `--hatch-widget-muted`, `--hatch-widget-border`, `--hatch-widget-accent`, and `--hatch-widget-shadow` when the host provides them. It has light and dark defaults, responsive columns, and no external assets. Atlas escapes ticket text before writing HTML.

If the host prints tags or strips `<style>` or `<details>`, use `tracker board --style markdown`. The HTML command changes presentation only; `tracker board --json` remains the machine-readable board contract.

## Grok Bot and text-only hosts

[Grok Bot's files and results guide](https://docs.x.ai/grok-bot/files-and-results) documents normal messages and file previews, while [its product overview](https://x.ai/news/designing-grok-bot) describes built-in inline cards. Neither documents arbitrary HTML in ordinary messages or a custom-font API for third-party board output. An observed Grok Bot chat displayed raw HTML as text. Use the `markdown` field from `atlas.board` or `tracker board --style markdown` for a readable board with status counts and emphasized ticket IDs. The app chooses its message font. If a specific host visibly renders ANSI code blocks, `tracker board --style chat` or MCP `format=chat` is also available; verify that before using it.

Grok Bot's [private skills](https://docs.x.ai/grok-bot/skills-routines-and-automations) are saved in its own app. Its repo-local Grok Build skill is a different integration. To make this default in Grok Bot, ask it to save this instruction as a private skill: “When I ask to see my Atlas board, read current tickets from the initialized workspace, run `tracker board --style markdown` with any requested project filter, and paste the complete Markdown in the chat. If the command fails, report the error; do not reuse an old board.”

For large MCP boards, inspect `shown_cards`, `total_cards`, and each column's truncation state. Narrow to a project or page through `cursor_by_status` so the displayed board does not imply hidden tickets are absent.
