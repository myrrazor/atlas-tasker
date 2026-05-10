# Atlas Examples

This directory contains executable demo/regeneration scripts. Checked-in prompt packs, transcripts, and screenshot-friendly output live under `docs/examples/`.

```bash
sh examples/generate-demo-assets.sh --check
sh examples/generate-demo-assets.sh --update
```

`--check` rebuilds the demo in a temporary git workspace and fails if the checked-in examples are stale. `--update` refreshes `docs/examples/`.
