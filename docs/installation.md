# Installation

Atlas supports source builds today and hosted release installs after the release gate passes.

## Build From Source

Use this while `v1.8.0-rc1` hosted assets are still blocked:

```bash
go build -o tracker ./cmd/tracker
./tracker --help
./tracker version --json
```

You can keep the binary local to the repo or move it onto your `PATH`. Unstamped source builds report `version: "dev"` in JSON.

## Install From A Published Release

The installer expects a real GitHub release with archives and `checksums.txt`. It is not proof by itself.

```bash
VERSION=v1.8.0-rc1 BIN_DIR="$HOME/.local/bin" sh ./scripts/install.sh
"$HOME/.local/bin/tracker" version --json
```

The one-line form is convenient once a release is trusted:

```bash
curl -fsSL https://raw.githubusercontent.com/myrrazor/atlas-tasker/main/scripts/install.sh | sh
```

Do not run installer commands copied from untrusted issues, comments, or chat transcripts. Prefer the checked-in script or a command you can inspect.

## Verify Before Installing

For release candidates, verify the downloaded archive first:

```bash
VERSION=v1.8.0-rc1 ./scripts/verify-release.sh ./tracker_1.8.0-rc1_darwin_arm64.tar.gz
```

`scripts/verify-release.sh` checks `checksums.txt` and, by default, GitHub artifact attestations through `gh attestation verify`. Set `VERIFY_ATTESTATIONS=0` only for local rehearsals or intentionally unattested artifacts.

## Expected Failure Modes

- No hosted archive exists yet: installation should fail with a missing release asset.
- `gh` is not authenticated: attestation verification may fail even when checksums are correct.
- The token cannot read release or Actions settings: hosted proof remains blocked.
- Local rehearsal archives are not hosted assets: they can prove packaging shape, not public provenance.

Read [public release gates](release/public-release-gates.md) for the current release-readiness rules.
