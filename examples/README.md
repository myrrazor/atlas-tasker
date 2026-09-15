# Atlas Examples

This directory contains executable demo/regeneration scripts. Checked-in prompt packs, transcripts, and screenshot-friendly output live under `docs/examples/`.

```bash
sh examples/generate-demo-assets.sh --check
sh examples/generate-demo-assets.sh --update
TRACKER_BIN=/absolute/path/to/tracker sh examples/create-web-demo.sh /tmp/atlas-web-demo
TRACKER_BIN=/absolute/path/to/tracker sh examples/create-multi-agent-demo.sh /tmp/atlas-multi-agent
```

`--check` rebuilds the demo in a temporary git workspace and fails if the checked-in examples are stale. `--update` refreshes generated files under `docs/examples/`. The create scripts seed empty directories for web and multi-agent captures.
