# Config

Atlas config lives under the local workspace state. Use the CLI instead of editing files by hand:

```bash
tracker config get
tracker config get release.verify_checksums
tracker config set release.verify_checksums true
tracker config set web.lang es
```

`web.lang` accepts `en`, `es`, `id`, `zh`, `ja`, and `ko`. A blank value lets the browser language
choose the board catalog, with English as the fallback.

Config changes that affect release, governance, sync, signing, redaction, or provider behavior should be recorded in PR notes and tested with targeted commands.
