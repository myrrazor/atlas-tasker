# Config

Atlas config lives under the local workspace state. Use the CLI instead of editing files by hand:

```bash
tracker config get
tracker config get release.verify_checksums
tracker config set release.verify_checksums true
```

Config changes that affect release, governance, sync, signing, redaction, or provider behavior should be recorded in PR notes and tested with targeted commands.
