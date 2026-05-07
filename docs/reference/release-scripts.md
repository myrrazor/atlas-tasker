# Release Scripts

Release scripts are proof helpers, not marketing status.

- `scripts/install.sh`: installs a published archive.
- `scripts/verify-release.sh`: verifies checksums and GitHub attestations by default.
- `scripts/release-rehearsal.sh`: builds local archives and runs an installer smoke.
- `scripts/stability-smoke.sh`: runs a local end-to-end smoke.
- `scripts/preflight-release-proof.sh`: checks repository release prerequisites available to the current GitHub token.

PR-807 adds `scripts/preflight-release.sh` as the public wrapper and keeps the existing proof script compatible.

Use:

```bash
VERSION=v1.8.0-rc1 ./scripts/release-rehearsal.sh
VERSION=v1.8.0-rc1 ./scripts/verify-release.sh ./tracker_1.8.0-rc1_darwin_arm64.tar.gz
```

If hosted assets do not exist, verification cannot complete and release evidence must say no-ship.
