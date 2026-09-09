# Roadmap

`v1.11.0` is the current stable release. It added MCP workflow coverage, `tracker update`, coding-agent detection and integration selection, and local security hardening. The local web console, scheduled tickets, agent queues, and six language catalogs remain the foundation.

## v1.12 release preparation

- Accept and validate all six web language codes (`en`, `es`, `id`, `zh`, `ja`, `ko`) through `tracker config set web.lang`, with configuration round-trip and rendered-page coverage.
- Bring README, MCP references, agent setup instructions, and the website docs in line with the shipped CLI and MCP tools.
- Offer optional agent guidance setup after a terminal install; make cancellation and unattended installs predictable.
- Use neutral, reproducible screenshots and the UI's existing Atlas Tasker wordmark across the public product pages.

## Next

Follow-ups beyond this release:

- the schedule workspace renders English-only; bring it into the language catalogs
- additional walkthroughs and packaged examples for common agent workflows

## Not planned

Hosted server, SaaS identity, CRDT rewrite, plugin marketplace, mandatory MCP flow, encrypted workspace storage, provider-rule controller, or a hidden scheduler daemon — unattended schedule ticking stays on the owner's explicit cron, launchd, or other trusted runner.
