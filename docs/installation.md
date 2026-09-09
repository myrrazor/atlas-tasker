# Installation

Atlas supports source builds and hosted release installs. Hosted v1.11.0 assets, checksums,
and attestations are recorded in [v1.11 release evidence](release/v1.11.0-release-evidence.md).

## Build From Source

```bash
go build -o tracker ./cmd/tracker
./tracker --help
./tracker version --json
```

You can keep the binary local to the repo or move it onto your `PATH`. Unstamped source builds report `version: "dev"` in JSON.

## Install From A Published Release

The installer expects a real GitHub release with archives, `checksums.txt`, and GitHub artifact attestations. It verifies checksums before installing and verifies attestations by default.

```bash
VERSION=v1.11.0 BIN_DIR="$HOME/.local/bin" sh ./scripts/install.sh
"$HOME/.local/bin/tracker" version --json
```

The one-line form is convenient once a release is trusted and hosted proof is green:

```bash
curl -fsSL https://raw.githubusercontent.com/myrrazor/atlas-tasker/v1.11.0/scripts/install.sh | sh
```

Do not run installer commands copied from untrusted issues, comments, or chat transcripts. Prefer the checked-in script or a command you can inspect.



## Coding Agent Integrations

After `tracker init`, an interactive terminal can detect local coding agents (Claude Code, Codex, Cursor, OpenClaw, Grok) and install Atlas guidance into the ones you select:

```bash
tracker init --integrations
# or later
tracker integrations detect
tracker integrations install
tracker integrations install --targets claude,cursor
```

Non-interactive environments skip the prompt. Use `--targets` or a concrete install subcommand in CI.

## Update An Existing Install

Once tracker is on your `PATH`, prefer the built-in updater over re-running the curl installer:

```bash
tracker update --check
tracker update --yes
```

`tracker update` resolves the latest GitHub release (or `--version`), downloads the platform archive, verifies `checksums.txt`, verifies attestations with `gh attestation verify` by default, then atomically replaces the running binary. Use `--dry-run` to preview, `--skip-attestations` only for local rehearsals, and `--json` for scripts.

## Verify Before Installing

Verify the downloaded archive first:

```bash
VERSION=v1.11.0 ./scripts/verify-release.sh ./tracker_1.11.0_darwin_arm64.tar.gz
```

`scripts/verify-release.sh` checks `checksums.txt` and, by default, GitHub artifact attestations through `gh attestation verify`. Set `VERIFY_ATTESTATIONS=0` only for local rehearsals or intentionally unattested artifacts.

## Expected Failure Modes

- No hosted archive exists yet: installation should fail with a missing release asset.
- `gh` is not authenticated: attestation verification may fail even when checksums are correct.
- The token cannot read release or Actions settings: hosted proof remains blocked.
- Local rehearsal archives are not hosted assets: they can prove packaging shape, not public provenance.
- `RELEASE_BASE_URL must use https://`: local rehearsals must set `ALLOW_INSECURE_RELEASE_BASE_URL=1` and use loopback only.
